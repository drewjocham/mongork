package migration

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestEnsureIndexNameAppliesPrefixSuffix(t *testing.T) {
	model := mongo.IndexModel{Keys: bson.D{{Key: "email", Value: 1}}}
	cfg := &indexCreateConfig{namePrefix: "pfx_", nameSuffix: "_sfx"}

	if err := ensureIndexName(&model, cfg); err != nil {
		t.Fatalf("ensureIndexName returned error: %v", err)
	}
	applied, err := buildIndexOptions(model.Options)
	if err != nil {
		t.Fatalf("buildIndexOptions returned error: %v", err)
	}
	if applied.Name == nil {
		t.Fatalf("expected index name to be set, got %+v", applied)
	}
	name := *applied.Name
	if name[:4] != "pfx_" || name[len(name)-4:] != "_sfx" {
		t.Fatalf("unexpected name %s", name)
	}
}

func TestBuildIndexBaseName(t *testing.T) {
	keys := bson.D{{Key: "email", Value: 1}, {Key: "created_at", Value: -1}}
	if got := buildIndexBaseName(keys); got != "email_1_created_at_-1" {
		t.Fatalf("unexpected base name %s", got)
	}
}

func TestBuildIndexOptionsSuffix(t *testing.T) {
	opts := options.Index().SetUnique(true).SetSparse(true).SetExpireAfterSeconds(3600)
	applied, err := buildIndexOptions(opts)
	if err != nil {
		t.Fatalf("buildIndexOptions returned error: %v", err)
	}
	if got := buildIndexOptionsSuffix(applied); got != "sparse_ttl_3600_unique" {
		t.Fatalf("unexpected suffix %s", got)
	}
}
