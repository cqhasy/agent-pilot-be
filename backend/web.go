package main

import (
	"context"

	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	"github.com/agent-pilot/agent-pilot-be/agent/tool"
	"github.com/agent-pilot/agent-pilot-be/agent/tool/skill"
	"github.com/agent-pilot/agent-pilot-be/config"
	"github.com/agent-pilot/agent-pilot-be/controller/auth"
	authService "github.com/agent-pilot/agent-pilot-be/controller/auth/service"
	"github.com/agent-pilot/agent-pilot-be/controller/chat"
	"github.com/agent-pilot/agent-pilot-be/controller/health"
	"github.com/agent-pilot/agent-pilot-be/ioc"
	"github.com/agent-pilot/agent-pilot-be/middleware"
	"github.com/agent-pilot/agent-pilot-be/pkg/jwt"
	mysqldao "github.com/agent-pilot/agent-pilot-be/repository/mysql/dao"
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
	mysqlDB := ioc.InitDB(conf.Mysql, logger)
	om := ioc.NewOpenAIModelClient(context.Background(),
		conf.OpenAIModel, conf.OpenAIBaseURL, conf.OpenAIAPIKey)

	// 加载 skills
	skillDir := "skills"
	skillReg, _ := skill.LoadSkills(skillDir)

	// 构建 tools
	tools := tool.BuildTools(skillReg)

	// 构建 system prompt
	systemMsg := chat.BuildSystemPrompt(skillReg.List())

	// 创建 ADK agent
	agent := chat.NewMainAgent(context.Background(), om.Model, systemMsg, tools)
	planner := agentplan.NewLLMPlanner(om.Model, skillReg)
	checkpointer := agentplan.NewMemoryCheckpointer()

	// 创建 chat controller
	cc := chat.NewController(context.Background(), agent, skillReg, systemMsg, planner, checkpointer)

	//pkg
	redisJWTHandler := jwt.NewRedisJWTHandler(conf.JwtConf)
	// middleware
	authM := middleware.NewAuthMiddleware(redisJWTHandler)
	corM := middleware.NewCorsMiddleware(conf.CorMiddlewareConf)
	logM := middleware.NewLoggerMiddleware(logger)
	// repository
	userDAO := mysqldao.NewUserDao(mysqlDB)
	// service
	userSvc := authService.NewUserService(newEmailUtil(conf.Smtp), userDAO)
	// controller
	hc := health.NewHealthController()
	emailAuthC := auth.NewController(userSvc, redisJWTHandler)
	srv := server.NewServer(hc, emailAuthC, cc, authM, corM, logM)
	return NewApp(srv, &conf)
}

func newEmailUtil(conf *config.SMTPConfig) *authService.EmailUtil {
	return &authService.EmailUtil{
		SMTPHost: conf.SmtpServer,
		SMTPPort: conf.SmtpPort,
		Email:    conf.SmtpEmail,
		Password: conf.SmtpCode,
	}
}
