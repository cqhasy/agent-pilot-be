package tool

import (
	"github.com/agent-pilot/agent-pilot-be/agent/tool/skill"
	einotool "github.com/cloudwego/eino/components/tool"
)

// BuildTools 构建工具列表
func BuildTools(reg *skill.Registry) []einotool.BaseTool {
	return []einotool.BaseTool{
		&LoadSkillTool{Reg: reg},
		&LoadSkillReferencesTool{Reg: reg},
		&PlanStepTool{},
		&RequestUserInputTool{},
		&ShellTool{},
	}
}
