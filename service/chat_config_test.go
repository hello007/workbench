package service

import (
	"context"
	"path/filepath"
	"testing"

	"workbench/model"
	"workbench/util/testutil"
)

// ===== 模板 CRUD =====

// TestChatService_AddTemplate_Validation Add 入参校验：scope 可选集、
// name/content 非空、directory 域 directoryID 必填、global 忽略 directoryID。
func TestChatService_AddTemplate_Validation(t *testing.T) {
	svc, _ := newChatSvcForTest(t)

	if _, err := svc.AddChatTemplate("other", "", "n", "c"); err == nil {
		t.Error("无效 scope 应报错")
	}
	if _, err := svc.AddChatTemplate(model.ChatTemplateScopeGlobal, "", "  ", "c"); err == nil {
		t.Error("空 name 应报错")
	}
	if _, err := svc.AddChatTemplate(model.ChatTemplateScopeGlobal, "", "n", "   "); err == nil {
		t.Error("空 content 应报错")
	}
	if _, err := svc.AddChatTemplate(model.ChatTemplateScopeDirectory, "  ", "n", "c"); err == nil {
		t.Error("directory 域缺 directoryID 应报错")
	}

	// global：directoryID 被忽略清空
	g, err := svc.AddChatTemplate(model.ChatTemplateScopeGlobal, "dir-x", " 全局模板 ", " 内容 ")
	if err != nil {
		t.Fatalf("AddChatTemplate global: %v", err)
	}
	if g.Scope != model.ChatTemplateScopeGlobal || g.DirectoryID != "" {
		t.Errorf("global 模板字段异常: %+v", g)
	}
	if g.Name != "全局模板" || g.Content != "内容" {
		t.Errorf("name/content 应去首尾空白: %+v", g)
	}

	// directory：directoryID 保留
	d, err := svc.AddChatTemplate(model.ChatTemplateScopeDirectory, "dir-1", "目录模板", "c")
	if err != nil {
		t.Fatalf("AddChatTemplate directory: %v", err)
	}
	if d.Scope != model.ChatTemplateScopeDirectory || d.DirectoryID != "dir-1" {
		t.Errorf("directory 模板字段异常: %+v", d)
	}
}

// TestChatService_ListTemplates_Merge List 合并语义：目录模板 + 全局模板
// 合并返回（目录在前全局在后），他目录模板与其他 scope 不混入；空 directoryID
// 仅返回全局。
func TestChatService_ListTemplates_Merge(t *testing.T) {
	svc, _ := newChatSvcForTest(t)

	g1, _ := svc.AddChatTemplate(model.ChatTemplateScopeGlobal, "", "g1", "c1")
	d1, _ := svc.AddChatTemplate(model.ChatTemplateScopeDirectory, "dir-1", "d1", "c2")
	_, _ = svc.AddChatTemplate(model.ChatTemplateScopeDirectory, "dir-2", "d2-other", "c3")
	g2, _ := svc.AddChatTemplate(model.ChatTemplateScopeGlobal, "", "g2", "c4")
	d1b, _ := svc.AddChatTemplate(model.ChatTemplateScopeDirectory, "dir-1", "d1b", "c5")

	list, err := svc.ListChatTemplates("dir-1")
	if err != nil {
		t.Fatalf("ListChatTemplates: %v", err)
	}
	if len(list) != 4 {
		t.Fatalf("dir-1 合并列表应 4 项（2 目录 + 2 全局）, got %d", len(list))
	}
	// 目录在前（创建序 d1→d1b），全局在后（创建序 g1→g2）；他目录的 d2-other 不混入
	gotIDs := []string{list[0].ID, list[1].ID, list[2].ID, list[3].ID}
	wantIDs := []string{d1.ID, d1b.ID, g1.ID, g2.ID}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Errorf("排序[%d]: got %s, want %s", i, gotIDs[i], wantIDs[i])
		}
	}
	for _, tpl := range list {
		if tpl.ID == "d2-other" || tpl.DirectoryID == "dir-2" {
			t.Errorf("他目录模板不应混入: %+v", tpl)
		}
	}

	// 空 directoryID 仅全局
	globals, err := svc.ListChatTemplates("")
	if err != nil {
		t.Fatalf("ListChatTemplates(''): %v", err)
	}
	for _, tpl := range globals {
		if tpl.Scope != model.ChatTemplateScopeGlobal {
			t.Errorf("空目录仅应返回全局模板, got %+v", tpl)
		}
	}
	if len(globals) != 2 {
		t.Errorf("全局模板应 2 项, got %d", len(globals))
	}

	// 无目录模板的新目录：仅全局
	other, _ := svc.ListChatTemplates("dir-none")
	if len(other) != 2 {
		t.Errorf("不存在目录应仅全局 2 项, got %d", len(other))
	}
}

// TestChatService_UpdateRemoveTemplate Update 改名与内容并刷新 UpdatedAt；
// Remove 删除；不存在项报错。
func TestChatService_UpdateRemoveTemplate(t *testing.T) {
	svc, _ := newChatSvcForTest(t)
	tpl, _ := svc.AddChatTemplate(model.ChatTemplateScopeGlobal, "", "旧名", "旧内容")
	oldUpdatedAt := tpl.UpdatedAt

	if err := svc.UpdateChatTemplate(tpl.ID, "  ", "c"); err == nil {
		t.Error("空 name 应报错")
	}
	if err := svc.UpdateChatTemplate(tpl.ID, "n", " "); err == nil {
		t.Error("空 content 应报错")
	}
	if err := svc.UpdateChatTemplate("tpl-x", "n", "c"); err == nil {
		t.Error("不存在模板应报错")
	}

	if err := svc.UpdateChatTemplate(tpl.ID, "新名", "新内容"); err != nil {
		t.Fatalf("UpdateChatTemplate: %v", err)
	}
	list, _ := svc.ListChatTemplates("")
	if list[0].Name != "新名" || list[0].Content != "新内容" {
		t.Errorf("更新未生效: %+v", list[0])
	}
	if list[0].UpdatedAt < oldUpdatedAt {
		t.Errorf("UpdatedAt 应刷新: got %d, old %d", list[0].UpdatedAt, oldUpdatedAt)
	}

	if err := svc.RemoveChatTemplate(tpl.ID); err != nil {
		t.Fatalf("RemoveChatTemplate: %v", err)
	}
	if list, _ := svc.ListChatTemplates(""); len(list) != 0 {
		t.Errorf("删除后列表应为空, got %d", len(list))
	}
	if err := svc.RemoveChatTemplate(tpl.ID); err == nil {
		t.Error("重复删除应报错")
	}
}

// TestChatService_TemplatesPersistenceReload 重构 service 后模板仍在（文件持久化）。
func TestChatService_TemplatesPersistenceReload(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "ai_chat")
	svc1 := NewChatService(context.Background(), dataDir)
	created, err := svc1.AddChatTemplate(model.ChatTemplateScopeGlobal, "", "持久模板", "内容")
	if err != nil {
		t.Fatalf("AddChatTemplate: %v", err)
	}

	svc2 := NewChatService(context.Background(), dataDir)
	list, err := svc2.ListChatTemplates("")
	if err != nil || len(list) != 1 || list[0].ID != created.ID || list[0].Name != "持久模板" {
		t.Errorf("重载模板不一致: %v, %+v", err, list)
	}
}

// TestChatService_TemplatesCorruptFallback 模板索引损坏：备份原文件后降级空索引。
func TestChatService_TemplatesCorruptFallback(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "ai_chat")
	testutil.WriteFile(t, filepath.Join(dataDir, "templates.json"), "{not-valid-json")

	svc := NewChatService(context.Background(), dataDir)
	list, err := svc.ListChatTemplates("")
	if err != nil || len(list) != 0 {
		t.Fatalf("损坏模板索引应降级为空列表, got %v, %v", list, err)
	}
	if _, err := svc.AddChatTemplate(model.ChatTemplateScopeGlobal, "", "t", "c"); err != nil {
		t.Fatalf("降级后新增模板失败: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dataDir, "templates.json.bak.*"))
	if len(matches) != 1 {
		t.Errorf("损坏模板索引应留备份文件, got %v", matches)
	}
}

// ===== 执行配置 =====

// TestChatService_Settings_DefaultAndSave 默认值、保存回读、越界收敛。
func TestChatService_Settings_DefaultAndSave(t *testing.T) {
	svc, _ := newChatSvcForTest(t)

	// 文件不存在：默认 default + 空 model
	got := svc.GetChatSettings()
	if got.PermissionMode != model.ChatPermissionModeDefault || got.ModelName != "" {
		t.Errorf("默认设置异常: %+v", got)
	}

	// 保存后回读
	if err := svc.SaveChatSettings(model.ChatPermissionModeBypassPermission, " opus "); err != nil {
		t.Fatalf("SaveChatSettings: %v", err)
	}
	got = svc.GetChatSettings()
	if got.PermissionMode != model.ChatPermissionModeBypassPermission || got.ModelName != "opus" {
		t.Errorf("保存后回读异常: %+v", got)
	}

	// 无效 permissionMode 拒绝保存且不破坏已有配置
	if err := svc.SaveChatSettings("invalid", "m"); err == nil {
		t.Error("无效权限模式应报错")
	}
	if got = svc.GetChatSettings(); got.PermissionMode != model.ChatPermissionModeBypassPermission {
		t.Errorf("保存失败不应破坏已有配置: %+v", got)
	}

	// modelName 空串合法（claude 默认模型语义）
	if err := svc.SaveChatSettings(model.ChatPermissionModePlan, ""); err != nil {
		t.Fatalf("空 modelName 应合法: %v", err)
	}
	if got = svc.GetChatSettings(); got.PermissionMode != model.ChatPermissionModePlan || got.ModelName != "" {
		t.Errorf("空 model 保存回读异常: %+v", got)
	}
}

// TestChatService_Settings_CorruptFallback 配置损坏回默认值不报错。
func TestChatService_Settings_CorruptFallback(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "ai_chat")
	testutil.WriteFile(t, filepath.Join(dataDir, "settings.json"), "{not-valid-json")

	svc := NewChatService(context.Background(), dataDir)
	got := svc.GetChatSettings()
	if got.PermissionMode != model.ChatPermissionModeDefault || got.ModelName != "" {
		t.Errorf("损坏配置应回默认值, got %+v", got)
	}
}

// TestChatService_Settings_InvalidModeNormalized 配置文件里 permissionMode
// 越界（手改文件）时读出收敛回 default。
func TestChatService_Settings_InvalidModeNormalized(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "ai_chat")
	testutil.WriteFile(t, filepath.Join(dataDir, "settings.json"),
		`{"permissionMode":"yolo","modelName":"sonnet"}`)

	svc := NewChatService(context.Background(), dataDir)
	got := svc.GetChatSettings()
	if got.PermissionMode != model.ChatPermissionModeDefault {
		t.Errorf("越界模式应收敛 default, got %q", got.PermissionMode)
	}
	if got.ModelName != "sonnet" {
		t.Errorf("modelName 应保留, got %q", got.ModelName)
	}
}
