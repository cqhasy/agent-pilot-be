package chat

import (
	"context"
	"encoding/json"

	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	"github.com/cloudwego/eino/compose"
)

func interruptKindFromInfo(info *compose.InterruptInfo) string {
	if info == nil || len(info.InterruptContexts) == 0 {
		return ""
	}
	if pause, ok := info.InterruptContexts[0].Info.(*humanPause); ok {
		return pause.Kind
	}
	return wsEventInterrupted
}

func (h *wsHub) hydrateWSPersist(ctx context.Context, session *wsSession) {
	if h.mem == nil {
		return
	}
	if history, err := h.mem.LoadWSHistory(ctx, session.id); err == nil && len(history) > 0 {
		session.replaceHistory(history)
	}
	snap, err := h.mem.LoadWSResume(ctx, session.id)
	if err != nil || snap == nil || snap.InterruptID == "" {
		return
	}

	var plan *agentplan.Plan
	if len(snap.PlanJSON) > 0 {
		var p agentplan.Plan
		if err := json.Unmarshal(snap.PlanJSON, &p); err == nil {
			plan = clonePlanForView(&p)
		}
	}

	session.setInterruptedRun(wsInterruptedState{
		message:   snap.Message,
		plan:      plan,
		stepID:    snap.StepID,
		requestID: snap.RequestID,
	})
	session.setPendingInterruptID(snap.InterruptID)
}

func (h *wsHub) persistWSResumeAfterPause(ctx context.Context, session *wsSession, requestID string, info *compose.InterruptInfo, activeRun wsInterruptedState) {
	if h.mem == nil {
		return
	}
	interruptID := session.pendingInterruptIDSnapshot()
	if interruptID == "" {
		return
	}

	planJSON, _ := json.Marshal(activeRun.plan)
	_ = h.mem.SaveWSResume(ctx, session.id, &memory.WSResumeSnapshot{
		InterruptID:   interruptID,
		Message:       activeRun.message,
		StepID:        activeRun.stepID,
		RequestID:     requestID,
		PlanJSON:      planJSON,
		InterruptKind: interruptKindFromInfo(info),
	})
}

func (h *wsHub) persistWSHistory(ctx context.Context, session *wsSession) {
	if h.mem == nil {
		return
	}
	_ = h.mem.SaveWSHistory(ctx, session.id, session.historySnapshot())
}
