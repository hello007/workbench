package model

import "testing"

func TestCommit_Structure(t *testing.T) {
	commit := Commit{
		SHA:       "abc123def4567890123456789012345678901234",
		ShortSHA:  "abc123de",
		Message:   "Test commit message",
		Author:    "Test Author",
		Email:     "test@example.com",
		Timestamp: 1234567890,
		DateTime:  "2009-02-13 23:31:30",
		Files:     []string{"file1.txt", "file2.txt"},
	}

	if commit.SHA != "abc123def4567890123456789012345678901234" {
		t.Errorf("Expected SHA abc123def4567890123456789012345678901234, got %s", commit.SHA)
	}

	if commit.ShortSHA != "abc123de" {
		t.Errorf("Expected ShortSHA abc123de, got %s", commit.ShortSHA)
	}

	if len(commit.Files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(commit.Files))
	}
}

func TestGitRemoteInfo_Structure(t *testing.T) {
	info := GitRemoteInfo{
		RemoteURL:  "https://github.com/user/repo.git",
		Branch:     "main",
		IsDetached: false,
	}

	if info.RemoteURL != "https://github.com/user/repo.git" {
		t.Errorf("Expected remote URL, got %s", info.RemoteURL)
	}

	if info.Branch != "main" {
		t.Errorf("Expected branch 'main', got %s", info.Branch)
	}

	if info.IsDetached {
		t.Error("Expected not detached")
	}
}
