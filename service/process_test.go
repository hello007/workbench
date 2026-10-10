package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"workbench/model"
)

// fakeProcessProvider 进程能力 fake：全部断言走注入数据，不依赖真实进程/端口/时间
// （docs/spec/test-stability.md：本机常驻 workbench.exe 监听 36115，真实枚举必然 flaky）。
type fakeProcessProvider struct {
	processes []model.ProcessInfo
	enumErr   error
	selfPIDs  map[int]bool

	stopDelivered bool // RequestProcessStop 是否送达优雅请求
	stopErr       error
	killErr       error

	aliveSeq   []bool // IsProcessAlive 按序返回，耗尽后保持最后一个值
	aliveCalls int
	stopCalls  []int
	killCalls  []fakeKillCall
}

type fakeKillCall struct {
	pid      int
	killTree bool
}

func (f *fakeProcessProvider) ListProcesses(ctx context.Context) ([]model.ProcessInfo, error) {
	return f.processes, f.enumErr
}

func (f *fakeProcessProvider) IsSelfProcess(pid int) bool {
	return f.selfPIDs[pid]
}

func (f *fakeProcessProvider) RequestProcessStop(pid int, killTree bool) (bool, error) {
	f.stopCalls = append(f.stopCalls, pid)
	return f.stopDelivered, f.stopErr
}

func (f *fakeProcessProvider) KillProcess(pid int, killTree bool) error {
	f.killCalls = append(f.killCalls, fakeKillCall{pid: pid, killTree: killTree})
	return f.killErr
}

func (f *fakeProcessProvider) IsProcessAlive(pid int) bool {
	idx := f.aliveCalls
	f.aliveCalls++
	if len(f.aliveSeq) == 0 {
		return true // 缺省：恒存活，用于驱动超时分支
	}
	if idx >= len(f.aliveSeq) {
		return f.aliveSeq[len(f.aliveSeq)-1]
	}
	return f.aliveSeq[idx]
}

// newTestProcessService 构造注入 fake 的服务：假时钟每次推进 500ms + 空 sleep，
// 使 5s 优雅等待在 10 次探测内确定结束，不真实等待（禁 sleep 等待真实时间）。
func newTestProcessService(p *fakeProcessProvider) *ProcessService {
	s := NewProcessService()
	s.provider = p
	var tick time.Duration
	s.now = func() time.Time {
		tick += 500 * time.Millisecond
		return time.Unix(0, 0).Add(tick)
	}
	s.sleep = func(time.Duration) {}
	return s
}

// appErrorCode 提取 AppError 错误码，便于断言错误分流。
func appErrorCode(t *testing.T, err error) string {
	t.Helper()
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("期望 *model.AppError，实际 %T: %v", err, err)
	}
	return appErr.Code
}

func sampleProcessInfos() []model.ProcessInfo {
	return []model.ProcessInfo{
		{Pid: 100, Name: "node.exe", Ports: []model.ProcessPort{
			{Port: 36115, Protocol: "TCP", State: "LISTENING"},
			{Port: 50000, Protocol: "TCP", State: "ESTABLISHED"},
		}, MemoryKB: 1024},
		{Pid: 200, Name: "java.exe", Ports: []model.ProcessPort{
			{Port: 8080, Protocol: "TCP", State: "LISTEN"},
		}, MemoryKB: 2048},
		{Pid: 300, Name: "notepad.exe"}, // 无端口进程：进程名检索须命中
		{Pid: 400, Name: "svchost.exe", Ports: []model.ProcessPort{
			{Port: 443, Protocol: "UDP", State: ""},
		}, IsSystem: true},
	}
}

// ===== 过滤三态 =====

func TestFilterProcesses_PortMode(t *testing.T) {
	all := sampleProcessInfos()

	got := filterProcesses(all, "36115", ProcessFilterModeAuto)
	if len(got) != 1 || got[0].Pid != 100 {
		t.Fatalf("auto 纯数字应按端口匹配: got %+v", got)
	}

	got = filterProcesses(all, "8080", ProcessFilterModePort)
	if len(got) != 1 || got[0].Pid != 200 {
		t.Fatalf("强制端口模式应命中 8080: got %+v", got)
	}

	if got := filterProcesses(all, "99999", ProcessFilterModeAuto); len(got) != 0 {
		t.Errorf("无进程占用该端口应返回空: got %+v", got)
	}
}

func TestFilterProcesses_NameMode(t *testing.T) {
	all := sampleProcessInfos()

	got := filterProcesses(all, "node", ProcessFilterModeAuto)
	if len(got) != 1 || got[0].Pid != 100 {
		t.Fatalf("含非数字应按进程名匹配: got %+v", got)
	}

	// 大小写不敏感
	if got := filterProcesses(all, "NODE", ProcessFilterModeName); len(got) != 1 || got[0].Pid != 100 {
		t.Errorf("进程名匹配应大小写不敏感: got %+v", got)
	}

	// 无端口进程在进程名检索下须命中（util 基线含不监听端口的进程）
	if got := filterProcesses(all, "notepad", ProcessFilterModeAuto); len(got) != 1 || got[0].Pid != 300 {
		t.Errorf("无端口进程应被进程名检索命中: got %+v", got)
	}

	// 强制进程名模式时纯数字也按名字匹配（消除「进程名恰为纯数字」歧义）
	numeric := []model.ProcessInfo{{Pid: 1, Name: "360"}}
	if got := filterProcesses(numeric, "360", ProcessFilterModeName); len(got) != 1 {
		t.Errorf("强制进程名模式下纯数字应按名字匹配: got %+v", got)
	}
}

func TestFilterProcesses_EmptyKeywordListsListening(t *testing.T) {
	all := sampleProcessInfos()

	got := filterProcesses(all, "", ProcessFilterModeAuto)
	if len(got) != 3 {
		t.Fatalf("空输入应保留监听态与状态无法识别的进程: got %d 条 %+v", len(got), got)
	}
	if got[0].Pid != 100 || got[1].Pid != 200 || got[2].Pid != 400 {
		t.Fatalf("应命中 100(LISTENING)、200(LISTEN) 与 400(UDP 空状态): got %+v", got)
	}
	// 端口列表裁剪为仅监听端口，避免 ESTABLISHED 淹没默认视图
	if len(got[0].Ports) != 1 || got[0].Ports[0].Port != 36115 {
		t.Errorf("端口应裁剪为仅监听端口: got %+v", got[0].Ports)
	}
	// 裁剪须写新切片，不得改坏入参底层数组
	if len(all[0].Ports) != 2 {
		t.Errorf("过滤不得修改入参的端口切片: got %+v", all[0].Ports)
	}
}

// TestFilterProcesses_EmptyKeywordKeepsUnknownState 防御式判定：状态无法识别时保守保留。
//
// 若把无法识别的状态一律当「非监听」排除，Windows 中文系统（netstat 状态列本地化）
// 的默认视图会整体退化为空列表——这是本次防御要消除的严重降级。
func TestFilterProcesses_EmptyKeywordKeepsUnknownState(t *testing.T) {
	all := []model.ProcessInfo{
		// 已知非监听态：排除
		{Pid: 1, Name: "known.exe", Ports: []model.ProcessPort{{Port: 50000, Protocol: "TCP", State: "ESTABLISHED"}}},
		// Windows netstat 的 UDP 行无 State 列（空串）：无法识别 → 保留且不裁剪
		{Pid: 2, Name: "udp.exe", Ports: []model.ProcessPort{{Port: 53, Protocol: "UDP"}}},
		// 本地化状态名（中文系统「监听」）：无法识别 → 保留且不裁剪
		{Pid: 3, Name: "localized.exe", Ports: []model.ProcessPort{{Port: 8080, Protocol: "TCP", State: "监听"}}},
		// 无端口进程：默认视图排除（只在进程名检索下命中）
		{Pid: 4, Name: "noport.exe"},
	}
	got := filterProcesses(all, "", ProcessFilterModeAuto)
	if len(got) != 2 || got[0].Pid != 2 || got[1].Pid != 3 {
		t.Fatalf("应仅保留状态无法识别的两条: got %+v", got)
	}
	if len(got[0].Ports) != 1 || len(got[1].Ports) != 1 {
		t.Errorf("状态无法识别时不得裁剪端口（裁成空列表即丢进程）: got %+v", got)
	}
}

// TestFilterProcesses_EmptyKeywordKeepsListeningWhenMixed 同一进程既有监听端口又有
// 未知状态端口时：按监听端口裁剪（监听态已可确认，无需保守保留全部）。
func TestFilterProcesses_EmptyKeywordKeepsListeningWhenMixed(t *testing.T) {
	all := []model.ProcessInfo{{Pid: 7, Name: "mixed.exe", Ports: []model.ProcessPort{
		{Port: 8080, Protocol: "TCP", State: "LISTENING"},
		{Port: 5353, Protocol: "UDP"},
		{Port: 50000, Protocol: "TCP", State: "ESTABLISHED"},
	}}}
	got := filterProcesses(all, "", ProcessFilterModeAuto)
	if len(got) != 1 || len(got[0].Ports) != 1 || got[0].Ports[0].Port != 8080 {
		t.Fatalf("存在监听态时应裁剪为仅监听端口: got %+v", got)
	}
}

func TestIsKnownPortState(t *testing.T) {
	known := []string{
		"LISTENING", "LISTEN", "ESTABLISHED", "TIME_WAIT", "CLOSE_WAIT", "SYN_SENT",
		"SYN_SEND", "SYN_RECEIVED", "FIN_WAIT_1", "FIN_WAIT_2", "CLOSE", "CLOSED",
		"LAST_ACK", "CLOSING", "DELETE_TCB", "NEW_SYN_RECV", "UNKNOWN",
		// 大小写与空白须归一化
		"  listen ",
	}
	for _, state := range known {
		if !isKnownPortState(state) {
			t.Errorf("%q 应判定为已知状态", state)
		}
	}
	unknown := []string{"", "监听", "LISTENING_CN", "0D"}
	for _, state := range unknown {
		if isKnownPortState(state) {
			t.Errorf("%q 应判定为无法识别的状态", state)
		}
	}
}

func TestFilterProcesses_WhitespaceKeywordTreatedAsEmpty(t *testing.T) {
	// 前端可能传入纯空白；服务层 TrimSpace 后与空输入同语义
	got := filterProcesses(sampleProcessInfos(), "   ", ProcessFilterModeAuto)
	if len(got) != 3 {
		t.Errorf("纯空白关键字应按空输入处理: got %d 条", len(got))
	}
}

func TestNormalizeProcessFilterMode(t *testing.T) {
	cases := map[string]string{
		"":        ProcessFilterModeAuto,
		"  ":      ProcessFilterModeAuto,
		"AUTO":    ProcessFilterModeAuto,
		"Port":    ProcessFilterModePort,
		"name":    ProcessFilterModeName,
		"unknown": ProcessFilterModeAuto,
	}
	for in, want := range cases {
		if got := normalizeProcessFilterMode(in); got != want {
			t.Errorf("normalizeProcessFilterMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilterByPort_NonNumericKeywordReturnsEmpty(t *testing.T) {
	// 模式强制为端口但输入非数字（如中文进程名）：不给错，返回空列表由前端提示无结果
	if got := filterByPort(sampleProcessInfos(), "node"); len(got) != 0 {
		t.Errorf("非数字端口关键字应返回空: got %+v", got)
	}
}

// ===== 枚举编排 =====

func TestProcessService_ListProcesses_FiltersByKeyword(t *testing.T) {
	p := &fakeProcessProvider{processes: sampleProcessInfos()}
	s := newTestProcessService(p)

	got, err := s.ListProcesses("node", "")
	if err != nil {
		t.Fatalf("ListProcesses: %v", err)
	}
	if len(got) != 1 || got[0].Pid != 100 {
		t.Errorf("应返回 node.exe: got %+v", got)
	}
}

func TestProcessService_ListProcesses_EnumErrorPassThrough(t *testing.T) {
	want := model.NewAppError(model.ErrCodeProcessEnumTimeout, "进程枚举超时，请重试")
	p := &fakeProcessProvider{enumErr: want}
	s := newTestProcessService(p)

	_, err := s.ListProcesses("", "")
	if err == nil {
		t.Fatal("枚举错误应透传")
	}
	if code := appErrorCode(t, err); code != model.ErrCodeProcessEnumTimeout {
		t.Errorf("错误码应原样透传: got %s", code)
	}
}

// ===== 停止：自我保护 =====

func TestProcessService_StopProcess_SelfProtect(t *testing.T) {
	p := &fakeProcessProvider{selfPIDs: map[int]bool{100: true}}
	s := newTestProcessService(p)

	_, err := s.StopProcess(100, false)
	if code := appErrorCode(t, err); code != model.ErrCodeProcessSelfProtect {
		t.Fatalf("应拒绝终止本应用自身: got %s", code)
	}
	if len(p.stopCalls) != 0 || len(p.killCalls) != 0 {
		t.Errorf("自我保护拒绝后不得发起任何终止调用: stop=%v kill=%v", p.stopCalls, p.killCalls)
	}
}

func TestProcessService_KillProcess_SelfProtect(t *testing.T) {
	p := &fakeProcessProvider{selfPIDs: map[int]bool{100: true}}
	s := newTestProcessService(p)

	err := s.KillProcess(100, true)
	if code := appErrorCode(t, err); code != model.ErrCodeProcessSelfProtect {
		t.Fatalf("强杀也应拒绝本应用自身: got %s", code)
	}
	if len(p.killCalls) != 0 {
		t.Errorf("拒绝后不得调用强杀: %+v", p.killCalls)
	}
}

// ===== 停止：全自动两级 =====

func TestProcessService_StopProcess_GracefulSuccess(t *testing.T) {
	p := &fakeProcessProvider{
		stopDelivered: true,
		aliveSeq:      []bool{true, false}, // 第二次探测已退出
	}
	s := newTestProcessService(p)

	got, err := s.StopProcess(100, false)
	if err != nil {
		t.Fatalf("StopProcess: %v", err)
	}
	if !got.Stopped || got.Method != model.ProcessStopMethodGraceful {
		t.Errorf("应报优雅退出: got %+v", got)
	}
	if len(p.killCalls) != 0 {
		t.Errorf("优雅成功不得转强杀: %+v", p.killCalls)
	}
}

func TestProcessService_StopProcess_TimeoutFallsBackToForce(t *testing.T) {
	p := &fakeProcessProvider{
		stopDelivered: true,
		aliveSeq:      []bool{true}, // 恒存活 → 轮询超时
	}
	s := newTestProcessService(p)

	got, err := s.StopProcess(100, true)
	if err != nil {
		t.Fatalf("StopProcess: %v", err)
	}
	if !got.Stopped || got.Method != model.ProcessStopMethodForced {
		t.Errorf("超时后应转强杀并如实报 forced: got %+v", got)
	}
	if len(p.killCalls) != 1 || p.killCalls[0].pid != 100 || !p.killCalls[0].killTree {
		t.Errorf("应以 killTree=true 强杀目标进程: %+v", p.killCalls)
	}
}

func TestProcessService_StopProcess_NoGracefulChannelForcesImmediately(t *testing.T) {
	p := &fakeProcessProvider{stopDelivered: false} // 目标拒绝优雅请求（如 cmd /k 主动忽略）：不轮询直接强杀
	s := newTestProcessService(p)

	got, err := s.StopProcess(200, false)
	if err != nil {
		t.Fatalf("StopProcess: %v", err)
	}
	if !got.Stopped || got.Method != model.ProcessStopMethodForced {
		t.Errorf("优雅请求被拒时应直落强杀并如实报 forced: got %+v", got)
	}
	if p.aliveCalls != 0 {
		t.Errorf("请求被拒不应进入轮询: alive 被调用 %d 次", p.aliveCalls)
	}
	if len(p.killCalls) != 1 {
		t.Errorf("应直接强杀一次: %+v", p.killCalls)
	}
}

func TestProcessService_StopProcess_RequestErrorPassThrough(t *testing.T) {
	// 进程已不存在 / 权限不足由 util 层识别为 AppError，service 原样透传
	for _, code := range []string{model.ErrCodeProcessNotFound, model.ErrCodeProcessAccessDenied} {
		p := &fakeProcessProvider{stopErr: model.NewAppError(code, "msg")}
		s := newTestProcessService(p)

		_, err := s.StopProcess(100, false)
		if got := appErrorCode(t, err); got != code {
			t.Errorf("错误码应透传: got %s, want %s", got, code)
		}
		if len(p.killCalls) != 0 {
			t.Errorf("请求失败不得继续强杀: %+v", p.killCalls)
		}
	}
}

func TestProcessService_StopProcess_ForceKillErrorPassThrough(t *testing.T) {
	p := &fakeProcessProvider{
		stopDelivered: true,
		aliveSeq:      []bool{true}, // 超时转强杀
		killErr:       model.NewAppError(model.ErrCodeProcessKillFailed, "强制结束失败"),
	}
	s := newTestProcessService(p)

	_, err := s.StopProcess(100, false)
	if code := appErrorCode(t, err); code != model.ErrCodeProcessKillFailed {
		t.Errorf("强杀失败错误码应透传: got %s", code)
	}
}

// ===== 强杀 =====

func TestProcessService_KillProcess_Success(t *testing.T) {
	p := &fakeProcessProvider{}
	s := newTestProcessService(p)

	if err := s.KillProcess(300, true); err != nil {
		t.Fatalf("KillProcess: %v", err)
	}
	if len(p.killCalls) != 1 || p.killCalls[0].pid != 300 || !p.killCalls[0].killTree {
		t.Errorf("强杀参数应透传: %+v", p.killCalls)
	}
}

func TestProcessService_KillProcess_ErrorPassThrough(t *testing.T) {
	p := &fakeProcessProvider{killErr: model.NewAppError(model.ErrCodeProcessAccessDenied, "权限不足")}
	s := newTestProcessService(p)

	err := s.KillProcess(300, false)
	if code := appErrorCode(t, err); code != model.ErrCodeProcessAccessDenied {
		t.Errorf("强杀错误码应透传: got %s", code)
	}
}

// ===== 生产实现转发 =====

func TestUtilProcessProvider_DelegatesToUtil(t *testing.T) {
	// 生产实现仅转发 util 平台分叉函数：此处只校验装配不 panic 且接口满足（不真发信号）
	var _ ProcessProvider = utilProcessProvider{}
	if s := NewProcessService(); s.provider == nil || s.stopTimeout != processStopTimeout {
		t.Fatalf("生产服务应装配真实 util 实现与默认轮询参数: %+v", s)
	}
}
