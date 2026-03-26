package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteArtifactsRespectsPathHintsAndNestedDirectories(t *testing.T) {
	dir := t.TempDir()
	bundle := &Bundle{
		Artifacts: []BundleArtifact{
			{
				ID:       "starter",
				FileName: "agent.py",
				PathHint: "examples/python/agent.py",
				Writable: true,
				Content:  "print('ok')\n",
			},
			{
				ID:       "note",
				FileName: "README.md",
				Writable: false,
				Content:  "# ignored\n",
			},
		},
	}

	written, err := WriteArtifacts(bundle, dir)
	if err != nil {
		t.Fatalf("WriteArtifacts: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("expected one writable artifact, got %+v", written)
	}
	if !strings.HasSuffix(written[0].Path, filepath.Join("examples", "python", "agent.py")) {
		t.Fatalf("expected nested path hint to be used, got %+v", written[0])
	}
	if _, err := os.Stat(filepath.Join(dir, "examples", "python", "agent.py")); err != nil {
		t.Fatalf("expected nested artifact to be written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("expected non-writable artifact to be skipped, err=%v", err)
	}
}

func TestWriteArtifactsRejectsMissingFileMetadata(t *testing.T) {
	dir := t.TempDir()
	bundle := &Bundle{
		Artifacts: []BundleArtifact{
			{
				ID:       "broken",
				Writable: true,
				Content:  "oops",
			},
		},
	}

	_, err := WriteArtifacts(bundle, dir)
	if err == nil || !strings.Contains(err.Error(), `artifact "broken" missing file name`) {
		t.Fatalf("expected missing file metadata error, got %v", err)
	}
}

func TestWriteArtifactsMakesExecutableArtifactsRunnable(t *testing.T) {
	dir := t.TempDir()
	bundle := &Bundle{
		Artifacts: []BundleArtifact{
			{
				ID:         "sample-call",
				FileName:   "smoke-test.sh",
				Writable:   true,
				Executable: true,
				Content:    "#!/usr/bin/env bash\necho ok\n",
			},
		},
	}

	written, err := WriteArtifacts(bundle, dir)
	if err != nil {
		t.Fatalf("WriteArtifacts: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("expected one written artifact, got %+v", written)
	}

	info, err := os.Stat(filepath.Join(dir, "smoke-test.sh"))
	if err != nil {
		t.Fatalf("stat executable artifact: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("expected executable permissions 0755, got %o", info.Mode().Perm())
	}
}

func TestWriteArtifactsRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
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

	_, err := WriteArtifacts(bundle, dir)
	if err == nil || !strings.Contains(err.Error(), `artifact "escape": artifact path "../escape.sh" must stay within the output directory`) {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}
