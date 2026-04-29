package model

import "time"

type ChatSession struct {
	ID            string    `bson:"_id"`
	UserID        string    `bson:"user_id"`
	CurrentPlanID string    `bson:"current_plan_id,omitempty"`
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
}
