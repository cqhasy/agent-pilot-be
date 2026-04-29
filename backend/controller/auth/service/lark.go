package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type LarkServiceInterface interface {
	ExchangeFeishuUser(appID, appSecret, redirectURI, code string) (FeishuUser, error)
}

type LarkService struct{}

func NewLarkService() *LarkService {
	return &LarkService{}
}

func (s *LarkService) ExchangeFeishuUser(appID, appSecret, redirectURI, code string) (FeishuUser, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	token, err := getFeishuAccessToken(client, appID, appSecret, code, redirectURI)
	if err != nil {
		return FeishuUser{}, err
	}

	user, err := getFeishuUserInfo(client, token)
	if err != nil {
		return FeishuUser{}, err
	}

	return user, nil
}

func getFeishuAccessToken(client *http.Client, appID, appSecret, code, redirectURI string) (string, error) {
	body := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"client_id":     appID,
		"client_secret": appSecret,
	}

	if redirectURI != "" {
		body["redirect_uri"] = redirectURI
	}

	data, _ := json.Marshal(body)

	// 封装 fallback
	endpoints := []string{
		"https://open.feishu.cn/open-apis/authen/v1/oidc/access_token",
		"https://open.feishu.cn/open-apis/authen/v2/oauth/token",
	}

	for _, ep := range endpoints {
		token, err := requestFeishuToken(client, ep, data)
		if err == nil {
			return token, nil
		}
	}

	return "", fmt.Errorf("all token endpoints failed")
}

func getFeishuUserInfo(client *http.Client, accessToken string) (FeishuUser, error) {
	req, err := http.NewRequest(http.MethodGet,
		"https://open.feishu.cn/open-apis/authen/v1/user_info", nil)
	if err != nil {
		return FeishuUser{}, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return FeishuUser{}, fmt.Errorf("user info request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	var r struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Name    string `json:"name"`
			OpenID  string `json:"open_id"`
			UnionID string `json:"union_id"`
			Avatar  string `json:"avatar_url"`
			Email   string `json:"email"`
		} `json:"data"`
	}

	if err := json.Unmarshal(raw, &r); err != nil {
		return FeishuUser{}, fmt.Errorf("decode user info failed: %s", string(raw))
	}

	if r.Code != 0 {
		return FeishuUser{}, fmt.Errorf("feishu error: %s", r.Msg)
	}

	return FeishuUser{
		Name:            r.Data.Name,
		OpenID:          r.Data.OpenID,
		UnionID:         r.Data.UnionID,
		Avatar:          r.Data.Avatar,
		Email:           r.Data.Email,
		UserAccessToken: accessToken,
	}, nil
}

func requestFeishuToken(client *http.Client, endpoint string, body []byte) (string, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed (%s): %w", endpoint, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	// HTTP 状态码先判断
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http error (%s): status=%d body=%s",
			endpoint, resp.StatusCode, string(raw))
	}

	// 兼容多种返回结构
	var tokenResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`

		AccessToken string `json:"access_token"`

		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}

	if err := json.Unmarshal(raw, &tokenResp); err != nil {
		return "", fmt.Errorf("decode failed (%s): %s", endpoint, string(raw))
	}

	// 兼容两种位置
	accessToken := strings.TrimSpace(tokenResp.AccessToken)
	if accessToken == "" {
		accessToken = strings.TrimSpace(tokenResp.Data.AccessToken)
	}

	if tokenResp.Code != 0 || accessToken == "" {
		return "", fmt.Errorf(
			"token exchange failed (%s): code=%d msg=%s raw=%s",
			endpoint, tokenResp.Code, tokenResp.Msg, string(raw),
		)
	}

	return accessToken, nil
}
