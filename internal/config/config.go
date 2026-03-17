package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/containerd/log"
	"github.com/joho/godotenv"
)

var (
	ErrEnvParse           = errors.New("env parse error")
	ErrGoogleCredsMissing = errors.New("google docs enabled but credentials missing")
)

type Config struct {
	Mongo      MongoConfig      `envPrefix:"MONGO_"`
	GoogleDocs GoogleDocsConfig `envPrefix:"GOOGLE_"`
	LogLevel   log.Level        `env:"LOG_LEVEL" envDefault:"info"`
	Timeout    time.Duration    `env:"TIMEOUT" envDefault:"60s"`
	LogFile    string           `env:"LOG_FILE" envDefault:"mcp.log"`
}

type MongoConfig struct {
	URL            string `env:"DATABASE_URL" envDefault:"mongodb://localhost:27017"`
	Database       string `env:"DATABASE_NAME,required"`
	MigrationsPath string `env:"MIGRATIONS_PATH" envDefault:"./migrations"`
	Collection     string `env:"MIGRATIONS_COLLECTION" envDefault:"schema_migrations"`
	Username       string `env:"USERNAME"`
	Password       string `env:"PASSWORD"`
	AuthSource     string `env:"AUTH_SOURCE" envDefault:"admin"`
	SSLEnabled     bool   `env:"SSL_ENABLED" envDefault:"false"`
	SSLInsecure    bool   `env:"SSL_INSECURE" envDefault:"false"`
	MaxPoolSize    int    `env:"MAX_POOL_SIZE" envDefault:"10"`
	MinPoolSize    int    `env:"MIN_POOL_SIZE" envDefault:"1"`
}

type GoogleDocsConfig struct {
	Enabled         bool   `env:"DOCS_ENABLED" envDefault:"false"`
	CredentialsPath string `env:"CREDENTIALS_PATH"`
	CredentialsJSON string `env:"CREDENTIALS_JSON"`
}

func Load(envFiles ...string) (*Config, error) {
	for _, file := range envFiles {
		if _, err := os.Stat(file); err == nil {
			_ = godotenv.Load(file)
		}
	}

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEnvParse, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.GoogleDocs.Enabled {
		if c.GoogleDocs.CredentialsPath == "" && c.GoogleDocs.CredentialsJSON == "" {
			return ErrGoogleCredsMissing
		}
	}
	return nil
}

func (c *Config) GetConnectionString() string {
	u, err := url.Parse(c.Mongo.URL)
	if err != nil {
		return c.Mongo.URL
	}
	if u.Path == "" {
		u.Path = "/"
	}

	if c.Mongo.Username != "" && u.User == nil {
		u.User = url.UserPassword(c.Mongo.Username, c.Mongo.Password)
	}

	q := u.Query()

	if strings.Contains(u.Host, "localhost") && !q.Has("connect") {
		q.Set("connect", "direct")
	}

	if c.Mongo.AuthSource != "" && !q.Has("authSource") {
		q.Set("authSource", c.Mongo.AuthSource)
	}

	if c.Mongo.SSLEnabled && !q.Has("ssl") {
		q.Set("ssl", "true")
	}

	u.RawQuery = q.Encode()
	return u.String()
}
