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

// TestFileChange_StatusLabel 覆盖各变更状态到中文标签的映射及默认分支。
func TestFileChange_StatusLabel(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"M", "已修改"},
		{"A", "已添加"},
		{"D", "已删除"},
		{"R", "已重命名"},
		{"?", "未跟踪"},
		{"X", "X"}, // 未知状态回退原值
		{"", ""},
	}
	for _, c := range cases {
		fc := &FileChange{Status: c.status}
		got := fc.StatusLabel()
		if got != c.want {
			t.Errorf("status %q: got %q, want %q", c.status, got, c.want)
		}
	}
}
