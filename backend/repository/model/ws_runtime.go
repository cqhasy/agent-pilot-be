package model

import "time"

// WSRuntimeDoc 持久化 websocket 会话的 eino 图检查点与可恢复中断元数据（同一 compose CheckPointID / session_id）。
type WSRuntimeDoc struct {
	ID            string    `bson:"_id"`
	Graph         []byte    `bson:"graph,omitempty"`
	HistoryJSON   []byte    `bson:"history_json,omitempty"`
	InterruptID   string    `bson:"interrupt_id,omitempty"`
	IntMessage    string    `bson:"int_message,omitempty"`
	IntStepID     string    `bson:"int_step_id,omitempty"`
	IntRequestID  string    `bson:"int_request_id,omitempty"`
	IntPlanJSON   []byte    `bson:"int_plan_json,omitempty"`
	InterruptKind string    `bson:"interrupt_kind,omitempty"`
	UpdatedAt     time.Time `bson:"updated_at"`
}
