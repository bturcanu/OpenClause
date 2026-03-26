package onboarding

import (
	"fmt"
	"path/filepath"
	"strings"
)

func validateArtifactPath(raw string) (string, error) {
	rel := strings.TrimSpace(raw)
	if rel == "" {
		return "", fmt.Errorf("artifact path required")
	}

	rel = filepath.Clean(rel)
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("artifact path %q must be relative", raw)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact path %q must stay within the output directory", raw)
	}
	return rel, nil
}
