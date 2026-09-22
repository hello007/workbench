package main

import (
	"errors"

	"workbench/model"
)

// ===== AI 对话工作台（多会话持续式对话）=====

// CreateChatSession 创建对话会话（元数据入索引，空消息）。
// title 为空时后端默认「新会话」；cwd 为 claude 子进程工作目录。
func (a *App) CreateChatSession(directoryID, title, cwd string) (*model.ChatSession, error) {
	if a.chatSvc == nil {
		return nil, errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.CreateChatSession(directoryID, title, cwd)
}

// ListChatSessions 列出指定目录的会话（按最近活跃降序，不含消息）。
// directoryID 为空返回全部目录的会话。
func (a *App) ListChatSessions(directoryID string) ([]*model.ChatSession, error) {
	if a.chatSvc == nil {
		return []*model.ChatSession{}, nil
	}
	list, err := a.chatSvc.ListChatSessions(directoryID)
	if list == nil {
		list = []*model.ChatSession{}
	}
	return list, err
}

// GetChatSession 读取会话完整内容（元数据 + 消息，按时间升序）。
// 会话不存在返回 AppError（E_CHAT_SESSION_NOT_FOUND）。
func (a *App) GetChatSession(sessionID string) (*model.ChatSession, error) {
	if a.chatSvc == nil {
		return nil, errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.GetChatSession(sessionID)
}

// DeleteChatSession 删除会话（索引项 + 消息文件）。
func (a *App) DeleteChatSession(sessionID string) error {
	if a.chatSvc == nil {
		return errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.DeleteChatSession(sessionID)
}

// UpdateChatSessionTitle 修改会话标题（并刷新活跃时间，列表排序前移）。
func (a *App) UpdateChatSessionTitle(sessionID, title string) error {
	if a.chatSvc == nil {
		return errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.UpdateChatSessionTitle(sessionID, title)
}

// RunChat 执行一轮对话：prompt 直发 claude（会话已有 claude session id 时
// 自动 --resume 续上下文），返回任务 id。
// permissionMode 取值 default/acceptEdits/plan/bypassPermissions（仅非 default
// 时传给 claude）；modelName 为空用 claude 默认模型。
// 输出经 Wails 事件 chat-task:queued/started/output/done 推送。
func (a *App) RunChat(chatSessionID, prompt, permissionMode, modelName string) (string, error) {
	if a.chatSvc == nil {
		return "", errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.RunChat(chatSessionID, prompt, permissionMode, modelName)
}

// CancelChatTask 取消运行中/排队中的对话任务（杀 claude 及其子进程树）。
func (a *App) CancelChatTask(taskID string) bool {
	if a.chatSvc == nil {
		return false
	}
	return a.chatSvc.CancelChatTask(taskID)
}

// GetChatTaskState 查询对话任务状态（回复累积文本、claude 会话 id、运行态）。
// 任务不存在返回 nil。
func (a *App) GetChatTaskState(taskID string) *model.ChatTaskState {
	if a.chatSvc == nil {
		return nil
	}
	return a.chatSvc.GetChatTaskState(taskID)
}

// AddChatDirectory 添加 AI 对话侧栏常用目录项（displayName 为空时后端取目录名）。
// 重复添加返回既有项。
func (a *App) AddChatDirectory(path, displayName string) (*model.ChatDirectory, error) {
	if a.chatSvc == nil {
		return nil, errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.AddChatDirectory(path, displayName)
}

// ListChatDirectories 列出 AI 对话侧栏常用目录项（按 SortOrder 升序）。
func (a *App) ListChatDirectories() ([]*model.ChatDirectory, error) {
	if a.chatSvc == nil {
		return []*model.ChatDirectory{}, nil
	}
	list, err := a.chatSvc.ListChatDirectories()
	if list == nil {
		list = []*model.ChatDirectory{}
	}
	return list, err
}

// UpdateChatDirectory 修改目录项显示名。
func (a *App) UpdateChatDirectory(id, displayName string) error {
	if a.chatSvc == nil {
		return errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.UpdateChatDirectory(id, displayName)
}

// RemoveChatDirectory 移除目录项（不影响已产生的会话与消息数据）。
func (a *App) RemoveChatDirectory(id string) error {
	if a.chatSvc == nil {
		return errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.RemoveChatDirectory(id)
}

// ReorderChatDirectories 按 ids 顺序持久化目录项排序（侧栏拖拽重排）。
func (a *App) ReorderChatDirectories(ids []string) error {
	if a.chatSvc == nil {
		return errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.ReorderChatDirectories(ids)
}

// AddChatTemplate 新增对话输入框模板。scope 取 global/directory（model.ChatTemplateScope*
// 常量）：global 忽略 directoryID；directory 时 directoryID 必填（归属侧栏目录项）。
func (a *App) AddChatTemplate(scope, directoryID, name, content string) (*model.ChatTemplate, error) {
	if a.chatSvc == nil {
		return nil, errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.AddChatTemplate(scope, directoryID, name, content)
}

// ListChatTemplates 返回「该目录的目录模板 + 全局模板」合并列表（带 scope 标记）。
// directoryID 为空时仅返回全局模板。
func (a *App) ListChatTemplates(directoryID string) ([]*model.ChatTemplate, error) {
	if a.chatSvc == nil {
		return []*model.ChatTemplate{}, nil
	}
	list, err := a.chatSvc.ListChatTemplates(directoryID)
	if list == nil {
		list = []*model.ChatTemplate{}
	}
	return list, err
}

// UpdateChatTemplate 修改模板名与内容（归属域不支持修改，删旧建新）。
func (a *App) UpdateChatTemplate(id, name, content string) error {
	if a.chatSvc == nil {
		return errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.UpdateChatTemplate(id, name, content)
}

// RemoveChatTemplate 删除模板。
func (a *App) RemoveChatTemplate(id string) error {
	if a.chatSvc == nil {
		return errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.RemoveChatTemplate(id)
}

// GetChatSettings 读对话执行配置（权限模式 + 模型）。
// 文件缺失/损坏回默认值（default + claude 默认模型），不报错。
func (a *App) GetChatSettings() *model.ChatSettings {
	if a.chatSvc == nil {
		return &model.ChatSettings{PermissionMode: model.ChatPermissionModeDefault}
	}
	return a.chatSvc.GetChatSettings()
}

// SaveChatSettings 持久化对话执行配置（permissionMode 须在可选集内）。
func (a *App) SaveChatSettings(permissionMode, modelName string) error {
	if a.chatSvc == nil {
		return errors.New("AI 对话服务未初始化")
	}
	return a.chatSvc.SaveChatSettings(permissionMode, modelName)
}
