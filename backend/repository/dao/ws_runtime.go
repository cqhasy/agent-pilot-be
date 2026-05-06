package dao

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/agent-pilot/agent-pilot-be/repository/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// WSRuntimeResume 与 websocket 侧 wsInterruptedState + interrupt_id 对齐的可持久化视图。
type WSRuntimeResume struct {
	InterruptID     string
	Message         string
	StepID          string
	RequestID       string
	PlanJSON        []byte
	ActiveExpertID  string
	InterruptKind   string
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

func (ad *agentDao) WSRuntimeHistoryItemsGet(ctx context.Context, sessionID string) ([][]byte, bool, error) {
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
	if len(doc.HistoryItems) == 0 {
		return nil, false, nil
	}
	out := make([][]byte, 0, len(doc.HistoryItems))
	for _, item := range doc.HistoryItems {
		out = append(out, append([]byte(nil), item...))
	}
	return out, true, nil
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

func (ad *agentDao) WSRuntimeHistoryAppend(ctx context.Context, sessionID string, items [][]byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	copied := make([]any, 0, len(items))
	for _, item := range items {
		copied = append(copied, append([]byte(nil), item...))
	}
	now := time.Now()
	_, err := ad.wsRuntimeCol.UpdateOne(
		ctx,
		bson.M{"_id": sessionID},
		bson.M{
			"$push": bson.M{
				"history_items": bson.M{"$each": copied},
			},
			"$set":         bson.M{"updated_at": now},
			"$setOnInsert": bson.M{"_id": sessionID},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (ad *agentDao) WSRuntimeSessionTouch(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	_, err := ad.wsRuntimeCol.UpdateOne(
		ctx,
		bson.M{"_id": sessionID},
		bson.M{
			"$set":         bson.M{"updated_at": now},
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
		InterruptID:    doc.InterruptID,
		Message:        doc.IntMessage,
		StepID:         doc.IntStepID,
		RequestID:      doc.IntRequestID,
		PlanJSON:       append([]byte(nil), doc.IntPlanJSON...),
		ActiveExpertID: strings.TrimSpace(doc.IntExpertID),
		InterruptKind:  doc.InterruptKind,
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
				"interrupt_id":    rec.InterruptID,
				"int_message":     rec.Message,
				"int_step_id":     rec.StepID,
				"int_request_id":  rec.RequestID,
				"int_plan_json":     append([]byte(nil), rec.PlanJSON...),
				"int_expert_id":     strings.TrimSpace(rec.ActiveExpertID),
				"interrupt_kind":    rec.InterruptKind,
				"updated_at":        now,
			},
			"$setOnInsert": bson.M{"_id": sessionID},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

// WSRuntimeListPrimarySessions 列出主 WebSocket 会话（_id 不含冒号，与专家图 checkpoint 文档区分）。
func (ad *agentDao) WSRuntimeListPrimarySessions(ctx context.Context, limit int64) ([]WSRuntimeSessionRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetLimit(limit).
		SetProjection(bson.M{"_id": 1, "updated_at": 1, "preview_title": 1})
	// 专家 compose 使用 session_id:expert:eid 作为 _id，主会话为纯 UUID 等无冒号形式
	filter := bson.M{"_id": bson.M{"$regex": `^[^:]+$`}}

	cur, err := ad.wsRuntimeCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []WSRuntimeSessionRow
	for cur.Next(ctx) {
		var doc struct {
			ID           string    `bson:"_id"`
			UpdatedAt    time.Time `bson:"updated_at"`
			PreviewTitle string    `bson:"preview_title"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		if doc.ID == "" {
			continue
		}
		out = append(out, WSRuntimeSessionRow{
			SessionID:    doc.ID,
			UpdatedAt:    doc.UpdatedAt,
			PreviewTitle: strings.TrimSpace(doc.PreviewTitle),
		})
	}
	return out, cur.Err()
}

func (ad *agentDao) WSRuntimeSetPreviewTitleIfEmpty(ctx context.Context, sessionID string, title string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sessionID = strings.TrimSpace(sessionID)
	title = strings.TrimSpace(title)
	if sessionID == "" || title == "" {
		return nil
	}
	now := time.Now()
	filter := bson.M{
		"_id": sessionID,
		"$or": []bson.M{
			{"preview_title": bson.M{"$exists": false}},
			{"preview_title": ""},
		},
	}
	_, err := ad.wsRuntimeCol.UpdateOne(
		ctx,
		filter,
		bson.M{"$set": bson.M{
			"preview_title": title,
			"updated_at":    now,
		}},
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
				"interrupt_id":    "",
				"int_message":     "",
				"int_step_id":     "",
				"int_request_id":  "",
				"int_plan_json":     "",
				"int_expert_id":     "",
			},
			"$set": bson.M{"updated_at": time.Now()},
		},
	)
	return err
}
