package router

import (
	"github.com/agent-pilot/agent-pilot-be/controller/chat"
	"github.com/agent-pilot/agent-pilot-be/middleware"
	"github.com/gin-gonic/gin"
)

func registerChat(s *gin.RouterGroup, authMiddleware *middleware.AuthMiddleware, c chat.ControllerInterface) {
	chatGroup := s.Group("/chat")
	// 流式响应不使用普通日志中间件，因为它会缓冲整个响应
	// 可以在 controller 内部自行记录日志
	chatGroup.POST("/stream", authMiddleware.MiddlewareFunc(), c.Chat)
	chatGroup.POST("/plan", authMiddleware.MiddlewareFunc(), c.Plan)
	chatGroup.POST("/execute", authMiddleware.MiddlewareFunc(), c.Execute)
	chatGroup.GET("/ws", c.ChatWS)

}
