package tool

import (
	"context"
	"encoding/json"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const PlanStepToolName = "update_plan_step"

type PlanStepUpdate struct {
	StepID string `json:"step_id"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

type PlanStepUpdater func(ctx context.Context, input PlanStepUpdate) (string, error)

type planStepUpdaterKey struct{}

type PlanStepTool struct{}

func WithPlanStepUpdater(ctx context.Context, updater PlanStepUpdater) context.Context {
	return context.WithValue(ctx, planStepUpdaterKey{}, updater)
}

func (t *PlanStepTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: PlanStepToolName,
		Desc: "Update the exact execution status of one approved plan step. Use this only for plan bookkeeping; it does not execute business actions.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"step_id": {
				Type:     schema.String,
				Desc:     "Exact step id from the approved plan, for example step_01.",
				Required: true,
			},
			"status": {
				Type:     schema.String,
				Desc:     "One of: pending, running, completed, failed, skipped.",
				Required: true,
			},
			"note": {
				Type: schema.String,
				Desc: "Short reason or result summary.",
			},
		}),
	}, nil
}

func (t *PlanStepTool) InvokableRun(ctx context.Context, args string, opts ...einotool.Option) (string, error) {
	var input PlanStepUpdate
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "invalid update_plan_step arguments: " + err.Error(), nil
	}

	updater, ok := ctx.Value(planStepUpdaterKey{}).(PlanStepUpdater)
	if !ok || updater == nil {
		return "no active plan step updater", nil
	}

	result, err := updater(ctx, input)
	if err != nil {
		return fmt.Sprintf("update_plan_step failed: %v", err), nil
	}
	return result, nil
}
