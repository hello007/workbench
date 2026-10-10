// model/process.go
package model

// ProcessPort 进程占用的单个网络端口条目（一个进程可占用多个端口）。
type ProcessPort struct {
	Port      int    `json:"port"`      // 端口号
	Protocol  string `json:"protocol"`  // 协议，取值 TCP / UDP（TCPv6、UDPv6 归并为基础协议名）
	State     string `json:"state"`     // 连接状态，如 LISTENING / ESTABLISHED；UDP 无状态列时为空串
	LocalAddr string `json:"localAddr"` // 本地地址（不含端口），如 "0.0.0.0" / "0000:...:0001"
}

// ProcessInfo 进程管理列表单行。
//
// IsSelf / IsSystem 由后端填充随列表下发，前端只消费不自判——避免前后端两套判据
// 漂移（自我保护判据只能用 PID，禁止按进程名：E2E 会把 exe 改名 workbench-e2e.exe
// 做实例隔离，见 docs/spec/e2e-testing.md）。
type ProcessInfo struct {
	Pid      int           `json:"pid"`      // 进程 ID
	Name     string        `json:"name"`     // 进程名（Windows 为映像名如 node.exe，Linux 为 comm）
	Ports    []ProcessPort `json:"ports"`    // 该进程占用的端口列表，无端口时为空
	MemoryKB int64         `json:"memoryKB"` // 内存占用（KB），不可读时为 0
	IsSelf   bool          `json:"isSelf"`   // 本应用自身或父进程链：前端禁用终止按钮，后端终止入口再拒一次
	IsSystem bool          `json:"isSystem"` // 系统关键进程：前端走红色强确认
}

// ProcessStopResult 进程停止结果。
//
// Method 如实反映最终生效手段：Windows 上 taskkill 不带 /F 会向控制台进程投递控制信号、
// 向窗口进程投递关闭消息，**能否生效取决于目标进程是否响应**，无法按进程类型事前预判
// （本机实证：`ping -n 300` 响应并退出；`cmd /k` 为保持交互会话主动忽略，报「只能强制终止」
// 且进程仍存活——见 research/process-enumeration.md 第 3 节实测更正）。故统一走「发请求 →
// 5s 轮询实测存活 → 未退则转强杀」，结果按实测如实报 graceful / forced，UI 不得谎称已优雅退出。
type ProcessStopResult struct {
	Stopped bool   `json:"stopped"` // 最终是否已终止
	Method  string `json:"method"`  // 生效手段：ProcessStopMethodGraceful / ProcessStopMethodForced
}

// 停止手段常量，service 填充 ProcessStopResult.Method 与前端提示文案分流共用。
const (
	ProcessStopMethodGraceful = "graceful" // 优雅退出生效：请求已送达且轮询实测进程确已退出（Windows 关闭请求 / Linux SIGTERM 被目标响应）
	ProcessStopMethodForced   = "forced"   // 强杀生效（直接强杀，或优雅请求被目标拒绝、优雅阶段轮询超时后降级）
)
