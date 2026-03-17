package schema

import (
	"context"
	"errors"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func ImportIndexesFromMongo(ctx context.Context, db *mongo.Database) (int, error) {
	if db == nil {
		return 0, nil
	}

	collections, err := db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return 0, err
	}

	imported := 0
	for _, collectionName := range collections {
		cur, err := db.Collection(collectionName).Indexes().List(ctx)
		if err != nil {
			return imported, err
		}

		for cur.Next(ctx) {
			var doc bson.M
			if err := cur.Decode(&doc); err != nil {
				_ = cur.Close(ctx)
				return imported, err
			}

			name, _ := doc["name"].(string)
			if name == "" || name == "_id_" {
				continue
			}

			spec := IndexSpec{
				Collection: collectionName,
				Name:       name,
				Keys:       toBsonD(doc["key"]),
			}
			if len(spec.Keys) == 0 {
				continue
			}

			if unique, ok := doc["unique"].(bool); ok {
				spec.Unique = unique
			}
			if sparse, ok := doc["sparse"].(bool); ok {
				spec.Sparse = sparse
			}
			if ttl, ok := toInt32(doc["expireAfterSeconds"]); ok {
				spec.ExpireAfterSeconds = &ttl
			}
			if partial := toBsonD(doc["partialFilterExpression"]); len(partial) > 0 {
				spec.PartialFilter = partial
			}

			if err := Register(spec); err != nil {
				if errors.Is(err, ErrIndexAlreadyRegistered) {
					continue
				}
				_ = cur.Close(ctx)
				return imported, err
			} else {
				imported++
			}
		}

		if err := cur.Err(); err != nil {
			_ = cur.Close(ctx)
			return imported, err
		}
		_ = cur.Close(ctx)
	}

	return imported, nil
}

func toBsonD(value any) bson.D {
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

func toInt32(value any) (int32, bool) {
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
