package atype

import (
	"context"
	"time"

	"github.com/cloudwego/eino/schema"
)

// Status 与 model.PlanStatus 取值一致。
type Status string

const (
	StatusDraft     Status = "draft"
	StatusReady     Status = "ready"
	StatusExecuting Status = "executing"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusPaused    StepStatus = "paused"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
)

// Step 与 model.Step 对齐。
type Step struct {
	ID          string     `json:"id" bson:"id"`
	Title       string     `json:"title" bson:"title"`
	Description string     `json:"description" bson:"description"`
	Result      string     `json:"result,omitempty" bson:"result,omitempty"`
	Status      StepStatus `json:"status" bson:"status"`
}

// Session 与 model.ChatSession 对齐（对话线程容器）。
type Session struct {
	ID            string    `json:"id" bson:"_id"`
	UserID        string    `json:"user_id" bson:"user_id"`
	CurrentPlanID string    `json:"current_plan_id,omitempty" bson:"current_plan_id,omitempty"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" bson:"updated_at"`
}

// Checkpoint 与 model.Checkpoint 对齐。
type Checkpoint struct {
	ID        string    `json:"id" bson:"id"`
	StepID    string    `json:"step_id" bson:"step_id"`
	Question  string    `json:"question" bson:"question"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// Plan 与 model.Plan 对齐。
type Plan struct {
	ID            string      `json:"id" bson:"_id"`
	SessionID     string      `json:"session_id,omitempty" bson:"session_id,omitempty"`
	Goal          string      `json:"goal" bson:"goal"`
	Steps         []Step      `json:"steps" bson:"steps"`
	Status        Status      `json:"status" bson:"status"`
	CurrentStepID string      `json:"current_step_id,omitempty" bson:"current_step_id,omitempty"`
	Checkpoint    *Checkpoint `json:"checkpoint,omitempty" bson:"checkpoint,omitempty"`
	CreatedAt     time.Time   `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at" bson:"updated_at"`
}

type MessageRole string

const (
	RoleUser       MessageRole = "user"
	RoleAssistant  MessageRole = "assistant"
	RoleToolCall   MessageRole = "tool_call"
	RoleToolResult MessageRole = "tool_result"
)

// Message 与 model.AgentMessage 对齐。
type Message struct {
	ID        string         `json:"id" bson:"_id"`
	SessionID string         `json:"session_id,omitempty" bson:"session_id,omitempty"`
	PlanID    string         `json:"plan_id" bson:"plan_id"`
	StepID    string         `json:"step_id,omitempty" bson:"step_id,omitempty"`
	Role      MessageRole    `json:"role" bson:"role"`
	Content   string         `json:"content" bson:"content"`
	Metadata  map[string]any `json:"metadata,omitempty" bson:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at" bson:"created_at"`
}

type Request struct {
	SessionID string
	UserInput string
	History   []*schema.Message
}

type Planner interface {
	Plan(ctx context.Context, req Request) (*Plan, error)
}
