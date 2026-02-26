package diff

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestCompareIndexesAndValidators(t *testing.T) {
	live := NewSchemaSpec()
	live.Indexes["users"] = map[string]IndexSpec{
		"idx_users_email": {
			Collection: "users",
			Name:       "idx_users_email",
			Keys:       bson.D{{Key: "email", Value: 1}},
		},
	}
	live.Validators["users"] = ValidatorSpec{
		Collection: "users",
		Schema:     bson.M{"$jsonSchema": bson.M{"bsonType": "object"}},
		Level:      "moderate",
	}

	target := NewSchemaSpec()
	target.Indexes["users"] = map[string]IndexSpec{
		"idx_users_email": {
			Collection: "users",
			Name:       "idx_users_email",
			Keys:       bson.D{{Key: "email", Value: 1}},
			Unique:     true,
		},
		"idx_users_created_at": {
			Collection: "users",
			Name:       "idx_users_created_at",
			Keys:       bson.D{{Key: "created_at", Value: -1}},
		},
	}

	diffs := Compare(live, target)
	if len(diffs) != 3 {
		t.Fatalf("expected 3 diffs, got %d: %+v", len(diffs), diffs)
	}

	assertHasDiff(t, diffs, "index", "UpdateIndex", "users.idx_users_email")
	assertHasDiff(t, diffs, "index", "AddIndex", "users.idx_users_created_at")
	assertHasDiff(t, diffs, "validator", "DropValidator", "users")
}

func assertHasDiff(t *testing.T, diffs []Diff, component, action, target string) {
	t.Helper()
	for _, d := range diffs {
		if d.Component == component && d.Action == action && d.Target == target {
			return
		}
	}
	t.Fatalf("missing diff %s %s %s", component, action, target)
}
