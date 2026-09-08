package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"workbench/model"
	"workbench/util"
)

// 默认超时（分钟），功能项 TimeoutMinutes 为 0 时使用
const aiTaskDefaultTimeoutMinutes = 10

// aiTaskMaxConcurrent 全局并发上限：同时运行的 claude -p 子进程数。
// 单进程常驻 ~150-300MB，3 并发约 ~1GB，兼顾本地资源与 API 限流。
// 先硬编码常量，后续随 schema v2 入配置文件可调。
const aiTaskMaxConcurrent = 3

// AiFunctionService AI 功能服务：功能项配置持久化 + claude headless 子进程执行器。
// 执行模型：每段一次 claude -p 调用（--output-format stream-json 流式回显），
// 多段编排由前端驱动——段完成后拿 session_id，下一段 RunStage 传 resumeSessionID 续会话。
// 并发控制：concurrencySem 限制同时运行的子进程数，超限任务排队等待（queued=true），
// 获取槽位后才创建执行 ctx（超时起算后移，排队等待不侵蚀执行预算）。
type AiFunctionService struct {
	ctx            context.Context
	configPath     string
	mu             sync.Mutex
	tasks          map[string]*aiTaskRuntime
	concurrencySem chan struct{} // 全局并发信号量，缓冲 = aiTaskMaxConcurrent
}

// aiTaskRuntime 一个运行中/已完成任务的内部状态
type aiTaskRuntime struct {
	id         string
	functionID string
	prompt     string
	sessionID  string
	cmd        *exec.Cmd
	ctx        context.Context
	cancel     context.CancelFunc
	running    bool
	canceled   bool
	startedAt  time.Time
	timeoutMin int
	output     strings.Builder
	errText    string
	metrics    *model.AiTaskMetrics // result 事件计量（P0-2），nil 表示无计量
	queued     bool                 // 排队中：等待并发槽位，未起进程（P0-3）
	queuedAt   time.Time            // 入队时间（P0-3），供排队时长展示
	queueCancel chan struct{}       // 排队取消信号（P0-3）：close 后唤醒 RunStage 的 select
}

// NewAiFunctionService 创建 AI 功能服务
func NewAiFunctionService(ctx context.Context, configPath string) *AiFunctionService {
	return &AiFunctionService{
		ctx:            ctx,
		configPath:     configPath,
		tasks:          make(map[string]*aiTaskRuntime),
		concurrencySem: make(chan struct{}, aiTaskMaxConcurrent),
	}
}

// ===== 配置持久化 =====

// LoadAiFunctions 加载功能项配置；文件不存在或加载结果为空数组时，
// 写入并返回默认四项（首批功能）。空数组自愈：配置界面保存过空数组会导致菜单空白。
func (s *AiFunctionService) LoadAiFunctions() ([]*model.AiFunction, error) {
	var funcs []*model.AiFunction
	if util.FileExists(s.configPath) {
		if err := util.LoadJSON(s.configPath, &funcs); err != nil {
			return nil, fmt.Errorf("加载 AI 功能配置失败: %w", err)
		}
	}
	if len(funcs) == 0 {
		defaults := defaultAiFunctions()
		if err := util.SaveJSON(s.configPath, defaults); err != nil {
			return nil, fmt.Errorf("写入默认 AI 功能配置失败: %w", err)
		}
		return defaults, nil
	}
	return funcs, nil
}

// SaveAiFunctions 保存功能项配置
func (s *AiFunctionService) SaveAiFunctions(funcs []*model.AiFunction) error {
	return util.SaveJSON(s.configPath, funcs)
}

// ===== 任务执行 =====

// RunStage 执行一段：组装 claude 命令、起子进程、异步流式推送输出。
// resumeSessionID 非空时以 --resume 续会话（多段编排的后续段）。
// 返回任务 id（用于事件流对号与取消），进程启动失败同步报错。
//
// 并发控制：先注册排队态 task（queued=true）入 map 并 emit ai-task:queued，
// select 等待 concurrencySem 槽位（响应 s.ctx 关闭与排队取消）；
// 获取槽位后才 WithTimeout 创建执行 ctx——超时起算后移，排队等待不侵蚀执行预算。
// 排队期间用户取消：从 map 删除并 emit done（canceled），不启动进程。
func (s *AiFunctionService) RunStage(functionID, prompt, resumeSessionID string) (string, error) {
	fn, err := s.loadFunction(functionID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("prompt 不能为空")
	}

	taskID := fmt.Sprintf("aitask-%d", time.Now().UnixNano())
	task := &aiTaskRuntime{
		id:          taskID,
		functionID:  functionID,
		prompt:      prompt,
		queued:      true,
		queuedAt:    time.Now(),
		queueCancel: make(chan struct{}),
	}
	s.mu.Lock()
	s.tasks[taskID] = task
	s.mu.Unlock()
	s.emit("ai-task:queued", map[string]any{"taskId": taskID})

	// 排队等待槽位；三路唤醒：
	//   - concurrencySem 槽位可用 → 转执行
	//   - queueCancel close → 排队取消（CancelAiTask 触发），不启动进程
	//   - s.ctx.Done → app 关闭
	select {
	case s.concurrencySem <- struct{}{}:
		// 获取到槽位，继续
	case <-task.queueCancel:
		// 排队取消：select 未成功写入 sem，无需归还槽位；清理 map
		s.mu.Lock()
		delete(s.tasks, taskID)
		s.mu.Unlock()
		return "", fmt.Errorf("任务已取消（排队中）")
	case <-s.ctx.Done():
		// app 关闭：select 未成功写入 sem，无需归还槽位；清理 map
		s.mu.Lock()
		delete(s.tasks, taskID)
		s.mu.Unlock()
		return "", fmt.Errorf("应用关闭，任务未启动")
	}

	// 排队期间被取消：不启动进程，清理 map 与槽位
	s.mu.Lock()
	if task.canceled {
		delete(s.tasks, taskID)
		s.mu.Unlock()
		<-s.concurrencySem // 归还槽位
		return "", fmt.Errorf("任务已取消（排队中）")
	}
	task.queued = false
	task.startedAt = time.Now()
	s.mu.Unlock()
	s.emit("ai-task:started", map[string]any{"taskId": taskID})

	// 获取槽位后才创建执行 ctx（超时起算后移）
	timeout := fn.TimeoutMinutes
	if timeout <= 0 {
		timeout = aiTaskDefaultTimeoutMinutes
	}
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(timeout)*time.Minute)

	args := buildClaudeArgs(fn, prompt, resumeSessionID)
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = fn.Cwd
	util.HideCommandWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		s.mu.Lock()
		task.running = false
		task.errText = "创建输出管道失败"
		s.mu.Unlock()
		<-s.concurrencySem // 归还槽位
		return "", fmt.Errorf("创建输出管道失败: %w", err)
	}
	cmd.Stderr = cmd.Stdout // claude 的诊断信息合并进同一流

	if err := cmd.Start(); err != nil {
		cancel()
		s.mu.Lock()
		task.running = false
		task.errText = "启动 claude 失败"
		s.mu.Unlock()
		<-s.concurrencySem // 归还槽位
		return "", fmt.Errorf("启动 claude 失败（请确认已安装并在 PATH 中）: %w", err)
	}

	task.cmd = cmd
	task.ctx = ctx
	task.cancel = cancel
	task.running = true
	task.timeoutMin = timeout

	go s.pumpOutput(task, stdout)
	return taskID, nil
}

// CancelAiTask 取消任务，区分两态：
//   - 排队中（queued=true，无进程）：从 map 删除，emit ai-task:done（canceled），
//     让出排队位（RunStage 的 select 醒来后走 canceled 分支自行清理，此处先 emit done 通知前端）
//   - 运行中（有进程）：标记 canceled 并杀整个子进程树（claude 可能再 spawn python MCP 子进程），
//     pumpOutput 末尾检测 canceled 构造 done 事件
func (s *AiFunctionService) CancelAiTask(taskID string) bool {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return false
	}
	task.canceled = true
	queued := task.queued
	s.mu.Unlock()

	if queued {
		// 排队中：无进程可杀。close queueCancel 唤醒 RunStage 的 select，
		// 由 select 的 queueCancel 分支 delete map（避免与 select 竞态重复删）。
		// 竞态保护：若 select 恰好命中 sem 分支（获取槽位），它会在持锁后检测
		// task.canceled 走归还槽位分支——此处 close 也能让后续不再阻塞。
		close(task.queueCancel)
		s.emit("ai-task:done", model.AiTaskRunResult{
			TaskID:   taskID,
			Canceled: true,
			Error:    "已取消（排队中）",
		})
		return true
	}
	killProcessTree(task.cmd)
	return true
}

// GetAiTaskState 查询任务状态（前端恢复面板用）
func (s *AiFunctionService) GetAiTaskState(taskID string) *model.AiTaskState {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return nil
	}
	return &model.AiTaskState{
		TaskID:     task.id,
		FunctionID: task.functionID,
		Running:    task.running,
		Queued:     task.queued,
		SessionID:  task.sessionID,
		Prompt:     task.prompt,
		Output:     task.output.String(),
		Error:      task.errText,
		StartedAt:  task.startedAt.UnixMilli(),
		Metrics:    task.metrics,
	}
}

// GetConcurrencyStatus 统计当前并发占用（运行中 + 排队中 + 上限），供前端标题栏展示「N/M」。
func (s *AiFunctionService) GetConcurrencyStatus() model.AiConcurrencyStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	var running, queued int
	for _, t := range s.tasks {
		if t.queued {
			queued++
		} else if t.running {
			running++
		}
	}
	return model.AiConcurrencyStatus{
		Running: running,
		Queued:  queued,
		Max:     cap(s.concurrencySem),
	}
}

// RemoveAiTask 清理已完成/已取消任务的后端 runtime（前端 Tab 关闭时调用）。
// 运行中或排队中的任务不允许清理（前端应禁止关闭运行中 Tab，排队任务先 CancelAiTask）。
// 返回 false 表示任务不存在或仍在运行/排队中，不可清理。
func (s *AiFunctionService) RemoveAiTask(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return false
	}
	if task.running || task.queued {
		return false
	}
	delete(s.tasks, taskID)
	return true
}

// CloseAll 应用退出时清理全部运行中任务
func (s *AiFunctionService) CloseAll() {
	s.mu.Lock()
	tasks := make([]*aiTaskRuntime, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	s.mu.Unlock()
	for _, t := range tasks {
		killProcessTree(t.cmd)
	}
}

// loadFunction 按 id 取功能项配置
func (s *AiFunctionService) loadFunction(functionID string) (*model.AiFunction, error) {
	funcs, err := s.LoadAiFunctions()
	if err != nil {
		return nil, err
	}
	for _, fn := range funcs {
		if fn.ID == functionID {
			return fn, nil
		}
	}
	return nil, fmt.Errorf("AI 功能 %s 不存在", functionID)
}

// buildClaudeArgs 组装 claude headless 命令参数（纯函数，便于单测）。
// 参数顺序：-p <prompt> [--resume <sid>] --output-format stream-json --verbose
// [--permission-mode <mode>] [--add-dir <dir>]... [--mcp-config <json>] [--settings <json>]
func buildClaudeArgs(fn *model.AiFunction, prompt, resumeSessionID string) []string {
	args := []string{"-p", prompt}
	if resumeSessionID != "" {
		args = append(args, "--resume", resumeSessionID)
	}
	args = append(args, "--output-format", "stream-json", "--verbose")

	mode := fn.PermissionMode
	if mode == "" {
		mode = "bypassPermissions" // 菜单场景无交互终端，默认放行权限（功能项可覆盖）
	}
	args = append(args, "--permission-mode", mode)

	for _, dir := range fn.AddDirs {
		if dir != "" {
			args = append(args, "--add-dir", dir)
		}
	}

	if fn.Mcp != nil && len(fn.Mcp.Servers) > 0 {
		// stdio server 的 env 走 expandEnvRef（与功能项 env 一致，token 等凭证不落配置），
		// 拷贝 mcp 副本展开避免修改原配置对象
		mcpCopy := &model.AiMcpConfig{Servers: make(map[string]model.AiMcpServer, len(fn.Mcp.Servers))}
		for name, srv := range fn.Mcp.Servers {
			if srv.Type == "stdio" && len(srv.Env) > 0 {
				expanded := make(map[string]string, len(srv.Env))
				for k, v := range srv.Env {
					expanded[k] = expandEnvRef(v)
				}
				srv.Env = expanded
			}
			mcpCopy.Servers[name] = srv
		}
		if raw, err := json.Marshal(mcpCopy); err == nil {
			args = append(args, "--mcp-config", string(raw))
		}
	}

	if len(fn.Env) > 0 {
		envVal := make(map[string]string, len(fn.Env))
		for k, v := range fn.Env {
			envVal[k] = expandEnvRef(v)
		}
		if raw, err := json.Marshal(map[string]any{"env": envVal}); err == nil {
			args = append(args, "--settings", string(raw))
		}
	}
	return args
}

// expandEnvRef 展开 "$ENV:VAR" 形式的环境变量引用；非该前缀的原样返回。
// 用于 token 等凭证不落配置文件：值写 "$ENV:TENCENT_MEETING_TOKEN"，运行时取宿主环境。
func expandEnvRef(v string) string {
	if strings.HasPrefix(v, "$ENV:") {
		if val, ok := os.LookupEnv(strings.TrimPrefix(v, "$ENV:")); ok {
			return val
		}
	}
	return v
}

// renderPrompt 将模板中的 {{key}} 替换为参数值（form 类型参数用）
func renderPrompt(template string, params map[string]string) string {
	out := template
	for k, v := range params {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

// joinParamLines 将多行参数值整理为单行追加串：按换行拆分、去空白、滤空；
// 多值时每值用双引号包裹后以空格连接（含空格路径安全），单值保持原样不加引号。
// file 多选（前端以 \n join 多路径）与 text 单值共用此逻辑。
func joinParamLines(val string) string {
	rawLines := strings.Split(val, "\n")
	parts := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		l = strings.TrimSpace(l)
		if l != "" {
			parts = append(parts, l)
		}
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		quoted := make([]string, len(parts))
		for i, p := range parts {
			quoted[i] = `"` + p + `"`
		}
		return strings.Join(quoted, " ")
	}
}

// BuildStagePrompt 组装一段的最终 prompt（纯函数，便于单测）。
// spec 为该段的参数规格（主段=fn.Params，后续段=followUp.Input）；command 为主段的斜杠命令。
// 规则：
//   - spec 为 nil / type 为空 / "none"：有 PromptTemplate 则渲染之，否则用 command
//   - "file" / "text"：值经 joinParamLines 整理（多行引号包裹拼接）后追加到 command
//     或经 {{key}} 进 PromptTemplate（key 取 TextFieldKey，缺省 "file"/"text"）
//   - "form"：渲染 PromptTemplate，Required 字段缺失报错
func BuildStagePrompt(command string, spec *model.AiParamSpec, params map[string]string) (string, error) {
	if spec == nil || spec.Type == "" || spec.Type == "none" {
		if spec != nil && spec.PromptTemplate != "" {
			return renderPrompt(spec.PromptTemplate, params), nil
		}
		return command, nil
	}
	switch spec.Type {
	case "file", "text":
		key := spec.TextFieldKey
		if key == "" {
			key = spec.Type
		}
		joined := joinParamLines(params[key])
		if joined == "" {
			return "", fmt.Errorf("参数「%s」不能为空", spec.Label)
		}
		// 有模板则整理后的值经 {{key}} 进模板；无模板则追加到命令尾部
		if spec.PromptTemplate != "" {
			merged := make(map[string]string, len(params))
			for k, v := range params {
				merged[k] = v
			}
			merged[key] = joined
			return renderPrompt(spec.PromptTemplate, merged), nil
		}
		if command == "" {
			return joined, nil
		}
		return command + " " + joined, nil
	case "form":
		if spec.PromptTemplate == "" {
			return "", fmt.Errorf("form 类型参数缺少 PromptTemplate")
		}
		for _, f := range spec.Fields {
			if f.Required && strings.TrimSpace(params[f.Key]) == "" {
				return "", fmt.Errorf("表单字段「%s」不能为空", f.Label)
			}
		}
		return renderPrompt(spec.PromptTemplate, params), nil
	default:
		return "", fmt.Errorf("不支持的参数类型: %s", spec.Type)
	}
}

// BuildFollowUpPrompt 组装后续段 prompt（纯函数，便于单测）。
// 优先级：Input 自带 PromptTemplate（经 BuildStagePrompt 完整处理，含必填校验）
// > FollowUp.PromptTemplate 渲染 {{key}} 占位
// > 参数裸值空格拼接（模板为空或占位符未命中时的退化路径）。
//
// 背景：Input 为 type=text 且无模板时，BuildStagePrompt 返回参数裸值（非空），
// 不能以其非空判定「已组装完成」而跳过 FollowUp.PromptTemplate——后者承载
// 直接执行指令（如取消会议），跳过会导致模型只收到裸参数、停在原地提问。
func BuildFollowUpPrompt(followUp *model.AiFollowUp, params map[string]string) (string, error) {
	if followUp == nil {
		return "", fmt.Errorf("后续段定义缺失")
	}
	// 一级：Input 自带模板（字段化输入），交给 BuildStagePrompt 完整处理
	if followUp.Input != nil && followUp.Input.PromptTemplate != "" {
		return BuildStagePrompt("", followUp.Input, params)
	}
	// 二级：FollowUp.PromptTemplate 渲染占位；渲染结果非空且无残留 {{ 才采用，
	// 避免模板引用了 params 不存在的 key 时把裸占位符发给模型
	if rendered := strings.TrimSpace(renderPrompt(followUp.PromptTemplate, params)); rendered != "" && !strings.Contains(rendered, "{{") {
		return rendered, nil
	}
	// 三级：退化路径——params 非空值空格拼接
	vals := make([]string, 0, len(params))
	for _, v := range params {
		if v = strings.TrimSpace(v); v != "" {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return "", fmt.Errorf("后续段 prompt 组装失败：无有效参数")
	}
	return strings.Join(vals, " "), nil
}

// ===== stream-json 解析与输出泵 =====

// streamEvent claude --output-format stream-json 的单行事件（只取关心的字段）。
// result 事件携带的计量字段（usage/duration_ms/total_cost_usd/num_turns）随事件一起解析。
type streamEvent struct {
	Type         string            `json:"type"`
	Subtype      string            `json:"subtype"`
	SessionID    string            `json:"session_id"`
	Result       string            `json:"result"`
	Message      *assistantMessage `json:"message"`
	IsError      bool              `json:"is_error"`
	DurationMs   int64             `json:"duration_ms"`
	NumTurns     int               `json:"num_turns"`
	TotalCostUSD float64           `json:"total_cost_usd"`
	Usage        *streamUsage      `json:"usage"`
}

// streamUsage result 事件 usage 字段的 token 用量（仅取关心的四项）
type streamUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type assistantMessage struct {
	Role    string               `json:"role"`
	Content []messageContentPart `json:"content"`
}

type messageContentPart struct {
	Type string `json:"type"` // "text" / "tool_use" / ...
	Text string `json:"text"`
}

// parseStreamLine 解析一行 stream-json，返回 (文本增量, 是否为终态 result 事件, 会话 id, 计量摘要)。
// 非 JSON 行（如 stderr 串入的诊断文本）原样作为文本增量返回，不丢输出。
// metrics 仅在 result 事件且含计量字段时非 nil（旧版或字段缺失时为 nil，前端判空跳过）。
func parseStreamLine(line string) (text string, isResult bool, sessionID string, metrics *model.AiTaskMetrics) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false, "", nil
	}
	var ev streamEvent
	if err := json.Unmarshal([]byte(trimmed), &ev); err != nil {
		return line + "\n", false, "", nil
	}
	if ev.SessionID != "" {
		sessionID = ev.SessionID
	}
	if ev.Type == "assistant" && ev.Message != nil {
		var sb strings.Builder
		for _, part := range ev.Message.Content {
			if part.Type == "text" && part.Text != "" {
				sb.WriteString(part.Text)
			}
		}
		return sb.String(), false, sessionID, nil
	}
	if ev.Type == "result" {
		// result 事件携带计量：duration_ms/num_turns/total_cost_usd 与 usage。
		// 任一计量字段非零或 usage 非 nil 即构造 metrics；全零（异常 result）返回 nil
		if ev.DurationMs != 0 || ev.NumTurns != 0 || ev.TotalCostUSD != 0 || ev.Usage != nil {
			metrics = &model.AiTaskMetrics{
				DurationMs: ev.DurationMs,
				NumTurns:   ev.NumTurns,
				CostUSD:    ev.TotalCostUSD,
			}
			if ev.Usage != nil {
				metrics.Usage = &model.AiTaskUsage{
					InputTokens:              ev.Usage.InputTokens,
					OutputTokens:             ev.Usage.OutputTokens,
					CacheCreationInputTokens: ev.Usage.CacheCreationInputTokens,
					CacheReadInputTokens:     ev.Usage.CacheReadInputTokens,
				}
			}
		}
		return "", true, sessionID, metrics
	}
	return "", false, sessionID, nil
}

// pumpOutput 逐行读取 stdout，解析 stream-json 并推送前端事件，等待进程退出。
func (s *AiFunctionService) pumpOutput(task *aiTaskRuntime, stdout pipeReader) {
	// 信号量随进程退出释放（cmd.Wait 返回后函数返回，defer 执行）。
	// pumpOutput 被调用时 task 已获取槽位（RunStage 排队取消分支不进 pumpOutput）。
	defer func() { <-s.concurrencySem }()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024) // 单行上限 4MB（长 JSON 事件）

	for scanner.Scan() {
		text, isResult, sessionID, metrics := parseStreamLine(scanner.Text())
		// sessionID/output/metrics 写入须持锁：GetAiTaskState 在锁内读取同字段，
		// 无锁并发写 strings.Builder 可能 panic；emit 放锁外避免拖长持锁时间
		s.mu.Lock()
		if sessionID != "" {
			task.sessionID = sessionID
		}
		if text != "" {
			task.output.WriteString(text)
		}
		if isResult && metrics != nil {
			task.metrics = metrics
		}
		s.mu.Unlock()
		if text != "" {
			s.emit("ai-task:output", map[string]any{
				"taskId": task.id,
				"text":   text,
			})
		}
		if isResult {
			// result 事件后进程随即退出，继续读完剩余行
			continue
		}
	}

	waitErr := task.cmd.Wait()

	s.mu.Lock()
	task.running = false
	result := model.AiTaskRunResult{
		TaskID:    task.id,
		SessionID: task.sessionID,
		Output:    task.output.String(),
		Metrics:   task.metrics,
	}
	if task.canceled {
		result.Canceled = true
		result.Error = "已取消"
	} else if waitErr != nil {
		if task.ctx != nil && task.ctx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Sprintf("执行超时（上限 %d 分钟）", task.timeoutMin)
		} else {
			result.Error = waitErr.Error()
		}
	}
	if state := task.cmd.ProcessState; state != nil {
		result.ExitCode = state.ExitCode()
	}
	s.mu.Unlock()

	s.emit("ai-task:done", result)
}

// emit 推送 Wails 事件（测试环境 ctx 为 nil 时静默跳过）
func (s *AiFunctionService) emit(name string, data ...any) {
	if s.ctx == nil {
		return
	}
	runtime.EventsEmit(s.ctx, name, data...)
}

// pipeReader 抽象 stdout 管道（测试替换用）
type pipeReader interface {
	Read(p []byte) (int, error)
}

// ===== 进程树终止 =====

// killProcessTree 杀掉 cmd 对应进程及其全部子进程。
// Windows 下 claude 可能 spawn python（MCP）等子进程，单独 Kill 父进程会留孤儿。
func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := util.KillProcessTree(cmd.Process.Pid); err != nil {
		// 兜底：至少杀父进程
		_ = cmd.Process.Kill()
	}
}

// ===== 默认功能项（首批四个）=====

func defaultAiFunctions() []*model.AiFunction {
	return []*model.AiFunction{
		{
			ID:             "speech-doc",
			Name:           "文档转 HTML 发言稿",
			Description:    "选一个 Markdown/Word 文档，转换为可直接宣讲的 HTML 发言稿（agree-slides 插件）",
			Icon:           "Microphone",
			Command:        "/ab-office:agree-slides",
			Cwd:            `D:\workspace\workspace_ai\all_in_ai\workspace_claudcode\u51_ppt生成`,
			PermissionMode: "bypassPermissions",
			TimeoutMinutes: 15,
			Completion:     "preview",
			Params: &model.AiParamSpec{
				Type:       "file",
				Label:      "选择源文档",
				StartDir:   `D:\工作\Typora`,
				Extensions: []string{".md", ".docx", ".txt"},
			},
		},
		{
			ID:             "weekly-report",
			Name:           "生成周报",
			Description:    "生成 ABX5 产品迭代项目周报：先出草稿与亮点候选，面板确认后落盘终稿",
			Icon:           "Calendar",
			Command:        "/ab-weekly-report",
			Cwd:            `D:\workspace\workspace_ai\all_in_ai\workspace_claudcode\u63_项目管理`,
			PermissionMode: "bypassPermissions",
			TimeoutMinutes: 20,
			Completion:     "open_dir",
			FollowUps: []model.AiFollowUp{
				{
					ID:             "confirm-finalize",
					Label:          "确认亮点，落盘终稿",
					PromptTemplate: "亮点已确认，请按 skill 流程落盘周报终稿",
				},
			},
		},
		{
			ID:             "meeting-book",
			Name:           "预约腾讯会议",
			Description:    "填写主题、开始时间、时长，创建腾讯会议并复制会议号",
			Icon:           "VideoCamera",
			Command:        "/tencent-meeting-mcp",
			Cwd:            `D:\workspace\workspace_ai\all_in_ai\workspace_claudcode\u54_tencent`,
			PermissionMode: "bypassPermissions",
			TimeoutMinutes: 5,
			Completion:     "copy",
			Params: &model.AiParamSpec{
				Type:           "form",
				Label:          "会议信息",
				PromptTemplate: "/tencent-meeting-mcp 为我预约一场腾讯会议：主题「{{topic}}」，开始时间 {{startTime}}，时长 {{duration}} 小时。创建成功后把会议号、会议链接、入会密码（如有）完整列出。MCP 未配置时通过 python .claude/scripts/tm.py schedule_meeting 调用。",
				Fields: []model.AiFormField{
					{Key: "topic", Label: "会议主题", Type: "text", Required: true, Placeholder: "如：ABX5 迭代评审"},
					{Key: "startTime", Label: "开始时间", Type: "datetime", Required: true, Placeholder: "如：2026-09-10 15:00"},
					{Key: "duration", Label: "时长（小时）", Type: "number", Required: true, Placeholder: "如：1（可输 1.5）"},
				},
			},
		},
		{
			ID:             "meeting-list",
			Name:           "查看/取消腾讯会议",
			Description:    "列出已预约的腾讯会议；选中后确认取消",
			Icon:           "Clock",
			Command:        "/tencent-meeting-mcp",
			Cwd:            `D:\workspace\workspace_ai\all_in_ai\workspace_claudcode\u54_tencent`,
			PermissionMode: "bypassPermissions",
			TimeoutMinutes: 5,
			Completion:     "none",
			Params: &model.AiParamSpec{
				Type:           "none",
				PromptTemplate: "/tencent-meeting-mcp 查询我已预约的会议列表（MCP 未配置时通过 python .claude/scripts/tm.py get_user_meetings 调用）。以 markdown 表格输出，固定列顺序：会议主题|会议号|开始时间|结束时间|时长|入会链接|入会密码|状态。入会链接用 get_user_meetings 返回的 join_url，入会密码没有填「无」。表格之外不要输出其他说明文字。",
			},
			FollowUps: []model.AiFollowUp{
				{
					ID:    "cancel-meeting",
					Label: "取消会议",
					// 用户确认已由 WorkBench 面板弹窗完成，此段 headless 直跑：
					// 明示跳过确认等待，避免模型（叠加 skill 自身「取消前必须确认」规范）停下向用户提问。
					PromptTemplate: "/tencent-meeting-mcp 取消会议：{{meeting}}。用户已在应用界面明确确认取消该会议，无需再向用户确认、无需等待回复，跳过一切确认步骤，直接通过 python .claude/scripts/tm.py cancel_meeting 执行取消（如需 meeting_id 先用 get_user_meetings 或 get_meeting_by_code 转换），取消成功后明确告知结果。",
					Input: &model.AiParamSpec{
						Type:         "text",
						Label:        "要取消的会议（会议号或主题）",
						TextFieldKey: "meeting",
					},
				},
			},
		},
	}
}
