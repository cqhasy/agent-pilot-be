package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agent-pilot/agent-pilot-be/agent/expert"
	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	agenttool "github.com/agent-pilot/agent-pilot-be/agent/tool"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	callbacksHelper "github.com/cloudwego/eino/utils/callbacks"
	"github.com/gin-gonic/gin"
)

func (h *wsHub) startRun(ctx context.Context, session *wsSession, requestID, message, stepID string, plan *agentplan.Plan) {
	runVer, ok := session.beginRun(message, stepID, requestID, plan)
	if !ok {
		session.broadcast(wsOutput{Type: wsEventError, SessionID: session.id, RequestID: requestID, Data: "session is already running"})
		return
	}

	if plan != nil {
		session.broadcast(wsOutput{Type: wsEventPlanUpdated, SessionID: session.id, RequestID: requestID, Data: gin.H{
			"plan": plan,
		}})
	}
	go h.invoke(ctx, session, requestID, message, stepID, nil, "", runVer)
}

func (h *wsHub) startResume(ctx context.Context, session *wsSession, requestID string, resume *humanResume, interruptID string) {
	var snap *memory.WSResumeSnapshot
	if h.mem != nil {
		var err error
		snap, err = h.mem.LoadWSResume(ctx, session.id)
		if err != nil {
			snap = nil
		}
	}
	intr := session.interruptedRunSnapshot()
	want := strings.TrimSpace(interruptID)
	// 回复落在「清空 interruptedRun + preparePlan」之后时内存已无快照；Consume 前从 Mongo 补回 plan/message/expert_id。
	if intr.plan == nil && strings.TrimSpace(intr.message) == "" && snap != nil &&
		want != "" && strings.TrimSpace(snap.InterruptID) == want {
		applyResumeSnapshotToSession(session, snap)
	}

	if h.mem != nil {
		_ = h.mem.ConsumeWSResume(ctx, session.id)
	}

	interrupted := session.interruptedRunSnapshot()
	session.ensureExpertScopeForResume(interrupted.expertID)

	runVer, ok := session.beginRun(interrupted.message, interrupted.stepID, requestID, interrupted.plan)
	if !ok {
		session.broadcast(wsOutput{Type: wsEventError, SessionID: session.id, RequestID: requestID, Data: "session is already running"})
		return
	}
	go h.invoke(ctx, session, requestID, interrupted.message, interrupted.stepID, resume, interruptID, runVer)
}

func (h *wsHub) invoke(ctx context.Context, session *wsSession, requestID, message, stepID string, resume *humanResume, interruptID string, runVer uint64) {
	defer session.clearPendingResumeFallback()
	runCtx, interrupt := compose.WithGraphInterrupt(ctx)
	history := session.attachCancelAndSnapshot(interrupt)

	baseCtx := runCtx
	baseCtx = context.WithValue(baseCtx, wsCtxSessionKey, session)
	baseCtx = context.WithValue(baseCtx, wsCtxRequestIDKey, requestID)
	baseCtx = context.WithValue(baseCtx, wsCtxRunVerKey, runVer)
	baseCtx = agenttool.WithPlanStepUpdater(baseCtx, planStepUpdater(session, requestID, runVer))
	baseCtx = expert.WithExpertSession(baseCtx, session)
	if reg := expertRegistryForHub(h); reg != nil {
		baseCtx = expert.WithRegistry(baseCtx, reg)
	}

	var outRole schema.RoleType
	var outContent strings.Builder
	handler := buildStreamingHandler(session, requestID, runVer, &outRole, &outContent)

	firstPass := true
	for {
		if !session.isCurrentRunVer(runVer) {
			session.finishRun(runVer)
			return
		}

		currentStepID := strings.TrimSpace(stepID)
		if currentStepID == "" {
			currentStepID = session.activeStepIDSnapshot()
		}
		iterCtx := context.WithValue(baseCtx, wsCtxStepIDKey, currentStepID)

		input := chatRuntimeInput{History: history, UserInput: message}
		activeRT := h.pickComposeRuntime(session)
		ckID := composeCheckpointID(session)
		opts := []compose.Option{compose.WithCheckPointID(ckID), compose.WithCallbacks(handler)}
		if resume != nil && firstPass {
			iterCtx = compose.ResumeWithData(iterCtx, interruptID, resume)
			session.setPendingResumeFallback(resume)
			// compile 里 InputBuildNodeKey 的 StatePreHandler 会用 input.History 覆盖 state.History；
			// 传空 History 会抹掉 checkpoint 已恢复的对话，模型等价于在极少上下文下重写 →「整篇重新生成」。
			// history 与 attachCancelAndSnapshot 一致（含 handleUserMessage 里刚追加的审核回复）；UserInput 留空以免 PreHandler 再追加一遍。
			input = chatRuntimeInput{History: history, UserInput: ""}
		} else {
			// 用户点击中断后继续同一轮执行时 resume 仍为 nil，但必须沿用 compose checkpoint，不能用 ForceNewRun 清空进度。
			intr := session.interruptedRunSnapshot()
			skipForceNew := firstPass && resume == nil &&
				(strings.TrimSpace(intr.message) != "" || intr.plan != nil)
			if !skipForceNew {
				opts = append(opts, compose.WithForceNewRun())
			}
			if !firstPass {
				input = chatRuntimeInput{History: session.composeHistorySnapshot(), UserInput: nextStepPrompt(session.activePlanSnapshot(), currentStepID)}
			}
		}

		beforeLen := outContent.Len()
		activeRun := session.snapshotActiveRun()
		activeRun.stepID = currentStepID
		persistedTurn := false
		persistTurn := func() {
			if persistedTurn {
				return
			}
			persistedTurn = true
			if strings.TrimSpace(input.UserInput) != "" {
				if !session.lastHistoryIsUser(input.UserInput) {
					msg := schema.UserMessage(input.UserInput)
					session.appendHistory(msg)
					h.appendWSHistory(ctx, session, msg)
				}
			}
			if outContent.Len() > beforeLen {
				if outRole == "" {
					outRole = schema.Assistant
				}
				msg := &schema.Message{Role: outRole, Content: outContent.String()[beforeLen:]}
				session.appendHistory(msg)
				h.appendWSHistory(ctx, session, msg)
			}
		}
		sr, err := activeRT.runner.Stream(iterCtx, input, opts...)
		if err != nil {
			persistTurn()
			if info, ok := compose.ExtractInterruptInfo(err); ok {
				h.pauseRun(ctx, session, requestID, message, currentStepID, activeRun, info, runVer)
				return
			}
			h.failRunStep(ctx, session, requestID, currentStepID, err, runVer)
			return
		}
		if err = drainStream(sr); err != nil {
			persistTurn()
			if info, ok := compose.ExtractInterruptInfo(err); ok {
				h.pauseRun(ctx, session, requestID, message, currentStepID, activeRun, info, runVer)
				return
			}
			h.failRunStep(ctx, session, requestID, currentStepID, err, runVer)
			return
		}
		if !session.isCurrentRunVer(runVer) {
			session.finishRun(runVer)
			return
		}

		persistTurn()
		if currentStepID != "" {
			status, ok := session.planStepStatusSnapshot(currentStepID)
			if ok {
				if status == agentplan.StepStatusFailed {
					break
				}
				if !isTerminalStepStatus(status) {
					break
				}
			}
		}

		nextStepID := session.nextPendingStepIDSnapshot()
		if nextStepID == "" {
			break
		}
		stepID = nextStepID
		firstPass = false
		resume = nil
		history = session.composeHistorySnapshot()
	}

	session.clearInterruptedRun()
	if h.mem != nil {
		// 正常跑完时清掉 Mongo 里缓存的中断元数据，避免重连后误以为仍处于中断态
		_ = h.mem.ConsumeWSResume(ctx, session.id)
	}
	session.broadcast(wsOutput{Type: wsEventDone, SessionID: session.id, RequestID: requestID})
	session.finishRun(runVer)
}

func (h *wsHub) pauseRun(ctx context.Context, session *wsSession,
	requestID, message, stepID string, activeRun wsInterruptedState,
	info *compose.InterruptInfo, runVer uint64) {
	if strings.TrimSpace(activeRun.message) == "" {
		activeRun.message = message
	}
	activeRun.stepID = stepID
	activeRun.requestID = requestID
	activeRun.plan = session.activePlanSnapshot()
	if strings.TrimSpace(activeRun.expertID) == "" {
		activeRun.expertID = strings.TrimSpace(session.activeExpertIDSnapshot())
	}
	session.setInterruptedRun(activeRun)
	session.releaseExpertBranchUserGate()

	h.handleInvokeInterrupt(session, requestID, info)
	h.persistWSResumeAfterPause(ctx, session, requestID, info, activeRun)
	session.finishRun(runVer)
}

func (h *wsHub) failRunStep(ctx context.Context, session *wsSession, requestID, stepID string, err error, runVer uint64) {
	if strings.TrimSpace(stepID) != "" {
		_, _ = h.setPlanStepStatus(ctx, session, requestID, runVer, stepID, agentplan.StepStatusFailed, err.Error())
	}
	session.finishRun(runVer)
	session.broadcast(wsOutput{Type: wsEventError, SessionID: session.id, RequestID: requestID, Data: err.Error()})
}

func nextStepPrompt(plan *agentplan.Plan, stepID string) string {
	stepID = strings.TrimSpace(stepID)
	if plan != nil {
		for _, step := range plan.Steps {
			if step.ID == stepID {
				return fmt.Sprintf("Continue the approved plan. Execute only CURRENT_STEP_ID %s: %s. Expected outcome: %s", step.ID, step.Title, step.ExpectedOutcome)
			}
		}
	}
	return "Continue the approved plan. Execute only CURRENT_STEP_ID " + stepID + "."
}

func isTerminalStepStatus(status agentplan.StepStatus) bool {
	return status == agentplan.StepStatusCompleted ||
		status == agentplan.StepStatusSkipped ||
		status == agentplan.StepStatusFailed
}

func drainStream[T any](sr *schema.StreamReader[T]) error {
	if sr == nil {
		return nil
	}
	defer sr.Close()
	for {
		_, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (h *wsHub) handleInvokeInterrupt(session *wsSession, requestID string, info *compose.InterruptInfo) {
	if info == nil || len(info.InterruptContexts) == 0 {
		session.broadcast(wsOutput{Type: wsEventInterrupted, SessionID: session.id, RequestID: requestID, Data: info})
		return
	}
	ictx := info.InterruptContexts[0]

	pause, ok := ictx.Info.(*humanPause)
	if !ok {
		session.broadcast(wsOutput{Type: wsEventInterrupted, SessionID: session.id, RequestID: requestID, Data: gin.H{
			"interrupt_id": ictx.ID,
			"info":         ictx.Info,
		}})
		return
	}
	session.setPendingInterruptID(ictx.ID)
	session.broadcast(wsOutput{Type: pause.Kind, SessionID: session.id, RequestID: requestID, Data: gin.H{
		"interrupt_id": ictx.ID,
		"pause":        pause,
	}})
}

func buildStreamingHandler(session *wsSession, requestID string, runVer uint64, outRole *schema.RoleType, outContent *strings.Builder) callbacks.Handler {
	var lastExpertPreview time.Time
	const expertPreviewMinInterval = 200 * time.Millisecond
	// 专家模式：聊天区只推送去掉 DOC_PREVIEW 正文后的可见增量；预览区只推送标记内正文。
	var prevExpertChatVisible string

	flushExpertPreview := func(force bool) {
		if !session.isCurrentRunVer(runVer) {
			return
		}
		eid := strings.TrimSpace(session.activeExpertIDSnapshot())
		if eid == "" {
			return
		}
		body := outContent.String()
		previewBody := memory.ExtractExpertPreviewMarkdown(body)
		if strings.TrimSpace(previewBody) == "" {
			return
		}
		now := time.Now()
		if !force && now.Sub(lastExpertPreview) < expertPreviewMinInterval {
			return
		}
		lastExpertPreview = now
		payload := PreviewPayloadForExpertContent(eid, previewBody)
		if payload == nil {
			return
		}
		BroadcastExpertPreview(session, requestID, payload)
	}

	broadcastAssistantChat := func(rawChunk string, role schema.RoleType) {
		if !session.isCurrentRunVer(runVer) {
			return
		}
		eid := strings.TrimSpace(session.activeExpertIDSnapshot())
		if eid == "" || role != schema.Assistant {
			session.broadcast(wsOutput{Type: wsEventMessage, SessionID: session.id, RequestID: requestID, Data: gin.H{
				"role":    string(role),
				"content": rawChunk,
			}})
			return
		}
		full := outContent.String()
		cur := memory.StripExpertPreviewRegionsForStream(full)
		var delta string
		if strings.HasPrefix(cur, prevExpertChatVisible) {
			delta = cur[len(prevExpertChatVisible):]
		} else if strings.HasPrefix(prevExpertChatVisible, cur) {
			prevExpertChatVisible = cur
			return
		} else if cur != "" {
			delta = cur
		}
		prevExpertChatVisible = cur
		if delta == "" {
			return
		}
		session.broadcast(wsOutput{Type: wsEventMessage, SessionID: session.id, RequestID: requestID, Data: gin.H{
			"role":    string(role),
			"content": delta,
		}})
	}

	return callbacksHelper.NewHandlerHelper().ChatModel(&callbacksHelper.ModelCallbackHandler{
		OnEnd: func(ctx context.Context, _ *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
			if output == nil || output.Message == nil || output.Message.Content == "" {
				return ctx
			}
			*outRole = output.Message.Role
			outContent.WriteString(output.Message.Content)
			if session.isCurrentRunVer(runVer) {
				broadcastAssistantChat(output.Message.Content, output.Message.Role)
				flushExpertPreview(true)
			}
			return ctx
		},
		OnEndWithStreamOutput: func(ctx context.Context, _ *callbacks.RunInfo, output *schema.StreamReader[*model.CallbackOutput]) context.Context {
			if output == nil {
				return ctx
			}
			defer output.Close()
			for {
				frame, err := output.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil || frame == nil || frame.Message == nil || frame.Message.Content == "" {
					continue
				}
				*outRole = frame.Message.Role
				outContent.WriteString(frame.Message.Content)
				if session.isCurrentRunVer(runVer) {
					broadcastAssistantChat(frame.Message.Content, frame.Message.Role)
					flushExpertPreview(false)
				}
			}
			flushExpertPreview(true)
			return ctx
		},
	}).Handler()
}
