package migration

import "testing"

func TestRegisterMigrationTableDriven(t *testing.T) {
	cases := []struct {
		name      string
		migration Migration
		wantErr   bool
	}{
		{
			name: "valid migration",
			migration: &TestMigration{
				version:     "20240215_001_add_users",
				description: "add users",
			},
		},
		{
			name: "invalid version format",
			migration: &TestMigration{
				version:     "15-02-2024",
				description: "bad version",
			},
			wantErr: true,
		},
		{
			name: "duplicate version",
			migration: &TestMigration{
				version:     "20240215_002_duplicate",
				description: "dup",
			},
			wantErr: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			registryMu.Lock()
			registered = make(map[string]Migration)
			if tt.name == "duplicate version" {
				registered["20240215_002_duplicate"] = tt.migration
			}
			registryMu.Unlock()

			err := Register(tt.migration)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestIsValidVersionFormat(t *testing.T) {
	tests := []struct {
		version string
		valid   bool
	}{
		{"20240215", true},
		{"20240215_001", true},
		{"20240215_001_add_index", true},
		{"2024-02-15", false},
		{"202402", false},
	}

	for _, tt := range tests {
		if got := isValidVersionFormat(tt.version); got != tt.valid {
			t.Fatalf("version %s expected %v, got %v", tt.version, tt.valid, got)
		}
	}
}
