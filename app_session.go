package main

import "workbench/model"

// ===== 会话快照域（崩溃恢复 UI 状态）=====

// GetSessionState 读取上次会话的 UI 状态快照（data/session.json）。
// 文件不存在或损坏时后端降级返回空快照（不报错），前端据此走冷启动或恢复流程。
// sessionSvc 未注入（零值构造 / startup 前）时返回空快照，不 panic。
func (a *App) GetSessionState() (*model.SessionState, error) {
	if a.sessionSvc == nil {
		return &model.SessionState{}, nil
	}
	return a.sessionSvc.Load()
}

// SaveSessionState 持久化当前 UI 状态快照。前端 watch store debounce 调用 + shutdown 最终写入。
// 保存失败仅后端 slog 记录，返回 error 供前端静默处理（不阻塞 UI）。sessionSvc 未注入时 no-op。
func (a *App) SaveSessionState(state *model.SessionState) error {
	if a.sessionSvc == nil {
		return nil
	}
	return a.sessionSvc.Save(state)
}
