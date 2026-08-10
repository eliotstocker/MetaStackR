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

func TestDetectVCSProvider(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"git@github.com:owner/repo.git", "github"},
		{"https://github.com/owner/repo", "github"},
		{"git@gitlab.com:owner/repo.git", "gitlab"},
		{"https://gitlab.org/owner/repo.git", "gitlab"},
		{"git@bitbucket.org:owner/repo.git", "unknown"},
		{"https://azure.com/owner/repo", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		got := DetectVCSProvider("", tt.url)
		if got != tt.expected {
			t.Errorf("DetectVCSProvider(\"\", %q) = %q; want %q", tt.url, got, tt.expected)
		}
	}

	// Test git config metastackr.vcs-provider override
	tempDir, err := os.MkdirTemp("", "metastackr-vcs-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	_, _ = ExecGit(tempDir, "init", "-b", "main")
	_, _ = ExecGit(tempDir, "config", "metastackr.vcs-provider", "github")

	// Custom URL without 'github' should return 'github' due to config override
	got := DetectVCSProvider(tempDir, "git@custom-enterprise-domain.com:owner/repo.git")
	if got != "github" {
		t.Errorf("Expected 'github' from config override, got %q", got)
	}
}

