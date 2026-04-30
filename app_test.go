package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetGitRemoteURL_ValidRepo(t *testing.T) {
	// Create temporary test repository
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "test-repo")
	os.MkdirAll(repoPath, 0755)

	// Initialize Git repository
	err := exec.Command("git", "init", repoPath).Run()
	if err != nil {
		t.Skip("Cannot create test repository")
	}

	app := NewApp()
	info, err := app.GetGitRemoteURL(repoPath)
	if err != nil {
		t.Fatalf("GetGitRemoteURL failed: %v", err)
	}

	if info == nil {
		t.Fatal("Expected GitRemoteInfo, got nil")
	}
}

func TestGetGitRemoteURL_InvalidPath(t *testing.T) {
	app := NewApp()
	_, err := app.GetGitRemoteURL("/invalid/nonexistent/path")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestGetGitRemoteURL_CurrentRepo(t *testing.T) {
	// Test with the current repository (git-manager)
	app := NewApp()
	info, err := app.GetGitRemoteURL(".")
	if err != nil {
		t.Fatalf("GetGitRemoteURL failed on current repo: %v", err)
	}

	if info == nil {
		t.Fatal("Expected GitRemoteInfo, got nil")
	}

	// The function should work even without origin remote
	// It will return empty strings in that case
	t.Logf("Repository Info - Branch: %s, RemoteURL: %s, IsDetached: %v",
		info.Branch, info.RemoteURL, info.IsDetached)

	// Verify the structure is valid (not nil)
	if info.RemoteURL == "" && info.Branch == "" && !info.IsDetached {
		t.Log("Repository has no origin remote (this is OK for the test)")
	}
}
