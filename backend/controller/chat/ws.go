package chat

import (
	"context"
	"net/http"
	"sync"

	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/gin-gonic/gin"
)

var composeWS sync.Map // map[*Controller]*wsHub

func EnableWebSocketChat(
	c *Controller,
	ctx context.Context,
	chatModel model.ToolCallingChatModel,
	tools []einotool.BaseTool,
	planner agentplan.Planner,
	checkpointer agentplan.Checkpointer,
) error {
	rt, err := newComposeRuntime(ctx, chatModel, tools, c.SystemMsg)
	if err != nil {
		return err
	}
	composeWS.Store(c, newWSHub(rt, planner, checkpointer))
	return nil
}

func (c *Controller) ChatWS(ctx *gin.Context) {
	value, ok := composeWS.Load(c)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "websocket chat is not configured"})
		return
	}
	value.(*wsHub).serve(ctx)
}
