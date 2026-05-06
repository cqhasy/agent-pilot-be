package chat

import (
	"context"
	"strings"
	"time"

	"github.com/agent-pilot/agent-pilot-be/agent/expert"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// composeCompileOptions 区分主会话图与专家独立图（独立 checkpoint / graph name / 是否处理 handoff 重写）。
type composeCompileOptions struct {
	graphName         string
	experts           *expert.Registry
	mainHandoffRewire bool // 仅主图：handoff 后重写 compose state，衔接专家
	isExpertGraph     bool // 专家图：runtimeModelInputMessages 不再按 session 覆盖系统提示
}

// compileComposeRuntime 编译一条 ReAct 工具环；专家与主会话共用此编译路径，通过选项区分行为。
func compileComposeRuntime(ctx context.Context, chatModel model.ToolCallingChatModel, tools []einotool.BaseTool, system string, checkpointStore compose.CheckPointStore, opts composeCompileOptions) (*composeRuntime, error) {
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
		tools:             invokable,
		infos:             infoByName,
		guards:            make([]toolCallPauseGuard, 0, 2),
		system:            system,
		experts:           opts.experts,
		isExpertGraph:     opts.isExpertGraph,
		mainHandoffRewire: opts.mainHandoffRewire,
	}
	rt.guards = append(rt.guards, rt.pauseOnUserInputRequest, rt.pauseOnMissingRequired, rt.pauseOnDangerousToolCall)

	graph := compose.NewGraph[chatRuntimeInput, *schema.Message](compose.WithGenLocalState(func(ctx context.Context) *chatRuntimeState {
		return &chatRuntimeState{}
	}))

	buildInput := compose.InvokableLambda(func(ctx context.Context, input chatRuntimeInput) ([]*schema.Message, error) {
		return runtimeModelInputMessages(ctx, system, input.History, rt), nil
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
			return runtimeModelInputMessages(ctx, system, state.History, rt), nil
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

	toolPost := func(ctx context.Context, output []*schema.Message, state *chatRuntimeState) ([]*schema.Message, error) {
		state.History = append(state.History, output...)
		if opts.mainHandoffRewire {
			if sess, ok := ctx.Value(wsCtxSessionKey).(*wsSession); ok && sess != nil && sess.consumeExpertRewireCompose() {
				branch := sess.expertBranchSnapshot()
				state.History = append(branch, cloneMessages(output)...)
			}
		}
		return output, nil
	}
	if err := graph.AddLambdaNode(ToolExecNodeKey, toolExecutor,
		compose.WithStatePostHandler(toolPost),
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
	if checkpointStore == nil {
		checkpointStore = newMemoryStore()
	}
	gn := opts.graphName
	if gn == "" {
		gn = GraphName
	}
	runner, err := graph.Compile(ctx,
		compose.WithGraphName(gn),
		compose.WithCheckPointStore(checkpointStore),
		compose.WithMaxRunSteps(96),
	)
	if err != nil {
		return nil, err
	}
	rt.runner = runner
	return rt, nil
}
