package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agent-pilot/agent-pilot-be/agent/tool/skill"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type LLMPlanner struct {
	model  model.ToolCallingChatModel
	skills []*skill.Skill
	now    func() time.Time
}

func NewLLMPlanner(chatModel model.ToolCallingChatModel, skillReg *skill.Registry) *LLMPlanner {
	var skills []*skill.Skill
	if skillReg != nil {
		skills = skillReg.List()
	}

	return &LLMPlanner{
		model:  chatModel,
		skills: skills,
		now:    time.Now,
	}
}

func (p *LLMPlanner) Plan(ctx context.Context, req Request) (*Plan, error) {
	if p == nil || p.model == nil {
		return nil, fmt.Errorf("planner model is nil")
	}
	if strings.TrimSpace(req.UserInput) == "" {
		return nil, fmt.Errorf("user input is required")
	}

	messages := []*schema.Message{
		schema.SystemMessage(p.systemPrompt()),
		schema.UserMessage(p.userPrompt(req)),
	}

	resp, err := p.model.Generate(ctx, messages)
	if err != nil {
		return nil, err
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return nil, fmt.Errorf("planner returned empty response")
	}

	out, err := parseOutput(resp.Content)
	if err != nil {
		return nil, err
	}

	now := p.now()
	plan := &Plan{
		ID:              NewID("plan"),
		SessionID:       req.SessionID,
		Objective:       strings.TrimSpace(out.Objective),
		Summary:         strings.TrimSpace(out.Summary),
		SubjectiveState: out.SubjectiveState,
		Assumptions:     compactStrings(out.Assumptions),
		Constraints:     compactStrings(out.Constraints),
		Steps:           normalizeSteps(out.Steps),
		Status:          StatusReady,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if plan.Objective == "" {
		plan.Objective = strings.TrimSpace(req.UserInput)
	}
	if plan.Summary == "" {
		plan.Summary = "Plan generated from user request."
	}
	if plan.SubjectiveState.Goal == "" {
		plan.SubjectiveState.Goal = plan.Objective
	}
	if plan.SubjectiveState.Stance == "" {
		plan.SubjectiveState.Stance = "proactive, careful, and checkpoint-aware"
	}

	return plan, nil
}

func (p *LLMPlanner) systemPrompt() string {
	var sb strings.Builder
	sb.WriteString(`You are the planning layer for a plan-execute agent.

Create a compact, executable plan before any action is taken. The executor will later use ReAct and skill tools, so your job is to decide intent, subjective stance, skill choices, dependencies, and checkpoints.

Rules:
- Return JSON only. No markdown fences.
- Do not execute tools.
- Prefer available skills when they match a step.
- A step should be small enough for one ReAct execution loop.
- Use clarifying_needs only for missing information that blocks safe planning.
- Keep plans practical and short unless the user asks for a broad project.

Required JSON shape:
{
  "objective": "string",
  "summary": "string",
  "subjective_state": {
    "goal": "string",
    "stance": "string",
    "preferences": ["string"],
    "risk_awareness": ["string"],
    "clarifying_needs": ["string"]
  },
  "assumptions": ["string"],
  "constraints": ["string"],
  "steps": [
    {
      "title": "string",
      "purpose": "string",
      "expected_outcome": "string",
      "skill": "optional skill name",
      "inputs": {"key": "value"},
      "dependencies": ["step id or title"]
    }
  ]
}

Available skills:
`)

	for _, s := range p.skills {
		if s == nil || s.DisableModelInvocation {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(s.Name)
		if s.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(strings.ReplaceAll(s.Description, "\n", " "))
		}
		if s.WhenToUse != "" {
			sb.WriteString(" When to use: ")
			sb.WriteString(strings.ReplaceAll(s.WhenToUse, "\n", " "))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (p *LLMPlanner) userPrompt(req Request) string {
	var sb strings.Builder
	sb.WriteString("User request:\n")
	sb.WriteString(req.UserInput)
	sb.WriteString("\n\nRecent conversation context:\n")

	for _, msg := range lastMessages(req.History, 8) {
		if msg == nil || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(string(msg.Role))
		sb.WriteString(": ")
		sb.WriteString(trimForPrompt(msg.Content, 1200))
		sb.WriteString("\n")
	}

	return sb.String()
}

type plannerOutput struct {
	Objective       string          `json:"objective"`
	Summary         string          `json:"summary"`
	SubjectiveState SubjectiveState `json:"subjective_state"`
	Assumptions     []string        `json:"assumptions"`
	Constraints     []string        `json:"constraints"`
	Steps           []Step          `json:"steps"`
}

func parseOutput(content string) (*plannerOutput, error) {
	raw := strings.TrimSpace(content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}

	var out plannerOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("decode planner output: %w", err)
	}
	return &out, nil
}

func normalizeSteps(steps []Step) []Step {
	out := make([]Step, 0, len(steps))
	for i, step := range steps {
		step.Title = strings.TrimSpace(step.Title)
		step.Purpose = strings.TrimSpace(step.Purpose)
		step.ExpectedOutcome = strings.TrimSpace(step.ExpectedOutcome)
		step.Skill = strings.TrimSpace(step.Skill)
		step.Dependencies = compactStrings(step.Dependencies)
		if step.Inputs == nil {
			step.Inputs = map[string]string{}
		}
		if step.ID == "" {
			step.ID = fmt.Sprintf("step_%02d", i+1)
		}
		if step.Status == "" {
			step.Status = StepStatusPending
		}
		if step.Title == "" {
			step.Title = fmt.Sprintf("Step %d", i+1)
		}
		out = append(out, step)
	}
	return out
}

func compactStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func lastMessages(in []*schema.Message, n int) []*schema.Message {
	if len(in) <= n {
		return in
	}
	return in[len(in)-n:]
}

func trimForPrompt(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
