package memory

import (
	"strings"

	"github.com/cloudwego/eino/schema"
)

const previewTitleMaxRunes = 80

// FirstUserPreviewTitle 从消息列表中取首条「真实用户」正文，用作会话列表标题（忽略系统桥接类 User 行）。
func FirstUserPreviewTitle(msgs []*schema.Message) string {
	for _, m := range msgs {
		if m == nil || m.Role != schema.User {
			continue
		}
		c := strings.TrimSpace(m.Content)
		if c == "" || isSyntheticSessionUserLine(c) {
			continue
		}
		return truncatePreviewTitleFirstLine(c, previewTitleMaxRunes)
	}
	return ""
}

func isSyntheticSessionUserLine(s string) bool {
	if strings.HasPrefix(s, "[会话]") {
		return true
	}
	if strings.Contains(s, "[专家专属线程") {
		return true
	}
	return false
}

func truncatePreviewTitleFirstLine(s string, maxRunes int) string {
	first := s
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		first = strings.TrimSpace(s[:idx])
	}
	first = strings.TrimSpace(first)
	if maxRunes <= 0 {
		return first
	}
	runes := []rune(first)
	if len(runes) <= maxRunes {
		return first
	}
	return string(runes[:maxRunes]) + "…"
}
