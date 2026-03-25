package main

import (
	"context"
	"fmt"
	"time"

	"github.com/drewjocham/mongork/pkg/desktop"
)

// App struct
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

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
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
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
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
