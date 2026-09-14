package service

import (
	"time"

	"workbench/model"
	"workbench/util"
)

// SessionService 持久化崩溃恢复用的 UI 状态快照（data/session.json）。
//
// 复用 SettingsService 模式：Load 文件不存在 / 损坏时降级返回空 SessionState（不报错），
// 保证崩溃恢复不阻塞启动；Save 失败仅 slog 记录不向上抛（前端 debounce 调用，失败静默）。
// 区别于 SettingsService 静默吞错：Load 损坏场景额外 slog.Warn 记录根因便于排查
// （符合 docs/spec/logging-and-errors.md「错误带业务上下文落盘」要求）。
type SessionService struct {
	path string
}

// NewSessionService 创建会话快照持久化服务。
func NewSessionService(path string) *SessionService {
	return &SessionService{path: path}
}

// Load 读取会话快照。文件不存在或损坏时降级返回空 SessionState（nil error），
// 不阻塞启动。损坏场景额外 slog.Warn 记录路径与根因，便于排查为何走了冷启动。
func (s *SessionService) Load() (*model.SessionState, error) {
	state := &model.SessionState{}
	if !util.FileExists(s.path) {
		return state, nil
	}
	if err := util.LoadJSON(s.path, state); err != nil {
		Logger().Warn("session snapshot corrupt, falling back to cold start",
			"path", s.path, "err", err)
		return &model.SessionState{}, nil
	}
	return state, nil
}

// Save 持久化会话快照。写入前补版本号与时间戳，保证快照自描述（便于未来迁移识别）。
// 失败 slog.Error 记录并返回 error，供调用方决定是否静默（前端 debounce 调用，失败不阻塞 UI）。
func (s *SessionService) Save(state *model.SessionState) error {
	if state == nil {
		state = &model.SessionState{}
	}
	state.Version = model.SessionStateVersion
	state.SavedAt = time.Now().Unix()
	if err := util.SaveJSON(s.path, state); err != nil {
		Logger().Error("save session snapshot failed", "path", s.path, "err", err)
		return err
	}
	return nil
}
