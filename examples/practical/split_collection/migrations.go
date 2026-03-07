package splitcollection

import (
	"context"
	"time"

	"github.com/drewjocham/mongork/examples/practical/internal/progress"
	"github.com/drewjocham/mongork/internal/migration"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	ordersColl        = "orders"
	ordersArchiveColl = "orders_archive"
	retentionDays     = 540
	moveBatchSize     = 2000
	moveProgressKey   = "split_orders_archive"
)

func Examples() []migration.Migration {
	return []migration.Migration{
		&createOrdersArchive{},
		&moveHistoricOrders{},
		&enforceHotRetention{},
	}
}

type createOrdersArchive struct{}

func (m *createOrdersArchive) Version() string {
	return "example_practical_20240301_create_orders_archive"
}

func (m *createOrdersArchive) Description() string {
	return "Create orders_archive collection with validators and indexes"
}

func (m *createOrdersArchive) Up(ctx context.Context, db *mongo.Database) error {
	_, err := migration.EnsureCollection(ctx, db, ordersArchiveColl,
		migration.WithValidator(migration.Schema().
			BsonType("object").
			Field("hot_order_id", migration.Schema().String()).
			Field("completed_at", migration.Schema().Date()).
			Field("moved_at", migration.Schema().Date()).
			Build()),
		migration.WithValidationLevel("moderate"),
	)
	if err != nil {
		return err
	}

	return migration.CreateIndexes(ctx, db.Collection(ordersArchiveColl),
		migration.Index(migration.Desc("completed_at")).Name("idx_orders_archive_completed_at").Model(),
		migration.Index(migration.Asc("hot_order_id")).Name("idx_orders_archive_hot_order_id").Unique().Model(),
	)
}

func (m *createOrdersArchive) Down(ctx context.Context, db *mongo.Database) error {
	return db.Collection(ordersArchiveColl).Drop(ctx)
}

type moveHistoricOrders struct{}

func (m *moveHistoricOrders) Version() string {
	return "example_practical_20240302_move_historic_orders"
}

func (m *moveHistoricOrders) Description() string {
	return "Move orders older than retention window into archive in batches"
}

func (m *moveHistoricOrders) Up(ctx context.Context, db *mongo.Database) error {
	store := progress.NewStore(db.Collection("migration_progress"))
	lastID, err := store.LastID(ctx, moveProgressKey)
	if err != nil {
		return err
	}

	orders := db.Collection(ordersColl)
	archive := db.Collection(ordersArchiveColl)
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	for {
		filter := bson.M{"completed_at": bson.M{"$lt": cutoff}}
		if lastID != bson.NilObjectID {
			filter["_id"] = bson.M{"$gt": lastID}
		}

		opts := options.Find().
			SetSort(bson.D{{Key: "_id", Value: 1}}).
			SetLimit(int64(moveBatchSize)).
			SetHint(bson.D{{Key: "_id", Value: 1}})

		cur, err := orders.Find(ctx, filter, opts)
		if err != nil {
			return err
		}

		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			return err
		}
		if len(docs) == 0 {
			break
		}

		now := time.Now().UTC()
		archiveDocs := make([]interface{}, 0, len(docs))
		deleteIDs := make([]bson.ObjectID, 0, len(docs))

		for _, doc := range docs {
			id, _ := doc["_id"].(bson.ObjectID)
			doc["hot_order_id"] = id
			doc["moved_at"] = now
			archiveDocs = append(archiveDocs, doc)
			deleteIDs = append(deleteIDs, id)
			lastID = id
		}

		if _, err := archive.InsertMany(ctx, archiveDocs, options.InsertMany().SetOrdered(false)); err != nil {
			return err
		}

		if _, err := orders.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": deleteIDs}}); err != nil {
			return err
		}

		if err := store.Save(ctx, moveProgressKey, lastID); err != nil {
			return err
		}

		time.Sleep(250 * time.Millisecond)
		if len(docs) < moveBatchSize {
			break
		}
	}

	return store.Clear(ctx, moveProgressKey)
}

func (m *moveHistoricOrders) Down(ctx context.Context, db *mongo.Database) error {
	archive := db.Collection(ordersArchiveColl)
	orders := db.Collection(ordersColl)

	cur, err := archive.Find(ctx, bson.M{}, options.Find().SetLimit(int64(moveBatchSize)))
	if err != nil {
		return err
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return err
	}
	if len(docs) == 0 {
		return nil
	}

	restoreDocs := make([]interface{}, 0, len(docs))
	deleteIDs := make([]bson.ObjectID, 0, len(docs))
	for _, doc := range docs {
		if id, ok := doc["hot_order_id"].(bson.ObjectID); ok {
			doc["_id"] = id
			delete(doc, "hot_order_id")
			restoreDocs = append(restoreDocs, doc)
			deleteIDs = append(deleteIDs, id)
		}
	}

	if _, err := orders.InsertMany(ctx, restoreDocs, options.InsertMany().SetOrdered(false)); err != nil {
		return err
	}
	_, err = archive.DeleteMany(ctx, bson.M{"hot_order_id": bson.M{"$in": deleteIDs}})
	return err
}

type enforceHotRetention struct{}

func (m *enforceHotRetention) Version() string {
	return "example_practical_20240303_enforce_hot_retention_window"
}

func (m *enforceHotRetention) Description() string {
	return "Add validator to orders collection enforcing hot retention window"
}

func (m *enforceHotRetention) Up(ctx context.Context, db *mongo.Database) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	return db.RunCommand(ctx, bson.D{
		{Key: "collMod", Value: ordersColl},
		{Key: "validator", Value: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"properties": bson.M{
					"completed_at": bson.M{"bsonType": "date", "minimum": cutoff},
				},
			},
		}},
		{Key: "validationLevel", Value: "moderate"},
	}).Err()
}

func (m *enforceHotRetention) Down(ctx context.Context, db *mongo.Database) error {
	return db.RunCommand(ctx, bson.D{
		{Key: "collMod", Value: ordersColl},
		{Key: "validator", Value: bson.M{}},
		{Key: "validationLevel", Value: "off"},
	}).Err()
}
