package chat

import (
	"context"
	"net/http"
	"sync"

	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/gin-gonic/gin"
)

var (
	chatWSHub   *wsHub
	chatWSHubMu sync.RWMutex
)

func EnableWebSocketChat(
	c *Controller,
	ctx context.Context,
	chatModel model.ToolCallingChatModel,
	tools []einotool.BaseTool,
	planner agentplan.Planner,
	checkpointer agentplan.Checkpointer,
	mem memory.MemoryService,
) error {
	var ckStore compose.CheckPointStore
	if mem != nil {
		ckStore = mem.GraphCheckPointStore()
	}
	rt, err := newComposeRuntime(ctx, chatModel, tools, c.SystemMsg, ckStore)
	if err != nil {
		return err
	}
	hub := newWSHub(rt, planner, checkpointer, mem)
	chatWSHubMu.Lock()
	chatWSHub = hub
	chatWSHubMu.Unlock()
	return nil
}

func (c *Controller) ChatWS(ctx *gin.Context) {
	chatWSHubMu.RLock()
	h := chatWSHub
	chatWSHubMu.RUnlock()
	if h == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "websocket chat is not configured"})
		return
	}
	h.serve(ctx)
}
