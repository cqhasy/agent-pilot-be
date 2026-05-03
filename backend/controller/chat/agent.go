package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/agent-pilot/agent-pilot-be/agent/memory"
	agentplan "github.com/agent-pilot/agent-pilot-be/agent/plan"
	"github.com/agent-pilot/agent-pilot-be/agent/react"
	"github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"

	"github.com/agent-pilot/agent-pilot-be/agent/tool/skill"
	pkgmodel "github.com/agent-pilot/agent-pilot-be/model"
)

type ControllerInterface interface {
	Chat(ctx *gin.Context)
	Plan(ctx *gin.Context)
	Execute(ctx *gin.Context)
	ChatWS(ctx *gin.Context)
}

type Controller struct {
	Mem          map[string]memory.Memory
	Agent        adk.Agent
	SkillReg     *skill.Registry
	SystemMsg    string
	Runner       *adk.Runner
	Planner      agentplan.Planner
	Checkpointer agentplan.Checkpointer
	Executor     *react.Executor
	mu           sync.Mutex
}

func NewController(
	ctx context.Context,
	agent adk.Agent,
	skillReg *skill.Registry,
	systemMsg string,
	planner agentplan.Planner,
	checkpointer agentplan.Checkpointer,
	executor *react.Executor,
) *Controller {
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
	})
	return &Controller{
		Agent:        agent,
		SkillReg:     skillReg,
		SystemMsg:    systemMsg,
		Runner:       runner,
		Planner:      planner,
		Checkpointer: checkpointer,
		Executor:     executor,
	}
}

// Chat 处理流式聊天请求
func (c *Controller) Chat(ctx *gin.Context) {
	var req request
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
			Code:    400,
			Message: err.Error(),
			Data:    nil,
		})
		return
	}

	if req.Message == "" {
		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
			Code:    400,
			Message: "Message is required",
			Data:    nil,
		})
		return
	}

	// 设置 SSE headers
	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
	ctx.Writer.Header().Set("Cache-Control", "no-cache")
	ctx.Writer.Header().Set("Connection", "keep-alive")
	ctx.Writer.Flush()
	// 构建消息历史
	history := c.getHistory("mock")

	// 加入用户输入
	history = append(history, schema.UserMessage(req.Message))

	// 调模型
	events := c.Runner.Run(ctx.Request.Context(), history)
	c.streamFromEvents(ctx, events, "mock", history)
}

func (c *Controller) Plan(ctx *gin.Context) {
	var req request
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
			Code:    400,
			Message: err.Error(),
			Data:    nil,
		})
		return
	}

	if req.Message == "" {
		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
			Code:    400,
			Message: "Message is required",
			Data:    nil,
		})
		return
	}
	if c.Planner == nil {
		ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
			Code:    500,
			Message: "planner is not configured",
			Data:    nil,
		})
		return
	}

	sessionID := "mock"
	history := c.getHistory(sessionID)
	p, err := c.Planner.Plan(ctx.Request.Context(), agentplan.Request{
		SessionID: sessionID,
		UserInput: req.Message,
		History:   history,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
			Code:    500,
			Message: err.Error(),
			Data:    nil,
		})
		return
	}

	var checkpointID string
	if c.Checkpointer != nil {
		cp, err := c.Checkpointer.Save(ctx.Request.Context(), p, "plan_created")
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
				Code:    500,
				Message: err.Error(),
				Data:    nil,
			})
			return
		}
		checkpointID = cp.ID
	}

	ctx.JSON(http.StatusOK, pkgmodel.Response{
		Code:    0,
		Message: "ok",
		Data: gin.H{
			"plan":          p,
			"checkpoint_id": checkpointID,
		},
	})
}

func (c *Controller) Execute(ctx *gin.Context) {
	var req request
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
			Code:    400,
			Message: err.Error(),
			Data:    nil,
		})
		return
	}

	if c.Executor == nil {
		ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
			Code:    500,
			Message: "react executor is not configured",
			Data:    nil,
		})
		return
	}

	p, err := c.planForExecution(ctx, req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, pkgmodel.Response{
			Code:    400,
			Message: err.Error(),
			Data:    nil,
		})
		return
	}

	result, err := c.Executor.Execute(ctx.Request.Context(), p)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, pkgmodel.Response{
			Code:    500,
			Message: err.Error(),
			Data:    result,
		})
		return
	}

	ctx.JSON(http.StatusOK, pkgmodel.Response{
		Code:    0,
		Message: "ok",
		Data:    result,
	})
}

func (c *Controller) planForExecution(ctx *gin.Context, req request) (*agentplan.Plan, error) {
	if req.CheckpointID != "" {
		if c.Checkpointer == nil {
			return nil, fmt.Errorf("checkpointer is not configured")
		}
		cp, err := c.Checkpointer.Load(ctx.Request.Context(), req.CheckpointID)
		if err != nil {
			return nil, err
		}
		return cp.Plan, nil
	}

	if strings.TrimSpace(req.Message) == "" {
		return nil, fmt.Errorf("message or checkpoint_id is required")
	}
	if c.Planner == nil {
		return nil, fmt.Errorf("planner is not configured")
	}

	sessionID := "mock"
	return c.Planner.Plan(ctx.Request.Context(), agentplan.Request{
		SessionID: sessionID,
		UserInput: req.Message,
		History:   c.getHistory(sessionID),
	})
}

// streamFromEvents 从事件流中提取内容并发送给客户端
func (c *Controller) streamFromEvents(
	ginCtx *gin.Context,
	events *adk.AsyncIterator[*adk.AgentEvent],
	sessionID string,
	history []*schema.Message,
) {
	var fullReply strings.Builder
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			c.sendEventGin(ginCtx, "error", fmt.Sprintf("Stream error: %v", event.Err))
			return
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}

		mv := event.Output.MessageOutput
		//if mv.Role != schema.Assistant {
		//	continue
		//}

		// 处理流式内容
		if mv.IsStreaming {
			mv.MessageStream.SetAutomaticClose()
			for {
				frame, err := mv.MessageStream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					c.sendEventGin(ginCtx, "error", fmt.Sprintf("Stream recv error: %v", err))
					return
				}

				if frame != nil && frame.Content != "" {
					fullReply.WriteString(frame.Content)
					c.sendEventGin(ginCtx, "message", frame.Content)
				}
			}
			continue
		}

		// 非流式内容
		if mv.Message != nil && mv.Message.Content != "" {
			fullReply.WriteString(mv.Message.Content)
			c.sendEventGin(ginCtx, "message", mv.Message.Content)
		}
	}

	if fullReply.Len() > 0 {
		history = append(history, schema.AssistantMessage(fullReply.String(), nil))
	}

	c.saveHistory(sessionID, history)
	fmt.Printf("%+v", history)
	// done
	c.sendEventGin(ginCtx, "done", "")
}

// sendEventGin 发送 SSE 事件
func (c *Controller) sendEventGin(ctx *gin.Context, event, data string) {
	fmt.Fprintf(ctx.Writer, "event: %s\n", event)
	// 如果 data 包含换行符，拆分为多行
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		fmt.Fprintf(ctx.Writer, "data: %s\n", line)
	}
	fmt.Fprintf(ctx.Writer, "\n")
	ctx.Writer.Flush()
}

// BuildSystemPrompt 构建系统提示
// todo: 这个的位置和内容需要优化
func BuildSystemPrompt(reg []*skill.Skill) string {
	var sb strings.Builder

	sb.WriteString(`
You are an 智能协作助手 CLI assistant.

You have access to the following skills.

When a user's request matches a skill:
- Say: USING_SKILL: <name> and use load skill tool
- The system will load the skill content for you
- Then follow its instructions strictly

Available skills:
`)

	for _, s := range reg {
		if s.DisableModelInvocation {
			continue
		}

		sb.WriteString("\n---\n")
		sb.WriteString("Name: " + s.Name + "\n")
		sb.WriteString("Description: " + s.Description + "\n")

		if s.WhenToUse != "" {
			sb.WriteString("WhenToUse: " + s.WhenToUse + "\n")
		}
	}

	sb.WriteString(`
When you decide to use a skill:
1. Output EXACTLY: USING_SKILL: <name>
2. Do NOT output any command yet
3. Wait for the system to load the skill content

Creative deliverable review protocol:
- Creative deliverables include documents, reports, copywriting, outlines, slides/PPT, presentations, images, diagrams, plans, proposals, and other subjective artifacts.
- After creating or materially revising a creative deliverable, show the draft/result to the user and ask whether they approve it or want changes.
- If the runtime has request_user_input available, use it for this review so the session pauses for user feedback.
- If the user requests changes, apply the feedback and repeat the review loop.
- Do not consider the creative task complete until the user approves it, says no more changes are needed, or explicitly asked to skip review.

!important 当你按计划执行任务时，你应该始终确认计划的每一步是否完成并及时更新计划状态
`)

	return sb.String()
}

// NewMainAgent 创建主 agent
func NewMainAgent(ctx context.Context, chatModel einomodel.ToolCallingChatModel, systemMsg string, tools []einotool.BaseTool) adk.Agent {
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "main_agent",
		Description: "Main agent that handles user requests and provides solutions.",
		Instruction: systemMsg,
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return a
}

func (c *Controller) getHistory(sessionID string) []*schema.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Mem == nil {
		c.Mem = make(map[string]memory.Memory)
	}

	h, ok := c.Mem[sessionID]
	if !ok {
		h = []*schema.Message{
			schema.SystemMessage(c.SystemMsg),
		}
		c.Mem[sessionID] = h
	}
	return h
}

func (c *Controller) saveHistory(sessionID string, history []*schema.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 这里肯定需要优化
	if len(history) > 20 {
		system := history[0]
		history = append([]*schema.Message{system}, history[len(history)-19:]...)
	}

	c.Mem[sessionID] = history
}
