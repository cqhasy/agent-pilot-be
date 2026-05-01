package chat

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/net/websocket"
)

func newWSHub(rt *composeRuntime, planner agentplan.Planner, checkpointer agentplan.Checkpointer) *wsHub {
	h := &wsHub{
		runtime:      rt,
		planner:      planner,
		checkpointer: checkpointer,
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
		session := h.getSession(sessionID)
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

func (h *wsHub) getSession(id string) *wsSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s := h.sessions[id]; s != nil {
		return s
	}
	s := &wsSession{id: id, clients: make(map[*websocket.Conn]struct{})}
	h.sessions[id] = s
	return s
}

func (h *wsHub) handle(ctx context.Context, session *wsSession, input wsInput) {
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
	// If runtime is waiting on a graph interrupt, any user message should resume that run
	// instead of starting a brand new plan/run.
	if interruptID := session.takePendingInterruptID(); strings.TrimSpace(interruptID) != "" {
		userReply := strings.TrimSpace(input.Message)
		if userReply != "" {
			// Preserve user-provided parameters in chat history so resumed model turns can consume them.
			session.appendHistory(schema.UserMessage(userReply))
		}
		resume := &humanResume{Action: "continue", Reason: userReply}
		h.startResume(ctx, session, input.RequestID, resume, interruptID)
		return nil
	}

	// 用户是否是继续意愿
	if isContinueMessage(input.Message) {
		// 检查是否已经生成了plan
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
