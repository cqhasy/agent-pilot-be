package plan

import (
	"context"
	"time"

	"github.com/cloudwego/eino/schema"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusReady     Status = "ready"
	StatusExecuting Status = "executing"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
)

type SubjectiveState struct {
	Goal            string   `json:"goal"`
	Stance          string   `json:"stance"`
	Preferences     []string `json:"preferences,omitempty"`
	RiskAwareness   []string `json:"risk_awareness,omitempty"`
	ClarifyingNeeds []string `json:"clarifying_needs,omitempty"`
}

type Step struct {
	ID              string            `json:"id"`
	Title           string            `json:"title"`
	Purpose         string            `json:"purpose"`
	ExpectedOutcome string            `json:"expected_outcome"`
	Skill           string            `json:"skill,omitempty"`
	Inputs          map[string]string `json:"inputs,omitempty"`
	Dependencies    []string          `json:"dependencies,omitempty"`
	Status          StepStatus        `json:"status"`
}

type Plan struct {
	ID              string          `json:"id"`
	SessionID       string          `json:"session_id,omitempty"`
	Objective       string          `json:"objective"`
	Summary         string          `json:"summary"`
	SubjectiveState SubjectiveState `json:"subjective_state"`
	Assumptions     []string        `json:"assumptions,omitempty"`
	Constraints     []string        `json:"constraints,omitempty"`
	Steps           []Step          `json:"steps"`
	Status          Status          `json:"status"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type Request struct {
	SessionID string
	UserInput string
	History   []*schema.Message
}

type Planner interface {
	Plan(ctx context.Context, req Request) (*Plan, error)
}
