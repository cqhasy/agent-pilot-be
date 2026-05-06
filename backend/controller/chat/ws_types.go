package chat

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/agent-pilot/agent-pilot-be/agent/expert"
	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/net/websocket"
)

const (
	InputBuildNodeKey = "input"
	ModelInputNodeKey = "model_input"
	ModelNodeKey      = "model"
	ToolExecNodeKey   = "tool_executor"
	InputPauseNodeKey = "input_pause"
	GraphName         = "agent_pilot_ws_chat"
	// GraphNameExpertPrefix 专家独立 compose 的 graph 名前缀，完整名为 Prefix + expert_id。
	GraphNameExpertPrefix = "agent_pilot_ws_expert_"
)

const (
	wsEventReady                = "ready"
	// wsEventPlanning Planner 异步生成计划开始前下发，便于前端展示「规划中」避免空白等待。
	wsEventPlanning             = "planning"
	wsEventPlanPending          = "plan_pending"
	wsEventPlanUpdated          = "plan_updated"
	wsEventPlanRejected         = "plan_rejected"
	wsEventMessage              = "message"
	wsEventDone                 = "done"
	wsEventError                = "error"
	wsEventInterrupted          = "interrupted"
	wsEventToolApprovalRequired = "tool_approval_required"
	wsEventInputRequired        = "input_required"
	wsEventToolResult           = "tool_result"
	wsEventExpertMode           = "expert_mode"
	// wsEventExpertPreview 自定义专家图中可向客户端推送的可视化/预览载荷（由 Lambda 或工具节点广播）。
	wsEventExpertPreview = "expert_preview"
)

const (
	wsInputUserMessage = "user_message"
	wsInputApprovePlan = "approve_plan"
	wsInputRejectPlan  = "reject_plan"
	wsInputApproveTool = "approve_tool"
	wsInputRejectTool  = "reject_tool"
	wsInputAnswer      = "answer"
	wsInputInterrupt   = "interrupt"
)

type wsInput struct {
	Type        string          `json:"type"`
	SessionID   string          `json:"session_id,omitempty"`
	RequestID   string          `json:"request_id,omitempty"`
	Message     string          `json:"message,omitempty"`
	StepID      string          `json:"step_id,omitempty"`
	InterruptID string          `json:"interrupt_id,omitempty"`
	ToolCallID  string          `json:"tool_call_id,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
	Reason      string          `json:"reason,omitempty"`
}

type wsMessageHandler func(ctx context.Context, session *wsSession, input wsInput) error

type wsOutput struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Data      any    `json:"data,omitempty"`
}

type chatRuntimeInput struct {
	History   []*schema.Message
	UserInput string
}

type chatRuntimeState struct {
	History []*schema.Message
}

type humanResume struct {
	Action     string `json:"action"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Arguments  string `json:"arguments,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type humanPause struct {
	Kind      string         `json:"kind"`
	Message   string         `json:"message"`
	ToolName  string         `json:"tool_name,omitempty"` // 触发本次暂停的工具名，如 request_user_input
	ToolCalls []toolCallView `json:"tool_calls,omitempty"`
	Missing   []string       `json:"missing,omitempty"`
	CanModify bool           `json:"can_modify"`
	CanReject bool           `json:"can_reject"`
	CreatedAt time.Time      `json:"created_at"`
}

type toolCallView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Risk      string `json:"risk"`
}

type composeRuntime struct {
	runner  compose.Runnable[chatRuntimeInput, *schema.Message]
	tools   map[string]einotool.InvokableTool
	infos   map[string]*schema.ToolInfo
	guards  []toolCallPauseGuard
	system  string
	experts *expert.Registry
	// isExpertGraph 为 true 时表示专家专用编译图，系统提示已在编译时固定，不再按 session 覆盖。
	isExpertGraph bool
	// mainHandoffRewire 为 true 时仅主图在 tool 执行后处理 handoff 状态重写。
	mainHandoffRewire bool
}

type toolCallPauseGuard func(call schema.ToolCall) *humanPause

type wsHub struct {
	runtime        *composeRuntime
	expertRuntimes map[string]*composeRuntime // expert_id -> 独立编译的 compose
	expertReg      *expert.Registry
	planner        agentplan.Planner
	checkpointer   agentplan.Checkpointer
	mem            memory.MemoryService

	mu       sync.Mutex
	sessions map[string]*wsSession
	handlers map[string]wsMessageHandler
}

type wsSession struct {
	id string

	mu                         sync.Mutex
	clients                    map[*websocket.Conn]struct{}
	history                    []*schema.Message
	activeExpertID             string
	expertBranch               []*schema.Message
	expertOmitNextUserInBranch bool
	expertRewireCompose        bool
	pending                    wsPendingState
	run                        wsRunState
	interrupted                wsInterruptedState
	runVer                     uint64
	planningReq                string
	planCancel                 context.CancelFunc
	// pendingResumeFallback：本轮 invoke 若 WithResume 传入的 humanResume 需在 executeToolCalls 兜底，仅允许消费一次，避免污染后续多轮 request_user_input。
	pendingResumeFallback *humanResume
}

type wsPendingState struct {
	text        string
	plan        *agentplan.Plan
	interruptID string
}

type wsRunState struct {
	cancel    func(opts ...compose.GraphInterruptOption)
	active    bool
	plan      *agentplan.Plan
	message   string
	stepID    string
	requestID string
}

type wsInterruptedState struct {
	message   string
	plan      *agentplan.Plan
	stepID    string
	requestID string
	// expertID 非空表示中断发生在该专家的独立 compose 上，恢复时必须与 composeCheckpointID(session) 一致。
	expertID string
}

type wsCtxKey int

const (
	wsCtxSessionKey wsCtxKey = iota + 1
	wsCtxRequestIDKey
	wsCtxRunVerKey
	wsCtxStepIDKey
)
