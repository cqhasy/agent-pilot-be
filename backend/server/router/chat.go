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

	// WebSocket 会话数据 REST（与 WS 推送并行；依赖 Mongo 持久化）
	chatGroup.POST("/ws/sessions", authMiddleware.MiddlewareFunc(), c.WSCreateSession)
	chatGroup.GET("/ws/sessions", authMiddleware.MiddlewareFunc(), c.WSListSessions)
	chatGroup.GET("/ws/sessions/:session_id/messages", authMiddleware.MiddlewareFunc(), c.WSGetMessages)
	chatGroup.GET("/ws/sessions/:session_id/preview", authMiddleware.MiddlewareFunc(), c.WSGetExpertPreview)

}
