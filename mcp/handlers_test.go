package mcp

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseVersionArgument(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      json.RawMessage
		required bool
		want     string
		wantErr  bool
	}{
		{
			name:     "optionalMissing",
			required: false,
			want:     "",
			wantErr:  false,
		},
		{
			name:     "optionalProvided",
			raw:      json.RawMessage([]byte("{\"version\":\"  20240101_001  \"}")),
			required: false,
			want:     "20240101_001",
			wantErr:  false,
		},
		{
			name:     "requiredMissing",
			required: true,
			wantErr:  true,
		},
		{
			name:     "requiredBlank",
			raw:      json.RawMessage([]byte("{\"version\":\"   \"}")),
			required: true,
			wantErr:  true,
		},
		{
			name:     "invalidJSON",
			raw:      json.RawMessage([]byte("{\"version\":{}}")),
			required: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseVersionArgument(tt.raw, tt.required)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseVersionArgument() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseVersionArgument() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestObjectSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		props    map[string]any
		required []string
		want     map[string]any
	}{
		{
			name:  "noProperties",
			props: nil,
			want: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           map[string]any{},
			},
		},
		{
			name:  "withPropertiesAndRequired",
			props: map[string]any{"version": stringProperty("desc")},
			required: []string{
				"version",
			},
			want: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"version": stringProperty("desc"),
				},
				"required": []string{"version"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := objectSchema(tt.props, tt.required...)
			if !reflect.DeepEqual(tt.want, got) {
				t.Fatalf("objectSchema() = %#v, want %#v", got, tt.want)
			}
			if len(tt.props) > 0 {
				gotProps := got["properties"].(map[string]any)
				gotVersion := gotProps["version"].(map[string]any)
				gotVersion["type"] = "number"
				original := tt.props["version"].(map[string]any)["type"]
				if original != "string" {
					t.Fatalf("objectSchema() mutated input properties")
				}
			}
		})
	}
}
