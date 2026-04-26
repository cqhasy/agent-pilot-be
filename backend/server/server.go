package server

import (
	"context"
	"github.com/agent-pilot/agent-pilot-be/controller/auth"
	"github.com/agent-pilot/agent-pilot-be/controller/health"
	"github.com/agent-pilot/agent-pilot-be/middleware"
	"github.com/agent-pilot/agent-pilot-be/server/router"
	"net/http"
	"time"
)

type Server struct {
	Router *http.Server
	close  func()
}

func NewServer(
	hc *health.Controller,
	ac *auth.LarkAuthController,
	AuthMiddleware *middleware.AuthMiddleware,
	corsMiddleware *middleware.CorsMiddleware,
	loggerMiddleware *middleware.LoggerMiddleware,
) *Server {
	return &Server{
		Router: &http.Server{
			Handler: router.NewRouter(AuthMiddleware, corsMiddleware, loggerMiddleware,
				hc, ac),
		},
	}
}

func (srv *Server) Run(addr string) error {
	srv.Router.Addr = addr

	if err := srv.Router.ListenAndServe(); err != nil {
		return err
	}

	return nil
}

func (srv *Server) Close() {
	if srv.close != nil {
		srv.close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Router.Shutdown(ctx)
}
