package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// === mergeMissingSeedSkills 单测 ===
//
// PR1 在 defaultAiFunctions 新增 commit-message/code-review 两个 epic 交付 skill。
// 老用户已存 data/ai_functions.json（仅旧 4 项）加载时缺这两项，mergeMissingSeedSkills
// 负责按白名单补齐：仅补 epic 新增项，不回种用户删过的旧 skill，不覆盖用户自定义同 id 项。

// TestMergeMissingSeedSkills_OldConfigAddsEpicSkills 旧配置（仅 4 项原 skill，无 commit-message/code-review）
// 合并后追加 epic 新增 2 项，changed=true，原 4 项保留。
func TestMergeMissingSeedSkills_OldConfigAddsEpicSkills(t *testing.T) {
	// 模拟 PR1 之前的旧配置：仅 4 项原 skill
	funcs := []*model.AiFunction{
		{ID: "speech-doc", Name: "文档转 HTML 发言稿", Command: "/ab-office:agree-slides"},
		{ID: "weekly-report", Name: "生成周报", Command: "/ab-weekly-report"},
		{ID: "meeting-book", Name: "预约腾讯会议", Command: "/tencent-meeting-mcp"},
		{ID: "meeting-list", Name: "查看/取消腾讯会议", Command: "/tencent-meeting-mcp"},
	}
	merged, changed := mergeMissingSeedSkills(funcs)
	if !changed {
		t.Error("旧配置缺 epic skill，changed 应为 true")
	}
	if len(merged) != 6 {
		t.Fatalf("合并后数量: 期望=6 实际=%d", len(merged))
	}
	ids := map[string]bool{}
	for _, fn := range merged {
		ids[fn.ID] = true
	}
	for _, want := range []string{"commit-message", "code-review"} {
		if !ids[want] {
			t.Errorf("合并后应含 epic skill %s", want)
		}
	}
	// 原 4 项保留（不因合并丢失）
	for _, want := range []string{"speech-doc", "weekly-report", "meeting-book", "meeting-list"} {
		if !ids[want] {
			t.Errorf("合并后原 skill %s 应保留", want)
		}
	}
}

// TestMergeMissingSeedSkills_FullConfigNoChange 全量配置（6 项含 epic skill）
// → changed=false，funcs 数量与内容不变（无追加）。
func TestMergeMissingSeedSkills_FullConfigNoChange(t *testing.T) {
	funcs := defaultAiFunctions() // 6 项全量
	originalLen := len(funcs)
	merged, changed := mergeMissingSeedSkills(funcs)
	if changed {
		t.Error("全量配置已含 epic skill，changed 应为 false")
	}
	if len(merged) != originalLen {
		t.Errorf("全量配置不应改变数量: 期望=%d 实际=%d", originalLen, len(merged))
	}
}

// TestMergeMissingSeedSkills_RespectsUserDeletion 用户删了 meeting-book（5 项含 epic skill）
// → changed=false，meeting-book 不加回（白名单仅 commit-message/code-review，尊重用户删除决策）。
func TestMergeMissingSeedSkills_RespectsUserDeletion(t *testing.T) {
	// 用户删除了 meeting-book，但已含 commit-message/code-review
	funcs := []*model.AiFunction{
		{ID: "speech-doc", Command: "/x"},
		{ID: "weekly-report", Command: "/x"},
		{ID: "meeting-list", Command: "/x"},
		{ID: "commit-message", Command: "", Params: &model.AiParamSpec{Type: "form", PromptTemplate: "tpl"}},
		{ID: "code-review", Command: "", Params: &model.AiParamSpec{Type: "form", PromptTemplate: "tpl"}},
	}
	merged, changed := mergeMissingSeedSkills(funcs)
	if changed {
		t.Error("已含 epic skill，changed 应为 false")
	}
	if len(merged) != 5 {
		t.Errorf("数量应保持 5 不变: 实际=%d", len(merged))
	}
	ids := map[string]bool{}
	for _, fn := range merged {
		ids[fn.ID] = true
	}
	if ids["meeting-book"] {
		t.Error("meeting-book 被用户删除，不应加回（白名单不含旧 skill）")
	}
}

// TestMergeMissingSeedSkills_RespectsUserCustomization 用户自定义 commit-message
// （ID 同但 Name/Params 改过）+ 已有 code-review → 不覆盖用户自定义项，changed=false。
func TestMergeMissingSeedSkills_RespectsUserCustomization(t *testing.T) {
	customName := "我的提交信息生成器"
	customTpl := "自定义模板 {{diff}}"
	funcs := []*model.AiFunction{
		{ID: "commit-message", Name: customName, Command: "", Params: &model.AiParamSpec{Type: "form", PromptTemplate: customTpl}},
		{ID: "code-review", Name: "AI 代码审查", Command: "", Params: &model.AiParamSpec{Type: "form", PromptTemplate: "tpl"}},
	}
	merged, changed := mergeMissingSeedSkills(funcs)
	if changed {
		t.Error("用户已有两个 epic skill（其一自定义），changed 应为 false")
	}
	if len(merged) != 2 {
		t.Fatalf("不应追加，数量: 期望=2 实际=%d", len(merged))
	}
	// 定位 commit-message 项验证未被 seed 覆盖
	var cm *model.AiFunction
	for _, fn := range merged {
		if fn.ID == "commit-message" {
			cm = fn
		}
	}
	if cm == nil {
		t.Fatal("commit-message 项丢失")
	}
	if cm.Name != customName {
		t.Errorf("用户自定义 Name 被覆盖: 期望=%s 实际=%s", customName, cm.Name)
	}
	if cm.Params == nil || cm.Params.PromptTemplate != customTpl {
		t.Errorf("用户自定义 PromptTemplate 被覆盖: 期望=%s", customTpl)
	}
}

// TestMergeMissingSeedSkills_PartialEpicSkill 缺一个 epic skill（有 commit-message 缺 code-review）
// → 仅补 code-review，changed=true，commit-message 项不变。
func TestMergeMissingSeedSkills_PartialEpicSkill(t *testing.T) {
	customTpl := "保留我的 commit 模板"
	funcs := []*model.AiFunction{
		{ID: "speech-doc", Command: "/x"},
		{ID: "commit-message", Name: "自定义提交", Command: "", Params: &model.AiParamSpec{Type: "form", PromptTemplate: customTpl}},
	}
	merged, changed := mergeMissingSeedSkills(funcs)
	if !changed {
		t.Error("缺 code-review，changed 应为 true")
	}
	if len(merged) != 3 {
		t.Fatalf("合并后数量: 期望=3 实际=%d", len(merged))
	}
	ids := map[string]bool{}
	var cm *model.AiFunction
	for _, fn := range merged {
		ids[fn.ID] = true
		if fn.ID == "commit-message" {
			cm = fn
		}
	}
	if !ids["code-review"] {
		t.Error("应补齐 code-review")
	}
	// 已有的 commit-message 自定义项不被覆盖
	if cm == nil || cm.Params.PromptTemplate != customTpl {
		t.Errorf("commit-message 自定义项应保留: %+v", cm)
	}
}

// === LoadAiFunctions 集成：老配置合并 epic skill 端到端 ===

// TestLoadAiFunctions_MergesSeedSkillsForOldConfig 验证旧配置（仅 4 项原 skill）加载时
// 自动合并 epic 新增 commit-message/code-review 并落盘，下次加载直读合并后结构。
func TestLoadAiFunctions_MergesSeedSkillsForOldConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ai_functions.json")
	// PR1 之前的旧配置：4 项原 skill（v2 结构，字段完整避免迁移干扰）
	old := `{"schemaVersion":2,"functions":[
		{"id":"speech-doc","name":"文档转 HTML 发言稿","command":"/ab-office:agree-slides","cwd":"D:\\a","permissionMode":"bypassPermissions","timeoutMinutes":15,"completion":"preview"},
		{"id":"weekly-report","name":"生成周报","command":"/ab-weekly-report","cwd":"D:\\w","permissionMode":"bypassPermissions","timeoutMinutes":20,"completion":"open_dir"},
		{"id":"meeting-book","name":"预约腾讯会议","command":"/tencent-meeting-mcp","cwd":"D:\\m","permissionMode":"bypassPermissions","timeoutMinutes":5,"completion":"copy"},
		{"id":"meeting-list","name":"查看会议","command":"/tencent-meeting-mcp","cwd":"D:\\m","permissionMode":"bypassPermissions","timeoutMinutes":5,"completion":"none"}
	]}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatalf("预写旧配置失败: %v", err)
	}
	svc := NewAiFunctionService(nil, path)
	funcs, err := svc.LoadAiFunctions()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(funcs) != 6 {
		t.Fatalf("旧配置合并后数量: 期望=6 实际=%d", len(funcs))
	}
	ids := map[string]bool{}
	for _, fn := range funcs {
		ids[fn.ID] = true
	}
	for _, want := range []string{"commit-message", "code-review"} {
		if !ids[want] {
			t.Errorf("合并后应含 epic skill %s", want)
		}
	}
	// 落盘验证：文件已被重写为 6 项（seedMerged 触发 saveConfig）
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读回配置文件失败: %v", err)
	}
	var persisted model.AiFunctionsConfig
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("落盘文件非合法 JSON: %v", err)
	}
	if persisted.SchemaVersion != model.CurrentSchemaVersion {
		t.Errorf("schemaVersion 不符: 期望=%d 实际=%d", model.CurrentSchemaVersion, persisted.SchemaVersion)
	}
	if len(persisted.Functions) != 6 {
		t.Errorf("落盘数量: 期望=6 实际=%d", len(persisted.Functions))
	}

	// 二次加载直读合并后结构（无再次追加，数量稳定为 6）
	funcs2, err := svc.LoadAiFunctions()
	if err != nil {
		t.Fatalf("二次加载失败: %v", err)
	}
	if len(funcs2) != 6 {
		t.Errorf("二次加载数量应稳定为 6: 实际=%d", len(funcs2))
	}
}
