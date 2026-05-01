package chat

import "strings"

func shouldSkipPlanning(message string) bool {
	msg := strings.ToLower(strings.TrimSpace(message))
	if msg == "" {
		return true
	}
	planTriggers := []string{
		"审批", "考勤", "日历", "会议", "文档", "云文档", "飞书", "lark",
		"发送", "创建", "更新", "删除", "查询", "搜索", "导出", "下载", "上传",
		"tool", "tools", "api", "shell", "命令", "脚本", "workflow", "工作流",
		"计划", "步骤", "先", "然后", "并且", "multi-step",
	}
	for _, token := range planTriggers {
		if strings.Contains(msg, token) {
			return false
		}
	}
	return true
}

func isContinueMessage(message string) bool {
	msg := strings.ToLower(strings.TrimSpace(message))
	if msg == "" {
		return false
	}
	continueTokens := []string{
		"继续", "继续执行", "继续吧", "继续进行",
		"resume", "continue", "go on",
		"好的", "ok", "okay", "yes", "y", "yep", "sure",
	}
	for _, token := range continueTokens {
		if msg == token {
			return true
		}
	}
	// allow short variants like "继续一下"
	if strings.HasPrefix(msg, "继续") {
		return true
	}
	return false
}
