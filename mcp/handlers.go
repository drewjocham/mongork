package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/drewjocham/mongork/internal/migration"
	"github.com/drewjocham/mongork/internal/observability"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	ErrMigrationUpFailed   = errors.New("migration up failed")
	ErrMigrationDownFailed = errors.New("migration down failed")
	ErrFailedToListColl    = errors.New("failed to list collections")
	ErrFailedToGetStatus   = errors.New("failed to get status")
)

const (
	optionalVersionDescription = "Optional target version"
	requiredVersionDescription = "The specific version to roll back"
)

type versionArgs struct {
	Version string `json:"version"`
}

func unmarshalArgs(req *mcpsdk.CallToolRequest, out any) error {
	raw := requestArguments(req)
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (s *McpServer) registerTools() {
	statusSchema := noArgsSchema()
	upSchema := optionalVersionSchema()
	downSchema := requiredVersionSchema()
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "migration_status",
		Description: "Check applied and pending migrations.",
		InputSchema: statusSchema,
	}, s.handleStatus)
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "migration_plan",
		Description: "Calculate which migrations are pending without executing them.",
		InputSchema: noArgsSchema(),
	}, s.handlePlan)
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "migration_up",
		Description: "Apply pending migrations.",
		InputSchema: upSchema,
	}, s.handleUp)
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "migration_down",
		Description: "Roll back migrations.",
		InputSchema: downSchema,
	}, s.handleDown)
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "database_schema",
		Description: "View collections and indexes.",
		InputSchema: noArgsSchema(),
	}, s.handleSchema)
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "db_health",
		Description: "Show database health report for operators.",
		InputSchema: noArgsSchema(),
	}, s.handleDBHealth)
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "db_collections",
		Description: "List collections in the active database.",
		InputSchema: noArgsSchema(),
	}, s.handleDBCollections)
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "db_indexes",
		Description: "List indexes (optionally filter by collection).",
		InputSchema: objectSchema(map[string]any{
			"collection": stringProperty("Optional collection filter"),
		}),
	}, s.handleDBIndexes)
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "db_collection_stats",
		Description: "Show collection storage and index size statistics.",
		InputSchema: objectSchema(map[string]any{
			"collection": stringProperty("Optional collection filter"),
		}),
	}, s.handleDBCollectionStats)
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "db_current_ops",
		Description: "List currently running MongoDB operations.",
		InputSchema: objectSchema(map[string]any{
			"limit": map[string]any{"type": "integer", "description": "Max operations to return (default 20, max 200)"},
		}),
	}, s.handleDBCurrentOps)
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "db_users",
		Description: "List Mongo users and roles.",
		InputSchema: noArgsSchema(),
	}, s.handleDBUsers)
	s.server.AddTool(&mcpsdk.Tool{
		Name:        "db_resource_summary",
		Description: "Show resource usage summary from serverStatus.",
		InputSchema: noArgsSchema(),
	}, s.handleDBResourceSummary)
}
func (s *McpServer) handleStatus(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		result, err := s.statusTableResult(ctx)
		recordToolResult("migration_status", "", err)
		return result, err
	})
}

func (s *McpServer) handleDBHealth(ctx context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		report, err := observability.BuildHealthReport(ctx, s.client, s.db.Name())
		recordToolResult("db_health", "", err)
		if err != nil {
			return nil, err
		}
		return jsonResult(report)
	})
}

func (s *McpServer) handleDBCollections(
	ctx context.Context,
	_ *mcpsdk.CallToolRequest,
) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		rows, err := observability.ListCollections(ctx, s.db)
		recordToolResult("db_collections", "", err)
		if err != nil {
			return nil, err
		}
		return jsonResult(rows)
	})
}

func (s *McpServer) handleDBIndexes(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		var args struct {
			Collection string `json:"collection"`
		}
		_ = unmarshalArgs(req, &args)
		rows, err := observability.ListIndexes(ctx, s.db, strings.TrimSpace(args.Collection))
		recordToolResult("db_indexes", args.Collection, err)
		if err != nil {
			return nil, err
		}
		return jsonResult(rows)
	})
}

func (s *McpServer) handleDBCollectionStats(
	ctx context.Context,
	req *mcpsdk.CallToolRequest,
) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		var args struct {
			Collection string `json:"collection"`
		}
		_ = unmarshalArgs(req, &args)
		rows, err := observability.CollectionStatistics(ctx, s.db, strings.TrimSpace(args.Collection))
		recordToolResult("db_collection_stats", args.Collection, err)
		if err != nil {
			return nil, err
		}
		return jsonResult(rows)
	})
}

func (s *McpServer) handleDBCurrentOps(
	ctx context.Context,
	req *mcpsdk.CallToolRequest,
) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		var args struct {
			Limit int `json:"limit"`
		}
		_ = unmarshalArgs(req, &args)
		if args.Limit <= 0 {
			args.Limit = 20
		}
		if args.Limit > 200 {
			args.Limit = 200
		}
		rows, err := observability.CurrentOperations(ctx, s.client, args.Limit)
		recordToolResult("db_current_ops", fmt.Sprintf("limit=%d", args.Limit), err)
		if err != nil {
			return nil, err
		}
		return jsonResult(rows)
	})
}

func (s *McpServer) handleDBUsers(ctx context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		rows, err := observability.Users(ctx, s.client)
		recordToolResult("db_users", "", err)
		if err != nil {
			return nil, err
		}
		return jsonResult(rows)
	})
}

func (s *McpServer) handleDBResourceSummary(
	ctx context.Context,
	_ *mcpsdk.CallToolRequest,
) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		summary, err := observability.BuildResourceSummary(ctx, s.client)
		recordToolResult("db_resource_summary", "", err)
		if err != nil {
			return nil, err
		}
		return jsonResult(summary)
	})
}

func (s *McpServer) handlePlan(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		result, err := s.statusTableResult(ctx)
		recordToolResult("migration_plan", "", err)
		return result, err
	})
}

func (s *McpServer) handleUp(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return s.runVersionedMigration(
		ctx,
		req,
		"migration_up",
		false,
		ErrMigrationUpFailed,
		"Migrations applied successfully.",
		s.engine.Up,
	)
}

func (s *McpServer) handleDown(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return s.runVersionedMigration(
		ctx,
		req,
		"migration_down",
		true,
		ErrMigrationDownFailed,
		"Rollback completed successfully.",
		s.engine.Down,
	)
}

func (s *McpServer) handleSchema(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		collections, err := s.db.ListCollectionNames(ctx, bson.D{})
		if err != nil {
			recordToolResult("database_schema", "", err)
			return nil, fmt.Errorf("%w: %w", ErrFailedToListColl, err)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "### Database Schema: `%s`\n\n", s.db.Name())
		for _, name := range collections {
			s.appendCollectionSchema(&b, ctx, name)
		}
		recordToolResult("database_schema", fmt.Sprintf("collections=%d", len(collections)), nil)
		return textResult(b.String()), nil
	})
}

func (s *McpServer) appendCollectionSchema(b *strings.Builder, ctx context.Context, name string) {
	fmt.Fprintf(b, "#### Collection: `%s`\n\n| Index Name | Keys | Unique |\n| :--- | :--- | :--- |\n", name)
	cursor, err := s.db.Collection(name).Indexes().List(ctx)
	if err != nil {
		fmt.Fprintf(b, "| *Error: %v* | | |\n\n", err)
		return
	}
	defer cursor.Close(ctx)
	var idxs []bson.M
	if err := cursor.All(ctx, &idxs); err != nil {
		fmt.Fprintf(b, "| *Error: %v* | | |\n\n", err)
		return
	}
	for _, idx := range idxs {
		unique := "No"
		if u, ok := idx["unique"].(bool); ok && u {
			unique = "Yes"
		}
		fmt.Fprintf(b, "| `%v` | `%s` | %s |\n", idx["name"], formatIndexKeys(idx["key"]), unique)
	}
	b.WriteString("\n")
}

func textResult(text string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: text}},
	}
}

func jsonResult(v any) (*mcpsdk.CallToolResult, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return textResult(strings.TrimSpace(b.String())), nil
}

func (s *McpServer) withConnection(
	ctx context.Context,
	run func() (*mcpsdk.CallToolResult, error),
) (*mcpsdk.CallToolResult, error) {
	if err := s.ensureConnection(ctx); err != nil {
		return nil, err
	}
	return run()
}

func (s *McpServer) statusTableResult(ctx context.Context) (*mcpsdk.CallToolResult, error) {
	status, err := s.engine.GetStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToGetStatus, err)
	}
	return textResult(migration.FormatStatusTable(status)), nil
}

func (s *McpServer) runVersionedMigration(
	ctx context.Context,
	req *mcpsdk.CallToolRequest,
	toolName string,
	required bool,
	wrapErr error,
	successMessage string,
	run func(context.Context, string) error,
) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		version, err := parseVersionArgument(requestArguments(req), required)
		if err != nil {
			recordToolResult(toolName, version, err)
			return nil, err
		}
		if err := run(ctx, version); err != nil {
			recordToolResult(toolName, version, err)
			return nil, fmt.Errorf("%w: %w", wrapErr, err)
		}
		recordToolResult(toolName, version, nil)
		return textResult(successMessage), nil
	})
}

func recordToolResult(tool, detail string, err error) {
	activity := Activity{
		Timestamp: time.Now().UTC(),
		Actor:     "MCP",
		Tool:      tool,
		Detail:    detail,
		Success:   err == nil,
	}
	if err != nil {
		activity.Error = err.Error()
	}
	recordActivity(activity)
}

func requestArguments(req *mcpsdk.CallToolRequest) json.RawMessage {
	if req == nil || req.Params == nil {
		return nil
	}
	return req.Params.Arguments
}

func parseVersionArgument(raw json.RawMessage, required bool) (string, error) {
	if len(raw) == 0 {
		if required {
			return "", fmt.Errorf("version is required")
		}
		return "", nil
	}

	var args versionArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	args.Version = strings.TrimSpace(args.Version)

	if required && args.Version == "" {
		return "", fmt.Errorf("version is required")
	}
	return args.Version, nil
}

func optionalVersionSchema() map[string]any {
	return versionSchema(optionalVersionDescription, false)
}

func requiredVersionSchema() map[string]any {
	return versionSchema(requiredVersionDescription, true)
}

func noArgsSchema() map[string]any {
	return objectSchema(nil)
}

func versionSchema(description string, required bool) map[string]any {
	if required {
		return objectSchema(map[string]any{
			"version": stringProperty(description),
		}, "version")
	}
	return objectSchema(map[string]any{
		"version": stringProperty(description),
	})
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{},
	}
	if len(properties) > 0 {
		schema["properties"] = cloneProperties(properties)
	}
	if len(required) > 0 {
		schema["required"] = append([]string(nil), required...)
	}
	return schema
}

func cloneProperties(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		if nested, ok := v.(map[string]any); ok {
			dst[k] = cloneProperties(nested)
			continue
		}
		dst[k] = v
	}
	return dst
}

func stringProperty(description string) map[string]any {
	prop := map[string]any{"type": "string"}
	if description != "" {
		prop["description"] = description
	}
	return prop
}
