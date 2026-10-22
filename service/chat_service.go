package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"workbench/model"
	"workbench/util"
)

// ===== 常量 =====

// chatIdleTimeout 会话进程空闲回收时限默认值：无执行轮且队列为空超过此时长，
// watchdog 强杀进程；下次发消息重新 spawn 并 --resume 续上下文。
// 构造时快照进 service 实例字段（测试注入短时限改实例字段，不改本 var——
// watchdog goroutine 与测试 defer 恢复全局变量存在竞态窗口）。
var chatIdleTimeout = 10 * time.Minute

// chatTurnTimeout 单轮执行超时默认值：执行轮超过此时长无 result，watchdog 发送
// interrupt 中断，被中断轮以 error_during_execution result 收尾；再超一倍
// 时长仍无收尾则强杀进程兜底。快照语义同上。
var chatTurnTimeout = 10 * time.Minute

// chatIdleScanInterval watchdog 扫描间隔默认值。快照语义同上。
var chatIdleScanInterval = 15 * time.Second

// chatMaxConcurrent 同时活跃的会话进程上限：每个常驻 claude 进程占一个槽位，
// spawn 时非阻塞获取，达到上限后新会话首条消息直接报错（ErrCodeChatProcessLimit）
// 引导用户等待——常驻进程排队等待意味着槽位被长期占用，不做排队。
// 与 AI 功能任务的 aiTaskMaxConcurrent 独立计数，互不挤占槽位。
const chatMaxConcurrent = 4

// chatIndexSchemaVersion sessions.json 当前 schema 版本。
// 字段演进时 +1，并在加载处追加迁移分支。
const chatIndexSchemaVersion = 1

// chatTaskIDSeq 任务 id 原子递增序号：UnixNano 在同一 tick 内可能重复（并发
// RunChat 或循环内连续创建），同 tick 覆盖 s.tasks 同键任务；追加单调序号保证唯一。
var chatTaskIDSeq atomic.Int64

// newChatTaskID 生成对话任务 id（chattask-<unixnano>-<seq>，前端视为不透明字符串）。
func newChatTaskID() string {
	return fmt.Sprintf("chattask-%d-%d", time.Now().UnixNano(), chatTaskIDSeq.Add(1))
}

// saveChatJSON 原子写 ChatService 域 JSON 文件：把现有旧文件复制为 <目标>.bak
// （保留上一好版本，替换后新文件若损坏仍有回退），原子核心（同目录 temp → fsync →
// rename 原子替换）委托 util.SaveJSON——temp+rename 管道已全仓统一，此处不再自建。
// .bak 为写侧保险，无加载方按精确路径消费（loadIndex 损坏时回空索引并另留时间戳留底）。
func saveChatJSON(filePath string, v interface{}) error {
	// 覆写前备份上一好版本（当前文件不存在时跳过）
	if old, rerr := os.ReadFile(filePath); rerr == nil {
		_ = os.WriteFile(filePath+".bak", old, 0o644)
	}
	return util.SaveJSON(filePath, v)
}

// ===== 子进程抽象（测试注入 fake，不真调 claude CLI）=====

// chatProcess 对话子进程抽象：生产包装 exec.Cmd，测试注入 fake 流式输出。
// 持续会话进程模型下抽象面覆盖 stdout 读（泵解析）、stdin 写（user 消息与
// control_response 应答注入）、启动/等待/终止/退出码。
type chatProcess interface {
	// StdoutPipe 返回子进程标准输出流（stderr 已并入该流）。
	StdoutPipe() (io.ReadCloser, error)
	// StdinPipe 返回子进程标准输入流（stream-json input 模式的消息写入端）。
	StdinPipe() (io.WriteCloser, error)
	// Start 启动子进程（非阻塞，等待经 Wait）。
	Start() error
	// Wait 阻塞等待进程退出并回收资源。
	Wait() error
	// Kill 终止进程及其全部子进程树（claude 可能再 spawn MCP 等子进程）。
	Kill()
	// ExitCode 进程退出码；未退出或未启动返回 -1。
	ExitCode() int
}

// chatProcessFactory 按 (ctx, name, args, dir) 构造进程实例。
// 生产实现 newExecChatProcess；测试注入捕获参数并回放预设输出行的 fake。
type chatProcessFactory func(ctx context.Context, name string, args []string, dir string) chatProcess

// execChatProcess chatProcess 的生产实现，包装 exec.Cmd。
type execChatProcess struct {
	cmd *exec.Cmd
}

// newExecChatProcess 生产进程工厂：构造 exec.Cmd 并隐藏 Windows 控制台窗口。
// cmd.Cancel 定制 ctx 超时/取消时的终止方式：CommandContext 默认只 Kill 根进程，
// claude spawn 的 MCP 等子进程会成孤儿；走 killProcessTree 整树终止（与手动
// 取消路径一致）。Cancel 在进程仍存活时被调用，整树杀真正生效。
func newExecChatProcess(ctx context.Context, name string, args []string, dir string) chatProcess {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Cancel = func() error {
		killProcessTree(cmd)
		return nil
	}
	util.HideCommandWindow(cmd)
	return &execChatProcess{cmd: cmd}
}

// StdoutPipe 返回输出管道，并把 stderr 并入同一流（claude 的诊断信息随行输出，
// 与 RunStage 行为一致：非 JSON 行经 parseStreamLine 原样作为文本增量透传）。
func (p *execChatProcess) StdoutPipe() (io.ReadCloser, error) {
	rc, err := p.cmd.StdoutPipe()
	if err == nil {
		p.cmd.Stderr = p.cmd.Stdout
	}
	return rc, err
}

// StdinPipe 返回子进程标准输入流（stream-json input 模式消息写入端）。
func (p *execChatProcess) StdinPipe() (io.WriteCloser, error) {
	return p.cmd.StdinPipe()
}

func (p *execChatProcess) Start() error { return p.cmd.Start() }

func (p *execChatProcess) Wait() error { return p.cmd.Wait() }

// Kill 终止进程树（Windows 下 claude 可能 spawn MCP 等子进程，须整树杀）。
func (p *execChatProcess) Kill() { killProcessTree(p.cmd) }

// ExitCode 进程退出码；未退出（ProcessState 为 nil）返回 -1。
func (p *execChatProcess) ExitCode() int {
	if p.cmd.ProcessState == nil {
		return -1
	}
	return p.cmd.ProcessState.ExitCode()
}

// ===== 运行时状态 =====

// chatTurnRuntime 单轮对话（一条 user 消息 → 一个 result 事件）的在途状态。
// 排队（chatSessionRuntime.queue）与执行（chatSessionRuntime.executing）两态
// 共用本结构，终态（done 事件发出）后从 ChatService.turns 摘除。
type chatTurnRuntime struct {
	id              string
	chatSessionID   string          // 所属 WorkBench 会话 id
	prompt          string          // 用户原文（落盘用；claude stdin 写入见 advanceSession 的声明注入）
	startedAt       time.Time       // stdin 写入时刻（轮超时起算）
	reply           strings.Builder // 本轮 assistant 回复累积（持锁写）
	canceled        bool            // 用户取消：执行中标记后发 interrupt，排队直接摘除
	timedOut        bool            // watchdog 判定轮超时（done.Error 展示超时文案）
	finished        bool            // 终态收尾单次认领：result 收尾（pump）与 stdin 写失败收尾（advanceSession）为并发双 finalizer，先持锁认领者发 done，防同轮双 done
	claudeSessionID string          // claude 会话 id（result 事件提取，done 载荷回传）
}

// chatSessionRuntime 一个会话的常驻 claude 进程运行时：进程存活期间承载该会话
// 全部轮次（运行中插话 = 追加排队，前序轮 result 后依序写入 stdin）。
// spawn 参数（权限模式/模型）为启动快照：变更且空闲时优雅重启，忙时沿用旧值。
type chatSessionRuntime struct {
	chatSessionID   string
	permissionMode  string // spawn 参数快照
	modelName       string // spawn 参数快照
	proc            chatProcess
	stdin           io.WriteCloser
	ctx             context.Context
	cancel          context.CancelFunc
	queue           []*chatTurnRuntime // Go 侧排队轮次（FIFO，前序轮 result 后才写 stdin，可取消）
	executing       *chatTurnRuntime   // 当前执行轮（nil = 空闲）
	claudeSessionID string             // claude 会话 id：spawn 时预载会话元数据（respawn --resume 依据），init/result 事件刷新；兼作 chat-question 声明注入判定（空 = claude 会话首条消息）
	lastActive      time.Time          // 最近轮次活动时刻（空闲回收起算）
	semRelease      sync.Once          // 进程槽位归还单次保护（pump 收尾与 spawn 失败清理双路径）
}

// chatApproval 待应答的工具授权请求（claude control_request/can_use_tool）。
// 生命周期：pump 解析到请求注册 → 前端弹窗应答（RespondChatApproval 写 stdin
// control_response）→ 或所属轮结束/进程退出时自动拒绝（settleApprovals）。
type chatApproval struct {
	requestID     string
	chatSessionID string
	turnID        string
	toolName      string
	input         map[string]any
	description   string
}

// ChatService AI 对话服务：多会话持续式对话的会话 CRUD、消息持久化与
// 自由 prompt 对话执行器（RunChat）。
//
// 执行模型（持续会话进程，网页版 Claude Code 体验）：
//   - 每会话一个常驻 claude 进程（claude -p --input-format stream-json
//     --output-format stream-json --verbose --permission-prompt-tool stdio），
//     stdin 写入驱动轮次，运行中插话 = 追加 Go 侧排队（CLI 侧逐轮串行处理）；
//   - stream-json 流式解析：assistant 文本增量推送 chat-task:output，每轮一个
//     result 事件收尾（先落盘后 done），system/init 提取 claude session id；
//   - 工具授权审批走 control_request/can_use_tool 控制协议：请求推送
//     chat-approval:request 事件，前端弹窗应答经 RespondChatApproval 写回
//     control_response（按 request_id 匹配）；轮结束/进程退出自动拒绝未决审批；
//   - 生命周期：空闲（无执行轮且队列空）超时由 watchdog 强杀回收，下次发消息
//     spawn 并 --resume 续上下文；进程槽位上限 chatMaxConcurrent；
//   - 会话已有 ClaudeSessionID 时 spawn 自动 --resume 续上下文（进程崩溃恢复）；
//   - 每轮完成后把 user 消息与 assistant 回复追加进会话消息文件，并把 claude
//     session id 更新回会话元数据；
//   - 事件前缀 chat-task:*（轮次）与 chat-approval:*（审批），与 AI 功能面板事件流隔离。
//
// 持久化布局（独立 data/ai_chat/ 目录，不与其他 service 文件交叉）：
//   - data/ai_chat/sessions.json           会话索引（元数据，不含消息）
//   - data/ai_chat/messages/<sessionId>.json 每会话消息数组
//   - data/ai_chat/directories.json        侧栏常用目录项（AI 对话入口列表）
type ChatService struct {
	ctx        context.Context
	sinkHolder        // 事件出口持有器（chat-task:*/chat-approval:* 事件推送），构造注入 + serve 模式经 SetEventSink 切换
	dataDir    string // 存储根目录 data/ai_chat/
	mu         sync.Mutex
	sessions   map[string]*chatSessionRuntime // 常驻进程注册表（chatSessionID → 运行时）
	turns      map[string]*chatTurnRuntime    // 在途轮次注册表（taskID → 轮；终态/失败即摘除）
	approvals  map[string]*chatApproval       // 待应答审批（requestID → 审批）
	procSem    chan struct{}                  // 进程槽位（缓冲 = chatMaxConcurrent），spawn 时非阻塞获取、进程退出归还
	processFactory chatProcessFactory         // 子进程工厂（测试注入 fake）

	// 生命周期参数（构造时从全局默认取值快照；测试注入短时限改实例字段，
	// 不改全局 var——watchdog goroutine 与测试 defer 恢复全局变量存在竞态窗口）
	idleTimeout  time.Duration // 进程空闲回收时限
	turnTimeout  time.Duration // 单轮执行超时
	scanInterval time.Duration // watchdog 扫描间隔
}

// NewChatService 创建 AI 对话服务。dataDir 为存储根目录（data/ai_chat/）。
func NewChatService(ctx context.Context, dataDir string) *ChatService {
	s := &ChatService{
		ctx:            ctx,
		dataDir:        dataDir,
		sessions:       make(map[string]*chatSessionRuntime),
		turns:          make(map[string]*chatTurnRuntime),
		approvals:      make(map[string]*chatApproval),
		procSem:        make(chan struct{}, chatMaxConcurrent),
		processFactory: newExecChatProcess,
		idleTimeout:    chatIdleTimeout,
		turnTimeout:    chatTurnTimeout,
		scanInterval:   chatIdleScanInterval,
	}
	s.SetEventSink(NewWailsEventSink(ctx))
	return s
}

// setProcessFactory 注入进程工厂（测试用，生产走构造默认值）。
func (s *ChatService) setProcessFactory(f chatProcessFactory) {
	s.processFactory = f
}

// ===== 存储路径 =====

// indexPath 会话索引文件路径（data/ai_chat/sessions.json）。
func (s *ChatService) indexPath() string {
	return filepath.Join(s.dataDir, "sessions.json")
}

// messagesDir 消息目录（data/ai_chat/messages/）。
func (s *ChatService) messagesDir() string {
	return filepath.Join(s.dataDir, "messages")
}

// messagesFilePath 单会话消息文件路径（data/ai_chat/messages/<sessionId>.json）。
func (s *ChatService) messagesFilePath(sessionID string) string {
	return filepath.Join(s.messagesDir(), sessionID+".json")
}

// ===== 会话 CRUD =====

// chatSessionIndex sessions.json 顶层结构（含 schema 版本，迁移预留）。
type chatSessionIndex struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Sessions      []*model.ChatSession `json:"sessions"`
}

// loadIndex 读会话索引。文件不存在返回空索引；损坏时备份原文件后回空索引
// （消息文件仍保留，不因索引损坏丢失对话内容），并 slog.Warn 记录根因。
func (s *ChatService) loadIndex() (*chatSessionIndex, error) {
	idx := &chatSessionIndex{SchemaVersion: chatIndexSchemaVersion}
	if !util.FileExists(s.indexPath()) {
		return idx, nil
	}
	raw, err := os.ReadFile(s.indexPath())
	if err == nil {
		// 解析错误覆写 err：日志须记录真实根因（ReadFile 成功时 err 为 nil，
		// 不覆写则 Warn 落 err=null 丢失定位信息）
		if uerr := json.Unmarshal(raw, idx); uerr != nil {
			err = uerr
		}
	}
	if err == nil {
		return idx, nil
	}
	// 损坏：备份留底后回空索引（读错误与解析错误统一走此分支）
	bakPath := s.indexPath() + ".bak." + time.Now().Format("20060102-150405")
	if raw != nil {
		_ = os.WriteFile(bakPath, raw, 0o644)
	}
	Logger().Warn("chat session index corrupt, fallback to empty index",
		"path", s.indexPath(), "err", err)
	return &chatSessionIndex{SchemaVersion: chatIndexSchemaVersion}, nil
}

// saveIndex 落盘会话索引（原子写，防崩溃截断）。
func (s *ChatService) saveIndex(idx *chatSessionIndex) error {
	idx.SchemaVersion = chatIndexSchemaVersion
	if err := saveChatJSON(s.indexPath(), idx); err != nil {
		return fmt.Errorf("保存会话索引失败: %w", err)
	}
	return nil
}

// CreateChatSession 创建会话并落盘索引。title 为空时默认「新会话」。
func (s *ChatService) CreateChatSession(directoryID, title, cwd string) (*model.ChatSession, error) {
	if strings.TrimSpace(title) == "" {
		title = "新会话"
	}
	now := time.Now()
	sess := &model.ChatSession{
		ID:          fmt.Sprintf("chatsession-%d", now.UnixNano()),
		DirectoryID: directoryID,
		Title:       title,
		Cwd:         cwd,
		CreatedAt:   now.UnixMilli(),
		UpdatedAt:   now.UnixMilli(),
		Messages:    []model.ChatMessage{},
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	idx.Sessions = append(idx.Sessions, sess)
	if err := s.saveIndex(idx); err != nil {
		return nil, err
	}
	return sess, nil
}

// ListChatSessions 列出指定目录的会话（按最近活跃降序）。
// directoryID 为空返回全部目录的会话。返回项不含消息（轻量列表，
// 消息经 GetChatSession 按需加载）。
func (s *ChatService) ListChatSessions(directoryID string) ([]*model.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	var list []*model.ChatSession
	for _, sess := range idx.Sessions {
		if sess == nil {
			continue
		}
		if directoryID != "" && sess.DirectoryID != directoryID {
			continue
		}
		list = append(list, sess)
	}
	// 最近活跃在前；稳定排序保持同时间戳时的插入序
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].UpdatedAt > list[i].UpdatedAt {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	return list, nil
}

// GetChatSession 读取会话完整内容（元数据 + 消息）。
// 会话不存在返回 ErrCodeChatSessionNotFound；消息文件缺失按空消息处理。
func (s *ChatService) GetChatSession(sessionID string) (*model.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	for _, sess := range idx.Sessions {
		if sess != nil && sess.ID == sessionID {
			msgs, err := s.readMessages(sessionID)
			if err != nil {
				return nil, err
			}
			out := *sess
			out.Messages = msgs
			return &out, nil
		}
	}
	return nil, model.NewAppError(model.ErrCodeChatSessionNotFound, "会话不存在或已被删除")
}

// DeleteChatSession 删除会话（索引项 + 消息文件）。会话不存在返回 ErrCodeChatSessionNotFound。
func (s *ChatService) DeleteChatSession(sessionID string) error {
	s.mu.Lock()
	idx, err := s.loadIndex()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	found := false
	kept := idx.Sessions[:0]
	for _, sess := range idx.Sessions {
		if sess != nil && sess.ID == sessionID {
			found = true
			continue
		}
		kept = append(kept, sess)
	}
	if !found {
		s.mu.Unlock()
		return model.NewAppError(model.ErrCodeChatSessionNotFound, "会话不存在或已被删除")
	}
	idx.Sessions = kept
	if err := s.saveIndex(idx); err != nil {
		s.mu.Unlock()
		return err
	}
	// 多会话并行模型：终止该会话常驻进程（在途轮次失败收尾、未决审批清理、
	// 进程槽位归还由 pump 收尾统一处理）；进程不存在（从未对话/已空闲回收）时无操作。
	// 先摘索引再杀进程：进程收尾的落盘路径会话已不可见，按 SessionNotFound 预期降级。
	rt := s.sessions[sessionID]
	s.mu.Unlock()
	if rt != nil {
		s.killSession(rt, "会话删除")
	}
	// 消息文件删除失败仅记录（索引已删，残留文件无消费方，下次同 id 概率趋零）
	if err := os.Remove(s.messagesFilePath(sessionID)); err != nil && !os.IsNotExist(err) {
		Logger().Warn("delete chat messages file failed", "session", sessionID, "err", err)
	}
	return nil
}

// UpdateChatSessionTitle 修改会话标题并刷新活跃时间。
func (s *ChatService) UpdateChatSessionTitle(sessionID, title string) error {
	return s.updateSessionMeta(sessionID, func(sess *model.ChatSession) {
		sess.Title = title
	})
}

// updateSessionMeta 更新单个会话元数据（持锁调用形态，fn 内改字段后统一落盘）。
// UpdatedAt 统一刷新为当前时间（任何元数据变更都视为会话活跃）。
func (s *ChatService) updateSessionMeta(sessionID string, fn func(sess *model.ChatSession)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	for _, sess := range idx.Sessions {
		if sess != nil && sess.ID == sessionID {
			fn(sess)
			sess.UpdatedAt = time.Now().UnixMilli()
			return s.saveIndex(idx)
		}
	}
	return model.NewAppError(model.ErrCodeChatSessionNotFound, "会话不存在或已被删除")
}

// ===== 目录项 CRUD（AI 对话侧栏常用目录）=====

// chatDirectorySchemaVersion directories.json 当前 schema 版本（迁移预留）。
const chatDirectorySchemaVersion = 1

// chatDirectoryIndex directories.json 顶层结构。
type chatDirectoryIndex struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Directories   []*model.ChatDirectory `json:"directories"`
}

// directoriesPath 目录项索引文件路径（data/ai_chat/directories.json）。
func (s *ChatService) directoriesPath() string {
	return filepath.Join(s.dataDir, "directories.json")
}

// loadDirectoriesIndex 读目录项索引。文件不存在返回空索引；损坏时备份原文件后
// 回空索引（与会话索引同款降级，不阻塞使用），并 slog.Warn 记录根因。
func (s *ChatService) loadDirectoriesIndex() (*chatDirectoryIndex, error) {
	idx := &chatDirectoryIndex{SchemaVersion: chatDirectorySchemaVersion}
	if !util.FileExists(s.directoriesPath()) {
		return idx, nil
	}
	raw, err := os.ReadFile(s.directoriesPath())
	if err == nil {
		// 解析错误覆写 err：保证 Warn 记录真实根因（同 loadIndex）
		if uerr := json.Unmarshal(raw, idx); uerr != nil {
			err = uerr
		}
	}
	if err == nil {
		return idx, nil
	}
	bakPath := s.directoriesPath() + ".bak." + time.Now().Format("20060102-150405")
	if raw != nil {
		_ = os.WriteFile(bakPath, raw, 0o644)
	}
	Logger().Warn("chat directory index corrupt, fallback to empty index",
		"path", s.directoriesPath(), "err", err)
	return &chatDirectoryIndex{SchemaVersion: chatDirectorySchemaVersion}, nil
}

// saveDirectoriesIndex 落盘目录项索引（原子写，防崩溃截断）。
func (s *ChatService) saveDirectoriesIndex(idx *chatDirectoryIndex) error {
	idx.SchemaVersion = chatDirectorySchemaVersion
	if err := saveChatJSON(s.directoriesPath(), idx); err != nil {
		return fmt.Errorf("保存目录列表失败: %w", err)
	}
	return nil
}

// chatDirectoryKey 目录去重键：小写 + 正斜杠归一。Windows 路径大小写不敏感，
// 前端 containsPath 已按小写比较，后端去重键同样归一，避免同路径不同大小写
// 产生重复目录项。
func chatDirectoryKey(path string) string {
	return strings.ToLower(filepath.ToSlash(path))
}

// AddChatDirectory 添加常用目录项。displayName 为空时取路径末段目录名。
// 路径须为已存在的目录（claude 子进程以此为 cwd）；重复添加（去重键相同，
// 大小写不敏感）返回既有项（幂等，前端提前查重给提示，此处兜底不产生重复项）。
func (s *ChatService) AddChatDirectory(path, displayName string) (*model.ChatDirectory, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("目录路径不能为空")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("目录不存在: %s", path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = filepath.Base(absPath)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadDirectoriesIndex()
	if err != nil {
		return nil, err
	}
	dedupeKey := chatDirectoryKey(absPath)
	for _, dir := range idx.Directories {
		if dir != nil && chatDirectoryKey(dir.Path) == dedupeKey {
			return dir, nil
		}
	}
	now := time.Now()
	item := &model.ChatDirectory{
		ID:          fmt.Sprintf("chatdir-%d", now.UnixNano()),
		Path:        absPath,
		DisplayName: displayName,
		SortOrder:   len(idx.Directories),
		CreatedAt:   now.UnixMilli(),
	}
	idx.Directories = append(idx.Directories, item)
	if err := s.saveDirectoriesIndex(idx); err != nil {
		return nil, err
	}
	return item, nil
}

// ListChatDirectories 列出常用目录项（按 SortOrder 升序，稳定排序保持同序插入位）。
func (s *ChatService) ListChatDirectories() ([]*model.ChatDirectory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadDirectoriesIndex()
	if err != nil {
		return nil, err
	}
	list := make([]*model.ChatDirectory, 0, len(idx.Directories))
	for _, dir := range idx.Directories {
		if dir != nil {
			list = append(list, dir)
		}
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].SortOrder < list[i].SortOrder {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	return list, nil
}

// UpdateChatDirectory 修改目录项显示名。项不存在返回错误。
func (s *ChatService) UpdateChatDirectory(id, displayName string) error {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return fmt.Errorf("显示名称不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadDirectoriesIndex()
	if err != nil {
		return err
	}
	for _, dir := range idx.Directories {
		if dir != nil && dir.ID == id {
			dir.DisplayName = displayName
			return s.saveDirectoriesIndex(idx)
		}
	}
	return fmt.Errorf("目录项不存在")
}

// RemoveChatDirectory 移除目录项。项不存在返回错误（幂等性由前端确认交互保证）。
func (s *ChatService) RemoveChatDirectory(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadDirectoriesIndex()
	if err != nil {
		return err
	}
	found := false
	kept := idx.Directories[:0]
	for _, dir := range idx.Directories {
		if dir != nil && dir.ID == id {
			found = true
			continue
		}
		kept = append(kept, dir)
	}
	if !found {
		return fmt.Errorf("目录项不存在")
	}
	idx.Directories = kept
	return s.saveDirectoriesIndex(idx)
}

// ReorderChatDirectories 按 ids 顺序重写目录项 SortOrder（侧栏拖拽重排持久化）。
// ids 未覆盖的项（防御局部更新）按原相对顺序排尾部；最终 SortOrder 归一为
// 0..n-1，避免新旧序号交错产生并列排序歧义。
func (s *ChatService) ReorderChatDirectories(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadDirectoriesIndex()
	if err != nil {
		return err
	}
	pos := make(map[string]int, len(ids))
	for i, id := range ids {
		pos[id] = i
	}
	byID := make(map[string]*model.ChatDirectory, len(idx.Directories))
	var rest []*model.ChatDirectory
	for _, dir := range idx.Directories {
		if dir == nil {
			continue
		}
		if _, ok := pos[dir.ID]; ok {
			byID[dir.ID] = dir
		} else {
			rest = append(rest, dir)
		}
	}
	// 按 ids 顺序取项（重复 id 经 delete 防御只取一次）；
	// rest 保持文件序（即既有 SortOrder 升序）稳定排尾部
	ordered := make([]*model.ChatDirectory, 0, len(idx.Directories))
	for _, id := range ids {
		if dir, ok := byID[id]; ok {
			ordered = append(ordered, dir)
			delete(byID, id)
		}
	}
	ordered = append(ordered, rest...)
	for i, dir := range ordered {
		dir.SortOrder = i
	}
	return s.saveDirectoriesIndex(idx)
}

// ===== 消息持久化 =====

// readMessages 读单会话消息数组（文件不存在或为空返回空数组非 nil）。
// 调用方须持 s.mu（与 appendMessages 写互斥）。
func (s *ChatService) readMessages(sessionID string) ([]model.ChatMessage, error) {
	path := s.messagesFilePath(sessionID)
	if !util.FileExists(path) {
		return []model.ChatMessage{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取会话消息失败: %w", err)
	}
	var msgs []model.ChatMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		// 消息损坏：备份留底后按空数组降级（对话历史不可恢复但功能可用）
		Logger().Warn("chat messages file corrupt, fallback to empty",
			"path", path, "err", err)
		_ = os.WriteFile(path+".bak."+time.Now().Format("20060102-150405"), raw, 0o644)
		return []model.ChatMessage{}, nil
	}
	if msgs == nil {
		msgs = []model.ChatMessage{}
	}
	return msgs, nil
}

// appendMessages 追加消息到会话消息文件（读-合并-整体写回）。
func (s *ChatService) appendMessages(sessionID string, msgs ...model.ChatMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, err := s.readMessages(sessionID)
	if err != nil {
		return err
	}
	existing = append(existing, msgs...)
	if err := saveChatJSON(s.messagesFilePath(sessionID), existing); err != nil {
		return fmt.Errorf("写入会话消息失败: %w", err)
	}
	return nil
}

// ===== 对话执行 =====

// chatQuestionPreamble 首轮注入 claude prompt 的交互环境声明：headless -p 模式
// 下 AskUserQuestion 工具不渲染界面且被自动应答（模型拿不到真实选择），已通过
// --disallowedTools 禁用；此处约定模型以 chat-question 代码块承载选择题，
// WorkBench 前端将其渲染为可点选卡片，用户选择结果作为下一条用户消息回传。
// 仅首轮注入（--resume 续轮上下文已含本声明，重复注入徒增 token）。
const chatQuestionPreamble = `【交互环境说明】你运行在非交互对话环境中，AskUserQuestion 工具已被禁用。当你需要用户在有限选项中做单选或多选决策时，不要用文字罗列选项让用户打字回复，而是输出一个 chat-question 代码块并立即结束本轮回复（代码块之后不要再输出任何内容），用户会在交互卡片上点选，选择结果将作为下一条消息发给你。代码块格式（JSON，questions 数组支持一次提出多个问题）：
` + "```chat-question" + `
{"questions":[{"question":"问题文本","options":[{"label":"选项一"},{"label":"选项二"}],"multiSelect":false}]}
` + "```" + `
不需要用户选择时正常对话，不要输出该代码块。`

// buildChatArgs 组装 claude 持续会话进程命令参数（纯函数，便于单测）。
// 形态：-p --input-format stream-json --output-format stream-json --verbose
// [--resume <sid>] [--permission-mode <mode>] [--model <model>]
// --disallowedTools AskUserQuestion --permission-prompt-tool stdio。
//   - -p + --input-format stream-json：流式输入模式必须配 -p（CLI 硬约束），
//     进程长驻；prompt 不走命令行，经 stdin 逐条写入（advanceSession）。
//   - --resume：重启/恢复场景续 claude 上下文（claudeSessionID 来自会话元数据
//     或 init/result 事件；首轮新会话不传）。
//   - --permission-mode：仅非 default 非空时追加（spawn 快照，变更且空闲时重启进程）。
//   - --model：仅非空时追加（空 = claude 默认模型）。
//   - --disallowedTools AskUserQuestion：headless 流式下该工具仍被 CLI 自动应答
//     （模型基于「用户未作答」的错误前提继续输出），禁用后配合首条消息声明
//     （chatQuestionPreamble）引导模型改用 chat-question 代码块承载选择题。
//   - --permission-prompt-tool stdio：开启权限审批控制协议（control_request/
//     can_use_tool 经 stdout 下发、control_response 经 stdin 应答）。**漏配 =
//     审批静默自动拒绝**（无任何报错，CLI 直接走 permission_denied，模型自行放弃），
//     参数组装必须单点维护、禁止在任何调用点旁路重建参数。
func buildChatArgs(resumeSessionID, permissionMode, modelName string) []string {
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose"}
	if resumeSessionID != "" {
		args = append(args, "--resume", resumeSessionID)
	}
	if permissionMode != "" && permissionMode != model.ChatPermissionModeDefault {
		args = append(args, "--permission-mode", permissionMode)
	}
	if modelName != "" {
		args = append(args, "--model", modelName)
	}
	args = append(args, "--disallowedTools", "AskUserQuestion", "--permission-prompt-tool", "stdio")
	return args
}

// writeChatStdinLine 向 claude stdin 写入一行协议消息（stream-json input /
// control 协议均为按行分隔的 JSON）。
func writeChatStdinLine(w io.Writer, line string) error {
	_, err := io.WriteString(w, line+"\n")
	return err
}

// chatUserStdinLine 构造写入 claude stdin 的 user 消息行（stream-json input
// 协议实测载荷形态：content 为 text 块数组，parent_tool_use_id 恒 null）。
func chatUserStdinLine(text string) string {
	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": []map[string]any{{"type": "text", "text": text}},
		},
		"parent_tool_use_id": nil,
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// chatControlResponseLine 构造 control_response 应答行（权限审批 allow/deny
// 双形态，按 request_id 匹配）。allow 恒回传 updatedInput（原 input 原样透传）——
// 旧版 CLI 对 allow 缺 updatedInput 判校验错误拒绝。
func chatControlResponseLine(requestID string, allow bool, denyMessage string, updatedInput map[string]any) string {
	inner := map[string]any{}
	if allow {
		inner["behavior"] = "allow"
		inner["updatedInput"] = updatedInput
	} else {
		inner["behavior"] = "deny"
		inner["message"] = denyMessage
	}
	payload := map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": requestID,
			"response":   inner,
		},
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// chatInterruptLine 构造 interrupt 控制请求行（中断当前执行轮，CLI 回执后
// 被中断轮以 error_during_execution result 收尾）。
func chatInterruptLine(requestID string) string {
	payload := map[string]any{
		"type":       "control_request",
		"request_id": requestID,
		"request":    map[string]any{"subtype": "interrupt"},
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// RunChat 向会话提交一轮对话：把 prompt 写入该会话常驻 claude 进程的 stdin
// （进程未起则先 spawn），流式解析回复并推送 chat-task:* 事件。
// 返回任务 id（供事件对号与取消）；进程无法启动同步报错。
//
// 运行中插话（排队注入）：同会话已有执行/排队轮次时不再拒绝（原
// ErrCodeChatInProgress 语义退役），轮次进入该会话的 Go 侧 FIFO 队列，前序轮
// result 后依序写入 stdin——CLI 侧轮次天然逐轮串行，--resume 上下文与消息文件
// 写入不交错；前序轮 result 前取消可直接摘除（CancelChatTask）。
//
// 事件序列（每轮）：chat-task:queued → chat-task:started → chat-task:output（增量）
// → chat-task:done（ChatTaskRunResult，含全文回复与 claude session id）。
func (s *ChatService) RunChat(chatSessionID, prompt, permissionMode, modelName string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", model.NewAppError(model.ErrCodeChatEmptyPrompt, "对话内容不能为空")
	}
	sess, err := s.GetChatSession(chatSessionID)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	rt := s.sessions[chatSessionID]
	if rt != nil && (rt.permissionMode != permissionMode || rt.modelName != modelName) && rt.executing == nil && len(rt.queue) == 0 {
		// 权限模式/模型变更且进程空闲：优雅重启（spawn 参数为启动快照，运行中
		// 无法变更）；忙时（执行轮/排队轮存在）沿用旧参数继续，不中断在途对话。
		s.mu.Unlock()
		s.restartSessionIdle(rt)
		s.mu.Lock()
		rt = s.sessions[chatSessionID]
	}
	if rt == nil {
		// 进程槽位非阻塞获取：达到上限直接报错引导用户（常驻进程排队等待
		// 意味着槽位被长期占用，不做排队）
		select {
		case s.procSem <- struct{}{}:
		default:
			s.mu.Unlock()
			return "", model.NewAppError(model.ErrCodeChatProcessLimit,
				fmt.Sprintf("同时活跃的对话进程已达上限（%d），请等待其他会话空闲后再发送", chatMaxConcurrent))
		}
		rt = &chatSessionRuntime{
			chatSessionID:   chatSessionID,
			permissionMode:  permissionMode,
			modelName:       modelName,
			claudeSessionID: sess.ClaudeSessionID, // 预载会话元数据：respawn 自动 --resume；空 = claude 会话首条消息
			lastActive:      time.Now(),
		}
		s.sessions[chatSessionID] = rt
	}
	turn := &chatTurnRuntime{
		id:            newChatTaskID(),
		chatSessionID: chatSessionID,
		prompt:        prompt,
	}
	rt.queue = append(rt.queue, turn)
	s.turns[turn.id] = turn
	s.mu.Unlock()
	s.emitCurrent("chat-task:queued", map[string]any{"taskId": turn.id, "chatSessionId": chatSessionID})

	s.mu.Lock()
	if rt.proc == nil {
		if err := s.spawnSessionProcess(rt, sess.Cwd); err != nil {
			// 启动失败：摘注册表、归还槽位；调用方本轮同步报错（不发 done，
			// 与既有启动失败契约一致），同进程其他排队轮次发 done 收尾
			if cur := s.sessions[chatSessionID]; cur == rt {
				delete(s.sessions, chatSessionID)
			}
			queued := rt.queue
			rt.queue = nil
			for _, t := range queued {
				delete(s.turns, t.id)
			}
			rt.semRelease.Do(func() { <-s.procSem })
			s.mu.Unlock()
			wrapped := fmt.Errorf("启动 claude 失败（请确认已安装并在 PATH 中）: %w", err)
			for _, t := range queued {
				if t.id == turn.id {
					continue
				}
				s.emitCurrent("chat-task:done", model.ChatTaskRunResult{
					TaskID: t.id, ChatSessionID: t.chatSessionID, Error: wrapped.Error(),
				})
			}
			return "", wrapped
		}
	}
	s.mu.Unlock()

	if err := s.advanceSession(rt); err != nil {
		// stdin 写入失败（进程已死）：该轮已发 done 失败收尾，同步报错给调用方
		return "", err
	}
	return turn.id, nil
}

// CancelChatTask 取消对话轮次，区分两态：
//   - 排队中（Go 侧队列，尚未写 stdin）：从队列摘除并立即 emit done（canceled），
//     该轮 user 消息不落盘（排队取消不留痕）
//   - 执行中：标记 canceled 并向 claude stdin 发送 interrupt 控制请求，被中断轮
//     以 error_during_execution result 收尾（finishExecutingTurn 按 canceled
//     优先级构造 done，部分回复保留入档）；interrupt 写入失败（进程已死）时
//     轮次由 pump 收尾兜底
//
// 幂等：重复取消直接返回 true，不重复发 interrupt、不重复 emit done。
func (s *ChatService) CancelChatTask(taskID string) bool {
	s.mu.Lock()
	turn, ok := s.turns[taskID]
	if !ok {
		s.mu.Unlock()
		return false
	}
	if turn.canceled {
		// 已取消过（interrupt 已发或排队已摘除），幂等返回
		s.mu.Unlock()
		return true
	}
	turn.canceled = true
	rt := s.sessions[turn.chatSessionID]
	executing := rt != nil && rt.executing == turn
	if !executing {
		// 排队中：从队列摘除并出锁后发 done
		if rt != nil {
			kept := rt.queue[:0]
			for _, t := range rt.queue {
				if t.id != taskID {
					kept = append(kept, t)
				}
			}
			rt.queue = kept
		}
		delete(s.turns, taskID)
		s.mu.Unlock()
		s.emitCurrent("chat-task:done", model.ChatTaskRunResult{
			TaskID:        taskID,
			ChatSessionID: turn.chatSessionID,
			Canceled:      true,
			Error:         "已取消（排队中）",
		})
		return true
	}
	// 执行中：发 interrupt 中断当前轮（CLI 回执后 result 收尾）
	stdin := rt.stdin
	s.mu.Unlock()
	if stdin != nil {
		if err := writeChatStdinLine(stdin, chatInterruptLine("chat-cancel-"+taskID)); err != nil {
			// 进程已死：轮次由 pump 收尾（finishExecutingTurn 读 canceled 标记）
			Logger().Warn("send chat interrupt failed", "task", taskID, "err", err)
		}
	}
	return true
}

// GetChatTaskState 查询对话轮次状态（前端恢复/轮询兜底用）。
// 轮次不存在返回 nil（前端按终态处理）。
func (s *ChatService) GetChatTaskState(taskID string) *model.ChatTaskState {
	s.mu.Lock()
	defer s.mu.Unlock()
	turn, ok := s.turns[taskID]
	if !ok {
		return nil
	}
	rt := s.sessions[turn.chatSessionID]
	executing := rt != nil && rt.executing == turn
	return &model.ChatTaskState{
		TaskID:          turn.id,
		ChatSessionID:   turn.chatSessionID,
		Running:         executing,
		Queued:          !executing,
		ClaudeSessionID: turn.claudeSessionID,
		Reply:           turn.reply.String(),
		StartedAt:       turn.startedAt.UnixMilli(),
	}
}

// ListChatActiveTasks 列出会话全部在途轮次（排队+执行中，按开始时间升序），
// 前端面板切回时恢复任务态用（持续会话下同一会话可有多条在途轮次，
// 单任务查询 GetChatTaskState 不再够用）。chatSessionID 为空返回全部会话。
func (s *ChatService) ListChatActiveTasks(chatSessionID string) []model.ChatTaskState {
	s.mu.Lock()
	list := make([]*chatTurnRuntime, 0, len(s.turns))
	for _, turn := range s.turns {
		if chatSessionID != "" && turn.chatSessionID != chatSessionID {
			continue
		}
		list = append(list, turn)
	}
	s.mu.Unlock()
	out := make([]model.ChatTaskState, 0, len(list))
	for _, turn := range list {
		if st := s.GetChatTaskState(turn.id); st != nil {
			out = append(out, *st)
		}
	}
	// 开始时间升序（执行轮在前，排队未开始的零值时间排后——与队列推进顺序一致）
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if chatTaskSortKey(out[j]) < chatTaskSortKey(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// chatTaskSortKey 轮次排序键：未开始（StartedAt=0）视为最大值排尾部。
func chatTaskSortKey(st model.ChatTaskState) int64 {
	if st.StartedAt <= 0 {
		return math.MaxInt64
	}
	return st.StartedAt
}

// RespondChatApproval 应答工具授权审批：allow=true 放行（回传原 input 作
// updatedInput——旧版 CLI 对 allow 缺 updatedInput 判校验错误）；false 拒绝
// （模型收到拒绝原因后可调整方案）。请求不存在或已失效（所属轮结束/进程退出
// 时已自动拒绝）报错。
func (s *ChatService) RespondChatApproval(requestID string, allow bool) error {
	s.mu.Lock()
	ap, ok := s.approvals[requestID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("审批请求不存在或已失效")
	}
	rt := s.sessions[ap.chatSessionID]
	if rt == nil || rt.stdin == nil {
		delete(s.approvals, requestID)
		s.mu.Unlock()
		return fmt.Errorf("对话进程已退出，审批请求已失效")
	}
	stdin := rt.stdin
	input := ap.input
	delete(s.approvals, requestID)
	s.mu.Unlock()

	var line string
	if allow {
		line = chatControlResponseLine(requestID, true, "", input)
	} else {
		line = chatControlResponseLine(requestID, false, "用户在审批弹窗中拒绝了该工具调用", nil)
	}
	if err := writeChatStdinLine(stdin, line); err != nil {
		return fmt.Errorf("写入审批应答失败: %w", err)
	}
	return nil
}

// CloseAll 应用退出时终止全部会话进程并释放执行 ctx（在途轮次与审批由
// 各 pump 收尾清理）。
func (s *ChatService) CloseAll() {
	s.mu.Lock()
	rts := make([]*chatSessionRuntime, 0, len(s.sessions))
	for _, rt := range s.sessions {
		rts = append(rts, rt)
	}
	s.mu.Unlock()
	for _, rt := range rts {
		if rt.cancel != nil {
			rt.cancel()
		}
		if rt.proc != nil {
			rt.proc.Kill()
		}
	}
}

// ===== 进程泵与生命周期 =====

// chatStreamKind chat 域 stream-json 行解析分类。
type chatStreamKind int

const (
	chatStreamOther      chatStreamKind = iota // 其余事件（user 回传/tool_result/hook 等，忽略）
	chatStreamText                             // assistant 文本增量
	chatStreamInit                             // system/init（流式模式下每轮可能重复出现，勿按一次性事件建模）
	chatStreamResult                           // result 终态（每轮一个）
	chatStreamApproval                         // control_request/can_use_tool 工具授权请求
	chatStreamControlResp                      // control_response（本方 interrupt 等控制请求的回执，忽略）
)

// chatControlRequest claude control_request 事件（权限审批下发）所需字段子集。
type chatControlRequest struct {
	RequestID string `json:"request_id"`
	Request   struct {
		Subtype     string         `json:"subtype"`
		ToolName    string         `json:"tool_name"`
		Input       map[string]any `json:"input"`
		Description string         `json:"description"`
	} `json:"request"`
}

// parseChatStreamLine 解析一行 chat 域 stream-json（持续会话进程输出）。
// 与 ai_function.go 的 parseStreamLine 分域独立（AI 功能零改动）：chat 需消费
// control_request/init 等对话专属事件类型，且 result 的 subtype 与 is_error
// 须组合判断（存在 subtype=success 且 is_error=true 的失败形态）。
// 非 JSON 行（stderr 串入的诊断文本）原样作为文本增量返回，不丢输出。
func parseChatStreamLine(line string) (kind chatStreamKind, text, sessionID string, approval *chatControlRequest, resultError string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	var ev streamEvent
	if err := json.Unmarshal([]byte(trimmed), &ev); err != nil {
		return chatStreamText, line + "\n", "", nil, ""
	}
	if ev.SessionID != "" {
		sessionID = ev.SessionID
	}
	switch {
	case ev.Type == "assistant" && ev.Message != nil:
		var sb strings.Builder
		for _, part := range ev.Message.Content {
			if part.Type == "text" && part.Text != "" {
				sb.WriteString(part.Text)
			}
		}
		return chatStreamText, sb.String(), sessionID, nil, ""
	case ev.Type == "result":
		if ev.IsError || (ev.Subtype != "" && ev.Subtype != "success") {
			resultError = strings.TrimSpace(ev.Result)
			if resultError == "" {
				resultError = fmt.Sprintf("claude 执行失败（%s）", ev.Subtype)
			}
		}
		return chatStreamResult, "", sessionID, nil, resultError
	case ev.Type == "system" && ev.Subtype == "init":
		return chatStreamInit, "", sessionID, nil, ""
	case ev.Type == "control_request":
		var cr chatControlRequest
		if uerr := json.Unmarshal([]byte(trimmed), &cr); uerr != nil || cr.Request.Subtype != "can_use_tool" {
			return chatStreamOther, "", sessionID, nil, ""
		}
		return chatStreamApproval, "", sessionID, &cr, ""
	case ev.Type == "control_response":
		return chatStreamControlResp, "", sessionID, nil, ""
	}
	return
}

// spawnSessionProcess 为会话启动常驻 claude 进程（调用方须持 s.mu）。
// 进程 ctx 为 s.ctx 派生（无单轮超时——生命周期由空闲回收/轮次超时/关停链管理，
// ctx 取消（CloseAll）经 cmd.Cancel 整树杀）；spawn 参数取运行时快照。
// 启动成功后拉起 stdout 泵与会话 watchdog。
func (s *ChatService) spawnSessionProcess(rt *chatSessionRuntime, cwd string) error {
	ctx, cancel := context.WithCancel(s.ctx)
	proc := s.processFactory(ctx, "claude", buildChatArgs(rt.claudeSessionID, rt.permissionMode, rt.modelName), cwd)
	stdout, err := proc.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("创建输出管道失败: %w", err)
	}
	stdin, err := proc.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("创建输入管道失败: %w", err)
	}
	if err := proc.Start(); err != nil {
		cancel()
		// 关闭 stdin 写端：进程未起，管道不再有消费方（防测试 fake 的
		// stdin 捕获 goroutine 泄漏，goleak 门禁会拦截）
		_ = stdin.Close()
		return err
	}
	rt.proc = proc
	rt.stdin = stdin
	rt.ctx = ctx
	rt.cancel = cancel
	go s.pumpSession(rt, stdout)
	go s.watchSession(rt)
	return nil
}

// advanceSession 推进会话轮次：空闲且队列非空时弹出队首轮写入 stdin 开始执行。
// stdin 写入失败视为进程已死：该轮失败收尾（done 事件），错误同步返回给调用方，
// 进程由 pump 收尾摘除。忙/空队列/进程未起时不动，返回 nil。
// 并发安全：executing 判定与赋值同临界区，两个并发 advance 只有一个推进。
func (s *ChatService) advanceSession(rt *chatSessionRuntime) error {
	s.mu.Lock()
	if rt.executing != nil || rt.proc == nil || len(rt.queue) == 0 {
		s.mu.Unlock()
		return nil
	}
	turn := rt.queue[0]
	rt.queue = rt.queue[1:]
	rt.executing = turn
	turn.startedAt = time.Now()
	rt.lastActive = time.Now()
	stdin := rt.stdin
	// chat-question 环境声明注入判定：仅 claude 会话第一条消息（rt 未收到任何
	// init/result 事件）在 prompt 前拼声明；声明只进 claude prompt，落盘 user
	// 消息保持用户原文（历史重载不混入环境噪音）。resume 重启的运行时已预载
	// claudeSessionID，天然跳过。
	claudeText := turn.prompt
	if rt.claudeSessionID == "" {
		claudeText = chatQuestionPreamble + "\n\n" + turn.prompt
	}
	chatSessionID, taskID, prompt := turn.chatSessionID, turn.id, turn.prompt
	s.mu.Unlock()

	if err := writeChatStdinLine(stdin, chatUserStdinLine(claudeText)); err != nil {
		// stdin 写入失败视为进程已死：该轮失败收尾（done 事件），错误同步返回给调用方，
		// 进程由 pump 收尾摘除（执行槽位复位与在途表摘除在 finishAbnormalTurn 认领后统一完成）
		werr := fmt.Errorf("写入对话进程输入失败: %w", err)
		s.finishAbnormalTurn(rt, turn, werr)
		return werr
	}
	// user 消息入档：执行开始（stdin 写入成功）才落盘（排队取消/启动失败不留痕）
	userMsg := model.ChatMessage{
		Role:      model.ChatRoleUser,
		Content:   prompt,
		Timestamp: time.Now().UnixMilli(),
		TaskID:    taskID,
	}
	if err := s.appendMessages(chatSessionID, userMsg); err != nil {
		// 消息落盘失败不阻断对话执行（回复仍可流式送达前端），仅记录
		Logger().Error("append chat user message failed", "session", chatSessionID, "err", err)
	}
	s.emitCurrent("chat-task:started", map[string]any{"taskId": taskID, "chatSessionId": chatSessionID})
	return nil
}

// pumpSession 常驻进程 stdout 泵：逐行解析 stream-json 多路分发——init 刷新
// claude session id、文本增量归属当前执行轮、审批请求注册推送、result 收尾
// 当前轮并推进队首。进程退出（stdin 关闭/被杀/崩溃）后收尾在途轮次与审批。
func (s *ChatService) pumpSession(rt *chatSessionRuntime, stdout io.ReadCloser) {
	// 输出流随读尽关闭；槽位归还/注册表摘除/在途清理统一在 sessionProcessExit
	defer func() { _ = stdout.Close() }()
	defer s.sessionProcessExit(rt)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024) // 单行上限 4MB（长 JSON 事件）

	for scanner.Scan() {
		kind, text, sessionID, approval, resultErr := parseChatStreamLine(scanner.Text())
		switch kind {
		case chatStreamInit, chatStreamResult:
			// claude session id 刷新（init 每轮重复出现；result 也携带）——
			// respawn --resume 依据与 chat-question 声明注入判定共用
			s.mu.Lock()
			if sessionID != "" {
				rt.claudeSessionID = sessionID
			}
			s.mu.Unlock()
			if kind == chatStreamResult {
				s.finishExecutingTurn(rt, sessionID, resultErr, nil, nil)
				// result 收尾后立即推进队首（运行中插话的下一条开始执行）
				_ = s.advanceSession(rt)
			}
		case chatStreamText:
			s.handleTurnText(rt, text)
		case chatStreamApproval:
			s.registerApproval(rt, approval)
		}
	}
	// Scan 退出后必须查 Err()：非 EOF（读管道错误/单行超 4MB 上限）意味着
	// 回复被截断，静默终止会让用户拿到不完整回复且 done 无任何错误提示
	scanErr := scanner.Err()
	waitErr := rt.proc.Wait()
	// 进程退出：执行中轮次（若有）按退出错误收尾
	s.finishExecutingTurn(rt, "", "", waitErr, scanErr)
}

// handleTurnText 追加 assistant 文本增量到当前执行轮并推送 output 事件。
// 无执行轮归属的文本（stderr 诊断/进程横幅在轮间串入）丢弃不推送。
func (s *ChatService) handleTurnText(rt *chatSessionRuntime, text string) {
	if text == "" {
		return
	}
	s.mu.Lock()
	turn := rt.executing
	if turn == nil {
		s.mu.Unlock()
		return
	}
	turn.reply.WriteString(text)
	taskID, chatSessionID := turn.id, turn.chatSessionID
	s.mu.Unlock()
	s.emitCurrent("chat-task:output", map[string]any{
		"taskId":        taskID,
		"chatSessionId": chatSessionID,
		"text":          text,
	})
}

// registerApproval 注册工具授权审批并推送前端弹窗事件（chat-approval:request）。
func (s *ChatService) registerApproval(rt *chatSessionRuntime, cr *chatControlRequest) {
	s.mu.Lock()
	turn := rt.executing
	turnID := ""
	if turn != nil {
		turnID = turn.id
	}
	ap := &chatApproval{
		requestID:     cr.RequestID,
		chatSessionID: rt.chatSessionID,
		turnID:        turnID,
		toolName:      cr.Request.ToolName,
		input:         cr.Request.Input,
		description:   cr.Request.Description,
	}
	s.approvals[ap.requestID] = ap
	s.mu.Unlock()
	s.emitCurrent("chat-approval:request", model.ChatApprovalRequest{
		RequestID:     ap.requestID,
		ChatSessionID: ap.chatSessionID,
		TaskID:        ap.turnID,
		ToolName:      ap.toolName,
		Input:         ap.input,
		Description:   ap.description,
	})
}

// finishExecutingTurn 收尾当前执行轮：构造 done 结果（错误优先级 canceled >
// timedOut > claudeErr > 进程/流错误）、落盘 assistant 回复与 claude session id、
// 发 done 事件并从在途表摘除。执行轮为 nil 或已由其他 finalizer 认领
// （turn.finished）时幂等返回——result 收尾与 stdin 写失败收尾（finishAbnormalTurn）
// 为并发双 finalizer，先持锁认领者收尾。
// 落盘时序契约：先落盘后 emit done——前端收到 done 时 GetChatSession 已可读完整消息。
func (s *ChatService) finishExecutingTurn(rt *chatSessionRuntime, sessionID, claudeErr string, waitErr, scanErr error) {
	s.mu.Lock()
	turn := rt.executing
	if turn == nil || turn.finished {
		// 已收尾（nil = 本路径幂等重入；finished = stdin 写失败路径先行认领）
		s.mu.Unlock()
		return
	}
	turn.finished = true
	rt.executing = nil
	rt.lastActive = time.Now()
	if sessionID != "" {
		turn.claudeSessionID = sessionID
	}
	canceled, timedOut := turn.canceled, turn.timedOut
	reply, turnID := turn.reply.String(), turn.id
	claudeSessionID, chatSessionID := turn.claudeSessionID, rt.chatSessionID
	s.mu.Unlock()

	result := model.ChatTaskRunResult{
		TaskID:          turnID,
		ChatSessionID:   chatSessionID,
		ClaudeSessionID: claudeSessionID,
		Reply:           reply,
	}
	switch {
	case canceled:
		result.Canceled = true
		result.Error = "已取消"
	case timedOut:
		result.Error = fmt.Sprintf("执行超时（上限 %s），已发送中断", s.turnTimeout)
	case claudeErr != "":
		// claude 侧报告失败：错误文本优先于裸退出码（"exit status 1" 不可读）
		if waitErr != nil {
			result.Error = waitErr.Error() + ": " + claudeErr
		} else {
			result.Error = claudeErr
		}
	case waitErr != nil || scanErr != nil:
		// 进程退出错误与输出流读取错误并存时合并展示
		if waitErr != nil {
			result.Error = waitErr.Error()
		}
		if scanErr != nil {
			if result.Error != "" {
				result.Error += "; "
			}
			result.Error += "输出流读取中断: " + scanErr.Error()
		}
	}
	s.persistTurnResult(chatSessionID, claudeSessionID, turnID, reply)

	s.mu.Lock()
	delete(s.turns, turnID)
	s.mu.Unlock()
	// 该轮未决审批自动拒绝（轮已结束，CLI 侧解除阻塞，前端撤弹窗）
	s.settleApprovals(chatSessionID, turnID, "该轮对话已结束，审批请求已失效")

	s.emitCurrent("chat-task:done", result)
}

// finishAbnormalTurn stdin 写入失败（进程已死）时执行轮尚未真正开始的失败收尾：
// 不落盘任何消息（user 消息尚未写入），仅发 done 失败事件并从在途表摘除（含
// 执行槽位复位）。与 finishExecutingTurn（pump 进程退出路径）构成同一轮的并发
// 双 finalizer：以 turn.finished 单次认领保证同一轮恰好一次 done——先持锁认领者
// 收尾发事件，后到者幂等返回（persistTurnResult 的锁外慢 IO 窗口内对方到场的
// 交错形态也被认领标志拦住）。
func (s *ChatService) finishAbnormalTurn(rt *chatSessionRuntime, turn *chatTurnRuntime, err error) {
	s.mu.Lock()
	if turn.finished {
		s.mu.Unlock()
		return
	}
	turn.finished = true
	if rt.executing == turn {
		rt.executing = nil
	}
	delete(s.turns, turn.id)
	s.mu.Unlock()
	s.emitCurrent("chat-task:done", model.ChatTaskRunResult{
		TaskID:        turn.id,
		ChatSessionID: turn.chatSessionID,
		Error:         err.Error(),
	})
}

// persistTurnResult 落盘本轮结果（锁外：appendMessages/updateSessionMeta 内部
// 各自加锁）。assistant 回复非空才入档（失败轮无回复不留空消息；取消轮保留
// 部分回复）；claude session id 更新回会话元数据（下一轮 spawn --resume 续
// 上下文），会话在途被删除时更新报 SessionNotFound 属预期降级（Warn 记录）。
func (s *ChatService) persistTurnResult(chatSessionID, claudeSessionID, taskID, reply string) {
	if reply != "" {
		assistantMsg := model.ChatMessage{
			Role:      model.ChatRoleAssistant,
			Content:   reply,
			Timestamp: time.Now().UnixMilli(),
			TaskID:    taskID,
		}
		if err := s.appendMessages(chatSessionID, assistantMsg); err != nil {
			Logger().Error("append chat assistant message failed",
				"session", chatSessionID, "task", taskID, "err", err)
		}
	}
	if claudeSessionID != "" {
		if err := s.updateSessionMeta(chatSessionID, func(sess *model.ChatSession) {
			sess.ClaudeSessionID = claudeSessionID
		}); err != nil {
			var ae *model.AppError
			if errors.As(err, &ae) && ae.Code == model.ErrCodeChatSessionNotFound {
				Logger().Warn("chat session removed during task, skip claude id update",
					"session", chatSessionID)
			} else {
				Logger().Error("update chat session claude id failed",
					"session", chatSessionID, "err", err)
			}
		}
	}
}

// settleApprovals 将某会话（turnID 非空时仅限该轮）的未决审批以 deny 应答回写
// stdin（CLI 解除阻塞继续收尾）并广播 chat-approval:cancelled 事件让前端撤弹窗。
// 进程退出路径 stdin 已不可写——写失败静默（审批随进程消亡，事件仍须发出）。
func (s *ChatService) settleApprovals(chatSessionID, turnID, reason string) {
	s.mu.Lock()
	var settled []*chatApproval
	for id, ap := range s.approvals {
		if ap.chatSessionID == chatSessionID && (turnID == "" || ap.turnID == turnID) {
			settled = append(settled, ap)
			delete(s.approvals, id)
		}
	}
	var stdin io.WriteCloser
	if rt := s.sessions[chatSessionID]; rt != nil {
		stdin = rt.stdin
	}
	s.mu.Unlock()
	for _, ap := range settled {
		if stdin != nil {
			_ = writeChatStdinLine(stdin, chatControlResponseLine(ap.requestID, false, reason, nil))
		}
		s.emitCurrent("chat-approval:cancelled", map[string]any{
			"requestId":     ap.requestID,
			"chatSessionId": ap.chatSessionID,
			"reason":        reason,
		})
	}
}

// sessionProcessExit 进程退出收尾（pumpSession defer）：失败收尾残留排队轮次
// （进程退出时未执行，user 消息未落盘）、清理未决审批、摘除注册表、归还进程
// 槽位、释放执行 ctx。幂等（killSession 先行摘除注册表时剩余步骤仍须执行）。
func (s *ChatService) sessionProcessExit(rt *chatSessionRuntime) {
	s.mu.Lock()
	if cur := s.sessions[rt.chatSessionID]; cur == rt {
		delete(s.sessions, rt.chatSessionID)
	}
	queued := rt.queue
	rt.queue = nil
	for _, t := range queued {
		delete(s.turns, t.id)
	}
	cancel := rt.cancel
	s.mu.Unlock()

	for _, t := range queued {
		s.emitCurrent("chat-task:done", model.ChatTaskRunResult{
			TaskID:        t.id,
			ChatSessionID: t.chatSessionID,
			Error:         "对话进程已退出，排队任务未执行",
		})
	}
	s.settleApprovals(rt.chatSessionID, "", "对话进程已退出，审批请求已失效")
	if cancel != nil {
		cancel()
	}
	rt.semRelease.Do(func() { <-s.procSem })
}

// killSession 终止会话进程并从注册表摘除（空闲回收/退出等待超时兜底共用）。
// 进程 Kill 后 pump 自行收尾（在途轮次失败收尾 + 槽位归还 + 审批清理）；
// 本函数只负责摘注册表与发终止信号，幂等。
func (s *ChatService) killSession(rt *chatSessionRuntime, reason string) {
	s.mu.Lock()
	if cur := s.sessions[rt.chatSessionID]; cur == rt {
		delete(s.sessions, rt.chatSessionID)
	}
	proc, cancel := rt.proc, rt.cancel
	s.mu.Unlock()
	Logger().Info("chat session process killed", "session", rt.chatSessionID, "reason", reason)
	if cancel != nil {
		cancel()
	}
	if proc != nil {
		proc.Kill()
	}
}

// restartSessionIdle 空闲会话进程优雅重启（权限模式/模型变更）：关闭 stdin 触发
// 进程处理完即退（空闲无在途轮次，立即退出），等待 pump 摘除并归还槽位后再
// 返回，保证后续 spawn 的槽位获取不因旧进程未清而假性失败；等待超时转强杀。
func (s *ChatService) restartSessionIdle(rt *chatSessionRuntime) {
	s.mu.Lock()
	stdin, proc := rt.stdin, rt.proc
	s.mu.Unlock()
	if stdin == nil || proc == nil {
		s.killSession(rt, "参数变更重启（进程未就绪）")
		return
	}
	_ = stdin.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		gone := s.sessions[rt.chatSessionID] != rt
		s.mu.Unlock()
		if gone {
			return
		}
		if time.Now().After(deadline) {
			s.killSession(rt, "参数变更重启（优雅退出超时）")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// watchSession 会话 watchdog：扫描执行轮超时（发 interrupt 中断，再超一倍时长
// 仍无 result 强杀进程兜底）与进程空闲超时（Kill 回收）。随进程 ctx 退出。
func (s *ChatService) watchSession(rt *chatSessionRuntime) {
	ticker := time.NewTicker(s.scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-rt.ctx.Done():
			return
		case <-ticker.C:
			s.watchSessionOnce(rt)
		}
	}
}

// watchSessionOnce 单次 watchdog 扫描（空闲回收与轮次超时判定均持锁快照，
// stdin 写出锁执行——与 advanceSession/CancelChatTask 同款锁纪律）。
func (s *ChatService) watchSessionOnce(rt *chatSessionRuntime) {
	s.mu.Lock()
	turn := rt.executing
	if turn != nil {
		elapsed := time.Since(turn.startedAt)
		switch {
		case elapsed > 2*s.turnTimeout && turn.timedOut:
			// 已发过 interrupt 仍无 result：CLI 失去响应，强杀进程兜底
			//（在途轮次与审批由 pump 收尾失败清理）
			s.mu.Unlock()
			s.killSession(rt, "轮次中断后仍无收尾")
		case elapsed > s.turnTimeout && !turn.timedOut:
			turn.timedOut = true
			stdin := rt.stdin
			s.mu.Unlock()
			if err := writeChatStdinLine(stdin, chatInterruptLine("chat-timeout-"+turn.id)); err != nil {
				Logger().Warn("send chat turn timeout interrupt failed", "task", turn.id, "err", err)
			}
		default:
			s.mu.Unlock()
		}
		return
	}
	idle := len(rt.queue) == 0 && time.Since(rt.lastActive) > s.idleTimeout
	s.mu.Unlock()
	if idle {
		s.killSession(rt, "空闲超时回收")
	}
}

