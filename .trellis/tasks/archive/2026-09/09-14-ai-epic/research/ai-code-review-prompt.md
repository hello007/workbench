# Research: AI 代码审查 prompt 工程最佳实践

- **Query**: 研究「AI 代码审查 prompt 工程最佳实践」，映射到 WorkBench（Go+Wails+Vue3，claude CLI 子进程，stream-json 事件流）的 PR3 代码审查功能
- **Scope**: mixed（内部代码实证 + 外部业界实践）
- **Date**: 2026-09-14
- **关联 PRD**: `.trellis/tasks/09-14-ai-epic/prd.md`（PR3：AI 代码审查）

## 摘要

本报告调研 GitHub Copilot code review、CodeRabbit（含早期开源 `ai-pr-reviewer`）、Cursor、开源 AI review action 四类可比工具的 prompt 设计模式，提炼六项通用约定（diff 粒度按文件拆分、上下文窗口截断并提示剩余量、JSON schema 三层约束、grounding 减少幻觉、五维分级、项目规范注入），逐项映射到本仓库已实证的 AI 执行链路（`service/ai_function.go` 的 `buildClaudeArgs` / `BuildStagePrompt` / `parseStreamLine`），并给出三个可行的 prompt 设计方案（含完整 JSON schema 草案与可复用 prompt 模板）。

**核心结论前置**：MVP 推荐方案 A（单次全量 diff + prompt 约束 JSON + 后端 `extractReviewJson` 预解析，类比现有 `extractTable`），复用现有 stream-json 链路零改动；大 diff 进阶方案 B（按文件拆分 + 聚合，CodeRabbit 模式）；最稳但最重方案 C（claude tool use 结构化输出，需 MCP server）。

---

## 一、可比工具调研

### 1.1 工具对比总览

| 维度 | GitHub Copilot code review | CodeRabbit（含开源 ai-pr-reviewer） | Cursor | 开源 AI review action（ai-pr-reviewer 等） |
|---|---|---|---|---|
| 形态 | IDE 内联 + PR 集成 | PR bot（GitHub/GitLab） | IDE 内 AI | GitHub Action |
| 输入粒度 | 按文件/hunk | 按文件（file-by-file） | 当前文件/选区 | 按文件（per-file） |
| 上下文窗口 | 模型原生（GPT-4o/Claude） | 模型原生 + 分文件规避超窗 | 模型原生 | 模型原生 + 分文件 |
| 输出形态 | inline comment（按 severity） | walkthrough + line-by-line comments | inline 建议 | PR comment（JSON 解析后渲染） |
| 结构化约束 | 内部 tool use | prompt 约束 JSON + 后处理 | 内部结构化 | prompt 约束 JSON |
| 审查维度 | bug/security/performance/quality | bug/security/performance/style/最佳实践 | bug/改进/可读性 | bug/security/style |
| 分级 | critical/warning/suggestion | critical/warning/ info | high/medium/low | high/medium/low |
| 减少幻觉 | 引用行号 + 仅基于 diff | 引用行号 + 「无问题不强找」 | 选区 grounding | 引用行号 + 空数组兜底 |
| 项目规范注入 | 仓库 `.github/copilot-instructions.md` | 仓库 `.coderabbit.yaml` 配置 | `.cursorrules` | action 配置文件 |

### 1.2 各工具 prompt 设计模式要点

#### GitHub Copilot code review

- **形态**：IDE 内「Copilot Review」按钮 + GitHub PR 的 Copilot review。审查结果以 inline comment 形式落在具体行。
- **维度**：官方文档明确四类——possible bugs、security issues、performance、code quality（含可读性、命名）。
- **分级**：critical（必须修复）/ warning（建议修复）/ suggestion（可选改进）。
- **grounding**：每条建议绑定具体文件行；审查仅针对变更行（diff），不审查未变更代码。
- **规范注入**：读取仓库 `.github/copilot-instructions.md`（2024 年起官方支持的自定义指令文件）作为审查依据。
- **prompt 不公开**：为内部系统 prompt，但其行为约束由官方文档描述。本报告据此描述设计模式，非逐字引用。

#### CodeRabbit（含早期开源 `ai-pr-reviewer`）

- **形态**：PR bot，两阶段输出——先「walkthrough」（整体变更摘要，自然语言），再「line-by-line comments」（逐行问题清单）。
- **粒度策略**：**按文件拆分**（file-by-file），每个文件独立调用 LLM。这是其规避大 diff 上下文爆炸的核心设计。
- **结构化约束**：早期开源版（`coderabbitai/ai-pr-reviewer`，后商业化）prompt 要求 LLM 返回 JSON array of findings，每项含 `type` / `severity` / `file` / `line` / `description` / `suggestion`；后端解析 JSON 后渲染为 PR comment。
- **减少幻觉**：prompt 明确「如果代码正确不要强行找问题」「只基于提供的 diff」「引用具体行号」。
- **规范注入**：仓库 `.coderabbit.yaml` 可配置审查规则、学习路径、ignore 规则。
- **分级**：critical / warning / info（与 WalkBench PRD 设计一致）。

#### Cursor

- **形态**：IDE 内 AI，核心是补全与对话，「Review」非其主推能力但存在（选中代码后请求审查）。
- **粒度**：当前文件或选区，上下文窗口由模型决定（Cursor 调用 Claude/GPT）。
- **维度**：bug 风险、可读性、改进建议，偏向「可操作的重构建议」而非严格分级。
- **规范注入**：`.cursorrules`（现为 `.cursor/rules/*.mdc`）注入项目规范。
- **结构化**：内部结构化，用户侧表现为 inline diff 建议。

#### 开源 AI review action（`ai-pr-reviewer` / `pr-reviewer` 等）

- **形态**：GitHub Action，PR 触发后拉取 diff，调 LLM，结果以 PR comment 发布。
- **粒度**：**按文件拆分**（per-file review），逐文件调 LLM，规避 token 上限。
- **结构化**：prompt 要求输出 JSON array，字段含 `type` / `severity` / `line` / `description` / `suggestion`；后端解析 JSON，失败时降级为纯文本 comment。
- **减少幻觉**：prompt 含「no issues → return empty array」「cite line numbers」「only review provided diff」。
- **分级**：high / medium / low（或 critical / warning / info）。
- **局限**：早期开源版 prompt 在仓库 `prompts/` 目录，但仓库经商业化迁移后部分路径失效；本报告描述其公开过的设计模式。

> **信息来源声明**：上述外部工具的 prompt 原文未逐字引用（部分仓库经商业化迁移后 raw 路径 404，无 token 访问 GitHub API 被限流）。描述基于官方文档与业界公开实践（技术博客、会议分享、早期开源 README）。设计模式层面的描述是稳定的工程共识，可放心参考；若需逐字 prompt 原文，建议实现期由人工核验对应仓库最新版本。

---

## 二、六项调研重点逐项分析

### 2.1 diff 粒度控制：整手 diff vs 按文件 vs 按 hunk

| 策略 | 优势 | 劣势 | 适用场景 |
|---|---|---|---|
| **整手 diff 一次喂** | 跨文件关联完整（能发现「A 文件改了接口，B 文件调用未同步」）；调用次数少、成本低 | 大 diff 易超上下文窗口；单次输出过长导致 LLM 注意力衰减、后半段 diff 审查质量下降 | 小型 PR（< 5 文件、< 10K 行 diff） |
| **按文件拆分**（CodeRabbit 模式） | 单文件上下文聚焦、审查深度一致；可并行；超窗风险低；单文件失败不阻塞其他 | 跨文件关联丢失（接口/调用不同步问题难发现）；调用次数 = 文件数，成本与延迟上升 | 中大型 PR（多文件、单文件 diff 适中） |
| **按 hunk 拆分** | 上下文最局部、token 最省 | 跨函数/跨 hunk 上下文严重丢失（函数签名改了但 hunk 只含实现体时无法判断）；调用次数爆炸；碎片化导致重复审查 | 极大单文件（如生成代码、锁文件除外后的超大重构） |

**业界共识**：**按文件拆分是主流**（CodeRabbit、ai-pr-reviewer 均如此），兼顾上下文聚焦与超窗规避。整手 diff 仅适合小 PR；hunk 拆分过细，业界少用。

**本仓库映射**：
- 现有 `GetRangeDiff(path, baseSHA, headSHA)`（`app_git.go:630`）返回全文件 unified diff，天然支持「整手 diff」路径（方案 A）。
- 按文件拆分需：`GetLocalChanges` 筛文件列表 + 逐文件 `GetDiff(repoPath, file)`（`git.go:669`）聚合，每文件独立 `RunStage` 调用，后端聚合 findings（方案 B）。
- `BuildStagePrompt` 的 form 类型 `PromptTemplate` 支持 `{{file}}` / `{{diff}}` 占位符（PRD 已确认），按文件拆分时模板复用零改动。

### 2.2 上下文窗口控制：超长 diff 截断策略

claude CLI 上下文窗口 200K tokens（约 800K 字节，中文约 1.2 字/token，故约 150-200 万中文字符）。但实际单次审查需预留输出空间（findings JSON 可能数 KB），且 diff 越长注意力越衰减，**不建议逼近上限**。

**截断策略对比**：

| 维度 | 按字节截断 | 按行数截断 | 按文件数截断 |
|---|---|---|---|
| 可控性 | 精确（直接对应 token 估算） | 直观（diff 以行为单位） | 粗粒度 |
| 截断点合理性 | 可能截断在行中间 | 截断在行边界，diff 语义完整 | 整文件取舍，无半截 hunk |
| 剩余量提示 | 字节数 | 行数 | 文件数 |
| 推荐 | 配合 token 估算做总上限 | **主推**（diff 天然按行） | 配合按文件拆分用 |

**业界共识截断阈值**：
- 单次调用 diff 输入控制在 **50K tokens 以内**（约 200K 字节 / 50K 行以内），预留输出与注意力余量。
- 超阈值时**按文件拆分**（方案 B），单文件 diff 仍超阈值则按 hunk 截断并标注。
- **截断后必须提示模型剩余量**，避免模型误以为 diff 完整而产生「审查不存在的代码」幻觉。

**截断后剩余量提示格式**（业界常用）：
```
注意：本次 diff 已截断。完整 diff 共 N 个文件 / M 行，本次仅提供前 K 个文件 / L 行，
剩余 R 个文件未纳入审查。请仅审查以下提供的 diff 内容，不要推测未提供部分的代码。
```

**本仓库映射**：
- 截断逻辑应独立为 util helper（PRD 已明确「截断逻辑独立 util helper 不污染 AI service」），建议落 `util/diff_truncate.go`（或 `service/diff_aggregator.go`），输入 `[]model.FileChange` + 阈值，输出截断后的 diff 文本 + `TruncationInfo{TotalFiles, IncludedFiles, TotalLines, IncludedLines, Truncated bool}`。
- `TruncationInfo` 渲染为提示文本，经 `{{truncationNote}}` 占位符注入 prompt（`BuildStagePrompt` form 类型天然支持）。
- 阈值常量化：`MaxDiffBytes = 200_000`（约 50K tokens）/ `MaxDiffFiles = 20` / `MaxDiffLines = 50_000`，可调。

### 2.3 输出 JSON schema 约束：三种方式对比

| 方式 | 原理 | 稳定性 | 实现成本 | claude CLI 支持 |
|---|---|---|---|---|
| **prompt 约束** | prompt 中描述 schema + 要求「仅输出 JSON」 | 中（LLM 可能加 markdown 围栏、多余文字、字段缺失） | 低（纯 prompt） | 完全支持 |
| **后端解析容错** | prompt 约束 + 后端从输出文本提取 JSON（正则剥围栏、修复常见错误） | 中高（容错后多数可解析） | 低中（类比现有 `extractTable`） | 完全支持 |
| **结构化输出工具**（tool use / JSON mode） | 模型调用 `submit_review` tool，参数即 schema；或原生 JSON mode 强制合法 JSON | 高（schema 强制校验，零格式错误） | 高（需 MCP server 或改 CLI 参数） | claude CLI 无原生 JSON mode；tool use 需 MCP server |

**业界共识**：
- OpenAI 有原生 `response_format: {type: "json_object"}` / `json_schema`，强制合法 JSON。**claude CLI 无原生 JSON mode**（`--output-format json` 只控制 CLI 输出封装，不强制 result 内容是合法 JSON）。
- 因此 claude CLI 路径下，**prompt 约束 + 后端解析容错**是性价比最高的方案；tool use 最稳但最重。
- 后端解析容错的关键：从输出文本中提取 JSON（处理 ```json 围栏、前后多余文字、尾随逗号等），类比本仓库现有 `extractTable`（`ai_function.go:1062`，从输出预解析 markdown 表格）。

**本仓库映射（关键实证）**：
- `buildClaudeArgs`（`ai_function.go:792`）硬编码 `--output-format stream-json --verbose`，`parseStreamLine`（`ai_function.go:1012`）在 `type=assistant` 时拼接 `message.content[].text` 块为输出文本，`type=result` 时只取 metrics 不取 result 文本。
- **因此模型输出的 JSON 会出现在 assistant text 增量里，被拼接进 `AiTaskRunResult.Output`**。前端 / 后端从 Output 全量文本（落盘 `data/ai_task_output/<id>.txt`，经 `GetAiTaskOutput` 读）提取 JSON。
- `extractTable`（`ai_function.go:1062`）已是「从输出文本预解析结构化数据供前端渲染」的先例。**PR3 可类比实现 `extractReviewJson(output string) (*ReviewResult, error)`**，逻辑：定位首个 `{` 到末尾 `}` 平衡的子串，`json.Unmarshal` 容错（剥 ```json 围栏、去尾随逗号），失败降级为空 findings + 原文展示。
- **方案 A 零链路改动**：复用 stream-json + 新增 `extractReviewJson` 预解析，与现有 `extractTable` 模式完全对齐。

### 2.4 减少幻觉：grounding 与空数组兜底

幻觉是 AI 代码审查最大痛点——模型编造 diff 中不存在的代码、引用不存在的行号、强行找问题。业界共识的 prompt 技巧：

| 技巧 | 作用 | prompt 写法 |
|---|---|---|
| **引用具体 diff 行号** | 强制 grounding，便于后端校验行号合法性 | 「每个 finding 必须给出 startLine/endLine，取自提供的 diff 行号」 |
| **标注置信度** | 区分确信问题与推测，降低低置信噪声 | 「confidence: high=确信 / medium=需人工确认 / low=推测」 |
| **空数组兜底** | 避免强行找问题 | 「若某维度无问题不要编造；无任何问题时返回空 findings 数组」 |
| **禁止编造代码** | 防止审查 diff 外代码 | 「只审查 diff 中实际变更的代码，不得编造 diff 中不存在的代码或行号」 |
| **描述 + 建议分离** | 描述基于事实、建议基于推断，区分严谨性 | 「description 须说明问题原因与影响（基于 diff 事实）；suggestion 给修复方向（可基于推断）」 |
| **维度边界明确** | 防止类别混淆导致重复或遗漏 | 「bug=逻辑错误；security=注入越权；performance=复杂度；style=命名格式；improvement=重构可读性」 |
| **diff 与审查依据分离** | 明确输入边界 | 「审查依据仅为下文 `# 待审查 diff` 与 `# 项目规范` 两节，不得引用其他信息」 |

**本仓库映射**：
- 上述技巧全部以 prompt 文本实现，经 `BuildStagePrompt` 的 `PromptTemplate` 注入，零代码改动。
- 后端可加校验：`extractReviewJson` 后校验每条 finding 的 `file` 在 diff 文件列表内、`startLine` 在合理范围，过滤掉明显编造的 finding（进阶，MVP 可不做）。

### 2.5 审查维度设计：五维分级标准

PRD 已定五维度：bug 风险 / 编码规范 / 安全漏洞 / 性能问题 / 改进建议，三级别：critical / warning / info。与业界对齐情况：

| 维度 | WorkBench | Copilot | CodeRabbit | ai-pr-reviewer | 业界共识 |
|---|---|---|---|---|---|
| bug 风险 | ✓ | ✓ possible bugs | ✓ | ✓ | 必有 |
| 安全漏洞 | ✓ | ✓ security | ✓ | ✓ | 必有 |
| 性能问题 | ✓ | ✓ performance | ✓ | — | 常有 |
| 编码规范 | ✓ style | ✓ quality | ✓ style | ✓ style | 必有 |
| 改进建议 | ✓ improvement | ✓ suggestion | ✓ best practice | — | 常有 |

**分级标准建议**（综合业界，给出可操作判定）：

| 级别 | 判定标准 | 处理建议 |
|---|---|---|
| **critical** | 会导致 bug、数据丢失、崩溃、安全漏洞、生产事故 | 必须修复，阻断提交 |
| **warning** | 潜在风险、应修复但非阻断（边界条件、错误处理缺失、资源泄漏、不规范但可运行） | 建议修复 |
| **info** | 改进建议、规范提示、可读性优化（命名、注释、重构空间） | 可选改进 |

**类别 × 级别矩阵**（指导 prompt 与前端分组渲染）：

| 类别 \ 级别 | critical | warning | info |
|---|---|---|---|
| bug | 空指针、死锁、逻辑错误 | 边界条件、异常未处理 | — |
| security | SQL 注入、越权、敏感信息泄漏 | 弱校验、硬编码凭证 | — |
| performance | O(n²) 在热路径、N+1 查询 | 不必要的拷贝、锁粒度过大 | 微优化建议 |
| style | — | 命名违反规范、缺失错误处理 | 格式、注释 |
| improvement | — | 重复代码、耦合过重 | 可读性、可维护性 |

> 业界实践：style/improvement 类别通常不出现 critical 级别（规范问题不会导致生产事故）。prompt 应明确此约束，避免「命名不规范标 critical」的噪声。

**本仓库映射**：
- 维度与分级以 prompt 文本定义，JSON schema 的 `severity` / `category` 用 `enum` 约束合法值。
- 前端按 severity 分组渲染（critical 红色置顶 / warning 黄色 / info 灰色），按 category 标签筛选。
- PRD 的 `model.FileChange` 行号定位 + 「一键跳转 diff 行」需求：finding 的 `file` + `startLine` 直接驱动跳转，与 `GetDiff` 的 unified diff 行号对齐。

### 2.6 项目规范上下文注入

避免泛泛而谈的关键：注入项目实际规范作审查依据。

| 规范源 | WorkBench 可用 | 注入方式 |
|---|---|---|
| `CLAUDE.md` | ✓（项目根 + 用户全局） | 读取后截取关键章节注入 `{{projectRules}}` |
| `.editorconfig` | ✓ | 解析为规则文本注入 |
| lint 规则 | ✓（Go `golangci-lint`、前端 eslint） | 提取规则摘要注入 |
| `docs/开发规范.md` | ✓（项目已有） | 读取关键章节注入 |
| 历史 commit 风格 | ✓（PR2 已设计 few-shot） | 取最近 N 条 commit message 注入 |

**业界共识**：
- 规范注入须**精简**（不能整份 CLAUDE.md 灌入，否则挤占 diff 上下文）。CodeRabbit 用 `.coderabbit.yaml` 配置精炼规则；Copilot 用 `.github/copilot-instructions.md`（官方建议简短）。
- 规范须**可引用**：prompt 中明示「审查依据为下文 `# 项目规范` 章节」，让模型 ground 到具体规则。
- 规范须**与维度绑定**：如「命名规范 → style 类别」「错误处理规范 → bug 类别」，避免规范与维度脱节。

**本仓库映射**：
- 项目根 `CLAUDE.md` 已含丰富规范（日志用 `log/slog`、错误用 `AppError{Code,Message}`、跨层契约三处同步等），可提取关键规则注入。
- 但 `CLAUDE.md` 较长，**建议提取审查相关摘要**（编码规范、错误处理、日志、跨层契约要点），而非整份注入。可预设一份 `data/code_review_rules.md`（用户可编辑）作为审查依据单一数据源，经 `{{projectRules}}` 注入。
- 进阶：按文件语言选规则（Go 文件注入 Go 规范、Vue 文件注入前端规范），MVP 可全量注入。

---

## 三、本仓库约束映射总览

基于 `service/ai_function.go` 实证（行号已核），PR3 代码审查的技术约束如下：

| 约束点 | 实证位置 | 对 PR3 的影响 |
|---|---|---|
| claude CLI 调用 | `buildClaudeArgs` (`ai_function.go:792`) 硬编码 `-p <prompt> --output-format stream-json --verbose` | prompt 经 `-p` 传入；输出为 stream-json 事件流 |
| prompt 组装 | `BuildStagePrompt` (`ai_function.go:895`) form 类型 + `renderPrompt` (`ai_function.go:854`) `{{key}}` 占位符 | diff / 项目规范 / 截断提示经 `{{diff}}` / `{{projectRules}}` / `{{truncationNote}}` 注入，零新增 helper |
| 输出提取 | `parseStreamLine` (`ai_function.go:1012`) 取 `type=assistant` 的 `message.content[].text` 拼接 | 模型输出的 JSON 在 assistant text 增量里，拼接进 `Output` |
| 结构化预解析先例 | `extractTable` (`ai_function.go:1062`) 从输出文本预解析 markdown 表格 | PR3 可类比 `extractReviewJson` 从 Output 提取 JSON |
| 输出落盘 | `AiTaskRunResult.Output` 末尾预览 ~4KB + 全量落盘 `data/ai_task_output/<id>.txt` + `GetAiTaskOutput` 全量读 | 大审查结果（多 findings）全量可读，前端不丢数据 |
| 事件流推送 | `RunStage` 经 Wails 事件 `ai-task:output` / `ai-task:done` 推前端 | 审查进度实时推送；结果 done 后前端读全量解析 JSON |
| 并发控制 | `aiTaskMaxConcurrent` 全局并发槽位 + 排队 | 按文件拆分（方案 B）时多文件调用受并发槽位限制，需串行或受限并行 |
| skill 配置持久化 | `SaveAiFunctions` (`ai_function.go:181`) 落 schema v2 `data/ai_functions.json` | code-review skill 配置项（Command / Params / PromptTemplate）走现有持久化 |
| 跨层契约 | 新增 App 方法同步 `frontend/wailsjs/{App.js,App.d.ts}` + `models.ts` 三处 | 若新增 `ExtractReviewJson` 等 App 方法须同步三处；建议 `extractReviewJson` 留 service 层不经 App 暴露 |

**关键决策点**：
1. **输出格式**：复用 stream-json（零改动），不引入 `--output-format json`（json 模式不流式推送，丧失 Wails 事件流优势）。
2. **JSON 约束**：prompt 约束 + 后端 `extractReviewJson` 容错解析（类比 `extractTable`），不引入 tool use / MCP server（MVP 过重）。
3. **diff 粒度**：MVP 方案 A 整手 diff（小 PR 够用）+ 截断保护；进阶方案 B 按文件拆分（大 PR）。
4. **截断**：独立 util helper，阈值常量化，截断后提示剩余量经 `{{truncationNote}}` 注入。

---

## 四、Prompt 设计方案（3 个，含 JSON schema 草案）

### 4.0 统一 JSON schema 草案（三方案共用）

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "AiCodeReviewResult",
  "type": "object",
  "required": ["summary", "findings"],
  "properties": {
    "summary": {
      "type": "string",
      "description": "本次审查总体评价，1-3 句"
    },
    "stats": {
      "type": "object",
      "description": "各级别问题计数，供前端分组标题展示",
      "properties": {
        "critical": { "type": "integer", "minimum": 0 },
        "warning": { "type": "integer", "minimum": 0 },
        "info": { "type": "integer", "minimum": 0 }
      }
    },
    "findings": {
      "type": "array",
      "description": "问题清单，无问题时为空数组",
      "items": {
        "type": "object",
        "required": ["id", "file", "severity", "category", "description"],
        "properties": {
          "id": { "type": "string", "pattern": "^F\\d+$", "description": "问题编号，如 F1" },
          "file": { "type": "string", "description": "相对仓库根的文件路径，须在 diff 文件列表内" },
          "startLine": { "type": "integer", "minimum": 1, "description": "diff 中的起始行号" },
          "endLine": { "type": "integer", "minimum": 1, "description": "diff 中的结束行号，缺省等于 startLine" },
          "severity": { "type": "string", "enum": ["critical", "warning", "info"] },
          "category": { "type": "string", "enum": ["bug", "security", "performance", "style", "improvement"] },
          "title": { "type": "string", "description": "问题标题，一句话" },
          "description": { "type": "string", "description": "问题原因与影响，须引用具体 diff 行号" },
          "suggestion": { "type": "string", "description": "修复方向，可基于推断" },
          "confidence": { "type": "string", "enum": ["high", "medium", "low"], "description": "high=确信 / medium=需人工确认 / low=推测" },
          "diffRef": { "type": "string", "description": "引用的 diff 行号区间或 hunk 标记，如 42-45" }
        }
      }
    }
  }
}
```

**字段与 PRD 需求对齐**：`file` / `startLine` / `endLine` 驱动「一键跳转 diff 行」；`severity` 驱动「按级别分组渲染」；`category` 驱动「类别标签筛选」；`description` / `suggestion` 对应 PRD 的「描述/建议」。

### 4.1 方案 A：单次全量 diff + prompt 约束 JSON（MVP 推荐）

**适用**：中小 PR（diff < 200K 字节 / < 20 文件）。复用现有 stream-json 链路零改动。

**完整 PromptTemplate**（落 `data/ai_functions.json` 的 code-review skill 配置项，`{{...}}` 经 `BuildStagePrompt` 渲染）：

```
你是资深代码审查工程师。审查以下 git diff，输出结构化问题清单。

# 审查维度与分级

| 级别 | 含义 | 处理建议 |
|---|---|---|
| critical | 会导致 bug、数据丢失、崩溃、安全漏洞、生产事故 | 必须修复 |
| warning | 潜在风险或应修复（边界条件、错误处理缺失、资源泄漏） | 建议修复 |
| info | 改进建议或规范提示（命名、注释、可读性） | 可选改进 |

| 类别 | 范围 |
|---|---|
| bug | 逻辑错误、异常处理、边界条件、空指针、死锁 |
| security | 注入、越权、敏感信息泄漏、硬编码凭证 |
| performance | 算法复杂度、N+1 查询、不必要的拷贝、锁粒度 |
| style | 命名、格式、注释、规范 |
| improvement | 重构、可读性、可维护性、重复代码 |

级别与类别约束：style 与 improvement 类别不得标 critical（规范问题不会导致生产事故）。

# 严格规则（必须遵守）

1. 只审查 diff 中实际变更的代码，不审查未在 diff 中出现的代码。
2. 每个 finding 必须给出 file（相对仓库根路径）与 startLine/endLine（取自提供的 diff 行号）。
3. 不得编造 diff 中不存在的代码或行号。
4. 若某维度无问题，不要强行编造；无任何问题时返回空 findings 数组。
5. 标注 confidence：high=确信、medium=需人工确认、low=推测。
6. description 须说明问题原因与影响（基于 diff 事实，引用行号）；suggestion 给修复方向（可基于推断）。
7. 审查依据仅为下文「# 项目规范」与「# 待审查 diff」两节，不得引用其他信息。

# 项目规范

{{projectRules}}

# 待审查 diff

{{diff}}

{{truncationNote}}

# 输出格式

仅输出一个 JSON 对象，不要输出任何其他文字、解释或 markdown 围栏。结构如下：

{
  "summary": "总体评价，1-3 句",
  "stats": { "critical": 0, "warning": 0, "info": 0 },
  "findings": [
    {
      "id": "F1",
      "file": "src/xxx.go",
      "startLine": 42,
      "endLine": 45,
      "severity": "warning",
      "category": "bug",
      "title": "问题标题",
      "description": "问题描述（引用行号）",
      "suggestion": "修复建议",
      "confidence": "high",
      "diffRef": "42-45"
    }
  ]
}
```

**后端处理**：
- `extractReviewJson(output string) (*ReviewResult, error)`：定位首个 `{` 到末尾 `}` 平衡子串，剥 ```json 围栏，`json.Unmarshal`，失败降级空 findings + 原文展示。类比 `extractTable`（`ai_function.go:1062`）。
- 截断 util：`TruncateDiff(changes []model.FileChange, maxBytes, maxFiles, maxLines) (diffText string, info TruncationInfo)`，渲染 `{{truncationNote}}`。

**skill 配置项**（schema v2，落 `data/ai_functions.json`）：
- `ID: "code-review"`，`Command: ""`（纯 prompt，无斜杠命令）或指向 claude
- `Params.Type: "form"`，`PromptTemplate` 为上文模板
- `Fields`: `diff`（hidden，diff 文本）、`projectRules`（hidden，规范文本）、`truncationNote`（hidden，截断提示）

**优势**：零链路改动，复用 `RunStage` / `BuildStagePrompt` / stream-json / 落盘 / `GetAiTaskOutput` 全套基础设施。
**劣势**：大 diff 超窗时截断丢失部分文件审查；跨文件关联虽完整但注意力衰减。

### 4.2 方案 B：按文件拆分 + 聚合（进阶，大 diff）

**适用**：大 PR（> 20 文件或单次 diff > 200K 字节）。CodeRabbit 模式。

**流程**：
1. 后端 `GetLocalChanges` 筛文件列表，逐文件 `GetDiff` 取单文件 diff。
2. 每文件独立 `RunStage` 调用（受 `aiTaskMaxConcurrent` 并发槽位限制，串行或受限并行）。
3. 每文件输出独立 JSON（同 schema，findings 仅含该文件）。
4. 后端聚合所有文件 findings 为统一清单，按 severity 排序，附加 `summary` 汇总。

**PromptTemplate**（方案 A 模板基础上调整「待审查 diff」为单文件）：

```
（前文维度与规则同方案 A）

# 待审查文件

文件路径：{{file}}

# 该文件 diff

{{diff}}

{{truncationNote}}

（输出格式同方案 A，findings 仅含此文件的问题）
```

**后端聚合**：
- `AggregateFileReviews(results []FileReviewResult) *ReviewResult`：合并 findings、重编号（F1...Fn）、按 severity 排序、汇总 stats、生成 summary。
- 每文件 `extractReviewJson` 独立解析，单文件失败不阻塞其他（降级为该文件原文展示）。

**优势**：单文件上下文聚焦、审查深度一致；超窗风险低；单文件失败不阻塞。
**劣势**：跨文件关联丢失（接口/调用不同步问题难发现）；调用次数 = 文件数，成本与延迟上升；并发受 `aiTaskMaxConcurrent` 限制。
**缓解跨文件丢失**：可选「方案 A+」——先整手 diff 生成跨文件关联 finding（仅 critical 级），再按文件拆分生成细粒度 finding，去重合并。MVP 不做。

### 4.3 方案 C：claude tool use 结构化输出（最稳，最重）

**适用**：对 JSON 格式稳定性要求极高的场景（如自动化阻断提交）。MVP 不推荐。

**原理**：通过 MCP server 提供 `submit_review` tool，参数 schema 即 4.0 的 JSON schema。模型审查后调用 tool 返回结构化结果，schema 强制校验，零格式错误。

**实现路径**：
1. WorkBench 内置一个轻量 MCP server（stdio），暴露 `submit_review(summary, findings)` tool。
2. code-review skill 配置 `Mcp.Servers` 指向该 server（现有 `buildClaudeArgs` 已支持 `--mcp-config`，`ai_function.go:811`）。
3. prompt 要求模型「审查完成后调用 submit_review tool 提交结果」。
4. `parseStreamLine` 扩展处理 `type=assistant` 的 `tool_use` content 块（当前只取 text，`ai_function.go:1024-1031`），提取 tool 参数为结构化结果。

**优势**：schema 强制校验，零格式错误；无需 `extractReviewJson` 容错。
**劣势**：
- 需新增 MCP server（Go 实现 stdio MCP 协议）。
- 需扩展 `parseStreamLine` / `messageContentPart` 处理 `tool_use` 块（当前 `Type` 仅识别 `text`，`ai_function.go:1005`）。
- 流式推送退化为「tool 调用时一次性返回」（审查过程中无文本流式反馈，用户体验下降）。
- 跨层契约：若 MCP server 配置经 App 暴露须同步 `frontend/wailsjs/` 三处。

**何时升级到方案 C**：MVP 上线后，若 `extractReviewJson` 解析失败率 > 5% 或需自动化阻断提交（critical finding 阻断 CI），再升级。

---

## 五、推荐实施路径

| 阶段 | 方案 | 内容 | 复用点 |
|---|---|---|---|
| **PR3 MVP** | 方案 A | 整手 diff + prompt 约束 JSON + `extractReviewJson` 预解析 + 截断 util + 项目规范注入 | `RunStage` / `BuildStagePrompt` / stream-json / `extractTable` 模式 / 落盘 / `GetAiTaskOutput` |
| **PR3 进阶** | 方案 B | 大 PR 自动降级为按文件拆分 + 聚合 | `GetLocalChanges` / `GetDiff` 逐文件 / `aiTaskMaxConcurrent` 并发 |
| **未来** | 方案 C | tool use 结构化输出（仅当稳定性不足或需阻断提交时） | `buildClaudeArgs` 已支持 `--mcp-config`；需扩展 `parseStreamLine` |

---

## Caveats / 待验证

1. **外部工具 prompt 原文未逐字引用**：GitHub Copilot / CodeRabbit / ai-pr-reviewer 的 prompt 为内部或经商业化迁移，raw 路径 404、无 token 访问 GitHub API 被限流。本报告描述的是设计模式层面的业界共识（基于官方文档、技术博客、早期开源 README），稳定可参考。若需逐字 prompt 原文，建议实现期人工核验对应仓库最新版本（如 `coderabbitai/ai-pr-reviewer` 历史提交、CodeRabbit 官方文档）。

2. **claude CLI 无原生 JSON mode**：`--output-format json` 仅控制 CLI 输出封装（返回单个 result JSON 对象），不强制 `result` 字段内容是合法 JSON。故方案 A 依赖 prompt 约束 + 后端 `extractReviewJson` 容错，非 100% 可靠。实现期建议统计解析失败率，超阈值再升级方案 C。待 research 主题 `claude-cli-structured-json.md` 进一步核证 claude CLI 的结构化输出能力（PRD Assumptions 已列）。

3. **stream-json 的 result 事件文本未被纳入 Output**：`parseStreamLine`（`ai_function.go:1033`）在 `type=result` 时返回空文本（`return "", true, ...`），仅取 metrics。模型输出的 JSON 实际来自 `type=assistant` 的 `message.content[].text` 增量拼接。此为已实证行为，方案 A 的 `extractReviewJson` 须从 `Output`（assistant text 拼接）提取，而非从 result 事件。实现期若发现 JSON 不完整（如被截断在 `Output` 末尾预览 ~4KB 之外），须走 `GetAiTaskOutput` 全量读后重解析（与 `extractTable` 的兜底路径一致，`ai_function.go:1059` 注释已说明此模式）。

4. **`model.FileChange` 字段名待确认**：PRD Assumptions 标注 `Staged` bool 字段待 codegraph_node 确认。截断 util 与按文件拆分依赖此字段筛暂存区。实现期确认；若字段名不同，调整 `TruncateDiff` 签名。

5. **`GetCommitFileDiff` 是否存在待确认**：PRD Assumptions 标注。PR3「审查此 commit」入口需要单 commit 单文件 diff。不存在则用 `GetRangeDiff` 或 `git show <sha> -- <file>` 补。实现期确认。

6. **项目规范注入的单一数据源未定**：建议预设 `data/code_review_rules.md`（用户可编辑）作审查依据单一数据源，经 `{{projectRules}}` 注入。是否复用 `CLAUDE.md` 关键章节摘要、还是独立文件，待实现期与产品确认。`CLAUDE.md` 整份注入会挤占 diff 上下文，不推荐。

7. **方案 B 的并发成本未量化**：按文件拆分时调用次数 = 文件数，受 `aiTaskMaxConcurrent` 限制。大 PR（50+ 文件）串行延迟可能超 `TimeoutMinutes`。实现期需评估是否提高并发上限或分批聚合。

8. **跨文件关联丢失（方案 B）**：按文件拆分无法发现「A 文件改接口、B 文件调用未同步」。MVP 接受此局限（Out of Scope 范围内的「跨仓库批量审查」已排除，但单仓库跨文件关联属 PR3 范围内）。若反馈强烈，未来用「方案 A+」补跨文件 critical finding。

## Related Specs / 内部参考

- `docs/spec/cross-layer-contracts.md` — 跨层契约三处同步（若新增 App 方法暴露 `extractReviewJson` 须遵守；建议留 service 层不经 App 暴露则不触发）
- `docs/spec/app-services-assembly.md` — 新增 service 须 `AppServices` struct 加字段 + `NewAppServices` 加构造行 2 处（若截断 / 聚合逻辑独立为 service）
- `docs/spec/logging-and-errors.md` — 错误用 `AppError{Code,Message}` 跨层分流（审查失败、JSON 解析失败的错误码须同步 `model/app_error.go` + `frontend/src/utils/error.js`）
- `docs/spec/test-coverage-gate.md` — 覆盖率门禁：service ≥76%（`extractReviewJson` / `TruncateDiff` 须达覆盖率）
- `docs/spec/e2e-testing.md` — E2E 用 `frontend/e2e/fixtures.js` mock claude 输出（PR3 审查结果展示 E2E 须 mock claude 返回 JSON）
