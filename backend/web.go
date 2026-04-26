package main

import (
	"github.com/agent-pilot/agent-pilot-be/config"
	"github.com/agent-pilot/agent-pilot-be/controller/auth"
	authService "github.com/agent-pilot/agent-pilot-be/controller/auth/service"
	"github.com/agent-pilot/agent-pilot-be/controller/health"
	"github.com/agent-pilot/agent-pilot-be/ioc"
	"github.com/agent-pilot/agent-pilot-be/middleware"
	"github.com/agent-pilot/agent-pilot-be/pkg/jwt"
	"github.com/agent-pilot/agent-pilot-be/server"
)

// 不引入wire了，wire有时候还是太吃屎了，中小型还是自己维护比较快
func initWebServer() *App {
	conf, err := config.LoadFromEnv()
	if err != nil {
		panic(err)
	}
	// ioc
	logger := ioc.InitLogger(conf.Logconf)
	//pkg
	redisJWTHandler := jwt.NewRedisJWTHandler(conf.JwtConf)
	// middleware
	authM := middleware.NewAuthMiddleware(redisJWTHandler)
	corM := middleware.NewCorsMiddleware(conf.CorMiddlewareConf)
	logM := middleware.NewLoggerMiddleware(logger)
	// controller
	hc := health.NewHealthController()
	larkSvc := &authService.LarkService{}
	authC := auth.NewLarkAuthController(
		conf.FeishuAppID,
		conf.FeishuAppSecret,
		conf.FeishuRedirectURI,
		conf.StateSecret,
		larkSvc,
		redisJWTHandler,
	)
	srv := server.NewServer(hc, authC, authM, corM, logM)
	return NewApp(srv, &conf)
}
