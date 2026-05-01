package chat

import (
	"context"
	"encoding/json"
	"sync"
	"time"

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
)

const (
	wsEventReady                = "ready"
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
	runner compose.Runnable[chatRuntimeInput, *schema.Message]
	tools  map[string]einotool.InvokableTool
	infos  map[string]*schema.ToolInfo
	guards []toolCallPauseGuard
	system string
}

type toolCallPauseGuard func(call schema.ToolCall) *humanPause

type wsHub struct {
	runtime      *composeRuntime
	planner      agentplan.Planner
	checkpointer agentplan.Checkpointer

	mu       sync.Mutex
	sessions map[string]*wsSession
	handlers map[string]wsMessageHandler
}

type wsSession struct {
	id string

	mu          sync.Mutex
	clients     map[*websocket.Conn]struct{}
	history     []*schema.Message
	pending     wsPendingState
	run         wsRunState
	interrupted wsInterruptedState
	runVer      uint64
	planningReq string
	planCancel  context.CancelFunc
}

type wsPendingState struct {
	text        string
	plan        *agentplan.Plan
	checkpoint  string
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
}

type wsCtxKey int

const (
	wsCtxSessionKey wsCtxKey = iota + 1
	wsCtxRequestIDKey
	wsCtxRunVerKey
	wsCtxStepIDKey
)
