package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	collLock      = "schema_migrations_lock"
	defaultLockID = "migration_engine_lock"
)

var (
	ErrFailedToLock     = errors.New("could not acquire migration lock; another process may be running")
	ErrChecksumMismatch = errors.New("migration checksum mismatch")
	ErrUnknownMigration = errors.New("migration not registered")
)

// MigrationRecord captures an applied migration entry for opslog output.
type MigrationRecord struct {
	Version     string    `json:"version" bson:"version"`
	Description string    `json:"description" bson:"description"`
	AppliedAt   time.Time `json:"applied_at" bson:"applied_at"`
	Checksum    string    `json:"checksum" bson:"checksum"`
}

type Engine struct {
	db         *mongo.Database
	coll       string
	migrations map[string]Migration
	logger     *slog.Logger
}

func NewEngine(db *mongo.Database, collection string, migrations map[string]Migration) *Engine {
	cloned := make(map[string]Migration, len(migrations))
	for k, v := range migrations {
		cloned[k] = v
	}
	engine := &Engine{
		db:         db,
		coll:       collection,
		migrations: cloned,
		logger:     slog.Default(),
	}
	if engine.logger == nil {
		engine.logger = slog.New(slog.NewTextHandler(ioDiscard{}, nil))
	}
	return engine
}

// SetLogger overrides the engine logger.
func (e *Engine) SetLogger(logger *slog.Logger) {
	if logger != nil {
		e.logger = logger
	}
}

func (e *Engine) collection() *mongo.Collection {
	return e.db.Collection(e.coll)
}

func (e *Engine) lockCollection() *mongo.Collection {
	return e.db.Collection(collLock)
}

func (e *Engine) Up(ctx context.Context, target string) error {
	if err := e.acquireLock(ctx); err != nil {
		return err
	}
	defer e.releaseLock(context.Background())

	if err := e.validateChecksums(ctx); err != nil {
		return err
	}

	plan, err := e.Plan(ctx, DirectionUp, target)
	if err != nil {
		return err
	}

	for _, version := range plan {
		m, ok := e.migrations[version]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownMigration, version)
		}

		e.logger.Info("applying migration", "version", version)
		if err := m.Up(ctx, e.db); err != nil {
			return fmt.Errorf("migration %s failed: %w", version, err)
		}
		if err := e.markApplied(ctx, m); err != nil {
			return err
		}
		e.logger.Info("migration applied", "version", version)
	}

	return nil
}

func (e *Engine) Down(ctx context.Context, target string) error {
	if err := e.acquireLock(ctx); err != nil {
		return err
	}
	defer e.releaseLock(context.Background())

	plan, err := e.Plan(ctx, DirectionDown, target)
	if err != nil {
		return err
	}

	for _, version := range plan {
		m, ok := e.migrations[version]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownMigration, version)
		}
		e.logger.Info("rolling back migration", "version", version)
		if err := m.Down(ctx, e.db); err != nil {
			return fmt.Errorf("rollback %s failed: %w", version, err)
		}
		if err := e.removeRecord(ctx, version); err != nil {
			return err
		}
		e.logger.Info("migration rolled back", "version", version)
	}
	return nil
}

// Plan returns the ordered list of versions that would be executed for the given direction.
func (e *Engine) Plan(ctx context.Context, direction Direction, target string) ([]string, error) {
	applied, err := e.getAppliedMap(ctx)
	if err != nil {
		return nil, err
	}

	all := e.sortedVersions()
	switch direction {
	case DirectionUp:
		return e.planUp(all, applied, target)
	case DirectionDown:
		return e.planDown(all, applied, target)
	default:
		return nil, fmt.Errorf("%w: %s", ErrNotSupported{Operation: "plan"}, direction.String())
	}
}

// GetStatus returns a full snapshot of registered migrations and their status.
func (e *Engine) GetStatus(ctx context.Context) ([]MigrationStatus, error) {
	applied, err := e.getAppliedMap(ctx)
	if err != nil {
		return nil, err
	}

	var status []MigrationStatus
	for _, version := range e.sortedVersions() {
		record, isApplied := applied[version]
		entry := MigrationStatus{
			Version:     version,
			Description: e.migrations[version].Description(),
			Applied:     isApplied,
		}
		if isApplied {
			appliedAt := record.AppliedAt
			entry.AppliedAt = &appliedAt
		}
		status = append(status, entry)
	}
	return status, nil
}

func (e *Engine) ListApplied(ctx context.Context) ([]MigrationRecord, error) {
	opts := options.Find().SetSort(bson.D{{Key: "applied_at", Value: -1}})
	cursor, err := e.collection().Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []MigrationRecord
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (e *Engine) Force(ctx context.Context, version string) error {
	m, ok := e.migrations[version]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownMigration, version)
	}
	return e.markApplied(ctx, m)
}

// ForceUnlock removes the migration lock document without executing migrations.
func (e *Engine) ForceUnlock(ctx context.Context) error {
	_, err := e.lockCollection().DeleteOne(ctx, bson.M{"lock_id": defaultLockID})
	return err
}

// --- helpers ---

func (e *Engine) acquireLock(ctx context.Context) error {
	doc := bson.M{
		"lock_id":     defaultLockID,
		"acquired_at": time.Now().UTC(),
	}
	_, err := e.lockCollection().InsertOne(ctx, doc)
	if mongo.IsDuplicateKeyError(err) {
		return ErrFailedToLock
	}
	return err
}

func (e *Engine) releaseLock(ctx context.Context) {
	_, _ = e.lockCollection().DeleteOne(ctx, bson.M{"lock_id": defaultLockID})
}

func (e *Engine) getAppliedMap(ctx context.Context) (map[string]MigrationRecord, error) {
	cursor, err := e.collection().Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	records := make(map[string]MigrationRecord)
	for cursor.Next(ctx) {
		var rec MigrationRecord
		if err := cursor.Decode(&rec); err != nil {
			return nil, err
		}
		records[rec.Version] = rec
	}
	return records, cursor.Err()
}

func (e *Engine) markApplied(ctx context.Context, m Migration) error {
	now := time.Now().UTC()
	record := MigrationRecord{
		Version:     m.Version(),
		Description: m.Description(),
		AppliedAt:   now,
		Checksum:    checksumFor(m),
	}
	opts := options.UpdateOne().SetUpsert(true)
	_, err := e.collection().UpdateOne(
		ctx,
		bson.M{"version": m.Version()},
		bson.M{"$set": record},
		opts,
	)
	return err
}

func (e *Engine) removeRecord(ctx context.Context, version string) error {
	_, err := e.collection().DeleteOne(ctx, bson.M{"version": version})
	return err
}

func (e *Engine) validateChecksums(ctx context.Context) error {
	records, err := e.ListApplied(ctx)
	if err != nil {
		return err
	}
	for _, rec := range records {
		m, ok := e.migrations[rec.Version]
		if !ok {
			return fmt.Errorf("%w: %s", ErrUnknownMigration, rec.Version)
		}
		if rec.Checksum == "" {
			continue
		}
		if checksumFor(m) != rec.Checksum {
			return fmt.Errorf("%w: %s", ErrChecksumMismatch, rec.Version)
		}
	}
	return nil
}

func checksumFor(m Migration) string {
	hash := sha256.Sum256([]byte(m.Version() + "|" + m.Description()))
	return hex.EncodeToString(hash[:])
}

func (e *Engine) sortedVersions() []string {
	versions := make([]string, 0, len(e.migrations))
	for version := range e.migrations {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	return versions
}

func (e *Engine) planUp(all []string, applied map[string]MigrationRecord, target string) ([]string, error) {
	var plan []string
	for _, version := range all {
		if _, ok := applied[version]; ok {
			continue
		}
		plan = append(plan, version)
		if target != "" && version == target {
			break
		}
	}
	if target != "" {
		found := false
		for _, version := range plan {
			if version == target {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("target %s is not pending", target)
		}
	}
	return plan, nil
}

func (e *Engine) planDown(all []string, applied map[string]MigrationRecord, target string) ([]string, error) {
	var plan []string
	targetSeen := target == ""

	for i := len(all) - 1; i >= 0; i-- {
		version := all[i]
		if _, ok := applied[version]; !ok {
			continue
		}
		if target != "" && version == target {
			targetSeen = true
			break
		}
		plan = append(plan, version)
	}

	if target != "" && !targetSeen {
		return nil, fmt.Errorf("target %s is not applied", target)
	}

	return plan, nil
}

// ioDiscard implements slog.TextHandler writer when no logger is configured.
type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
