package chat

import (
	"context"
	"fmt"
	"strings"

	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	agenttool "github.com/agent-pilot/agent-pilot-be/agent/tool"
	"github.com/gin-gonic/gin"
)

func (h *wsHub) preparePlan(ctx context.Context, session *wsSession, input wsInput) error {
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return fmt.Errorf("message is required")
	}
	if h.planner == nil {
		runMessage := message
		if session.lastHistoryIsUser(message) {
			runMessage = ""
		}
		h.startRun(ctx, session, input.RequestID, runMessage, input.StepID, nil)
		return nil
	}
	if shouldSkipPlanning(message) {
		runMessage := message
		if session.lastHistoryIsUser(message) {
			runMessage = ""
		}
		h.startRun(ctx, session, input.RequestID, runMessage, input.StepID, nil)
		return nil
	}

	h.startPlanBuild(ctx, session, input.RequestID, message, input.StepID)
	return nil
}

func (h *wsHub) startPlanBuild(ctx context.Context, session *wsSession, requestID, message, stepID string) {
	// 在异步 Planner 返回前先记下本轮目标，便于中断后继续能回到同一条用户诉求重新出 plan。
	session.cancelPlanning()
	session.setPendingText(message)
	session.markPlanningRequest(requestID)
	planCtx, cancel := context.WithCancel(ctx)
	session.setPlanCancel(cancel)

	session.broadcast(wsOutput{
		Type:      wsEventPlanning,
		SessionID: session.id,
		RequestID: requestID,
		Data: gin.H{
			"phase": "prepare",
		},
	})

	go func() {
		defer session.setPlanCancel(nil)
		p, checkpointID, err := h.buildPendingPlan(planCtx, session, message)
		if err != nil {
			// Planning can be canceled by newer user input/interrupt; ignore canceled rounds.
			if !session.isCurrentPlanningRequest(requestID) {
				return
			}
			session.broadcast(wsOutput{
				Type:      wsEventError,
				SessionID: session.id,
				RequestID: requestID,
				Data:      err.Error(),
			})
			return
		}
		if !session.isCurrentPlanningRequest(requestID) {
			return
		}
		session.setPendingPlan(message, p)
		session.broadcast(wsOutput{Type: wsEventPlanPending, SessionID: session.id, RequestID: requestID, Data: gin.H{
			"plan":          p,
			"checkpoint_id": checkpointID,
		}})
	}()
}

func (h *wsHub) buildPendingPlan(ctx context.Context, session *wsSession, message string) (*agentplan.Plan, string, error) {
	p, err := h.planner.Plan(ctx, agentplan.Request{
		SessionID: session.id,
		UserInput: message,
		History:   session.historySnapshot(),
	})
	if err != nil {
		return nil, "", err
	}
	if h.checkpointer == nil {
		return p, "", nil
	}

	cp, err := h.checkpointer.Save(ctx, p, "ws_plan_created")
	if err != nil {
		return nil, "", err
	}
	return p, cp.ID, nil
}

func (h *wsHub) approvePlan(ctx context.Context, session *wsSession, input wsInput) error {
	message, plan := session.takePendingPlan()
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("no pending plan")
	}

	session.clearInterruptedRun()
	runMessage := message
	if session.lastHistoryIsUser(message) {
		runMessage = ""
	}
	h.startRun(ctx, session, input.RequestID, runMessage, input.StepID, plan)
	return nil
}

func (h *wsHub) resume(ctx context.Context, session *wsSession, input wsInput) error {
	resume := buildResumeAction(input)
	interruptID := session.takeInterruptID(input.InterruptID)
	if interruptID == "" {
		return fmt.Errorf("interrupt_id is required")
	}
	h.startResume(ctx, session, input.RequestID, resume, interruptID)
	return nil
}

func buildResumeAction(input wsInput) *humanResume {
	action := input.Type
	if action == wsInputAnswer {
		action = wsInputApproveTool
	}
	resume := &humanResume{
		Action:     action,
		ToolCallID: input.ToolCallID,
		Arguments:  strings.TrimSpace(string(input.Arguments)),
		Reason:     input.Reason,
	}
	if action == wsInputRejectTool && resume.Reason == "" {
		resume.Reason = "rejected by user"
	}
	return resume
}

func planStepUpdater(session *wsSession, requestID string, runVer uint64) agenttool.PlanStepUpdater {
	return func(ctx context.Context, input agenttool.PlanStepUpdate) (string, error) {
		status, ok := parseStepStatus(input.Status)
		if !ok {
			return "", fmt.Errorf("invalid step status: %s", input.Status)
		}

		if session == nil {
			return "", fmt.Errorf("no active websocket session")
		}

		if runVer != 0 && !session.isCurrentRunVer(runVer) {
			return "stale run ignored", nil
		}

		updated, err := session.updatePlanStep(input.StepID, status)
		if err != nil {
			return "", err
		}
		if updated == nil {
			return "", fmt.Errorf("plan step not found or no active plan: %s", input.StepID)
		}
		if strings.TrimSpace(requestID) != "" {
			session.broadcast(wsOutput{Type: wsEventPlanUpdated, SessionID: session.id, RequestID: requestID, Data: gin.H{
				"plan": updated,
				"step_update": gin.H{
					"step_id": input.StepID,
					"status":  string(status),
					"note":    input.Note,
				},
			}})
		}
		return "plan step updated", nil
	}
}

func (h *wsHub) setPlanStepStatus(ctx context.Context, session *wsSession, requestID string, runVer uint64, stepID string, status agentplan.StepStatus, note string) (*agentplan.Plan, error) {
	if session == nil {
		return nil, fmt.Errorf("no active websocket session")
	}
	if runVer != 0 && !session.isCurrentRunVer(runVer) {
		return nil, nil
	}
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return nil, nil
	}
	updated, err := session.updatePlanStep(stepID, status)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, fmt.Errorf("plan step not found or no active plan: %s", stepID)
	}
	if h.checkpointer != nil {
		_, _ = h.checkpointer.Save(ctx, updated, "ws_step_"+string(status))
	}
	if strings.TrimSpace(requestID) != "" {
		session.broadcast(wsOutput{Type: wsEventPlanUpdated, SessionID: session.id, RequestID: requestID, Data: gin.H{
			"plan": updated,
			"step_update": gin.H{
				"step_id": stepID,
				"status":  string(status),
				"note":    note,
			},
		}})
	}
	return updated, nil
}
