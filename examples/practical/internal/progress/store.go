package progress

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Store persists chunk checkpoints so batch migrations can resume safely.
type Store struct {
	coll *mongo.Collection
}

func NewStore(coll *mongo.Collection) *Store {
	return &Store{coll: coll}
}

func (s *Store) LastID(ctx context.Context, key string) (bson.ObjectID, error) {
	if s == nil || s.coll == nil {
		return bson.NilObjectID, nil
	}
	var doc struct {
		LastID bson.ObjectID `bson:"last_id"`
	}
	err := s.coll.FindOne(ctx, bson.M{"_id": key}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return bson.NilObjectID, nil
	}
	if err != nil {
		return bson.NilObjectID, err
	}
	return doc.LastID, nil
}

func (s *Store) Save(ctx context.Context, key string, id bson.ObjectID) error {
	if s == nil || s.coll == nil {
		return nil
	}
	_, err := s.coll.UpdateOne(
		ctx,
		bson.M{"_id": key},
		bson.M{
			"$set": bson.M{
				"last_id":  id,
				"updated":  time.Now().UTC(),
				"batch_id": fmt.Sprintf("%s-%s", key, id.Hex()),
			},
		},
		options.UpdateOne().SetUpsert(true))

	return err
}

func (s *Store) Clear(ctx context.Context, key string) error {
	if s == nil || s.coll == nil {
		return nil
	}
	_, err := s.coll.DeleteOne(ctx, bson.M{"_id": key})
	return err
}
