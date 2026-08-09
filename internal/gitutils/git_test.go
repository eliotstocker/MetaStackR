package gitutils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreatePRs_NoFeatureBranch(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "metastackr-git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Init git repo
	_, err = ExecGit(tempDir, "init", "-b", "main")
	if err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	dummyFile := filepath.Join(tempDir, "README.md")
	_ = os.WriteFile(dummyFile, []byte("# Test Repo"), 0644)
	_, _ = ExecGit(tempDir, "add", ".")
	_, _ = ExecGit(tempDir, "commit", "-m", "initial commit")

	opts := CreatePROptions{
		BaseBranch: "main",
	}

	_, err = CreatePRs(tempDir, opts)
	if err == nil {
		t.Errorf("Expected error when no feature branches exist, got nil")
	}
}
