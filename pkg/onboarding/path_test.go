package onboarding

import (
	"strings"
	"testing"
)

func TestValidateArtifactPath(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{
			name:  "nested relative path",
			input: "examples/python/agent.py",
			want:  "examples/python/agent.py",
		},
		{
			name:    "reject empty path",
			input:   "   ",
			wantErr: "artifact path required",
		},
		{
			name:    "reject traversal",
			input:   "../escape.sh",
			wantErr: `artifact path "../escape.sh" must stay within the output directory`,
		},
		{
			name:    "reject absolute path",
			input:   "/tmp/escape.sh",
			wantErr: `artifact path "/tmp/escape.sh" must be relative`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateArtifactPath(tc.input)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateArtifactPath(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
