package addcolumn

import (
	"context"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/drewjocham/mongork/examples/practical/internal/progress"
	"github.com/drewjocham/mongork/internal/migration"
)

const (
	customersColl        = "customers"
	progressColl         = "migration_progress"
	progressKey          = "backfill_preferred_locale"
	preferredLocaleField = "profile.preferred_locale"
	legacyLocaleField    = "locale"
	defaultLocale        = "de_DE"
	batchSize            = 1000
)

func Examples() []migration.Migration {
	return []migration.Migration{
		&addShadowLocaleField{},
		&backfillShadowLocale{},
		&removeLegacyLocale{},
	}
}

type addShadowLocaleField struct{}

func (m *addShadowLocaleField) Version() string {
	return "example_practical_20240201_add_shadow_locale_field"
}

func (m *addShadowLocaleField) Description() string {
	return "Expand schema with profile.preferred_locale and seed pilot documents"
}

func (m *addShadowLocaleField) Up(ctx context.Context, db *mongo.Database) error {
	if err := ensureCustomersValidator(ctx, db, true); err != nil {
		return err
	}

	collection := db.Collection(customersColl)
	_, err := collection.UpdateMany(
		ctx,
		bson.M{preferredLocaleField: bson.M{"$exists": false}},
		bson.M{"$set": bson.M{preferredLocaleField: defaultLocale}},
	)
	return err
}

func (m *addShadowLocaleField) Down(ctx context.Context, db *mongo.Database) error {
	if err := ensureCustomersValidator(ctx, db, false); err != nil {
		return err
	}
	collection := db.Collection(customersColl)
	_, err := collection.UpdateMany(
		ctx,
		bson.M{},
		bson.M{"$unset": bson.M{preferredLocaleField: ""}},
	)
	return err
}

type backfillShadowLocale struct{}

func (m *backfillShadowLocale) Version() string {
	return "example_practical_20240202_backfill_shadow_locale"
}

func (m *backfillShadowLocale) Description() string {
	return "Backfill preferred_locale in batches with resume checkpoints"
}

func (m *backfillShadowLocale) Up(ctx context.Context, db *mongo.Database) error {
	progressStore := progress.NewStore(db.Collection(progressColl))
	lastID, err := progressStore.LastID(ctx, progressKey)
	if err != nil {
		return err
	}

	coll := db.Collection(customersColl)
	for {
		filter := bson.M{preferredLocaleField: bson.M{"$exists": false}}
		if lastID != bson.NilObjectID {
			filter["_id"] = bson.M{"$gt": lastID}
		}

		cur, err := coll.Find(ctx, filter, options.Find().
			SetSort(bson.D{{Key: "_id", Value: 1}}).
			SetLimit(int64(batchSize)),
		)
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

		bulk := make([]mongo.WriteModel, 0, len(docs))
		for _, doc := range docs {
			id, _ := doc["_id"].(bson.ObjectID)
			update := mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": id}).
				SetUpdate(bson.M{
					"$set": bson.M{
						preferredLocaleField: localeFromDoc(doc),
					},
				})
			bulk = append(bulk, update)
			lastID = id
		}

		if _, err := coll.BulkWrite(ctx, bulk, options.BulkWrite().SetOrdered(false)); err != nil {
			return err
		}
		if err := progressStore.Save(ctx, progressKey, lastID); err != nil {
			return err
		}

		if len(docs) < batchSize {
			break
		}
	}

	return progressStore.Clear(ctx, progressKey)
}

func (m *backfillShadowLocale) Down(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection(customersColl).UpdateMany(
		ctx,
		bson.M{},
		bson.M{"$unset": bson.M{preferredLocaleField: ""}},
	)
	return err
}

type removeLegacyLocale struct{}

func (m *removeLegacyLocale) Version() string {
	return "example_practical_20240203_remove_legacy_locale"
}

func (m *removeLegacyLocale) Description() string {
	return "Contract schema by dropping legacy users.locale"
}

func (m *removeLegacyLocale) Up(ctx context.Context, db *mongo.Database) error {
	coll := db.Collection("users")
	if _, err := coll.UpdateMany(ctx, bson.M{}, bson.M{"$unset": bson.M{legacyLocaleField: ""}}); err != nil {
		return err
	}
	return ensureCustomersValidator(ctx, db, true)
}

func (m *removeLegacyLocale) Down(ctx context.Context, db *mongo.Database) error {
	return ensureCustomersValidator(ctx, db, false)
}

func ensureCustomersValidator(ctx context.Context, db *mongo.Database, enforce bool) error {
	builder := migration.Schema().
		BsonType("object").
		Field("profile", bson.M{
			"bsonType": "object",
			"properties": bson.M{
				"preferred_locale": bson.M{
					"bsonType": "string",
					"enum":     []string{"en-US", "en-GB", "fr-FR", "es-ES"},
				},
			},
		})

	validator := builder.Build()
	if !enforce {
		delete(validator["$jsonSchema"].(bson.M)["properties"].(bson.M), "profile")
	}

	cmd := bson.D{
		{Key: "collMod", Value: customersColl},
		{Key: "validator", Value: validator},
		{Key: "validationLevel", Value: func() string {
			if enforce {
				return "moderate"
			}
			return "off"
		}()},
	}

	return db.RunCommand(ctx, cmd).Err()
}

func localeFromDoc(doc bson.M) string {
	if legacy, ok := doc["locale"].(string); ok && legacy != "" {
		return legacy
	}
	return defaultLocale
}
