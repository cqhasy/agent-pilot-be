package model

import "time"

type AgentMessage struct {
	ID        string         `bson:"_id"`
	SessionID string         `bson:"session_id"`
	PlanID    string         `bson:"plan_id"`
	StepID    string         `bson:"step_id,omitempty"`
	Role      string         `bson:"role"`
	Content   string         `bson:"content"`
	Metadata  map[string]any `bson:"metadata,omitempty"`
	CreatedAt time.Time      `bson:"created_at"`
}
