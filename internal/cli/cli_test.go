package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetAgentsMDContent(t *testing.T) {
	content := GetAgentsMDContent()
	if !strings.Contains(content, "# Repository Agent Guidelines") {
		t.Errorf("Expected title in AGENTS.md content")
	}
	if !strings.Contains(content, "MetaStackr") {
		t.Errorf("Expected MetaStackr description in AGENTS.md content")
	}
	if !strings.Contains(content, "git meta status --json") {
		t.Errorf("Expected git-meta operations in AGENTS.md content")
	}
}

func TestWriteAgentsMD(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "metastackr-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	err = WriteAgentsMD(tempDir)
	if err != nil {
		t.Fatalf("WriteAgentsMD failed: %v", err)
	}

	rootPath := filepath.Join(tempDir, "AGENTS.md")
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		t.Errorf("Root AGENTS.md was not created")
	}

	dotAgentsPath := filepath.Join(tempDir, ".agents", "AGENTS.md")
	if _, err := os.Stat(dotAgentsPath); os.IsNotExist(err) {
		t.Errorf(".agents/AGENTS.md was not created")
	}

	content, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatalf("Failed to read created AGENTS.md: %v", err)
	}

	if !strings.Contains(string(content), "MetaStackr") {
		t.Errorf("AGENTS.md missing expected content")
	}
}
