package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/drewjocham/mongork/pkg/desktop"
)

type App struct {
	ctx     context.Context
	service *desktop.Service
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		service: desktop.NewService(),
	}
}

// The context is saved so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown is called when the app closes
func (a *App) shutdown(ctx context.Context) {
	if a.service != nil {
		_ = a.service.Disconnect()
	}
}

// Connect establishes a connection to MongoDB
func (a *App) Connect(connectionString, database, username, password string) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	err := a.service.Connect(ctx, connectionString, database, username, password)
	if err != nil {
		return "", fmt.Errorf("connection failed: %w", err)
	}
	return "Connected successfully", nil
}

// Disconnect closes the MongoDB connection
func (a *App) Disconnect() (string, error) {
	err := a.service.Disconnect()
	if err != nil {
		return "", fmt.Errorf("disconnect failed: %w", err)
	}
	return "Disconnected successfully", nil
}

// GetStatus returns the current migration status
func (a *App) GetStatus() ([]desktop.MigrationStatus, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.GetStatus(ctx)
}

// Up applies pending migrations
func (a *App) Up(target string, dryRun bool) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.Up(ctx, target, dryRun)
}

// Down rolls back migrations to a target version
func (a *App) Down(targetVersion string, dryRun bool) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.Down(ctx, targetVersion, dryRun)
}

// GetOplog retrieves recent oplog entries
func (a *App) GetOplog(limit int) ([]interface{}, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.GetOplog(ctx, limit)
}

// GetSchemaDiff returns schema differences between registered and live MongoDB
func (a *App) GetSchemaDiff() ([]desktop.Diff, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.GetSchemaDiff(ctx)
}

// GetSchemaIndexes returns registered index specifications
func (a *App) GetSchemaIndexes() ([]desktop.IndexSpec, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.GetSchemaIndexes(ctx)
}

// GetDBHealth returns database health report
func (a *App) GetDBHealth() (desktop.HealthReport, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.GetDBHealth(ctx)
}

// GetOpslog returns applied migration records with filtering
func (a *App) GetOpslog(search, version, regex, from, to string, limit int) ([]desktop.MigrationRecord, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.GetOpslog(ctx, search, version, regex, from, to, limit)
}

// CreateMigration creates a new migration file
func (a *App) CreateMigration(name string) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.CreateMigration(ctx, name)
}

// StartMCPServer starts the MCP server
func (a *App) StartMCPServer(transport, listenAddr string) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.StartMCPServer(ctx, transport, listenAddr)
}

// StopMCPServer stops the MCP server
func (a *App) StopMCPServer() (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.StopMCPServer(ctx)
}

// GetMCPServerStatus returns MCP server status
func (a *App) GetMCPServerStatus() (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.GetMCPServerStatus(ctx)
}

// GetMCPActivity returns recent MCP activity
func (a *App) GetMCPActivity(limit int) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.service.GetMCPActivity(ctx, limit)
}

// SaveConnection saves a named connection to disk
func (a *App) SaveConnection(conn SavedConnection) error {
	return upsertConnection(conn)
}

// LoadConnections returns all saved connections
func (a *App) LoadConnections() ([]SavedConnection, error) {
	return loadConnections()
}

// DeleteConnection removes a saved connection by name
func (a *App) DeleteConnection(name string) error {
	return removeConnection(name)
}

// ParseConnectionString parses a MongoDB URI and returns its components
func (a *App) ParseConnectionString(uri string) map[string]string {
	result := map[string]string{
		"url":      uri,
		"database": "",
		"username": "",
		"password": "",
	}
	u, err := url.Parse(uri)
	if err != nil {
		return result
	}
	if u.User != nil {
		result["username"] = u.User.Username()
		result["password"], _ = u.User.Password()
		u.User = nil
		result["url"] = u.String()
	}
	if path := strings.TrimPrefix(u.Path, "/"); path != "" {
		result["database"] = path
		u.Path = "/"
		result["url"] = u.String()
	}
	return result
}

// GetAIKey returns the stored Anthropic API key
func (a *App) GetAIKey() (string, error) {
	s, err := loadSettings()
	if err != nil {
		return "", err
	}
	return s.AIKey, nil
}

// SetAIKey persists the Anthropic API key
func (a *App) SetAIKey(key string) error {
	s, err := loadSettings()
	if err != nil {
		s = &appSettings{}
	}
	s.AIKey = key
	return persistSettings(s)
}

// AskAI sends a question to Claude and returns the answer
func (a *App) AskAI(question string) (string, error) {
	s, err := loadSettings()
	if err != nil || s.AIKey == "" {
		return "", fmt.Errorf("no API key configured — add your Anthropic API key in the AI tab")
	}
	system := "You are an expert MongoDB database assistant helping developers manage migrations, schema design, and database health using the MongoRK tool. Be concise and practical."
	return askMongork(s.AIKey, system, question)
}

// SetMigrationsPath configures the output path for new migrations
func (a *App) SetMigrationsPath(path string) error {
	return a.service.SetMigrationsPath(path)
}

// GetMigrationsPath returns the current migrations output path
func (a *App) GetMigrationsPath() string {
	return a.service.GetMigrationsPath()
}
