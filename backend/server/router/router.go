package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/agent-pilot/agent-pilot-be/controller/auth"
	"github.com/agent-pilot/agent-pilot-be/controller/chat"
	"github.com/agent-pilot/agent-pilot-be/controller/health"
	"github.com/agent-pilot/agent-pilot-be/middleware"
)

func NewRouter(
	AuthMiddleware *middleware.AuthMiddleware,
	corsMiddleware *middleware.CorsMiddleware,
	loggerMiddleware *middleware.LoggerMiddleware,

	hc *health.Controller,
	ac *auth.LarkAuthController,
	cc *chat.Controller,
) *gin.Engine {
	r := gin.Default()
	//使用gin的Panic捕获中间件
	r.Use(gin.Recovery())
	// 添加 CORS 中间件,跨域中间件
	r.Use(corsMiddleware.MiddlewareFunc())
	//暴露给前端的api前缀
	g := r.Group("/api/v1")
	//注册router
	registerHealth(g, loggerMiddleware, hc)
	registerAuth(g, loggerMiddleware, AuthMiddleware, ac)
	registerChat(g, AuthMiddleware, cc)

	registerFrontend(r)
	return r
}

func registerFrontend(r *gin.Engine) {
	frontendDir := resolveFrontendDir()
	indexPath := filepath.Join(frontendDir, "index.html")
	wsTestPath := filepath.Join(frontendDir, "ws-test.html")

	r.Static("/js", filepath.Join(frontendDir, "js"))
	r.Static("/styles", filepath.Join(frontendDir, "styles"))

	r.GET("/", func(ctx *gin.Context) {
		ctx.File(indexPath)
	})
	r.GET("/index.html", func(ctx *gin.Context) {
		ctx.File(indexPath)
	})
	r.GET("/ws-test", func(ctx *gin.Context) {
		ctx.File(wsTestPath)
	})
	r.GET("/ws-test.html", func(ctx *gin.Context) {
		ctx.File(wsTestPath)
	})

	r.NoRoute(func(ctx *gin.Context) {
		if strings.HasPrefix(ctx.Request.URL.Path, "/api/") {
			ctx.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "Not Found"})
			return
		}
		ctx.File(indexPath)
	})
}

func resolveFrontendDir() string {
	candidates := []string{
		"frontend",
		filepath.Join("..", "frontend"),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return filepath.Join("..", "frontend")
}
