package react

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	einomodel "github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const defaultMaxTurns = 8

type Executor struct {
	model        einomodel.ToolCallingChatModel
	tools        []einotool.BaseTool
	checkpointer agentplan.Checkpointer
	maxTurns     int
	now          func() time.Time
}

type Result struct {
	Plan    *agentplan.Plan `json:"plan"`
	Steps   []StepResult    `json:"steps"`
	Summary string          `json:"summary"`
}

type StepResult struct {
	StepID string `json:"step_id"`
	Output string `json:"output"`
}

func NewExecutor(
	model einomodel.ToolCallingChatModel,
	tools []einotool.BaseTool,
	checkpointer agentplan.Checkpointer,
) *Executor {
	return &Executor{
		model:        model,
		tools:        tools,
		checkpointer: checkpointer,
		maxTurns:     defaultMaxTurns,
		now:          time.Now,
	}
}

func (e *Executor) Execute(ctx context.Context, p *agentplan.Plan) (*Result, error) {
	if e == nil || e.model == nil {
		return nil, fmt.Errorf("react executor model is nil")
	}
	if p == nil {
		return nil, fmt.Errorf("plan is nil")
	}

	toolInfos, invokables, err := e.prepareTools(ctx)
	if err != nil {
		return nil, err
	}

	modelWithTools, err := e.model.WithTools(toolInfos)
	if err != nil {
		return nil, err
	}

	p.Status = agentplan.StatusExecuting
	p.UpdatedAt = e.now()

	results := make([]StepResult, 0, len(p.Steps))
	for i := range p.Steps {
		step := &p.Steps[i]
		if step.Status == agentplan.StepStatusCompleted || step.Status == agentplan.StepStatusSkipped {
			continue
		}

		step.Status = agentplan.StepStatusRunning
		p.UpdatedAt = e.now()
		e.saveCheckpoint(ctx, p, step.ID, "step_started")

		out, err := e.executeStep(ctx, modelWithTools, invokables, p, step)
		if err != nil {
			step.Status = agentplan.StepStatusFailed
			p.Status = agentplan.StatusFailed
			p.UpdatedAt = e.now()
			e.saveCheckpoint(ctx, p, step.ID, "step_failed")
			return &Result{Plan: p, Steps: results, Summary: out}, err
		}

		step.Status = agentplan.StepStatusCompleted
		p.UpdatedAt = e.now()
		e.saveCheckpoint(ctx, p, step.ID, "step_completed")
		results = append(results, StepResult{StepID: step.ID, Output: out})
	}

	p.Status = agentplan.StatusCompleted
	p.UpdatedAt = e.now()
	e.saveCheckpoint(ctx, p, "", "plan_completed")

	return &Result{
		Plan:    p,
		Steps:   results,
		Summary: summarizeResults(results),
	}, nil
}

func (e *Executor) executeStep(
	ctx context.Context,
	model einomodel.ToolCallingChatModel,
	tools map[string]einotool.InvokableTool,
	p *agentplan.Plan,
	step *agentplan.Step,
) (string, error) {
	messages := []*schema.Message{
		schema.SystemMessage(e.systemPrompt()),
		schema.UserMessage(stepPrompt(p, step)),
	}

	for turn := 0; turn < e.maxTurns; turn++ {
		msg, err := model.Generate(ctx, messages)
		if err != nil {
			return "", err
		}
		if msg == nil {
			return "", fmt.Errorf("model returned nil message")
		}

		messages = append(messages, msg)
		if len(msg.ToolCalls) == 0 {
			return strings.TrimSpace(msg.Content), nil
		}

		for _, call := range msg.ToolCalls {
			toolName := call.Function.Name
			t, ok := tools[toolName]
			if !ok {
				messages = append(messages, schema.ToolMessage("unknown tool: "+toolName, call.ID, schema.WithToolName(toolName)))
				continue
			}

			result, err := t.InvokableRun(ctx, call.Function.Arguments)
			if err != nil {
				result = "tool execution error: " + err.Error()
			}
			messages = append(messages, schema.ToolMessage(result, call.ID, schema.WithToolName(toolName)))
		}
	}

	return "", fmt.Errorf("react executor exceeded max turns for step %s", step.ID)
}

func (e *Executor) prepareTools(ctx context.Context) ([]*schema.ToolInfo, map[string]einotool.InvokableTool, error) {
	infos := make([]*schema.ToolInfo, 0, len(e.tools))
	invokables := make(map[string]einotool.InvokableTool, len(e.tools))

	for _, baseTool := range e.tools {
		info, err := baseTool.Info(ctx)
		if err != nil {
			return nil, nil, err
		}
		infos = append(infos, info)

		if invokable, ok := baseTool.(einotool.InvokableTool); ok {
			invokables[info.Name] = invokable
		}
	}

	return infos, invokables, nil
}

func (e *Executor) systemPrompt() string {
	return `You are the execute layer of a plan-execute agent.

Use ReAct:
- Think silently about the next action.
- Use load_skill before following a skill.
- Use load_skill_references only when the loaded skill lists reference files needed for the step.
- Use shell for lark-cli commands.
- Lark user access token is already provided by the runtime environment. Do not ask the user for it and never print it.
- Prefer --as user for user-owned Lark resources unless the skill says bot identity is required.
- Stop after this step is complete and return a concise result.`
}

func stepPrompt(p *agentplan.Plan, step *agentplan.Step) string {
	var sb strings.Builder
	sb.WriteString("Plan objective:\n")
	sb.WriteString(p.Objective)
	sb.WriteString("\n\nCurrent step:\n")
	sb.WriteString("ID: " + step.ID + "\n")
	sb.WriteString("Title: " + step.Title + "\n")
	sb.WriteString("Purpose: " + step.Purpose + "\n")
	sb.WriteString("Expected outcome: " + step.ExpectedOutcome + "\n")
	if step.Skill != "" {
		sb.WriteString("Preferred skill: " + step.Skill + "\n")
	}
	if len(step.Inputs) > 0 {
		sb.WriteString("\nInputs:\n")
		for k, v := range step.Inputs {
			sb.WriteString("- " + k + ": " + v + "\n")
		}
	}
	return sb.String()
}

func (e *Executor) saveCheckpoint(ctx context.Context, p *agentplan.Plan, stepID, reason string) {
	if e.checkpointer == nil {
		return
	}
	if stepSaver, ok := e.checkpointer.(interface {
		SaveStep(context.Context, *agentplan.Plan, string, string) (*agentplan.Checkpoint, error)
	}); ok {
		_, _ = stepSaver.SaveStep(ctx, p, stepID, reason)
		return
	}
	_, _ = e.checkpointer.Save(ctx, p, reason)
}

func summarizeResults(results []StepResult) string {
	var sb strings.Builder
	for _, result := range results {
		if result.Output == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(result.Output)
	}
	return strings.TrimSpace(sb.String())
}
