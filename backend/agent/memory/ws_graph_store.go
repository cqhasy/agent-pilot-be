package memory

import (
	"context"

	"github.com/agent-pilot/agent-pilot-be/repository/dao"
	"github.com/cloudwego/eino/compose"
)

type graphCheckPointStore struct {
	d dao.AgentDao
}

// GraphCheckPointStore 返回基于 Mongo 的 eino compose 检查点存储，与 websocket session_id（compose CheckPointID）对齐。
func (s *memoryService) GraphCheckPointStore() compose.CheckPointStore {
	if s == nil || s.dao == nil {
		return nil
	}
	return &graphCheckPointStore{d: s.dao}
}

func (g *graphCheckPointStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	return g.d.WSRuntimeGraphGet(ctx, checkPointID)
}

func (g *graphCheckPointStore) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	return g.d.WSRuntimeGraphSet(ctx, checkPointID, checkPoint)
}
