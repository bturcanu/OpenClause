package approvals

import (
	"testing"
)

func TestMatchResource(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		resource string
		want     bool
	}{
		{"empty pattern matches everything", "", "anything", true},
		{"star matches everything", "*", "anything", true},
		{"exact match", "channel-1", "channel-1", true},
		{"exact mismatch", "channel-1", "channel-2", false},
		{"glob star", "channel-*", "channel-general", true},
		{"glob question mark", "ch?nnel", "channel", true},
		{"no partial/substring fallback", "admin", "not-admin-panel", false},
		{"complex glob", "projects/*/issues", "projects/PROJ/issues", true},
		{"malformed pattern returns false", "[invalid", "anything", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchResource(tt.pattern, tt.resource)
			if got != tt.want {
				t.Errorf("matchResource(%q, %q) = %v, want %v", tt.pattern, tt.resource, got, tt.want)
			}
		})
	}
}
