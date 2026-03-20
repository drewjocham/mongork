package diff

import (
	"context"
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func InspectLive(ctx context.Context, db *mongo.Database) (SchemaSpec, error) {
	spec := NewSchemaSpec()

	cur, err := db.ListCollections(ctx, bson.D{}, options.ListCollections().SetNameOnly(false))
	if err != nil {
		return spec, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			return spec, err
		}

		collName, ok := doc["name"].(string)
		if !ok {
			continue
		}
		spec.Collections[collName] = struct{}{}

		validator, level := parseValidator(doc)
		if len(validator) > 0 {
			spec.Validators[collName] = ValidatorSpec{
				Collection: collName,
				Schema:     validator,
				Level:      level,
			}
		}

		indexes, err := readIndexes(ctx, db.Collection(collName))
		if err != nil {
			return spec, err
		}
		if len(indexes) > 0 {
			spec.Indexes[collName] = indexes
		}
	}

	return spec, cur.Err()
}

func parseValidator(collDoc bson.M) (bson.M, string) {
	opts, _ := collDoc["options"].(bson.M)
	if opts == nil {
		return nil, "off"
	}

	validator, _ := opts["validator"].(bson.M)
	level, ok := opts["validationLevel"].(string)
	if !ok || level == "" {
		level = "off"
	}

	return validator, level
}

func readIndexes(ctx context.Context, coll *mongo.Collection) (map[string]IndexSpec, error) {
	cur, err := coll.Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	indexes := make(map[string]IndexSpec)
	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}

		name, _ := doc["name"].(string)
		if name == "" || name == "_id_" {
			continue
		}

		index := IndexSpec{
			Collection: coll.Name(),
			Name:       name,
			Keys:       toBsonD(doc["key"]),
		}

		if unique, ok := doc["unique"].(bool); ok {
			index.Unique = unique
		}
		if sparse, ok := doc["sparse"].(bool); ok {
			index.Sparse = sparse
		}
		if ttl, ok := toInt32(doc["expireAfterSeconds"]); ok {
			index.ExpireAfterSeconds = &ttl
		}
		if partial := toBsonD(doc["partialFilterExpression"]); len(partial) > 0 {
			index.PartialFilter = partial
		}

		indexes[name] = index
	}
	return indexes, cur.Err()
}

func toBsonD(value interface{}) bson.D {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case bson.D:
		return v
	case bson.M:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(bson.D, 0, len(keys))
		for _, k := range keys {
			out = append(out, bson.E{Key: k, Value: v[k]})
		}
		return out
	default:
		return nil
	}
}

func toInt32(value interface{}) (int32, bool) {
	switch v := value.(type) {
	case int32:
		return v, true
	case int64:
		return int32(v), true
	case int:
		return int32(v), true
	case float64:
		return int32(v), true
	default:
		return 0, false
	}
}

func describeIndex(idx IndexSpec) string {
	return fmt.Sprintf("%s (unique=%t sparse=%t ttl=%s partial=%s)",
		formatBsonD(idx.Keys),
		idx.Unique,
		idx.Sparse,
		ttlString(idx.ExpireAfterSeconds),
		formatBsonD(idx.PartialFilter))
}
