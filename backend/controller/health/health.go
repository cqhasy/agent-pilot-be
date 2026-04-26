package health

import (
	"github.com/agent-pilot/agent-pilot-be/model"
	"github.com/gin-gonic/gin"
)

type ControllerInterface interface {
	GetHealth(c *gin.Context) (model.Response, error)
}
type Controller struct {
}

func NewHealthController() *Controller {
	return &Controller{}
}

func (hc *Controller) GetHealth(ctx *gin.Context) (model.Response, error) {
	return model.Response{Code: 200, Message: "healthy"}, nil
}
