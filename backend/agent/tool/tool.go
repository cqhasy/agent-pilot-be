package tool

import (
	"github.com/agent-pilot/agent-pilot-be/agent/expert"
	"github.com/agent-pilot/agent-pilot-be/agent/tool/skill"
	einotool "github.com/cloudwego/eino/components/tool"
)

// BuildTools 构建工具列表。expertReg 为 nil 时不注册 handoff 相关工具（保持纯通用 Agent）。
func BuildTools(reg *skill.Registry, expertReg *expert.Registry) []einotool.BaseTool {
	tools := []einotool.BaseTool{
		&LoadSkillTool{Reg: reg},
		&LoadSkillReferencesTool{Reg: reg},
		&PlanStepTool{},
		&RequestUserInputTool{},
		&ShellTool{},
	}
	if expertReg != nil {
		tools = append(tools,
			&HandoffToExpertTool{Reg: expertReg},
			&ReleaseExpertTool{},
		)
	}
	return tools
}
