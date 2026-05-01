package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

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
	interrupted := session.interruptedRunSnapshot()
	runVer, ok := session.beginRun(interrupted.message, interrupted.stepID, requestID, interrupted.plan)
	if !ok {
		session.broadcast(wsOutput{Type: wsEventError, SessionID: session.id, RequestID: requestID, Data: "session is already running"})
		return
	}
	go h.invoke(ctx, session, requestID, interrupted.message, interrupted.stepID, resume, interruptID, runVer)
}

func (h *wsHub) invoke(ctx context.Context, session *wsSession, requestID, message, stepID string, resume *humanResume, interruptID string, runVer uint64) {
	runCtx, interrupt := compose.WithGraphInterrupt(ctx)
	history := session.attachCancelAndSnapshot(interrupt)

	baseCtx := runCtx
	baseCtx = context.WithValue(baseCtx, wsCtxSessionKey, session)
	baseCtx = context.WithValue(baseCtx, wsCtxRequestIDKey, requestID)
	baseCtx = context.WithValue(baseCtx, wsCtxRunVerKey, runVer)
	baseCtx = agenttool.WithPlanStepUpdater(baseCtx, planStepUpdater(session, requestID, runVer))

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
		opts := []compose.Option{compose.WithCheckPointID(session.id), compose.WithCallbacks(handler)}
		if resume != nil && firstPass {
			iterCtx = compose.ResumeWithData(iterCtx, interruptID, resume)
			input = chatRuntimeInput{}
		} else {
			opts = append(opts, compose.WithForceNewRun())
			if !firstPass {
				input = chatRuntimeInput{History: session.historySnapshot(), UserInput: nextStepPrompt(session.activePlanSnapshot(), currentStepID)}
			}
		}

		beforeLen := outContent.Len()
		activeRun := session.snapshotActiveRun()
		activeRun.stepID = currentStepID
		sr, err := h.runtime.runner.Stream(iterCtx, input, opts...)
		if err != nil {
			if info, ok := compose.ExtractInterruptInfo(err); ok {
				h.pauseRun(session, requestID, message, currentStepID, activeRun, info, runVer)
				return
			}
			h.failRunStep(ctx, session, requestID, currentStepID, err, runVer)
			return
		}
		if err = drainStream(sr); err != nil {
			if info, ok := compose.ExtractInterruptInfo(err); ok {
				h.pauseRun(session, requestID, message, currentStepID, activeRun, info, runVer)
				return
			}
			h.failRunStep(ctx, session, requestID, currentStepID, err, runVer)
			return
		}
		if !session.isCurrentRunVer(runVer) {
			session.finishRun(runVer)
			return
		}

		if strings.TrimSpace(input.UserInput) != "" {
			session.appendHistory(schema.UserMessage(input.UserInput))
		}
		if outContent.Len() > beforeLen {
			if outRole == "" {
				outRole = schema.Assistant
			}
			session.appendHistory(&schema.Message{Role: outRole, Content: outContent.String()[beforeLen:]})
		}
		if currentStepID != "" {
			status, ok := session.planStepStatusSnapshot(currentStepID)
			if ok {
				if status == agentplan.StepStatusFailed {
					break
				}
				// Strict sequencing: never advance to the next step until the current one is terminal.
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
		history = session.historySnapshot()
	}

	session.clearInterruptedRun()
	session.broadcast(wsOutput{Type: wsEventDone, SessionID: session.id, RequestID: requestID})
	session.finishRun(runVer)
}

func (h *wsHub) pauseRun(session *wsSession, requestID, message, stepID string, activeRun wsInterruptedState, info *compose.InterruptInfo, runVer uint64) {
	if strings.TrimSpace(activeRun.message) == "" {
		activeRun.message = message
	}
	activeRun.stepID = stepID
	activeRun.requestID = requestID
	// Use the freshest plan snapshot at pause time to avoid resuming with stale step states.
	activeRun.plan = session.activePlanSnapshot()
	session.setInterruptedRun(activeRun)
	h.handleInvokeInterrupt(session, requestID, info)
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
	return callbacksHelper.NewHandlerHelper().ChatModel(&callbacksHelper.ModelCallbackHandler{
		OnEnd: func(ctx context.Context, _ *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
			if output == nil || output.Message == nil || output.Message.Content == "" {
				return ctx
			}
			*outRole = output.Message.Role
			outContent.WriteString(output.Message.Content)
			if session.isCurrentRunVer(runVer) {
				session.broadcast(wsOutput{Type: wsEventMessage, SessionID: session.id, RequestID: requestID, Data: gin.H{
					"role":    string(output.Message.Role),
					"content": output.Message.Content,
				}})
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
					session.broadcast(wsOutput{Type: wsEventMessage, SessionID: session.id, RequestID: requestID, Data: gin.H{
						"role":    string(frame.Message.Role),
						"content": frame.Message.Content,
					}})
				}
			}
			return ctx
		},
	}).Handler()
}
