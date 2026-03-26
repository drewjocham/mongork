package desktop

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/drewjocham/mongork/internal/config"
	"github.com/drewjocham/mongork/internal/migration"
	"github.com/drewjocham/mongork/internal/observability"
	"github.com/drewjocham/mongork/internal/schema"
	"github.com/drewjocham/mongork/internal/schema/diff"
	"github.com/drewjocham/mongork/mcp"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

type MigrationStatus struct {
	Version     string     `json:"version"`
	Description string     `json:"description"`
	Applied     bool       `json:"applied"`
	AppliedAt   *time.Time `json:"applied_at,omitempty"`
}

type OplogEntry map[string]interface{}

type HealthReport struct {
	Database    string            `json:"database"`
	Role        string            `json:"role"`
	OplogWindow string            `json:"oplog_window"`
	OplogSize   string            `json:"oplog_size"`
	Connections string            `json:"connections"`
	Lag         map[string]string `json:"lag,omitempty"`
	Warnings    []string          `json:"warnings,omitempty"`
}

type Diff struct {
	Component string `json:"component"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Current   string `json:"current"`
	Proposed  string `json:"proposed"`
	Risk      string `json:"risk"`
}

type MigrationRecord struct {
	Version     string    `json:"version"`
	Description string    `json:"description"`
	AppliedAt   time.Time `json:"applied_at"`
	Checksum    string    `json:"checksum"`
}

type IndexSpec struct {
	Collection         string `json:"collection"`
	Name               string `json:"name"`
	Keys               string `json:"keys"`
	Unique             bool   `json:"unique"`
	Sparse             bool   `json:"sparse"`
	PartialFilter      string `json:"partial_filter,omitempty"`
	ExpireAfterSeconds *int32 `json:"expire_after_seconds,omitempty"`
}

type Service struct {
	config         *config.Config
	engine         *migration.Engine
	mongoClient    *mongo.Client
	mu             sync.RWMutex
	migrationsPath string

	mcpMu         sync.RWMutex
	mcpServer     *mcp.McpServer
	mcpRunning    bool
	mcpTransport  string
	mcpListenAddr string
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Connect(ctx context.Context, connectionString, database, username, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.mongoClient != nil {
		_ = s.disconnectLocked()
	}

	cfg := &config.Config{
		Mongo: config.MongoConfig{
			URL:      connectionString,
			Database: database,
			Username: username,
			Password: password,
		},
		MigrationsCollection: "schema_migrations",
		MigrationsPath:       "./migrations",
	}

	client, err := s.dial(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	s.config = cfg
	s.mongoClient = client
	s.engine = migration.NewEngine(
		client.Database(cfg.Mongo.Database),
		cfg.MigrationsCollection,
	)
	s.engine.SetLogger(slog.Default())

	return nil
}

func (s *Service) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.disconnectLocked()
}

func (s *Service) disconnectLocked() error {
	if s.mongoClient == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.mongoClient.Disconnect(ctx)
	s.mongoClient = nil
	s.engine = nil
	s.config = nil
	_ = zap.L().Sync()
	return err
}

func (s *Service) GetStatus(ctx context.Context) ([]MigrationStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.engine == nil {
		return nil, errors.New("not connected")
	}
	internalStatus, err := s.engine.GetStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}
	result := make([]MigrationStatus, len(internalStatus))
	for i, status := range internalStatus {
		result[i] = MigrationStatus{
			Version:     status.Version,
			Description: status.Description,
			Applied:     status.Applied,
			AppliedAt:   status.AppliedAt,
		}
	}
	return result, nil
}

func (s *Service) Up(ctx context.Context, target string, dryRun bool) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.engine == nil {
		return "", errors.New("not connected")
	}
	if dryRun {
		plan, err := s.engine.Plan(ctx, migration.DirectionUp, target)
		if err != nil {
			return "", fmt.Errorf("dry-run failed: %w", err)
		}
		return fmt.Sprintf("Dry-run successful, would apply %d migrations", len(plan)), nil
	}
	if err := s.engine.Up(ctx, target); err != nil {
		return "", fmt.Errorf("up failed: %w", err)
	}
	return "Migrations applied successfully", nil
}

func (s *Service) Down(ctx context.Context, targetVersion string, dryRun bool) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.engine == nil {
		return "", errors.New("not connected")
	}
	if dryRun {
		plan, err := s.engine.Plan(ctx, migration.DirectionDown, targetVersion)
		if err != nil {
			return "", fmt.Errorf("dry-run failed: %w", err)
		}
		return fmt.Sprintf("Dry-run successful, would roll back %d migrations", len(plan)), nil
	}
	if err := s.engine.Down(ctx, targetVersion); err != nil {
		return "", fmt.Errorf("down failed: %w", err)
	}
	return fmt.Sprintf("Rolled back to version %s", targetVersion), nil
}

func (s *Service) GetOplog(ctx context.Context, limit int) ([]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.mongoClient == nil {
		return nil, errors.New("not connected")
	}
	coll := s.mongoClient.Database("local").Collection("oplog.rs")
	findOpts := options.Find().SetLimit(int64(limit)).SetSort(map[string]interface{}{"$natural": -1})
	cursor, err := coll.Find(ctx, map[string]interface{}{}, findOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to query oplog: %w", err)
	}
	defer cursor.Close(ctx)
	var results []interface{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode oplog entries: %w", err)
	}
	return results, nil
}

func (s *Service) GetSchemaDiff(ctx context.Context) ([]Diff, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.mongoClient == nil {
		return nil, errors.New("not connected")
	}
	live, err := diff.InspectLive(ctx, s.mongoClient.Database(s.config.Mongo.Database))
	if err != nil {
		return nil, fmt.Errorf("failed to inspect live schema: %w", err)
	}
	target := diff.FromRegistry()
	diffs := diff.Compare(live, target)
	result := make([]Diff, len(diffs))
	for i, d := range diffs {
		result[i] = Diff{
			Component: d.Component,
			Action:    d.Action,
			Target:    d.Target,
			Current:   d.Current,
			Proposed:  d.Proposed,
			Risk:      d.Risk,
		}
	}
	return result, nil
}

func (s *Service) GetSchemaIndexes(ctx context.Context) ([]IndexSpec, error) {
	indexes := schema.Indexes()
	result := make([]IndexSpec, len(indexes))
	for i, idx := range indexes {
		result[i] = IndexSpec{
			Collection:         idx.Collection,
			Name:               idx.Name,
			Keys:               idx.KeyString(),
			Unique:             idx.Unique,
			Sparse:             idx.Sparse,
			PartialFilter:      idx.PartialFilterString(),
			ExpireAfterSeconds: idx.ExpireAfterSeconds,
		}
	}
	return result, nil
}

func (s *Service) GetDBHealth(ctx context.Context) (HealthReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.mongoClient == nil {
		return HealthReport{}, errors.New("not connected")
	}
	report, err := observability.BuildHealthReport(ctx, s.mongoClient, s.config.Mongo.Database)
	if err != nil {
		return HealthReport{}, fmt.Errorf("failed to build health report: %w", err)
	}
	return HealthReport{
		Database:    report.Database,
		Role:        report.Role,
		OplogWindow: report.OplogWindow,
		OplogSize:   report.OplogSize,
		Connections: report.Connections,
		Lag:         report.Lag,
		Warnings:    report.Warnings,
	}, nil
}

func (s *Service) GetOpslog(ctx context.Context, search, version,
	regexStr, from, to string, limit int) ([]MigrationRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.engine == nil {
		return nil, errors.New("not connected")
	}
	records, err := s.engine.ListApplied(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list applied migrations: %w", err)
	}

	var fromTime, toTime time.Time
	if from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			fromTime = t
		}
	}
	if to != "" {
		if t, err := time.Parse("2006-01-02", to); err == nil {
			toTime = t.Add(24*time.Hour - time.Nanosecond)
		}
	}

	var re *regexp.Regexp
	if regexStr != "" {
		re, _ = regexp.Compile(regexStr)
	}

	var filtered []MigrationRecord
	searchLower := strings.ToLower(search)

	for _, rec := range records {
		if version != "" && rec.Version != version {
			continue
		}
		if search != "" {
			if !strings.Contains(strings.ToLower(rec.Version), searchLower) &&
				!strings.Contains(strings.ToLower(rec.Description), searchLower) {
				continue
			}
		}
		if re != nil && !re.MatchString(rec.Description) {
			continue
		}
		if !fromTime.IsZero() && rec.AppliedAt.Before(fromTime) {
			continue
		}
		if !toTime.IsZero() && rec.AppliedAt.After(toTime) {
			continue
		}
		filtered = append(filtered, MigrationRecord{
			Version:     rec.Version,
			Description: rec.Description,
			AppliedAt:   rec.AppliedAt,
			Checksum:    rec.Checksum,
		})
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (s *Service) CreateMigration(ctx context.Context, name string) (string, error) {
	s.mu.RLock()
	pathStr := s.migrationsPath
	if pathStr == "" && s.config != nil {
		pathStr = s.config.MigrationsPath
	}
	if pathStr == "" {
		pathStr = "./migrations"
	}
	s.mu.RUnlock()

	absPath, err := filepath.Abs(pathStr)
	if err != nil {
		absPath = pathStr
	}

	gen := &migration.Generator{OutputPath: absPath}
	path, version, err := gen.Create(name)
	if err != nil {
		return "", fmt.Errorf("failed to create migration: %w", err)
	}
	return fmt.Sprintf("Migration created: %s (version: %s)", path, version), nil
}

// SetMigrationsPath sets the output directory for generated migrations.
func (s *Service) SetMigrationsPath(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.migrationsPath = path
	return nil
}

// GetMigrationsPath returns the configured migrations output directory.
func (s *Service) GetMigrationsPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.migrationsPath != "" {
		return s.migrationsPath
	}
	if s.config != nil {
		return s.config.MigrationsPath
	}
	return "./migrations"
}

func (s *Service) dial(cfg *config.Config) (*mongo.Client, error) {
	opts := options.Client().
		ApplyURI(cfg.GetConnectionString()).
		SetMaxPoolSize(uint64(cfg.Mongo.MaxPoolSize)).
		SetMinPoolSize(uint64(cfg.Mongo.MinPoolSize))

	if cfg.Mongo.SSLEnabled {
		opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: cfg.Mongo.SSLInsecure})
	}

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}

	// Use a fresh context independent of the Wails app context, which can be
	// cancelled by the runtime before the ping completes.
	pingCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping %s: %w", cfg.GetConnectionString(), err)
	}

	return client, nil
}

func (s *Service) StartMCPServer(ctx context.Context, transport, listenAddr string) (string, error) {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()

	if s.mcpRunning {
		return "", errors.New("MCP server already running")
	}

	s.mu.RLock()
	cfg := s.config
	client := s.mongoClient
	s.mu.RUnlock()

	if cfg == nil || client == nil {
		return "", errors.New("not connected to MongoDB")
	}

	server, err := mcp.NewMCPServer(cfg, slog.Default())
	if err != nil {
		return "", fmt.Errorf("failed to create MCP server: %w", err)
	}

	s.mcpServer = server
	s.mcpTransport = transport
	s.mcpListenAddr = listenAddr
	s.mcpRunning = true

	go func() {
		var err error
		switch transport {
		case "stdio":
			err = server.StartStdio()
		case "http":
			err = server.StartHTTP(listenAddr, "/mcp")
		default:
			err = fmt.Errorf("unsupported transport: %s", transport)
		}

		s.mcpMu.Lock()
		if err != nil && !isClosingError(err) {
			slog.Error("MCP server error", "err", err)
		}
		s.mcpRunning = false
		s.mcpServer = nil
		s.mcpMu.Unlock()
	}()

	msg := fmt.Sprintf("MCP server started with %s transport", transport)
	if transport == "http" {
		msg = fmt.Sprintf("%s on %s", msg, listenAddr)
	}
	return msg, nil
}

func (s *Service) StopMCPServer(ctx context.Context) (string, error) {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()

	if !s.mcpRunning || s.mcpServer == nil {
		return "", errors.New("MCP server not running")
	}

	s.mcpServer.Close(ctx)
	s.mcpRunning = false
	s.mcpServer = nil

	return "MCP server stopped", nil
}

func (s *Service) GetMCPServerStatus(ctx context.Context) (map[string]interface{}, error) {
	s.mcpMu.RLock()
	defer s.mcpMu.RUnlock()

	status := map[string]interface{}{
		"running":    s.mcpRunning,
		"transport":  s.mcpTransport,
		"listenAddr": s.mcpListenAddr,
		"status":     "stopped",
	}

	if s.mcpRunning {
		status["status"] = "running"
	}

	return status, nil
}

func (s *Service) GetMCPActivity(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	activities := mcp.RecentActivity(limit)
	result := make([]map[string]interface{}, len(activities))
	for i, activity := range activities {
		result[i] = map[string]interface{}{
			"timestamp": activity.Timestamp.Format(time.RFC3339),
			"actor":     activity.Actor,
			"tool":      activity.Tool,
			"detail":    activity.Detail,
			"success":   activity.Success,
			"error":     activity.Error,
		}
	}
	return result, nil
}

func isClosingError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return msg == "EOF" ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "closed pipe")
}
