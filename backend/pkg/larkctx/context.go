package larkctx

import (
	"context"
	"strings"
)

type userAccessTokenKey struct{}

func WithUserAccessToken(ctx context.Context, token string) context.Context {
	token = strings.TrimSpace(token)
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, userAccessTokenKey{}, token)
}

func UserAccessToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(userAccessTokenKey{}).(string)
	token = strings.TrimSpace(token)
	return token, ok && token != ""
}
