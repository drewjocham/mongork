package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/drewjocham/mongork/internal/migration"
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
}
func (s *McpServer) handleStatus(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		return s.statusTableResult(ctx)
	})
}

func (s *McpServer) handlePlan(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		return s.statusTableResult(ctx)
	})
}

func (s *McpServer) handleUp(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return s.runVersionedMigration(
		ctx,
		req,
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
			return nil, fmt.Errorf("%w: %w", ErrFailedToListColl, err)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "### Database Schema: `%s`\n\n", s.db.Name())
		for _, name := range collections {
			s.appendCollectionSchema(&b, ctx, name)
		}
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
	required bool,
	wrapErr error,
	successMessage string,
	run func(context.Context, string) error,
) (*mcpsdk.CallToolResult, error) {
	return s.withConnection(ctx, func() (*mcpsdk.CallToolResult, error) {
		version, err := parseVersionArgument(requestArguments(req), required)
		if err != nil {
			return nil, err
		}
		if err := run(ctx, version); err != nil {
			return nil, fmt.Errorf("%w: %w", wrapErr, err)
		}
		return textResult(successMessage), nil
	})
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
