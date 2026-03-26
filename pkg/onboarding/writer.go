package onboarding

import (
	"fmt"
	"os"
	"path/filepath"
)

type WrittenArtifact struct {
	ArtifactID string `json:"artifact_id"`
	Path       string `json:"path"`
}

func WriteArtifacts(bundle *Bundle, outputDir string) ([]WrittenArtifact, error) {
	if bundle == nil {
		return nil, fmt.Errorf("bundle required")
	}
	if outputDir == "" {
		return nil, fmt.Errorf("output directory required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	written := make([]WrittenArtifact, 0, len(bundle.Artifacts))
	for _, artifact := range bundle.Artifacts {
		if !artifact.Writable {
			continue
		}
		rel := artifact.PathHint
		if rel == "" {
			rel = artifact.FileName
		}
		if rel == "" {
			return nil, fmt.Errorf("artifact %q missing file name", artifact.ID)
		}
		validated, err := validateArtifactPath(rel)
		if err != nil {
			return nil, fmt.Errorf("artifact %q: %w", artifact.ID, err)
		}
		target := filepath.Join(outputDir, validated)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("create artifact directory: %w", err)
		}
		mode := os.FileMode(0o644)
		if artifact.Executable {
			mode = 0o755
		}
		if err := os.WriteFile(target, []byte(artifact.Content), mode); err != nil {
			return nil, fmt.Errorf("write artifact %q: %w", artifact.ID, err)
		}
		written = append(written, WrittenArtifact{ArtifactID: artifact.ID, Path: target})
	}

	return written, nil
}
