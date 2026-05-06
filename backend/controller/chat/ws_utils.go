package chat

import (
	"context"
	"strings"
	"sync"
	"time"

	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	agenttool "github.com/agent-pilot/agent-pilot-be/agent/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/net/websocket"
)

func (s *wsSession) add(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[conn] = struct{}{}
}

func (s *wsSession) remove(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, conn)
}

func (s *wsSession) send(conn *websocket.Conn, output wsOutput) {
	_ = websocket.JSON.Send(conn, output)
}

func (s *wsSession) broadcast(output wsOutput) {
	s.mu.Lock()
	clients := make([]*websocket.Conn, 0, len(s.clients))
	for conn := range s.clients {
		clients = append(clients, conn)
	}
	s.mu.Unlock()

	for _, conn := range clients {
		_ = websocket.JSON.Send(conn, output)
	}
}

// EnterExpertMode 实现 expert.ExpertSession：进入专家模式并启动与主对话隔离的专属消息线程。
func (s *wsSession) EnterExpertMode(expertID, taskBrief string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prevExpert := strings.TrimSpace(s.activeExpertID)
	s.activeExpertID = strings.TrimSpace(expertID)
	brief := strings.TrimSpace(taskBrief)
	if brief == "" {
		brief = "（主路由未提供 task_brief；请根据你的领域职责用一两句话向用户确认需求后再执行。）"
	}
	s.expertBranch = []*schema.Message{
		schema.UserMessage("[专家专属线程 — 不依赖主助手阶段其它轮次；仅根据下列任务与后续用户消息工作。]\n\n" + brief),
	}
	s.expertOmitNextUserInBranch = true
	// 仅「从主会话首次切入专家」时需要重写主 compose 内 state；专家 A→专家 B 由 pickComposeRuntime 换独立图，不在主图里 rewire。
	s.expertRewireCompose = prevExpert == ""
}

// ExitExpertMode 实现 expert.ExpertSession：退出专家模式。
func (s *wsSession) ExitExpertMode() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearExpertLocked()
}

func (s *wsSession) clearExpertLocked() {
	s.activeExpertID = ""
	s.expertBranch = nil
	s.expertOmitNextUserInBranch = false
	s.expertRewireCompose = false
}

func (s *wsSession) consumeExpertRewireCompose() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.expertRewireCompose {
		return false
	}
	s.expertRewireCompose = false
	return true
}

// releaseExpertBranchUserGate 清除「下一条 User 不写入专家线程」门闩。
// 若切入专家后首轮尚未消费该门闩就发生中断，用户对 request_user_input 的回复会误被跳过，
// expertBranch 缺用户答案，compose 恢复时模型看不到刚回复的内容。
func (s *wsSession) releaseExpertBranchUserGate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expertOmitNextUserInBranch = false
}

func (s *wsSession) activeExpertIDSnapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeExpertID
}

func (s *wsSession) appendHistory(messages ...*schema.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, messages...)
	if strings.TrimSpace(s.activeExpertID) == "" {
		return
	}
	for _, m := range messages {
		if m == nil {
			continue
		}
		switch m.Role {
		case schema.User:
			if s.expertOmitNextUserInBranch {
				s.expertOmitNextUserInBranch = false
				continue
			}
			cp := *m
			cp.ToolCalls = cloneToolCalls(m.ToolCalls)
			s.expertBranch = append(s.expertBranch, &cp)
		case schema.Assistant:
			cp := *m
			cp.ToolCalls = cloneToolCalls(m.ToolCalls)
			s.expertBranch = append(s.expertBranch, &cp)
		}
	}
}

func (s *wsSession) historySnapshot() []*schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneMessages(s.history)
}

func (s *wsSession) lastHistoryIsUser(content string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) == 0 {
		return false
	}
	last := s.history[len(s.history)-1]
	return last != nil &&
		last.Role == schema.User &&
		strings.TrimSpace(last.Content) == strings.TrimSpace(content)
}

// composeHistorySnapshot 供 compose 入参使用：专家模式下仅返回专家线程，不含主 Agent 历史。
func (s *wsSession) composeHistorySnapshot() []*schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.activeExpertID) != "" && len(s.expertBranch) > 0 {
		return cloneMessages(s.expertBranch)
	}
	return cloneMessages(s.history)
}

func (s *wsSession) expertBranchSnapshot() []*schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneMessages(s.expertBranch)
}

func (s *wsSession) replaceHistory(messages []*schema.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = cloneMessages(messages)
}

func (s *wsSession) resetPendingPlan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending.plan = nil
	s.pending.text = ""
}

func (s *wsSession) setPendingText(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending.text = text
}

func (s *wsSession) setPendingPlan(text string, plan *agentplan.Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending.text = text
	s.pending.plan = plan
}

// takingPendingPlan 将pending plan升格为正式执行的plan并返回相关信息
func (s *wsSession) takePendingPlan() (string, *agentplan.Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	message := s.pending.text
	plan := s.pending.plan
	s.pending.text = ""
	s.pending.plan = nil
	return message, plan
}

func (s *wsSession) setPendingInterruptID(interruptID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending.interruptID = interruptID
}

func (s *wsSession) takePendingInterruptID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	interruptID := s.pending.interruptID
	s.pending.interruptID = ""
	return interruptID
}

func (s *wsSession) takeInterruptID(inputInterruptID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	interruptID := firstNonEmpty(inputInterruptID, s.pending.interruptID)
	s.pending.interruptID = ""
	return interruptID
}

func (s *wsSession) pendingInterruptIDSnapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending.interruptID
}

// beginRun 检查是否可以run并为run进行一系列的状态设置
func (s *wsSession) beginRun(message, stepID, requestID string, plan *agentplan.Plan) (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 检查是否有plan在执行
	if s.run.active {
		return 0, false
	}
	// 更改当前run version
	s.runVer++
	if plan != nil {
		plan.Status = agentplan.StatusExecuting
		plan.UpdatedAt = time.Now()
		s.run.plan = plan
	}
	s.run.message = message
	s.run.stepID = strings.TrimSpace(stepID)
	s.run.requestID = requestID
	s.run.active = true
	return s.runVer, true
}

func (s *wsSession) attachCancelAndSnapshot(cancel func(opts ...compose.GraphInterruptOption)) []*schema.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run.cancel = cancel
	s.run.active = true
	if strings.TrimSpace(s.activeExpertID) != "" && len(s.expertBranch) > 0 {
		return cloneMessages(s.expertBranch)
	}
	return cloneMessages(s.history)
}

func (s *wsSession) finishRun(runVer uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runVer != runVer {
		return false
	}
	s.run.cancel = nil
	s.run.active = false
	s.run.plan = nil
	s.run.message = ""
	s.run.stepID = ""
	s.run.requestID = ""
	return true
}

func (s *wsSession) cancel() func(opts ...compose.GraphInterruptOption) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run.cancel
}

func (s *wsSession) invalidateRun() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runVer++
	s.run.cancel = nil
	s.run.active = false
	s.run.plan = nil
	s.run.message = ""
	s.run.stepID = ""
	s.run.requestID = ""
	s.interrupted = wsInterruptedState{}
}

func (s *wsSession) isCurrentRunVer(ver uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runVer == ver
}

func (s *wsSession) interruptedMessage() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interrupted.message
}

func (s *wsSession) interruptedRunSnapshot() wsInterruptedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return wsInterruptedState{
		message:   s.interrupted.message,
		plan:      clonePlanForView(s.interrupted.plan),
		stepID:    s.interrupted.stepID,
		requestID: s.interrupted.requestID,
		expertID:  s.interrupted.expertID,
	}
}

func (s *wsSession) snapshotActiveRun() wsInterruptedState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return wsInterruptedState{
		message:   s.run.message,
		plan:      clonePlanForView(s.run.plan),
		stepID:    s.run.stepID,
		requestID: s.run.requestID,
		expertID:  strings.TrimSpace(s.activeExpertID),
	}
}

func (s *wsSession) setInterruptedRun(state wsInterruptedState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interrupted = wsInterruptedState{
		message:   state.message,
		plan:      clonePlanForView(state.plan),
		stepID:    state.stepID,
		requestID: state.requestID,
		expertID:  strings.TrimSpace(state.expertID),
	}
}

// ensureExpertScopeForResume 在 compose 恢复前挂上 expert_id，使 pickComposeRuntime / composeCheckpointID 与中断时一致（文档专家等为独立检查点命名空间）。
func (s *wsSession) ensureExpertScopeForResume(expertID string) {
	eid := strings.TrimSpace(expertID)
	if eid == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeExpertID = eid
}

func (s *wsSession) clearInterruptedRun() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interrupted = wsInterruptedState{}
}

func (s *wsSession) setPendingResumeFallback(r *humanResume) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingResumeFallback = r
}

func (s *wsSession) clearPendingResumeFallback() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingResumeFallback = nil
}

func (s *wsSession) peekPendingResumeFallback() *humanResume {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingResumeFallback
}

func (s *wsSession) consumePendingResumeFallbackOnce() *humanResume {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.pendingResumeFallback
	s.pendingResumeFallback = nil
	return r
}

func (s *wsSession) activePlanSnapshot() *agentplan.Plan {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePlanForView(s.run.plan)
}

func (s *wsSession) activeStepIDSnapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run.plan == nil || len(s.run.plan.Steps) == 0 {
		return ""
	}
	// Prefer the currently running step; otherwise pick the first pending one.
	for _, step := range s.run.plan.Steps {
		if step.Status == agentplan.StepStatusRunning && strings.TrimSpace(step.ID) != "" {
			return step.ID
		}
	}
	for _, step := range s.run.plan.Steps {
		if step.Status == agentplan.StepStatusPending && strings.TrimSpace(step.ID) != "" {
			return step.ID
		}
	}
	return ""
}

func (s *wsSession) isActiveStepRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run.plan == nil || len(s.run.plan.Steps) == 0 {
		return false
	}
	for _, step := range s.run.plan.Steps {
		if step.Status == agentplan.StepStatusRunning {
			return true
		}
	}
	return false
}

func (s *wsSession) nextPendingStepIDSnapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run.plan == nil || len(s.run.plan.Steps) == 0 {
		return ""
	}
	for _, step := range s.run.plan.Steps {
		if step.Status == agentplan.StepStatusPending && strings.TrimSpace(step.ID) != "" {
			return step.ID
		}
	}
	return ""
}

func (s *wsSession) updatePlanStep(stepID string, status agentplan.StepStatus) (*agentplan.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run.plan == nil {
		return nil, nil
	}

	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return nil, nil
	}

	for i := range s.run.plan.Steps {
		step := &s.run.plan.Steps[i]
		if step.ID != stepID {
			continue
		}
		if !validStepTransition(step.Status, status) {
			return clonePlanForView(s.run.plan), nil
		}
		step.Status = status
		s.run.plan.UpdatedAt = time.Now()
		s.updatePlanStatusLocked()
		return clonePlanForView(s.run.plan), nil
	}
	return nil, nil
}

func (s *wsSession) planStepStatusSnapshot(stepID string) (agentplan.StepStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run.plan == nil {
		return "", false
	}
	stepID = strings.TrimSpace(stepID)
	for _, step := range s.run.plan.Steps {
		if step.ID == stepID {
			return step.Status, true
		}
	}
	return "", false
}

func validStepTransition(from, to agentplan.StepStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case "":
		return true
	case agentplan.StepStatusPending:
		return to == agentplan.StepStatusRunning ||
			to == agentplan.StepStatusCompleted ||
			to == agentplan.StepStatusFailed ||
			to == agentplan.StepStatusSkipped
	case agentplan.StepStatusRunning:
		return to == agentplan.StepStatusCompleted ||
			to == agentplan.StepStatusFailed ||
			to == agentplan.StepStatusSkipped
	case agentplan.StepStatusCompleted, agentplan.StepStatusFailed, agentplan.StepStatusSkipped:
		return false
	default:
		return true
	}
}

func (s *wsSession) updatePlanStatusLocked() {
	if s.run.plan == nil {
		return
	}
	if len(s.run.plan.Steps) == 0 {
		return
	}
	hasOpen := false
	for _, step := range s.run.plan.Steps {
		switch step.Status {
		case agentplan.StepStatusFailed:
			s.run.plan.Status = agentplan.StatusFailed
			return
		case agentplan.StepStatusPending, agentplan.StepStatusRunning:
			hasOpen = true
		}
	}
	if hasOpen {
		s.run.plan.Status = agentplan.StatusExecuting
		return
	}
	s.run.plan.Status = agentplan.StatusCompleted
}

func parseStepStatus(status string) (agentplan.StepStatus, bool) {
	switch agentplan.StepStatus(strings.ToLower(strings.TrimSpace(status))) {
	case agentplan.StepStatusPending:
		return agentplan.StepStatusPending, true
	case agentplan.StepStatusRunning:
		return agentplan.StepStatusRunning, true
	case agentplan.StepStatusCompleted:
		return agentplan.StepStatusCompleted, true
	case agentplan.StepStatusFailed:
		return agentplan.StepStatusFailed, true
	case agentplan.StepStatusSkipped:
		return agentplan.StepStatusSkipped, true
	default:
		return "", false
	}
}

func clonePlanForView(in *agentplan.Plan) *agentplan.Plan {
	if in == nil {
		return nil
	}
	out := *in
	out.Assumptions = append([]string(nil), in.Assumptions...)
	out.Constraints = append([]string(nil), in.Constraints...)
	out.SubjectiveState.Preferences = append([]string(nil), in.SubjectiveState.Preferences...)
	out.SubjectiveState.RiskAwareness = append([]string(nil), in.SubjectiveState.RiskAwareness...)
	out.SubjectiveState.ClarifyingNeeds = append([]string(nil), in.SubjectiveState.ClarifyingNeeds...)
	out.Steps = make([]agentplan.Step, len(in.Steps))
	for i, step := range in.Steps {
		out.Steps[i] = step
		out.Steps[i].Dependencies = append([]string(nil), step.Dependencies...)
		if step.Inputs != nil {
			out.Steps[i].Inputs = make(map[string]string, len(step.Inputs))
			for k, v := range step.Inputs {
				out.Steps[i].Inputs[k] = v
			}
		}
	}
	return &out
}

func (s *wsSession) resetPendingInterrupt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending.interruptID = ""
}

func (s *wsSession) hasPendingInterrupt() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(s.pending.interruptID) != ""
}

func (s *wsSession) hasPendingPlan() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(s.pending.text) != "" && s.pending.plan != nil
}

// peekInterruptedPlanBuildGoal 已记录本轮「待生成 plan」的用户目标，但 plan 尚未落地（含规划中被中断）。
// 与 shouldSkipPlanning 直跑路径区分：后者不再写入 pending.text。
func (s *wsSession) peekInterruptedPlanBuildGoal() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.pending.text) == "" || s.pending.plan != nil {
		return "", false
	}
	return s.pending.text, true
}

func (s *wsSession) setPlanCancel(cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planCancel = cancel
}

func (s *wsSession) cancelPlanning() {
	s.mu.Lock()
	cancel := s.planCancel
	s.planCancel = nil
	s.planningReq = ""
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *wsSession) markPlanningRequest(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planningReq = strings.TrimSpace(requestID)
}

func (s *wsSession) isCurrentPlanningRequest(requestID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(requestID) != "" && s.planningReq == strings.TrimSpace(requestID)
}

func getHumanResume(ctx context.Context) (*humanResume, bool) {
	_, hasData, resume := compose.GetResumeContext[*humanResume](ctx)
	if hasData && resume != nil {
		return resume, true
	}
	return nil, false
}

func applyResumeToToolCalls(calls []schema.ToolCall, resume *humanResume) {
	if resume == nil || strings.TrimSpace(resume.Arguments) == "" {
		return
	}
	for i := range calls {
		if matchesResumeCall(resume, calls[i]) {
			calls[i].Function.Arguments = resume.Arguments
		}
	}
}

func matchesResumeCall(resume *humanResume, call schema.ToolCall) bool {
	return resume.ToolCallID == "" || resume.ToolCallID == call.ID
}

func isDangerousToolCall(call schema.ToolCall) bool {
	name := strings.ToLower(call.Function.Name)
	if name == strings.ToLower(agenttool.PlanStepToolName) {
		return false
	}
	args := strings.ToLower(call.Function.Arguments)
	if name == "shell" {
		dangerousTokens := []string{" rm ", "del ", "remove-item", "git reset", "drop table", "shutdown", "format ", "mkfs", "chmod -r", "chown -r"}
		for _, token := range dangerousTokens {
			if strings.Contains(" "+args+" ", token) {
				return true
			}
		}
		return true
	}
	return strings.Contains(name, "delete") || strings.Contains(name, "remove") || strings.Contains(name, "send") || strings.Contains(name, "update")
}

func viewToolCall(call schema.ToolCall, risk string) toolCallView {
	return toolCallView{
		ID:        call.ID,
		Name:      call.Function.Name,
		Arguments: call.Function.Arguments,
		Risk:      risk,
	}
}

func withSystemMessage(system string, history []*schema.Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(history)+1)
	if strings.TrimSpace(system) != "" {
		out = append(out, schema.SystemMessage(system))
	}
	out = append(out, cloneMessages(history)...)
	return out
}

func cloneMessages(messages []*schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		cp := *msg
		cp.ToolCalls = cloneToolCalls(msg.ToolCalls)
		out = append(out, &cp)
	}
	return out
}

func cloneToolCalls(calls []schema.ToolCall) []schema.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]schema.ToolCall, len(calls))
	copy(out, calls)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type memoryStore struct {
	mu sync.RWMutex
	m  map[string][]byte
}

func newMemoryStore() *memoryStore {
	return &memoryStore{m: make(map[string][]byte)}
}

func (s *memoryStore) Get(ctx context.Context, id string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.m[id]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), data...), true, nil
}

func (s *memoryStore) Set(ctx context.Context, id string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = append([]byte(nil), data...)
	return nil
}
