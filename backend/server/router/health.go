package router

import (
	"github.com/agent-pilot/agent-pilot-be/controller/health"
	"github.com/agent-pilot/agent-pilot-be/middleware"
	"github.com/agent-pilot/agent-pilot-be/pkg/ginx"
	"github.com/gin-gonic/gin"
)

func registerHealth(s *gin.RouterGroup, l *middleware.LoggerMiddleware, c health.ControllerInterface) {
	s.GET("/health", l.NormalMiddlewareFunc(), ginx.Wrap(c.GetHealth))
}
