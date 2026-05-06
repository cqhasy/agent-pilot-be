package memory

import (
	"context"
)

func (s *memoryService) ListWSRuntimeSessions(ctx context.Context, limit int) ([]WSRuntimeSessionRef, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.dao.WSRuntimeListPrimarySessions(ctx, int64(limit))
	if err != nil {
		return nil, err
	}
	out := make([]WSRuntimeSessionRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, WSRuntimeSessionRef{
			SessionID: r.SessionID,
			Title:     r.PreviewTitle,
			UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}
