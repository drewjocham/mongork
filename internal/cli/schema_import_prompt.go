package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/drewjocham/mongork/internal/config"
	"github.com/drewjocham/mongork/internal/schema"
	"github.com/drewjocham/mongork/internal/schema/diff"
	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func maybePromptSchemaImport(ctx context.Context, cmd *cobra.Command, cfg *config.Config, db *mongo.Database) error {
	if cmd == nil || cfg == nil || db == nil {
		return nil
	}
	if !isInteractiveCommand(cmd) {
		return nil
	}

	live, err := diff.InspectLive(ctx, db)
	if err != nil {
		return err
	}
	cache, err := loadSchemaImportCache(cfg.Mongo.MigrationsPath)
	if err != nil {
		return err
	}
	applySchemaImportCache(cache, live)
	target := diff.FromRegistry()

	untrackedCollections := findUntrackedCollections(live, target, cfg.Mongo.Collection)
	untrackedIndexes := findUntrackedIndexes(live, target)
	if len(untrackedCollections) == 0 && len(untrackedIndexes) == 0 {
		return nil
	}

	fmt.Fprintf(
		cmd.OutOrStdout(),
		"\nDetected %d untracked collection(s) and %d untracked index(es).\n",
		len(untrackedCollections),
		len(untrackedIndexes),
	)
	if len(untrackedCollections) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Collections: %s\n", strings.Join(untrackedCollections, ", "))
	}
	if len(untrackedIndexes) > 0 {
		preview := make([]string, 0, len(untrackedIndexes))
		for _, idx := range untrackedIndexes {
			preview = append(preview, idx.Collection+"."+idx.Name)
		}
		sort.Strings(preview)
		fmt.Fprintf(cmd.OutOrStdout(), "Indexes: %s\n", strings.Join(preview, ", "))
	}

	if !promptConfirmation(cmd, "Import and version these schema items now? [y/N]: ") {
		return nil
	}

	for _, c := range untrackedCollections {
		if err := schema.RegisterCollection(c); err != nil {
			return err
		}
	}
	for _, idx := range untrackedIndexes {
		if err := schema.Register(idx); err != nil {
			return err
		}
	}

	path, err := writeImportedSchemaFile(cfg.Mongo.MigrationsPath, untrackedCollections, untrackedIndexes)
	if err != nil {
		return err
	}
	cache = mergeSchemaImportCache(cache, untrackedCollections, untrackedIndexes)
	if err := saveSchemaImportCache(cfg.Mongo.MigrationsPath, cache); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Imported schema metadata written to %s\n", path)
	return nil
}

func findUntrackedCollections(live, target diff.SchemaSpec, migrationCollection string) []string {
	ignored := map[string]struct{}{
		migrationCollection:           {},
		migrationCollection + "_lock": {},
		"schema_migrations_lock":      {},
	}
	out := make([]string, 0)
	for collection := range live.Collections {
		if _, skip := ignored[collection]; skip || strings.HasPrefix(collection, "system.") {
			continue
		}
		if _, tracked := target.Collections[collection]; !tracked {
			out = append(out, collection)
		}
	}
	sort.Strings(out)
	return out
}

func findUntrackedIndexes(live, target diff.SchemaSpec) []schema.IndexSpec {
	out := make([]schema.IndexSpec, 0)
	for collection, indexes := range live.Indexes {
		for name, idx := range indexes {
			if _, tracked := target.Indexes[collection][name]; tracked {
				continue
			}
			out = append(out, schema.IndexSpec{
				Collection:         idx.Collection,
				Name:               idx.Name,
				Keys:               idx.Keys,
				Unique:             idx.Unique,
				Sparse:             idx.Sparse,
				PartialFilter:      idx.PartialFilter,
				ExpireAfterSeconds: idx.ExpireAfterSeconds,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Collection == out[j].Collection {
			return out[i].Name < out[j].Name
		}
		return out[i].Collection < out[j].Collection
	})
	return out
}

func writeImportedSchemaFile(basePath string, collections []string, indexes []schema.IndexSpec) (string, error) {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%s_import_schema_metadata.go", time.Now().UTC().Format("20060102_150405"))
	fullPath := filepath.Join(basePath, filename)

	var b strings.Builder
	b.WriteString("package migrations\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"github.com/drewjocham/mongork/internal/schema\"\n")
	if len(indexes) > 0 {
		b.WriteString("\t\"go.mongodb.org/mongo-driver/v2/bson\"\n")
	}
	b.WriteString(")\n\n")
	b.WriteString("func init() { //nolint:gochecknoinits // generated schema import\n")
	if len(collections) > 0 {
		b.WriteString("\tschema.MustRegisterCollections(\n")
		for _, collection := range collections {
			b.WriteString(fmt.Sprintf("\t\t%q,\n", collection))
		}
		b.WriteString("\t)\n")
	}
	if len(indexes) > 0 {
		b.WriteString("\tschema.MustRegister(\n")
		for _, idx := range indexes {
			b.WriteString("\t\tschema.IndexSpec{\n")
			b.WriteString(fmt.Sprintf("\t\t\tCollection: %q,\n", idx.Collection))
			b.WriteString(fmt.Sprintf("\t\t\tName: %q,\n", idx.Name))
			b.WriteString("\t\t\tKeys: ")
			b.WriteString(renderBsonD(idx.Keys))
			b.WriteString(",\n")
			if idx.Unique {
				b.WriteString("\t\t\tUnique: true,\n")
			}
			if idx.Sparse {
				b.WriteString("\t\t\tSparse: true,\n")
			}
			if idx.ExpireAfterSeconds != nil {
				b.WriteString(fmt.Sprintf("\t\t\tExpireAfterSeconds: int32Ptr(%d),\n", *idx.ExpireAfterSeconds))
			}
			if len(idx.PartialFilter) > 0 {
				b.WriteString("\t\t\tPartialFilter: ")
				b.WriteString(renderBsonD(idx.PartialFilter))
				b.WriteString(",\n")
			}
			b.WriteString("\t\t},\n")
		}
		b.WriteString("\t)\n")
	}
	if hasTTL(indexes) {
		b.WriteString("}\n\n")
		b.WriteString("func int32Ptr(v int32) *int32 { return &v }\n")
	} else {
		b.WriteString("}\n")
	}

	if err := os.WriteFile(fullPath, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return fullPath, nil
}

func hasTTL(indexes []schema.IndexSpec) bool {
	for _, idx := range indexes {
		if idx.ExpireAfterSeconds != nil {
			return true
		}
	}
	return false
}

func renderBsonD(doc bson.D) string {
	if len(doc) == 0 {
		return "bson.D{}"
	}
	var b strings.Builder
	b.WriteString("bson.D{")
	for _, elem := range doc {
		b.WriteString("{Key: ")
		b.WriteString(fmt.Sprintf("%q", elem.Key))
		b.WriteString(", Value: ")
		b.WriteString(renderGoValue(elem.Value))
		b.WriteString("}, ")
	}
	b.WriteString("}")
	return b.String()
}

func renderBsonM(doc bson.M) string {
	if len(doc) == 0 {
		return "bson.M{}"
	}
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("bson.M{")
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("%q: %s, ", k, renderGoValue(doc[k])))
	}
	b.WriteString("}")
	return b.String()
}

func renderGoValue(v any) string {
	switch t := v.(type) {
	case string:
		return fmt.Sprintf("%q", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int32:
		return fmt.Sprintf("int32(%d)", t)
	case int64:
		return fmt.Sprintf("int64(%d)", t)
	case float64:
		return fmt.Sprintf("%v", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case bson.D:
		return renderBsonD(t)
	case bson.M:
		return renderBsonM(t)
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, renderGoValue(item))
		}
		return "[]any{" + strings.Join(parts, ", ") + "}"
	case nil:
		return "nil"
	default:
		return fmt.Sprintf("%#v", t)
	}
}

func isInteractiveCommand(cmd *cobra.Command) bool {
	inFile, inOK := cmd.InOrStdin().(*os.File)
	outFile, outOK := cmd.OutOrStdout().(*os.File)
	if !inOK || !outOK {
		return false
	}
	inStat, err := inFile.Stat()
	if err != nil {
		return false
	}
	outStat, err := outFile.Stat()
	if err != nil {
		return false
	}
	return (inStat.Mode()&os.ModeCharDevice) != 0 && (outStat.Mode()&os.ModeCharDevice) != 0
}
