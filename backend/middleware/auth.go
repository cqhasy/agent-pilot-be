package middleware

import (
	api_errors "github.com/agent-pilot/agent-pilot-be/errors"
	"github.com/agent-pilot/agent-pilot-be/pkg/ginx"
	"github.com/agent-pilot/agent-pilot-be/pkg/jwt"
	"github.com/gin-gonic/gin"
)

type AuthMiddleware struct {
	jwtHandler *jwt.RedisJWTHandler
}

func NewAuthMiddleware(jwtHandler *jwt.RedisJWTHandler) *AuthMiddleware {
	return &AuthMiddleware{jwtHandler: jwtHandler}
}

func (am *AuthMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 从请求中提取并解析 Token

		userClaims, err := am.jwtHandler.ParseToken(ctx)

		if err != nil {
			ctx.Error(api_errors.UNAUTHORIED_ERROR(err))
			return
		}

		// 将解析后的用户信息存入上下文，供后续逻辑使用
		ginx.SetClaims[jwt.UserClaims](ctx, userClaims)

		// 继续处理请求
		ctx.Next()
	}
}
