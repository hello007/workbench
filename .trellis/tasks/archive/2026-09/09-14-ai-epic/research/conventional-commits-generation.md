# Research: Conventional Commits 提交信息生成的 prompt 工程最佳实践

- **Query**: AI 提交信息生成（claude CLI 子进程，输入暂存区 diff，输出 2-3 个 Conventional Commits 候选）的 prompt 工程最佳实践
- **Scope**: mixed（外部可比工具源码 + 内部仓库约束）
- **Date**: 2026-09-14
- **外部源抓取时间**: 2026-09-14（raw.githubusercontent.com / api.github.com）

---

## 一、可比工具/模式对比（4 个）

### 1. aicommits（Nutlope/aicommits）

**源码核证**（`src/utils/prompt.ts` / `src/utils/openai.ts` / `src/utils/git.ts` / `src/cli.ts`，2026-09-14 抓取）：

- **diff 来源**：`git diff --cached --diff-algorithm=minimal`，排除 lock 文件（`package-lock.json` 等）；若暂存的只有 lock 文件则不排除。
- **prompt 组成**（`generatePrompt(locale, maxLength, type, customPrompt?)`，作为 system message）：
  - `Generate a concise git commit message title in present tense that precisely describes the key changes in the following code diff. Focus on what was changed, not just file names. Provide only the title, no description or body.`
  - `Message language: ${locale}`
  - `Commit message must be a maximum of ${maxLength} characters.`
  - `Exclude anything unnecessary such as translation. Your entire response will be passed directly into git commit.`
  - `IMPORTANT: Do not include any explanations, introductions, or additional text... Respond with ONLY the commit message text.`
  - `Be specific: include concrete details (package names, versions, functionality) rather than generic statements.`
  - 用户自定义 `customPrompt`（可覆盖/追加）
  - **type-to-description JSON**（conventional 模式）：`docs/style/refactor/perf/test/build/ci/chore/revert/feat/fix` 每类带一句话定义，附 `IMPORTANT: The type MUST be lowercase`。
  - **格式约束**：`<type>[optional (<scope>)]: <commit message>\nThe commit message subject must start with a lowercase letter`
- **多候选机制**：`--generate N`（默认 1）独立采样 N 条 → `deduplicateMessages`（Set 去重）→ `sanitizeMessage`（仅取首行、去首尾引号、去尾句号、去前导 `<tag>`）→ 推理模型 `CppMethodInitializedthink>-tags` 提取；超长则二次 LLM 调用 `shortenCommitMessage`（system `You are a tool that shortens git commit messages...`，temperature 0.2、maxOutputTokens 500）。
- **few-shot**：**不取历史 commit**，靠模型先验 + 严格格式指令 + type 定义 JSON。
- **多样性**：仅靠 N 次独立采样 + Set 去重，**无显式多样性约束**（候选趋同风险仅靠 dedup 兜底）。

### 2. opencommit（di-sukharev/opencommit）

**源码核证**（`src/prompts.ts` / `src/modules/commitlint/prompts.ts` / `src/generateCommitMessageFromGitDiff.ts`，2026-09-14 抓取）：

- **diff 来源**：`git diff --staged`；大 diff 用 `splitByTokenLimit`（`TOKEN_BOUNDARY_RESERVE` + `ADJUSTMENT_FACTOR=20`）切片 → `mergeDiffs` 合并 → `runTasksWithConcurrency`（`MAX_CONCURRENT_GENERATIONS=3`）并发生成。
- **prompt 结构**（`getMainCommitPrompt`，3 条 message 的 one-shot 模板）：
  1. **system `INIT_MAIN_PROMPT`**：`IDENTITY="You are to act as an author of a commit message in git."` + mission `create clean and comprehensive commit messages as per the Conventional Commit Convention and explain WHAT were the changes and mainly WHY the changes were done.` + `I'll send you an output of 'git diff --staged' command` + `CONVENTIONAL_COMMIT_KEYWORDS="fix, feat, build, chore, ci, docs, style, refactor, perf, test"` + description guideline + one-line guideline + scope instruction + `Use the present tense. Lines must not be longer than 74 characters. Use ${language}` + 用户额外 context
  2. **user `INIT_DIFF_PROMPT`**：一段**固定示例 diff**（server.ts `port`→`PORT`，one-shot 样例输入）
  3. **assistant `INIT_CONSISTENCY_PROMPT`**：**预填的期望输出**（目标语言的示例 fix + feat commit + description，one-shot 样例输出）
  4. user：真实 diff
- **commitlint 模块**（`OCO_PROMPT_MODULE=@commitlint`）：读取项目 `@commitlint` 配置，把每条规则（`type-enum`/`scope-case`/`subject-max-length`/`header-full-stop`...）经 `rulesPrompts` map 转成 LLM 可读自然语言，再跑一次 meta-LLM 调用（`GEN_COMMITLINT_CONSISTENCY_PROMPT`）生成语言一致性 JSON，按 hash 缓存。**这是"项目规范注入"的最完备方案**。
- **配置开关**：`OCO_EMOJI`（gitmoji）/ `OCO_DESCRIPTION`（生成 WHY body）/ `OCO_ONE_LINE_COMMIT` / `OCO_OMIT_SCOPE` / `OCO_LANGUAGE` / `OCO_PROMPT_MODULE` / `OCO_AI_PROVIDER`+`OCO_MODEL`。
- **关键指令原文**：
  - description：`Add a short description of WHY the changes are done after the commit message. Don't start it with "This commit", just describe the changes.`
  - one-line：`Craft a concise, single sentence... If the modifications share a common theme or scope, mention it succinctly; otherwise, leave the scope out to maintain focus.`
  - scope：`OCO_OMIT_SCOPE` → `Do not include a scope... Use the format: <type>: <subject>`
- **多候选**：**单候选**，交互式 confirm/编辑/重生成。
- **few-shot**：用**固定示例**（非历史 commit）+ 可选 commitlint 规则注入。

### 3. commitizen（commitizen/cz-cli）—— 非 AI 基线

- 交互式问答：type → scope → subject → body → breaking → footer，由 adapter（`cz-conventional-changelog`）驱动。
- 靠 commitlint 规则（`type-enum`/`scope-enum`/`subject-max-length`）硬约束，**不分析 diff**，分类全靠人。
- 参考价值：opencommit 的 commitlint 模块复用的就是这套规则源；其 `type-enum` 即 Angular/Conventional 类型集。

### 4. GitHub Copilot "Generate Commit Message" / Claude Code 提交辅助

- **prompt 非公开**，下述为可观测行为，**细节未经源码核证**，仅供方向参考：
  - Copilot：Source Control 面板"闪光"图标触发，读暂存变更，输出单条 conventional message，祈使句、小写 type，用户可编辑。
  - Claude Code：读取暂存 diff 生成提交信息（具体是否为 `/commit` 斜杠命令、prompt 细节未公开）。
- **不作为本仓库 prompt 设计的事实依据**，仅说明"单候选 + 祈使句 + conventional + 用户编辑"是工业默认交互。

---

## 二、通用约定与存在原因

| 约定 | 采用方 | 存在原因 |
|---|---|---|
| 仅取暂存 diff（`--cached`/`--staged`） | aicommits、opencommit | 提交信息须描述"本次将提交的内容"，纳入未暂存会误判 |
| 祈使句 / 现在时 / subject 首字母小写 | aicommits、opencommit、本仓库 docs/开发规范.md | conventional commits 规范 + git 惯例（"if applied, this commit will <subject>"） |
| type 小写、枚举固定 | aicommits、opencommit | changelog 自动生成依赖类型一致；大写/变体会破坏解析 |
| 行宽上限（opencommit 74 / aicommits maxLength） | aicommits、opencommit | git 50/72 惯例，终端与 GitHub UI 显示 |
| type-to-description JSON 注入 | aicommits | 给模型可对照的判定标准，降低 type 误判 |
| "Focus on what changed / explain WHAT & WHY" | aicommits、opencommit | diff 已含文件名，message 须提供 diff 没有的语义价值 |
| "Respond with ONLY..." 严格输出 | aicommits、opencommit | LLM 易加前导语（"Here's your commit message:"），破坏 `git commit -F`；aicommits 额外做 sanitize 兜底 |
| scope 可选、无明确 scope 时省略 | opencommit | 强行造 scope 对跨模块变更产生噪声 |
| body 写 WHY 不写 WHAT、不以 "This commit" 开头 | opencommit | WHY 是 diff 推不出的上下文，WHAT 已在 diff 里 |
| 语言/locale 注入 | aicommits、opencommit | 模型默认英文，须显式指定输出语言 |

---

## 三、映射到本仓库约束（Go + Wails + claude CLI 子进程）

### 已具备（PRD 已 codegraph 验证 + 本次核证）

| 复用点 | 位置 | 与 prompt 方案的关联 |
|---|---|---|
| `FileChange{Path,Status,Staged bool}` | `model/commit.go:32-36` | 暂存筛选字段确认存在（PRD 假设成立） |
| `App.GetLocalChanges(path)` | `app_git.go:574` | 取 `[]FileChange`，筛 `Staged==true` 得暂存文件清单 |
| `App.GetCommitFileDiff(path,sha,file)` | `app_git.go:619` | PR3 代码审查单 commit 单文件 diff（PRD 假设成立） |
| `App.GetRangeDiff(path,baseSHA,headSHA)` | `app_git.go:630` | PR3 区间 diff |
| `App.CommitFiles(path,message,filePaths)` | `app_git.go:590` → `gitSvc.Commit`（`git.go:581`） | 候选填入后走现有提交链路 |
| `AiFunctionService.RunStage(functionID,prompt,resumeSessionID)` | `service/ai_function.go:338` | 起 claude `-p` 子进程 + `--output-format stream-json`，事件 `ai-task:output`/`ai-task:done` 推前端 |
| `BuildStagePrompt(command,spec,params)` | `service/ai_function.go:895` | form 类型经 `PromptTemplate` `{{key}}` 占位渲染，**diff/history 文本注入无需新 helper** |
| `AiFunction` 配置项 | `model/ai_function.go:5` | 新增 commit-message skill 只加配置（`data/ai_functions.json`），不改代码 |
| `gitCmd.Execute(gitRoot,"log","--format=%s","-N")` | `service/git.go`（`gitCmd` 已用于 branch/tag） | **取历史 commit 做 few-shot 无需新 Wails 绑定**，service helper 即可 |

### 历史 commit 风格（few-shot 样本质量核证）

最近 30 条 commit（`git log --oneline -30`）均为规范 conventional 格式：

```
feat(session): 崩溃恢复 UI 状态快照持久化
perf(frontend): mermaid 懒加载减首屏 eager 1.8MB
refactor(test): 抽 util/testutil 跨包测试辅助收敛 service/util/main 三包
fix(diff): 代码审核修复——SHA 注入校验、降级语义收紧、配置清空残留路径
chore(deps): 依赖升级 minor/patch + 安全扫描入 CI
docs(roadmap): 同步 v1.4 平台加固 epic 完成项
```

- 格式：`<type>(<scope>): <中文描述>`，scope 为顶层模块名（session/frontend/test/diff/deps/roadmap）。
- body 用 `-` 列表 + 末尾 `Co-Authored-By` trailer。
- **结论**：本仓库历史 commit 是高质量 few-shot 源，scope 命名范式清晰，可直接给模型对照。

### 关键 gap（须 PR1 处理，非 prompt 层）

- **`GetDiff`（`git.go:669`）走 `git diff HEAD -- <file>`（工作区对比 HEAD），非暂存区 diff**。aicommits/opencommit 均用 `--cached`/`--staged`。PR1 diff 聚合 helper **不能直接复用 GetDiff**，需新增 staged-diff 变体（`git diff --cached -- <file>`，未跟踪文件用 `git diff --cached --no-index /dev/null <file>` 仅当已 `git add`）。否则生成的是"工作区全部改动"的描述，与实际提交内容错位。
- **diff 截断**：claude 上下文 200K tokens，但大 diff（如 vendor 全量）仍须截断。opencommit 的 `splitByTokenLimit`+`mergeDiffs` 过重，MVP 建议按字节/文件数截断 + 末尾提示"剩余 N 文件 M 行未纳入"。
- **结构化输出**：现有 `AiTaskRunResult.Output` 是流式文本末尾预览，2-3 候选须前端解析。方案：prompt 约束模型输出"编号列表"或"fenced JSON 数组"，前端正则/JSON 解析；或走 `--output-format json`（详见 research/claude-cli-structured-json.md）。

---

## 四、六大调研重点结论

### 1. few-shot 历史样本策略

| 维度 | aicommits | opencommit | 建议 |
|---|---|---|---|
| 是否用历史 commit | 否 | 否（用固定示例） | **用历史**（本仓库历史质量高） |
| few-shot 形态 | — | 1 段示例 diff + 期望输出 | 取最近 3 条 `subject`（`%s`，不带 body） |
| 选取策略 | — | — | 最近 N 条 + 去噪 |

- **N 取值**：3 条即可捕捉 type/scope/描述语气；>5 条边际收益递减且 token 上升。3 条 subject ≈ 100-200 token，远低于 diff 本身。
- **选取策略对比**：
  - 最近 N 条：捕捉最新风格演进，但可能偏向近期高频 type（本仓库近期多 `chore: record journal` 噪声，须过滤）。
  - 同类型 N 条：需先判 type（鸡生蛋），可二阶段但 MVP 过重。
  - 同文件/同目录 N 条：scope 命名最准，但冷启动文件无样本。
- **推荐**：最近 3 条非噪声 commit（过滤 `chore: record journal`/`chore(task): archive` 这类归档噪声），**只取 subject 不取 body**——降 token 同时提供 type/scope/描述范式。风格学习与 token 的平衡点在此。

### 2. type 分类准确率

- **aicommits trick**：type-to-description JSON（每类一句话定义）+ `type MUST be lowercase` + `Choose a type that best describes the git diff`。
- **opencommit trick**：`CONVENTIONAL_COMMIT_KEYWORDS` 枚举 + commitlint `type-enum` 规则注入。
- **经典误判**：refactor ↔ feat。aicommits 给的判定锚点是"是否改变功能/对外行为"：refactor = `improves code structure without changing functionality`，feat = `A new feature`。
- **多类型主导选择**：opencommit one-line guideline `primary updates` / `common theme`；aicommits `Focus on what was changed`。
- **推荐 type 判定决策树（注入 prompt）**：

```
新增对外能力/用户可感知功能 → feat
修复缺陷/异常行为 → fix
仅结构/可读性/命名调整，不变行为 → refactor
性能优化（同功能更快） → perf
新增/修正测试 → test
构建系统/依赖变更 → build
CI 配置/脚本 → ci
文档 → docs
代码风格/格式（不改语义） → style
其他杂项（不涉 src/test） → chore
多类型并存 → 取"影响最大/最贴近用户感知"的主导类型
```

### 3. scope 推断

- **opencommit**：`OCO_OMIT_SCOPE` 开关；one-line guideline `If modifications share a common theme or scope, mention it succinctly; otherwise, leave the scope out`。
- **aicommits**：`optional (<scope>)`，不强制。
- **策略**：从 diff 文件路径推断（`service/auth.go`→`auth`，`frontend/src/components/Foo.vue`→`foo`）；多目录取公共父或主导目录；无明确 scope 省略。
- **推荐 scope 推断规则（注入 prompt）**：
  - 优先取变更文件最集中的顶层模块名（本仓库范式：`service/`→服务名、`frontend/src/components/`→组件域、`docs/`→`docs`）。
  - 跨多模块且无主导 → **省略 scope**（opencommit 共识：leave scope out 优于硬造）。
  - scope 小写、连字符分隔（如 `repo-config`）。
  - **把变更文件路径列表随 diff 一起喂给模型**（本仓库 `GetLocalChanges` 已有 Path），显式提供 scope 推断原料。

### 4. 描述质量

| 维度 | aicommits | opencommit | 本仓库规范 | 建议 |
|---|---|---|---|---|
| 长度 | 仅 title | 可选 body + 74 字符行限 | — | subject 单行；MVP 不生成 body |
| 语气 | present tense | present tense | 祈使句 | 中文祈使句（"添加/修复/重构"） |
| 首字母 | lowercase | — | — | type 小写；中文 subject 无首字母问题 |
| body | 不生成 | WHY 不以 "This commit" 开头 | 说明为什么 | 候选阶段只要 subject；body 留用户手填或 PR3 |
| 动词 | — | — | — | 取历史 commit 动词范式（添加/修复/重构/建立/扩展/抽/收敛） |

- **推荐**：subject ≤50 中文字符（git 50/72 的中文近似）；MVP 候选仅 subject，不生成 body（避免候选冗长、降低 token、降低前端解析复杂度）。如需 body，按 opencommit 的 WHY 范式 1-2 行。

### 5. 候选多样性

- **aicommits**：N 次独立采样 + Set 去重（无显式多样性保证）。
- **opencommit**：单候选 + 交互重生成。
- **多样性策略**：显式要求每候选至少在 type/scope/详略之一不同。
- **推荐 3 候选设计**：
  - **候选 A**：主导 type + 精确 scope（最贴近历史风格）
  - **候选 B**：同 type，换描述角度（更概括或更具体）
  - **候选 C**：备选 type（feat↔refactor 边界时各给一个）或换 scope 粒度
  - temperature 0.7 鼓励差异；prompt 附 `3 个候选的 type+scope 组合不得完全相同`。
- **防趋同**：比 aicommits 的纯 dedup 更强——显式约束维度差异，而非仅去重。

### 6. 规范注入

| 方案 | 采用方 | 适配本仓库 |
|---|---|---|
| commitlint 规则→NL（动态） | opencommit | **不走**（本仓库无 commitlint 配置，引入成本高） |
| 硬编码 type 定义 JSON | aicommits | **采用**（轻、稳） |
| 项目规范文档提炼成规则块 | — | **采用**（源自 docs/开发规范.md 提交规范章节 + CLAUDE.md） |

- **推荐**：静态规则块（type 清单+定义、scope 规则、subject 规则：祈使句/中文/type 小写）+ 动态 few-shot（最近 3 条 subject）+ 变更文件路径列表（scope 原料）。三层注入，无需 commitlint。

---

## 五、Prompt 设计方案（3 个，由轻到重）

### 方案 A：轻量单候选（aicommits 风格，最小可行）

```
[system]
你是 Git 提交信息作者。根据下方 diff 生成一条 Conventional Commits 规范的提交信息。

类型清单（小写，选最贴近的一个）：
- feat: 新增对外能力/用户可感知功能
- fix: 修复缺陷/异常行为
- refactor: 调整结构/可读性/命名，不改变功能
- perf: 性能优化（同功能更快）
- test: 新增/修正测试
- build: 构建系统/依赖变更
- ci: CI 配置/脚本
- docs: 文档
- style: 代码风格/格式（不改语义）
- chore: 其他杂项（不涉 src/test）

格式：<type>(<scope>): <subject>
- scope 可选：取变更最集中的顶层模块名，跨多模块无主导则省略
- subject：中文祈使句，≤50 字，说明 what/why 而非罗列文件名
- type 小写，subject 不以句号结尾

历史提交风格参考：
1. feat(session): 崩溃恢复 UI 状态快照持久化
2. refactor(test): 抽 util/testutil 跨包测试辅助收敛三包
3. fix(diff): 代码审核修复——SHA 注入校验、降级语义收紧

[user]
变更文件：
{{fileList}}

暂存区 diff：
{{diff}}

只输出一条提交信息，不要任何解释或前后缀。
```

- **优点**：token 小、快、链路最简（复用 BuildStagePrompt）。
- **缺点**：无候选选择，用户只能接受或手改。
- **适用**：PR2 链路验证的最小骨架。

### 方案 B：多候选 + few-shot（推荐 MVP）

```
[system]
你是 Git 提交信息作者。根据下方 diff 生成 3 个 Conventional Commits 规范的提交信息候选，供用户选择。

【类型判定】（小写，按决策树选最贴近的）
新增对外能力 → feat；修复缺陷 → fix；仅结构/命名调整不变行为 → refactor；
性能优化 → perf；测试 → test；构建/依赖 → build；CI → ci；文档 → docs；
风格/格式 → style；其他杂项 → chore。多类型并存取"影响最大/最贴近用户感知"的主导类型。

【格式】<type>(<scope>): <subject>
- scope：取变更最集中的顶层模块名（见变更文件列表）；跨多模块无主导则省略 scope；
  scope 小写、连字符分隔。
- subject：中文祈使句，≤50 字，说明 what/why 而非罗列文件名，不以句号结尾。

【多样性要求】3 个候选的 type+scope 组合不得完全相同；至少一个维度（type / scope / 详略）不同：
- 候选1：主导 type + 精确 scope
- 候选2：同 type，换描述角度（更概括或更具体）
- 候选3：备选 type（如 feat↔refactor 边界各给一个）或换 scope 粒度

【历史风格参考】
1. feat(session): 崩溃恢复 UI 状态快照持久化
2. perf(frontend): mermaid 懒加载减首屏 eager 1.8MB
3. refactor(test): 抽 util/testutil 跨包测试辅助收敛三包

[user]
变更文件：
{{fileList}}

暂存区 diff：
{{diff}}

[输出格式] 严格按以下 JSON 数组输出，不要 markdown 代码块、不要解释：
[{"type":"...","scope":"...","subject":"..."},{"type":"...","scope":"...","subject":"..."},{"type":"...","scope":"...","subject":"..."}]
scope 为空时填空字符串。
```

- **优点**：候选可选、多样性显式约束、JSON 易解析（前端直接渲染选项）。
- **缺点**：token 中等；依赖模型遵守 JSON 格式（须前端兜底解析：先 JSON.parse，失败则按行/编号解析）。
- **适用**：PR2 正式交付（2-3 候选 + 前端选择填入）。
- **注入路径**：作为 `AiFunction` 配置项的 `Params.PromptTemplate`，`{{fileList}}`/`{{diff}}` 经 BuildStagePrompt 渲染；few-shot 部分可静态写死模板（方案 B1）或由 service helper 动态填充最近 3 条 subject（方案 B2，更优但须加 history 取数 helper）。

### 方案 C：二阶段（type 先判 + 同类型 few-shot）

- **阶段 1**：判主导 type + scope（基于 diff + 文件列表），输出 `{type, scope}`。
- **阶段 2**：取同 type 历史 N 条 commit 作 few-shot，生成 3 候选 subject。
- **优点**：type 准确率最高、few-shot 最相关。
- **缺点**：两次 claude 调用，成本/延迟翻倍；阶段间须 `--resume` 续会话或拼上下文。
- **适用**：MVP 之后的质量提升阶段，非 PR2 范围。

### 推荐

**PR2 采用方案 B**（B2 动态 few-shot 版）。理由：
1. 复用现有 `BuildStagePrompt` form 模板机制，零新增 prompt 组装代码。
2. 多样性显式约束优于 aicommits 的纯 dedup。
3. JSON 输出便于前端结构化渲染候选列表。
4. 动态 few-shot（最近 3 条非噪声 subject）比静态示例更贴合本仓库风格演进。

---

## 六、Caveats / 未决项

1. **`GetDiff` 非暂存 diff**（`git.go:669` 走 `git diff HEAD`）：PR1 须新增 staged-diff 变体，不能直接复用。这是链路正确性前提，非 prompt 层。
2. **结构化输出**：方案 B 的 JSON 数组依赖模型遵守格式，须前端兜底解析（JSON.parse 失败 → 按编号/行解析）。是否走 `--output-format json` 由 research/claude-cli-structured-json.md 决定。
3. **diff 截断阈值**：未定（按字节/文件数/行数），PR1 须定；超长截断后提示"剩余 N 文件 M 行未纳入"。
4. **few-shot 噪声过滤**：本仓库历史含大量 `chore: record journal`/`chore(task): archive` 归档噪声，取最近 3 条 subject 时须过滤（如排除 `chore: record journal`/`chore(task): archive`/`chore: ` 开头噪声），否则 few-shot 质量被拉低。
5. **GitHub Copilot / Claude Code 提交辅助的 prompt 非公开**：本文对其描述仅为可观测行为方向，未作源码核证，不作为本仓库设计的事实依据。
6. **type-to-description 定义取自 aicommits**（commitlint config-conventional 同源），与 docs/开发规范.md 的类型清单一致（feat/fix/refactor/docs/test/chore），本仓库规范未列 perf/build/ci/style 但 conventional 官方有——建议保留全集，让模型有更细判定粒度。
7. **body 生成**：MVP 不生成（候选只要 subject）；如后续要 body，按 opencommit 的 WHY 范式，且不以 "This commit"/"本次提交" 开头。

---

## 七、外部参考

- aicommits 源码（2026-09-14 抓取）：
  - `src/utils/prompt.ts` — https://github.com/Nutlope/aicommits/blob/main/src/utils/prompt.ts
  - `src/utils/openai.ts` — https://github.com/Nutlope/aicommits/blob/main/src/utils/openai.ts
  - `src/utils/git.ts` — https://github.com/Nutlope/aicommits/blob/main/src/utils/git.ts
- opencommit 源码（2026-09-14 抓取）：
  - `src/prompts.ts` — https://github.com/di-sukharev/opencommit/blob/master/src/prompts.ts
  - `src/modules/commitlint/prompts.ts` — https://github.com/di-sukharev/opencommit/blob/master/src/modules/commitlint/prompts.ts
  - `src/generateCommitMessageFromGitDiff.ts` — https://github.com/di-sukharev/opencommit/blob/master/src/generateCommitMessageFromGitDiff.ts
- Conventional Commits 1.0.0 规范 — https://www.conventionalcommits.org/
- commitlint config-conventional type-enum — https://github.com/conventional-changelog/commitlint（aicommits prompt.ts 注释引用）
