package model

import "time"

// WSRuntimeDoc 持久化 websocket 会话的 eino 图检查点与可恢复中断元数据（同一 compose CheckPointID / session_id）。
type WSRuntimeDoc struct {
	ID            string    `bson:"_id"`
	Graph         []byte    `bson:"graph,omitempty"`
	HistoryJSON   []byte    `bson:"history_json,omitempty"`
	HistoryItems  [][]byte  `bson:"history_items,omitempty"`
	InterruptID   string    `bson:"interrupt_id,omitempty"`
	IntMessage    string    `bson:"int_message,omitempty"`
	IntStepID     string    `bson:"int_step_id,omitempty"`
	IntRequestID  string    `bson:"int_request_id,omitempty"`
	IntPlanJSON   []byte    `bson:"int_plan_json,omitempty"`
	IntExpertID   string    `bson:"int_expert_id,omitempty"`
	InterruptKind string    `bson:"interrupt_kind,omitempty"`
	// PreviewTitle 会话列表展示用：首条真实用户输入摘要（与 ChatGPT 侧栏标题类似）。
	PreviewTitle string `bson:"preview_title,omitempty"`
	UpdatedAt    time.Time `bson:"updated_at"`
}
