package memory

import (
	"context"
	"errors"
	"github.com/cloudwego/eino/schema"

	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	"github.com/agent-pilot/agent-pilot-be/repository/dao"
	"github.com/google/uuid"
)

type MemoryService interface {
	CreateChatSession(ctx context.Context, userID string) (atype.Session, error)
	GetChatSession(ctx context.Context, chatSessionID string) (atype.Session, error)

	GetActivePlan(ctx context.Context, chatSessionID string) (*atype.Plan, bool, error)
	CreatePlan(ctx context.Context, chatSessionID, goal string) (*atype.Plan, error)
	SavePlan(ctx context.Context, p *atype.Plan) error
	UpdatePlanStatus(ctx context.Context, planID string, status atype.Status) error

	StartStep(ctx context.Context, planID string, stepID string) error
	CompleteStep(ctx context.Context, planID, stepID, result string) error
	FailStep(ctx context.Context, planID, stepID string) error

	PausePlan(ctx context.Context, planID, stepID, question string) error
	ResumePlan(ctx context.Context, planID string) (*ResumeContext, error)

	AppendMessage(ctx context.Context, msg *atype.Message) error
	GetPlanMessages(ctx context.Context, planID string) ([]atype.Message, error)
	GetStepMessages(ctx context.Context, planID, stepID string) ([]atype.Message, error)

	BuildStepContext(ctx context.Context, planID, stepID string) (*StepContext, error)
}

type StepContext struct {
	Plan     *atype.Plan
	Step     atype.Step
	Messages []atype.Message
}

type ResumeContext struct {
	Plan       *atype.Plan
	Step       *atype.Step
	Checkpoint *atype.Checkpoint
	Messages   []atype.Message
}

type memoryService struct {
	dao dao.AgentDao
}

func NewMemoryService(d dao.AgentDao) MemoryService {
	return &memoryService{dao: d}
}

func (s *memoryService) CreateChatSession(ctx context.Context, userID string) (atype.Session, error) {
	return s.dao.CreateChatSession(ctx, userID)
}

func (s *memoryService) GetChatSession(ctx context.Context, chatSessionID string) (atype.Session, error) {
	return s.dao.GetChatSession(ctx, chatSessionID)
}

func (s *memoryService) GetActivePlan(ctx context.Context, chatSessionID string) (*atype.Plan, bool, error) {
	sess, err := s.dao.GetChatSession(ctx, chatSessionID)
	if err != nil {
		return nil, false, err
	}
	if sess.CurrentPlanID == "" {
		return nil, false, nil
	}
	p, err := s.dao.GetPlan(ctx, sess.CurrentPlanID)
	if err != nil {
		return nil, false, err
	}
	return p, true, nil
}

func (s *memoryService) CreatePlan(ctx context.Context, chatSessionID, goal string) (*atype.Plan, error) {
	p := &atype.Plan{
		ID:        uuid.New().String(),
		SessionID: chatSessionID,
		Goal:      goal,
		Status:    atype.StatusDraft,
		Steps:     []atype.Step{},
	}
	if err := s.dao.InsertPlan(ctx, p); err != nil {
		return nil, err
	}
	if err := s.dao.UpdateActivePlan(ctx, chatSessionID, p.ID); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *memoryService) SavePlan(ctx context.Context, p *atype.Plan) error {
	err := s.dao.ReplacePlan(ctx, p)
	if err != nil {
		return err
	}

	return s.dao.UpdatePlanStatus(ctx, p.ID, atype.StatusReady)
}

func (s *memoryService) UpdatePlanStatus(ctx context.Context, planID string, status atype.Status) error {
	return s.dao.UpdatePlanStatus(ctx, planID, status)
}

func (s *memoryService) StartStep(ctx context.Context, planID, stepID string) error {
	//更新currentStep
	err := s.dao.UpdateCurrentStepID(ctx, planID, stepID)
	if err != nil {
		return err
	}

	//更新当前步骤状态
	err = s.dao.UpdateStepStatus(ctx, planID, stepID, atype.StepStatusRunning)
	if err != nil {
		return err
	}

	//更新plan状态
	return s.dao.UpdatePlanStatus(ctx, planID, atype.StatusExecuting)
}

func (s *memoryService) CompleteStep(ctx context.Context, planID, stepID, result string) error {
	//更新状态
	if err := s.dao.UpdateStepResult(ctx, planID, stepID, result); err != nil {
		return err
	}
	err := s.dao.UpdateStepStatus(ctx, planID, stepID, atype.StepStatusCompleted)
	if err != nil {
		return err
	}
	//清除checkpoint
	err = s.dao.ClearCheckpoint(ctx, planID)
	if err != nil {
		return err
	}
	plan, err := s.dao.GetPlan(ctx, planID)
	if err != nil {
		return err
	}
	if s.findNextStep(plan.Steps, stepID) == nil {
		return s.dao.UpdatePlanStatus(ctx, planID, atype.StatusCompleted)
	}

	nextStep := s.findNextStep(plan.Steps, stepID)
	if nextStep == nil {
		err := s.dao.UpdatePlanStatus(ctx, planID, atype.StatusCompleted)
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (s *memoryService) FailStep(ctx context.Context, planID, stepID string) error {
	err := s.dao.UpdateStepStatus(ctx, planID, stepID, atype.StepStatusFailed)
	if err != nil {
		return err
	}

	//清除checkpoint
	err = s.dao.ClearCheckpoint(ctx, planID)
	if err != nil {
		return err
	}

	return s.dao.UpdatePlanStatus(ctx, planID, atype.StatusFailed)
}

func (s *memoryService) PausePlan(ctx context.Context, planID, stepID, question string) error {
	err := s.dao.UpdatePlanStatus(ctx, planID, atype.StatusPaused)
	if err != nil {
		return err
	}
	err = s.dao.UpdateStepStatus(ctx, planID, stepID, atype.StepStatusPaused)
	if err != nil {
		return err
	}
	return s.dao.SaveCheckpoint(ctx, planID, &atype.Checkpoint{
		ID:       uuid.New().String(),
		StepID:   stepID,
		Question: question,
	})
}

func (s *memoryService) ResumePlan(ctx context.Context, planID string) (*ResumeContext, error) {

	plan, err := s.dao.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}

	if plan.Checkpoint == nil {
		return nil, errors.New("checkpoint not found")
	}
	stepID := plan.Checkpoint.StepID
	var step *atype.Step
	for i := range plan.Steps {
		if plan.Steps[i].ID == stepID {
			step = &plan.Steps[i]
			break
		}
	}
	if step == nil {
		return nil, errors.New("step not found")
	}
	msgs, err := s.dao.GetStepMessages(ctx, planID, stepID)
	if err != nil {
		return nil, err
	}

	//恢复状态
	err = s.dao.UpdatePlanStatus(ctx, planID, atype.StatusExecuting)
	if err != nil {
		return nil, err
	}
	err = s.dao.UpdateStepStatus(ctx, planID, stepID, atype.StepStatusRunning)
	if err != nil {
		return nil, err
	}
	err = s.dao.UpdateCurrentStepID(ctx, planID, stepID)
	if err != nil {
		return nil, err
	}

	//清除checkpoint
	err = s.dao.ClearCheckpoint(ctx, planID)
	if err != nil {
		return nil, err
	}

	return &ResumeContext{
		Plan:       plan,
		Step:       step,
		Checkpoint: plan.Checkpoint,
		Messages:   msgs,
	}, nil
}

func (s *memoryService) AppendMessage(ctx context.Context, msg *atype.Message) error {
	return s.dao.AppendMessage(ctx, msg)
}

func (s *memoryService) GetPlanMessages(ctx context.Context, planID string) ([]atype.Message, error) {
	return s.dao.GetPlanMessages(ctx, planID)
}

func (s *memoryService) GetStepMessages(ctx context.Context, planID, stepID string) ([]atype.Message, error) {
	return s.dao.GetStepMessages(ctx, planID, stepID)
}

func (s *memoryService) BuildStepContext(ctx context.Context, planID, stepID string) (*StepContext, error) {
	p, err := s.dao.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	var st atype.Step
	for i := range p.Steps {
		if p.Steps[i].ID == stepID {
			st = p.Steps[i]
			break
		}
	}
	msgs, err := s.dao.GetStepMessages(ctx, planID, stepID)
	if err != nil {
		return nil, err
	}
	return &StepContext{Plan: p, Step: st, Messages: msgs}, nil
}

func (s *memoryService) findNextStep(steps []atype.Step, currentStepID string) *atype.Step {
	for i := range steps {
		if steps[i].ID == currentStepID {
			if i+1 >= len(steps) {
				return nil
			}
			return &steps[i+1]
		}
	}
	return nil
}

type Memory []*schema.Message
