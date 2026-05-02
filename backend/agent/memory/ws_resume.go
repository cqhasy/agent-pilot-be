package memory

import (
	"context"
	"encoding/json"

	"github.com/agent-pilot/agent-pilot-be/repository/dao"
	"github.com/cloudwego/eino/schema"
)

// WSResumeSnapshot 可跨进程恢复的 websocket 中断上下文（不含 eino 图状态，图状态由 GraphCheckPointStore 单独持久化）。
type WSResumeSnapshot struct {
	InterruptID   string
	Message       string
	StepID        string
	RequestID     string
	PlanJSON      []byte
	InterruptKind string
}

func (s *memoryService) SaveWSResume(ctx context.Context, sessionID string, snap *WSResumeSnapshot) error {
	if s == nil || s.dao == nil || snap == nil || snap.InterruptID == "" {
		return nil
	}
	return s.dao.WSRuntimeResumeSet(ctx, sessionID, dao.WSRuntimeResume{
		InterruptID:   snap.InterruptID,
		Message:       snap.Message,
		StepID:        snap.StepID,
		RequestID:     snap.RequestID,
		PlanJSON:      snap.PlanJSON,
		InterruptKind: snap.InterruptKind,
	})
}

func (s *memoryService) LoadWSResume(ctx context.Context, sessionID string) (*WSResumeSnapshot, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	rec, ok, err := s.dao.WSRuntimeResumeGet(ctx, sessionID)
	if err != nil || !ok {
		return nil, err
	}
	return &WSResumeSnapshot{
		InterruptID:   rec.InterruptID,
		Message:       rec.Message,
		StepID:        rec.StepID,
		RequestID:     rec.RequestID,
		PlanJSON:      rec.PlanJSON,
		InterruptKind: rec.InterruptKind,
	}, nil
}

func (s *memoryService) ConsumeWSResume(ctx context.Context, sessionID string) error {
	if s == nil || s.dao == nil {
		return nil
	}
	return s.dao.WSRuntimeResumeClear(ctx, sessionID)
}

func (s *memoryService) SaveWSHistory(ctx context.Context, sessionID string, history []*schema.Message) error {
	if s == nil || s.dao == nil {
		return nil
	}
	data, err := json.Marshal(history)
	if err != nil {
		return err
	}
	return s.dao.WSRuntimeHistorySet(ctx, sessionID, data)
}

func (s *memoryService) LoadWSHistory(ctx context.Context, sessionID string) ([]*schema.Message, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	data, ok, err := s.dao.WSRuntimeHistoryGet(ctx, sessionID)
	if err != nil || !ok || len(data) == 0 {
		return nil, err
	}
	var history []*schema.Message
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	return history, nil
}
