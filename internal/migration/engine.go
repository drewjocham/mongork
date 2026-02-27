package migration

import (
	"context"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"log/slog"
	"sort"
	"time"

	"github.com/drewjocham/mongork/internal/config"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var (
	ErrLockAcquisitionFailed = errors.New("could not acquire migration lock; another process may be running")
)

type Engine struct {
	db     *mongo.Database
	logger *slog.Logger
	cfg    *config.Config
}

func NewEngine(db *mongo.Database, logger *slog.Logger, cfg *config.Config) *Engine {
	return &Engine{
		db:     db,
		logger: logger,
		cfg:    cfg,
	}
}

func (e *Engine) Up(ctx context.Context) error {
	if err := e.acquireLock(ctx); err != nil {
		return err
	}
	defer e.releaseLock(context.Background())

	applied, err := e.getAppliedVersions(ctx)
	if err != nil {
		return err
	}

	all := RegisteredMigrations()
	versions := e.sortedVersions(all)

	for _, v := range versions {
		if _, ok := applied[v]; ok {
			continue
		}

		e.logger.Info("applying migration", "version", v)
		m := all[v]

		if err := m.Up(ctx, e.db); err != nil {
			return fmt.Errorf("migration %s failed: %w", v, err)
		}

		if err := e.markApplied(ctx, m); err != nil {
			return err
		}
		e.logger.Info("migration successful", "version", v)
	}

	return nil
}

func (e *Engine) Plan(ctx context.Context) ([]MigrationStatus, error) {
	applied, err := e.getAppliedVersions(ctx)
	if err != nil {
		return nil, err
	}

	all := RegisteredMigrations()
	versions := e.sortedVersions(all)
	var pending []MigrationStatus

	for _, v := range versions {
		if _, exists := applied[v]; !exists {
			pending = append(pending, MigrationStatus{
				Version:     v,
				Description: all[v].Description(),
				Applied:     false,
			})
		}
	}
	return pending, nil
}

func (e *Engine) Down(ctx context.Context, targetVersion string) error {
	if err := e.acquireLock(ctx); err != nil {
		return err
	}
	defer e.releaseLock(context.Background())

	applied, err := e.getAppliedVersions(ctx)
	if err != nil {
		return err
	}

	if _, ok := applied[targetVersion]; !ok {
		return fmt.Errorf("version %s is not currently applied", targetVersion)
	}

	m, ok := RegisteredMigrations()[targetVersion]
	if !ok {
		return fmt.Errorf("migration code for %s not found in registry", targetVersion)
	}

	e.logger.Info("rolling back migration", "version", targetVersion)
	if err := m.Down(ctx, e.db); err != nil {
		return err
	}

	return e.markUnapplied(ctx, targetVersion)
}

func (e *Engine) Status(ctx context.Context) ([]MigrationStatus, error) {
	applied, err := e.getAppliedVersions(ctx)
	if err != nil {
		return nil, err
	}

	all := RegisteredMigrations()
	versions := e.sortedVersions(all)

	var res []MigrationStatus
	for _, v := range versions {
		_, isApplied := applied[v]
		res = append(res, MigrationStatus{
			Version:     v,
			Description: all[v].Description(),
			Applied:     isApplied,
		})
	}

	return res, nil
}

// --- Internal Helpers ---

func (e *Engine) acquireLock(ctx context.Context) error {
	coll := e.db.Collection(e.cfg.Mongo.Collection)
	doc := bson.M{
		"_id":        "migration_lock",
		"created_at": time.Now().UTC(),
		"owner":      "mongork_engine",
	}

	_, err := coll.InsertOne(ctx, doc)
	if mongo.IsDuplicateKeyError(err) {
		return ErrLockAcquisitionFailed
	}
	return err
}

func (e *Engine) releaseLock(ctx context.Context) {
	coll := e.db.Collection(e.cfg.Mongo.Collection)
	_, _ = coll.DeleteOne(ctx, bson.M{"_id": "migration_lock"})
}

func (e *Engine) getAppliedVersions(ctx context.Context) (map[string]bool, error) {
	coll := e.db.Collection(e.cfg.Mongo.Collection)

	cursor, err := coll.Find(ctx, bson.M{
		"version": bson.M{"$exists": true},
	})
	if err != nil {
		return nil, err
	}

	var results []MigrationStatus
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	applied := make(map[string]bool)
	for _, r := range results {
		applied[r.Version] = true
	}
	return applied, nil
}

func (e *Engine) markApplied(ctx context.Context, m Migration) error {
	coll := e.db.Collection(e.cfg.Mongo.Collection)
	now := time.Now().UTC()

	status := MigrationStatus{
		Version:     m.Version(),
		Description: m.Description(),
		Applied:     true,
		AppliedAt:   &now,
	}

	opts := options.UpdateOne().SetUpsert(true)
	_, err := coll.UpdateOne(ctx,
		bson.M{"version": m.Version()},
		bson.M{"$set": status},
		opts,
	)
	return err
}

func (e *Engine) markUnapplied(ctx context.Context, version string) error {
	coll := e.db.Collection(e.cfg.Mongo.Collection)
	_, err := coll.DeleteOne(ctx, bson.M{"version": version})
	return err
}

func (e *Engine) removeRecord(ctx context.Context, version string) error {
	coll := e.db.Collection(e.cfg.Mongo.Collection)
	_, err := coll.DeleteOne(ctx, bson.M{"version": version})
	return err
}

func (e *Engine) sortedVersions(m map[string]Migration) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
