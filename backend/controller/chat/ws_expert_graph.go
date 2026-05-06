package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/agent-pilot/agent-pilot-be/agent/expert"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

// ExpertComposeDeps 自定义专家图编译可用依赖。
type ExpertComposeDeps struct {
	Model           model.ToolCallingChatModel
	Tools           []einotool.BaseTool
	CheckpointStore compose.CheckPointStore
	ExpertRegistry  *expert.Registry
	GraphName       string
}

// ExpertComposeFactory 替换某一 expert_id 的默认 ReAct 编译逻辑，返回完整 *composeRuntime（可自建任意 eino Graph）。
type ExpertComposeFactory func(ctx context.Context, def expert.Definition, deps ExpertComposeDeps) (*composeRuntime, error)

var (
	expertComposeMu     sync.RWMutex
	customExpertCompose = map[string]ExpertComposeFactory{}
)

// RegisterExpertCompose 注册自定义专家编排（预览节点、分支审批等）。进程启动后、EnableWebSocketChat 前调用。
func RegisterExpertCompose(expertID string, fn ExpertComposeFactory) {
	expertComposeMu.Lock()
	defer expertComposeMu.Unlock()
	customExpertCompose[expertID] = fn
}

func lookupExpertComposeFactory(expertID string) ExpertComposeFactory {
	expertComposeMu.RLock()
	defer expertComposeMu.RUnlock()
	return customExpertCompose[expertID]
}

// BuildExpertGraphSystem 专家独立 compose 使用的系统提示（与主会话 BuildSystemPrompt 分离）。
func BuildExpertGraphSystem(def *expert.Definition) string {
	if def == nil {
		return ""
	}
	return buildExpertFocusedSystem(def)
}

// BuildExpertComposeRuntimes 为注册表中每位专家编译独立 compose（独立 graph name / checkpoint 命名空间由上层 session id 后缀区分）。
func BuildExpertComposeRuntimes(ctx context.Context, chatModel model.ToolCallingChatModel, tools []einotool.BaseTool, checkpointStore compose.CheckPointStore, reg *expert.Registry) (map[string]*composeRuntime, error) {
	if reg == nil {
		return nil, nil
	}
	out := make(map[string]*composeRuntime)
	for _, d := range reg.List() {
		id := d.ID
		sys := BuildExpertGraphSystem(&d)
		gn := GraphNameExpertPrefix + id
		deps := ExpertComposeDeps{
			Model:           chatModel,
			Tools:           tools,
			CheckpointStore: checkpointStore,
			ExpertRegistry:  reg,
			GraphName:       gn,
		}
		if fn := lookupExpertComposeFactory(id); fn != nil {
			rt, err := fn(ctx, d, deps)
			if err != nil {
				return nil, fmt.Errorf("expert graph %s: %w", id, err)
			}
			out[id] = rt
			continue
		}
		rt, err := compileComposeRuntime(ctx, chatModel, tools, sys, checkpointStore, composeCompileOptions{
			graphName:         gn,
			experts:           reg,
			mainHandoffRewire: false,
			isExpertGraph:     true,
		})
		if err != nil {
			return nil, fmt.Errorf("default expert graph %s: %w", id, err)
		}
		out[id] = rt
	}
	return out, nil
}

func (h *wsHub) pickComposeRuntime(session *wsSession) *composeRuntime {
	eid := strings.TrimSpace(session.activeExpertIDSnapshot())
	if eid != "" && h.expertRuntimes != nil {
		if rt := h.expertRuntimes[eid]; rt != nil {
			return rt
		}
	}
	return h.runtime
}

func composeCheckpointID(session *wsSession) string {
	eid := strings.TrimSpace(session.activeExpertIDSnapshot())
	if eid != "" {
		return session.id + ":expert:" + eid
	}
	return session.id
}

func expertRegistryForHub(h *wsHub) *expert.Registry {
	if h == nil {
		return nil
	}
	if h.expertReg != nil {
		return h.expertReg
	}
	if h.runtime != nil {
		return h.runtime.experts
	}
	return nil
}

// BroadcastExpertPreview 向 WS 推送 expert_preview 事件，供 frontend/ws.html 右侧预览区渲染。建议 data 形态：
//   文档: map[string]any{"kind":"document","title":"...","format":"markdown|html|text","content":"..."}
//   演示: map[string]any{"kind":"presentation","title":"...","xml":"<presentation>...</presentation>"} 或与工具结果一致的含 <presentation> 的嵌套结构
func BroadcastExpertPreview(session *wsSession, requestID string, data any) {
	if session == nil {
		return
	}
	session.broadcast(wsOutput{Type: wsEventExpertPreview, SessionID: session.id, RequestID: requestID, Data: data})
}

// PreviewPayloadForExpertContent 根据专家 ID 与「预览区正文」构造 expert_preview 载荷；content 应先经 memory.ExtractExpertPreviewMarkdown 过滤（仅标记内 / 围栏内正文）。
func PreviewPayloadForExpertContent(expertID, content string) map[string]any {
	eid := strings.TrimSpace(expertID)
	if eid == "" {
		return nil
	}
	c := content // 保留空白以便 Markdown 缩进；仅 expert_id 必填
	switch eid {
	case "document":
		return map[string]any{
			"kind": "document", "title": "文档草稿", "format": "markdown", "content": c,
		}
	case "presentation":
		return map[string]any{
			"kind": "presentation", "title": "演示草稿", "content": c,
		}
	default:
		return map[string]any{
			"kind": "document", "title": "专家输出", "format": "markdown", "content": c, "expert_id": eid,
		}
	}
}
