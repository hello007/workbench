package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"workbench/model"
	"workbench/util"
)

// 进程管理编排常量。
//
// 枚举超时：netstat 实测 0.075s、tasklist 实测 ~1.0s（research/process-enumeration.md），
// 10s 为宽松上限，覆盖高负载与慢盘场景。
// 停止轮询：PRD 已定「优雅请求 → 轮询上限 5s（间隔 300~500ms）→ 未退自动转强杀」。
const (
	processEnumTimeout  = 10 * time.Second
	processStopTimeout  = 5 * time.Second
	processStopInterval = 400 * time.Millisecond
)

// 检索模式取值。用**普通 string** 而非具名类型：wails generate 不为 `type X string`
// 生成 models.ts 类型别名，App.d.ts 引用后 npm run build 会报 MISSING_EXPORT
// （vitest 走 esbuild 不报类型错，极易漏，见 docs/spec/cross-layer-contracts.md）。
const (
	ProcessFilterModeAuto = "auto" // 自动：纯数字按端口，含非数字按进程名
	ProcessFilterModePort = "port" // 强制按端口精确匹配
	ProcessFilterModeName = "name" // 强制按进程名关键字模糊匹配
)

// ProcessProvider 进程枚举与终止能力抽象。
//
// 生产实现委托 util 包平台分叉函数（Windows netstat/tasklist/taskkill，Linux /proc + 信号）；
// 单测注入 fake——真实枚举结果随本机常驻进程变化（实测 36115 常被 workbench 自身占用），
// 依赖真实进程的断言属环境外部状态、必然 flaky（docs/spec/test-stability.md）。
type ProcessProvider interface {
	ListProcesses(ctx context.Context) ([]model.ProcessInfo, error)
	IsSelfProcess(pid int) bool
	RequestProcessStop(pid int, killTree bool) (bool, error)
	KillProcess(pid int, killTree bool) error
	IsProcessAlive(pid int) bool
}

// utilProcessProvider 生产实现，转发到 util 包同名平台分叉函数。
type utilProcessProvider struct{}

func (utilProcessProvider) ListProcesses(ctx context.Context) ([]model.ProcessInfo, error) {
	return util.ListProcesses(ctx)
}

func (utilProcessProvider) IsSelfProcess(pid int) bool {
	return util.IsSelfProcess(pid)
}

func (utilProcessProvider) RequestProcessStop(pid int, killTree bool) (bool, error) {
	return util.RequestProcessStop(pid, killTree)
}

func (utilProcessProvider) KillProcess(pid int, killTree bool) error {
	return util.KillProcess(pid, killTree)
}

func (utilProcessProvider) IsProcessAlive(pid int) bool {
	return util.IsProcessAlive(pid)
}

// ProcessService 进程管理服务（工具箱·进程管理）：枚举检索 + 自我保护 + 停止/强杀编排。
type ProcessService struct {
	provider ProcessProvider
	// 优雅停止轮询参数（超时上限与间隔），测试可注入以便直接驱动超时分支。
	stopTimeout  time.Duration
	stopInterval time.Duration
	// 时间注入点：单测注入假时钟与空 sleep，避免真实等待（docs/spec/test-stability.md
	// 禁 sleep 等待真实时间）。
	now   func() time.Time
	sleep func(time.Duration)
}

// NewProcessService 创建进程管理服务（生产路径：真实 util 实现 + 真实时钟）。
func NewProcessService() *ProcessService {
	return &ProcessService{
		provider:     utilProcessProvider{},
		stopTimeout:  processStopTimeout,
		stopInterval: processStopInterval,
		now:          time.Now,
		sleep:        time.Sleep,
	}
}

// ListProcesses 检索进程列表。
//
// keyword 为空（或仅空白）时列出全部处于监听状态的进程（PRD R4：默认视图）。
// mode 取值 auto / port / name，空串与未知值按 auto 处理。
func (s *ProcessService) ListProcesses(keyword, mode string) ([]model.ProcessInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), processEnumTimeout)
	defer cancel()

	all, err := s.provider.ListProcesses(ctx)
	if err != nil {
		Logger().Error("list processes failed", "keyword", keyword, "mode", mode, "err", err)
		return nil, err
	}
	return filterProcesses(all, keyword, normalizeProcessFilterMode(mode)), nil
}

// StopProcess 停止进程：发优雅停止请求 → 轮询存活上限 5s → 未退自动转强杀（PRD 已定的全自动两级）。
//
// 返回 ProcessStopResult.Method 如实反映最终生效手段：优雅请求能否生效取决于目标进程是否
// 响应，无法按进程类型事前预判（本机实证：ping 响应退出、cmd /k 主动忽略且仍存活），故不做
// 类型预判，统一「发请求 → 轮询实测存活 → 未退转强杀」，UI 不得谎称已优雅退出。
func (s *ProcessService) StopProcess(pid int, killTree bool) (*model.ProcessStopResult, error) {
	if err := s.checkTerminable(pid); err != nil {
		return nil, err
	}

	delivered, err := s.provider.RequestProcessStop(pid, killTree)
	if err != nil {
		Logger().Warn("request process stop failed", "pid", pid, "err", err)
		return nil, err
	}
	if !delivered {
		// 优雅请求被目标拒绝（Windows 报「只能强制终止」，如 cmd /k 为保持交互主动忽略）：
		// 直接转强杀，不做无意义的轮询等待
		Logger().Info("graceful stop request rejected by target, force kill", "pid", pid, "killTree", killTree)
		if err := s.provider.KillProcess(pid, killTree); err != nil {
			return nil, err
		}
		return &model.ProcessStopResult{Stopped: true, Method: model.ProcessStopMethodForced}, nil
	}

	if s.waitForExit(pid) {
		return &model.ProcessStopResult{Stopped: true, Method: model.ProcessStopMethodGraceful}, nil
	}
	// 优雅阶段超时：不产生面向用户的错误码（全自动转强杀），仅后端留痕
	Logger().Warn("graceful stop timed out, force kill", "pid", pid, "timeout", s.stopTimeout, "killTree", killTree)
	if err := s.provider.KillProcess(pid, killTree); err != nil {
		return nil, err
	}
	return &model.ProcessStopResult{Stopped: true, Method: model.ProcessStopMethodForced}, nil
}

// KillProcess 强制终止进程，跳过优雅阶段。killTree 为 true 时连带子进程。
func (s *ProcessService) KillProcess(pid int, killTree bool) error {
	if err := s.checkTerminable(pid); err != nil {
		return err
	}
	if err := s.provider.KillProcess(pid, killTree); err != nil {
		Logger().Warn("kill process failed", "pid", pid, "err", err)
		return err
	}
	return nil
}

// checkTerminable 终止入口的后端最终防线：拒绝终止本应用自身与父进程链。
// 前端按钮禁用可被绕过（直调绑定方法），后端必须再判一次（PRD L1 硬禁）。
func (s *ProcessService) checkTerminable(pid int) error {
	if s.provider.IsSelfProcess(pid) {
		Logger().Warn("refuse to terminate self process", "pid", pid)
		return model.NewAppError(model.ErrCodeProcessSelfProtect, "不能结束 WorkBench 自身或其父进程")
	}
	return nil
}

// waitForExit 轮询目标进程是否已退出，超时返回 false（轮询期间由注入的 sleep 间隔让出）。
func (s *ProcessService) waitForExit(pid int) bool {
	deadline := s.now().Add(s.stopTimeout)
	for {
		if !s.provider.IsProcessAlive(pid) {
			return true
		}
		if !s.now().Before(deadline) {
			return false
		}
		s.sleep(s.stopInterval)
	}
}

// normalizeProcessFilterMode 归一化检索模式：空串与未知值回落 auto（前端下拉值不受信任）。
func normalizeProcessFilterMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ProcessFilterModePort:
		return ProcessFilterModePort
	case ProcessFilterModeName:
		return ProcessFilterModeName
	default:
		return ProcessFilterModeAuto
	}
}

// filterProcesses 按关键字与模式过滤进程列表（纯函数，fixture 单测入口）。
//
// 模式判定：auto 下纯数字关键字按端口精确匹配，含非数字字符按进程名模糊匹配（PRD R2）。
// 进程名匹配须兼容无端口进程——util 枚举基线含不监听端口的进程（端口检索才需要端口）。
func filterProcesses(all []model.ProcessInfo, keyword, mode string) []model.ProcessInfo {
	keyword = strings.TrimSpace(keyword) // 纯空白与空串同语义（前端可能传入未裁剪的输入框值）
	if keyword == "" {
		return filterListeningProcesses(all)
	}
	if mode == ProcessFilterModePort || (mode == ProcessFilterModeAuto && isNumericKeyword(keyword)) {
		return filterByPort(all, keyword)
	}
	return filterByName(all, keyword)
}

// filterByPort 按端口号精确匹配。关键字非合法端口（mode 强制 port 但输入非数字）时返回空列表。
func filterByPort(all []model.ProcessInfo, keyword string) []model.ProcessInfo {
	port, err := strconv.Atoi(keyword)
	if err != nil || port < 0 {
		return nil
	}
	result := make([]model.ProcessInfo, 0, len(all))
	for _, info := range all {
		for _, p := range info.Ports {
			if p.Port == port {
				result = append(result, info)
				break
			}
		}
	}
	return result
}

// filterByName 按进程名关键字模糊匹配（大小写不敏感）。
func filterByName(all []model.ProcessInfo, keyword string) []model.ProcessInfo {
	lower := strings.ToLower(keyword)
	result := make([]model.ProcessInfo, 0, len(all))
	for _, info := range all {
		if strings.Contains(strings.ToLower(info.Name), lower) {
			result = append(result, info)
		}
	}
	return result
}

// filterListeningProcesses 空关键字（默认视图）的保留判定：只看「谁在占端口」。
//
// 保留规则（防御式，避免默认视图整体空列表）：
//   - 无端口的进程 → 排除（默认视图是端口视图，这类进程只在进程名检索下命中）
//   - 存在监听态端口 → 保留，端口裁剪为仅监听端口（混入 ESTABLISHED 会淹没列表）
//   - 端口状态全部无法识别 → **保守保留且不裁剪端口**
//   - 端口状态全部可识别但都不是监听态 → 排除
//
// 第二、三条的区分是防御核心：Windows netstat 的状态列随系统 UI 语言本地化
// （中文系统为「监听」等），若把无法识别的状态一律当「非监听」排除，默认视图
// 在非英文系统上会退化为空列表——属严重降级，故以「保留」为安全方向。
func filterListeningProcesses(all []model.ProcessInfo) []model.ProcessInfo {
	result := make([]model.ProcessInfo, 0, len(all))
	for _, info := range all {
		if len(info.Ports) == 0 {
			continue
		}
		listening := make([]model.ProcessPort, 0, len(info.Ports))
		hasUnknownState := false
		for _, p := range info.Ports {
			if isListeningState(p.State) {
				listening = append(listening, p)
				continue
			}
			if !isKnownPortState(p.State) {
				hasUnknownState = true
			}
		}
		if len(listening) == 0 && !hasUnknownState {
			continue // 状态全部可识别且都不是监听态
		}
		filtered := info
		if len(listening) > 0 {
			filtered.Ports = listening // 新切片，不改调用方底层数组
		}
		// 全部状态无法识别时保留完整端口列表：裁剪会直接裁成空列表
		result = append(result, filtered)
	}
	return result
}

// isListeningState 判定端口状态是否为监听态（两平台状态名不同，见 filterListeningProcesses）。
func isListeningState(state string) bool {
	switch normalizePortState(state) {
	case "LISTENING", "LISTEN":
		return true
	}
	return false
}

// isKnownPortState 判定端口状态是否为「已知连接态」——即能否确认它**不是**监听态。
//
// 不在集合内的取值包含两类：本地化状态名（中文系统的「监听」）与新版本新增状态，
// 两者都不能判定为非监听，须由调用方保守保留。空串（Windows netstat 的 UDP 行无
// State 列）同样属无法识别，而非「非监听」。
func isKnownPortState(state string) bool {
	return knownPortStates[normalizePortState(state)]
}

// normalizePortState 归一化状态名：去空白 + 转大写（Windows 与 Linux 的大小写风格不同）。
func normalizePortState(state string) string {
	return strings.ToUpper(strings.TrimSpace(state))
}

// knownPortStates 已知连接状态名集合，覆盖 Windows netstat 与 Linux /proc/net/tcp
// 内核 tcp_states 枚举两平台的取值（含 Windows 的 FIN_WAIT_1 / SYN_RECEIVED 变体）。
var knownPortStates = map[string]bool{
	"LISTENING":    true,
	"LISTEN":       true,
	"ESTABLISHED":  true,
	"SYN_SENT":     true,
	"SYN_SEND":     true,
	"SYN_RECV":     true,
	"SYN_RECEIVED": true,
	"FIN_WAIT1":    true,
	"FIN_WAIT_1":   true,
	"FIN_WAIT2":    true,
	"FIN_WAIT_2":   true,
	"TIME_WAIT":    true,
	"CLOSE":        true,
	"CLOSED":       true,
	"CLOSE_WAIT":   true,
	"LAST_ACK":     true,
	"CLOSING":      true,
	"DELETE_TCB":   true,
	"NEW_SYN_RECV": true,
	"UNKNOWN":      true,
}

// isNumericKeyword 判定关键字是否全为数字（非空）。空串已在调用方按「空输入」分流。
func isNumericKeyword(keyword string) bool {
	for _, r := range keyword {
		if r < '0' || r > '9' {
			return false
		}
	}
	return keyword != ""
}
