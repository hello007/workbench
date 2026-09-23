package model

// SessionStateVersion 会话快照格式版本，便于未来字段迁移时识别旧快照并降级处理。
//
// 版本历史：
//   - "1"：单终端快照（TerminalSnapshot 仅 visible/height/workDir 三字段）。
//   - "2"：多终端 tab 快照（TerminalSnapshot 增加 tabs/activeIndex/fullscreen）。
//
// 降级路径（v1 -> v2）：版本 "1" 快照的 workDir 单值经 TerminalSnapshot.NormalizedTabs
// 降级为单元素 Tabs（ShellType 留空，前端回退设置页默认 shell）。识别不依赖 Version 字段，
// 而按字段形态判断（Tabs 空且 WorkDir 非空即旧快照），保证版本号写入前的历史文件同样可读；
// 归一化幂等，Load 一次再 Save 即落盘为 v2 形态。
const SessionStateVersion = "2"

// SessionState 崩溃恢复用 UI 状态快照，持久化到 data/session.json。
//
// 设计原则（MVP 边界，见 .trellis/tasks/09-14-v1-4/prd.md PR2 与
// .trellis/tasks/09-22-terminal-multi-session-and-ui/prd.md R4）：
//   - 仅恢复 UI 状态（当前工作目录、活动面板、终端面板可见性/高度/多 tab 列表）。
//   - 不恢复未提交代码 / 编辑器未保存内容（超 MVP）。
//   - 文件树展开状态已由前端 useTreeState 持久化到 localStorage（按工作目录隔离），
//     恢复 selectedDirectoryId 后前端按路径从 localStorage 还原，不纳入本快照避免重复。
//   - 终端不真实复用进程，仅恢复 tab 配置（目录/shell 类型），由前端逐 tab 新建会话。
//
// 字段对齐前端 Pinia store 实际结构（见 frontend/src/store/{directory,ui}.js）：
//   - SelectedDirectoryID <-> directoryStore.selectedDirectoryId
//   - ActivePanel <-> uiStore.activePanel（'directory' | 'ai' | 'ai-chat' | 'toolbox' | 'dashboard' | 'stats'，
//     与前端 useSessionState VALID_PANELS 白名单及 ActivityBar items 严格对齐）
//   - Terminal <-> uiStore.terminalVisible / terminalHeight / 终端 tab 快照镜像
//     （uiStore.terminalTabsSnapshot / terminalActiveIndex，由 TerminalPanel 同步）
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

// TerminalTabSnapshot 单个终端 tab 的恢复信息（多终端会话，每 tab 一个独立 shell 进程）。
type TerminalTabSnapshot struct {
	// WorkDir 该 tab 的终端工作目录，恢复时新建会话以此为目标目录。
	WorkDir string `json:"workDir,omitempty"`
	// ShellType 该 tab 的 shell 类型（powershell/cmd/gitbash/wsl）。
	// 旧版快照降级生成的单 tab 此字段为空，前端回退设置页默认 shell。
	ShellType string `json:"shellType,omitempty"`
}

// TerminalSnapshot 终端面板 UI 状态快照。
//
// 与 model.TerminalSession（运行时会话，含 pty/cmd 句柄与互斥锁）区分：
// 本类型仅持久化用，不持有进程资源。崩溃恢复时前端按 Tabs 列表循环新建终端会话，
// 不真实复用旧进程（旧 sessionID 失效，前端逐 tab 重新 CreateTerminal）。
type TerminalSnapshot struct {
	// Visible 终端面板是否展开。
	Visible bool `json:"visible,omitempty"`
	// Height 终端面板高度（像素），恢复面板尺寸。
	Height int `json:"height,omitempty"`
	// WorkDir 旧版（v1）单终端快照的工作目录，仅作旧数据兼容读取源；
	// 新快照不再写入该字段，终端目录统一经 Tabs 承载。
	// 保留字段使 JSON 反序列化天然兼容旧 session.json（无需迁移脚本）。
	WorkDir string `json:"workDir,omitempty"`
	// Tabs 多终端 tab 列表（v2 起生效）。恢复时前端按列表循环新建会话，
	// 超前端上限（useTerminalTabs 的 MAX_TERMINAL_TABS = 8）截断；
	// 后端不截断——上限常量单一来源在前端，避免双处维护。
	Tabs []TerminalTabSnapshot `json:"tabs,omitempty"`
	// ActiveIndex 恢复时激活的 tab 下标（0 基）。omitempty 在 0 值时省略字段，
	// 反序列化仍得 0，无损。越界值由前端 clamp 到 [0, len(tabs)-1]。
	ActiveIndex int `json:"activeIndex,omitempty"`
	// Fullscreen 终端整窗全屏态（PR4 整窗全屏功能消费）。
	// 快照结构随多 tab 升版先行落地，避免 PR4 二次改 model 与 wailsjs 绑定。
	Fullscreen bool `json:"fullscreen,omitempty"`
}

// NormalizedTabs 返回归一化后的 tab 列表，统一新旧快照形态：
//   - v2 快照（Tabs 非空）：直接返回 Tabs，忽略 WorkDir（新结构优先）；
//   - v1 旧快照（Tabs 空且 WorkDir 非空）：降级为单 tab，ShellType 留空由前端回退默认值；
//   - 两者皆空：返回 nil（无可恢复终端）。
//
// 幂等：对归一化结果再次调用结果不变，Load 归一化写回后重复 Load 无副作用。
func (t *TerminalSnapshot) NormalizedTabs() []TerminalTabSnapshot {
	if t == nil {
		return nil
	}
	if len(t.Tabs) > 0 {
		return t.Tabs
	}
	if t.WorkDir != "" {
		return []TerminalTabSnapshot{{WorkDir: t.WorkDir}}
	}
	return nil
}
