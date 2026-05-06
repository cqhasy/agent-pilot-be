package memory

import (
	"context"
	"encoding/json"

	"github.com/agent-pilot/agent-pilot-be/repository/dao"
	"github.com/cloudwego/eino/schema"
)

// WSResumeSnapshot 可跨进程恢复的 websocket 中断上下文（不含 eino 图状态，图状态由 GraphCheckPointStore 单独持久化）。
type WSResumeSnapshot struct {
	InterruptID    string
	Message        string
	StepID         string
	RequestID      string
	PlanJSON       []byte
	ActiveExpertID string
	InterruptKind  string
}

func (s *memoryService) SaveWSResume(ctx context.Context, sessionID string, snap *WSResumeSnapshot) error {
	if s == nil || s.dao == nil || snap == nil || snap.InterruptID == "" {
		return nil
	}
	return s.dao.WSRuntimeResumeSet(ctx, sessionID, dao.WSRuntimeResume{
		InterruptID:    snap.InterruptID,
		Message:        snap.Message,
		StepID:         snap.StepID,
		RequestID:      snap.RequestID,
		PlanJSON:       snap.PlanJSON,
		ActiveExpertID: snap.ActiveExpertID,
		InterruptKind:  snap.InterruptKind,
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
		InterruptID:    rec.InterruptID,
		Message:        rec.Message,
		StepID:         rec.StepID,
		RequestID:      rec.RequestID,
		PlanJSON:       rec.PlanJSON,
		ActiveExpertID: rec.ActiveExpertID,
		InterruptKind:  rec.InterruptKind,
	}, nil
}

func (s *memoryService) ConsumeWSResume(ctx context.Context, sessionID string) error {
	if s == nil || s.dao == nil {
		return nil
	}
	return s.dao.WSRuntimeResumeClear(ctx, sessionID)
}

func (s *memoryService) CreateWSRuntimeSession(ctx context.Context, sessionID string) error {
	if s == nil || s.dao == nil || sessionID == "" {
		return nil
	}
	return s.dao.WSRuntimeSessionTouch(ctx, sessionID)
}

func (s *memoryService) SaveWSHistory(ctx context.Context, sessionID string, history []*schema.Message) error {
	if s == nil || s.dao == nil {
		return nil
	}
	data, err := json.Marshal(history)
	if err != nil {
		return err
	}
	if err := s.dao.WSRuntimeHistorySet(ctx, sessionID, data); err != nil {
		return err
	}
	if t := FirstUserPreviewTitle(history); t != "" {
		_ = s.dao.WSRuntimeSetPreviewTitleIfEmpty(ctx, sessionID, t)
	}
	return nil
}

func (s *memoryService) AppendWSHistory(ctx context.Context, sessionID string, messages ...*schema.Message) error {
	if s == nil || s.dao == nil || len(messages) == 0 {
		return nil
	}
	items := make([][]byte, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		items = append(items, data)
	}
	if err := s.dao.WSRuntimeHistoryAppend(ctx, sessionID, items); err != nil {
		return err
	}
	if t := FirstUserPreviewTitle(messages); t != "" {
		_ = s.dao.WSRuntimeSetPreviewTitleIfEmpty(ctx, sessionID, t)
	}
	return nil
}

func (s *memoryService) LoadWSHistory(ctx context.Context, sessionID string) ([]*schema.Message, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	items, ok, err := s.dao.WSRuntimeHistoryItemsGet(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if ok {
		history := make([]*schema.Message, 0, len(items))
		for _, item := range items {
			var msg schema.Message
			if err := json.Unmarshal(item, &msg); err != nil {
				return nil, err
			}
			history = append(history, &msg)
		}
		return history, nil
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
