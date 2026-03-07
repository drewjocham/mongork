package migration

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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

type MigrationMetadata struct {
	ExecutionTime time.Duration `json:"execution_time" bson:"execution_time"`
	Error         string        `json:"error,omitempty" bson:"error,omitempty"`
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

type SchemaBuilder struct {
	data   bson.M
	isRoot bool
}

func Schema() *SchemaBuilder {
	return &SchemaBuilder{
		data:   bson.M{"bsonType": "object"},
		isRoot: true,
	}
}

func (s *SchemaBuilder) BsonType(t string) *SchemaBuilder {
	s.data["bsonType"] = t
	return s
}

func (s *SchemaBuilder) Required(fields ...string) *SchemaBuilder {
	if len(fields) > 0 {
		s.data["required"] = fields
	}
	return s
}

func (s *SchemaBuilder) Properties(props bson.M) *SchemaBuilder {
	s.data["properties"] = props
	return s
}

func (s *SchemaBuilder) Field(name string, value any) *SchemaBuilder {
	props, ok := s.data["properties"].(bson.M)
	if !ok {
		props = bson.M{}
		s.data["properties"] = props
	}
	props[name] = value
	return s
}

func (s *SchemaBuilder) String() bson.M { return bson.M{"bsonType": "string"} }
func (s *SchemaBuilder) Int() bson.M    { return bson.M{"bsonType": "int"} }
func (s *SchemaBuilder) Long() bson.M   { return bson.M{"bsonType": "long"} }
func (s *SchemaBuilder) Bool() bson.M   { return bson.M{"bsonType": "bool"} }
func (s *SchemaBuilder) Date() bson.M   { return bson.M{"bsonType": "date"} }
func (s *SchemaBuilder) Object(props bson.M) bson.M {
	return bson.M{"bsonType": "object", "properties": props}
}
func (s *SchemaBuilder) Array(items any) bson.M {
	return bson.M{"bsonType": "array", "items": items}
}

func (s *SchemaBuilder) Build() bson.M {
	if s.isRoot {
		s.isRoot = false
		return bson.M{"$jsonSchema": s.data}
	}
	return s.data
}

type IndexBuilder struct {
	model mongo.IndexModel
}

func Index(keys any) *IndexBuilder {
	return &IndexBuilder{
		model: mongo.IndexModel{Keys: keys},
	}
}

func (b *IndexBuilder) Name(name string) *IndexBuilder {
	if b.model.Options == nil {
		b.model.Options = options.Index()
	}
	b.model.Options.SetName(name)
	return b
}

func (b *IndexBuilder) Build() mongo.IndexModel {
	return b.model
}

func Asc(field string) bson.D  { return bson.D{{Key: field, Value: 1}} }
func Desc(field string) bson.D { return bson.D{{Key: field, Value: -1}} }
func Text(field string) bson.D { return bson.D{{Key: field, Value: "text"}} }

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

type CollectionOption func(*options.CreateCollectionOptionsBuilder)

func WithValidator(schema bson.M) CollectionOption {
	return func(o *options.CreateCollectionOptionsBuilder) { o.SetValidator(schema) }
}

func WithValidationLevel(level string) CollectionOption {
	return func(o *options.CreateCollectionOptionsBuilder) { o.SetValidationLevel(level) }
}

func EnsureCollection(
	ctx context.Context,
	db *mongo.Database,
	name string,
	opts ...CollectionOption,
) (*mongo.Collection, error) {
	o := options.CreateCollection()
	for _, opt := range opts {
		opt(o)
	}
	_ = db.CreateCollection(ctx, name, o)
	return db.Collection(name), nil
}
