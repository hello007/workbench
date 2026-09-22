package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"workbench/model"
	"workbench/util"
)

// 本文件承载 ChatService 的对话配置域：输入框模板 CRUD 与执行配置
// （权限模式/模型）读写。与会话/消息持久化同住 data/ai_chat/ 目录：
//   - data/ai_chat/templates.json  模板列表（global + directory 两域）
//   - data/ai_chat/settings.json   执行配置（permissionMode + modelName）

// chatTemplateSchemaVersion templates.json 当前 schema 版本（迁移预留）。
const chatTemplateSchemaVersion = 1

// chatTemplateIndex templates.json 顶层结构。
type chatTemplateIndex struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Templates     []*model.ChatTemplate `json:"templates"`
}

// templatesPath 模板文件路径（data/ai_chat/templates.json）。
func (s *ChatService) templatesPath() string {
	return filepath.Join(s.dataDir, "templates.json")
}

// settingsPath 执行配置文件路径（data/ai_chat/settings.json）。
func (s *ChatService) settingsPath() string {
	return filepath.Join(s.dataDir, "settings.json")
}

// ===== 模板 CRUD =====

// loadTemplatesIndex 读模板索引。文件不存在返回空索引；损坏时备份原文件后
// 回空索引（与会话/目录索引同款降级，不阻塞功能），并 slog.Warn 记录根因。
func (s *ChatService) loadTemplatesIndex() (*chatTemplateIndex, error) {
	idx := &chatTemplateIndex{SchemaVersion: chatTemplateSchemaVersion}
	if !util.FileExists(s.templatesPath()) {
		return idx, nil
	}
	raw, err := os.ReadFile(s.templatesPath())
	if err == nil {
		// 解析错误覆写 err：保证 Warn 记录真实根因（同 loadIndex）
		if uerr := json.Unmarshal(raw, idx); uerr != nil {
			err = uerr
		}
	}
	if err == nil {
		return idx, nil
	}
	bakPath := s.templatesPath() + ".bak." + time.Now().Format("20060102-150405")
	if raw != nil {
		_ = os.WriteFile(bakPath, raw, 0o644)
	}
	Logger().Warn("chat template index corrupt, fallback to empty index",
		"path", s.templatesPath(), "err", err)
	return &chatTemplateIndex{SchemaVersion: chatTemplateSchemaVersion}, nil
}

// saveTemplatesIndex 落盘模板索引。
func (s *ChatService) saveTemplatesIndex(idx *chatTemplateIndex) error {
	idx.SchemaVersion = chatTemplateSchemaVersion
	if err := util.SaveJSON(s.templatesPath(), idx); err != nil {
		return fmt.Errorf("保存模板失败: %w", err)
	}
	return nil
}

// AddChatTemplate 新增模板。scope 取 model.ChatTemplateScope* 常量：
// global 忽略 directoryID；directory 时 directoryID 必填（归属侧栏目录项）。
// name/content 去除首尾空白后须非空。返回创建的模板项。
func (s *ChatService) AddChatTemplate(scope, directoryID, name, content string) (*model.ChatTemplate, error) {
	if scope != model.ChatTemplateScopeGlobal && scope != model.ChatTemplateScopeDirectory {
		return nil, fmt.Errorf("模板归属域无效: %s", scope)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("模板名称不能为空")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("模板内容不能为空")
	}
	if scope == model.ChatTemplateScopeDirectory {
		directoryID = strings.TrimSpace(directoryID)
		if directoryID == "" {
			return nil, fmt.Errorf("目录模板须指定归属目录")
		}
	} else {
		directoryID = ""
	}

	now := time.Now()
	item := &model.ChatTemplate{
		ID:          fmt.Sprintf("chattpl-%d", now.UnixNano()),
		Scope:       scope,
		DirectoryID: directoryID,
		Name:        name,
		Content:     content,
		CreatedAt:   now.UnixMilli(),
		UpdatedAt:   now.UnixMilli(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadTemplatesIndex()
	if err != nil {
		return nil, err
	}
	idx.Templates = append(idx.Templates, item)
	if err := s.saveTemplatesIndex(idx); err != nil {
		return nil, err
	}
	return item, nil
}

// ListChatTemplates 返回「指定目录的目录模板 + 全局模板」合并列表（带 scope
// 标记，前端按 scope 分组展示）。目录模板在前、全局在后，组内按创建时间升序
// （稳定排序保持插入序）。directoryID 为空时仅返回全局模板。
func (s *ChatService) ListChatTemplates(directoryID string) ([]*model.ChatTemplate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadTemplatesIndex()
	if err != nil {
		return nil, err
	}
	dirTemplates := make([]*model.ChatTemplate, 0)
	globalTemplates := make([]*model.ChatTemplate, 0)
	for _, tpl := range idx.Templates {
		if tpl == nil {
			continue
		}
		switch tpl.Scope {
		case model.ChatTemplateScopeDirectory:
			if directoryID != "" && tpl.DirectoryID == directoryID {
				dirTemplates = append(dirTemplates, tpl)
			}
		case model.ChatTemplateScopeGlobal:
			globalTemplates = append(globalTemplates, tpl)
		}
	}
	// 组内创建时间升序（稳定排序保持同时间戳插入序）
	stableSortTemplatesByCreatedAt(dirTemplates)
	stableSortTemplatesByCreatedAt(globalTemplates)
	out := make([]*model.ChatTemplate, 0, len(dirTemplates)+len(globalTemplates))
	out = append(out, dirTemplates...)
	out = append(out, globalTemplates...)
	return out, nil
}

// stableSortTemplatesByCreatedAt 冒泡升序（与 ListChatSessions 同款实现风格，
// 数据量为个位数到十位数级别，无性能压力）。
func stableSortTemplatesByCreatedAt(list []*model.ChatTemplate) {
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].CreatedAt < list[i].CreatedAt {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
}

// UpdateChatTemplate 修改模板名与内容。模板不存在返回错误；scope 不支持修改
// （归属域变更语义上等于删旧建新，由用户在管理弹窗自行操作）。
func (s *ChatService) UpdateChatTemplate(id, name, content string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("模板名称不能为空")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("模板内容不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadTemplatesIndex()
	if err != nil {
		return err
	}
	for _, tpl := range idx.Templates {
		if tpl != nil && tpl.ID == id {
			tpl.Name = name
			tpl.Content = content
			tpl.UpdatedAt = time.Now().UnixMilli()
			return s.saveTemplatesIndex(idx)
		}
	}
	return fmt.Errorf("模板不存在")
}

// RemoveChatTemplate 删除模板。模板不存在返回错误（幂等性由前端确认交互保证）。
func (s *ChatService) RemoveChatTemplate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadTemplatesIndex()
	if err != nil {
		return err
	}
	found := false
	kept := idx.Templates[:0]
	for _, tpl := range idx.Templates {
		if tpl != nil && tpl.ID == id {
			found = true
			continue
		}
		kept = append(kept, tpl)
	}
	if !found {
		return fmt.Errorf("模板不存在")
	}
	idx.Templates = kept
	return s.saveTemplatesIndex(idx)
}

// ===== 执行配置（权限模式 / 模型）=====

// GetChatSettings 读执行配置。文件不存在或损坏时返回默认值
// （permissionMode=default、modelName 空 = claude 默认模型），不报错——
// 配置缺失/损坏均不影响对话功能可用。
func (s *ChatService) GetChatSettings() *model.ChatSettings {
	defaults := &model.ChatSettings{
		PermissionMode: model.ChatPermissionModeDefault,
		ModelName:      "",
	}
	if !util.FileExists(s.settingsPath()) {
		return defaults
	}
	raw, err := os.ReadFile(s.settingsPath())
	if err == nil {
		// 解析错误覆写 err：保证 Warn 记录真实根因（同 loadIndex）
		if uerr := json.Unmarshal(raw, defaults); uerr != nil {
			err = uerr
		}
	}
	if err != nil {
		Logger().Warn("chat settings corrupt, fallback to defaults",
			"path", s.settingsPath(), "err", err)
		return &model.ChatSettings{
			PermissionMode: model.ChatPermissionModeDefault,
			ModelName:      "",
		}
	}
	// 越界收敛：permissionMode 不在可选集回默认；modelName 空值即默认模型语义
	switch defaults.PermissionMode {
	case model.ChatPermissionModeDefault,
		model.ChatPermissionModeAcceptEdits,
		model.ChatPermissionModePlan,
		model.ChatPermissionModeBypassPermission:
	default:
		defaults.PermissionMode = model.ChatPermissionModeDefault
	}
	defaults.ModelName = strings.TrimSpace(defaults.ModelName)
	return defaults
}

// SaveChatSettings 持久化执行配置。permissionMode 须在可选集内
// （modelName 空串合法 = 使用 claude 默认模型）。
func (s *ChatService) SaveChatSettings(permissionMode, modelName string) error {
	switch permissionMode {
	case model.ChatPermissionModeDefault,
		model.ChatPermissionModeAcceptEdits,
		model.ChatPermissionModePlan,
		model.ChatPermissionModeBypassPermission:
	default:
		return fmt.Errorf("权限模式无效: %s", permissionMode)
	}
	settings := &model.ChatSettings{
		PermissionMode: permissionMode,
		ModelName:      strings.TrimSpace(modelName),
	}
	if err := util.SaveJSON(s.settingsPath(), settings); err != nil {
		return fmt.Errorf("保存对话设置失败: %w", err)
	}
	return nil
}
