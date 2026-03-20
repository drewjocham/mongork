package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/drewjocham/mongork/internal/config"
	"github.com/drewjocham/mongork/internal/jsonutil"
)

var ErrRenderConfig = errors.New("failed to render configuration")

type safeConfig struct {
	Mongo                safeMongoConfig      `json:"mongo"`
	GoogleDocs           safeGoogleDocsConfig `json:"google_docs"`
	LogLevel             string               `json:"log_level"`
	Timeout              string               `json:"timeout"`
	MigrationsPath       string               `json:"migrations_path"`
	MigrationsCollection string               `json:"migrations_collection"`
}

type safeMongoConfig struct {
	URL         string `json:"url"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	AuthSource  string `json:"auth_source"`
	SSLEnabled  bool   `json:"ssl_enabled"`
	SSLInsecure bool   `json:"ssl_insecure"`
	MaxPoolSize int    `json:"max_pool_size"`
	MinPoolSize int    `json:"min_pool_size"`
}

type safeGoogleDocsConfig struct {
	Enabled         bool   `json:"enabled"`
	CredentialsPath string `json:"credentials_path"`
	CredentialsJSON string `json:"credentials_json"`
}

func renderConfig(out io.Writer, cfg *config.Config) error {
	enc := jsonutil.NewEncoder(out)
	enc.SetIndent("", "  ")

	safe := safeConfig{
		LogLevel:             cfg.LogLevel.String(),
		Timeout:              cfg.Timeout.String(),
		MigrationsPath:       cfg.MigrationsPath,
		MigrationsCollection: cfg.MigrationsCollection,
		Mongo: safeMongoConfig{
			URL:         cfg.Mongo.URL,
			Database:    cfg.Mongo.Database,
			Username:    cfg.Mongo.Username,
			Password:    maskSecret(cfg.Mongo.Password),
			AuthSource:  cfg.Mongo.AuthSource,
			SSLEnabled:  cfg.Mongo.SSLEnabled,
			SSLInsecure: cfg.Mongo.SSLInsecure,
			MaxPoolSize: cfg.Mongo.MaxPoolSize,
			MinPoolSize: cfg.Mongo.MinPoolSize,
		},
		GoogleDocs: safeGoogleDocsConfig{
			Enabled:         cfg.GoogleDocs.Enabled,
			CredentialsPath: maskSecret(cfg.GoogleDocs.CredentialsPath),
			CredentialsJSON: maskSecret(cfg.GoogleDocs.CredentialsJSON),
		},
	}

	if err := enc.Encode(safe); err != nil {
		return fmt.Errorf("%w: %w", ErrRenderConfig, err)
	}
	return nil
}

func maskSecret(v string) string {
	if v == "" {
		return ""
	}
	if len(v) < 8 {
		return "********"
	}
	return fmt.Sprintf("%s***%s", v[:2], v[len(v)-2:])
}
