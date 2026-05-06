package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-pilot/agent-pilot-be/agent/expert"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	HandoffToExpertToolName = "handoff_to_expert"
	ReleaseExpertToolName   = "release_expert"
)

// HandoffToExpertTool 将对话切换到专家专属线程：模型侧不再看到主 Agent 历史，仅看到 task_brief 与后续专家轮次。
type HandoffToExpertTool struct {
	Reg *expert.Registry
}

type handoffToExpertArgs struct {
	ExpertID  string `json:"expert_id"`
	TaskBrief string `json:"task_brief"`
}

func (t *HandoffToExpertTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	var idsDesc string
	if t != nil && t.Reg != nil {
		var parts []string
		for _, d := range t.Reg.List() {
			parts = append(parts, fmt.Sprintf("%s (%s)", d.ID, d.Name))
		}
		if len(parts) > 0 {
			idsDesc = "Allowed expert_id values: " + strings.Join(parts, ", ") + "."
		}
	}
	handoffDesc := "Switch to a domain expert. REQUIRED first step for substantive document work (long prose, 小说/文章/报告, Feishu doc tasks) → expert_id=document; for slides/PPT/演示 → expert_id=presentation. Isolated thread: only task_brief + follow-up user turns reach that expert — put ALL context in task_brief (goal, constraints, tone, length, prior outputs when chaining). " + idsDesc + " Callable from main or another expert. Use release_expert when returning to general Q&A."
	return &schema.ToolInfo{
		Name: HandoffToExpertToolName,
		Desc: handoffDesc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"expert_id": {
				Type:     schema.String,
				Desc:     "document | presentation | other registered id. Match the user’s primary deliverable type.",
				Required: true,
			},
			"task_brief": {
				Type:     schema.String,
				Desc:     "Standalone brief: user goal, format, language, length, structure, Feishu/skill constraints. Experts do not see main-thread history.",
				Required: true,
			},
		}),
	}, nil
}

func (t *HandoffToExpertTool) InvokableRun(ctx context.Context, arguments string, _ ...einotool.Option) (string, error) {
	if t == nil || t.Reg == nil {
		return "", fmt.Errorf("expert registry not configured")
	}
	var args handoffToExpertArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", err
	}
	id := strings.TrimSpace(args.ExpertID)
	if id == "" {
		return "", fmt.Errorf("expert_id is required")
	}
	def, ok := t.Reg.Get(id)
	if !ok {
		return "", fmt.Errorf("unknown expert_id: %s", id)
	}
	brief := strings.TrimSpace(args.TaskBrief)
	if brief == "" {
		return "", fmt.Errorf("task_brief is required")
	}
	xs := expert.ExpertSessionFrom(ctx)
	if xs == nil {
		return "Expert handoff is only available in WebSocket chat sessions.", nil
	}
	xs.EnterExpertMode(id, brief)
	return fmt.Sprintf("Switched to isolated expert thread: %s (%s). The specialist only sees task_brief and follow-up messages in this mode.", def.Name, def.ID), nil
}

// ReleaseExpertTool 退出专家模式，回到通用主 Agent 行为。
type ReleaseExpertTool struct{}

func (t *ReleaseExpertTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: ReleaseExpertToolName,
		Desc: "Leave expert mode and return to the main agent (full history + routing). Call after the delegated doc/deck task is approved/completed and persistence for this step is done, before handling general follow-ups (share to group, new topics). Required to avoid staying stuck in specialist mode.",
	}, nil
}

func (t *ReleaseExpertTool) InvokableRun(ctx context.Context, _ string, _ ...einotool.Option) (string, error) {
	xs := expert.ExpertSessionFrom(ctx)
	if xs == nil {
		return "Release expert is only meaningful in WebSocket chat sessions.", nil
	}
	xs.ExitExpertMode()
	return "Returned to general assistant mode (full session history restored for routing).", nil
}

var (
	_ einotool.InvokableTool = (*HandoffToExpertTool)(nil)
	_ einotool.InvokableTool = (*ReleaseExpertTool)(nil)
)
