package router

import (
	"github.com/agent-pilot/agent-pilot-be/controller/auth"
	"github.com/agent-pilot/agent-pilot-be/middleware"
	"github.com/agent-pilot/agent-pilot-be/pkg/ginx"
	"github.com/gin-gonic/gin"
)

func registerAuth(s *gin.RouterGroup, l *middleware.LoggerMiddleware,
	authMiddleware *middleware.AuthMiddleware,
	authC auth.ControllerInterface) {
	authGroup := s.Group("/auth")
	authGroup.POST("/email/login", l.NormalMiddlewareFunc(), ginx.WrapReq(authC.Login))
	authGroup.POST("/email/register", l.NormalMiddlewareFunc(), ginx.WrapReq(authC.Register))
	authGroup.POST("/email/send", l.NormalMiddlewareFunc(), ginx.WrapReq(authC.SendEmail))
	authGroup.GET("/me", l.NormalMiddlewareFunc(), authMiddleware.MiddlewareFunc(), ginx.Wrap(authC.GetMe))
	authGroup.POST("/email/logout", l.NormalMiddlewareFunc(), authMiddleware.MiddlewareFunc(), ginx.Wrap(authC.Logout))
}
