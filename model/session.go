package model

// SessionStateVersion 会话快照格式版本，便于未来字段迁移时识别旧快照并降级处理。
const SessionStateVersion = "1"

// SessionState 崩溃恢复用 UI 状态快照，持久化到 data/session.json。
//
// 设计原则（MVP 边界，见 .trellis/tasks/09-14-v1-4/prd.md PR2）：
//   - 仅恢复 UI 状态（当前工作目录、活动面板、终端面板可见性/高度/工作目录）。
//   - 不恢复未提交代码 / 编辑器未保存内容（超 MVP）。
//   - 文件树展开状态已由前端 useTreeState 持久化到 localStorage（按工作目录隔离），
//     恢复 selectedDirectoryId 后前端按路径从 localStorage 还原，不纳入本快照避免重复。
//   - 终端不真实复用进程，仅恢复工作目录，由前端用 WorkDir 新建会话。
//
// 字段对齐前端 Pinia store 实际结构（见 frontend/src/store/{directory,ui}.js）：
//   - SelectedDirectoryID <-> directoryStore.selectedDirectoryId
//   - ActivePanel <-> uiStore.activePanel（'directory' | 'ai' | 'stats' | 'toolbox'）
//   - Terminal <-> uiStore.terminalVisible / terminalHeight / terminalDir
//
// Terminal 用指针类型：omitempty 仅对 nil 指针生效（零值 struct 不会被 omit），
// 空快照序列化为 "{}"，前端据此判定冷启动。
type SessionState struct {
	SelectedDirectoryID string            `json:"selectedDirectoryId,omitempty"`
	ActivePanel         string            `json:"activePanel,omitempty"`
	Terminal            *TerminalSnapshot `json:"terminal,omitempty"`
	Version             string            `json:"version,omitempty"`
	SavedAt             int64             `json:"savedAt,omitempty"`
}

// TerminalSnapshot 终端面板 UI 状态快照。
//
// 与 model.TerminalSession（运行时会话，含 pty/cmd 句柄与互斥锁）区分：
// 本类型仅持久化用，不持有进程资源。崩溃恢复时前端据 WorkDir 新建终端会话，
// 不真实复用旧进程（旧 sessionID 失效，前端用 WorkDir 重新 CreateTerminal）。
type TerminalSnapshot struct {
	// Visible 终端面板是否展开。
	Visible bool `json:"visible,omitempty"`
	// Height 终端面板高度（像素），恢复面板尺寸。
	Height int `json:"height,omitempty"`
	// WorkDir 终端工作目录，恢复时新建会话以此为目标目录。
	WorkDir string `json:"workDir,omitempty"`
}
