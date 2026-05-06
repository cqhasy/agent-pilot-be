package chat

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	pkgmodel "github.com/agent-pilot/agent-pilot-be/model"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// WSCreateSession POST /api/v1/chat/ws/sessions — 在 Mongo 预创建主 WS 会话文档（与 WS 连接 session_id 对齐，供 history_items 等增量写入）。
func (c *Controller) WSCreateSession(ctx *gin.Context) {
	if c.WSMem == nil {
		ctx.JSON(http.StatusServiceUnavailable, pkgmodel.Response{
			Code:    503,
			Message: "websocket persistence not configured (Mongo)",
			Data:    nil,
		})
		return
	}
	var body struct {
		SessionID string `json:"session_id"`
	}
	_ = ctx.ShouldBindJSON(&body)
	sid := strings.TrimSpace(body.SessionID)
	if sid == "" {
		sid = uuid.NewString()
	}
	if strings.Contains(sid, ":") {
		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
			Code:    400,
			Message: "session_id must not contain ':' (reserved for expert checkpoint ids)",
			Data:    nil,
		})
		return
	}
	if err := c.WSMem.CreateWSRuntimeSession(ctx.Request.Context(), sid); err != nil {
		ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
			Code:    500,
			Message: err.Error(),
			Data:    nil,
		})
		return
	}
	ctx.JSON(http.StatusOK, pkgmodel.Response{
		Code:    0,
		Message: "ok",
		Data: gin.H{
			"session_id": sid,
		},
	})
}

// WSListSessions GET /api/v1/chat/ws/sessions — 列出已持久化的主 WS 会话（Mongo）。
func (c *Controller) WSListSessions(ctx *gin.Context) {
	if c.WSMem == nil {
		ctx.JSON(http.StatusServiceUnavailable, pkgmodel.Response{
			Code:    503,
			Message: "websocket persistence not configured (Mongo)",
			Data:    nil,
		})
		return
	}
	limit := 100
	if v := strings.TrimSpace(ctx.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	items, err := c.WSMem.ListWSRuntimeSessions(ctx.Request.Context(), limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
			Code:    500,
			Message: err.Error(),
			Data:    nil,
		})
		return
	}
	ctx.JSON(http.StatusOK, pkgmodel.Response{
		Code:    0,
		Message: "ok",
		Data: gin.H{
			"sessions": items,
		},
	})
}

// WSGetMessages GET /api/v1/chat/ws/sessions/:session_id/messages — 拉取该会话持久化对话历史。
func (c *Controller) WSGetMessages(ctx *gin.Context) {
	if c.WSMem == nil {
		ctx.JSON(http.StatusServiceUnavailable, pkgmodel.Response{
			Code:    503,
			Message: "websocket persistence not configured (Mongo)",
			Data:    nil,
		})
		return
	}
	sid := strings.TrimSpace(ctx.Param("session_id"))
	if sid == "" {
		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{Code: 400, Message: "session_id is required", Data: nil})
		return
	}
	msgs, err := c.WSMem.LoadWSHistory(ctx.Request.Context(), sid)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
			Code:    500,
			Message: err.Error(),
			Data:    nil,
		})
		return
	}
	includeSynthetic := strings.TrimSpace(ctx.Query("full")) == "1"
	ctx.JSON(http.StatusOK, pkgmodel.Response{
		Code:    0,
		Message: "ok",
		Data: gin.H{
			"session_id": sid,
			"messages":   wsMessagesToDTOs(msgs, includeSynthetic),
		},
	})
}

// WSGetExpertPreview GET /api/v1/chat/ws/sessions/:session_id/preview — 从历史派生右侧预览区 Markdown（与 WS expert_preview 语义一致）。
func (c *Controller) WSGetExpertPreview(ctx *gin.Context) {
	if c.WSMem == nil {
		ctx.JSON(http.StatusServiceUnavailable, pkgmodel.Response{
			Code:    503,
			Message: "websocket persistence not configured (Mongo)",
			Data:    nil,
		})
		return
	}
	sid := strings.TrimSpace(ctx.Param("session_id"))
	if sid == "" {
		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{Code: 400, Message: "session_id is required", Data: nil})
		return
	}
	expertID := strings.TrimSpace(ctx.Query("expert_id"))
	if expertID == "" {
		expertID = "document"
	}
	msgs, err := c.WSMem.LoadWSHistory(ctx.Request.Context(), sid)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
			Code:    500,
			Message: err.Error(),
			Data:    nil,
		})
		return
	}
	md := memory.ExpertPreviewMarkdownFromHistory(msgs)
	payload := PreviewPayloadForExpertContent(expertID, md)
	ctx.JSON(http.StatusOK, pkgmodel.Response{
		Code:    0,
		Message: "ok",
		Data: gin.H{
			"session_id": sid,
			"expert_id":  expertID,
			"preview":    payload,
			"has_body":   strings.TrimSpace(md) != "",
		},
	})
}

// skipSyntheticPlanUserMessage：invoke 在续跑时把 nextStepPrompt 写入 UserMessage，但从未通过 WS 推到前端；REST 默认过滤以免重放时多出「你」的长英文续跑句，与在线观感一致。
func skipSyntheticPlanUserMessage(m *schema.Message) bool {
	if m == nil || m.Role != schema.User {
		return false
	}
	c := strings.TrimSpace(m.Content)
	return strings.HasPrefix(c, "Continue the approved plan.")
}

func wsMessagesToDTOs(msgs []*schema.Message, includeSynthetic bool) []gin.H {
	if len(msgs) == 0 {
		return []gin.H{}
	}
	out := make([]gin.H, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}
		if !includeSynthetic && skipSyntheticPlanUserMessage(m) {
			continue
		}
		content := m.Content
		// 与 WS 聊天区一致：助手消息去掉预览区与泄漏标记，避免 REST 重放再出现 <!--DOC_PREVIEW_START / think 碎片
		if m.Role == schema.Assistant {
			content = memory.StripExpertPreviewRegions(content)
		}
		row := gin.H{
			"role":    string(m.Role),
			"content": content,
		}
		if len(m.ToolCalls) > 0 {
			row["tool_calls"] = m.ToolCalls
		}
		if m.ResponseMeta != nil {
			row["response_meta"] = m.ResponseMeta
		}
		out = append(out, row)
	}
	return out
}
