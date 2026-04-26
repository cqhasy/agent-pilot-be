package router

import (
	"github.com/agent-pilot/agent-pilot-be/controller/auth"
	"github.com/agent-pilot/agent-pilot-be/middleware"
	"github.com/agent-pilot/agent-pilot-be/pkg/ginx"
	"github.com/gin-gonic/gin"
)

func registerAuth(s *gin.RouterGroup, l *middleware.LoggerMiddleware,
	authMiddleware *middleware.AuthMiddleware,
	c auth.LarkAuthControllerInterface) {
	authGroup := s.Group("/auth")
	authGroup.GET("/feishu/login", l.NormalMiddlewareFunc(), ginx.Wrap(c.GetFeishuLogin))
	authGroup.GET("/feishu/callback", c.FeishuCallbackGin)
	authGroup.GET("/me", l.NormalMiddlewareFunc(), authMiddleware.MiddlewareFunc(), ginx.Wrap(c.GetMe))
	authGroup.POST("/logout", l.NormalMiddlewareFunc(), authMiddleware.MiddlewareFunc(), ginx.Wrap(c.Logout))
}
