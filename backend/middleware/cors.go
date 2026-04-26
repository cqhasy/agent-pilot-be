package middleware

import (
	"github.com/agent-pilot/agent-pilot-be/config"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"time"
)

type CorsMiddleware struct {
	allowedOrigins []string
}

func NewCorsMiddleware(conf *config.CorMiddlewareConfig) *CorsMiddleware {
	return &CorsMiddleware{allowedOrigins: conf.AllowedOrigins}
}

func (cm *CorsMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return cors.New(cors.Config{
		// 允许的请求头
		AllowHeaders: []string{"Content-Type", "Authorization", "Origin"},
		// 是否允许携带凭证（如 Cookies）
		AllowCredentials: true,
		// 解决跨域问题,这个地方允许所有请求跨域了,之后要改成允许前端的请求,比如localhost
		AllowOriginFunc: func(origin string) bool {
			//暂时允许所有跨域请求,根据需要进行调整
			return true
			//只允许在列表里面的origin可以跨域
			//if slices.Contains(cm.allowedOrigins, origin) {
			//	return true
			//} else {
			//	return false
			//}
		},

		// 预检请求的缓存时间
		MaxAge: 12 * time.Hour,
	})
}
