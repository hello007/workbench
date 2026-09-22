package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// chatDefaultTimeoutMinutes 单轮对话默认超时（分钟），超出强杀子进程。
// 对话场景单轮回复耗时与 AI 功能任务相当，沿用 10 分钟基准。
const chatDefaultTimeoutMinutes = 10

// chatMaxConcurrent 对话任务全局并发上限：同时运行的 claude -p 子进程数。
// 与 AI 功能任务的 aiTaskMaxConcurrent 独立计数，互不挤占槽位。
const chatMaxConcurrent = 3

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

// saveChatJSON 原子写 JSON 文件：先写同目录 temp 文件，成功后把现有旧文件复制为
// <目标>.bak（保留上一好版本，rename 后新文件若损坏仍有回退），再 os.Rename
// temp 原子替换目标。进程崩溃不再把目标截断为半截内容（util.SaveJSON 的
// os.WriteFile 是截断写，崩溃即丢会话历史/索引且 .bak 备份的也是已截断内容）。
// 仅 ChatService 域使用；util.SaveJSON 有其他 service 在用，保持原样不动。
func saveChatJSON(filePath string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(filePath)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// 成功 rename 后目标名已不存在，此删除为 no-op；失败路径清理 temp 残留
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// 覆写前备份上一好版本（当前文件不存在时跳过）
	if old, rerr := os.ReadFile(filePath); rerr == nil {
		_ = os.WriteFile(filePath+".bak", old, 0o644)
	}
	return os.Rename(tmpName, filePath)
}

// ===== 子进程抽象（测试注入 fake，不真调 claude CLI）=====

// chatProcess 对话子进程抽象：生产包装 exec.Cmd，测试注入 fake 流式输出。
// 抽象面仅覆盖 pumpChatOutput 实际触达的操作（管道/启动/等待/终止/退出码）。
type chatProcess interface {
	// StdoutPipe 返回子进程标准输出流（stderr 已并入该流）。
	StdoutPipe() (io.ReadCloser, error)
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

// chatTaskRuntime 一个运行中/已完成对话任务的内部状态。
// 与 AiFunctionService 的 aiTaskRuntime 同构，但无输出文件（assistant 回复
// 直接内存累积入消息文件，原始 stream-json 行对话场景无需留档）。
type chatTaskRuntime struct {
	id              string
	chatSessionID   string // 所属 WorkBench 会话 id
	prompt          string
	claudeSessionID string          // claude 会话 id（stream 事件提取）
	reply           strings.Builder // 本轮 assistant 回复累积（持锁写）
	proc            chatProcess
	ctx             context.Context
	cancel          context.CancelFunc
	running         bool
	canceled        bool
	queued          bool
	queuedAt        time.Time
	startedAt       time.Time
	finishedAt      time.Time
	timeoutMin      int
	errText         string
	queueCancel     chan struct{} // 排队取消信号：close 后唤醒 RunChat 的 select
}

// ChatService AI 对话服务：多会话持续式对话的会话 CRUD、消息持久化与
// 自由 prompt 对话执行器（RunChat）。
//
// 执行模型与 AiFunctionService 的任务链路同构：claude -p 子进程 +
// stream-json 流式解析 + chat-task:queued/started/output/done 事件推送 +
// 并发信号量排队 + 超时控制。差异点：
//   - 不绑定 AiFunction 配置，prompt 由前端直传（自由对话）；
//   - 会话已有 ClaudeSessionID 时自动 --resume 续上下文（多轮对话）；
//   - 每轮完成后把 user 消息与 assistant 回复追加进会话消息文件，
//     并把 claude session id 更新回会话元数据；
//   - 事件名独立前缀 chat-task:*，与 AI 功能面板事件流隔离。
//
// 持久化布局（独立 data/ai_chat/ 目录，不与其他 service 文件交叉）：
//   - data/ai_chat/sessions.json           会话索引（元数据，不含消息）
//   - data/ai_chat/messages/<sessionId>.json 每会话消息数组
//   - data/ai_chat/directories.json        侧栏常用目录项（AI 对话入口列表）
type ChatService struct {
	ctx        context.Context
	sinkHolder        // 事件出口持有器（chat-task:* 事件推送），构造注入 + serve 模式经 SetEventSink 切换
	dataDir    string // 存储根目录 data/ai_chat/
	mu         sync.Mutex
	tasks      map[string]*chatTaskRuntime
	// 全局并发信号量，缓冲 = chatMaxConcurrent
	concurrencySem chan struct{}
	processFactory chatProcessFactory // 子进程工厂（测试注入 fake）
}

// NewChatService 创建 AI 对话服务。dataDir 为存储根目录（data/ai_chat/）。
func NewChatService(ctx context.Context, dataDir string) *ChatService {
	s := &ChatService{
		ctx:            ctx,
		dataDir:        dataDir,
		tasks:          make(map[string]*chatTaskRuntime),
		concurrencySem: make(chan struct{}, chatMaxConcurrent),
		processFactory: newExecChatProcess,
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
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
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
		return model.NewAppError(model.ErrCodeChatSessionNotFound, "会话不存在或已被删除")
	}
	idx.Sessions = kept
	if err := s.saveIndex(idx); err != nil {
		return err
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

// buildChatArgs 组装 claude headless 对话命令参数（纯函数，便于单测）。
// 参数顺序：-p <prompt> [--resume <sid>] --output-format stream-json --verbose
// [--permission-mode <mode>] [--model <model>]。
// --resume：会话已有 claude session id 时续上下文（多轮对话）。
// --permission-mode：仅非 default 非空时追加（default 为 claude 默认行为，不传等价）。
// --model：仅非空时追加（空 = claude 默认模型）。
func buildChatArgs(prompt, resumeSessionID, permissionMode, modelName string) []string {
	args := []string{"-p", prompt}
	if resumeSessionID != "" {
		args = append(args, "--resume", resumeSessionID)
	}
	args = append(args, "--output-format", "stream-json", "--verbose")
	if permissionMode != "" && permissionMode != model.ChatPermissionModeDefault {
		args = append(args, "--permission-mode", permissionMode)
	}
	if modelName != "" {
		args = append(args, "--model", modelName)
	}
	return args
}

// RunChat 执行一轮对话：把 prompt 发给 claude（会话已有 claude session id 时
// --resume 续上下文），流式解析回复并推送 chat-task:* 事件。
// 返回任务 id（供事件对号与取消）；进程启动失败同步报错。
//
// 同一会话存在进行中/排队中任务时拒绝（ErrCodeChatInProgress）——
// 同会话轮次必须串行，否则 --resume 上下文与消息文件写入交错损坏。
//
// 事件序列：chat-task:queued → chat-task:started → chat-task:output（增量）
// → chat-task:done（ChatTaskRunResult，含全文回复与 claude session id）。
func (s *ChatService) RunChat(chatSessionID, prompt, permissionMode, modelName string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", model.NewAppError(model.ErrCodeChatEmptyPrompt, "对话内容不能为空")
	}
	sess, err := s.GetChatSession(chatSessionID)
	if err != nil {
		return "", err
	}

	// 同会话串行保护：已存在该会话的进行中/排队任务则拒绝。
	// 检查与注册同一临界区完成，防两个并发 RunChat 同时通过检查（检查-注册
	// 分离的窗口内双双入队，同会话两轮并行破坏 --resume 上下文与消息文件）。
	taskID := newChatTaskID()
	task := &chatTaskRuntime{
		id:            taskID,
		chatSessionID: chatSessionID,
		prompt:        prompt,
		queued:        true,
		queuedAt:      time.Now(),
		queueCancel:   make(chan struct{}),
	}
	s.mu.Lock()
	for _, t := range s.tasks {
		if t != nil && t.chatSessionID == chatSessionID && (t.running || t.queued) {
			s.mu.Unlock()
			return "", model.NewAppError(model.ErrCodeChatInProgress, "该会话有对话进行中，请等待完成或取消后再发送")
		}
	}
	s.tasks[taskID] = task
	s.mu.Unlock()
	s.emitCurrent("chat-task:queued", map[string]any{"taskId": taskID})

	// 排队等待槽位；三路唤醒与 RunStage 同构：
	//   - concurrencySem 槽位可用 → 转执行
	//   - queueCancel close → 排队取消（CancelChatTask 触发），不启动进程
	//   - s.ctx.Done → app 关闭
	select {
	case s.concurrencySem <- struct{}{}:
	case <-task.queueCancel:
		s.mu.Lock()
		delete(s.tasks, taskID)
		s.mu.Unlock()
		return "", fmt.Errorf("任务已取消（排队中）")
	case <-s.ctx.Done():
		s.mu.Lock()
		delete(s.tasks, taskID)
		s.mu.Unlock()
		return "", fmt.Errorf("应用关闭，任务未启动")
	}

	// 排队期间被取消：不启动进程，归还槽位
	s.mu.Lock()
	if task.canceled {
		delete(s.tasks, taskID)
		s.mu.Unlock()
		<-s.concurrencySem
		return "", fmt.Errorf("任务已取消（排队中）")
	}
	// queued=false 与 running=true 同临界区翻转：注册态（queued=true）到执行态
	// （running=true）之间不得出现双 false 窗口，否则同会话串行保护检查
	// （running || queued）被绕过，两轮 claude 并行同会话破坏 --resume 上下文。
	// 后续启动失败路径负责复位并摘除任务。
	task.queued = false
	task.running = true
	task.startedAt = time.Now()
	s.mu.Unlock()
	s.emitCurrent("chat-task:started", map[string]any{"taskId": taskID, "chatSessionId": chatSessionID})

	// 获取槽位后才创建执行 ctx（超时起算后移，排队等待不侵蚀执行预算）
	timeout := chatDefaultTimeoutMinutes
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(timeout)*time.Minute)

	args := buildChatArgs(prompt, sess.ClaudeSessionID, permissionMode, modelName)
	proc := s.processFactory(ctx, "claude", args, sess.Cwd)

	stdout, err := proc.StdoutPipe()
	if err != nil {
		cancel()
		// 启动失败：复位标志并摘除任务（失败任务不残留 s.tasks，防内存无界
		// 增长与串行保护永久拒绝该会话）；错误经返回值同步报给前端
		s.mu.Lock()
		task.running = false
		delete(s.tasks, taskID)
		s.mu.Unlock()
		<-s.concurrencySem
		return "", fmt.Errorf("创建输出管道失败: %w", err)
	}
	if err := proc.Start(); err != nil {
		cancel()
		s.mu.Lock()
		task.running = false
		delete(s.tasks, taskID)
		s.mu.Unlock()
		<-s.concurrencySem
		return "", fmt.Errorf("启动 claude 失败（请确认已安装并在 PATH 中）: %w", err)
	}

	// 进程启动成功：user 消息即刻入档（排队取消/启动失败的轮次不留痕）
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

	s.mu.Lock()
	task.proc = proc
	task.ctx = ctx
	task.cancel = cancel
	task.running = true
	task.timeoutMin = timeout
	lateCanceled := task.canceled
	s.mu.Unlock()
	if lateCanceled {
		// 取消发生在启动窗口（queued 翻转后、proc 赋值前）：取消时无进程可杀，
		// 此刻进程已起，补杀防「已取消但进程继续跑完」
		proc.Kill()
	}

	go s.pumpChatOutput(task, proc, stdout)
	return taskID, nil
}

// CancelChatTask 取消对话任务，区分两态（与 CancelAiTask 同构）：
//   - 排队中（无进程）：close queueCancel 唤醒 RunChat 的 select 自行清理，
//     并先 emit done（canceled）通知前端
//   - 运行中：标记 canceled 并杀进程树，pumpChatOutput 末尾构造 done 事件
//
// 幂等：重复取消直接返回 true，不重复 close queueCancel（double-close panic）、
// 不重复 emit done。
func (s *ChatService) CancelChatTask(taskID string) bool {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return false
	}
	if task.canceled {
		// 已取消过（排队 close 或运行杀进程均已触发），幂等返回
		s.mu.Unlock()
		return true
	}
	task.canceled = true
	queued := task.queued
	proc := task.proc
	s.mu.Unlock()

	if queued {
		close(task.queueCancel)
		s.emitCurrent("chat-task:done", model.ChatTaskRunResult{
			TaskID:        taskID,
			ChatSessionID: task.chatSessionID,
			Canceled:      true,
			Error:         "已取消（排队中）",
		})
		return true
	}
	// queued=false → running=true 的启动窗口内 proc 尚未赋值（nil），
	// 跳过杀进程；若进程随后启动成功，RunChat 的 lateCanceled 检查补杀
	if proc != nil {
		proc.Kill()
	}
	return true
}

// GetChatTaskState 查询对话任务状态（前端恢复/轮询兜底用）。
// 任务不存在返回 nil。
func (s *ChatService) GetChatTaskState(taskID string) *model.ChatTaskState {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return nil
	}
	return &model.ChatTaskState{
		TaskID:          task.id,
		ChatSessionID:   task.chatSessionID,
		Running:         task.running,
		Queued:          task.queued,
		ClaudeSessionID: task.claudeSessionID,
		Reply:           task.reply.String(),
		Error:           task.errText,
		StartedAt:       task.startedAt.UnixMilli(),
	}
}

// CloseAll 应用退出时终止全部对话子进程并回收执行 ctx。
func (s *ChatService) CloseAll() {
	s.mu.Lock()
	tasks := make([]*chatTaskRuntime, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	s.mu.Unlock()
	for _, t := range tasks {
		if t.cancel != nil {
			t.cancel()
		}
		if t.proc != nil {
			t.proc.Kill()
		}
	}
}

// pumpChatOutput 逐行读取子进程 stdout，解析 stream-json 并推送事件，等待退出。
// 退出后落盘 assistant 回复与 claude session id（先落盘再 emit done，
// 保证前端收到 done 时消息已可经 GetChatSession 读到）。
func (s *ChatService) pumpChatOutput(task *chatTaskRuntime, proc chatProcess, stdout io.ReadCloser) {
	// 信号量随进程退出释放；输出流随读尽关闭（退出后 reader 不再被引用）
	defer func() { <-s.concurrencySem }()
	defer func() { _ = stdout.Close() }()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024) // 单行上限 4MB（长 JSON 事件）

	// claudeErr result 事件的 claude 侧错误文本（is_error/非 success subtype），
	// pump 局部变量：仅本 goroutine 读写，Wait 后统一消费
	claudeErr := ""
	for scanner.Scan() {
		text, isResult, sessionID, _, _, resultErr := parseStreamLine(scanner.Text())
		if resultErr != "" {
			claudeErr = resultErr
		}
		// 状态写入须持锁：GetChatTaskState 在锁内读同字段；emit 放锁外
		s.mu.Lock()
		if sessionID != "" {
			task.claudeSessionID = sessionID
		}
		if text != "" {
			task.reply.WriteString(text)
		}
		s.mu.Unlock()
		if text != "" {
			s.emitCurrent("chat-task:output", map[string]any{
				"taskId":        task.id,
				"chatSessionId": task.chatSessionID,
				"text":          text,
			})
		}
		if isResult {
			// result 事件后进程随即退出，继续读完剩余行
			continue
		}
	}
	// Scan 退出后必须查 Err()：非 EOF（读管道错误/单行超 4MB 上限）意味着
	// 回复被截断，静默终止会让用户拿到不完整回复且 done 无任何错误提示
	scanErr := scanner.Err()

	waitErr := proc.Wait()

	s.mu.Lock()
	task.running = false
	task.finishedAt = time.Now()
	// 超时判定须在读 ctx.Err() 之后调 cancel（cancel 会把 Err 覆写为 Canceled）
	timedOut := task.ctx != nil && task.ctx.Err() == context.DeadlineExceeded
	reply := task.reply.String()
	result := model.ChatTaskRunResult{
		TaskID:          task.id,
		ChatSessionID:   task.chatSessionID,
		ClaudeSessionID: task.claudeSessionID,
		Reply:           reply,
	}
	switch {
	case task.canceled:
		result.Canceled = true
		result.Error = "已取消"
	case timedOut:
		result.Error = fmt.Sprintf("执行超时（上限 %d 分钟）", task.timeoutMin)
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
	if code := proc.ExitCode(); code >= 0 {
		result.ExitCode = code
	}
	cancelFn := task.cancel
	s.mu.Unlock()

	// 释放执行 ctx（超时定时器）：正常完成/手动取消路径不再等 10 分钟定时器
	// 自然到期才释放；超时路径 ctx 已自取消，重复调用幂等无害
	if cancelFn != nil {
		cancelFn()
	}

	// 落盘本轮结果（锁外：appendMessages/updateSessionMeta 内部各自加锁）。
	// assistant 回复非空才入档（失败轮无回复不留空消息；取消轮保留部分回复）。
	if reply != "" {
		assistantMsg := model.ChatMessage{
			Role:      model.ChatRoleAssistant,
			Content:   reply,
			Timestamp: time.Now().UnixMilli(),
			TaskID:    task.id,
		}
		if err := s.appendMessages(task.chatSessionID, assistantMsg); err != nil {
			Logger().Error("append chat assistant message failed",
				"session", task.chatSessionID, "task", task.id, "err", err)
		}
	}
	// claude session id 更新回会话元数据（下一轮 --resume 续上下文）。
	// 会话在途被删除时更新报 SessionNotFound 属预期降级（仅影响下轮 resume），Warn 记录。
	if task.claudeSessionID != "" {
		if err := s.updateSessionMeta(task.chatSessionID, func(sess *model.ChatSession) {
			sess.ClaudeSessionID = task.claudeSessionID
		}); err != nil {
			var ae *model.AppError
			if errors.As(err, &ae) && ae.Code == model.ErrCodeChatSessionNotFound {
				Logger().Warn("chat session removed during task, skip claude id update",
					"session", task.chatSessionID)
			} else {
				Logger().Error("update chat session claude id failed",
					"session", task.chatSessionID, "err", err)
			}
		}
	}

	s.emitCurrent("chat-task:done", result)

	// 终态任务摘除：done 事件已发出，任务表仅保留在途任务（防已完成任务
	// 从不删除导致内存无界增长）。前端 restoreChatTaskState 对「查无此任务」
	// 已按终态处理（清空任务态防卡死），摘除不破坏事件丢失恢复链路。
	s.mu.Lock()
	delete(s.tasks, task.id)
	s.mu.Unlock()
}
