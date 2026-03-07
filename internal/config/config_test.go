package config

import (
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("MONGO_URL", "mongodb://testhost:27017")
	t.Setenv("MONGO_DATABASE", "testdb")
	t.Setenv("MONGO_MIGRATIONS_COLLECTION", "test_migrations")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	assert(t, cfg.Mongo.URL, "mongodb://testhost:27017", "Mongo.URL")
	assert(t, cfg.Mongo.Database, "testdb", "Mongo.Database")
	assert(t, cfg.Mongo.Collection, "test_migrations", "Mongo.Collection")
	assert(t, cfg.LogLevel.String(), "debug", "LogLevel")
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("MONGO_DATABASE", "default_test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	assert(t, cfg.Mongo.URL, "mongodb://localhost:27017", "Default Mongo.URL")
	assert(t, cfg.Mongo.Collection, "schema_migrations", "Default Mongo.Collection")
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "Valid Configuration",
			config: &Config{
				Mongo: MongoConfig{Database: "ok"},
			},
			wantErr: false,
		},
		{
			name: "Google Docs Enabled with path",
			config: &Config{
				Mongo:      MongoConfig{Database: "ok"},
				GoogleDocs: GoogleDocsConfig{Enabled: true, CredentialsPath: "/path/to/json"},
			},
			wantErr: false,
		},
		{
			name: "Google Docs Enabled but missing credentials",
			config: &Config{
				Mongo:      MongoConfig{Database: "ok"},
				GoogleDocs: GoogleDocsConfig{Enabled: true},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func assert(t *testing.T, got, want, field string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
