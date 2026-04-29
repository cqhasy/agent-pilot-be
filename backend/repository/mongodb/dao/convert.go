package dao

import (
	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	"github.com/agent-pilot/agent-pilot-be/repository/mongodb/model"
)

func planFromModel(r *model.Plan) *atype.Plan {
	if r == nil {
		return nil
	}
	steps := make([]atype.Step, len(r.Steps))
	for i, s := range r.Steps {
		steps[i] = atype.Step{
			ID:          s.ID,
			Title:       s.Title,
			Description: s.Description,
			Result:      s.Result,
			Status:      atype.StepStatus(s.Status),
		}
	}
	return &atype.Plan{
		ID:            r.ID,
		SessionID:     r.SessionID,
		Goal:          r.Goal,
		Steps:         steps,
		Status:        atype.Status(r.Status),
		CurrentStepID: r.CurrentStepID,
		Checkpoint:    checkpointFromModel(r.Checkpoint),
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

func checkpointFromModel(m *model.Checkpoint) *atype.Checkpoint {
	if m == nil {
		return nil
	}
	return &atype.Checkpoint{
		ID:        m.ID,
		StepID:    m.StepID,
		Question:  m.Question,
		CreatedAt: m.CreatedAt,
	}
}

func modelFromPlan(p *atype.Plan) *model.Plan {
	if p == nil {
		return nil
	}
	steps := make([]model.Step, len(p.Steps))
	for i, s := range p.Steps {
		steps[i] = model.Step{
			ID:          s.ID,
			Title:       s.Title,
			Description: s.Description,
			Result:      s.Result,
			Status:      string(s.Status),
		}
	}
	return &model.Plan{
		ID:            p.ID,
		SessionID:     p.SessionID,
		Goal:          p.Goal,
		Steps:         steps,
		Status:        string(p.Status),
		CurrentStepID: p.CurrentStepID,
		Checkpoint:    modelCheckpointFromPlan(p.Checkpoint),
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

func modelCheckpointFromPlan(c *atype.Checkpoint) *model.Checkpoint {
	if c == nil {
		return nil
	}
	return &model.Checkpoint{
		ID:        c.ID,
		StepID:    c.StepID,
		Question:  c.Question,
		CreatedAt: c.CreatedAt,
	}
}

func sessionFromChatSession(m model.ChatSession) atype.Session {
	return atype.Session{
		ID:            m.ID,
		UserID:        m.UserID,
		CurrentPlanID: m.CurrentPlanID,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

func messageFromAgent(m *model.AgentMessage) atype.Message {
	return atype.Message{
		ID:        m.ID,
		SessionID: m.SessionID,
		PlanID:    m.PlanID,
		StepID:    m.StepID,
		Role:      atype.MessageRole(m.Role),
		Content:   m.Content,
		Metadata:  m.Metadata,
		CreatedAt: m.CreatedAt,
	}
}

func agentMessageFromPlan(m *atype.Message) *model.AgentMessage {
	if m == nil {
		return nil
	}
	return &model.AgentMessage{
		ID:        m.ID,
		SessionID: m.SessionID,
		PlanID:    m.PlanID,
		StepID:    m.StepID,
		Role:      string(m.Role),
		Content:   m.Content,
		Metadata:  m.Metadata,
		CreatedAt: m.CreatedAt,
	}
}
