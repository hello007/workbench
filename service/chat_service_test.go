package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"workbench/model"
	"workbench/util/testutil"
)

// ===== 测试辅助 =====

// capturedSink 捕获 ChatService 发出的事件（测试断言事件序列与 payload）。
type capturedSink struct {
	mu     sync.Mutex
	events []capturedEvent
	done   chan model.ChatTaskRunResult // chat-task:done 载荷队列（多轮并发下按发生序取，防「取 last」错位）
}

type capturedEvent struct {
	name string
	data []any
}

func newCapturedSink() *capturedSink {
	return &capturedSink{done: make(chan model.ChatTaskRunResult, 16)}
}

func (c *capturedSink) Emit(name string, data ...any) {
	c.mu.Lock()
	c.events = append(c.events, capturedEvent{name: name, data: data})
	c.mu.Unlock()
	if name == "chat-task:done" {
		if r, ok := data[0].(model.ChatTaskRunResult); ok {
			select {
			case c.done <- r:
			default: // 队列满丢弃（测试断言面不消费如此多 done）
			}
		}
	}
}

// names 返回已捕获的事件名序列。
func (c *capturedSink) names() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.events))
	for _, e := range c.events {
		out = append(out, e.name)
	}
	return out
}

// countEvents 返回指定事件名的发生次数（带锁快照）。
func (c *capturedSink) countEvents(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, e := range c.events {
		if e.name == name {
			n++
		}
	}
	return n
}

// last 返回指定事件名的最后一次 payload（不存在时 t.Fatal）。
func (c *capturedSink) last(t *testing.T, name string) any {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.events) - 1; i >= 0; i-- {
		if c.events[i].name == name {
			return c.events[i].data[0]
		}
	}
	t.Fatalf("事件 %s 未捕获", name)
	return nil
}

// queuedIDs 返回全部 chat-task:queued 事件的 taskId（带锁快照，
// 供 waitFor 轮询闭包并发安全读取）。
func (c *capturedSink) queuedIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var ids []string
	for _, e := range c.events {
		if e.name == "chat-task:queued" {
			if id, ok := e.data[0].(map[string]any)["taskId"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// waitDone 等待一次 chat-task:done 事件并返回其载荷（按发生序消费，
// 多轮并发测试下不错位；超时 fail）。
func (c *capturedSink) waitDone(t *testing.T) model.ChatTaskRunResult {
	t.Helper()
	select {
	case r := <-c.done:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("等待 chat-task:done 超时")
		return model.ChatTaskRunResult{}
	}
}

// fakeChatProcess chatProcess 的 fake 实现：按预设行回放 stream-json 输出，
// 并捕获写入 stdin 的协议行（user 消息/control_response/interrupt 断言用）。
// lineCh 由调用方构造（正常流：带缓冲且已关闭；阻塞流：不关闭，靠 Kill 退出）。
type fakeChatProcess struct {
	lineCh     <-chan string
	startErr   error
	waitErr    error
	pipeErr    error // 非 nil 时输出流末尾以该错误关闭（scanner.Err 透传路径）
	mu         sync.Mutex
	started    bool
	killCalled bool
	exitCode   int
	gotName    string
	gotArgs    []string
	gotDir     string
	writer     *io.PipeWriter
	writeDone  chan struct{}
	killCh     chan struct{}
	killOnce   sync.Once
	stdinLines      []string      // 写入 stdin 的协议行（advanceSession/审批应答/中断断言用）
	stdinReadCloser io.ReadCloser // stdin 捕获管道读端（Kill 时关闭防捕获 goroutine 泄漏）
}

// newFakeLines 构造「立即回放全部行」的 fake（回放完进程退出，模拟单轮收尾）。
func newFakeLines(lines []string) *fakeChatProcess {
	ch := make(chan string, len(lines))
	for _, l := range lines {
		ch <- l
	}
	close(ch)
	return &fakeChatProcess{lineCh: ch, exitCode: -1, killCh: make(chan struct{})}
}

// newFakeBlocking 构造「输出后挂起等 Kill」的 fake，返回可继续发送行的发送端
// （持续会话进程模型：轮次结果由测试按需推送，进程长驻不退出）。
func newFakeBlocking(firstLine string) (*fakeChatProcess, chan<- string) {
	ch := make(chan string, 8)
	if firstLine != "" {
		ch <- firstLine
	}
	return &fakeChatProcess{lineCh: ch, exitCode: -1, killCh: make(chan struct{})}, ch
}

// newFakeLinesThenIdle 构造「回放全部行后进程保持存活」的 fake（行 channel 不
// 关闭，回放完阻塞至 Kill——模拟轮次收尾后空闲常驻的进程）。
func newFakeLinesThenIdle(lines ...string) *fakeChatProcess {
	ch := make(chan string, len(lines))
	for _, l := range lines {
		ch <- l
	}
	return &fakeChatProcess{lineCh: ch, exitCode: -1, killCh: make(chan struct{})}
}

func (f *fakeChatProcess) StdoutPipe() (io.ReadCloser, error) {
	r, w := io.Pipe()
	f.mu.Lock()
	f.writer = w
	f.mu.Unlock()
	return r, nil
}

// StdinPipe 返回捕获型 stdin 写端：写入行记录进 stdinLines（并发安全）。
// 读端句柄留存 Kill 时关闭（真实 exec.Cmd 进程退出后管道随之关闭，
// fake 须显式收口，否则捕获 goroutine 泄漏——goleak 门禁会拦截）。
func (f *fakeChatProcess) StdinPipe() (io.WriteCloser, error) {
	r, w := io.Pipe()
	f.mu.Lock()
	f.stdinReadCloser = r
	f.mu.Unlock()
	go func() {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			f.mu.Lock()
			f.stdinLines = append(f.stdinLines, sc.Text())
			f.mu.Unlock()
		}
	}()
	return w, nil
}

// stdinSnapshot 返回已写入 stdin 的协议行快照。
func (f *fakeChatProcess) stdinSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stdinLines...)
}

func (f *fakeChatProcess) Start() error {
	if f.startErr != nil {
		return f.startErr
	}
	f.mu.Lock()
	f.started = true
	f.mu.Unlock()
	f.writeDone = make(chan struct{})
	go func() {
		defer close(f.writeDone)
		for {
			select {
			case line, ok := <-f.lineCh:
				if !ok {
					if f.pipeErr != nil {
						f.writer.CloseWithError(f.pipeErr)
					} else {
						f.writer.Close()
					}
					return
				}
				if _, err := io.WriteString(f.writer, line+"\n"); err != nil {
					return
				}
			case <-f.killCh:
				f.writer.Close()
				return
			}
		}
	}()
	return nil
}

func (f *fakeChatProcess) Wait() error {
	if f.writeDone != nil {
		<-f.writeDone
	}
	if f.waitErr == nil {
		f.mu.Lock()
		f.exitCode = 0
		f.mu.Unlock()
	}
	// 进程退出后 stdin 管道随之关闭（对齐 exec.Cmd 行为，防捕获 goroutine 泄漏）
	f.mu.Lock()
	stdinRC := f.stdinReadCloser
	f.mu.Unlock()
	if stdinRC != nil {
		_ = stdinRC.Close()
	}
	return f.waitErr
}

func (f *fakeChatProcess) Kill() {
	f.mu.Lock()
	f.killCalled = true
	stdinRC := f.stdinReadCloser
	f.mu.Unlock()
	f.killOnce.Do(func() {
		close(f.killCh)
		// 关闭 stdin 读端：捕获 goroutine 随 Scan 退出（防泄漏）
		if stdinRC != nil {
			_ = stdinRC.Close()
		}
	})
}

func (f *fakeChatProcess) ExitCode() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exitCode
}

// wireFakeFactory 注入 fake 进程工厂并记录每次构造的参数（多轮断言用）。
func wireFakeFactory(svc *ChatService, makeFake func() *fakeChatProcess) *[]*fakeChatProcess {
	fakes := &[]*fakeChatProcess{}
	svc.setProcessFactory(func(ctx context.Context, name string, args []string, dir string) chatProcess {
		f := makeFake()
		f.mu.Lock()
		f.gotName = name
		f.gotArgs = append([]string(nil), args...)
		f.gotDir = dir
		f.mu.Unlock()
		*fakes = append(*fakes, f)
		return f
	})
	return fakes
}

// newChatSvcForTest 构造指向临时目录、事件捕获注入完成的 ChatService。
func newChatSvcForTest(t *testing.T) (*ChatService, *capturedSink) {
	t.Helper()
	svc := NewChatService(context.Background(), filepath.Join(t.TempDir(), "ai_chat"))
	sink := newCapturedSink()
	svc.SetEventSink(sink)
	return svc, sink
}

// assistantLine 构造一条含文本增量的 assistant stream-json 事件行。
func assistantLine(sessionID, text string) string {
	return fmt.Sprintf(`{"type":"assistant","session_id":%q,"message":{"role":"assistant","content":[{"type":"text","text":%q}]}}`, sessionID, text)
}

// resultLine 构造一条 result stream-json 事件行。
func resultLine(sessionID string) string {
	return fmt.Sprintf(`{"type":"result","subtype":"success","is_error":false,"duration_ms":120,"num_turns":1,"total_cost_usd":0.01,"session_id":%q,"usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`, sessionID)
}

// resultLineError 构造一条指定 subtype/is_error 的 result 失败事件行
// （error_during_execution = 中断收尾形态）。
func resultLineError(sessionID, subtype string) string {
	return fmt.Sprintf(`{"type":"result","subtype":%q,"is_error":true,"duration_ms":50,"num_turns":1,"total_cost_usd":0,"session_id":%q,"result":"已被用户中断"}`, subtype, sessionID)
}

// waitFor 轮询等待条件成立（超时 fail），替代固定 sleep（非 flaky 约定）。
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

// chatApprovalPending 锁内查询审批是否待应答（-race 下测试直读 svc.approvals
// 会与 pump 写入竞态，统一走本访问器）。
func chatApprovalPending(svc *ChatService, requestID string) bool {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return svc.approvals[requestID] != nil
}

// appErrCode 断言 err 为 AppError 并返回其 Code。
func appErrCode(t *testing.T, err error) string {
	t.Helper()
	var ae *model.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("期望 AppError，got %v", err)
	}
	return ae.Code
}

// ===== 会话 CRUD =====

// TestChatService_CreateAndList 创建与列表：目录过滤 + 最近活跃降序 + 列表不含消息。
func TestChatService_CreateAndList(t *testing.T) {
	svc, _ := newChatSvcForTest(t)

	s1, err := svc.CreateChatSession("dir-1", "项目管理", "C:/proj/a")
	if err != nil {
		t.Fatalf("CreateChatSession: %v", err)
	}
	time.Sleep(2 * time.Millisecond) // 制造不同 UpdatedAt
	s2, _ := svc.CreateChatSession("dir-1", "", "C:/proj/b")
	time.Sleep(2 * time.Millisecond)
	s3, _ := svc.CreateChatSession("dir-2", "其他目录", "C:/proj/c")

	if s1.ID == "" || s2.Title != "新会话" {
		t.Errorf("创建结果异常: s1.ID=%q s2.Title=%q", s1.ID, s2.Title)
	}

	// 目录过滤
	list1, err := svc.ListChatSessions("dir-1")
	if err != nil {
		t.Fatalf("ListChatSessions: %v", err)
	}
	if len(list1) != 2 {
		t.Fatalf("dir-1 会话数: got %d, want 2", len(list1))
	}
	// 最近活跃降序：s2 创建晚于 s1
	if list1[0].ID != s2.ID || list1[1].ID != s1.ID {
		t.Errorf("排序错误: got [%s, %s], want [%s, %s]", list1[0].ID, list1[1].ID, s2.ID, s1.ID)
	}
	for _, sess := range list1 {
		if sess.Messages != nil {
			t.Error("列表项不应携带消息（轻量列表）")
		}
	}

	listAll, _ := svc.ListChatSessions("")
	if len(listAll) != 3 || listAll[0].ID != s3.ID {
		t.Errorf("空目录 ID 应返回全部且按活跃降序: got %d 项", len(listAll))
	}

	empty, _ := svc.ListChatSessions("dir-none")
	if len(empty) != 0 {
		t.Errorf("不存在目录应返回空列表, got %d", len(empty))
	}
}

// TestChatService_GetSession_Messages 会话读取：消息加载与追加、空消息返回非 nil 数组。
func TestChatService_GetSession_Messages(t *testing.T) {
	svc, _ := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "测试", "C:/proj")

	// 新会话：消息为空数组（非 nil，前端可直接遍历）
	got, err := svc.GetChatSession(sess.ID)
	if err != nil {
		t.Fatalf("GetChatSession: %v", err)
	}
	if got.Messages == nil || len(got.Messages) != 0 {
		t.Errorf("新会话消息应为空数组, got %#v", got.Messages)
	}

	// 追加消息后可读回（追加序 = 时间升序）
	msgs := []model.ChatMessage{
		{Role: model.ChatRoleUser, Content: "第一问", Timestamp: 1, TaskID: "t1"},
		{Role: model.ChatRoleAssistant, Content: "第一答", Timestamp: 2, TaskID: "t1"},
	}
	if err := svc.appendMessages(sess.ID, msgs...); err != nil {
		t.Fatalf("appendMessages: %v", err)
	}
	got, err = svc.GetChatSession(sess.ID)
	if err != nil {
		t.Fatalf("GetChatSession after append: %v", err)
	}
	if len(got.Messages) != 2 || got.Messages[0].Content != "第一问" || got.Messages[1].Role != model.ChatRoleAssistant {
		t.Errorf("消息读回异常: %#v", got.Messages)
	}
}

// TestChatService_UpdateTitle_Delete 标题更新刷新活跃时间；删除移除索引与消息文件。
func TestChatService_UpdateTitle_Delete(t *testing.T) {
	svc, _ := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "旧标题", "C:/proj")
	svc.appendMessages(sess.ID, model.ChatMessage{Role: model.ChatRoleUser, Content: "hi", Timestamp: 1})
	oldUpdatedAt := sess.UpdatedAt
	time.Sleep(2 * time.Millisecond)

	if err := svc.UpdateChatSessionTitle(sess.ID, "新标题"); err != nil {
		t.Fatalf("UpdateChatSessionTitle: %v", err)
	}
	got, _ := svc.GetChatSession(sess.ID)
	if got.Title != "新标题" || got.UpdatedAt <= oldUpdatedAt {
		t.Errorf("标题/活跃时间未更新: title=%q updatedAt=%d", got.Title, got.UpdatedAt)
	}

	if err := svc.DeleteChatSession(sess.ID); err != nil {
		t.Fatalf("DeleteChatSession: %v", err)
	}
	if _, err := svc.GetChatSession(sess.ID); appErrCode(t, err) != model.ErrCodeChatSessionNotFound {
		t.Errorf("删除后读取应返回 SessionNotFound, got %v", err)
	}
	if list, _ := svc.ListChatSessions("dir-1"); len(list) != 0 {
		t.Errorf("删除后列表应为空, got %d", len(list))
	}
	if _, err := os.Stat(filepath.Join(svc.messagesDir(), sess.ID+".json")); !os.IsNotExist(err) {
		t.Error("删除后消息文件应被移除")
	}
	if err := svc.DeleteChatSession(sess.ID); appErrCode(t, err) != model.ErrCodeChatSessionNotFound {
		t.Errorf("重复删除应返回 SessionNotFound, got %v", err)
	}
	if err := svc.UpdateChatSessionTitle(sess.ID, "x"); appErrCode(t, err) != model.ErrCodeChatSessionNotFound {
		t.Errorf("改标题命中不存在会话应返回 SessionNotFound, got %v", err)
	}
}

// TestChatService_PersistenceReload 重新构造 service（同目录）后会话与消息仍在（文件持久化）。
func TestChatService_PersistenceReload(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "ai_chat")
	svc1 := NewChatService(context.Background(), dataDir)
	sess, _ := svc1.CreateChatSession("dir-1", "持久化", "C:/proj")
	svc1.appendMessages(sess.ID, model.ChatMessage{Role: model.ChatRoleUser, Content: " persists", Timestamp: 1})

	svc2 := NewChatService(context.Background(), dataDir)
	got, err := svc2.GetChatSession(sess.ID)
	if err != nil {
		t.Fatalf("重载后 GetChatSession: %v", err)
	}
	if got.Title != "持久化" || len(got.Messages) != 1 {
		t.Errorf("重载数据不一致: %+v", got)
	}
}

// TestChatService_IndexCorruptFallback 索引损坏时备份原文件并降级为空索引，不阻塞功能。
func TestChatService_IndexCorruptFallback(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "ai_chat")
	testutil.WriteFile(t, filepath.Join(dataDir, "sessions.json"), "{not-valid-json")

	svc := NewChatService(context.Background(), dataDir)
	list, err := svc.ListChatSessions("")
	if err != nil || len(list) != 0 {
		t.Fatalf("损坏索引应降级为空列表, got %v, %v", list, err)
	}
	// 降级后仍可正常创建（备份文件已生成）
	if _, err := svc.CreateChatSession("d", "t", "c"); err != nil {
		t.Fatalf("降级后创建会话失败: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dataDir, "sessions.json.bak.*"))
	if len(matches) != 1 {
		t.Errorf("损坏索引应留备份文件, got %v", matches)
	}
}

// ===== 参数组装 =====

// TestBuildChatArgs 表驱动：持续会话进程形态的 resume/permission-mode/model
// 追加规则与审批协议开关（--permission-prompt-tool stdio 漏配 = 审批静默自动
// 拒绝，单测守护该开关恒在）。permission-mode 恒传（含 default 与空串归一），
// 防 CLI 继承用户全局 settings defaultMode 绕过审批协议。
func TestBuildChatArgs(t *testing.T) {
	cases := []struct {
		name   string
		resume string
		mode   string
		mdl    string
		want   []string
	}{
		{
			name: "首轮默认模式显式传参", resume: "", mode: "default", mdl: "",
			want: []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-mode", "default", "--disallowedTools", "AskUserQuestion", "--permission-prompt-tool", "stdio"},
		},
		{
			name: "恢复会话全参数", resume: "s1", mode: "acceptEdits", mdl: "sonnet",
			want: []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--resume", "s1", "--permission-mode", "acceptEdits", "--model", "sonnet", "--disallowedTools", "AskUserQuestion", "--permission-prompt-tool", "stdio"},
		},
		{
			name: "权限模式为空归一为 default", resume: "", mode: "", mdl: "opus",
			want: []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-mode", "default", "--model", "opus", "--disallowedTools", "AskUserQuestion", "--permission-prompt-tool", "stdio"},
		},
		{
			name: "plan 模式", resume: "", mode: "plan", mdl: "",
			want: []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-mode", "plan", "--disallowedTools", "AskUserQuestion", "--permission-prompt-tool", "stdio"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildChatArgs(tc.resume, tc.mode, tc.mdl)
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("buildChatArgs:\n got  %v\n want %v", got, tc.want)
			}
		})
	}
}

// ===== RunChat 全链路（fake 进程注入，不真调 claude）=====

// controlRequestLine 构造一条 control_request/can_use_tool 审批下发事件行。
func controlRequestLine(requestID, toolName, inputJSON, description string) string {
	return fmt.Sprintf(`{"type":"control_request","request_id":%q,"request":{"subtype":"can_use_tool","tool_name":%q,"input":%s,"description":%q}}`,
		requestID, toolName, inputJSON, description)
}

// stdinUserText 解析 fake 捕获的 stdin user 消息行，返回 claude 收到的文本。
func stdinUserText(t *testing.T, line string) string {
	t.Helper()
	var payload struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("解析 stdin user 消息行失败: %v\nline: %s", err, line)
	}
	if payload.Type != "user" || payload.Message.Role != "user" || len(payload.Message.Content) == 0 {
		t.Fatalf("stdin user 消息形态异常: %+v", payload)
	}
	return payload.Message.Content[0].Text
}

// TestChatService_RunChat_FullFlow 首轮对话全链路：事件序列、进程参数组装
// （审批协议开关恒在）、stdin user 消息声明注入、user/assistant 消息入档、
// claude session id 回写会话。
func TestChatService_RunChat_FullFlow(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, err := svc.CreateChatSession("dir-1", "流程", "C:/proj/a")
	if err != nil {
		t.Fatalf("CreateChatSession: %v", err)
	}

	lines := []string{
		fmt.Sprintf(`{"type":"system","subtype":"init","session_id":%q}`, "claude-s-1"),
		assistantLine("claude-s-1", "你好，有什么可以帮你？"),
		resultLine("claude-s-1"),
	}
	fakes := wireFakeFactory(svc, func() *fakeChatProcess { return newFakeLines(lines) })

	taskID, err := svc.RunChat(sess.ID, "你好", "acceptEdits", "sonnet")
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}

	result := sink.waitDone(t)

	// done payload
	if result.TaskID != taskID || result.ChatSessionID != sess.ID {
		t.Errorf("done payload id 不符: %+v", result)
	}
	if result.Reply != "你好，有什么可以帮你？" {
		t.Errorf("回复全文不符: %q", result.Reply)
	}
	if result.ClaudeSessionID != "claude-s-1" || result.Canceled || result.Error != "" || result.ExitCode != 0 {
		t.Errorf("done payload 异常: %+v", result)
	}

	// 事件序列（同步段 queued/started 在前，goroutine 段 output/done 在后）
	names := sink.names()
	wantNames := []string{"chat-task:queued", "chat-task:started", "chat-task:output", "chat-task:done"}
	if len(names) < len(wantNames) {
		t.Fatalf("事件数不足: %v", names)
	}
	for i, want := range wantNames {
		if names[i] != want {
			t.Errorf("事件序列[%d]: got %q, want %q (all=%v)", i, names[i], want, names)
		}
	}

	// 子进程参数与工作目录：prompt 不走命令行（经 stdin 注入）
	f := (*fakes)[0]
	f.mu.Lock()
	args := f.gotArgs
	dir := f.gotDir
	name := f.gotName
	f.mu.Unlock()
	if name != "claude" {
		t.Errorf("进程名: got %q", name)
	}
	if dir != "C:/proj/a" {
		t.Errorf("工作目录: got %q", dir)
	}
	gotArgs := strings.Join(args, " ")
	if !strings.Contains(gotArgs, "--input-format stream-json") || !strings.Contains(gotArgs, "--output-format stream-json --verbose") {
		t.Errorf("args 缺流式双向格式: %v", args)
	}
	if strings.Contains(gotArgs, "--resume") {
		t.Errorf("首轮不应带 --resume: %v", args)
	}
	if !strings.Contains(gotArgs, "--permission-mode acceptEdits") {
		t.Errorf("args 缺 permission-mode: %v", args)
	}
	if !strings.Contains(gotArgs, "--model sonnet") {
		t.Errorf("args 缺 model: %v", args)
	}
	// 审批协议开关恒在（漏配 = 审批静默自动拒绝，无报错极难察觉）
	if !strings.Contains(gotArgs, "--permission-prompt-tool stdio") {
		t.Errorf("args 缺 --permission-prompt-tool stdio: %v", args)
	}
	if !strings.Contains(gotArgs, "--disallowedTools AskUserQuestion") {
		t.Errorf("args 缺 --disallowedTools: %v", args)
	}

	// stdin user 消息：首轮 = chat-question 环境声明前缀 + 用户原文（仅进 claude stdin）
	stdinLines := f.stdinSnapshot()
	if len(stdinLines) != 1 {
		t.Fatalf("stdin 写入行数: got %d, want 1 (%v)", len(stdinLines), stdinLines)
	}
	text := stdinUserText(t, stdinLines[0])
	if !strings.HasPrefix(text, chatQuestionPreamble) || !strings.HasSuffix(text, "你好") {
		t.Errorf("首轮 claude 文本应为声明前缀+原文: %q", text)
	}

	// 消息入档：user + assistant 各一条，taskID 关联（落盘 user 消息保持原文）
	got, err := svc.GetChatSession(sess.ID)
	if err != nil {
		t.Fatalf("GetChatSession: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("消息条数: got %d, want 2", len(got.Messages))
	}
	if got.Messages[0].Role != model.ChatRoleUser || got.Messages[0].Content != "你好" || got.Messages[0].TaskID != taskID {
		t.Errorf("user 消息异常: %+v", got.Messages[0])
	}
	if got.Messages[1].Role != model.ChatRoleAssistant || got.Messages[1].Content != "你好，有什么可以帮你？" || got.Messages[1].TaskID != taskID {
		t.Errorf("assistant 消息异常: %+v", got.Messages[1])
	}

	// claude session id 回写会话元数据（下次 spawn --resume 依据）
	if got.ClaudeSessionID != "claude-s-1" {
		t.Errorf("会话 ClaudeSessionID 未回写: %q", got.ClaudeSessionID)
	}

	// 终态轮次已从在途表摘除：GetChatTaskState 返回 nil（前端
	// restoreChatTaskState 对「查无此任务」按终态清态处理，恢复链路不破坏）
	if st := svc.GetChatTaskState(taskID); st != nil {
		t.Errorf("终态轮次应已摘除, got %+v", st)
	}
}

// TestChatService_RunChat_ResumeSecondTurn 进程退出后再次发消息自动 respawn 并
// --resume 续上一轮 claude 会话；续轮不重复注入环境声明。
func TestChatService_RunChat_ResumeSecondTurn(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "续会话", "C:/proj")

	turn := 0
	fakes := wireFakeFactory(svc, func() *fakeChatProcess {
		turn++
		return newFakeLines([]string{
			assistantLine(fmt.Sprintf("claude-s-%d", turn), fmt.Sprintf("答%d", turn)),
			resultLine(fmt.Sprintf("claude-s-%d", turn)),
		})
	})

	if _, err := svc.RunChat(sess.ID, "第一问", "default", ""); err != nil {
		t.Fatalf("第一轮 RunChat: %v", err)
	}
	sink.waitDone(t)
	// 第一轮 fake 回放完即退出（单轮回放形态），注册表应已摘除
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.sessions) == 0
	})

	if _, err := svc.RunChat(sess.ID, "第二问", "default", ""); err != nil {
		t.Fatalf("第二轮 RunChat: %v", err)
	}
	result := sink.waitDone(t)

	// 第二轮 spawn 参数带 --resume claude-s-1（第一轮回写的会话 id）
	waitFor(t, 5*time.Second, func() bool { return len(*fakes) >= 2 })
	f := (*fakes)[1]
	f.mu.Lock()
	args := strings.Join(f.gotArgs, " ")
	f.mu.Unlock()
	if !strings.Contains(args, "--resume claude-s-1") {
		t.Errorf("第二轮应带 --resume: %v", f.gotArgs)
	}
	// 续轮不重复注入环境声明（claude 上下文已含）
	stdinLines := f.stdinSnapshot()
	if len(stdinLines) != 1 {
		t.Fatalf("第二轮 stdin 写入行数: got %d, want 1", len(stdinLines))
	}
	if text := stdinUserText(t, stdinLines[0]); text != "第二问" {
		t.Errorf("第二轮 claude 文本不应含声明前缀: %q", text)
	}
	if result.ClaudeSessionID != "claude-s-2" {
		t.Errorf("第二轮 claude 会话 id: got %q", result.ClaudeSessionID)
	}

	// 消息累积四条（两轮 user+assistant）
	got, _ := svc.GetChatSession(sess.ID)
	if len(got.Messages) != 4 {
		t.Errorf("两轮消息应为 4 条, got %d", len(got.Messages))
	}
}

// TestChatService_RunChat_InterjectionQueue 运行中插话（排队注入）：同会话
// 执行轮在途时再发消息不再拒绝（原 ErrCodeChatInProgress 语义退役），进入
// FIFO 队列，前序轮 result 后依序写入 stdin、逐轮收尾。
func TestChatService_RunChat_InterjectionQueue(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "插话", "C:/proj")

	fake, sender := newFakeBlocking("")
	wireFakeFactory(svc, func() *fakeChatProcess { return fake })

	// 第一轮：阻塞在执行中（无 result）
	taskID1, err := svc.RunChat(sess.ID, "第一问", "default", "")
	if err != nil {
		t.Fatalf("第一轮 RunChat: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		st := svc.GetChatTaskState(taskID1)
		return st != nil && st.Running
	})

	// 插话：同会话第二条消息被接受（排队态），不报 ErrCodeChatInProgress
	taskID2, err := svc.RunChat(sess.ID, "插话内容", "default", "")
	if err != nil {
		t.Fatalf("插话 RunChat 应被接受: %v", err)
	}
	st := svc.GetChatTaskState(taskID2)
	if st == nil || !st.Queued {
		t.Errorf("插话轮应处于排队态: %+v", st)
	}
	// 排队轮尚未写 stdin（前序轮 result 前不推进）
	if n := len(fake.stdinSnapshot()); n != 1 {
		t.Errorf("排队轮不应写 stdin, stdin 行数: %d", n)
	}

	// 第一轮收尾：result#1 → 插话轮自动推进写入 stdin
	sender <- assistantLine("claude-s-1", "答一")
	sender <- resultLine("claude-s-1")
	result1 := sink.waitDone(t)
	if result1.TaskID != taskID1 {
		t.Errorf("第一个 done 应为第一轮: %+v", result1)
	}
	waitFor(t, 5*time.Second, func() bool {
		st := svc.GetChatTaskState(taskID2)
		return st != nil && st.Running
	})
	waitFor(t, 5*time.Second, func() bool { return len(fake.stdinSnapshot()) >= 2 })
	stdinLines := fake.stdinSnapshot()
	if text := stdinUserText(t, stdinLines[1]); text != "插话内容" {
		t.Errorf("第二条 stdin 消息应为插话原文: %q", text)
	}
	// 插话轮不再注入环境声明（init 已收到，claudeSessionID 非空）
	if strings.Contains(stdinLines[1], "chat-question") {
		t.Error("插话轮不应注入环境声明")
	}

	// 插话轮收尾
	sender <- assistantLine("claude-s-1", "答二")
	sender <- resultLine("claude-s-1")
	result2 := sink.waitDone(t)
	if result2.TaskID != taskID2 || result2.Reply != "答二" {
		t.Errorf("插话轮收尾异常: %+v", result2)
	}

	// 两轮消息入档 4 条（user+assistant 各二）
	got, _ := svc.GetChatSession(sess.ID)
	if len(got.Messages) != 4 {
		t.Errorf("两轮消息应为 4 条, got %d", len(got.Messages))
	}

	// 收尾：杀进程防 goroutine 泄漏
	svc.CloseAll()
	waitFor(t, 5*time.Second, func() bool {
		return svc.GetChatTaskState(taskID2) == nil
	})
}

// TestChatService_CancelChatTask_Running 执行中取消：发 interrupt 控制请求、
// 被中断轮 result 收尾后 done canceled、部分回复保留入档、进程不被杀
// （持续会话进程存活，后续消息可继续）。
func TestChatService_CancelChatTask_Running(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "取消", "C:/proj")

	fake, sender := newFakeBlocking(assistantLine("claude-s-1", "部分回复"))
	wireFakeFactory(svc, func() *fakeChatProcess { return fake })

	taskID, err := svc.RunChat(sess.ID, "长任务", "default", "")
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		st := svc.GetChatTaskState(taskID)
		return st != nil && st.Running
	})

	if !svc.CancelChatTask(taskID) {
		t.Fatal("CancelChatTask 应返回 true")
	}
	// 幂等：重复取消不重复发 interrupt、不重复 emit done
	_ = svc.CancelChatTask(taskID)
	// stdin 应收到 interrupt 控制请求恰一次
	waitFor(t, 5*time.Second, func() bool {
		return len(fake.stdinSnapshot()) == 2 // 第 1 行 user 消息 + 第 2 行 interrupt
	})
	var interruptCount int
	for _, line := range fake.stdinSnapshot() {
		if strings.Contains(line, `"subtype":"interrupt"`) {
			interruptCount++
		}
	}
	if interruptCount != 1 {
		t.Errorf("interrupt 应恰一次, got %d", interruptCount)
	}

	// 被中断轮以 error_during_execution result 收尾 → done canceled
	sender <- resultLineError("claude-s-1", "error_during_execution")
	result := sink.waitDone(t)
	if !result.Canceled || result.Error == "" {
		t.Errorf("取消结果异常: %+v", result)
	}
	if !strings.Contains(result.Reply, "部分回复") {
		t.Errorf("取消轮应保留部分回复: %q", result.Reply)
	}
	fake.mu.Lock()
	killed := fake.killCalled
	fake.mu.Unlock()
	if killed {
		t.Error("执行中取消应走 interrupt 协议，不应杀进程（会话需继续可用）")
	}

	// 部分回复已入档（取消轮 user+assistant 各一条）
	got, _ := svc.GetChatSession(sess.ID)
	if len(got.Messages) != 2 || got.Messages[1].Content != "部分回复" {
		t.Errorf("取消轮消息入档异常: %+v", got.Messages)
	}

	// done 恒一次（重复取消幂等，不重复 emit）
	waitFor(t, 5*time.Second, func() bool {
		return svc.GetChatTaskState(taskID) == nil
	})
	if n := sink.countEvents("chat-task:done"); n != 1 {
		t.Errorf("done 事件应恰一次, got %d", n)
	}

	// 进程仍存活：后续消息可直接复用（不重新 spawn）
	if _, err := svc.RunChat(sess.ID, "继续", "default", ""); err != nil {
		t.Fatalf("取消后继续会话失败: %v", err)
	}
	svc.CloseAll()
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.turns) == 0
	})
}

// TestChatService_CancelChatTask_QueuedInjection 排队轮取消：插话排队中的轮次
// 从 Go 侧队列摘除、立即 done canceled、不写 stdin、user 消息不落盘。
func TestChatService_CancelChatTask_QueuedInjection(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "排队取消", "C:/proj")

	fake, sender := newFakeBlocking("")
	wireFakeFactory(svc, func() *fakeChatProcess { return fake })

	taskID1, err := svc.RunChat(sess.ID, "第一问", "default", "")
	if err != nil {
		t.Fatalf("第一轮 RunChat: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		st := svc.GetChatTaskState(taskID1)
		return st != nil && st.Running
	})
	taskID2, err := svc.RunChat(sess.ID, "排队消息", "default", "")
	if err != nil {
		t.Fatalf("插话 RunChat: %v", err)
	}

	if !svc.CancelChatTask(taskID2) {
		t.Fatal("排队取消应返回 true")
	}
	_ = svc.CancelChatTask(taskID2) // 幂等
	result := sink.waitDone(t)
	if result.TaskID != taskID2 || !result.Canceled {
		t.Errorf("排队取消 done 异常: %+v", result)
	}

	// 第一轮收尾：被取消的排队轮不再推进（stdin 恒 1 行 user 消息）
	sender <- assistantLine("claude-s-1", "答一")
	sender <- resultLine("claude-s-1")
	result1 := sink.waitDone(t)
	if result1.TaskID != taskID1 {
		t.Errorf("剩余 done 应为第一轮: %+v", result1)
	}
	waitFor(t, 5*time.Second, func() bool {
		return svc.GetChatTaskState(taskID1) == nil
	})
	if n := len(fake.stdinSnapshot()); n != 1 {
		t.Errorf("被取消排队轮不应写 stdin, stdin 行数: %d", n)
	}
	// user 消息只落盘第一轮的（排队取消不留痕）
	got, _ := svc.GetChatSession(sess.ID)
	if len(got.Messages) != 2 || got.Messages[0].Content != "第一问" {
		t.Errorf("排队取消轮不应落盘消息: %+v", got.Messages)
	}
	if n := sink.countEvents("chat-task:done"); n != 2 {
		t.Errorf("done 事件应恰两次（排队取消+第一轮收尾）, got %d", n)
	}
	svc.CloseAll()
}

// TestChatService_RunChat_ProcessLimit 进程槽位上限：第 chatMaxConcurrent+1 个
// 会话首条消息直接报 ErrCodeChatProcessLimit（不排队占位）。
func TestChatService_RunChat_ProcessLimit(t *testing.T) {
	svc, _ := newChatSvcForTest(t)

	for i := 0; i < chatMaxConcurrent; i++ {
		sess, _ := svc.CreateChatSession(fmt.Sprintf("dir-%d", i), "占位", "C:/p")
		fake, _ := newFakeBlocking("")
		wireFakeFactory(svc, func() *fakeChatProcess { return fake })
		if _, err := svc.RunChat(sess.ID, "占用", "default", ""); err != nil {
			t.Fatalf("占位 RunChat: %v", err)
		}
	}

	limSess, _ := svc.CreateChatSession("dir-limit", "超限", "C:/p")
	_, err := svc.RunChat(limSess.ID, "超限消息", "default", "")
	if code := appErrCode(t, err); code != model.ErrCodeChatProcessLimit {
		t.Errorf("超限应返回 ProcessLimit, got %v", err)
	}
	// 已占位会话不受影响（同会话轮次不新增进程）
	svc.CloseAll()
}

// TestChatService_RunChat_ParamChangeRestart 权限模式/模型变更且进程空闲：
// 优雅重启进程（旧进程摘除，新进程 spawn 带 --resume 与新参数）；执行中变更
// 沿用旧参数不重启。
func TestChatService_RunChat_ParamChangeRestart(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "参数重启", "C:/proj")

	turn := 0
	fakes := wireFakeFactory(svc, func() *fakeChatProcess {
		turn++
		return newFakeLines([]string{
			assistantLine(fmt.Sprintf("claude-s-%d", turn), fmt.Sprintf("答%d", turn)),
			resultLine(fmt.Sprintf("claude-s-%d", turn)),
		})
	})

	if _, err := svc.RunChat(sess.ID, "第一问", "acceptEdits", ""); err != nil {
		t.Fatalf("第一轮 RunChat: %v", err)
	}
	sink.waitDone(t)
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.sessions) == 0
	})

	// 参数变更且空闲：重启进程，新参数生效 + --resume 续上下文
	if _, err := svc.RunChat(sess.ID, "第二问", "plan", "sonnet"); err != nil {
		t.Fatalf("参数变更 RunChat: %v", err)
	}
	sink.waitDone(t)
	waitFor(t, 5*time.Second, func() bool { return len(*fakes) >= 2 })
	f := (*fakes)[1]
	f.mu.Lock()
	args := strings.Join(f.gotArgs, " ")
	f.mu.Unlock()
	if !strings.Contains(args, "--resume claude-s-1") {
		t.Errorf("重启进程应带 --resume: %v", f.gotArgs)
	}
	if !strings.Contains(args, "--permission-mode plan") || !strings.Contains(args, "--model sonnet") {
		t.Errorf("重启进程应带新参数: %v", f.gotArgs)
	}
	svc.CloseAll()
}

// TestChatService_RunChat_Validation 入参与会话校验：空 prompt、会话不存在。
func TestChatService_RunChat_Validation(t *testing.T) {
	svc, _ := newChatSvcForTest(t)

	if _, err := svc.RunChat("sess-x", "  ", "default", ""); appErrCode(t, err) != model.ErrCodeChatEmptyPrompt {
		t.Errorf("空 prompt 应返回 EmptyPrompt, got %v", err)
	}
	if _, err := svc.RunChat("sess-x", "hi", "default", ""); appErrCode(t, err) != model.ErrCodeChatSessionNotFound {
		t.Errorf("会话不存在应返回 SessionNotFound, got %v", err)
	}
}

// TestChatService_RunChat_StartFailure 进程启动失败：同步报错、不留 user 消息、
// 槽位归还、失败轮次不残留在途表与注册表。
func TestChatService_RunChat_StartFailure(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "启动失败", "C:/proj")

	wireFakeFactory(svc, func() *fakeChatProcess {
		return &fakeChatProcess{startErr: errors.New("exec: not found"), exitCode: -1, killCh: make(chan struct{})}
	})

	if _, err := svc.RunChat(sess.ID, "hi", "default", ""); err == nil {
		t.Fatal("启动失败应返回错误")
	}

	// 失败轮次不残留在途表与进程注册表（防内存无界增长与假性进程占用）
	svc.mu.Lock()
	residueTurns, residueSessions := len(svc.turns), len(svc.sessions)
	svc.mu.Unlock()
	if residueTurns != 0 || residueSessions != 0 {
		t.Errorf("启动失败后应无残留, turns=%d sessions=%d", residueTurns, residueSessions)
	}

	// 槽位已归还：后续任务可正常获取（再注入正常 fake 跑通一轮）
	lines := []string{assistantLine("claude-s-9", "ok"), resultLine("claude-s-9")}
	wireFakeFactory(svc, func() *fakeChatProcess { return newFakeLines(lines) })
	sess2, _ := svc.CreateChatSession("dir-1", "正常", "C:/proj")
	if _, err := svc.RunChat(sess2.ID, "hi", "default", ""); err != nil {
		t.Fatalf("槽位归还后 RunChat 失败: %v", err)
	}
	// 等待正常轮收尾（pump 落盘完成后测试才返回，防与 TempDir 清理竞争）
	sink.waitDone(t)
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.turns) == 0
	})

	// 启动失败轮不留 user 消息（仅正常轮 2 条）
	got, _ := svc.GetChatSession(sess.ID)
	if len(got.Messages) != 0 {
		t.Errorf("启动失败轮不应留消息, got %d", len(got.Messages))
	}
}

// TestChatService_GetChatTaskState_Missing 查询/取消不存在的任务：查询返回 nil
// （前端 restoreChatTaskState 以此判终态清态）、取消返回 false。
// 「排队不侵蚀超时预算」的真实排队用例见 TestChatService_RunChat_QueuedTimeoutCtxNotStarted。
func TestChatService_GetChatTaskState_Missing(t *testing.T) {
	svc, _ := newChatSvcForTest(t)
	if st := svc.GetChatTaskState("nonexistent"); st != nil {
		t.Errorf("不存在任务应返回 nil, got %+v", st)
	}
	if svc.CancelChatTask("nonexistent") {
		t.Error("不存在任务取消应返回 false")
	}
}

// ===== 审核缺陷回归测试（09-22-ai-5-5-21）=====

// TestNewChatTaskID_Unique B5：task id 含原子递增序号，同 tick 连续生成不重复。
func TestNewChatTaskID_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := newChatTaskID()
		if _, dup := seen[id]; dup {
			t.Fatalf("task id 重复: %s（第 %d 次生成）", id, i)
		}
		seen[id] = struct{}{}
	}
}

// ===== 审批与生命周期（持续会话进程模型）=====

// TestChatService_Approval_AllowDeny 工具授权审批双路径：control_request 下发
// → chat-approval:request 事件 → RespondChatApproval allow/deny → stdin 收到
// 对应 control_response（按 request_id 匹配）。
func TestChatService_Approval_AllowDeny(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "审批", "C:/proj")

	fake, sender := newFakeBlocking(assistantLine("claude-s-1", "开始"))
	wireFakeFactory(svc, func() *fakeChatProcess { return fake })

	taskID, err := svc.RunChat(sess.ID, "建目录", "default", "")
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		st := svc.GetChatTaskState(taskID)
		return st != nil && st.Running
	})

	// CLI 下发审批请求 → 事件推送
	sender <- controlRequestLine("req-allow-1", "Bash", `{"command":"mkdir d1"}`, "创建目录 d1")
	var approvalEv model.ChatApprovalRequest
	waitFor(t, 5*time.Second, func() bool {
		sink.mu.Lock()
		defer sink.mu.Unlock()
		for i := len(sink.events) - 1; i >= 0; i-- {
			if sink.events[i].name == "chat-approval:request" {
				approvalEv = sink.events[i].data[0].(model.ChatApprovalRequest)
				return true
			}
		}
		return false
	})
	if approvalEv.RequestID != "req-allow-1" || approvalEv.ToolName != "Bash" || approvalEv.TaskID != taskID {
		t.Errorf("审批事件载荷异常: %+v", approvalEv)
	}

	// allow 应答：stdin 收到 control_response/allow（updatedInput 回传原 input）
	if err := svc.RespondChatApproval("req-allow-1", true); err != nil {
		t.Fatalf("RespondChatApproval allow: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return len(fake.stdinSnapshot()) >= 2 })
	lines := fake.stdinSnapshot()
	var found bool
	for _, line := range lines[1:] {
		var payload map[string]any
		if json.Unmarshal([]byte(line), &payload) != nil || payload["type"] != "control_response" {
			continue
		}
		resp := payload["response"].(map[string]any)
		if resp["request_id"] == "req-allow-1" {
			found = true
			inner := resp["response"].(map[string]any)
			if inner["behavior"] != "allow" {
				t.Errorf("allow 应答应带 behavior=allow: %v", inner)
			}
			if _, ok := inner["updatedInput"].(map[string]any); !ok {
				t.Errorf("allow 应答须带 updatedInput（旧版 CLI 缺失判校验错误）: %v", inner)
			}
		}
	}
	if !found {
		t.Errorf("stdin 未收到 req-allow-1 的 control_response: %v", lines)
	}

	// deny 应答
	sender <- controlRequestLine("req-deny-1", "Write", `{"path":"x"}`, "写文件")
	// 等审批请求注册（事件推送先于应答，stdin 行数才可能增长）
	waitFor(t, 5*time.Second, func() bool { return chatApprovalPending(svc, "req-deny-1") })
	if err := svc.RespondChatApproval("req-deny-1", false); err != nil {
		t.Fatalf("RespondChatApproval deny: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return len(fake.stdinSnapshot()) >= 3 })
	for _, line := range fake.stdinSnapshot()[2:] {
		var payload map[string]any
		if json.Unmarshal([]byte(line), &payload) != nil || payload["type"] != "control_response" {
			continue
		}
		resp := payload["response"].(map[string]any)
		if resp["request_id"] == "req-deny-1" {
			inner := resp["response"].(map[string]any)
			if inner["behavior"] != "deny" || inner["message"] == "" {
				t.Errorf("deny 应答形态异常: %v", inner)
			}
		}
	}

	// 不存在的请求报错
	if err := svc.RespondChatApproval("req-nonexistent", true); err == nil {
		t.Error("不存在的审批请求应报错")
	}

	svc.CloseAll()
	waitFor(t, 5*time.Second, func() bool {
		return svc.GetChatTaskState(taskID) == nil
	})
}

// TestChatService_Approval_AutoSettleOnTurnEnd 轮结束自动拒绝未决审批：result
// 收尾时该轮未决审批以 deny 回写（CLI 解除阻塞）+ chat-approval:cancelled 事件
// （前端撤弹窗），后续 RespondChatApproval 报错。
func TestChatService_Approval_AutoSettleOnTurnEnd(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "审批失效", "C:/proj")

	fake, sender := newFakeBlocking("")
	wireFakeFactory(svc, func() *fakeChatProcess { return fake })

	taskID, err := svc.RunChat(sess.ID, "长任务", "default", "")
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		st := svc.GetChatTaskState(taskID)
		return st != nil && st.Running
	})

	sender <- controlRequestLine("req-stale-1", "Bash", `{"command":"rm -rf x"}`, "危险操作")
	waitFor(t, 5*time.Second, func() bool { return chatApprovalPending(svc, "req-stale-1") })

	// 轮直接收尾（模拟 CLI 自行结束）：未决审批自动拒绝
	sender <- assistantLine("claude-s-1", "跳过该工具")
	sender <- resultLine("claude-s-1")
	sink.waitDone(t)

	// cancelled 事件 + deny 回写 stdin
	waitFor(t, 5*time.Second, func() bool {
		return sink.countEvents("chat-approval:cancelled") >= 1
	})
	waitFor(t, 5*time.Second, func() bool { return len(fake.stdinSnapshot()) >= 2 })
	var denyWritten bool
	for _, line := range fake.stdinSnapshot()[1:] {
		if strings.Contains(line, `"request_id":"req-stale-1"`) && strings.Contains(line, `"behavior":"deny"`) {
			denyWritten = true
		}
	}
	if !denyWritten {
		t.Errorf("未决审批应以 deny 回写 stdin: %v", fake.stdinSnapshot())
	}
	if err := svc.RespondChatApproval("req-stale-1", true); err == nil {
		t.Error("已失效审批应答应报错")
	}
	svc.CloseAll()
}

// TestChatService_TurnTimeoutWatchdog 轮超时：执行轮超过 chatTurnTimeout 无
// result，watchdog 发 interrupt 并标记超时（done 展示超时文案）。
func TestChatService_TurnTimeoutWatchdog(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	// 短时限注入走实例字段（不改全局 var：watchdog goroutine 与全局恢复存在竞态窗口）
	svc.scanInterval = 20 * time.Millisecond
	svc.turnTimeout = 50 * time.Millisecond
	sess, _ := svc.CreateChatSession("dir-1", "轮超时", "C:/proj")

	fake, sender := newFakeBlocking("")
	wireFakeFactory(svc, func() *fakeChatProcess { return fake })

	taskID, err := svc.RunChat(sess.ID, "慢任务", "default", "")
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		st := svc.GetChatTaskState(taskID)
		return st != nil && st.Running
	})

	// watchdog 发 interrupt（stdin 第 2 行）
	waitFor(t, 5*time.Second, func() bool { return len(fake.stdinSnapshot()) >= 2 })
	if line := fake.stdinSnapshot()[1]; !strings.Contains(line, `"subtype":"interrupt"`) {
		t.Errorf("watchdog 应发 interrupt: %q", line)
	}

	// CLI 以 error_during_execution 收尾 → done 带超时文案
	sender <- resultLineError("claude-s-1", "error_during_execution")
	result := sink.waitDone(t)
	if !strings.Contains(result.Error, "执行超时") {
		t.Errorf("done 应展示超时文案: %q", result.Error)
	}
	svc.CloseAll()
	waitFor(t, 5*time.Second, func() bool {
		return svc.GetChatTaskState(taskID) == nil
	})
	// 等 watchdog goroutine 全部退出（defer 恢复全局超时变量前，防 -race 竞态）
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.sessions) == 0
	})
}

// TestChatService_IdleProcessRecycle 空闲回收：无在途轮次超过 chatIdleTimeout，
// watchdog 强杀进程并摘除注册表；下次发消息 respawn（--resume 由会话元数据驱动）。
func TestChatService_IdleProcessRecycle(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	// 短时限注入走实例字段（不改全局 var：watchdog goroutine 与全局恢复存在竞态窗口）
	svc.scanInterval = 20 * time.Millisecond
	svc.idleTimeout = 60 * time.Millisecond
	sess, _ := svc.CreateChatSession("dir-1", "空闲回收", "C:/proj")

	turn := 0
	fakes := wireFakeFactory(svc, func() *fakeChatProcess {
		turn++
		if turn == 1 {
			// 第一次 spawn：回放一轮完整结果后进程保持存活（空闲，
			// 等待 watchdog 回收）
			return newFakeLinesThenIdle(assistantLine("claude-s-1", "答一"), resultLine("claude-s-1"))
		}
		return newFakeLines([]string{
			assistantLine("claude-s-2", "答二"),
			resultLine("claude-s-2"),
		})
	})

	if _, err := svc.RunChat(sess.ID, "第一问", "default", ""); err != nil {
		t.Fatalf("第一轮 RunChat: %v", err)
	}
	sink.waitDone(t) // 消费第一轮 done 信号（防错位污染后续 waitDone）

	// 等待空闲回收（无在途轮 + 超时限 → Kill → 注册表摘除）
	waitFor(t, 10*time.Second, func() bool {
		fake := (*fakes)[0]
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return fake.killCalled
	})
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.sessions) == 0
	})
	fake := (*fakes)[0]
	fake.mu.Lock()
	killed := fake.killCalled
	fake.mu.Unlock()
	if !killed {
		t.Error("空闲回收应 Kill 进程")
	}

	// 回收后再次发消息：respawn（--resume claude-s-1 由第一轮回写的元数据驱动）
	if _, err := svc.RunChat(sess.ID, "第二问", "default", ""); err != nil {
		t.Fatalf("回收后 respawn RunChat: %v", err)
	}
	sink.waitDone(t)
	waitFor(t, 5*time.Second, func() bool { return len(*fakes) >= 2 })
	f := (*fakes)[1]
	f.mu.Lock()
	args := strings.Join(f.gotArgs, " ")
	f.mu.Unlock()
	if !strings.Contains(args, "--resume claude-s-1") {
		t.Errorf("回收后 respawn 应带 --resume: %v", f.gotArgs)
	}
	svc.CloseAll()
	// 等 watchdog goroutine 全部退出（defer 恢复全局超时变量前，防 -race 竞态）
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.sessions) == 0
	})
}

// TestChatService_ListChatActiveTasks 在途轮次列表：排队+执行中按开始时间升序，
// 会话过滤生效；空会话返回空。
func TestChatService_ListChatActiveTasks(t *testing.T) {
	svc, _ := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "列表", "C:/proj")

	fake, _ := newFakeBlocking("")
	wireFakeFactory(svc, func() *fakeChatProcess { return fake })

	if _, err := svc.RunChat(sess.ID, "第一问", "default", ""); err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		st := svc.ListChatActiveTasks(sess.ID)
		return len(st) == 1 && st[0].Running
	})
	if _, err := svc.RunChat(sess.ID, "插话", "default", ""); err != nil {
		t.Fatalf("插话 RunChat: %v", err)
	}

	tasks := svc.ListChatActiveTasks(sess.ID)
	if len(tasks) != 2 {
		t.Fatalf("在途轮次数: got %d, want 2", len(tasks))
	}
	if !tasks[0].Running || !tasks[1].Queued {
		t.Errorf("轮次状态序异常: %+v", tasks)
	}
	if tasks[0].ChatSessionID != sess.ID {
		t.Errorf("会话过滤异常: %+v", tasks[0])
	}
	other := svc.ListChatActiveTasks("sess-other")
	if len(other) != 0 {
		t.Errorf("其他会话应为空列表, got %d", len(other))
	}
	svc.CloseAll()
	// 等 watchdog goroutine 全部退出（注册表摘除 = ctx 已取消，防 goroutine 泄漏）
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.sessions) == 0
	})
}

// ===== 多会话并行（PR2）=====

// TestChatService_MultiSessionParallel 两会话并行互不串流：各自常驻进程独立
// 推进，输出事件按 taskId 归属各轮，回复累积与落盘不交叉。
func TestChatService_MultiSessionParallel(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sessA, _ := svc.CreateChatSession("dir-a", "会话A", "C:/proj/a")
	sessB, _ := svc.CreateChatSession("dir-b", "会话B", "C:/proj/b")

	wireFakeFactory(svc, func() *fakeChatProcess {
		f, _ := newFakeBlocking("")
		return f
	})

	taskA, err := svc.RunChat(sessA.ID, "A 问", "default", "")
	if err != nil {
		t.Fatalf("A 会话 RunChat: %v", err)
	}
	taskB, err := svc.RunChat(sessB.ID, "B 问", "default", "")
	if err != nil {
		t.Fatalf("B 会话 RunChat: %v", err)
	}
	// 拿两个 fake 实例（spawn 后注册表内 proc，blocking fake 长驻不退出）
	svc.mu.Lock()
	procA := svc.sessions[sessA.ID].proc
	procB := svc.sessions[sessB.ID].proc
	svc.mu.Unlock()
	fakeA, okA := procA.(*fakeChatProcess)
	fakeB, okB := procB.(*fakeChatProcess)
	if !okA || !okB {
		t.Fatal("两会话进程应为 fake 实例")
	}

	// 交错推进：A 输出 → B 输出 → A result → B result（直接写 fake stdout 管道，
	// blocking fake 的回放 goroutine 阻塞在空 lineCh 上不触碰 writer）
	fakeA.mu.Lock()
	writerA := fakeA.writer
	fakeA.mu.Unlock()
	fakeB.mu.Lock()
	writerB := fakeB.writer
	fakeB.mu.Unlock()
	_, _ = io.WriteString(writerA, assistantLine("claude-a", "A 部分回复")+"\n")
	_, _ = io.WriteString(writerB, assistantLine("claude-b", "B 部分回复")+"\n")
	_, _ = io.WriteString(writerA, resultLine("claude-a")+"\n")
	_, _ = io.WriteString(writerB, resultLine("claude-b")+"\n")

	resultA := sink.waitDone(t)
	resultB := sink.waitDone(t)
	// done 序不定（两 pump 并发），按 taskId 归配断言互不串流
	byID := map[string]model.ChatTaskRunResult{resultA.TaskID: resultA, resultB.TaskID: resultB}
	if byID[taskA].Reply != "A 部分回复" || byID[taskA].ChatSessionID != sessA.ID {
		t.Errorf("A 轮结果串流: %+v", byID[taskA])
	}
	if byID[taskB].Reply != "B 部分回复" || byID[taskB].ChatSessionID != sessB.ID {
		t.Errorf("B 轮结果串流: %+v", byID[taskB])
	}
	// 落盘隔离：各会话消息文件只含本会话轮次
	gotA, _ := svc.GetChatSession(sessA.ID)
	gotB, _ := svc.GetChatSession(sessB.ID)
	if len(gotA.Messages) != 2 || gotA.Messages[1].Content != "A 部分回复" {
		t.Errorf("A 会话消息串流: %+v", gotA.Messages)
	}
	if len(gotB.Messages) != 2 || gotB.Messages[1].Content != "B 部分回复" {
		t.Errorf("B 会话消息串流: %+v", gotB.Messages)
	}
	if gotA.ClaudeSessionID != "claude-a" || gotB.ClaudeSessionID != "claude-b" {
		t.Errorf("claude session id 回写串流: %q / %q", gotA.ClaudeSessionID, gotB.ClaudeSessionID)
	}
	// 两进程均在册（并行共存）
	svc.mu.Lock()
	n := len(svc.sessions)
	svc.mu.Unlock()
	if n != 2 {
		t.Errorf("并行会话进程数: got %d, want 2", n)
	}
	svc.CloseAll()
	waitFor(t, 5*time.Second, func() bool {
		return svc.GetChatTaskState(taskA) == nil && svc.GetChatTaskState(taskB) == nil
	})
}

// TestChatService_DeleteSessionKillsProcess 删除会话终止其常驻进程：注册表
// 摘除、进程槽位归还（删除后可继续 spawn 到上限），其他会话不受影响。
func TestChatService_DeleteSessionKillsProcess(t *testing.T) {
	svc, _ := newChatSvcForTest(t)
	sessA, _ := svc.CreateChatSession("dir-a", "删除目标", "C:/a")
	sessB, _ := svc.CreateChatSession("dir-b", "共存会话", "C:/b")

	fakesA := wireFakeFactory(svc, func() *fakeChatProcess {
		return newFakeLinesThenIdle(assistantLine("claude-a", "答"), resultLine("claude-a"))
	})
	if _, err := svc.RunChat(sessA.ID, "问", "default", ""); err != nil {
		t.Fatalf("A RunChat: %v", err)
	}
	wireFakeFactory(svc, func() *fakeChatProcess {
		return newFakeLinesThenIdle(assistantLine("claude-b", "答B"), resultLine("claude-b"))
	})
	if _, err := svc.RunChat(sessB.ID, "问B", "default", ""); err != nil {
		t.Fatalf("B RunChat: %v", err)
	}
	// 两轮收尾（进程仍存活长驻，空闲回收未触发）
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.turns) == 0
	})

	if err := svc.DeleteChatSession(sessA.ID); err != nil {
		t.Fatalf("DeleteChatSession: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		_, alive := svc.sessions[sessA.ID]
		return !alive
	})
	(*fakesA)[0].mu.Lock()
	killed := (*fakesA)[0].killCalled
	(*fakesA)[0].mu.Unlock()
	if !killed {
		t.Error("删除会话应终止其常驻进程")
	}
	// B 不受影响
	svc.mu.Lock()
	_, bAlive := svc.sessions[sessB.ID]
	n := len(svc.sessions)
	svc.mu.Unlock()
	if !bAlive || n != 1 {
		t.Errorf("共存会话不应受删除影响: bAlive=%v n=%d", bAlive, n)
	}
	// 槽位已归还：可再起 3 个新会话进程（注册表现 1 + 新 3 = 4 恰满上限）
	for i := 0; i < 3; i++ {
		s, _ := svc.CreateChatSession(fmt.Sprintf("dir-n%d", i), "新增", "C:/n")
		wireFakeFactory(svc, func() *fakeChatProcess {
			return newFakeLinesThenIdle(
				assistantLine(fmt.Sprintf("claude-n%d", i), "答"),
				resultLine(fmt.Sprintf("claude-n%d", i)),
			)
		})
		if _, err := svc.RunChat(s.ID, "问", "default", ""); err != nil {
			t.Fatalf("槽位归还后第 %d 个新会话失败: %v", i+1, err)
		}
	}
	svc.CloseAll()
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.sessions) == 0
	})
}

// TestChatService_RunChat_ScanErrorSurfaced B9：输出流读取中断（读错误）时
// 回复截断必须透传 done.Error，不得静默终止。
func TestChatService_RunChat_ScanErrorSurfaced(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "读中断", "C:/proj")

	fake := newFakeLines([]string{assistantLine("claude-s-1", "半截回复")})
	fake.pipeErr = errors.New("pipe boom")
	wireFakeFactory(svc, func() *fakeChatProcess { return fake })

	taskID, err := svc.RunChat(sess.ID, "hi", "default", "")
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	result := sink.waitDone(t)
	if !strings.Contains(result.Error, "输出流读取中断") || !strings.Contains(result.Error, "pipe boom") {
		t.Errorf("读取中断错误应透传 done.Error: %q", result.Error)
	}
	if !strings.Contains(result.Reply, "半截回复") {
		t.Errorf("中断前已读到的回复不应丢失: %q", result.Reply)
	}
	if result.TaskID != taskID {
		t.Errorf("taskID 不符: %q", result.TaskID)
	}
	waitFor(t, 5*time.Second, func() bool {
		return svc.GetChatTaskState(taskID) == nil
	})
}

// TestChatService_RunChat_ClaudeErrorSurfaced B10：result 事件 is_error 时
// claude 侧错误文本透传 done.Error（用户看到具体原因而非裸 exit status）。
func TestChatService_RunChat_ClaudeErrorSurfaced(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "claude 失败", "C:/proj")

	lines := []string{
		assistantLine("claude-s-1", "部分输出"),
		`{"type":"result","subtype":"error_during_execution","is_error":true,"duration_ms":50,"num_turns":1,"total_cost_usd":0,"session_id":"claude-s-1","result":"claude 内部错误：配额耗尽"}`,
	}
	wireFakeFactory(svc, func() *fakeChatProcess { return newFakeLines(lines) })

	if _, err := svc.RunChat(sess.ID, "hi", "default", ""); err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	result := sink.waitDone(t)
	if !strings.Contains(result.Error, "claude 内部错误：配额耗尽") {
		t.Errorf("claude 侧错误文本应透传 done.Error: %q", result.Error)
	}
	waitFor(t, 5*time.Second, func() bool {
		return svc.GetChatTaskState(result.TaskID) == nil
	})
}

// TestChatService_SettingsConcurrentAccess B11：GetChatSettings/SaveChatSettings
// 持 s.mu，并发读写无数据竞争（须配合 go test -race 验证）。
func TestChatService_SettingsConcurrentAccess(t *testing.T) {
	svc, _ := newChatSvcForTest(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = svc.SaveChatSettings(model.ChatPermissionModePlan, "sonnet")
		}()
		go func() {
			defer wg.Done()
			_ = svc.GetChatSettings()
		}()
	}
	wg.Wait()
	got := svc.GetChatSettings()
	if got.PermissionMode != model.ChatPermissionModePlan || got.ModelName != "sonnet" {
		t.Errorf("并发读写后配置应一致: %+v", got)
	}
}

// TestChatDirectoryKey B13：去重键小写 + 分隔符归一（跨平台纯函数断言）。
// 仅覆盖 POSIX 路径语义（Linux 上 \ 为合法文件名字符，不参与归一）；
// Windows 路径语义用例见 TestChatDirectoryKey_WindowsPaths。
func TestChatDirectoryKey(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		same bool
	}{
		{"POSIX 大小写归一", "/tmp/work/x", "/TMP/WORK/X", true},
		{"不同路径", "/tmp/work/a", "/tmp/work/b", false},
	}
	for _, tc := range cases {
		if got := chatDirectoryKey(tc.a) == chatDirectoryKey(tc.b); got != tc.same {
			t.Errorf("%s: chatDirectoryKey(%q)==chatDirectoryKey(%q) = %v, want %v", tc.name, tc.a, tc.b, got, tc.same)
		}
	}
}

// TestChatDirectoryKey_WindowsPaths Windows 路径语义：盘符大小写归一、
// 反斜杠与正斜杠等价（依赖 Windows 分隔符语义，非 Windows 平台跳过）。
func TestChatDirectoryKey_WindowsPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows 路径分隔符语义（\\ 不参与 Linux 归一），非 Windows 平台跳过")
	}
	cases := []struct {
		name string
		a, b string
		same bool
	}{
		{"同路径不同盘符大小写", `C:\Proj\A`, "c:/proj/a", true},
		{"不同路径", `C:\Proj\A`, `C:\Proj\B`, false},
		{"反斜杠与正斜杠等价", `C:\a\b`, "C:/a/b", true},
	}
	for _, tc := range cases {
		if got := chatDirectoryKey(tc.a) == chatDirectoryKey(tc.b); got != tc.same {
			t.Errorf("%s: chatDirectoryKey(%q)==chatDirectoryKey(%q) = %v, want %v", tc.name, tc.a, tc.b, got, tc.same)
		}
	}
}

// TestChatService_AddChatDirectory_CaseInsensitiveDedup B13 端到端：同路径不同
// 大小写命中既有项，不产生重复目录项。
func TestChatService_AddChatDirectory_CaseInsensitiveDedup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("大小写不敏感文件系统仅 Windows 可构造同目录不同大小写的真实路径")
	}
	svc, _ := newChatSvcForTest(t)
	dir := t.TempDir()
	first, err := svc.AddChatDirectory(dir, "首个")
	if err != nil {
		t.Fatalf("AddChatDirectory: %v", err)
	}
	dup, err := svc.AddChatDirectory(strings.ToUpper(dir), "重复")
	if err != nil {
		t.Fatalf("大小写变体添加: %v", err)
	}
	if dup.ID != first.ID {
		t.Errorf("大小写变体应返回既有项: got %q, want %q", dup.ID, first.ID)
	}
	list, _ := svc.ListChatDirectories()
	if len(list) != 1 {
		t.Errorf("同路径不同大小写不应产生重复项, got %d", len(list))
	}
}

// TestSaveChatJSON_AtomicSequence B4：原子写序列——temp 写入 → 上一好版本备份
// .bak → rename 替换；目标始终为完整内容，temp 无残留。
func TestSaveChatJSON_AtomicSequence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "file.json")

	type payload struct {
		N int `json:"n"`
	}
	if err := saveChatJSON(path, payload{N: 1}); err != nil {
		t.Fatalf("首次写入: %v", err)
	}
	raw1, _ := os.ReadFile(path)
	if !strings.Contains(string(raw1), "\"n\": 1") {
		t.Errorf("首次内容不符: %s", raw1)
	}

	// 二次写入：.bak 保留上一好版本，目标更新为新内容
	if err := saveChatJSON(path, payload{N: 2}); err != nil {
		t.Fatalf("覆写: %v", err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("应生成上一好版本备份 .bak: %v", err)
	}
	if !strings.Contains(string(bak), "\"n\": 1") {
		t.Errorf(".bak 应为上一好版本: %s", bak)
	}
	raw2, _ := os.ReadFile(path)
	if !strings.Contains(string(raw2), "\"n\": 2") {
		t.Errorf("覆写后内容不符: %s", raw2)
	}

	// temp 文件不残留
	matches, _ := filepath.Glob(filepath.Join(dir, "nested", ".*.tmp-*"))
	if len(matches) != 0 {
		t.Errorf("temp 文件残留: %v", matches)
	}
}

// TestNewExecChatProcess_CancelKillsTreeWired B7：生产进程须设置 cmd.Cancel
// （ctx 超时走整树杀而非默认 Kill 根进程留孤儿），且对未启动进程调用安全。
func TestNewExecChatProcess_CancelKillsTreeWired(t *testing.T) {
	p := newExecChatProcess(context.Background(), "claude", []string{"-p", "hi"}, ".")
	ep, ok := p.(*execChatProcess)
	if !ok {
		t.Fatalf("生产工厂应返回 *execChatProcess, got %T", p)
	}
	if ep.cmd.Cancel == nil {
		t.Fatal("生产进程须设置 cmd.Cancel：ctx 超时走整树杀（CommandContext 默认仅杀根进程）")
	}
	// Cancel 闭包对未启动进程（Process 为 nil）调用须安全无错
	if err := ep.cmd.Cancel(); err != nil {
		t.Errorf("未启动进程的 Cancel 调用应安全: %v", err)
	}
}
