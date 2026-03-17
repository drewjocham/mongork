package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/drewjocham/mongork/internal/schema"
	"github.com/drewjocham/mongork/internal/schema/diff"
)

const schemaImportCacheFile = ".schema_import_cache.json"

type schemaImportCache struct {
	Collections      []string           `json:"collections"`
	IndexSpecs       []schema.IndexSpec `json:"index_specs"`
	LegacyIndexNames []string           `json:"indexes,omitempty"` // legacy cache shape: collection.index_name
}

func loadSchemaImportCache(basePath string) (schemaImportCache, error) {
	path := filepath.Join(basePath, schemaImportCacheFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return schemaImportCache{}, nil
		}
		return schemaImportCache{}, err
	}

	var cache schemaImportCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return schemaImportCache{}, err
	}
	cache.Collections = uniqueSorted(cache.Collections)
	cache.IndexSpecs = dedupeIndexSpecs(cache.IndexSpecs)
	cache.LegacyIndexNames = uniqueSorted(cache.LegacyIndexNames)
	return cache, nil
}

func saveSchemaImportCache(basePath string, cache schemaImportCache) error {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return err
	}

	cache.Collections = uniqueSorted(cache.Collections)
	cache.IndexSpecs = dedupeIndexSpecs(cache.IndexSpecs)
	cache.LegacyIndexNames = uniqueSorted(cache.LegacyIndexNames)

	path := filepath.Join(basePath, schemaImportCacheFile)
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func mergeSchemaImportCache(base schemaImportCache, collections []string, indexes []schema.IndexSpec) schemaImportCache {
	merged := schemaImportCache{
		Collections:      append([]string{}, base.Collections...),
		IndexSpecs:       append([]schema.IndexSpec{}, base.IndexSpecs...),
		LegacyIndexNames: append([]string{}, base.LegacyIndexNames...),
	}
	merged.Collections = append(merged.Collections, collections...)
	merged.IndexSpecs = append(merged.IndexSpecs, indexes...)
	merged.Collections = uniqueSorted(merged.Collections)
	merged.IndexSpecs = dedupeIndexSpecs(merged.IndexSpecs)
	merged.LegacyIndexNames = uniqueSorted(merged.LegacyIndexNames)
	return merged
}

func applySchemaImportCache(cache schemaImportCache, live diff.SchemaSpec) {
	for _, collection := range cache.Collections {
		_ = schema.RegisterCollection(collection)
	}

	for _, idx := range cache.IndexSpecs {
		_ = schema.Register(idx)
		_ = schema.RegisterCollection(idx.Collection)
	}

	for _, full := range cache.LegacyIndexNames {
		collection, name, ok := splitIndexFullName(full)
		if !ok {
			continue
		}
		liveIndexes := live.Indexes[collection]
		if len(liveIndexes) == 0 {
			continue
		}
		liveIndex, exists := liveIndexes[name]
		if !exists {
			continue
		}
		_ = schema.Register(schema.IndexSpec{
			Collection:         liveIndex.Collection,
			Name:               liveIndex.Name,
			Keys:               liveIndex.Keys,
			Unique:             liveIndex.Unique,
			Sparse:             liveIndex.Sparse,
			PartialFilter:      liveIndex.PartialFilter,
			ExpireAfterSeconds: liveIndex.ExpireAfterSeconds,
		})
		_ = schema.RegisterCollection(liveIndex.Collection)
	}
}

func splitIndexFullName(full string) (string, string, bool) {
	i := strings.Index(full, ".")
	if i <= 0 || i >= len(full)-1 {
		return "", "", false
	}
	return full[:i], full[i+1:], true
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		set[v] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func dedupeIndexSpecs(values []schema.IndexSpec) []schema.IndexSpec {
	byKey := make(map[string]schema.IndexSpec, len(values))
	for _, v := range values {
		if v.Collection == "" || v.Name == "" {
			continue
		}
		byKey[v.Collection+"."+v.Name] = v
	}
	out := make([]schema.IndexSpec, 0, len(byKey))
	for _, v := range byKey {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Collection == out[j].Collection {
			return out[i].Name < out[j].Name
		}
		return out[i].Collection < out[j].Collection
	})
	return out
}
