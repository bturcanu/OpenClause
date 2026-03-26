package onboarding

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestArchiveBundleIncludesWritableArtifactsOnly(t *testing.T) {
	bundle := &Bundle{
		Artifacts: []BundleArtifact{
			{
				ID:       "env",
				FileName: "setup-env.sh",
				PathHint: "scripts/setup-env.sh",
				Writable: true,
				Content:  "export OPENCLAUSE_BASE_URL=http://localhost:8080\n",
			},
			{
				ID:       "readonly-note",
				FileName: "README.md",
				Writable: false,
				Content:  "# note\n",
			},
		},
	}

	archiveBytes, err := ArchiveBundle(bundle)
	if err != nil {
		t.Fatalf("ArchiveBundle: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	if len(reader.File) != 1 {
		t.Fatalf("expected only writable artifacts in archive, got %d entries", len(reader.File))
	}
	if reader.File[0].Name != "scripts/setup-env.sh" {
		t.Fatalf("unexpected archive entry name: %s", reader.File[0].Name)
	}
	rc, err := reader.File[0].Open()
	if err != nil {
		t.Fatalf("Open zip entry: %v", err)
	}
	defer rc.Close()
	content, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Read zip entry: %v", err)
	}
	if !strings.Contains(string(content), "OPENCLAUSE_BASE_URL") {
		t.Fatalf("unexpected archive content: %s", string(content))
	}
}

func TestArchiveBundleRejectsMissingPathMetadata(t *testing.T) {
	bundle := &Bundle{
		Artifacts: []BundleArtifact{
			{
				ID:       "broken",
				Writable: true,
				Content:  "oops",
			},
		},
	}

	_, err := ArchiveBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), `artifact "broken" missing path`) {
		t.Fatalf("expected missing path error, got %v", err)
	}
}

func TestArchiveBundleRejectsPathTraversal(t *testing.T) {
	bundle := &Bundle{
		Artifacts: []BundleArtifact{
			{
				ID:       "escape",
				FileName: "escape.sh",
				PathHint: "../escape.sh",
				Writable: true,
				Content:  "echo nope\n",
			},
		},
	}

	_, err := ArchiveBundle(bundle)
	if err == nil || !strings.Contains(err.Error(), `artifact "escape": artifact path "../escape.sh" must stay within the output directory`) {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestArchiveBundleRejectsNilBundle(t *testing.T) {
	_, err := ArchiveBundle(nil)
	if err == nil || !strings.Contains(err.Error(), "bundle required") {
		t.Fatalf("expected nil bundle error, got %v", err)
	}
}

func TestBundleArchiveNameIsStable(t *testing.T) {
	got := BundleArchiveName(&BundleResponse{
		Mode: "regenerated_defaults",
		Tenant: BundleTenant{
			Name: "Alpha Corp",
		},
		Agent: BundleAgent{
			Name: "Support Bot",
		},
		Bundle: &Bundle{Title: "ignored"},
	})
	if got != "openclause-alpha-corp-support-bot-regenerated-defaults.zip" {
		t.Fatalf("unexpected archive name: %s", got)
	}

	if fallback := BundleArchiveName(nil); fallback != "openclause-onboarding-bundle.zip" {
		t.Fatalf("unexpected fallback archive name: %s", fallback)
	}
}
