package service

import (
	"path/filepath"
	"testing"
)

// TestGetDiscoveredSkillsIn_EmptyDir 空目录 / 不存在目录 / 空串均返回空切片不报错。
func TestGetDiscoveredSkillsIn_EmptyDir(t *testing.T) {
	svc := NewSkillDiscoveryService(nil)
	workDir := t.TempDir()

	for _, dir := range []string{"", "   ", workDir, filepath.Join(t.TempDir(), "not-exist")} {
		got := svc.GetDiscoveredSkillsIn(dir)
		if got == nil {
			t.Errorf("dir=%q 应返回非 nil 空切片", dir)
		}
		if len(got) != 0 {
			t.Errorf("dir=%q 应返回空列表, got %d", dir, len(got))
		}
	}
}

// TestGetDiscoveredSkillsIn_ScansDirectory 扫描指定目录 .claude/skills，按项目级语义填充 command/cwd/source。
func TestGetDiscoveredSkillsIn_ScansDirectory(t *testing.T) {
	svc := NewSkillDiscoveryService(nil)
	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, ".claude", "skills")
	writeSkill(t, filepath.Join(skillsDir, "alpha"), "---\nname: alpha\ndescription: \"a\"\n---\n")
	writeSkill(t, filepath.Join(skillsDir, "beta"), "---\nname: beta\ndescription: \"b\"\n---\n")
	// 无 SKILL.md 的子目录与非法 frontmatter 均应跳过。
	writeSkill(t, filepath.Join(skillsDir, "bad"), "# 无 frontmatter\n")

	got := svc.GetDiscoveredSkillsIn(workDir)
	if len(got) != 2 {
		t.Fatalf("got %d skills, want 2", len(got))
	}
	cmds := map[string]string{}
	for _, sd := range got {
		if sd.Source != "project" {
			t.Errorf("source 应为 project, got %q", sd.Source)
		}
		if sd.Cwd != workDir {
			t.Errorf("cwd 应为 %q, got %q", workDir, sd.Cwd)
		}
		cmds[sd.Name] = sd.Command
	}
	if cmds["alpha"] != "/alpha" || cmds["beta"] != "/beta" {
		t.Errorf("command 不符: %v", cmds)
	}
}
