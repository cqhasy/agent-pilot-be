package expert

import "context"

type expertSessionKey struct{}

type registryCtxKey struct{}

// ExpertSession 由 WebSocket 会话实现：进入/退出专家专属线程（与主 Agent 上下文隔离）。
type ExpertSession interface {
	EnterExpertMode(expertID, taskBrief string)
	ExitExpertMode()
}

// WithExpertSession 将会话写入 context，供 handoff / release 工具使用。
func WithExpertSession(ctx context.Context, s ExpertSession) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, expertSessionKey{}, s)
}

// ExpertSessionFrom 取出会话；非 WS 或未注入时为 nil。
func ExpertSessionFrom(ctx context.Context) ExpertSession {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(expertSessionKey{}).(ExpertSession)
	return s
}

// WithRegistry 注入专家注册表，供运行时拼接专家系统指令（与 ExpertSession 配合使用）。
func WithRegistry(ctx context.Context, r *Registry) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, registryCtxKey{}, r)
}

// RegistryFrom 获取当前请求注入的注册表。
func RegistryFrom(ctx context.Context) *Registry {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(registryCtxKey{}).(*Registry)
	return r
}
