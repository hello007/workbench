package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
	"workbench/service"
)

// writeSkillFixture 在 skillsRoot/<name>/SKILL.md 写一个可被 parseSkillFrontmatter 解析的 skill。
func writeSkillFixture(t *testing.T, skillsRoot, name, desc string) {
	t.Helper()
	dir := filepath.Join(skillsRoot, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func hasSkillCommand(list []*model.SkillDescriptor, command string) bool {
	for _, sd := range list {
		if sd != nil && sd.Command == command {
			return true
		}
	}
	return false
}

func sliceContains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// TestCollectDraftSkills_GlobalPlusDirectoryDedup 候选 skill = 全局发现 ∪ 所选目录扫描，
// 同一来源（source+sourceDir）去重，仅保留有命令项。
func TestCollectDraftSkills_GlobalPlusDirectoryDedup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	writeSkillFixture(t, filepath.Join(home, ".claude", "skills"), "global-skill", "全局技能")

	proj := t.TempDir()
	writeSkillFixture(t, filepath.Join(proj, ".claude", "skills"), "proj-skill", "项目技能")

	// 目录列表为空：候选 = 全局发现 ∪ 所选目录扫描。
	app := &App{AppServices: &AppServices{
		skillDiscoverySvc: service.NewSkillDiscoveryService(service.NewDirectoryService(filepath.Join(t.TempDir(), "directories.json"))),
	}}
	got := app.collectDraftSkills(proj)
	if !hasSkillCommand(got, "/global-skill") {
		t.Errorf("应含全局 skill, got %v", commandsOf(got))
	}
	if !hasSkillCommand(got, "/proj-skill") {
		t.Errorf("应含所选目录 skill, got %v", commandsOf(got))
	}
	if empty := app.collectDraftSkills(""); hasSkillCommand(empty, "/proj-skill") {
		t.Error("未指定目录且目录列表为空时不应包含项目级 skill")
	}

	// 所选目录同时登记在目录列表：GetCached 与 GetDiscoveredSkillsIn 产出同一来源，
	// 由 collectDraftSkills 的 source+sourceDir 去重键收敛为一条。
	dirCfg := filepath.Join(t.TempDir(), "directories.json")
	if err := os.WriteFile(dirCfg, []byte(fmt.Sprintf(`{"directories":[{"id":"d1","name":"proj","path":%q}]}`, proj)), 0o644); err != nil {
		t.Fatalf("write directories.json: %v", err)
	}
	app2 := &App{AppServices: &AppServices{
		skillDiscoverySvc: service.NewSkillDiscoveryService(service.NewDirectoryService(dirCfg)),
	}}
	projCount := 0
	for _, sd := range app2.collectDraftSkills(proj) {
		if sd != nil && sd.Command == "/proj-skill" {
			projCount++
		}
	}
	if projCount != 1 {
		t.Errorf("重复来源应去重为一条, got %d", projCount)
	}
}

func commandsOf(list []*model.SkillDescriptor) []string {
	out := make([]string, 0, len(list))
	for _, sd := range list {
		if sd != nil {
			out = append(out, sd.Command)
		}
	}
	return out
}

// TestCollectAiFunctionIDs_ReadOnly 只读收集 ID：不创建/不改写配置文件，
// 不触发 LoadAiFunctions 的 seed 合并副作用。
func TestCollectAiFunctionIDs_ReadOnly(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "ai_functions.json")
	svc := service.NewAiFunctionService(context.Background(), cfg)
	app := &App{AppServices: &AppServices{aiFuncSvc: svc}}

	// 文件不存在：返回内置默认 ID，且不得创建配置文件。
	ids := app.collectAiFunctionIDs()
	if !sliceContains(ids, "weekly-report") {
		t.Errorf("文件缺失时应返回内置默认 ID, got %v", ids)
	}
	if _, err := os.Stat(cfg); !os.IsNotExist(err) {
		t.Fatalf("只读读取不应创建配置文件: %v", err)
	}

	// 老配置（仅 1 项、缺 epic seed）：只读不得做 seed 合并，也不得改写文件。
	raw := `{"schemaVersion":2,"functions":[{"id":"speech-doc","name":"n","command":"/x"}]}`
	if err := os.WriteFile(cfg, []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	ids = app.collectAiFunctionIDs()
	if !sliceContains(ids, "speech-doc") {
		t.Errorf("应读出已有 ID, got %v", ids)
	}
	if sliceContains(ids, "commit-message") {
		t.Errorf("只读读取不应触发 seed 合并, got %v", ids)
	}
	after, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(after) != raw {
		t.Error("只读读取不得改写配置文件")
	}

	// 非法 JSON：降级空清单，不阻断生成。
	if err := os.WriteFile(cfg, []byte("{bad"), 0o644); err != nil {
		t.Fatalf("write bad config: %v", err)
	}
	if ids := app.collectAiFunctionIDs(); len(ids) != 0 {
		t.Errorf("非法配置应降级空清单, got %v", ids)
	}
}
