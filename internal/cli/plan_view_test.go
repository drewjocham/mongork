package cli

import (
	"bytes"
	"testing"
)

func TestRenderPlan_Empty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer

	renderPlan(&buf, "up", nil)

	if got := buf.String(); got != "No migrations to up.\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRenderPlan_FormatsSteps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		direction string
		plan      []string
		expected  string
	}{
		{
			name:      "up plan",
			direction: "up",
			plan:      []string{"20240101_001", "20240101_002"},
			expected:  "Planned migrations to up:\n  01. 20240101_001\n  02. 20240101_002\n",
		},
		{
			name:      "down plan",
			direction: "down",
			plan:      []string{"20240101_002"},
			expected:  "Planned migrations to down:\n  01. 20240101_002\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderPlan(&buf, tt.direction, tt.plan)
			if got := buf.String(); got != tt.expected {
				t.Fatalf("unexpected output: %q", got)
			}
		})
	}
}
