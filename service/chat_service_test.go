package service

import (
	"context"
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
	done   chan struct{} // 每次 chat-task:done 发信号（容量 1，非阻塞发送）
}

type capturedEvent struct {
	name string
	data []any
}

func newCapturedSink() *capturedSink {
	return &capturedSink{done: make(chan struct{}, 8)}
}

func (c *capturedSink) Emit(name string, data ...any) {
	c.mu.Lock()
	c.events = append(c.events, capturedEvent{name: name, data: data})
	c.mu.Unlock()
	if name == "chat-task:done" {
		select {
		case c.done <- struct{}{}:
		default:
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

// waitDone 等待一次 chat-task:done 事件并返回 payload（超时 fail）。
func (c *capturedSink) waitDone(t *testing.T) model.ChatTaskRunResult {
	t.Helper()
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
		t.Fatal("等待 chat-task:done 超时")
	}
	return c.last(t, "chat-task:done").(model.ChatTaskRunResult)
}

// fakeChatProcess chatProcess 的 fake 实现：按预设行回放 stream-json 输出。
// lineCh 由调用方构造（正常流：带缓冲且已关闭；取消流：不关闭，靠 Kill 退出）。
type fakeChatProcess struct {
	lineCh     <-chan string
	startErr   error
	waitErr    error
	pipeGate   chan struct{} // 非 nil 时 StdoutPipe 阻塞直至关闭（复现 queued 翻转后到 proc 赋值前的启动窗口）
	pipeErr    error         // 非 nil 时输出流末尾以该错误关闭（scanner.Err 透传路径）
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
}

// newFakeLines 构造「立即回放全部行」的 fake。
func newFakeLines(lines []string) *fakeChatProcess {
	ch := make(chan string, len(lines))
	for _, l := range lines {
		ch <- l
	}
	close(ch)
	return &fakeChatProcess{lineCh: ch, exitCode: -1, killCh: make(chan struct{})}
}

// newFakeBlocking 构造「输出后挂起等 Kill」的 fake，返回可继续发送行的发送端。
func newFakeBlocking(firstLine string) (*fakeChatProcess, chan<- string) {
	ch := make(chan string, 8)
	if firstLine != "" {
		ch <- firstLine
	}
	return &fakeChatProcess{lineCh: ch, exitCode: -1, killCh: make(chan struct{})}, ch
}

func (f *fakeChatProcess) StdoutPipe() (io.ReadCloser, error) {
	if f.pipeGate != nil {
		<-f.pipeGate
	}
	r, w := io.Pipe()
	f.writer = w
	return r, nil
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
	return f.waitErr
}

func (f *fakeChatProcess) Kill() {
	f.mu.Lock()
	f.killCalled = true
	f.mu.Unlock()
	f.killOnce.Do(func() { close(f.killCh) })
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

// TestBuildChatArgs 表驱动：resume/permission-mode/model 的追加规则。
func TestBuildChatArgs(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		resume string
		mode   string
		mdl    string
		want   []string
	}{
		{
			name: "首轮默认模式", prompt: "hi", resume: "", mode: "default", mdl: "",
			want: []string{"-p", "hi", "--output-format", "stream-json", "--verbose"},
		},
		{
			name: "续会话全参数", prompt: "hi", resume: "s1", mode: "acceptEdits", mdl: "sonnet",
			want: []string{"-p", "hi", "--resume", "s1", "--output-format", "stream-json", "--verbose", "--permission-mode", "acceptEdits", "--model", "sonnet"},
		},
		{
			name: "权限模式为空不追加", prompt: "hi", resume: "", mode: "", mdl: "opus",
			want: []string{"-p", "hi", "--output-format", "stream-json", "--verbose", "--model", "opus"},
		},
		{
			name: "plan 模式", prompt: "hi", resume: "", mode: "plan", mdl: "",
			want: []string{"-p", "hi", "--output-format", "stream-json", "--verbose", "--permission-mode", "plan"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildChatArgs(tc.prompt, tc.resume, tc.mode, tc.mdl)
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("buildChatArgs:\n got  %v\n want %v", got, tc.want)
			}
		})
	}
}

// ===== RunChat 全链路（fake 进程注入，不真调 claude）=====

// TestChatService_RunChat_FullFlow 首轮对话全链路：事件序列、参数组装、
// user/assistant 消息入档、claude session id 回写会话。
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

	// 子进程参数与工作目录
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
	if !strings.Contains(gotArgs, "-p 你好") {
		t.Errorf("args 缺 prompt: %v", args)
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
	if !strings.Contains(gotArgs, "--output-format stream-json --verbose") {
		t.Errorf("args 缺 stream-json: %v", args)
	}

	// 消息入档：user + assistant 各一条，taskID 关联
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

	// claude session id 回写会话元数据（下一轮 --resume 依据）
	if got.ClaudeSessionID != "claude-s-1" {
		t.Errorf("会话 ClaudeSessionID 未回写: %q", got.ClaudeSessionID)
	}

	// 终态任务已从任务表摘除：GetChatTaskState 返回 nil（前端
	// restoreChatTaskState 对「查无此任务」按终态清态处理，恢复链路不破坏）
	if st := svc.GetChatTaskState(taskID); st != nil {
		t.Errorf("终态任务应已摘除, got %+v", st)
	}
}

// TestChatService_RunChat_ResumeSecondTurn 第二轮对话自动 --resume 续上一轮 claude 会话。
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
	if _, err := svc.RunChat(sess.ID, "第二问", "default", ""); err != nil {
		t.Fatalf("第二轮 RunChat: %v", err)
	}
	result := sink.waitDone(t)

	// 第二轮参数带 --resume claude-s-1（第一轮回写的会话 id）
	f := (*fakes)[1]
	f.mu.Lock()
	args := strings.Join(f.gotArgs, " ")
	f.mu.Unlock()
	if !strings.Contains(args, "--resume claude-s-1") {
		t.Errorf("第二轮应带 --resume: %v", f.gotArgs)
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

// TestChatService_RunChat_InProgress 同会话串行保护：进行中再发一轮被拒绝。
func TestChatService_RunChat_InProgress(t *testing.T) {
	svc, _ := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "串行", "C:/proj")

	fake, _ := newFakeBlocking(assistantLine("claude-s-1", "部分"))
	wireFakeFactory(svc, func() *fakeChatProcess { return fake })

	taskID, err := svc.RunChat(sess.ID, "第一问", "default", "")
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		st := svc.GetChatTaskState(taskID)
		return st != nil && st.Running
	})

	_, err = svc.RunChat(sess.ID, "第二问", "default", "")
	if code := appErrCode(t, err); code != model.ErrCodeChatInProgress {
		t.Errorf("同会话并发轮次应拒绝, got %v", err)
	}

	// 收尾取消并等待任务终止（终态任务被摘除，查无此任务即收尾完成）
	svc.CancelChatTask(taskID)
	waitFor(t, 5*time.Second, func() bool {
		return svc.GetChatTaskState(taskID) == nil
	})
	svc.CloseAll()
}

// TestChatService_CancelChatTask_Running 运行中取消：杀进程、done canceled、部分回复入档。
func TestChatService_CancelChatTask_Running(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "取消", "C:/proj")

	fake, _ := newFakeBlocking(assistantLine("claude-s-1", "部分回复"))
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
	// 幂等：重复取消不 panic、不重复 emit done（任务或仍在表返回 true，
	// 或已被 pump 摘除返回 false，两者皆合法；done 恒一次由尾部断言保证）
	_ = svc.CancelChatTask(taskID)
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
	if !killed {
		t.Error("运行中取消应调用 Kill")
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
}

// TestChatService_CancelChatTask_Queued 排队中取消：占满槽位后取消第 4 个任务。
func TestChatService_CancelChatTask_Queued(t *testing.T) {
	svc, sink := newChatSvcForTest(t)

	// 三个不同会话占满并发槽位（blocking fake 挂起）
	type runningTask struct {
		taskID string
		fake   *fakeChatProcess
	}
	var running []runningTask
	for i := 0; i < chatMaxConcurrent; i++ {
		sess, _ := svc.CreateChatSession(fmt.Sprintf("dir-%d", i), "占位", "C:/p")
		fake, _ := newFakeBlocking("")
		wireFakeFactory(svc, func() *fakeChatProcess { return fake })
		taskID, err := svc.RunChat(sess.ID, "占用", "default", "")
		if err != nil {
			t.Fatalf("占位 RunChat: %v", err)
		}
		running = append(running, runningTask{taskID, fake})
	}
	waitFor(t, 5*time.Second, func() bool {
		for _, rt := range running {
			st := svc.GetChatTaskState(rt.taskID)
			if st == nil || !st.Running {
				return false
			}
		}
		return true
	})

	// 第 4 个任务（新会话）进入排队
	qsess, _ := svc.CreateChatSession("dir-q", "排队", "C:/p")
	fake, _ := newFakeBlocking("")
	wireFakeFactory(svc, func() *fakeChatProcess { return fake })
	type runResult struct {
		taskID string
		err    error
	}
	resCh := make(chan runResult, 1)
	go func() {
		taskID, err := svc.RunChat(qsess.ID, "排队任务", "default", "")
		resCh <- runResult{taskID, err}
	}()

	// 从 queued 事件取第 4 个任务 id（前 3 个为占位任务）后取消
	var queuedTaskID string
	waitFor(t, 5*time.Second, func() bool {
		ids := sink.queuedIDs()
		if len(ids) >= chatMaxConcurrent+1 {
			queuedTaskID = ids[chatMaxConcurrent]
			return true
		}
		return false
	})
	if !svc.CancelChatTask(queuedTaskID) {
		t.Fatal("排队取消应返回 true")
	}
	// 幂等：重复取消已排队任务不 double-close queueCancel（修复前此处 panic）；
	// 任务或仍在表（幂等 true）或已被 RunChat 清理（false），两者皆合法
	_ = svc.CancelChatTask(queuedTaskID)

	select {
	case res := <-resCh:
		if res.err == nil {
			t.Error("排队取消后 RunChat 应返回错误")
		}
		if res.taskID != "" {
			t.Errorf("排队取消不返回任务 id, got %q", res.taskID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("排队取消后 RunChat 未返回")
	}

	// done 事件带 canceled
	waitFor(t, 5*time.Second, func() bool {
		return strings.Contains(strings.Join(sink.names(), ","), "chat-task:done")
	})

	// 收尾：取消占位任务并等待全部终止（终态任务被摘除，防 pump goroutine 泄漏）
	for _, rt := range running {
		svc.CancelChatTask(rt.taskID)
	}
	waitFor(t, 5*time.Second, func() bool {
		for _, rt := range running {
			if svc.GetChatTaskState(rt.taskID) != nil {
				return false
			}
		}
		return true
	})
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
// 槽位归还、失败任务不残留任务表。
func TestChatService_RunChat_StartFailure(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "启动失败", "C:/proj")

	wireFakeFactory(svc, func() *fakeChatProcess {
		return &fakeChatProcess{startErr: errors.New("exec: not found"), exitCode: -1, killCh: make(chan struct{})}
	})

	if _, err := svc.RunChat(sess.ID, "hi", "default", ""); err == nil {
		t.Fatal("启动失败应返回错误")
	}

	// 失败任务不残留任务表（终态/失败任务均摘除，防内存无界增长）
	svc.mu.Lock()
	residue := len(svc.tasks)
	svc.mu.Unlock()
	if residue != 0 {
		t.Errorf("启动失败后任务表应无残留, got %d", residue)
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
		return len(svc.tasks) == 0
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

// TestChatService_RunChat_CancelDuringStartupWindow B1+B3：复现 queued 翻转后
// 到 proc 赋值前的启动窗口——取消不 panic（proc nil 守卫）、同会话第二轮被拒绝
// （串行保护无双 false 绕过窗口）、进程启动成功后取消信号补杀。
func TestChatService_RunChat_CancelDuringStartupWindow(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "启动窗口", "C:/proj")

	fake := newFakeLines([]string{assistantLine("claude-s-1", "窗口回复"), resultLine("claude-s-1")})
	fake.pipeGate = make(chan struct{})
	wireFakeFactory(svc, func() *fakeChatProcess { return fake })

	type runResult struct {
		taskID string
		err    error
	}
	resCh := make(chan runResult, 1)
	go func() {
		taskID, err := svc.RunChat(sess.ID, "窗口问题", "default", "")
		resCh <- runResult{taskID, err}
	}()

	// 从 queued 事件拿 taskID（此时任务已注册）
	var taskID string
	waitFor(t, 5*time.Second, func() bool {
		ids := sink.queuedIDs()
		if len(ids) > 0 {
			taskID = ids[0]
			return true
		}
		return false
	})
	// started 事件已发出 = queued 已翻转（fake 阻塞在 StdoutPipe，proc 尚未赋值）
	waitFor(t, 5*time.Second, func() bool {
		return strings.Contains(strings.Join(sink.names(), ","), "chat-task:started")
	})

	// B3：启动窗口内同会话第二轮必须被拒绝（修复前 running/queued 双 false 放行，
	// 两 claude 并行同会话破坏 --resume 上下文）
	if _, err := svc.RunChat(sess.ID, "插队", "default", ""); appErrCode(t, err) != model.ErrCodeChatInProgress {
		t.Errorf("启动窗口内同会话第二轮应拒绝, got %v", err)
	}

	// B1：启动窗口内取消（修复前 task.proc 为 nil 直接 Kill → panic）
	if !svc.CancelChatTask(taskID) {
		t.Fatal("启动窗口内取消应返回 true")
	}

	// 放行启动流程：进程启动成功，lateCanceled 检查补杀
	close(fake.pipeGate)
	select {
	case res := <-resCh:
		if res.err != nil {
			t.Fatalf("取消发生在启动窗口，RunChat 仍应正常返回 taskId: %v", res.err)
		}
		if res.taskID != taskID {
			t.Errorf("taskID 不符: got %q, want %q", res.taskID, taskID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunChat 未返回")
	}
	result := sink.waitDone(t)
	if !result.Canceled {
		t.Errorf("启动窗口取消轮 done 应标记 canceled: %+v", result)
	}
	waitFor(t, 5*time.Second, func() bool {
		return svc.GetChatTaskState(taskID) == nil
	})
	svc.CloseAll()
}

// TestChatService_RunChat_CancelTimerReleased B6：任务终结后执行 ctx 已取消，
// 超时定时器不悬挂至 10 分钟自然到期。
func TestChatService_RunChat_CancelTimerReleased(t *testing.T) {
	svc, sink := newChatSvcForTest(t)
	sess, _ := svc.CreateChatSession("dir-1", "定时器", "C:/proj")
	wireFakeFactory(svc, func() *fakeChatProcess {
		return newFakeLines([]string{assistantLine("claude-s-1", "ok"), resultLine("claude-s-1")})
	})

	taskID, err := svc.RunChat(sess.ID, "hi", "default", "")
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	sink.waitDone(t)
	// done 后任务或已摘除、或处于摘除前窗口；两者都蕴含 cancelFn 已调用
	// （摘除路径必经 cancelFn 调用点）
	waitFor(t, 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		task := svc.tasks[taskID]
		if task == nil {
			return true
		}
		return task.ctx != nil && task.ctx.Err() != nil
	})
}

// TestChatService_RunChat_QueuedTimeoutCtxNotStarted B12：占满并发槽位使任务
// 排队，排队期执行 ctx（超时定时器）未创建——排队等待不侵蚀超时预算。
func TestChatService_RunChat_QueuedTimeoutCtxNotStarted(t *testing.T) {
	svc, sink := newChatSvcForTest(t)

	// 不同会话占满并发槽位（blocking fake 挂起）
	type runningTask struct {
		taskID string
		fake   *fakeChatProcess
	}
	var running []runningTask
	for i := 0; i < chatMaxConcurrent; i++ {
		sess, _ := svc.CreateChatSession(fmt.Sprintf("dir-%d", i), "占位", "C:/p")
		fake, _ := newFakeBlocking("")
		wireFakeFactory(svc, func() *fakeChatProcess { return fake })
		taskID, err := svc.RunChat(sess.ID, "占用", "default", "")
		if err != nil {
			t.Fatalf("占位 RunChat: %v", err)
		}
		running = append(running, runningTask{taskID, fake})
	}
	waitFor(t, 5*time.Second, func() bool {
		for _, rt := range running {
			st := svc.GetChatTaskState(rt.taskID)
			if st == nil || !st.Running {
				return false
			}
		}
		return true
	})

	// 第 4 个任务（新会话）进入排队
	qsess, _ := svc.CreateChatSession("dir-q", "排队", "C:/p")
	wireFakeFactory(svc, func() *fakeChatProcess {
		f, _ := newFakeBlocking("")
		return f
	})
	type runResult struct {
		taskID string
		err    error
	}
	resCh := make(chan runResult, 1)
	go func() {
		taskID, err := svc.RunChat(qsess.ID, "排队任务", "default", "")
		resCh <- runResult{taskID, err}
	}()
	var queuedID string
	waitFor(t, 5*time.Second, func() bool {
		ids := sink.queuedIDs()
		if len(ids) >= chatMaxConcurrent+1 {
			queuedID = ids[chatMaxConcurrent]
			return true
		}
		return false
	})

	// 排队态确认 + 执行 ctx 未创建（WithTimeout 在获取槽位后才调用）
	st := svc.GetChatTaskState(queuedID)
	if st == nil || !st.Queued {
		t.Fatalf("任务应处于排队态: %+v", st)
	}
	svc.mu.Lock()
	ctxCreated := svc.tasks[queuedID] != nil && svc.tasks[queuedID].ctx != nil
	svc.mu.Unlock()
	if ctxCreated {
		t.Error("排队期执行 ctx 不应创建（超时预算不被排队侵蚀）")
	}

	// 收尾：取消排队任务与占位任务并等待终态摘除（防 goroutine 泄漏）
	svc.CancelChatTask(queuedID)
	select {
	case res := <-resCh:
		if res.err == nil {
			t.Error("排队取消后 RunChat 应返回错误")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("排队取消后 RunChat 未返回")
	}
	for _, rt := range running {
		svc.CancelChatTask(rt.taskID)
	}
	waitFor(t, 5*time.Second, func() bool {
		if svc.GetChatTaskState(queuedID) != nil {
			return false
		}
		for _, rt := range running {
			if svc.GetChatTaskState(rt.taskID) != nil {
				return false
			}
		}
		return true
	})
	svc.CloseAll()
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

// TestChatDirectoryKey B13：去重键小写 + 正斜杠归一（跨平台纯函数断言）。
func TestChatDirectoryKey(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		same bool
	}{
		{"同路径不同盘符大小写", `C:\Proj\A`, "c:/proj/a", true},
		{"不同路径", `C:\Proj\A`, `C:\Proj\B`, false},
		{"POSIX 大小写归一", "/tmp/work/x", "/TMP/WORK/X", true},
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
