package dao

import (
	"context"
	"errors"
	"time"

	"github.com/agent-pilot/agent-pilot-be/repository/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// WSRuntimeResume 与 websocket 侧 wsInterruptedState + interrupt_id 对齐的可持久化视图。
type WSRuntimeResume struct {
	InterruptID   string
	Message       string
	StepID        string
	RequestID     string
	PlanJSON      []byte
	InterruptKind string
}

func (ad *agentDao) WSRuntimeHistoryGet(ctx context.Context, sessionID string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	var doc model.WSRuntimeDoc
	err := ad.wsRuntimeCol.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	if len(doc.HistoryJSON) == 0 {
		return nil, false, nil
	}
	return append([]byte(nil), doc.HistoryJSON...), true, nil
}

func (ad *agentDao) WSRuntimeHistorySet(ctx context.Context, sessionID string, historyJSON []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	now := time.Now()
	_, err := ad.wsRuntimeCol.UpdateOne(
		ctx,
		bson.M{"_id": sessionID},
		bson.M{
			"$set": bson.M{
				"history_json": append([]byte(nil), historyJSON...),
				"updated_at":   now,
			},
			"$setOnInsert": bson.M{"_id": sessionID},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (ad *agentDao) WSRuntimeGraphGet(ctx context.Context, sessionID string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	var doc model.WSRuntimeDoc
	err := ad.wsRuntimeCol.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	if len(doc.Graph) == 0 {
		return nil, false, nil
	}
	return append([]byte(nil), doc.Graph...), true, nil
}

func (ad *agentDao) WSRuntimeGraphSet(ctx context.Context, sessionID string, graph []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	_, err := ad.wsRuntimeCol.UpdateOne(
		ctx,
		bson.M{"_id": sessionID},
		bson.M{
			"$set": bson.M{
				"graph":      append([]byte(nil), graph...),
				"updated_at": now,
			},
			"$setOnInsert": bson.M{"_id": sessionID},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (ad *agentDao) WSRuntimeResumeGet(ctx context.Context, sessionID string) (WSRuntimeResume, bool, error) {
	var zero WSRuntimeResume
	if err := ctx.Err(); err != nil {
		return zero, false, err
	}

	var doc model.WSRuntimeDoc
	err := ad.wsRuntimeCol.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}

	if doc.InterruptID == "" {
		return zero, false, nil
	}
	return WSRuntimeResume{
		InterruptID:   doc.InterruptID,
		Message:       doc.IntMessage,
		StepID:        doc.IntStepID,
		RequestID:     doc.IntRequestID,
		PlanJSON:      append([]byte(nil), doc.IntPlanJSON...),
		InterruptKind: doc.InterruptKind,
	}, true, nil
}

func (ad *agentDao) WSRuntimeResumeSet(ctx context.Context, sessionID string, rec WSRuntimeResume) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	_, err := ad.wsRuntimeCol.UpdateOne(
		ctx,
		bson.M{"_id": sessionID},
		bson.M{
			"$set": bson.M{
				"interrupt_id":   rec.InterruptID,
				"int_message":    rec.Message,
				"int_step_id":    rec.StepID,
				"int_request_id": rec.RequestID,
				"int_plan_json":  append([]byte(nil), rec.PlanJSON...),
				"interrupt_kind": rec.InterruptKind,
				"updated_at":     now,
			},
			"$setOnInsert": bson.M{"_id": sessionID},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (ad *agentDao) WSRuntimeResumeClear(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := ad.wsRuntimeCol.UpdateOne(
		ctx,
		bson.M{"_id": sessionID},
		bson.M{
			"$unset": bson.M{
				"interrupt_id":   "",
				"int_message":    "",
				"int_step_id":    "",
				"int_request_id": "",
				"int_plan_json":  "",
			},
			"$set": bson.M{"updated_at": time.Now()},
		},
	)
	return err
}
