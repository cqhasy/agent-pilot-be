package auth

import (
	"errors"
	"strings"

	"github.com/agent-pilot/agent-pilot-be/controller/auth/service"
	autherrors "github.com/agent-pilot/agent-pilot-be/errors"
	"github.com/agent-pilot/agent-pilot-be/model"
	"github.com/agent-pilot/agent-pilot-be/pkg/jwt"
	"github.com/gin-gonic/gin"
)

type ControllerInterface interface {
	Login(ctx *gin.Context, req LoginRequest) (model.Response, error)
	Register(ctx *gin.Context, req RegisterRequest) (model.Response, error)
	SendEmail(ctx *gin.Context, req SendEmailRequest) (model.Response, error)
	GetMe(ctx *gin.Context) (model.Response, error)
	Logout(ctx *gin.Context) (model.Response, error)
}

type Controller struct {
	userService service.UserServiceInterface
	jwtHandler  *jwt.RedisJWTHandler
}

func NewController(userService service.UserServiceInterface, jwtHandler *jwt.RedisJWTHandler) *Controller {
	return &Controller{
		userService: userService,
		jwtHandler:  jwtHandler,
	}
}

func (c *Controller) Login(ctx *gin.Context, req LoginRequest) (model.Response, error) {
	email := strings.TrimSpace(req.UserEmail)
	if email == "" || req.Password == "" {
		return model.Response{}, autherrors.BAD_REQUEST_ERROR(errors.New("email and password are required"))
	}

	user, err := c.userService.Login(ctx.Request.Context(), email, req.Password)
	if err != nil {
		return model.Response{}, toAuthError(err)
	}

	token, err := c.jwtHandler.Jwt.SetJWTToken(user.ID, user.Name, "", user.Email, user.Avatar)
	if err != nil {
		return model.Response{}, err
	}

	return userLoginResponse(user.ID, user.Name, user.Email, user.Avatar, token), nil
}

func (c *Controller) Register(ctx *gin.Context, req RegisterRequest) (model.Response, error) {
	email := strings.TrimSpace(req.UserEmail)
	if email == "" || req.Password == "" || req.ConfirmPassword == "" || req.Code == "" {
		return model.Response{
			Code:    400,
			Message: "参数缺失",
		}, autherrors.BAD_REQUEST_ERROR(errors.New("email, password, confirmPassword and code are required"))
	}

	user, err := c.userService.Register(ctx.Request.Context(), strings.TrimSpace(req.UserName), email, req.Password, req.ConfirmPassword, req.Code)
	if err != nil {
		return model.Response{
			Code:    400,
			Message: err.Error(),
		}, toAuthError(err)
	}

	token, err := c.jwtHandler.Jwt.SetJWTToken(user.ID, user.Name, "", user.Email, user.Avatar)
	if err != nil {
		return model.Response{
			Code: 400,
		}, err
	}

	return userLoginResponse(user.ID, user.Name, user.Email, user.Avatar, token), nil
}

func (c *Controller) SendEmail(ctx *gin.Context, req SendEmailRequest) (model.Response, error) {
	email := strings.TrimSpace(req.UserEmail)
	if email == "" {
		return model.Response{
			Code:    400,
			Message: "email is required",
		}, autherrors.BAD_REQUEST_ERROR(errors.New("email is required"))
	}

	if err := c.userService.SendEmail(ctx.Request.Context(), email); err != nil {
		return model.Response{
			Code:    400,
			Message: err.Error(),
		}, err
	}

	return model.Response{
		Code:    200,
		Message: "ok",
	}, nil
}

func (c *Controller) GetMe(ctx *gin.Context) (model.Response, error) {
	claims, err := c.jwtHandler.ParseToken(ctx)
	if err != nil {
		return model.Response{}, err
	}

	return model.Response{
		Code:    200,
		Message: "ok",
		Data: LoginResponse{
			Id:       claims.Uid,
			UserName: claims.Name,
			Avatar:   claims.Avatar,
			Email:    claims.Email,
		},
	}, nil
}

func (c *Controller) Logout(ctx *gin.Context) (model.Response, error) {
	if err := c.jwtHandler.ClearToken(ctx); err != nil {
		return model.Response{}, err
	}

	return model.Response{
		Code:    200,
		Message: "ok",
		Data: map[string]string{
			"message": "logout success",
		},
	}, nil
}

func userLoginResponse(id uint, name, email, avatar, token string) model.Response {
	return model.Response{
		Code:    200,
		Message: "ok",
		Data: LoginResponse{
			Id:       id,
			UserName: name,
			Avatar:   avatar,
			Email:    email,
			Token:    token,
		},
	}
}

func toAuthError(err error) error {
	switch {
	case errors.Is(err, service.ErrUserNotFound),
		errors.Is(err, service.ErrPassword),
		errors.Is(err, service.ErrConfirmPassword),
		errors.Is(err, service.ErrEmailAlreadyExists),
		errors.Is(err, service.ErrCodeErrors),
		errors.Is(err, service.ErrTimeOut):
		return autherrors.BAD_REQUEST_ERROR(err)
	default:
		return err
	}
}
