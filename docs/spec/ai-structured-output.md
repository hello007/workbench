# AI 结构化输出契约（--json-schema 方案）

> 跨层契约：AI 功能项结构化输出链路。新增需结构化输出的 AI skill（候选数组/问题清单/表单数据等）必知。

## 背景

WorkBench AI 功能（`AiFunctionService.RunStage`）起 claude 子进程走 `--output-format stream-json --verbose`，输出经 `ai-task:output` 流式推送 + `ai-task:done` 全量落盘。PR1 引入结构化输出需求（AI 提交信息生成要候选数组、AI 代码审查要问题清单 JSON），claude CLI 的 `--json-schema` flag 提供 tool use 强制结构化能力。

## 实测结论（2026-09-15，claude 2.1.158）

- `stream-json --verbose --json-schema '<schema>'` 组合通过：result 事件含 `structured_output` 字段，严格符合 schema
- `structured_output` 与自由文本 `result` **解耦**：自由文本带 markdown 包裹（如 `**7**`）不影响结构化输出正确性 —— 方案 A（prompt 约束 JSON + 后端容错解析）会被 markdown 包裹破坏，方案 C（`--json-schema`）不会
- stop_reason=`end_turn` 非 tool_use，structured_output 不依赖特定 stop_reason
- `--print` + `stream-json` 硬性要求 `--verbose`（`buildClaudeArgs` 已加）

## 跨层字段链路

```
AiFunction.OutputSchema (json.RawMessage, 配置层)
  ↓ buildClaudeArgs：OutputSchema 非 nil 追加 --json-schema <schema>
claude 子进程
  ↓ result 事件 structured_output 字段
streamEvent.StructuredOutput (json.RawMessage, service/ai_function.go)
  ↓ parseStreamLine 第 5 返回值
aiTaskRuntime.structuredOutput (service 缓存)
  ↓ pumpOutput 末尾构造 + GetAiTaskState 透传
AiTaskRunResult.StructuredOutput (done 事件 payload)
AiTaskState.StructuredOutput (GetAiTaskState 返回)
  ↓ Wails 事件/方法 IPC
frontend/wailsjs/go/models.ts: AiTaskState.structuredOutput (any)
wails-mock-defaults.js: done payload structuredOutput (测试单一数据源)
```

## 新增结构化 AI skill 步骤

1. **model**：`AiFunction.OutputSchema` 字段已存在（json.RawMessage，`omitempty`），配 JSON schema 字面量
2. **service**：`buildClaudeArgs` 自动追加 `--json-schema`（OutputSchema 非 nil），`parseStreamLine` 自动提取 `structured_output` —— **零新增 service 代码**
3. **wailsjs**：`AiFunction.outputSchema` / `AiTaskState.structuredOutput` 字段已同步（models.ts）；若新增 struct 字段须同步 `frontend/wailsjs/{App.js,App.d.ts,models.ts}` 三处（见 [cross-layer-contracts.md](cross-layer-contracts.md)）
4. **前端消费**：`AiFunctionPanel.vue` 的 `onDone` 读 `result.structuredOutput` 渲染结构化面板（按功能定制：候选列表 / 问题清单 / 表格）；**PR1 只搭链路不消费，消费在 PR2/PR3**
5. **测试**：`wails-mock-defaults.js` 的 done payload 加 `structuredOutput` 字段（单一数据源，vitest/E2E 共用）
6. **seed skill 合并白名单**：新增 skill 须在 `mergeMissingSeedSkills`（service/ai_function.go）的白名单数组加入其 ID，否则 PR1 前已建 `data/ai_functions.json` 的老用户配置不含该 skill，`RunAiFunction` 报「AI 功能 <id> 不存在」。详见下方「seed skill 合并白名单」节。

## seed skill 合并白名单

### 背景

`defaultAiFunctions()` 内置 skill 配置（speech-doc / weekly-report / meeting-book / meeting-list / commit-message / code-review）。`LoadAiFunctions` 对已存在配置走 `migrateFunctions`（补字段）+ `validateFunctions`（校验），但**不按 ID 合并新 seed skill**。PR1 前已建 `data/ai_functions.json`（仅含 4 项原 skill）的用户升级后，`commit-message` / `code-review` 缺失，`RunAiFunction` 报「AI 功能 commit-message 不存在」。

### 契约

- 新增 seed skill **必须**加入 `mergeMissingSeedSkills` 的白名单数组，否则老用户配置不合并
- 白名单仅含 epic 交付物：`["commit-message", "code-review"]`
- **旧 seed skill（speech-doc / weekly-report / meeting-book / meeting-list）不进白名单** —— 用户可能故意删除，不加回尊重用户删除决策
- 用户已存在同 ID 项（含自定义 Name/Params）**不覆盖**，尊重用户自定义
- seed 按 ID 索引 `map[string]*model.AiFunction` O(1) 查表，非线性扫 `defaultAiFunctions`
- 合并后 `changed=true`，`LoadAiFunctions` 落盘条件 `migrated || len(invalidIDs) > 0 || seedMerged` 触发落盘，下次加载直读

### 签名

```go
// mergeMissingSeedSkills 按 ID 合并白名单内缺失的 seed skill。
// 白名单 = epic 交付物（commit-message/code-review）。
// 用户已存同 ID 项不覆盖；旧 seed skill 不进白名单尊重删除决策。
// 返回合并后的 funcs 与是否发生合并。
func mergeMissingSeedSkills(funcs []*model.AiFunction) ([]*model.AiFunction, bool)
```

### 验证矩阵

| 条件 | 行为 | changed |
|---|---|---|
| 老配置（4 项原 skill，缺 epic skill） | 追加 commit-message/code-review | true |
| 全量配置（6 项） | 不变 | false |
| 用户删了 meeting-book（5 项含 epic skill） | meeting-book 不加回 | false |
| 用户自定义 commit-message（ID 同字段改） | 不覆盖 | false |
| 部分缺项（有 commit-message 缺 code-review） | 仅补 code-review | true |

### Wrong vs Correct

#### Wrong
```go
// 全量同步 defaultAiFunctions —— 用户故意删过的 skill 被加回
func mergeMissingSeedSkills(funcs []*model.AiFunction) ([]*model.AiFunction, bool) {
    defaults := defaultAiFunctions()
    // 遍历 defaults 全追加缺失项，无视用户删除决策
}
```

#### Correct
```go
// 白名单仅 epic 交付物，尊重用户删除/自定义
var seedMergeWhitelist = []string{"commit-message", "code-review"}

func mergeMissingSeedSkills(funcs []*model.AiFunction) ([]*model.AiFunction, bool) {
    existing := make(map[string]bool, len(funcs))
    for _, fn := range funcs {
        if fn != nil && fn.ID != "" {
            existing[fn.ID] = true
        }
    }
    seedByID := make(map[string]*model.AiFunction)
    for _, fn := range defaultAiFunctions() {
        seedByID[fn.ID] = fn
    }
    changed := false
    for _, id := range seedMergeWhitelist {
        if !existing[id] {
            if seed, ok := seedByID[id]; ok {
                funcs = append(funcs, seed)
                changed = true
            }
        }
    }
    return funcs, changed
}
```

### 测试

- `service/ai_function_seed_merge_test.go`：6 单测覆盖验证矩阵全 5 行 + 端到端集成（老 4 项配置经 `LoadAiFunctions` → 6 项 + 落盘 + 二次加载稳定）
- 断言点：合并后 funcs 含白名单 skill、用户自定义项原字段未变、meeting-book 未加回、`changed` 返回值正确

## 纯 prompt 模式（Command 空 + PromptTemplate 驱动）

- AI 功能项支持纯 prompt 模式：`Command` 空 + `Params.PromptTemplate` 驱动，`Cwd` 空继承父进程目录。
- `validateFunctions` 校验放宽为 Command 与 PromptTemplate **至少一非空**。
- 斜杠命令 skill（Command 非空）与纯 prompt skill（如 AI 提交信息生成/代码审查）两类并存。
- 纯 prompt skill 经 `BuildStagePrompt` form 模板 `{{key}}` 占位注入 diff/历史等参数。

## 模板占位符默认值（`{{名称:默认值}}`）

`renderPrompt` 解析式替换（正则 `\{\{([^{}\r\n]+)\}\}` + `ReplaceAllStringFunc`），与前端提示词面板同一套语法与未填语义（详见 [prompt-library.md](prompt-library.md) §2）：

- 名称与默认值以**首个半角冒号**分隔，两者去首尾空白；名称 ≤30 码点、默认值 ≤200 码点，超限整段按普通文本处理；
- 三级回退：参数值（`TrimSpace` 后非空，输出保留原内容）→ 该名称首个非空默认值 → 保留整段原样；
- `parsePromptPlaceholders` / `promptDefaultsByName` 为解析单点（同名首次为准）；
- **form Required 校验放行**：参数为空但模板中该 key 有非空默认值时视为已满足（`promptDefaultsByName` 命中）；
- **file / text 单字段模式不扩展默认值语义**：参数值仍必填，模板默认值不替代必填校验（回归测试 `TestBuildStagePrompt_FileRequiredIgnoresDefault`）；
- `BuildFollowUpPrompt` 二级「无残留 `{{`」判定沿用——默认值替换后产物无 `{{` 可正常采用。

## 独立生成链路（自然语言生成配置草稿，2026-11-24）

配置管理弹窗「✨ 自然语言创建」复用结构化输出能力，但走**独立同步链路**，与 RunStage 的任务链路互不影响：

- 入口：`App.GenerateAiFunctionDraft(req)` / `App.CancelAiFunctionDraft()` / `App.GetDiscoveredSkillsIn(dir)`，三者均为纯桌面能力，已加入 `main.go` 的 `RPCHiddenMethods`（会拉起本地 claude 子进程并扫描本地目录）。
- service：`AiFunctionService.GenerateAiFunctionDraft` 构造临时 `AiFunction`（固定 `OutputSchema = aiFunctionDraftSchema`、超时 2 分钟），复用 `claudeCommandResolver` / `buildClaudeArgs` / `parseStreamLine`，同步读到进程结束并取 result 事件的 `structured_output`。
- **安全加固（生成只产出 JSON）**：
  - 权限模式 `default`（非 `bypassPermissions`，集成测试已验证与 `--json-schema` 组合可用）+ 显式 `--disallowedTools Write,Edit,MultiEdit,NotebookEdit,Bash,BashOutput,KillBash,KillShell` 双保险，禁止写文件/执行命令（参数组装单点 `draftClaudeArgs`）。
  - 生成进程 cwd = 应用数据目录（`draftRunDir`，目录不存在则空继承父进程），**不使用用户所选 cwd**——用户目录只用于收集候选 skill 并写入 prompt。
  - prompt 拼接前对 skill `Name/Description/Command` 等外部元数据做单行折叠（换行折叠为空格）与 500 字符截断（`sanitizePromptField`），防注入破坏规则段落。
  - `exec.CommandContext` 定制 `cmd.Cancel = killProcessTree(cmd)`，取消/超时整树终止不留 MCP 子进程孤儿。
  - 输出读取：`scanner.Err()` 显式判定（单行超 4MB / 管道中断 → 先整树杀再 Wait，返回可读错误而非静默走到超时）；`cmd.Start()` 失败关闭两端管道；stderr 诊断行单行 500 字符 + 最多 20 行 + 汇总 2000 字符截断。
- **超时起算**：先取 `concurrencySem` 槽位、后 `context.WithTimeout`（与 RunStage 对齐），排队等待不侵蚀 2 分钟生成预算；排队阶段经基础 ctx 仍可取消。
- **隔离纪律**：不注册任务、不发 `ai-task:*` 事件、不写任务输出文件、不进历史；并发受全局 `concurrencySem` 约束。
- 取消面：`draftMu` 保护 `draftCancel` + `draftSeq` 代次；重入会取消上一轮，defer 按代次清理（第一轮不得清掉第二轮 cancel），取消/超时翻译为「生成已取消」/「生成超时」。
- 结果规范化（纯函数 `buildAiFunctionDraftResult`）：
  - 用户指定 skill 优先（未命中候选仍采用并告警）、模型自选 command 未命中候选仅告警、ID 冲突加 `-2`/`-3` 后缀、字段缺省回退默认值，最终过 `validateFunctions`。
  - **参数模板校验**：`form` 类型必须有非空 `promptTemplate`；有 command 无模板时降级 `type:"none"` 并告警，无 command 无模板则阻断报错。`file`/`text` 无模板仍可运行（BuildStagePrompt 会把参数拼到 command 尾部），不强制。
  - **skill + 模板一致性**：command 非空且模板非空时，模板未包含该 command 则自动前置 `<command> ` 并告警（运行期有模板只渲染模板、不会自动拼 command，否则「配置写着 skill、运行时不调用」）。
  - 未知 `permissionMode` / `completion` / 参数类型回退默认并告警；高级字段（env/mcp/addDirs/followUps/outputSchema）非空时逐项写入警告提示确认。
- **只读避让**：`App.collectAiFunctionIDs` 走 `AiFunctionService.ExistingFunctionIDsReadOnly`（直接读文件 + 内存迁移，不走 `LoadAiFunctions`），避免 seed 合并/损坏回种写盘，保持「生成不落盘」。
- 前端只渲染草稿（字段 + 警告），「填入配置」作为新增项回填现有表单，不落盘；关闭/取消中止在途生成。

## 约束

- `OutputSchema` 可选（nil 不加 `--json-schema` flag），兼容现有非结构化 skill 零回归
- `structured_output` 仅在 result 事件出现（流式过程无法增量渲染 JSON）—— 流式 `assistant` 文本增量照推前端展示进度，完成后渲染结构化面板
- `structured_output` 解析在后端（`parseStreamLine` 提取为 `json.RawMessage` 透传），前端只渲染不解析
- `structured_output` 为空（未配 OutputSchema 或 result 无该字段）时 `json.RawMessage` 为 nil，前端判空降级展示原始文本

## 关键文件

| 文件 | 符号 | 说明 |
|---|---|---|
| model/ai_function.go | `AiFunction.OutputSchema` | struct 字段 |
| model/ai_function.go | `AiTaskRunResult.StructuredOutput` / `AiTaskState.StructuredOutput` | struct 字段 |
| service/ai_function.go | `buildClaudeArgs`（追加 `--json-schema`） | 主任务参数组装 |
| service/ai_function.go | `streamEvent.StructuredOutput` / `parseStreamLine` 第 5 返回值 | 结构化输出解析 |
| service/ai_function.go | `mergeMissingSeedSkills`（seed skill 合并白名单） | 老配置补齐 |
| service/ai_function.go | `aiFunctionDraftSchema` / `GenerateAiFunctionDraft` / `CancelAiFunctionDraft` / `buildAiFunctionDraftResult` / `runDraftClaudeProcess` / `draftClaudeArgs` / `ExistingFunctionIDsReadOnly` / `sanitizePromptField` | 草稿生成与安全加固单点 |
| model/ai_function_draft.go | `AiFunctionDraftRequest` / `AiFunctionDraftResult` | struct |
| service/ai_function.go | `aiTaskRuntime.structuredOutput` / `pumpOutput` 缓存 / `GetAiTaskState` 透传 | 任务态透传 |
| frontend/wailsjs/go/models.ts | `AiFunction.outputSchema` / `AiTaskState.structuredOutput` | 字段同步 |
| frontend/src/test/wails-mock-defaults.js | done payload `structuredOutput` | 测试单一数据源 |

## 相关

- [cross-layer-contracts.md](cross-layer-contracts.md) — wailsjs 三处同步规则
- [logging-and-errors.md](logging-and-errors.md) — AI 功能错误链路
- research/claude-cli-structured-json.md — 三方案对比 + 实测证据
