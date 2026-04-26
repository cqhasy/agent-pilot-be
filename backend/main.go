package main

import (
	"errors"
	"github.com/agent-pilot/agent-pilot-be/config"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/agent-pilot/agent-pilot-be/server"
)

func main() {
	// 可选加载 .env（不存在则忽略）；容器内建议用环境变量或 env_file 注入
	_ = godotenv.Load()
	app := initWebServer()
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic recovered: %v", err)
				app.srv.Close()
			}
		}()
		app.Run()
	}()

	signal.Notify(app.sig, syscall.SIGINT, syscall.SIGTERM)
	<-app.sig

	log.Println("Shutting down server...")
	app.srv.Close()
	log.Println("main exit")
}

type App struct {
	srv *server.Server
	c   *config.Config
	sig chan os.Signal
}

func NewApp(srv *server.Server, c *config.Config) *App {
	return &App{
		srv: srv,
		c:   c,
		sig: make(chan os.Signal, 1),
	}
}

// 启动
func (a *App) Run() {
	if err := a.srv.Run(a.c.Addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
