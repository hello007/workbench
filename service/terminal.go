package service

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// TerminalService 终端服务，管理终端会话的创建、输入、切换目录、调整大小和关闭
type TerminalService struct {
	sinkHolder // 事件出口持有器（terminal-output/terminal-exit 推送），构造注入 + serve 模式经 SetEventSink 切换
	sessions   map[string]*model.TerminalSession
	mu         sync.Mutex

	// 会话级 watch 停止信号 + 退出确认：CloseTerminal/CloseAll close(done) 让
	// watchProcess 立即退出（不等 500ms 轮询周期），watch goroutine defer
	// close(watchExit) 反向确认，关闭入口 <-watchExit 等待且仅等待本会话 watch
	// 退出——per-session chan 无服务级共享计数（共享 WaitGroup 会让双 tab 关其一
	// 的等待被在册会话的 watch 计数永久阻塞），PTY Close 只断 ConPTY 句柄不杀
	// 子进程，watch 不得依赖进程退出自证退出（goleak 硬门禁实测抓出的驻留面）
	watchDones map[string]chan struct{}
	watchExits map[string]chan struct{}
}

// NewTerminalService 创建终端服务实例。
// ctx 为 Wails 上下文，用于构造事件出口；传 nil（单测等无 Wails 上下文场景）
// 时事件推送静默跳过（防护集中在 EventSink，修复原直调 EventsEmit 的 fatal 隐患）。
func NewTerminalService(ctx context.Context) *TerminalService {
	s := &TerminalService{
		sessions:   make(map[string]*model.TerminalSession),
		watchDones: make(map[string]chan struct{}),
		watchExits: make(map[string]chan struct{}),
	}
	s.SetEventSink(NewWailsEventSink(ctx))
	return s
}

// CreateTerminal 创建终端会话
func (s *TerminalService) CreateTerminal(dir, shellType, customPath string, cols, rows uint16) (string, error) {
	config := s.resolveShellConfig(shellType, customPath)

	ptyProc, err := util.NewPtyProcess(config.Executable, config.Args, dir, cols, rows)
	if err != nil {
		return "", fmt.Errorf("创建终端失败: %w", err)
	}

	sessionID := fmt.Sprintf("term-%d", time.Now().UnixNano())
	session := &model.TerminalSession{
		ID:        sessionID,
		Dir:       dir,
		ShellType: shellType,
		Running:   true,
	}
	session.SetRunning(true)

	storePtyProcess(session, ptyProc)

	s.mu.Lock()
	s.sessions[sessionID] = session
	done := make(chan struct{})
	exit := make(chan struct{})
	s.watchDones[sessionID] = done
	s.watchExits[sessionID] = exit
	s.mu.Unlock()

	go s.startOutputPump(sessionID, ptyProc)
	go func() {
		defer close(exit)
		s.watchProcess(sessionID, ptyProc, done)
	}()

	return sessionID, nil
}

// WriteInput 向终端写入用户输入
func (s *TerminalService) WriteInput(sessionID, input string) error {
	session, ptyProc, err := s.getSessionAndPty(sessionID)
	if err != nil {
		return err
	}
	if !session.IsRunning() {
		return fmt.Errorf("终端会话 %s 已停止", sessionID)
	}
	_, err = ptyProc.Write([]byte(input))
	return err
}

// ChangeDir 切换终端工作目录
func (s *TerminalService) ChangeDir(sessionID, dir string) error {
	session, ptyProc, err := s.getSessionAndPty(sessionID)
	if err != nil {
		return err
	}
	if !session.IsRunning() {
		return fmt.Errorf("终端会话 %s 已停止", sessionID)
	}
	cdCmd, err := s.buildCdCommand(dir, session.ShellType)
	if err != nil {
		// 路径不合法（如含换行符，见 buildPosixCdCommand）时不写 PTY，错误透传前端提示
		return err
	}
	_, err = ptyProc.Write([]byte(cdCmd))
	if err != nil {
		return err
	}
	session.Dir = dir
	return nil
}

// Resize 调整终端窗口大小
func (s *TerminalService) Resize(sessionID string, cols, rows uint16) error {
	_, ptyProc, err := s.getSessionAndPty(sessionID)
	if err != nil {
		return err
	}
	return ptyProc.Resize(cols, rows)
}

// CloseTerminal 关闭终端会话
func (s *TerminalService) CloseTerminal(sessionID string) error {
	s.mu.Lock()
	session, exists := s.sessions[sessionID]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("终端会话 %s 不存在", sessionID)
	}
	delete(s.sessions, sessionID)
	done := s.watchDones[sessionID]
	exit := s.watchExits[sessionID]
	delete(s.watchDones, sessionID)
	delete(s.watchExits, sessionID)
	s.mu.Unlock()

	session.SetRunning(false)

	ptyProc := getPtyProcess(session)
	if ptyProc != nil {
		ptyProc.Close()
	}
	// 关闭信号立即唤醒本会话 watchProcess（不等轮询周期），等待其退出确认返回：
	// per-session chan 无共享计数，双 tab 场景关其一不耦合他会话；输出泵经 Read
	// 断开自行退出，不在等待面
	if done != nil {
		close(done)
		<-exit
	}
	return nil
}

// CloseAll 关闭所有终端会话
func (s *TerminalService) CloseAll() {
	s.mu.Lock()
	sessions := make([]*model.TerminalSession, 0, len(s.sessions))
	dones := make([]chan struct{}, 0, len(s.watchDones))
	exits := make([]chan struct{}, 0, len(s.watchExits))
	for id, session := range s.sessions {
		sessions = append(sessions, session)
		if done, ok := s.watchDones[id]; ok {
			dones = append(dones, done)
		}
		if exit, ok := s.watchExits[id]; ok {
			exits = append(exits, exit)
		}
	}
	s.sessions = make(map[string]*model.TerminalSession)
	s.watchDones = make(map[string]chan struct{})
	s.watchExits = make(map[string]chan struct{})
	s.mu.Unlock()

	for _, session := range sessions {
		session.SetRunning(false)
		ptyProc := getPtyProcess(session)
		if ptyProc != nil {
			ptyProc.Close()
		}
	}
	// 同 CloseTerminal：全部关闭信号立即唤醒各自 watchProcess，逐个等退出确认
	for _, done := range dones {
		close(done)
	}
	for _, exit := range exits {
		<-exit
	}
}

// resolveShellConfig 解析 Shell 配置
func (s *TerminalService) resolveShellConfig(shellType, customPath string) *model.ShellConfig {
	return model.ResolveShellConfig(shellType, customPath)
}

// isUnixShellType 判断 Shell 类型是否为 POSIX shell（bash/zsh/fish/sh，
// 与 model.GetShellConfigs 非 Windows 分支对齐）。cd 命令须按 POSIX 单引号语法输出。
// 非 Windows 平台上的未知类型兜底按 POSIX 处理，避免向 Unix shell 输出 Windows 语法（cd /d）。
func isUnixShellType(shellType string) bool {
	switch shellType {
	case "bash", "zsh", "fish", "sh":
		return true
	}
	if runtime.GOOS == "windows" {
		return false
	}
	switch shellType {
	case "powershell", "cmd", "gitbash", "wsl":
		return false
	}
	return true
}

// buildCdCommand 根据 Shell 类型构建 cd 命令
// POSIX shell（bash/zsh/fish/sh）: cd -- '<path>'（fish 省略 `--`，见 buildPosixCdCommand）
// CMD: cd /d "path"（/d 标志切换驱动器+目录）
// PowerShell: cd "path"（自动处理驱动器切换）
// Git Bash: cd "path"（反斜杠转正斜杠）
// WSL: cd "/mnt/x/path"（Windows 路径转 WSL 挂载路径）
// 错误返回：POSIX 分支路径含换行符时拒绝生成命令（见 buildPosixCdCommand），由 ChangeDir
// 透传前端；Windows 分支恒返回 nil（Windows 文件名不允许含换行，无对应注入面）
func (s *TerminalService) buildCdCommand(dir string, shellType string) (string, error) {
	normalizedDir := filepath.Clean(dir)

	if isUnixShellType(shellType) {
		// ToSlash 归一为 POSIX 分隔符：Linux 上无操作；
		// Windows 上误配 bash 类型时 Clean 产生的反斜杠转回正斜杠（Git Bash 兼容）
		return buildPosixCdCommand(filepath.ToSlash(normalizedDir), shellType)
	}

	switch shellType {
	case "cmd":
		// CMD: /d 标志用于同时切换驱动器和目录
		return fmt.Sprintf(`cd /d "%s"`, normalizedDir) + "\r", nil
	case "gitbash":
		// Git Bash: 无 /d 标志，反斜杠转正斜杠
		unixDir := strings.ReplaceAll(normalizedDir, `\`, `/`)
		return fmt.Sprintf(`cd "%s"`, unixDir) + "\r", nil
	case "wsl":
		// WSL: D:\path → /mnt/d/path
		wslDir := toWslPath(normalizedDir)
		return fmt.Sprintf(`cd "%s"`, wslDir) + "\r", nil
	default:
		// PowerShell 及其他: 无需 /d 标志
		return fmt.Sprintf(`cd "%s"`, normalizedDir) + "\r", nil
	}
}

// toWslPath 将 Windows 路径转换为 WSL 挂载路径
// D:\work → /mnt/d/work，C:\ → /mnt/c/
func toWslPath(path string) string {
	if len(path) >= 2 && path[1] == ':' {
		drive := strings.ToLower(string(path[0]))
		rest := strings.ReplaceAll(path[3:], `\`, `/`)
		return fmt.Sprintf("/mnt/%s/%s", drive, rest)
	}
	return strings.ReplaceAll(path, `\`, `/`)
}

// buildPosixCdCommand 构建 POSIX shell 的 cd 命令：cd -- '<path>'（fish 为 cd '<path>'）。
// `--` 防止以 - 开头的路径被解析为选项；单引号包裹路径，路径内单引号按 POSIX
// 规则转义（结束引号、转义引号、重开引号）：
//
//	'\''
//
// 示例须置于缩进码块——gofmt ≥1.19 的 doc comment 规范化会把行文中的连续两个
// 单引号改写为 Unicode 右弯引号，破坏转义序列的字面表达。
// G2 fish 兼容结论（防御分支）：fish 的 cd 为函数（share/functions/cd.fish）包装
// builtin cd；依 fish 3.0（2018-12 发布）changelog，3.0 起所有 builtin 统一支持 `--`
// 终止选项解析，更早版本（2.x）对 `--` 的处理不一致（会把 `--` 当目录名报错）。
// 本任务实现环境无法在线复核 fishshell.com 文档原文，且用户环境 fish 版本运行时不可控，
// 故按 PRD 防御路径：对 fish 省略 `--` 输出 cd '<path>'（该形式全版本 fish 必然有效）。
// 代价：fish 下以 - 开头的罕见路径会被误判为选项而 cd 失败（无注入风险，属可接受降级）；
// bash/zsh/sh 及非 Windows 未知类型兜底保持 cd -- '<path>'。
// G1 换行防护：路径含 \n/\r 时返回错误拒绝生成命令——Linux 文件名合法含换行，
// 单引号包裹无法阻止 PTY 按行分割，cd 截断后剩余片段会被 shell 当作独立命令执行
// （注入面）；错误经 ChangeDir 透传前端提示路径不合法。
// 末尾保留 \r（回车），与 Windows 分支语义一致：PTY 中回车触发命令执行。
func buildPosixCdCommand(dir, shellType string) (string, error) {
	if strings.ContainsAny(dir, "\n\r") {
		return "", fmt.Errorf("路径含换行符，无法在终端中安全切换: %q", dir)
	}
	escaped := strings.ReplaceAll(dir, "'", `'\''`)
	if shellType == "fish" {
		return "cd '" + escaped + "'\r", nil
	}
	return "cd -- '" + escaped + "'\r", nil
}

// startOutputPump 输出泵，持续读取 PTY 输出并经事件出口推送给前端
func (s *TerminalService) startOutputPump(sessionID string, ptyProc *util.PtyProcess) {
	buf := make([]byte, 4096)
	for {
		n, err := ptyProc.Read(buf)
		if err != nil {
			s.mu.Lock()
			session, exists := s.sessions[sessionID]
			s.mu.Unlock()
			if exists {
				session.SetRunning(false)
				s.emitCurrent("terminal-exit", sessionID)
			}
			return
		}
		if n > 0 {
			s.emitCurrent("terminal-output", sessionID, string(buf[:n]))
		}
	}
}

// watchProcess 监控进程退出。退出条件双通道：进程退出（生产主路径，仍在册会话
// 发 terminal-exit）或关闭信号（CloseTerminal/CloseAll close done 立即触发，静默
// 退出——terminal-exit 由关闭入口自身语义决定不发）。PTY Close 只断 ConPTY 句柄
// 不杀子进程，交互 shell 可在句柄断开后驻留，watch 不得依赖 IsProcessRunning
// 翻转自证退出（goleak 硬门禁实测抓出的驻留面）；done 信号让关闭路径零轮询周期
// 等待，会话存在性检查兜底 close 前的窗口。
func (s *TerminalService) watchProcess(sessionID string, ptyProc *util.PtyProcess, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-time.After(500 * time.Millisecond):
		}
		s.mu.Lock()
		session, exists := s.sessions[sessionID]
		s.mu.Unlock()
		if !exists || !session.IsRunning() {
			return
		}
		if !ptyProc.IsProcessRunning() {
			// emit 前持锁复验存在性：上方读 map 到此处 emit 的窗口内会话可能已被
			// CloseTerminal/CloseAll 移除（关闭入口自身不发 terminal-exit，前端
			// 已收到关闭语义），复验收紧误发窗口
			s.mu.Lock()
			_, stillExists := s.sessions[sessionID]
			s.mu.Unlock()
			if stillExists {
				session.SetRunning(false)
				s.emitCurrent("terminal-exit", sessionID)
			}
			return
		}
	}
}

// getSessionAndPty 获取会话和 PTY 进程
func (s *TerminalService) getSessionAndPty(sessionID string) (*model.TerminalSession, *util.PtyProcess, error) {
	s.mu.Lock()
	session, exists := s.sessions[sessionID]
	s.mu.Unlock()
	if !exists {
		return nil, nil, fmt.Errorf("终端会话 %s 不存在", sessionID)
	}
	ptyProc := getPtyProcess(session)
	if ptyProc == nil {
		return nil, nil, fmt.Errorf("终端会话 %s PTY 进程不可用", sessionID)
	}
	return session, ptyProc, nil
}

// --- PTY 进程存储辅助 ---

var ptyStore sync.Map

func storePtyProcess(session *model.TerminalSession, proc *util.PtyProcess) {
	ptyStore.Store(session.ID, proc)
}

func getPtyProcess(session *model.TerminalSession) *util.PtyProcess {
	val, ok := ptyStore.Load(session.ID)
	if !ok {
		return nil
	}
	proc, ok := val.(*util.PtyProcess)
	if !ok {
		return nil
	}
	return proc
}
