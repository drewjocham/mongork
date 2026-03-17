//go:build integration

package migration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcMongo "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestEnginePlanAndApplyIntegration(t *testing.T) {
	t.Parallel()
	suite := newMongoSuite(t)
	defer suite.Close()

	migrations := map[string]Migration{
		"20240101_add_flag": scriptMigration{
			version:     "20240101_add_flag",
			description: "add feature flag",
			upFn: func(ctx context.Context, db *mongo.Database) error {
				_, err := db.Collection("features").InsertOne(ctx, bson.M{"_id": "beta", "enabled": false})
				return err
			},
			downFn: func(ctx context.Context, db *mongo.Database) error {
				_, err := db.Collection("features").DeleteOne(ctx, bson.M{"_id": "beta"})
				return err
			},
		},
		"20240102_enable_flag": scriptMigration{
			version:     "20240102_enable_flag",
			description: "enable beta",
			upFn: func(ctx context.Context, db *mongo.Database) error {
				_, err := db.Collection("features").UpdateOne(ctx, bson.M{"_id": "beta"}, bson.M{"$set": bson.M{"enabled": true}})
				return err
			},
			downFn: func(ctx context.Context, db *mongo.Database) error {
				_, err := db.Collection("features").UpdateOne(ctx, bson.M{"_id": "beta"}, bson.M{"$set": bson.M{"enabled": false}})
				return err
			},
		},
		"20240103_add_index": scriptMigration{
			version:     "20240103_add_index",
			description: "index enabled",
			upFn: func(ctx context.Context, db *mongo.Database) error {
				return CreateIndexes(ctx, db.Collection("features"),
					Index(Asc("enabled")).Name("idx_features_enabled"),
				)
			},
			downFn: func(ctx context.Context, db *mongo.Database) error {
				return DropIndexes(ctx, db.Collection("features"), "idx_features_enabled")
			},
		},
	}

	engine := NewEngineWithMigrations(suite.DB, suite.CollName("schema_migrations"), migrations)

	ctx := context.Background()
	plan, err := engine.Plan(ctx, DirectionUp, "")
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if len(plan) != 3 {
		t.Fatalf("expected 3 migrations, got %v", plan)
	}

	if err := engine.Up(ctx, "20240102_enable_flag"); err != nil {
		t.Fatalf("up failed: %v", err)
	}

	var doc bson.M
	if err := suite.DB.Collection("features").FindOne(ctx, bson.M{"_id": "beta"}).Decode(&doc); err != nil {
		t.Fatalf("expected feature document: %v", err)
	}
	if enabled, _ := doc["enabled"].(bool); !enabled {
		t.Fatalf("expected enabled=true, got %v", doc["enabled"])
	}

	downPlan, err := engine.Plan(ctx, DirectionDown, "")
	if err != nil {
		t.Fatalf("down plan failed: %v", err)
	}
	if len(downPlan) != 2 || downPlan[0] != "20240102_enable_flag" {
		t.Fatalf("unexpected down plan: %v", downPlan)
	}

	if err := engine.Down(ctx, "20240101_add_flag"); err != nil {
		t.Fatalf("down failed: %v", err)
	}
	count, _ := suite.DB.Collection("features").CountDocuments(ctx, bson.M{})
	if count != 0 {
		t.Fatalf("expected collection empty after down, got %d", count)
	}
}

var (
	containerOnce sync.Once
	containerErr  error
	sharedMongo   *tcMongo.MongoDBContainer
	sharedClient  *mongo.Client
)

func TestMain(m *testing.M) {
	containerOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		sharedMongo, containerErr = tcMongo.RunContainer(ctx,
			testcontainers.WithImage("mongo:7.0"),
		)
		if containerErr != nil {
			return
		}
		uri, err := sharedMongo.ConnectionString(ctx)
		if err != nil {
			containerErr = err
			return
		}
		sharedClient, containerErr = mongo.Connect(options.Client().ApplyURI(uri))
	})

	if containerErr != nil {
		fmt.Printf("failed to start mongo container: %v\n", containerErr)
		os.Exit(1)
	}

	code := m.Run()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if sharedClient != nil {
		_ = sharedClient.Disconnect(ctx)
	}
	if sharedMongo != nil {
		_ = sharedMongo.Terminate(ctx)
	}
	os.Exit(code)
}

func TestEngineChecksumMismatchIntegration(t *testing.T) {
	t.Parallel()
	suite := newMongoSuite(t)
	defer suite.Close()

	mig := scriptMigration{
		version:     "20240201_seed",
		description: "seed config",
		upFn: func(ctx context.Context, db *mongo.Database) error {
			_, err := db.Collection("configs").InsertOne(ctx, bson.M{"_id": "app", "value": "v1"})
			return err
		},
		downFn: func(ctx context.Context, db *mongo.Database) error {
			_, err := db.Collection("configs").DeleteOne(ctx, bson.M{"_id": "app"})
			return err
		},
	}
	engine := NewEngineWithMigrations(suite.DB, suite.CollName("schema_migrations"), map[string]Migration{mig.version: mig})
	ctx := context.Background()

	if err := engine.Up(ctx, ""); err != nil {
		t.Fatalf("initial up failed: %v", err)
	}

	coll := suite.DB.Collection(engine.coll)
	if _, err := coll.UpdateOne(ctx, bson.M{"version": mig.version}, bson.M{"$set": bson.M{"description": "tampered"}}); err != nil {
		t.Fatalf("failed to tamper record: %v", err)
	}

	if err := engine.Up(ctx, ""); err == nil {
		t.Fatalf("expected checksum mismatch error")
	}
}

func TestEngineLockContentionIntegration(t *testing.T) {
	t.Parallel()
	suite := newMongoSuite(t)
	defer suite.Close()

	engine := NewEngineWithMigrations(suite.DB, suite.CollName("schema_migrations"), map[string]Migration{})
	ctx := context.Background()

	lockColl := suite.DB.Collection(collLock)
	_, err := lockColl.InsertOne(ctx, bson.M{"lock_id": defaultLockID, "acquired_at": time.Now()})
	if err != nil {
		t.Fatalf("failed to seed lock: %v", err)
	}

	err = engine.Up(ctx, "")
	if !errors.Is(err, ErrFailedToLock) {
		t.Fatalf("expected lock error, got %v", err)
	}
}

func TestEngineTransactionRollbackIntegration(t *testing.T) {
	t.Parallel()
	suite := newMongoSuite(t)
	defer suite.Close()

	if !supportsTransactions(suite.Client) {
		t.Skip("transactions not supported in test container")
	}

	mig := scriptMigration{
		version:     "20240210_fail_after_insert",
		description: "simulate failure",
		upFn: func(ctx context.Context, db *mongo.Database) error {
			if _, err := db.Collection("accounts").InsertOne(ctx, bson.M{"_id": "u1"}); err != nil {
				return err
			}
			return fmt.Errorf("boom")
		},
		downFn: func(ctx context.Context, db *mongo.Database) error {
			_, err := db.Collection("accounts").DeleteOne(ctx, bson.M{"_id": "u1"})
			return err
		},
	}

	engine := NewEngineWithMigrations(suite.DB, suite.CollName("schema_migrations"), map[string]Migration{mig.version: mig})
	ctx := context.Background()

	if err := engine.Up(ctx, ""); err == nil {
		t.Fatalf("expected migration failure")
	}

	count, _ := suite.DB.Collection("accounts").CountDocuments(ctx, bson.M{"_id": "u1"})
	if count != 0 {
		t.Fatalf("expected rollback to remove inserted doc, count=%d", count)
	}
}

func TestEngineContextCancellationIntegration(t *testing.T) {
	t.Parallel()
	suite := newMongoSuite(t)
	defer suite.Close()

	mig := scriptMigration{
		version:     "20240211_slow",
		description: "slow migration",
		upFn: func(ctx context.Context, db *mongo.Database) error {
			select {
			case <-time.After(200 * time.Millisecond):
				_, err := db.Collection("slow").InsertOne(ctx, bson.M{"_id": "done"})
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		downFn: func(ctx context.Context, db *mongo.Database) error {
			_, err := db.Collection("slow").DeleteOne(ctx, bson.M{"_id": "done"})
			return err
		},
	}

	engine := NewEngineWithMigrations(suite.DB, suite.CollName("schema_migrations"), map[string]Migration{mig.version: mig})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := engine.Up(ctx, ""); err == nil {
		t.Fatalf("expected context cancellation error")
	}
}

func TestEngineIdempotentUpIntegration(t *testing.T) {
	t.Parallel()
	suite := newMongoSuite(t)
	defer suite.Close()

	// Pre-create index to simulate a dirty database.
	if err := CreateIndexes(context.Background(), suite.DB.Collection("features"),
		Index(Asc("enabled")).Name("idx_features_enabled"),
	); err != nil {
		t.Fatalf("failed to pre-create index: %v", err)
	}

	mig := scriptMigration{
		version:     "20240220_add_enabled_index",
		description: "add enabled index",
		upFn: func(ctx context.Context, db *mongo.Database) error {
			return CreateIndexes(ctx, db.Collection("features"),
				Index(Asc("enabled")).Name("idx_features_enabled"),
			)
		},
		downFn: func(ctx context.Context, db *mongo.Database) error {
			return DropIndexes(ctx, db.Collection("features"), "idx_features_enabled")
		},
	}

	engine := NewEngineWithMigrations(suite.DB, suite.CollName("schema_migrations"), map[string]Migration{mig.version: mig})
	if err := engine.Up(context.Background(), ""); err != nil {
		t.Fatalf("expected idempotent up, got %v", err)
	}
}

// --- Helpers ---

type mongoSuite struct {
	T         *testing.T
	Client    *mongo.Client
	DB        *mongo.Database
	container *tcMongo.MongoDBContainer
	dbName    string
}

func newMongoSuite(t *testing.T) *mongoSuite {
	if containerErr != nil {
		t.Fatalf("mongo container unavailable: %v", containerErr)
	}
	dbName := fmt.Sprintf("test_%d", time.Now().UnixNano())
	return &mongoSuite{
		T:         t,
		Client:    sharedClient,
		DB:        sharedClient.Database(dbName),
		container: sharedMongo,
		dbName:    dbName,
	}
}

func (s *mongoSuite) CollName(base string) string {
	return fmt.Sprintf("%s_%s", base, s.dbName)
}

func (s *mongoSuite) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_ = s.DB.Drop(ctx)
}

type scriptMigration struct {
	version     string
	description string
	upFn        func(context.Context, *mongo.Database) error
	downFn      func(context.Context, *mongo.Database) error
}

func (m scriptMigration) Version() string     { return m.version }
func (m scriptMigration) Description() string { return m.description }
func (m scriptMigration) Up(ctx context.Context, db *mongo.Database) error {
	return m.upFn(ctx, db)
}
func (m scriptMigration) Down(ctx context.Context, db *mongo.Database) error {
	return m.downFn(ctx, db)
}

func TestEnginePlanTableDrivenIntegration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		applied  []string
		dir      Direction
		target   string
		expected []string
	}{
		{
			name:     "plan up with pending",
			applied:  []string{"20240101_add_flag"},
			dir:      DirectionUp,
			expected: []string{"20240102_enable_flag", "20240103_add_index"},
		},
		{
			name:     "plan down to target",
			applied:  []string{"20240101_add_flag", "20240102_enable_flag", "20240103_add_index"},
			dir:      DirectionDown,
			target:   "20240102_enable_flag",
			expected: []string{"20240103_add_index", "20240102_enable_flag"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			suite := newMongoSuite(t)
			defer suite.Close()

			migrations := map[string]Migration{
				"20240101_add_flag":    scriptMigration{version: "20240101_add_flag"},
				"20240102_enable_flag": scriptMigration{version: "20240102_enable_flag"},
				"20240103_add_index":   scriptMigration{version: "20240103_add_index"},
			}
			engine := NewEngineWithMigrations(suite.DB, suite.CollName("schema_migrations"), migrations)

			seedApplied(t, suite.DB.Collection(engine.coll), tt.applied)

			plan, err := engine.Plan(context.Background(), tt.dir, tt.target)
			if err != nil {
				t.Fatalf("plan failed: %v", err)
			}
			if len(plan) != len(tt.expected) {
				t.Fatalf("expected %d steps, got %d (%v)", len(tt.expected), len(plan), plan)
			}
			for i, expected := range tt.expected {
				if plan[i] != expected {
					t.Fatalf("expected plan[%d]=%s, got %s", i, expected, plan[i])
				}
			}
		})
	}
}
func supportsTransactions(client *mongo.Client) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.StartSession()
	if err != nil {
		return false
	}
	defer session.EndSession(ctx)
	err = session.StartTransaction()
	if err != nil {
		return !isTransactionNotSupported(err)
	}
	return true
}

func seedApplied(t *testing.T, coll *mongo.Collection, versions []string) {
	t.Helper()
	now := time.Now().UTC()
	for _, v := range versions {
		if _, err := coll.InsertOne(context.Background(), MigrationRecord{
			Version:     v,
			Description: v,
			AppliedAt:   now,
			Checksum:    "seed",
		}); err != nil {
			t.Fatalf("failed to seed applied migration %s: %v", v, err)
		}
	}
}
