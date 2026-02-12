package mcp

import (
	"context"
	"errors"
	"fmt"
	migrate "github.com/drewjocham/mongork/internal/migration"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/drewjocham/mongork/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	ErrConfigRequired       = errors.New("config is required")
	ErrFailedToConnect      = errors.New("failed to connect to mongodb")
	ErrServerAlreadyRunning = errors.New("mcp server already running")
	ErrFailedToDisconnect   = errors.New("failed to disconnect mongo client")
	ErrUnsupportedOutput    = errors.New("unsupported output format")
	ErrInvalidRegex         = errors.New("invalid regex")
	ErrFailedToReadOpsLog   = errors.New("failed to read opslog")
	ErrInvalidTime          = errors.New("invalid time")
	ErrFailedToMarshalJSON  = errors.New("failed to marshal json")
)

type MCPServer struct {
	mu        sync.RWMutex
	mcpServer *mcp.Server
	engine    *migrate.Engine
	db        *mongo.Database
	client    *mongo.Client
	config    *config.Config
	cancel    context.CancelFunc
	logger    *slog.Logger
}

func NewMCPServer(cfg *config.Config, logger *slog.Logger) (*MCPServer, error) {
	if cfg == nil {
		return nil, ErrConfigRequired
	}

	s := mcp.NewServer(&mcp.Implementation{
		Name:    "mongork",
		Version: "1.0.0",
	}, nil)

	srv := &MCPServer{
		mcpServer: s,
		config:    cfg,
		logger:    logger,
	}

	srv.registerTools()
	return srv, nil
}

func (s *MCPServer) ensureConnection(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		if err := s.client.Ping(ctx, nil); err == nil {
			return nil
		}
	}

	client, err := mongo.Connect(options.Client().ApplyURI(s.config.MongoURL))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToConnect, err)
	}

	s.client = client
	s.db = client.Database(s.config.Database)
	s.engine = migrate.NewEngine(s.db, s.config.MigrationsCollection, migrate.RegisteredMigrations())

	s.logger.Info("connected to mongodb", "database", s.config.Database)
	return nil
}

func (s *MCPServer) Start() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		stop()
		return ErrServerAlreadyRunning
	}
	s.cancel = stop
	s.mu.Unlock()

	defer func() {
		stop()
		s.mu.Lock()
		s.cancel = nil
		s.mu.Unlock()
	}()

	s.logger.Info("starting mcp server")
	return s.Serve(ctx, os.Stdin, os.Stdout)
}

func (s *MCPServer) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	return s.mcpServer.Run(ctx, &mcp.IOTransport{
		Reader: io.NopCloser(r),
		Writer: nopWriteCloser{Writer: w},
	})
}

func (s *MCPServer) Close(ctx context.Context) error {
	s.mu.Lock()
	client := s.client
	s.client = nil
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	var errs []error
	if client != nil {
		if err := client.Disconnect(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%w: %w", ErrFailedToDisconnect, err))
		}
	}

	return errors.Join(errs...)
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }
