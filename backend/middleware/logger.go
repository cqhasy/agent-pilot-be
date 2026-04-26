package middleware

import (
	"fmt"
	api_errors "github.com/agent-pilot/agent-pilot-be/errors"
	"github.com/agent-pilot/agent-pilot-be/model"
	"github.com/agent-pilot/agent-pilot-be/pkg/errorx"
	"github.com/agent-pilot/agent-pilot-be/pkg/ginx"
	"github.com/agent-pilot/agent-pilot-be/pkg/logger"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

type LoggerMiddleware struct {
	log logger.Logger
}

func NewLoggerMiddleware(log logger.Logger) *LoggerMiddleware {
	return &LoggerMiddleware{
		log: log,
	}
}

// NormalMiddlewareFunc 处理非流式响应日志
func (lm *LoggerMiddleware) NormalMiddlewareFunc() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Next() // 执行后续逻辑

		// 处理返回值或错误
		res, httpCode := lm.handleNormalResponse(ctx)
		if !ctx.IsAborted() { // 避免重复返回响应
			ctx.JSON(httpCode, res)
		}
	}
}

// 提取的日志逻辑：记录自定义错误日志
func (lm *LoggerMiddleware) logCustomError(customError *errorx.CustomError, ctx *gin.Context) {
	lm.log.Error("处理请求出错",
		logger.Error(customError),
		logger.String("timestamp", time.Now().Format(time.RFC3339)),
		logger.String("ip", ctx.ClientIP()),
		logger.String("path", ctx.Request.URL.Path),
		logger.String("method", ctx.Request.Method),
		logger.String("headers", fmt.Sprintf("%v", ctx.Request.Header)),
		logger.Int("httpCode", customError.HttpCode),
		logger.Int("code", customError.Code),
		logger.String("msg", customError.Msg),
		logger.String("category", customError.Category),
		logger.String("file", customError.File),
		logger.Int("line", customError.Line),
		logger.String("function", customError.Function),
	)
}

// 提取的日志逻辑：记录未知错误日志
func (lm *LoggerMiddleware) logUnexpectedError(err error, ctx *gin.Context) {
	lm.log.Error("意外错误类型",
		logger.Error(err),
		logger.String("timestamp", time.Now().Format(time.RFC3339)),
		logger.String("ip", ctx.ClientIP()),
		logger.String("path", ctx.Request.URL.Path),
		logger.String("method", ctx.Request.Method),
		logger.String("headers", fmt.Sprintf("%v", ctx.Request.Header)),
	)
}
func (lm *LoggerMiddleware) commonInfo(ctx *gin.Context) {
	lm.log.Info("常规日志",
		logger.String("timestamp", time.Now().Format(time.RFC3339)),
		logger.String("ip", ctx.ClientIP()),
		logger.String("path", ctx.Request.URL.Path),
		logger.String("method", ctx.Request.Method),
		logger.String("headers", fmt.Sprintf("%v", ctx.Request.Header)),
	)
}

// 处理非流式响应逻辑
func (lm *LoggerMiddleware) handleNormalResponse(ctx *gin.Context) (model.Response, int) {
	var res model.Response
	httpCode := ctx.Writer.Status()

	//有错误则进行错误处理
	if len(ctx.Errors) > 0 {
		err := ctx.Errors.Last().Err
		customError := errorx.ToCustomError(err)
		if customError == nil {
			lm.logUnexpectedError(err, ctx)
			return model.Response{Code: api_errors.ERROR_TYPE_ERROR_CODE, Message: err.Error(), Data: nil}, http.StatusInternalServerError
		}
		lm.logCustomError(customError, ctx)
		return model.Response{Code: customError.Code, Message: customError.Msg, Data: nil}, customError.HttpCode
	} else {

		//无错误则记录常规日志
		lm.commonInfo(ctx)
		res = ginx.GetResp[model.Response](ctx)
	}

	return res, httpCode
}
