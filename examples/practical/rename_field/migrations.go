package migration

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

var (
	versionPattern = regexp.MustCompile(`^\d{8}(?:_[a-z0-9_]+)*$`)

	ErrMigrationNil        = errors.New("migration must not be nil")
	ErrInvalidVersionFmt   = errors.New("invalid version format: expected YYYYMMDD[_slug]")
	ErrMigrationRegistered = errors.New("migration already registered")

	registryMu sync.RWMutex
	registered = make(map[string]Migration)
)

type Migration interface {
	Version() string
	Description() string
	Up(ctx context.Context, db *mongo.Database) error
	Down(ctx context.Context, db *mongo.Database) error
}

type MigrationStatus struct {
	Version     string     `json:"version" bson:"version"`
	Description string     `json:"description" bson:"description"`
	Applied     bool       `json:"applied" bson:"applied"`
	AppliedAt   *time.Time `json:"applied_at,omitempty" bson:"applied_at,omitempty"`
}

func Register(m Migration) error {
	if m == nil {
		return ErrMigrationNil
	}

	version := m.Version()
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("%w: %s", ErrInvalidVersionFmt, version)
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registered[version]; exists {
		return fmt.Errorf("%w: %s", ErrMigrationRegistered, version)
	}

	registered[version] = m
	return nil
}

func MustRegister(ms ...Migration) {
	for _, m := range ms {
		if err := Register(m); err != nil {
			panic(fmt.Sprintf("migration registration failed: %v", err))
		}
	}
}

func RegisteredMigrations() map[string]Migration {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return maps.Clone(registered)
}

type MigrationFilter func(version string, m Migration) bool

func GetMigrations(filters ...MigrationFilter) map[string]Migration {
	registryMu.RLock()
	defer registryMu.RUnlock()

	results := make(map[string]Migration)
	for v, m := range registered {
		if matchesAll(v, m, filters) {
			results[v] = m
		}
	}
	return results
}

func matchesAll(v string, m Migration, filters []MigrationFilter) bool {
	for _, filter := range filters {
		if !filter(v, m) {
			return false
		}
	}
	return true
}

func CreateIndexes(ctx context.Context, coll *mongo.Collection, models ...mongo.IndexModel) error {
	_, err := coll.Indexes().CreateMany(ctx, models)
	return err
}

func DropIndexes(ctx context.Context, coll *mongo.Collection, names ...string) error {
	for _, name := range names {
		if err := coll.Indexes().DropOne(ctx, name); err != nil {
			return err
		}
	}
	return nil
}
