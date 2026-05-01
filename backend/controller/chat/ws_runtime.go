package chat

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	agenttool "github.com/agent-pilot/agent-pilot-be/agent/tool"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
)

// newComposeRuntime 这个主要做了几件事：
// 1.定义好主要的编排图：构造提示词节点-> 模型相关节点 ->无工具执行（end）
//
//	|-> 工具节点（执行）->回到模型节点
//
// 2.定义runner 暴露的state结构与每个节点对state的前后操作。
// 3. 注册checkpointStore
func newComposeRuntime(ctx context.Context, chatModel model.ToolCallingChatModel, tools []einotool.BaseTool, system string) (*composeRuntime, error) {
	/*
		eino文档指出对于用户自定义类型，checkPoint保存需要提前注册类型
		详见此处https://www.cloudwego.io/zh/docs/eino/core_modules/chain_and_graph_orchestration/checkpoint_interrupt/#%E6%B3%A8%E5%86%8C%E5%BA%8F%E5%88%97%E5%8C%96%E6%96%B9%E6%B3%95
	*/
	schema.RegisterName[*chatRuntimeState]("agent_pilot_chat_runtime_state")
	schema.RegisterName[*humanResume]("agent_pilot_human_resume")
	schema.RegisterName[*humanPause]("agent_pilot_human_pause")

	infos := make([]*schema.ToolInfo, 0, len(tools))
	invokable := make(map[string]einotool.InvokableTool, len(tools))
	infoByName := make(map[string]*schema.ToolInfo, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
		infoByName[info.Name] = info
		if it, ok := t.(einotool.InvokableTool); ok {
			invokable[info.Name] = it
		}
	}

	modelWithTools, err := chatModel.WithTools(infos)
	if err != nil {
		return nil, err
	}

	rt := &composeRuntime{
		tools:  invokable,
		infos:  infoByName,
		guards: make([]toolCallPauseGuard, 0, 2),
		system: system,
	}
	// 这里目前只定义了两个比较宽泛的pause标准（危险操作与参数缺失）,先执行参数检查再执行危险操作判别感觉更合理些
	rt.guards = append(rt.guards, rt.pauseOnUserInputRequest, rt.pauseOnMissingRequired, rt.pauseOnDangerousToolCall)

	graph := compose.NewGraph[chatRuntimeInput, *schema.Message](compose.WithGenLocalState(func(ctx context.Context) *chatRuntimeState {
		return &chatRuntimeState{}
	}))

	buildInput := compose.InvokableLambda(func(ctx context.Context, input chatRuntimeInput) ([]*schema.Message, error) {
		return runtimeModelInputMessages(ctx, system, input.History), nil
	})
	if err := graph.AddLambdaNode(InputBuildNodeKey, buildInput,
		compose.WithStatePreHandler(func(ctx context.Context, input chatRuntimeInput, state *chatRuntimeState) (chatRuntimeInput, error) {
			state.History = cloneMessages(input.History)
			if strings.TrimSpace(input.UserInput) != "" {
				state.History = append(state.History, schema.UserMessage(input.UserInput))
			}
			input.History = cloneMessages(state.History)
			return input, nil
		}),
	); err != nil {
		return nil, err
	}

	modelInput := compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
		return cloneMessages(input), nil
	})
	if err := graph.AddLambdaNode(ModelInputNodeKey, modelInput,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, state *chatRuntimeState) ([]*schema.Message, error) {
			return runtimeModelInputMessages(ctx, system, state.History), nil
		}),
	); err != nil {
		return nil, err
	}

	if err := graph.AddChatModelNode(ModelNodeKey, modelWithTools,
		compose.WithStatePostHandler(func(ctx context.Context, output *schema.Message, state *chatRuntimeState) (*schema.Message, error) {
			if output != nil {
				state.History = append(state.History, output)
			}
			return output, nil
		}),
	); err != nil {
		return nil, err
	}

	toolExecutor := compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) ([]*schema.Message, error) {
		return rt.executeToolCalls(ctx, msg)
	})
	if err := graph.AddLambdaNode(ToolExecNodeKey, toolExecutor,
		compose.WithStatePostHandler(func(ctx context.Context, output []*schema.Message, state *chatRuntimeState) ([]*schema.Message, error) {
			state.History = append(state.History, output...)
			return output, nil
		}),
	); err != nil {
		return nil, err
	}

	inputPause := compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) (*schema.Message, error) {
		if msg == nil {
			return nil, nil
		}
		if shouldWaitForUserInput(msg.Content) {
			return nil, compose.Interrupt(ctx, &humanPause{
				Kind:      wsEventInputRequired,
				Message:   strings.TrimSpace(msg.Content),
				CanModify: true,
				CanReject: true,
				CreatedAt: time.Now(),
			})
		}
		return msg, nil
	})
	if err := graph.AddLambdaNode(InputPauseNodeKey, inputPause); err != nil {
		return nil, err
	}

	if err := graph.AddEdge(compose.START, InputBuildNodeKey); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(InputBuildNodeKey, ModelNodeKey); err != nil {
		return nil, err
	}
	if err := graph.AddBranch(ModelNodeKey, compose.NewGraphBranch(func(ctx context.Context, msg *schema.Message) (string, error) {
		if msg != nil && len(msg.ToolCalls) > 0 {
			return ToolExecNodeKey, nil
		}
		// 如果msg不为空而且没有工具调用，如果msg包含了明显的需要用户输入信息的词语会暂停，并用step状态检测做兜底
		if msg != nil && (shouldWaitForUserInput(msg.Content) || shouldPauseForNonTerminalStep(ctx)) {
			return InputPauseNodeKey, nil
		}
		return compose.END, nil
	}, map[string]bool{ToolExecNodeKey: true, InputPauseNodeKey: true, compose.END: true})); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(ToolExecNodeKey, ModelInputNodeKey); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(ModelInputNodeKey, ModelNodeKey); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(InputPauseNodeKey, compose.END); err != nil {
		return nil, err
	}
	runner, err := graph.Compile(ctx,
		compose.WithGraphName(GraphName),
		compose.WithCheckPointStore(newMemoryStore()),
		compose.WithMaxRunSteps(24),
	)
	if err != nil {
		return nil, err
	}
	rt.runner = runner
	return rt, nil
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
- Use the exact step_id from the approved plan.`
}

func userInputProtocolPrompt() string {
	return `Missing input protocol:
- If required user parameters are missing, call request_user_input instead of asking in plain text.
- Provide question and fields in request_user_input.
- The runtime will pause and wait for user reply.
- After resume, continue the same step and use request_user_input answer to finish execution.`
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

func runtimeModelInputMessages(ctx context.Context, system string, history []*schema.Message) []*schema.Message {
	msgs := cloneMessages(history)
	if session, _ := ctx.Value(wsCtxSessionKey).(*wsSession); session != nil {
		if plan := session.activePlanSnapshot(); plan != nil {
			msgs = append([]*schema.Message{schema.UserMessage(planExecutionPrompt(plan))}, msgs...)
		}
		stepID := strings.TrimSpace(preferredStepID(ctx, session))
		if stepID != "" {
			system = system + "\n\nCURRENT_STEP_ID: " + stepID
		}
	}
	return sanitizeMessagesForModel(withSystemMessage(system+"\n\n"+planStepProtocolPrompt()+"\n\n"+userInputProtocolPrompt(), msgs))
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
	return schema.ToolMessage(result, call.ID, schema.WithToolName(call.Function.Name))
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
		Kind:    wsEventInputRequired,
		Message: msg,
		Missing: missing,
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
