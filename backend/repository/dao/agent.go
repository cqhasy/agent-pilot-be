package dao

import (
	"context"
	"time"

	atype "github.com/agent-pilot/agent-pilot-be/agent/type"
	"github.com/agent-pilot/agent-pilot-be/repository/model"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AgentDao interface {
	CreateChatSession(ctx context.Context, userID string) (atype.Session, error)
	GetChatSession(ctx context.Context, chatSessionID string) (atype.Session, error)

	UpdateActivePlan(ctx context.Context, chatSessionID string, planID string) error
	CleanActivePlan(ctx context.Context, planID string) error
	InsertPlan(ctx context.Context, p *atype.Plan) error
	GetPlan(ctx context.Context, planID string) (*atype.Plan, error)
	UpdatePlanStatus(ctx context.Context, planID string, status atype.Status) error
	ReplacePlan(ctx context.Context, p *atype.Plan) error

	UpdateCurrentStepID(ctx context.Context, planID string, stepID string) error
	UpdateStepStatus(ctx context.Context, planID string, stepID string, status atype.StepStatus) error
	UpdateStepResult(ctx context.Context, planID string, stepID string, result string) error

	SaveCheckpoint(ctx context.Context, planID string, checkpoint *atype.Checkpoint) error
	ClearCheckpoint(ctx context.Context, planID string) error

	AppendMessage(ctx context.Context, msg *atype.Message) error
	GetPlanMessages(ctx context.Context, planID string) ([]atype.Message, error)
	GetStepMessages(ctx context.Context, planID string, stepID string) ([]atype.Message, error)

	// WS 长连接：eino 图检查点与中断恢复元数据（按 session_id / compose CheckPointID）。
	WSRuntimeGraphGet(ctx context.Context, sessionID string) ([]byte, bool, error)
	WSRuntimeGraphSet(ctx context.Context, sessionID string, graph []byte) error
	WSRuntimeHistoryGet(ctx context.Context, sessionID string) ([]byte, bool, error)
	WSRuntimeHistorySet(ctx context.Context, sessionID string, historyJSON []byte) error
	WSRuntimeResumeGet(ctx context.Context, sessionID string) (WSRuntimeResume, bool, error)
	WSRuntimeResumeSet(ctx context.Context, sessionID string, rec WSRuntimeResume) error
	WSRuntimeResumeClear(ctx context.Context, sessionID string) error
}

type agentDao struct {
	chatSessionCol *mongo.Collection
	planCol        *mongo.Collection
	messageCol     *mongo.Collection
	wsRuntimeCol   *mongo.Collection
}

func NewAgentDao(db *mongo.Database) AgentDao {
	return &agentDao{
		chatSessionCol: db.Collection("agent_sessions"),
		planCol:        db.Collection("agent_plans"),
		messageCol:     db.Collection("agent_messages"),
		wsRuntimeCol:   db.Collection("agent_ws_runtime"),
	}
}

func (ad *agentDao) CreateChatSession(ctx context.Context, userID string) (atype.Session, error) {
	now := time.Now()
	m := model.ChatSession{
		ID:        uuid.New().String(),
		UserID:    userID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := ad.chatSessionCol.InsertOne(ctx, &m)
	if err != nil {
		return atype.Session{}, err
	}
	return sessionFromChatSession(m), nil
}

func (ad *agentDao) GetChatSession(ctx context.Context, chatSessionID string) (atype.Session, error) {
	var m model.ChatSession
	err := ad.chatSessionCol.FindOne(ctx, bson.M{"_id": chatSessionID}).Decode(&m)
	if err != nil {
		return atype.Session{}, err
	}
	return sessionFromChatSession(m), nil
}

func (ad *agentDao) UpdateActivePlan(ctx context.Context, chatSessionID string, planID string) error {
	_, err := ad.chatSessionCol.UpdateOne(
		ctx,
		bson.M{"_id": chatSessionID},
		bson.M{"$set": bson.M{
			"current_plan_id": planID,
			"updated_at":      time.Now(),
		}},
	)
	return err
}

func (ad *agentDao) CleanActivePlan(ctx context.Context, planID string) error {
	if planID == "" {
		return nil
	}
	_, err := ad.chatSessionCol.UpdateMany(
		ctx,
		bson.M{"current_plan_id": planID},
		bson.M{"$set": bson.M{
			"current_plan_id": "",
			"updated_at":      time.Now(),
		}},
	)
	return err
}

func (ad *agentDao) InsertPlan(ctx context.Context, p *atype.Plan) error {
	if p == nil {
		return nil
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	rec := modelFromPlan(p)
	_, err := ad.planCol.InsertOne(ctx, rec)
	return err
}

func (ad *agentDao) GetPlan(ctx context.Context, planID string) (*atype.Plan, error) {
	var r model.Plan
	err := ad.planCol.FindOne(ctx, bson.M{"_id": planID}).Decode(&r)
	if err != nil {
		return nil, err
	}
	return planFromModel(&r), nil
}

func (ad *agentDao) UpdatePlanStatus(ctx context.Context, planID string, status atype.Status) error {
	_, err := ad.planCol.UpdateOne(
		ctx,
		bson.M{"_id": planID},
		bson.M{"$set": bson.M{
			"status":     status,
			"updated_at": time.Now(),
		}},
	)
	return err
}

func (ad *agentDao) ReplacePlan(ctx context.Context, p *atype.Plan) error {
	if p == nil {
		return nil
	}
	p.UpdatedAt = time.Now()
	rec := modelFromPlan(p)
	_, err := ad.planCol.ReplaceOne(ctx, bson.M{"_id": rec.ID}, rec)
	return err
}

func (ad *agentDao) UpdateCurrentStepID(ctx context.Context, planID string, stepID string) error {
	_, err := ad.planCol.UpdateOne(
		ctx,
		bson.M{"_id": planID},
		bson.M{"$set": bson.M{
			"current_step_id": stepID,
			"updated_at":      time.Now(),
		}},
	)
	return err
}

func (ad *agentDao) UpdateStepStatus(ctx context.Context, planID string, stepID string, status atype.StepStatus) error {
	_, err := ad.planCol.UpdateOne(
		ctx,
		bson.M{"_id": planID},
		bson.M{"$set": bson.M{
			"steps.$[s].status": status,
			"updated_at":        time.Now(),
		}},
		options.Update().SetArrayFilters(options.ArrayFilters{
			Filters: []any{bson.M{"s.id": stepID}},
		}),
	)
	return err
}

func (ad *agentDao) UpdateStepResult(ctx context.Context, planID string, stepID string, result string) error {
	_, err := ad.planCol.UpdateOne(
		ctx,
		bson.M{"_id": planID},
		bson.M{"$set": bson.M{
			"steps.$[s].result": result,
			"updated_at":        time.Now(),
		}},
		options.Update().SetArrayFilters(options.ArrayFilters{
			Filters: []any{bson.M{"s.id": stepID}},
		}),
	)
	return err
}

func (ad *agentDao) SaveCheckpoint(ctx context.Context, planID string, checkpoint *atype.Checkpoint) error {
	if checkpoint == nil {
		return ad.ClearCheckpoint(ctx, planID)
	}
	pc := *checkpoint
	pc.CreatedAt = time.Now()
	mc := modelCheckpointFromPlan(&pc)

	_, err := ad.planCol.UpdateOne(
		ctx,
		bson.M{"_id": planID},
		bson.M{"$set": bson.M{
			"checkpoint": mc,
			"updated_at": time.Now(),
		}},
	)
	return err
}

func (ad *agentDao) ClearCheckpoint(ctx context.Context, planID string) error {
	_, err := ad.planCol.UpdateOne(
		ctx,
		bson.M{"_id": planID},
		bson.M{"$set": bson.M{
			"checkpoint": nil,
			"updated_at": time.Now(),
		}},
	)
	return err
}

func (ad *agentDao) AppendMessage(ctx context.Context, msg *atype.Message) error {
	if msg == nil {
		return nil
	}
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	am := agentMessageFromPlan(msg)
	_, err := ad.messageCol.InsertOne(ctx, am)
	return err
}

func (ad *agentDao) GetPlanMessages(ctx context.Context, planID string) ([]atype.Message, error) {
	return ad.findMessages(ctx, bson.M{"plan_id": planID})
}

func (ad *agentDao) GetStepMessages(ctx context.Context, planID string, stepID string) ([]atype.Message, error) {
	return ad.findMessages(ctx, bson.M{"plan_id": planID, "step_id": stepID})
}

func (ad *agentDao) findMessages(ctx context.Context, filter bson.M) ([]atype.Message, error) {
	cur, err := ad.messageCol.Find(
		ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []atype.Message
	for cur.Next(ctx) {
		var m model.AgentMessage
		if err := cur.Decode(&m); err != nil {
			return nil, err
		}
		out = append(out, messageFromAgent(&m))
	}
	return out, cur.Err()
}
