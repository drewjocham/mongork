package cli

import (
	"context"
	"errors"

	"github.com/drewjocham/mongork/internal/config"
	"github.com/drewjocham/mongork/internal/migration"
)

var (
	ErrServicesNotFound = errors.New("unable to retrieve services from context (check if command is running offline)")
	ErrEngineNotFound   = errors.New("migration engine not found in context")
	ErrConfigNotFound   = errors.New("config not found in context")
)

type ctxKey string

const (
	ctxServicesKey ctxKey = "services"
	ctxConfigKey   ctxKey = "config"
	ctxEngineKey   ctxKey = "engine"
)

func getServices(ctx context.Context) (*Services, error) {
	s, ok := ctx.Value(ctxServicesKey).(*Services)
	if !ok {
		return nil, ErrServicesNotFound
	}
	return s, nil
}

func getEngine(ctx context.Context) (*migration.Engine, error) {
	e, ok := ctx.Value(ctxEngineKey).(*migration.Engine)
	if !ok {
		return nil, ErrEngineNotFound
	}
	return e, nil
}

func getConfig(ctx context.Context) (*config.Config, error) {
	cfg, ok := ctx.Value(ctxConfigKey).(*config.Config)
	if !ok {
		return nil, ErrConfigNotFound
	}
	return cfg, nil
}
