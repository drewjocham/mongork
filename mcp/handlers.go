package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/drewjocham/mongork/internal/migration"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	ErrMigrationUpFailed   = errors.New("migration up failed")
	ErrMigrationDownFailed = errors.New("migration down failed")
	ErrFailedToListColl    = errors.New("failed to list collections")
	ErrFailedToGetStatus   = errors.New("failed to get status")
)

type versionArgs struct {
	Version string `json:"version"`
}

func (s *McpServer) registerTools() {
	// In the latest SDK, AddTool takes the Tool struct and the handler function directly.
	// If this still fails, ensure your McpServer struct uses the correct pointer type.

	s.McpServer.AddTool(&mcp.Tool{
		Name:        "migration_status",
		Description: "Check applied and pending migrations.",
	}, s.handleStatus)

	s.McpServer.AddTool(&mcp.Tool{
		Name:        "migration_plan",
		Description: "Calculate which migrations are pending without executing them.",
	}, s.handlePlan)

	s.McpServer.AddTool(&mcp.Tool{
		Name:        "migration_up",
		Description: "Apply pending migrations.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"version": map[string]any{"type": "string", "description": "Optional target version"},
			},
		},
	}, s.handleUp)

	s.McpServer.AddTool(&mcp.Tool{
		Name:        "migration_down",
		Description: "Roll back migrations.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"version": map[string]any{"type": "string", "description": "The specific version to roll back"},
			},
			Required: []string{"version"},
		},
	}, s.handleDown)

	s.McpServer.AddTool(&mcp.Tool{
		Name:        "database_schema",
		Description: "View collections and indexes.",
	}, s.handleSchema)
}

func (s *McpServer) handleStatus(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.ensureConnection(ctx); err != nil {
		return nil, err
	}
	status, err := s.engine.GetStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToGetStatus, err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Text: migration.FormatStatusTable(status)}},
	}, nil
}

func (s *McpServer) handlePlan(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.ensureConnection(ctx); err != nil {
		return nil, err
	}
	pending, err := s.engine.Plan(ctx)
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Text: migration.FormatStatusTable(pending)}},
	}, nil
}

func (s *McpServer) handleUp(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.ensureConnection(ctx); err != nil {
		return nil, err
	}
	var args versionArgs
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
	}
	if err := s.engine.Up(ctx, args.Version); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMigrationUpFailed, err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Text: "Migrations applied successfully."}},
	}, nil
}

func (s *McpServer) handleDown(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.ensureConnection(ctx); err != nil {
		return nil, err
	}
	var args versionArgs
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}
	if err := s.engine.Down(ctx, args.Version); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMigrationDownFailed, err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Text: "Rollback completed successfully."}},
	}, nil
}

func (s *McpServer) handleSchema(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.ensureConnection(ctx); err != nil {
		return nil, err
	}
	collections, err := s.db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToListColl, err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### Database Schema: `%s`\n\n", s.db.Name())
	for _, name := range collections {
		s.appendCollectionSchema(&b, ctx, name)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Text: b.String()}},
	}, nil
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
		return
	}
	for _, idx := range idxs {
		unique := "No"
		if u, ok := idx["unique"].(bool); ok && u {
			unique = "Yes"
		}
		fmt.Fprintf(b, "| `%v` | `%v` | %s |\n", idx["name"], idx["key"], unique)
	}
	b.WriteString("\n")
}
