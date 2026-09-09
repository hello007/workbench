package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
)

// === buildClaudeArgs 测试 ===

func TestBuildClaudeArgs_Basic(t *testing.T) {
	fn := &model.AiFunction{ID: "f1", Command: "/ab-weekly-report", Cwd: `D:\proj`}
	args := buildClaudeArgs(fn, "/ab-weekly-report", "")
	want := []string{
		"-p", "/ab-weekly-report",
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", "bypassPermissions",
	}
	if len(args) != len(want) {
		t.Fatalf("参数数量不符: 期望=%v 实际=%v", want, args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d]: 期望=%q 实际=%q", i, want[i], args[i])
		}
	}
}

func TestBuildClaudeArgs_Resume(t *testing.T) {
	fn := &model.AiFunction{ID: "f1"}
	args := buildClaudeArgs(fn, "确认落盘", "sess-123")
	found := false
	for i, a := range args {
		if a == "--resume" && i+1 < len(args) && args[i+1] == "sess-123" {
			found = true
		}
	}
	if !found {
		t.Errorf("未找到 --resume sess-123: %v", args)
	}
}

func TestBuildClaudeArgs_AddDirs(t *testing.T) {
	fn := &model.AiFunction{
		ID:      "f1",
		AddDirs: []string{`D:\a`, "", `D:\b`}, // 空 dir 应被跳过
	}
	args := buildClaudeArgs(fn, "p", "")
	count := 0
	for i, a := range args {
		if a == "--add-dir" && i+1 < len(args) {
			count++
		}
	}
	if count != 2 {
		t.Errorf("--add-dir 数量: 期望=2 实际=%d, args=%v", count, args)
	}
}

func TestBuildClaudeArgs_McpAndSettings(t *testing.T) {
	fn := &model.AiFunction{
		ID:  "f1",
		Env: map[string]string{"TENCENT_MEETING_TOKEN": "tok-1"},
		Mcp: &model.AiMcpConfig{
			Servers: map[string]model.AiMcpServer{
				"tencent-meeting": {Type: "http", URL: "https://mcp.example.com/v1"},
			},
		},
	}
	args := buildClaudeArgs(fn, "p", "")
	var mcpJSON, settingsJSON string
	for i, a := range args {
		if a == "--mcp-config" && i+1 < len(args) {
			mcpJSON = args[i+1]
		}
		if a == "--settings" && i+1 < len(args) {
			settingsJSON = args[i+1]
		}
	}
	if mcpJSON == "" {
		t.Fatalf("缺少 --mcp-config: %v", args)
	}
	if !strings.Contains(mcpJSON, "mcp.example.com") {
		t.Errorf("mcp-config 未含 server URL: %s", mcpJSON)
	}
	if settingsJSON == "" {
		t.Fatalf("缺少 --settings: %v", args)
	}
	if !strings.Contains(settingsJSON, "tok-1") {
		t.Errorf("settings 未注入 env token: %s", settingsJSON)
	}
}

// === renderPrompt 测试 ===

func TestRenderPrompt(t *testing.T) {
	out := renderPrompt("主题{{topic}}，时间{{startTime}}", map[string]string{
		"topic":     "评审会",
		"startTime": "15:00",
	})
	if out != "主题评审会，时间15:00" {
		t.Errorf("模板渲染结果不符: %s", out)
	}
}

func TestRenderPrompt_MissingParamKept(t *testing.T) {
	out := renderPrompt("{{a}}-{{b}}", map[string]string{"a": "1"})
	if out != "1-{{b}}" {
		t.Errorf("未提供的占位符应原样保留: %s", out)
	}
}

// === parseStreamLine 测试 ===

func TestParseStreamLine_AssistantText(t *testing.T) {
	line := `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[{"type":"text","text":"会议号 123"}]}}`
	text, isResult, sid, metrics := parseStreamLine(line)
	if text != "会议号 123" {
		t.Errorf("文本增量不符: %q", text)
	}
	if isResult {
		t.Errorf("assistant 事件不应是终态")
	}
	if sid != "s1" {
		t.Errorf("session_id 不符: %q", sid)
	}
	if metrics != nil {
		t.Errorf("assistant 事件不应有计量: %+v", metrics)
	}
}

func TestParseStreamLine_ResultEvent(t *testing.T) {
	line := `{"type":"result","subtype":"success","session_id":"s2","result":"done"}`
	_, isResult, sid, metrics := parseStreamLine(line)
	if !isResult {
		t.Errorf("result 事件应标记终态")
	}
	if sid != "s2" {
		t.Errorf("session_id 不符: %q", sid)
	}
	// result 事件无计量字段时 metrics 为 nil（全零判定）
	if metrics != nil {
		t.Errorf("无计量字段的 result 事件 metrics 应为 nil: %+v", metrics)
	}
}

// TestParseStreamLine_ResultWithMetrics result 事件携带计量字段时解析为 AiTaskMetrics
func TestParseStreamLine_ResultWithMetrics(t *testing.T) {
	line := `{"type":"result","subtype":"success","session_id":"s3","duration_ms":8268,"num_turns":1,"total_cost_usd":0.14502,"usage":{"input_tokens":28919,"output_tokens":17,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`
	_, isResult, _, metrics := parseStreamLine(line)
	if !isResult {
		t.Errorf("result 事件应标记终态")
	}
	if metrics == nil {
		t.Fatalf("带计量字段的 result 事件 metrics 不应为 nil")
	}
	if metrics.DurationMs != 8268 {
		t.Errorf("durationMs 不符: %d", metrics.DurationMs)
	}
	if metrics.NumTurns != 1 {
		t.Errorf("numTurns 不符: %d", metrics.NumTurns)
	}
	if metrics.CostUSD != 0.14502 {
		t.Errorf("costUsd 不符: %f", metrics.CostUSD)
	}
	if metrics.Usage == nil {
		t.Fatalf("usage 不应为 nil")
	}
	if metrics.Usage.InputTokens != 28919 {
		t.Errorf("inputTokens 不符: %d", metrics.Usage.InputTokens)
	}
	if metrics.Usage.OutputTokens != 17 {
		t.Errorf("outputTokens 不符: %d", metrics.Usage.OutputTokens)
	}
}

func TestParseStreamLine_NonJSONPassthrough(t *testing.T) {
	text, isResult, _, metrics := parseStreamLine("some diagnostic text")
	if text != "some diagnostic text\n" {
		t.Errorf("非 JSON 行应原样透传加换行: %q", text)
	}
	if isResult {
		t.Errorf("非 JSON 行不应是终态")
	}
	if metrics != nil {
		t.Errorf("非 JSON 行不应有计量: %+v", metrics)
	}
}

func TestParseStreamLine_ToolUseIgnored(t *testing.T) {
	line := `{"type":"assistant","session_id":"s1","message":{"content":[{"type":"tool_use","name":"get_meeting"}]}}`
	text, _, _, _ := parseStreamLine(line)
	if text != "" {
		t.Errorf("tool_use 片段不应产出文本: %q", text)
	}
}

// === expandEnvRef 测试 ===

func TestExpandEnvRef(t *testing.T) {
	os.Setenv("WB_TEST_TOK", "abc")
	defer os.Unsetenv("WB_TEST_TOK")

	if got := expandEnvRef("$ENV:WB_TEST_TOK"); got != "abc" {
		t.Errorf("env 引用未展开: %q", got)
	}
	if got := expandEnvRef("literal"); got != "literal" {
		t.Errorf("普通值应原样返回: %q", got)
	}
	if got := expandEnvRef("$ENV:WB_TEST_MISSING_XYZ"); got != "$ENV:WB_TEST_MISSING_XYZ" {
		t.Errorf("缺失 env 应保留原值: %q", got)
	}
}

// === 配置持久化测试 ===

func TestLoadAiFunctions_DefaultSeed(t *testing.T) {
	dir := t.TempDir()
	svc := NewAiFunctionService(nil, filepath.Join(dir, "ai_functions.json"))

	funcs, err := svc.LoadAiFunctions()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(funcs) != 4 {
		t.Fatalf("默认功能项数量: 期望=4 实际=%d", len(funcs))
	}
	ids := map[string]bool{}
	for _, f := range funcs {
		ids[f.ID] = true
	}
	for _, want := range []string{"speech-doc", "weekly-report", "meeting-book", "meeting-list"} {
		if !ids[want] {
			t.Errorf("默认配置缺少功能项 %s", want)
		}
	}
	// 二次加载应读文件而非重新 seed
	funcs2, _ := svc.LoadAiFunctions()
	if len(funcs2) != 4 {
		t.Errorf("二次加载数量不符: %d", len(funcs2))
	}
}

func TestLoadAiFunctions_EmptyArrayReseed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ai_functions.json")

	// 预写空数组（配置界面保存过空数组的现场），Load 应自愈回种默认四项
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatalf("预写空数组失败: %v", err)
	}
	svc := NewAiFunctionService(nil, path)
	funcs, err := svc.LoadAiFunctions()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(funcs) != 4 {
		t.Fatalf("空数组自愈后数量: 期望=4 实际=%d", len(funcs))
	}

	// 文件应被重写为 schema v2 结构的默认四项（直接读文件验证，二次 Load 无法区分回种与重写）
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读回配置文件失败: %v", err)
	}
	var persisted model.AiFunctionsConfig
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("配置文件不是合法 JSON: %v", err)
	}
	if persisted.SchemaVersion != model.CurrentSchemaVersion {
		t.Errorf("schemaVersion 不符: 期望=%d 实际=%d", model.CurrentSchemaVersion, persisted.SchemaVersion)
	}
	if len(persisted.Functions) != 4 {
		t.Errorf("配置文件未被重写为默认四项: 期望=4 实际=%d", len(persisted.Functions))
	}
}

func TestSaveAiFunctions_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	svc := NewAiFunctionService(nil, filepath.Join(dir, "ai_functions.json"))

	in := []*model.AiFunction{
		{ID: "x1", Name: "自定义", Command: "/x", Cwd: `D:\x`, Completion: "copy"},
	}
	if err := svc.SaveAiFunctions(in); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	out, err := svc.LoadAiFunctions()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(out) != 1 || out[0].ID != "x1" || out[0].Completion != "copy" {
		t.Errorf("roundtrip 数据不符: %+v", out)
	}
}

// === BuildStagePrompt 测试 ===

func TestBuildStagePrompt_NilSpec(t *testing.T) {
	p, err := BuildStagePrompt("/cmd", nil, nil)
	if err != nil || p != "/cmd" {
		t.Errorf("nil spec 应直接用 command: p=%q err=%v", p, err)
	}
}

func TestBuildStagePrompt_NoneWithTemplate(t *testing.T) {
	spec := &model.AiParamSpec{Type: "none", PromptTemplate: "/x 查询列表"}
	p, err := BuildStagePrompt("/cmd", spec, nil)
	if err != nil || p != "/x 查询列表" {
		t.Errorf("none+模板应渲染模板: p=%q err=%v", p, err)
	}
}

func TestBuildStagePrompt_File(t *testing.T) {
	spec := &model.AiParamSpec{Type: "file", Label: "源文档", TextFieldKey: "file"}
	p, err := BuildStagePrompt("/ab-office:agree-slides", spec, map[string]string{"file": `D:\a.md`})
	if err != nil {
		t.Fatalf("file 组装失败: %v", err)
	}
	if p != `/ab-office:agree-slides D:\a.md` {
		t.Errorf("file prompt 不符: %q", p)
	}
	// 缺参数报错
	_, err = BuildStagePrompt("/c", spec, map[string]string{"file": " "})
	if err == nil {
		t.Errorf("空 file 参数应报错")
	}
}

func TestBuildStagePrompt_FileMultiLine(t *testing.T) {
	spec := &model.AiParamSpec{Type: "file", Label: "源文档", TextFieldKey: "file"}
	// 两路径含空格 + 空行/空白行，应逐行 trim 滤空后按顺序引号包裹拼接
	p, err := BuildStagePrompt("/ab-office:agree-slides", spec, map[string]string{
		"file": "D:\\docs\\周报 草稿.md\nD:\\工作\\Typora\\a b.md\n  \n",
	})
	if err != nil {
		t.Fatalf("file 多行组装失败: %v", err)
	}
	want := `/ab-office:agree-slides "D:\docs\周报 草稿.md" "D:\工作\Typora\a b.md"`
	if p != want {
		t.Errorf("多行 file prompt 不符: 期望=%q 实际=%q", want, p)
	}
}

func TestBuildStagePrompt_FormRequired(t *testing.T) {
	spec := &model.AiParamSpec{
		Type:           "form",
		PromptTemplate: "主题{{topic}} 时长{{duration}}",
		Fields: []model.AiFormField{
			{Key: "topic", Label: "主题", Required: true},
			{Key: "duration", Label: "时长"},
		},
	}
	p, err := BuildStagePrompt("", spec, map[string]string{"topic": "评审", "duration": "30"})
	if err != nil || p != "主题评审 时长30" {
		t.Errorf("form 渲染不符: p=%q err=%v", p, err)
	}
	_, err = BuildStagePrompt("", spec, map[string]string{"duration": "30"})
	if err == nil {
		t.Errorf("缺 Required 字段应报错")
	}
}

func TestBuildStagePrompt_TextFollowUp(t *testing.T) {
	spec := &model.AiParamSpec{Type: "text", Label: "会议", TextFieldKey: "meeting", PromptTemplate: "/x 取消会议 {{meeting}}，先展示信息"}
	p, err := BuildStagePrompt("", spec, map[string]string{"meeting": "123-456"})
	if err != nil {
		t.Fatalf("text 组装失败: %v", err)
	}
	if p != "/x 取消会议 123-456，先展示信息" {
		t.Errorf("text prompt 不符: %q", p)
	}
}

// === BuildFollowUpPrompt 测试 ===

// 一级优先级：Input 自带 PromptTemplate 时走 BuildStagePrompt 完整处理，
// FollowUp.PromptTemplate 不参与（作为干扰值验证不被误用）
func TestBuildFollowUpPrompt_InputTemplateFirst(t *testing.T) {
	fu := &model.AiFollowUp{
		ID:             "fu1",
		PromptTemplate: "followUp 模板 {{meeting}}",
		Input: &model.AiParamSpec{
			Type:           "text",
			Label:          "会议",
			TextFieldKey:   "meeting",
			PromptTemplate: "/input 模板 {{meeting}} 处理",
		},
	}
	p, err := BuildFollowUpPrompt(fu, map[string]string{"meeting": "123-456"})
	if err != nil {
		t.Fatalf("组装失败: %v", err)
	}
	if p != "/input 模板 123-456 处理" {
		t.Errorf("Input 模板应优先: %q", p)
	}
}

// 二级优先级（Bug 1 回归）：Input 为 type=text 无模板时，BuildStagePrompt 会返回
// 参数裸值，FollowUp.PromptTemplate（含直接执行指令）必须生效而非被裸值顶替
func TestBuildFollowUpPrompt_RenderTemplate(t *testing.T) {
	fu := &model.AiFollowUp{
		ID:             "cancel-meeting",
		PromptTemplate: "/tencent-meeting-mcp 取消会议：{{meeting}}。用户已确认，直接执行取消。",
		Input:          &model.AiParamSpec{Type: "text", Label: "要取消的会议（会议号或主题）", TextFieldKey: "meeting"},
	}
	p, err := BuildFollowUpPrompt(fu, map[string]string{"meeting": "评审会（会议号 599984404）"})
	if err != nil {
		t.Fatalf("组装失败: %v", err)
	}
	if !strings.Contains(p, "取消会议：评审会（会议号 599984404）") {
		t.Errorf("FollowUp 模板未渲染生效: %q", p)
	}
	if !strings.Contains(p, "直接执行取消") {
		t.Errorf("模板中的执行指令丢失: %q", p)
	}
	if strings.Contains(p, "{{") {
		t.Errorf("渲染后不应残留占位符: %q", p)
	}
}

// 三级退化：模板为空（或占位符未命中残留 {{）时，params 非空值空格拼接
func TestBuildFollowUpPrompt_FallbackRawParams(t *testing.T) {
	// 模板为空 + 无 Input
	fu := &model.AiFollowUp{ID: "fu2"}
	p, err := BuildFollowUpPrompt(fu, map[string]string{"meeting": "123-456", "note": "补充说明"})
	if err != nil {
		t.Fatalf("组装失败: %v", err)
	}
	if !strings.Contains(p, "123-456") || !strings.Contains(p, "补充说明") {
		t.Errorf("退化拼接应含全部非空参数值: %q", p)
	}

	// 模板占位符未命中 params（残留 {{）→ 同样退化
	fu3 := &model.AiFollowUp{ID: "fu3", PromptTemplate: "取消 {{missing}}"}
	p3, err := BuildFollowUpPrompt(fu3, map[string]string{"meeting": "789"})
	if err != nil {
		t.Fatalf("组装失败: %v", err)
	}
	if p3 != "789" {
		t.Errorf("占位符未命中应退化为裸值拼接: %q", p3)
	}
}

// 全空报错：模板空且无有效参数
func TestBuildFollowUpPrompt_EmptyError(t *testing.T) {
	_, err := BuildFollowUpPrompt(&model.AiFollowUp{ID: "fu4"}, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "无有效参数") {
		t.Errorf("全空应报「无有效参数」: err=%v", err)
	}
	_, err = BuildFollowUpPrompt(&model.AiFollowUp{ID: "fu5", PromptTemplate: "取消 {{meeting}}"}, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "无有效参数") {
		t.Errorf("模板占位符无值也应报「无有效参数」: err=%v", err)
	}
}

// TestGetConcurrencyStatus 验证并发占用统计：running/queued/max 计数正确
func TestGetConcurrencyStatus(t *testing.T) {
	svc := NewAiFunctionService(nil, "")
	// 手动注入三种状态的 task
	svc.tasks["t1"] = &aiTaskRuntime{running: true, queued: false}
	svc.tasks["t2"] = &aiTaskRuntime{running: true, queued: false}
	svc.tasks["t3"] = &aiTaskRuntime{running: false, queued: true}
	svc.tasks["t4"] = &aiTaskRuntime{running: false, queued: false} // 已完成，不计 running/queued

	st := svc.GetConcurrencyStatus()
	if st.Running != 2 {
		t.Errorf("running 计数不符: %d（期望 2）", st.Running)
	}
	if st.Queued != 1 {
		t.Errorf("queued 计数不符: %d（期望 1）", st.Queued)
	}
	if st.Max != aiTaskMaxConcurrent {
		t.Errorf("max 不符: %d（期望 %d）", st.Max, aiTaskMaxConcurrent)
	}
}

// TestRemoveAiTask 验证清理规则：运行中/排队中不可清理，已完成可清理
func TestRemoveAiTask(t *testing.T) {
	svc := NewAiFunctionService(nil, "")
	svc.tasks["running"] = &aiTaskRuntime{running: true, queued: false}
	svc.tasks["queued"] = &aiTaskRuntime{running: false, queued: true}
	svc.tasks["done"] = &aiTaskRuntime{running: false, queued: false}

	// 运行中不可清理
	if svc.RemoveAiTask("running") {
		t.Error("运行中任务应不可清理")
	}
	// 排队中不可清理
	if svc.RemoveAiTask("queued") {
		t.Error("排队中任务应不可清理")
	}
	// 已完成可清理
	if !svc.RemoveAiTask("done") {
		t.Error("已完成任务应可清理")
	}
	if _, ok := svc.tasks["done"]; ok {
		t.Error("清理后 tasks map 不应再含该任务")
	}
	// 不存在的任务返回 false
	if svc.RemoveAiTask("nonexistent") {
		t.Error("不存在的任务应返回 false")
	}
}

// TestParseStreamLine_ResultWithMetrics 字段映射已在上方测试；
// 此处确保 max 常量与构造函数一致（信号量缓冲长度）
func TestNewAiFunctionService_SemCapacity(t *testing.T) {
	svc := NewAiFunctionService(nil, "")
	if cap(svc.concurrencySem) != aiTaskMaxConcurrent {
		t.Errorf("信号量缓冲长度不符: %d（期望 %d）", cap(svc.concurrencySem), aiTaskMaxConcurrent)
	}
}

// TestBuildClaudeArgs_McpStdio 验证 stdio 类型 MCP server 的序列化：
// 字段结构（command/args/env，不含 url/headers/cwd）、env 走 expandEnvRef、原配置不被修改
func TestBuildClaudeArgs_McpStdio(t *testing.T) {
	os.Setenv("WB_TEST_MCP_TOK", "sec-xyz")
	defer os.Unsetenv("WB_TEST_MCP_TOK")

	fn := &model.AiFunction{
		ID: "f1",
		Mcp: &model.AiMcpConfig{
			Servers: map[string]model.AiMcpServer{
				"fs": {
					Type:    "stdio",
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", `D:\docs`},
					Env:     map[string]string{"API_KEY": "$ENV:WB_TEST_MCP_TOK"},
				},
			},
		},
	}
	args := buildClaudeArgs(fn, "p", "")
	var mcpJSON string
	for i, a := range args {
		if a == "--mcp-config" && i+1 < len(args) {
			mcpJSON = args[i+1]
		}
	}
	if mcpJSON == "" {
		t.Fatalf("缺少 --mcp-config: %v", args)
	}
	// stdio 字段输出
	if !strings.Contains(mcpJSON, `"command":"npx"`) {
		t.Errorf("stdio 未输出 command: %s", mcpJSON)
	}
	if !strings.Contains(mcpJSON, `"args":[`) {
		t.Errorf("stdio 未输出 args: %s", mcpJSON)
	}
	// env 走 expandEnvRef：$ENV:WB_TEST_MCP_TOK 展开为 sec-xyz
	if !strings.Contains(mcpJSON, "sec-xyz") {
		t.Errorf("stdio env 未走 expandEnvRef 展开: %s", mcpJSON)
	}
	if strings.Contains(mcpJSON, "$ENV:WB_TEST_MCP_TOK") {
		t.Errorf("stdio env 引用未展开，残留 $ENV:: %s", mcpJSON)
	}
	// http 字段 omitempty 不输出（stdio server 无 url/headers）
	if strings.Contains(mcpJSON, `"url"`) {
		t.Errorf("stdio server 不应输出 url 字段: %s", mcpJSON)
	}
	if strings.Contains(mcpJSON, `"headers"`) {
		t.Errorf("stdio server 不应输出 headers 字段: %s", mcpJSON)
	}
	// 不含 cwd（官方不支持，见 research/mcp-stdio-config-format.md）
	if strings.Contains(mcpJSON, `"cwd"`) {
		t.Errorf("不应输出 cwd 字段（官方不支持）: %s", mcpJSON)
	}
	// 顶层 mcpServers 结构
	if !strings.Contains(mcpJSON, `"mcpServers"`) {
		t.Errorf("顶层应为 mcpServers: %s", mcpJSON)
	}
	// 原配置不被修改：fn.Mcp 中 env 仍为 $ENV: 引用（拷贝展开未污染原对象）
	orig := fn.Mcp.Servers["fs"].Env["API_KEY"]
	if orig != "$ENV:WB_TEST_MCP_TOK" {
		t.Errorf("原配置 env 被修改: 期望=$ENV:WB_TEST_MCP_TOK 实际=%s", orig)
	}
}

// TestBuildClaudeArgs_McpHttpStdioMixed 验证 http 与 stdio server 同块混用，
// 各自按 type 输出对应字段，交叉字段 omitempty 不输出
func TestBuildClaudeArgs_McpHttpStdioMixed(t *testing.T) {
	fn := &model.AiFunction{
		ID: "f1",
		Mcp: &model.AiMcpConfig{
			Servers: map[string]model.AiMcpServer{
				"web":   {Type: "http", URL: "https://mcp.example.com/v1", Headers: map[string]string{"Authorization": "Bearer tok"}},
				"local": {Type: "stdio", Command: "python", Args: []string{"-m", "mcp_server"}},
			},
		},
	}
	args := buildClaudeArgs(fn, "p", "")
	var mcpJSON string
	for i, a := range args {
		if a == "--mcp-config" && i+1 < len(args) {
			mcpJSON = args[i+1]
		}
	}
	if mcpJSON == "" {
		t.Fatalf("缺少 --mcp-config: %v", args)
	}
	// 解析为结构校验（交叉字段 omitempty：http 无 command/args/env，stdio 无 url/headers）
	var parsed struct {
		McpServers map[string]struct {
			Type    string            `json:"type"`
			URL     string            `json:"url,omitempty"`
			Headers map[string]string `json:"headers,omitempty"`
			Command string            `json:"command,omitempty"`
			Args    []string          `json:"args,omitempty"`
			Env     map[string]string `json:"env,omitempty"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(mcpJSON), &parsed); err != nil {
		t.Fatalf("mcp-config 非合法 JSON: %v", err)
	}
	if len(parsed.McpServers) != 2 {
		t.Fatalf("server 数量不符: 期望=2 实际=%d", len(parsed.McpServers))
	}
	web := parsed.McpServers["web"]
	if web.Type != "http" || web.URL != "https://mcp.example.com/v1" || web.Command != "" {
		t.Errorf("http server 字段不符: %+v", web)
	}
	if _, ok := web.Headers["Authorization"]; !ok {
		t.Errorf("http headers 丢失: %+v", web.Headers)
	}
	local := parsed.McpServers["local"]
	if local.Type != "stdio" || local.Command != "python" || len(local.Args) != 2 || local.URL != "" {
		t.Errorf("stdio server 字段不符: %+v", local)
	}
	if local.Headers != nil {
		t.Errorf("stdio server 不应输出 headers: %+v", local.Headers)
	}
}

// === 3.3 流式文件：表格预解析、尾部读取、全量读取、RemoveAiTask 删文件 ===

// TestExtractTable_ValidMarkdown 验证从输出文本预解析 markdown 表格：
// 取最后一个连续表格行块，跳过分隔行，首行为表头、其余为数据行，按表头名映射
func TestExtractTable_ValidMarkdown(t *testing.T) {
	text := "一些说明文字\n" +
		"| 会议主题 | 会议号 | 开始时间 |\n" +
		"| --- | --- | --- |\n" +
		"| 评审会 | 123 | 09:00 |\n" +
		"| 站会 | 456 | 09:30 |\n"
	tbl := extractTable(text, int64(len(text)))
	if tbl == nil {
		t.Fatal("应解析出表格，返回 nil")
	}
	wantHeaders := []string{"会议主题", "会议号", "开始时间"}
	if len(tbl.Headers) != 3 {
		t.Fatalf("表头数量不符: %v", tbl.Headers)
	}
	for i, h := range wantHeaders {
		if tbl.Headers[i] != h {
			t.Errorf("表头[%d]: 期望=%s 实际=%s", i, h, tbl.Headers[i])
		}
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("数据行数量不符: %d（期望 2）", len(tbl.Rows))
	}
	if tbl.Rows[0]["会议号"] != "123" {
		t.Errorf("首行会议号不符: %s", tbl.Rows[0]["会议号"])
	}
	if tbl.Rows[1]["会议主题"] != "站会" {
		t.Errorf("次行会议主题不符: %s", tbl.Rows[1]["会议主题"])
	}
}

// TestExtractTable_NoTable 无合法表格（行块不足）返回 nil
func TestExtractTable_NoTable(t *testing.T) {
	if extractTable("纯文本无表格", 100) != nil {
		t.Error("无表格应返回 nil")
	}
	if extractTable("| 单行 |", 100) != nil {
		t.Error("单行表格块（不足 2 行）应返回 nil")
	}
}

// TestExtractTable_TakesLastBlock 多个表格块取最后一个
func TestExtractTable_TakesLastBlock(t *testing.T) {
	text := "| A | B |\n| --- | --- |\n| 1 | 2 |\n\n| C | D |\n| --- | --- |\n| 3 | 4 |\n"
	tbl := extractTable(text, int64(len(text)))
	if tbl == nil {
		t.Fatal("应解析出表格")
	}
	if tbl.Headers[0] != "C" {
		t.Errorf("应取最后一个表格块，表头: %v", tbl.Headers)
	}
	if tbl.Rows[0]["C"] != "3" {
		t.Errorf("最后一个表格数据行不符: %v", tbl.Rows[0])
	}
}

// TestReadTail 验证从输出文件读末尾 N 字节：超过 maxBytes 截断取末尾，不足返回全文
func TestReadTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	content := strings.Repeat("x", 10000) + "TAIL"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写测试文件失败: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("打开文件失败: %v", err)
	}
	defer f.Close()
	size := int64(len(content))
	// 末尾 4KB 预览：大文件截断取末尾，应以 TAIL 结尾
	tail := readTail(f, size, 4096)
	if !strings.HasSuffix(tail, "TAIL") {
		t.Errorf("末尾预览应以 TAIL 结尾: %q", tail[len(tail)-20:])
	}
	if len(tail) > 4096 {
		t.Errorf("末尾预览不应超 4KB: %d", len(tail))
	}
	// 小文件（短于 maxBytes）返回全文：用独立小文件验证，size 须为真实大小
	shortPath := filepath.Join(dir, "short.txt")
	if err := os.WriteFile(shortPath, []byte("TAIL"), 0o644); err != nil {
		t.Fatalf("写小文件失败: %v", err)
	}
	sf, err := os.Open(shortPath)
	if err != nil {
		t.Fatalf("打开小文件失败: %v", err)
	}
	defer sf.Close()
	short := readTail(sf, 4, 4096)
	if short != "TAIL" {
		t.Errorf("小文件应返回全文: %s", short)
	}
	// size=0 返回空
	if readTail(f, 0, 4096) != "" {
		t.Error("size=0 应返回空")
	}
}

// TestGetAiTaskOutput 全量读取输出文件：任务在 map 读运行期文件，已清理读归档目录
func TestGetAiTaskOutput(t *testing.T) {
	dir := t.TempDir()
	svc := NewAiFunctionService(nil, filepath.Join(dir, "ai_functions.json"))
	// 运行期输出文件
	outDir := svc.outputDir()
	_ = os.MkdirAll(outDir, 0o755)
	taskID := "aitask-test-1"
	outPath := svc.outputFilePath(taskID)
	want := "这是完整输出内容\n多行"
	if err := os.WriteFile(outPath, []byte(want), 0o644); err != nil {
		t.Fatalf("写输出文件失败: %v", err)
	}
	svc.tasks[taskID] = &aiTaskRuntime{
		id:         taskID,
		outputFile: nil,
		outputPath: outPath,
		outputSize: int64(len(want)),
	}
	got, err := svc.GetAiTaskOutput(taskID)
	if err != nil {
		t.Fatalf("读取输出失败: %v", err)
	}
	if got != want {
		t.Errorf("输出内容不符: 期望=%q 实际=%q", want, got)
	}
	// 任务从 map 清理后：读归档目录（不存在则报错）
	delete(svc.tasks, taskID)
	_, err = svc.GetAiTaskOutput(taskID)
	if err == nil {
		// 归档目录无该文件应报错（除非恰好存在同名归档）
		// 写一份归档文件再验证可读
		histDir := filepath.Join(dir, aiTaskHistoryDirName)
		_ = os.MkdirAll(histDir, 0o755)
		_ = os.WriteFile(filepath.Join(histDir, taskID+".txt"), []byte("archived"), 0o644)
		got2, err2 := svc.GetAiTaskOutput(taskID)
		if err2 != nil {
			t.Errorf("归档文件读取应成功: %v", err2)
		}
		if got2 != "archived" {
			t.Errorf("归档内容不符: %s", got2)
		}
	}
}

// TestRemoveAiTask_DeletesOutputFile 验证 RemoveAiTask 同步删除运行期输出文件
func TestRemoveAiTask_DeletesOutputFile(t *testing.T) {
	dir := t.TempDir()
	svc := NewAiFunctionService(nil, filepath.Join(dir, "ai_functions.json"))
	outDir := svc.outputDir()
	_ = os.MkdirAll(outDir, 0o755)
	taskID := "aitask-cleanup"
	outPath := svc.outputFilePath(taskID)
	if err := os.WriteFile(outPath, []byte("temp output"), 0o644); err != nil {
		t.Fatalf("写输出文件失败: %v", err)
	}
	svc.tasks[taskID] = &aiTaskRuntime{id: taskID, outputPath: outPath}
	if !svc.RemoveAiTask(taskID) {
		t.Fatal("已完成任务应可清理")
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("清理后输出文件应被删除: %v", err)
	}
}

// TestGetAiTaskState_PreviewNotFull 验证 GetAiTaskState 返回末尾预览而非全量，
// 且 OutputSize/OutputFile/TableExtracted 字段正确
func TestGetAiTaskState_PreviewNotFull(t *testing.T) {
	dir := t.TempDir()
	svc := NewAiFunctionService(nil, filepath.Join(dir, "ai_functions.json"))
	outDir := svc.outputDir()
	_ = os.MkdirAll(outDir, 0o755)
	taskID := "aitask-state"
	outPath := svc.outputFilePath(taskID)
	// 输出小于预览窗口（4KB）：preview 即全文，表格完整可预解析
	content := "一些前置说明\n| 会议主题 | 会议号 |\n| --- | --- |\n| 评审 | 123 |\n"
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		t.Fatalf("写输出文件失败: %v", err)
	}
	f, _ := os.Open(outPath)
	defer f.Close()
	svc.tasks[taskID] = &aiTaskRuntime{
		id:         taskID,
		functionID: "meeting-list",
		prompt:     "/tencent-meeting-mcp",
		outputFile: f,
		outputPath: outPath,
		outputSize: int64(len(content)),
	}
	st := svc.GetAiTaskState(taskID)
	if st == nil {
		t.Fatal("状态不应为 nil")
	}
	if st.OutputSize != int64(len(content)) {
		t.Errorf("OutputSize 不符: %d（期望 %d）", st.OutputSize, int64(len(content)))
	}
	if st.OutputFile == "" {
		t.Error("OutputFile 不应为空")
	}
	// 小文件预览即全文，应含表格
	if !strings.Contains(st.Output, "| 评审 | 123 |") {
		t.Errorf("预览应含表格行: %q", st.Output)
	}
	if len(st.Output) > aiTaskOutputPreviewSize {
		t.Errorf("预览不应超 %d: %d", aiTaskOutputPreviewSize, len(st.Output))
	}
	// 表格预解析应成功（会议号在表头）
	if st.TableExtracted == nil {
		t.Fatal("TableExtracted 不应为 nil（输出含表格）")
	}
	if !containsHeader(st.TableExtracted.Headers, "会议号") {
		t.Errorf("预解析表头应含会议号: %v", st.TableExtracted.Headers)
	}
}

// TestGetAiTaskState_LargeOutputPreviewTruncated 大输出预览截断验证：
// 输出远超 4KB 时预览不超过 4KB，且 OutputSize 准确
func TestGetAiTaskState_LargeOutputPreviewTruncated(t *testing.T) {
	dir := t.TempDir()
	svc := NewAiFunctionService(nil, filepath.Join(dir, "ai_functions.json"))
	outDir := svc.outputDir()
	_ = os.MkdirAll(outDir, 0o755)
	taskID := "aitask-large"
	outPath := svc.outputFilePath(taskID)
	content := strings.Repeat("x", 20000) // 20KB，远超 4KB 预览
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		t.Fatalf("写输出文件失败: %v", err)
	}
	f, _ := os.Open(outPath)
	defer f.Close()
	svc.tasks[taskID] = &aiTaskRuntime{
		id:         taskID,
		outputFile: f,
		outputPath: outPath,
		outputSize: int64(len(content)),
	}
	st := svc.GetAiTaskState(taskID)
	if st.OutputSize != int64(len(content)) {
		t.Errorf("OutputSize 不符: %d", st.OutputSize)
	}
	if len(st.Output) > aiTaskOutputPreviewSize {
		t.Errorf("大输出预览应不超过 %d: %d", aiTaskOutputPreviewSize, len(st.Output))
	}
	if len(st.Output) < aiTaskOutputPreviewSize-100 {
		t.Errorf("大输出预览应接近 %d: %d", aiTaskOutputPreviewSize, len(st.Output))
	}
	// 无表格时 TableExtracted 为 nil
	if st.TableExtracted != nil {
		t.Error("无表格输出 TableExtracted 应为 nil")
	}
}

// containsHeader 辅助：表头列表是否含指定列
func containsHeader(headers []string, want string) bool {
	for _, h := range headers {
		if h == want {
			return true
		}
	}
	return false
}

// TestClassifyStatus 验证任务终态归类（success/failed/canceled/timeout）
func TestClassifyStatus(t *testing.T) {
	// canceled 优先
	task := &aiTaskRuntime{canceled: true, cmd: &exec.Cmd{}}
	if s := classifyStatus(task, fmt.Errorf("exit 1")); s != "canceled" {
		t.Errorf("canceled 应优先: %s", s)
	}
	// 超时（ctx 真实触发 DeadlineExceeded，不 cancel 否则变 Canceled）
	toCtx, toCancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer toCancel()
	time.Sleep(10 * time.Millisecond) // 等超时触发
	task2 := &aiTaskRuntime{cmd: &exec.Cmd{}, ctx: toCtx}
	if s := classifyStatus(task2, fmt.Errorf("timeout")); s != "timeout" {
		t.Errorf("超时应归类 timeout: %s", s)
	}
	// 非 0 退出（waitErr 非 nil）→ failed
	task3 := &aiTaskRuntime{cmd: &exec.Cmd{}}
	if s := classifyStatus(task3, fmt.Errorf("exit 1")); s != "failed" {
		t.Errorf("非 0 退出应 failed: %s", s)
	}
	// 成功（无错误无 canceled）
	task4 := &aiTaskRuntime{cmd: &exec.Cmd{}}
	if s := classifyStatus(task4, nil); s != "success" {
		t.Errorf("无错误应 success: %s", s)
	}
}

// TestLoadAiFunctions_V1ArrayMigrate 验证 v1 裸数组加载迁移：
// 旧格式（无 schemaVersion 的数组）加载后补全缺失字段、落盘 v2 结构，tags/pinned 零值合法。
func TestLoadAiFunctions_V1ArrayMigrate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ai_functions.json")
	v1 := `[{"id":"a","name":"A","command":"/a","cwd":"D:\\a"},{"id":"b","name":"B","command":"/b","cwd":"D:\\b","permissionMode":"default","timeoutMinutes":5,"completion":"copy"}]`
	if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
		t.Fatalf("预写 v1 配置失败: %v", err)
	}
	svc := NewAiFunctionService(nil, path)
	funcs, err := svc.LoadAiFunctions()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(funcs) != 2 {
		t.Fatalf("迁移后数量不符: 期望=2 实际=%d", len(funcs))
	}
	if funcs[0].PermissionMode != "bypassPermissions" || funcs[0].TimeoutMinutes != aiTaskDefaultTimeoutMinutes || funcs[0].Completion != "none" {
		t.Errorf("a 项字段补全不符: mode=%s timeout=%d completion=%s", funcs[0].PermissionMode, funcs[0].TimeoutMinutes, funcs[0].Completion)
	}
	if funcs[1].PermissionMode != "default" || funcs[1].TimeoutMinutes != 5 || funcs[1].Completion != "copy" {
		t.Errorf("b 项已有字段被覆盖: mode=%s timeout=%d completion=%s", funcs[1].PermissionMode, funcs[1].TimeoutMinutes, funcs[1].Completion)
	}
	raw, _ := os.ReadFile(path)
	var persisted model.AiFunctionsConfig
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("落盘文件非 v2 结构: %v", err)
	}
	if persisted.SchemaVersion != model.CurrentSchemaVersion || len(persisted.Functions) != 2 {
		t.Errorf("落盘 v2 结构不符: version=%d count=%d", persisted.SchemaVersion, len(persisted.Functions))
	}
}

// TestLoadAiFunctions_PartialInvalidTrimmed 验证部分功能项非法：备份 + 剔除非法项保留合法项。
func TestLoadAiFunctions_PartialInvalidTrimmed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ai_functions.json")
	cfg := `{"schemaVersion":2,"functions":[{"id":"a","name":"A","command":"/a","cwd":"D:\\a"},{"id":"b","name":"B","cwd":"D:\\b"},{"id":"c","name":"C","command":"/c"}]}`
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatalf("预写配置失败: %v", err)
	}
	svc := NewAiFunctionService(nil, path)
	funcs, err := svc.LoadAiFunctions()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(funcs) != 1 || funcs[0].ID != "a" {
		t.Errorf("应剔除 b/c 保留 a: %+v", funcs)
	}
	matches, _ := filepath.Glob(path + ".bak.*")
	if len(matches) != 1 {
		t.Errorf("应生成 1 个备份文件: %v", matches)
	}
}

// TestLoadAiFunctions_AllInvalidReseed 验证全部非法时备份后回种 seed。
func TestLoadAiFunctions_AllInvalidReseed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ai_functions.json")
	cfg := `{"schemaVersion":2,"functions":[{"id":"a","name":"A","cwd":"D:\\a"}]}`
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatalf("预写配置失败: %v", err)
	}
	svc := NewAiFunctionService(nil, path)
	funcs, err := svc.LoadAiFunctions()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(funcs) != 4 {
		t.Errorf("全部非法应回种 4 项 seed: %d", len(funcs))
	}
	matches, _ := filepath.Glob(path + ".bak.*")
	if len(matches) != 1 {
		t.Errorf("应生成 1 个备份文件: %v", matches)
	}
}

// TestLoadAiFunctions_GarbledTopLevelReseed 验证顶层结构非法（非数组非对象）备份回种。
func TestLoadAiFunctions_GarbledTopLevelReseed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ai_functions.json")
	if err := os.WriteFile(path, []byte("not a json at all"), 0o644); err != nil {
		t.Fatalf("预写非法内容失败: %v", err)
	}
	svc := NewAiFunctionService(nil, path)
	funcs, err := svc.LoadAiFunctions()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(funcs) != 4 {
		t.Errorf("非法内容应回种 4 项 seed: %d", len(funcs))
	}
	matches, _ := filepath.Glob(path + ".bak.*")
	if len(matches) != 1 {
		t.Errorf("应生成 1 个备份文件: %v", matches)
	}
}
