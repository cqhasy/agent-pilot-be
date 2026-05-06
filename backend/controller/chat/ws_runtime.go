package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agent-pilot/agent-pilot-be/agent/expert"
	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	agenttool "github.com/agent-pilot/agent-pilot-be/agent/tool"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
)

// newComposeRuntime 编译主会话 compose（handoff 时可重写 state 以接入专家线程种子）。
func newComposeRuntime(ctx context.Context, chatModel model.ToolCallingChatModel, tools []einotool.BaseTool, system string, checkpointStore compose.CheckPointStore, experts *expert.Registry) (*composeRuntime, error) {
	schema.RegisterName[*chatRuntimeState]("agent_pilot_chat_runtime_state")
	schema.RegisterName[*humanResume]("agent_pilot_human_resume")
	schema.RegisterName[*humanPause]("agent_pilot_human_pause")
	return compileComposeRuntime(ctx, chatModel, tools, system, checkpointStore, composeCompileOptions{
		graphName:         GraphName,
		experts:           experts,
		mainHandoffRewire: true,
		isExpertGraph:     false,
	})
}

func planStepProtocolPrompt() string {
	return `Plan step status protocol:
- When an approved plan is present, you MUST update step state explicitly with the update_plan_step tool. This is not optional.
- The backend will provide CURRENT_STEP_ID in the system prompt when a plan is active. Unless you are explicitly switching steps, ALWAYS use CURRENT_STEP_ID as step_id.
- For each step you work on, emit the following updates in order:
  1) update_plan_step {step_id: CURRENT_STEP_ID, status: "running"} BEFORE any business/tool action for that step.
  2) update_plan_step {step_id: CURRENT_STEP_ID, status: "completed"} ONLY AFTER that step’s expected outcome is satisfied.
  - If the step cannot be completed, use status "failed" with a short note.
- Never start the next step while the current step is still pending/running.
- Do not mark a step completed just because a business tool call returned. A step may require multiple tools or no tools.
- For steps that require user approval or review (creative drafts, legal/copy checks), do NOT mark "completed" until the user has actually responded via request_user_input (or equivalent), not when you assume approval.
- Use the exact step_id from the approved plan.`
}

func userInputProtocolPrompt() string {
	return `Missing input protocol:
- If required user parameters are missing, call request_user_input instead of asking in plain text.
- Provide question and fields in request_user_input.
- The runtime will pause and wait for user reply.
- After resume, continue the same step and use request_user_input answer to finish execution.`
}

func expertHandbackProtocolPrompt() string {
	return `Specialist / main handback (mandatory when appropriate):
- You have tools handoff_to_expert and release_expert. The main agent routes work to specialists; specialists MUST give control back when appropriate.
- After the user has approved the creative deliverable via request_user_input (confirmed / no further edits needed for this phase), AND you have applied any required updates from that approval (e.g. marked plan step completed, persisted to Feishu if that step requires it), call release_expert in the SAME turn before finishing, unless the user explicitly wants another specialist-only action immediately (e.g. more drafting in the same thread).
- release_expert restores full session history routing for the main agent — required before answering prompts like “分享到群”, general coordination, or tasks outside your specialist scope.
- Do NOT stay in specialist mode indefinitely after the delegated slice is done and approved; failing to release causes “专家与主 agent 状态不一致” from the user’s perspective.`
}

func creativeReviewProtocolPrompt() string {
	return `Creative deliverable review protocol:
- Creative deliverables include documents, reports, copywriting, outlines, slides/PPT, presentations, images, diagrams, plans, proposals, and other subjective artifacts.
- In WebSocket UI: put the draft body between <!--DOC_PREVIEW_START--> and <!--DOC_PREVIEW_END--> so the preview pane shows only the artifact; ask for approval in this session — do not tell the user to open Feishu solely to review the creative draft before it is approved here.
- After creating or materially revising a creative deliverable, call request_user_input to ask whether they approve it or want changes (after they have seen the preview).
- The review question should explicitly tell the user they can approve, request edits, or ask for another revision.
- If the user requests changes, apply the feedback and repeat the review loop.
- Only after approval, persist to Feishu/docs or share externally if the plan requires it.
- Do not mark the final creative-deliverable step completed until the user has approved it or clearly says no more changes are needed.
- If the user explicitly asked for no review, only then skip this protocol.`
}

func planExecutionPrompt(plan *agentplan.Plan) string {
	if plan == nil {
		return ""
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return ""
	}
	return "Approved execution plan. Follow it and report step status using update_plan_step:\n" + string(data)
}

func buildExpertFocusedSystem(def *expert.Definition) string {
	return fmt.Sprintf(`You are the specialist "%s" (id=%s).
You are running in an ISOLATED thread: do NOT rely on any prior "main assistant" chat — only messages in this thread and the delegated task apply.

%s`,
		def.Name, def.ID, strings.TrimSpace(def.Instruction))
}

func runtimeModelInputMessages(ctx context.Context, system string, history []*schema.Message, rt *composeRuntime) []*schema.Message {
	msgs := cloneMessages(history)
	finalSystem := system
	if session, _ := ctx.Value(wsCtxSessionKey).(*wsSession); session != nil {
		if plan := session.activePlanSnapshot(); plan != nil {
			msgs = append([]*schema.Message{schema.UserMessage(planExecutionPrompt(plan))}, msgs...)
		}
		stepID := strings.TrimSpace(preferredStepID(ctx, session))
		// 主图 + 已进入专家模式：用注册表里的 Instruction 覆盖系统提示（同一会话尚未切到独立专家 runner 时的首轮）。
		if rt != nil && !rt.isExpertGraph {
			if reg := expert.RegistryFrom(ctx); reg != nil {
				if eid := strings.TrimSpace(session.activeExpertIDSnapshot()); eid != "" {
					if def, ok := reg.Get(eid); ok {
						finalSystem = buildExpertFocusedSystem(def)
					}
				}
			}
		}
		if stepID != "" {
			finalSystem = finalSystem + "\n\nCURRENT_STEP_ID: " + stepID
		}
	}
	proto := "\n\n" + planStepProtocolPrompt() + "\n\n" + userInputProtocolPrompt() + "\n\n" + creativeReviewProtocolPrompt()
	if session, _ := ctx.Value(wsCtxSessionKey).(*wsSession); session != nil && strings.TrimSpace(session.activeExpertIDSnapshot()) != "" {
		proto += "\n\n" + expertHandbackProtocolPrompt()
	}
	return sanitizeMessagesForModel(withSystemMessage(finalSystem+proto, msgs))
}

func preferredStepID(ctx context.Context, session *wsSession) string {
	if stepID, _ := ctx.Value(wsCtxStepIDKey).(string); strings.TrimSpace(stepID) != "" {
		return strings.TrimSpace(stepID)
	}
	if session != nil {
		return strings.TrimSpace(session.activeStepIDSnapshot())
	}
	return ""
}

func sanitizeMessagesForModel(messages []*schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		cp := *msg
		if strings.TrimSpace(cp.Content) == "" {
			switch cp.Role {
			case schema.System, schema.User, schema.Assistant, schema.Tool:
				cp.Content = " "
			}
		}
		out = append(out, &cp)
	}
	return out
}

func (rt *composeRuntime) executeToolCalls(ctx context.Context, msg *schema.Message) ([]*schema.Message, error) {
	if msg == nil || len(msg.ToolCalls) == 0 {
		return nil, nil
	}
	// 如果有tool要执行，先判断是否是从interrupt恢复的状态，检索用户的回复，如果没有检查tool是否需要执行中断，这里上游只有一个model。应该最多有一个toolcall，但为了扩展能力，还是这么写
	resumeAction, hasResume := getHumanResume(ctx)
	if sess, ok := ctx.Value(wsCtxSessionKey).(*wsSession); ok && sess != nil {
		// compose 已给出完整 Reason 时丢弃兜底，避免同一次 invoke 内后续 tool 批次误用旧回复。
		if hasResume && resumeAction != nil && strings.TrimSpace(resumeAction.Reason) != "" {
			sess.clearPendingResumeFallback()
		}
		if !hasResume {
			if fb := sess.consumePendingResumeFallbackOnce(); fb != nil {
				resumeAction = fb
				hasResume = true
			}
		} else if resumeAction == nil {
			if fb := sess.consumePendingResumeFallbackOnce(); fb != nil {
				resumeAction = fb
				hasResume = true
			}
		} else if strings.TrimSpace(resumeAction.Reason) == "" {
			if fb := sess.peekPendingResumeFallback(); fb != nil && strings.TrimSpace(fb.Reason) != "" {
				resumeAction.Reason = strings.TrimSpace(fb.Reason)
				sess.clearPendingResumeFallback()
			}
		}
	}
	calls := cloneToolCalls(msg.ToolCalls)

	// 在有正在运行的plan时，我们应当保证,正常的tool的执行是在一个running状态的step下，
	if session, _ := ctx.Value(wsCtxSessionKey).(*wsSession); session != nil {
		stepID := strings.TrimSpace(preferredStepID(ctx, session))
		if stepID != "" && !session.isActiveStepRunning() {
			needsRunning := false
			for _, call := range calls {
				if strings.EqualFold(call.Function.Name, agenttool.PlanStepToolName) {
					continue
				}

				if strings.EqualFold(call.Function.Name, agenttool.RequestUserInputToolName) {
					continue
				}
				if strings.EqualFold(call.Function.Name, agenttool.HandoffToExpertToolName) ||
					strings.EqualFold(call.Function.Name, agenttool.ReleaseExpertToolName) {
					continue
				}
				// Any non-bookkeeping tool call counts as business action.
				needsRunning = true
				break
			}
			// 兜底更新step为running状态
			if needsRunning {
				args, _ := json.Marshal(agenttool.PlanStepUpdate{StepID: stepID, Status: string(agentplan.StepStatusRunning), Note: "running"})
				auto := schema.ToolCall{
					ID: "auto_running_" + stepID,
					Function: schema.FunctionCall{
						Name:      agenttool.PlanStepToolName,
						Arguments: string(args),
					},
				}
				calls = append([]schema.ToolCall{auto}, calls...)
			}
		}
	}

	if hasResume {
		applyResumeToUserInputToolCalls(calls, resumeAction)
		applyResumeToToolCalls(calls, resumeAction)
	} else if pause := rt.pauseForToolCalls(calls); pause != nil {
		return nil, compose.Interrupt(ctx, pause)
	}

	output := make([]*schema.Message, 0, len(calls))
	for _, call := range calls {
		if pause := rt.pauseOnUserInputRequest(call); pause != nil {
			return nil, compose.Interrupt(ctx, pause)
		}
		if hasResume && resumeAction.Action == wsInputRejectTool && matchesResumeCall(resumeAction, call) {
			reason := strings.TrimSpace(resumeAction.Reason)
			if reason == "" {
				reason = "rejected by user"
			}
			output = append(output, schema.ToolMessage(reason, call.ID, schema.WithToolName(call.Function.Name)))
			continue
		}
		output = append(output, rt.runToolCall(ctx, call))
	}
	return output, nil
}

func (rt *composeRuntime) runToolCall(ctx context.Context, call schema.ToolCall) *schema.Message {
	tool, ok := rt.tools[call.Function.Name]
	if !ok {
		return schema.ToolMessage("tool is not invokable: "+call.Function.Name, call.ID, schema.WithToolName(call.Function.Name))
	}

	if session, _ := ctx.Value(wsCtxSessionKey).(*wsSession); session != nil {
		if requestID, _ := ctx.Value(wsCtxRequestIDKey).(string); strings.TrimSpace(requestID) != "" {
			runVer, _ := ctx.Value(wsCtxRunVerKey).(uint64)
			if runVer == 0 || session.isCurrentRunVer(runVer) {
				session.broadcast(wsOutput{Type: wsEventToolResult, SessionID: session.id, RequestID: requestID, Data: gin.H{
					"phase":        "start",
					"tool_call_id": call.ID,
					"tool_name":    call.Function.Name,
					"arguments":    call.Function.Arguments,
				}})
			}
		}
	}

	result, err := tool.InvokableRun(ctx, call.Function.Arguments)
	if err != nil {
		result = "tool failed: " + err.Error()
	}
	// 这里我又取了一遍，是因为在调试时遇到这样的问题：invoke时，这个过程期间用户取消了对话，而你如果继续用上次取的会导致对话中止却还在广播消息，所以前后取两次，作用域区分
	if session, _ := ctx.Value(wsCtxSessionKey).(*wsSession); session != nil {
		if requestID, _ := ctx.Value(wsCtxRequestIDKey).(string); strings.TrimSpace(requestID) != "" {
			runVer, _ := ctx.Value(wsCtxRunVerKey).(uint64)
			if runVer == 0 || session.isCurrentRunVer(runVer) {
				session.broadcast(wsOutput{Type: wsEventToolResult, SessionID: session.id, RequestID: requestID, Data: gin.H{
					"phase":        "end",
					"tool_call_id": call.ID,
					"tool_name":    call.Function.Name,
					"ok":           err == nil,
					"result":       result,
				}})
			}
		}
	}
	toolMsg := schema.ToolMessage(result, call.ID, schema.WithToolName(call.Function.Name))
	rt.maybeBroadcastExpertMode(ctx, call)
	return toolMsg
}

func (rt *composeRuntime) maybeBroadcastExpertMode(ctx context.Context, call schema.ToolCall) {
	session, _ := ctx.Value(wsCtxSessionKey).(*wsSession)
	if rt == nil || session == nil || rt.experts == nil {
		return
	}
	name := strings.TrimSpace(call.Function.Name)
	if !strings.EqualFold(name, agenttool.HandoffToExpertToolName) && !strings.EqualFold(name, agenttool.ReleaseExpertToolName) {
		return
	}
	if requestID, _ := ctx.Value(wsCtxRequestIDKey).(string); strings.TrimSpace(requestID) == "" {
		return
	}
	runVer, _ := ctx.Value(wsCtxRunVerKey).(uint64)
	if runVer != 0 && !session.isCurrentRunVer(runVer) {
		return
	}
	requestID, _ := ctx.Value(wsCtxRequestIDKey).(string)
	eid := strings.TrimSpace(session.activeExpertIDSnapshot())
	payload := gin.H{"expert_id": eid, "general_mode": eid == ""}
	if eid != "" {
		if def, ok := rt.experts.Get(eid); ok {
			payload["name"] = def.Name
			payload["description"] = def.Description
		}
	}
	session.broadcast(wsOutput{Type: wsEventExpertMode, SessionID: session.id, RequestID: requestID, Data: payload})
}

func (rt *composeRuntime) pauseForToolCalls(calls []schema.ToolCall) *humanPause {
	for _, call := range calls {
		for _, guard := range rt.guards {
			if pause := guard(call); pause != nil {
				return pause
			}
		}
	}
	return nil
}

func (rt *composeRuntime) pauseOnMissingRequired(call schema.ToolCall) *humanPause {
	missing := rt.missingRequired(call)
	if len(missing) == 0 {
		return nil
	}
	return &humanPause{
		Kind:      wsEventInputRequired,
		Message:   "tool call needs missing required parameters",
		ToolName:  call.Function.Name,
		ToolCalls: []toolCallView{viewToolCall(call, "missing_params")},
		Missing:   missing,
		CanModify: true,
		CanReject: true,
		CreatedAt: time.Now(),
	}
}

func (rt *composeRuntime) pauseOnDangerousToolCall(call schema.ToolCall) *humanPause {
	if !isDangerousToolCall(call) {
		return nil
	}
	return &humanPause{
		Kind:      wsEventToolApprovalRequired,
		Message:   "dangerous tool call requires approval",
		ToolCalls: []toolCallView{viewToolCall(call, "dangerous")},
		CanModify: true,
		CanReject: true,
		CreatedAt: time.Now(),
	}
}

func (rt *composeRuntime) pauseOnUserInputRequest(call schema.ToolCall) *humanPause {
	if !strings.EqualFold(call.Function.Name, agenttool.RequestUserInputToolName) {
		return nil
	}
	var args map[string]any
	_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
	question, _ := args["question"].(string)
	answer, _ := args["answer"].(string)
	if strings.TrimSpace(answer) != "" {
		return nil
	}
	msg := strings.TrimSpace(question)
	if msg == "" {
		msg = "missing required user input"
	}
	var missing []string
	if fields, ok := args["fields"].([]any); ok {
		for _, f := range fields {
			if s, ok := f.(string); ok && strings.TrimSpace(s) != "" {
				missing = append(missing, strings.TrimSpace(s))
			}
		}
	}
	return &humanPause{
		Kind:     wsEventInputRequired,
		Message:  msg,
		ToolName: strings.TrimSpace(call.Function.Name),
		Missing:  missing,
		// This pause is not an approval workflow; user should reply with normal message.
		ToolCalls: nil,
		CanModify: false,
		CanReject: false,
		CreatedAt: time.Now(),
	}
}

func (rt *composeRuntime) missingRequired(call schema.ToolCall) []string {
	info := rt.infos[call.Function.Name]
	if info == nil || info.ParamsOneOf == nil {
		return nil
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil || js == nil || len(js.Required) == 0 {
		return nil
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return js.Required
	}
	var missing []string
	for _, key := range js.Required {
		value, ok := args[key]
		if !ok || value == nil || value == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func shouldWaitForUserInput(content string) bool {
	text := strings.ToLower(strings.TrimSpace(content))
	if text == "" {
		return false
	}
	signals := []string{
		"请提供", "请告诉我", "需要你提供", "请确认", "请选择",
		"provide", "please provide", "tell me", "confirm", "which", "what is",
	}
	for _, signal := range signals {
		if strings.Contains(text, signal) {
			return true
		}
	}
	return false
}

func applyResumeToUserInputToolCalls(calls []schema.ToolCall, resume *humanResume) {
	if resume == nil || strings.TrimSpace(resume.Reason) == "" {
		return
	}
	answer := strings.TrimSpace(resume.Reason)
	for i := range calls {
		call := &calls[i]
		if !strings.EqualFold(call.Function.Name, agenttool.RequestUserInputToolName) {
			continue
		}
		var args map[string]any
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil || args == nil {
			args = map[string]any{}
		}
		if v, ok := args["answer"].(string); ok && strings.TrimSpace(v) != "" {
			continue
		}
		args["answer"] = answer
		b, _ := json.Marshal(args)
		call.Function.Arguments = string(b)
	}
}

func shouldPauseForNonTerminalStep(ctx context.Context) bool {
	session, _ := ctx.Value(wsCtxSessionKey).(*wsSession)
	if session == nil {
		return false
	}
	stepID := strings.TrimSpace(preferredStepID(ctx, session))
	if stepID == "" {
		return false
	}
	status, ok := session.planStepStatusSnapshot(stepID)
	if !ok {
		return false
	}
	return status != agentplan.StepStatusCompleted &&
		status != agentplan.StepStatusSkipped &&
		status != agentplan.StepStatusFailed
}
