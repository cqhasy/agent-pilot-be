package model

import "time"

type Step struct {
	ID          string `bson:"id"`
	Title       string `bson:"title"`
	Description string `bson:"description"`
	Result      string `bson:"result,omitempty"`
	Status      string `bson:"status"`
}

type Checkpoint struct {
	ID        string    `bson:"id"`
	StepID    string    `bson:"step_id"`
	Question  string    `bson:"question"`
	CreatedAt time.Time `bson:"created_at"`
}

type Plan struct {
	ID            string      `bson:"_id"`
	SessionID     string      `bson:"session_id,omitempty"`
	Goal          string      `bson:"goal"`
	Steps         []Step      `bson:"steps"`
	Status        string      `bson:"status"`
	CurrentStepID string      `bson:"current_step_id,omitempty"`
	Checkpoint    *Checkpoint `bson:"checkpoint,omitempty"`
	CreatedAt     time.Time   `bson:"created_at"`
	UpdatedAt     time.Time   `bson:"updated_at"`
}
