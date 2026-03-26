package onboarding

import (
	"archive/zip"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
)

func ArchiveBundle(bundle *Bundle) ([]byte, error) {
	if bundle == nil {
		return nil, fmt.Errorf("bundle required")
	}

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, artifact := range bundle.Artifacts {
		if !artifact.Writable {
			continue
		}
		name := artifact.PathHint
		if strings.TrimSpace(name) == "" {
			name = artifact.FileName
		}
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("artifact %q missing path", artifact.ID)
		}
		validated, err := validateArtifactPath(name)
		if err != nil {
			return nil, fmt.Errorf("artifact %q: %w", artifact.ID, err)
		}
		entry, err := writer.Create(filepath.ToSlash(validated))
		if err != nil {
			return nil, fmt.Errorf("create zip entry: %w", err)
		}
		if _, err := entry.Write([]byte(artifact.Content)); err != nil {
			return nil, fmt.Errorf("write zip entry: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close zip writer: %w", err)
	}
	return buf.Bytes(), nil
}

func BundleArchiveName(resp *BundleResponse) string {
	if resp == nil || resp.Bundle == nil {
		return "openclause-onboarding-bundle.zip"
	}
	tenant := safeName(resp.Tenant.Name)
	if tenant == "" {
		tenant = safeName(resp.Tenant.ID)
	}
	agent := safeName(resp.Agent.Name)
	if agent == "" {
		agent = safeName(resp.Agent.ID)
	}
	mode := safeName(resp.Mode)
	if mode == "" {
		mode = "bundle"
	}
	parts := []string{"openclause", tenant, agent, mode}
	return strings.Trim(strings.Join(parts, "-"), "-") + ".zip"
}

func safeName(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	text = strings.ReplaceAll(text, " ", "-")
	text = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_':
			return '-'
		default:
			return -1
		}
	}, text)
	return strings.Trim(text, "-")
}
