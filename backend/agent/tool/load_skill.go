package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/agent-pilot/agent-pilot-be/agent/tool/skill"
)

type LoadSkillTool struct {
	Reg *skill.Registry
}

func (t *LoadSkillTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "load_skill",
		Desc: "Load a skill by name and inject it into the conversation. Reference file contents are not loaded automatically; use load_skill_references when needed.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"name": {
				Type:     schema.String,
				Desc:     "skill name",
				Required: true,
			},
		}),
	}, nil
}

func (t *LoadSkillTool) InvokableRun(
	ctx context.Context,
	args string,
	opts ...tool.Option,
) (string, error) {

	var input struct {
		Name string `json:"name"`
	}

	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "invalid load_skill arguments: " + err.Error(), nil
	}

	get := t.Reg.Get(input.Name)
	if get == nil {
		return "get not found: " + input.Name, nil
	}

	refNames := get.ReferenceNames()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[SKILL LOADED: %s]\n\n", get.Name))
	if get.Version != "" {
		sb.WriteString(fmt.Sprintf("Version: %s\n\n", get.Version))
	}
	writeSkillMetadata(&sb, get.Metadata)
	sb.WriteString(get.Content)

	if len(refNames) > 0 {
		sb.WriteString("\n\n=== REFERENCE FILES ===\n")
		for _, name := range refNames {
			sb.WriteString(fmt.Sprintf("- %s\n", name))
		}
		sb.WriteString("\nUse load_skill_references with the skill name and one or more file names to read reference content only when needed.\n")
	}

	return sb.String(), nil
}

func writeSkillMetadata(sb *strings.Builder, metadata skill.Metadata) {
	hasBins := len(metadata.Requires.Bins) > 0
	hasCLIHelp := metadata.CLIHelp != ""
	if !hasBins && !hasCLIHelp {
		return
	}

	sb.WriteString("=== METADATA ===\n")
	if hasBins {
		sb.WriteString("requires.bins:\n")
		for _, bin := range metadata.Requires.Bins {
			sb.WriteString(fmt.Sprintf("- %s\n", bin))
		}
	}
	if hasCLIHelp {
		sb.WriteString(fmt.Sprintf("cliHelp: %s\n", metadata.CLIHelp))
	}
	sb.WriteString("\n")
}

type LoadSkillReferencesTool struct {
	Reg *skill.Registry
}

func (t *LoadSkillReferencesTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "load_skill_references",
		Desc: "Load selected reference files for a skill. Use after load_skill when listed reference files are needed.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"name": {
				Type:     schema.String,
				Desc:     "skill name",
				Required: true,
			},
			"files": {
				Type:     schema.Array,
				Desc:     "reference file names to load, for example [\"usage.md\", \"api/errors.md\"]",
				Required: true,
				ElemInfo: &schema.ParameterInfo{
					Type: schema.String,
				},
			},
		}),
	}, nil
}

func (t *LoadSkillReferencesTool) InvokableRun(
	ctx context.Context,
	args string,
	opts ...tool.Option,
) (string, error) {

	var input struct {
		Name  string   `json:"name"`
		Files []string `json:"files"`
	}

	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "invalid load_skill_references arguments: " + err.Error(), nil
	}

	get := t.Reg.Get(input.Name)
	if get == nil {
		return "get not found: " + input.Name, nil
	}

	refs, err := get.LoadReferenceFiles(input.Files)
	if err != nil {
		return "failed to load skill references: " + err.Error(), nil
	}

	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[SKILL REFERENCES LOADED: %s]\n", get.Name))
	for _, name := range names {
		sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", name, refs[name]))
	}

	return sb.String(), nil
}
