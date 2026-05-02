package chat

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/net/websocket"
)

func newWSHub(rt *composeRuntime, planner agentplan.Planner, checkpointer agentplan.Checkpointer, mem memory.MemoryService) *wsHub {
	h := &wsHub{
		runtime:      rt,
		planner:      planner,
		checkpointer: checkpointer,
		mem:          mem,
		sessions:     make(map[string]*wsSession),
		handlers:     make(map[string]wsMessageHandler),
	}
	h.registerDefaultHandlers()
	return h
}

func (h *wsHub) serve(ctx *gin.Context) {
	websocket.Handler(func(conn *websocket.Conn) {
		sessionID := ctx.Query("session_id")
		if sessionID == "" {
			sessionID = uuid.NewString()
		}

		// 从缓存或者mongodb中拿session的history,plan和执行状态
		session := h.getSession(ctx.Request.Context(), sessionID)
		session.add(conn)
		defer session.remove(conn)

		session.send(conn, wsOutput{Type: wsEventReady, SessionID: sessionID, Data: gin.H{
			"session_id": sessionID,
			"protocol":   "agent-pilot-chat-ws-v1",
		}})

		for {
			var input wsInput
			if err := websocket.JSON.Receive(conn, &input); err != nil {
				if !errors.Is(err, io.EOF) {
					session.broadcast(wsOutput{Type: wsEventError, SessionID: sessionID, Data: err.Error()})
				}
				return
			}
			if input.SessionID == "" {
				input.SessionID = sessionID
			}
			h.handle(ctx.Request.Context(), session, input)
		}
	}).ServeHTTP(ctx.Writer, ctx.Request)
}

func (h *wsHub) getSession(ctx context.Context, id string) *wsSession {
	h.mu.Lock()
	if s := h.sessions[id]; s != nil {
		h.mu.Unlock()
		return s
	}
	s := &wsSession{id: id, clients: make(map[*websocket.Conn]struct{})}
	h.sessions[id] = s
	h.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}

	h.hydrateWSPersist(ctx, s)
	return s
}

func (h *wsHub) handle(ctx context.Context, session *wsSession, input wsInput) {
	// 根据input事件的类型调用不同的处理函数，非法状态报错给前端
	handler, ok := h.lookupHandler(input.Type)
	if !ok {
		session.broadcast(wsOutput{Type: wsEventError, SessionID: session.id, RequestID: input.RequestID, Data: "unknown message type: " + input.Type})
		return
	}

	if err := handler(ctx, session, input); err != nil {
		session.broadcast(wsOutput{
			Type:      wsEventError,
			SessionID: session.id,
			RequestID: input.RequestID,
			Data:      err.Error(),
		})
	}
}

func (h *wsHub) registerDefaultHandlers() {
	// 针对用户主动输入，plan的批准拒绝，工具调用的批准拒绝与参数给予(给予这个词感觉不好但是想不到别的)，中断请求注册了五个处理函数
	h.registerHandler(h.handleUserMessage, wsInputUserMessage)
	h.registerHandler(h.handleApprovePlan, wsInputApprovePlan)
	h.registerHandler(h.handleRejectPlan, wsInputRejectPlan)
	h.registerHandler(h.handleResumeTool, wsInputApproveTool, wsInputRejectTool, wsInputAnswer)
	h.registerHandler(h.handleInterrupt, wsInputInterrupt)
}

func (h *wsHub) registerHandler(handler wsMessageHandler, types ...string) {
	for _, inputType := range types {
		h.handlers[inputType] = handler
	}
}

func (h *wsHub) lookupHandler(inputType string) (wsMessageHandler, bool) {
	handler, ok := h.handlers[inputType]
	return handler, ok
}

func (h *wsHub) handleUserMessage(ctx context.Context, session *wsSession, input wsInput) error {
	// 需要明确两种情况，当接收用户消息时，系统可能处于用户主动中断状态，上一个任务处理完毕状态，新session刚刚接收信息状态，工具调用需要用户输参状态
	// 对于工具请求输参直接恢复并继续执行
	if interruptID := session.takePendingInterruptID(); strings.TrimSpace(interruptID) != "" {
		userReply := strings.TrimSpace(input.Message)
		if userReply != "" {
			session.appendHistory(schema.UserMessage(userReply))
			h.persistWSHistory(ctx, session)
		}

		resume := &humanResume{Action: "continue", Reason: userReply}
		h.startResume(ctx, session, input.RequestID, resume, interruptID)
		return nil
	}

	// 用户是否是继续意愿
	if isContinueMessage(input.Message) {
		// 检查是否已经生成了但没请求同意的plan
		if session.hasPendingPlan() {
			return h.approvePlan(ctx, session, input)
		}

		interrupted := session.interruptedRunSnapshot()
		if interrupted.plan != nil || strings.TrimSpace(interrupted.message) != "" {
			resumeMsg := nextStepPrompt(interrupted.plan, interrupted.stepID)
			if strings.TrimSpace(resumeMsg) == "" {
				resumeMsg = strings.TrimSpace(interrupted.message)
			}
			h.startRun(ctx, session, input.RequestID, resumeMsg, interrupted.stepID, interrupted.plan)
			return nil
		}
	}

	// 如果不满足继续执行的条件，就当成一个全新的请求处理，清空plan和interrupt状态
	session.cancelPlanning()
	session.resetPendingInterrupt()
	session.resetPendingPlan()
	return h.preparePlan(ctx, session, input)
}

func (h *wsHub) handleApprovePlan(ctx context.Context, session *wsSession, input wsInput) error {
	return h.approvePlan(ctx, session, input)
}

func (h *wsHub) handleRejectPlan(_ context.Context, session *wsSession, input wsInput) error {
	session.resetPendingPlan()
	session.broadcast(wsOutput{Type: wsEventPlanRejected, SessionID: session.id, RequestID: input.RequestID, Data: input.Reason})
	return nil
}

func (h *wsHub) handleResumeTool(ctx context.Context, session *wsSession, input wsInput) error {
	return h.resume(ctx, session, input)
}

func (h *wsHub) handleInterrupt(_ context.Context, session *wsSession, _ wsInput) error {
	session.cancelPlanning()
	if cancel := session.cancel(); cancel != nil {
		cancel(compose.WithGraphInterruptTimeout(2 * time.Second))
	}
	return nil
}
