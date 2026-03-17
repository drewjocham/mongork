package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/drewjocham/mongork/internal/config"
	migrate "github.com/drewjocham/mongork/internal/migration"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	ErrConfigRequired       = errors.New("config is required")
	ErrFailedToConnect      = errors.New("failed to connect to mongodb")
	ErrServerAlreadyRunning = errors.New("mcp server already running")
	ErrFailedToDisconnect   = errors.New("failed to disconnect mongo client")
)

type McpServer struct {
	mu     sync.RWMutex
	server *mcpsdk.Server
	engine *migrate.Engine
	db     *mongo.Database
	client *mongo.Client
	config *config.Config
	cancel context.CancelFunc
	logger *slog.Logger
}

func NewMCPServer(cfg *config.Config, logger *slog.Logger) (*McpServer, error) {
	if cfg == nil {
		return nil, ErrConfigRequired
	}

	s := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "mongork",
		Version: "1.0.0",
	}, &mcpsdk.ServerOptions{})

	srv := &McpServer{
		server: s,
		config: cfg,
		logger: logger,
	}

	srv.registerTools()
	return srv, nil
}

func (s *McpServer) ensureConnection(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		if err := s.client.Ping(ctx, nil); err == nil {
			return nil
		}
	}

	client, err := mongo.Connect(options.Client().ApplyURI(s.config.Mongo.URL))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToConnect, err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return fmt.Errorf("%w: %w", ErrFailedToConnect, err)
	}

	s.client = client
	s.db = client.Database(s.config.Mongo.Database)
	engine := migrate.NewEngine(s.db, s.config.Mongo.Collection)
	engine.SetLogger(s.logger)
	s.engine = engine

	s.logger.Info("connected to mongodb", "database", s.config.Mongo.Database)
	return nil
}

func (s *McpServer) Start() error {
	return s.StartStdio()
}

func (s *McpServer) StartStdio() error {
	return s.runWithSignalContext(func(ctx context.Context) error {
		s.logger.Info("starting mcp server", "transport", "stdio")
		return s.ServeStdio(ctx, os.Stdin, os.Stdout)
	})
}

func (s *McpServer) StartHTTP(listenAddr string, basePath string) error {
	if strings.TrimSpace(listenAddr) == "" {
		listenAddr = "0.0.0.0:8080"
	}
	basePath = strings.TrimSpace(basePath)
	if basePath == "" {
		basePath = "/mcp"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}

	return s.runWithSignalContext(func(ctx context.Context) error {
		s.logger.Info("starting mcp server", "transport", "http", "listen", listenAddr, "base_path", basePath)
		return s.ServeHTTP(ctx, listenAddr, basePath)
	})
}

func (s *McpServer) runWithSignalContext(run func(context.Context) error) error {
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
	return run(ctx)
}

func (s *McpServer) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	return s.ServeStdio(ctx, r, w)
}

func (s *McpServer) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	return s.server.Run(ctx, &mcpsdk.IOTransport{
		Reader: io.NopCloser(r),
		Writer: nopWriteCloser{Writer: w},
	})
}

func (s *McpServer) ServeHTTP(ctx context.Context, listenAddr string, basePath string) error {
	handler := mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server {
		return s.server
	}, &mcpsdk.StreamableHTTPOptions{})
	mux := http.NewServeMux()
	mux.Handle(basePath, handler)

	httpServer := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *McpServer) Close(ctx context.Context) error {
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
