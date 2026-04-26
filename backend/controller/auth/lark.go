package auth

import (
	"encoding/binary"
	"fmt"
	"github.com/agent-pilot/agent-pilot-be/controller/auth/service"
	"github.com/agent-pilot/agent-pilot-be/model"
	"github.com/agent-pilot/agent-pilot-be/pkg/jwt"
	"github.com/agent-pilot/agent-pilot-be/pkg/state"
	"github.com/gin-gonic/gin"
	"hash/fnv"
	"net/http"
	"net/url"
	"strings"
)

type LarkAuthControllerInterface interface {
	GetFeishuLogin(ctx *gin.Context) (model.Response, error)
	FeishuCallbackGin(ctx *gin.Context)
	GetMe(ctx *gin.Context) (model.Response, error)
	Logout(ctx *gin.Context) (model.Response, error)
}

type LarkAuthController struct {
	appID       string
	appSecret   string
	redirectURI string
	stateSecret string
	service     service.LarkServiceInterface
	jwtHandler  *jwt.RedisJWTHandler
}

func NewLarkAuthController(appID, appSecret, redirectURI, stateSecret string,
	larkService *service.LarkService, handler *jwt.RedisJWTHandler) *LarkAuthController {
	return &LarkAuthController{
		appID:       appID,
		appSecret:   appSecret,
		redirectURI: redirectURI,
		stateSecret: stateSecret,
		service:     larkService,
		jwtHandler:  handler,
	}
}

func (ac *LarkAuthController) GetFeishuLogin(ctx *gin.Context) (model.Response, error) {
	returnTo := ctx.Query("returnTo")
	if strings.TrimSpace(returnTo) == "" {
		returnTo = "/"
	}

	// 限制跳转（防开放重定向）
	if !strings.HasPrefix(returnTo, "/") {
		returnTo = "/"
	}

	sta, _ := state.Generate(returnTo, ac.stateSecret)

	authURL := fmt.Sprintf(
		"https://open.feishu.cn/open-apis/authen/v1/authorize?app_id=%s&redirect_uri=%s&state=%s",
		url.QueryEscape(ac.appID),
		url.QueryEscape(ac.redirectURI),
		url.QueryEscape(sta),
	)

	return model.Response{
		Code:    200,
		Message: "ok",
		Data: map[string]string{
			"authUrl": authURL,
		},
	}, nil
}

func (ac *LarkAuthController) FeishuCallbackGin(ctx *gin.Context) {
	code := ctx.Query("code")
	sta := ctx.Query("state")
	if code == "" || sta == "" {
		ctx.Redirect(http.StatusFound, "/?login=failed")
		return
	}

	// 校验 state
	returnTo, err := state.Verify(sta, ac.stateSecret, 300)
	if err != nil {
		ctx.Redirect(http.StatusFound, "/?login=failed")
		return
	}

	// 防开放重定向
	if !strings.HasPrefix(returnTo, "/") {
		returnTo = "/"
	}

	// 换用户信息
	user, err := ac.service.ExchangeFeishuUser(ac.appID, ac.appSecret, ac.redirectURI, code)
	if err != nil {
		ctx.Redirect(http.StatusFound, "/?login=failed")
		return
	}

	uid := user.ID
	if uid == 0 {
		uid = stableUID(user.OpenID, user.UnionID, user.Email)
	}

	// 生成 JWT
	token, err := ac.jwtHandler.Jwt.SetJWTToken(uid, user.Name, user.OpenID, user.Email, user.Avatar)
	if err != nil {
		ctx.Redirect(http.StatusFound, "/?login=failed")
		return
	}

	// 带 token 重定向（简单版核心）
	target := returnTo
	if strings.Contains(target, "?") {
		target += "&"
	} else {
		target += "?"
	}
	target += "token=" + url.QueryEscape(token)

	ctx.Redirect(http.StatusFound, target)
}

func (ac *LarkAuthController) GetMe(ctx *gin.Context) (model.Response, error) {
	claims, err := ac.jwtHandler.ParseToken(ctx)
	if err != nil {
		return model.Response{}, err
	}
	name := strings.TrimSpace(claims.Name)
	if name == "" {
		name = "飞书用户"
	}

	return model.Response{
		Code:    200,
		Message: "ok",
		Data: map[string]interface{}{
			"id":     claims.Uid,
			"name":   name,
			"email":  claims.Email,
			"avatar": claims.Avatar,
			"openId": claims.OpenID,
		},
	}, nil
}

func (ac *LarkAuthController) Logout(ctx *gin.Context) (model.Response, error) {
	if err := ac.jwtHandler.ClearToken(ctx); err != nil {
		return model.Response{}, err
	}

	return model.Response{
		Code:    200,
		Message: "ok",
		Data: map[string]string{
			"message": "已退出登录",
		},
	}, nil
}

func stableUID(openID, unionID, email string) uint {
	seed := strings.TrimSpace(unionID)
	if seed == "" {
		seed = strings.TrimSpace(openID)
	}
	if seed == "" {
		seed = strings.TrimSpace(email)
	}
	if seed == "" {
		return 1
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, h.Sum64())
	return uint(binary.LittleEndian.Uint32(buf))
}
