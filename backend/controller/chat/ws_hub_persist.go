package chat

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
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

// applyResumeSnapshotToSession 将 Mongo 中的中断快照写回会话（与 hydrate / startResume 共用）。
func applyResumeSnapshotToSession(session *wsSession, snap *memory.WSResumeSnapshot) {
	if session == nil || snap == nil || strings.TrimSpace(snap.InterruptID) == "" {
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
		expertID:  strings.TrimSpace(snap.ActiveExpertID),
	})
	session.ensureExpertScopeForResume(snap.ActiveExpertID)
}

// hydrateWSPersist 只恢复 history / 中断元数据；右侧专家预览由前端通过 GET /chat/ws/sessions/:id/preview 主动拉取。
func (h *wsHub) hydrateWSPersist(ctx context.Context, session *wsSession) {
	if h.mem == nil {
		return
	}
	if history, err := h.mem.LoadWSHistory(ctx, session.id); err == nil && len(history) > 0 {
		session.replaceHistory(history)
	}
	snap, err := h.mem.LoadWSResume(ctx, session.id)
	if err == nil && snap != nil && snap.InterruptID != "" {
		applyResumeSnapshotToSession(session, snap)
		kind := strings.TrimSpace(snap.InterruptKind)
		if kind == wsEventInputRequired || kind == wsEventToolApprovalRequired {
			session.setPendingInterruptID(snap.InterruptID)
		}
	}
}

func (h *wsHub) persistWSResumeAfterPause(ctx context.Context, session *wsSession, requestID string, info *compose.InterruptInfo, activeRun wsInterruptedState) {
	if h.mem == nil {
		return
	}
	interruptID := strings.TrimSpace(session.pendingInterruptIDSnapshot())
	if interruptID == "" && info != nil && len(info.InterruptContexts) > 0 {
		interruptID = strings.TrimSpace(info.InterruptContexts[0].ID)
	}
	if interruptID == "" {
		return
	}

	planJSON, _ := json.Marshal(activeRun.plan)
	_ = h.mem.SaveWSResume(ctx, session.id, &memory.WSResumeSnapshot{
		InterruptID:    interruptID,
		Message:        activeRun.message,
		StepID:         activeRun.stepID,
		RequestID:      requestID,
		PlanJSON:       planJSON,
		ActiveExpertID: strings.TrimSpace(activeRun.expertID),
		InterruptKind:  interruptKindFromInfo(info),
	})
}

func (h *wsHub) persistWSHistory(ctx context.Context, session *wsSession) {
	if h.mem == nil {
		return
	}
	_ = h.mem.SaveWSHistory(ctx, session.id, session.historySnapshot())
}

func (h *wsHub) appendWSHistory(ctx context.Context, session *wsSession, messages ...*schema.Message) {
	if h.mem == nil || len(messages) == 0 {
		return
	}
	_ = h.mem.AppendWSHistory(ctx, session.id, messages...)
}
