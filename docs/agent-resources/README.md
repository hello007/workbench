---
harnessVersion: 1.10.1
stack: mixed-generic-vue
initMode: init
initDate: "2026-11-05"
---

# agent-resources 目录导览

> 本目录存放**给 coding agent 读的项目资料**，按内容性质分目录。
> 本文是**总导览与最终裁判入口**：目录职责、主题权威、命名规则、新文件归属都在这里。任何两份文档说法冲突时，以本文的「主题 → 权威文件」表为准。
> 路径一律用反引号标注，表示按需读取——需要时再打开，不要一次性全读。

## 1. 装载方式

本目录的文档按**装载时机**分两层，职责互不越界：

| 装载方式 | 触发时机 | 只装什么 | 不装什么 |
|----------|----------|----------|----------|
| **常驻** | 每次会话 | 摘要、指针、硬约束 | 规则正文、操作步骤 |
| **按需** | 索引导航后 Read | 全部规则与现状细节 | 摘要（那是常驻层的职责） |

常驻层由仓根 `AGENTS.md`（Claude Code 经 `CLAUDE.md` 的 `@AGENTS.md` 导入）承担；按需文档一律经根目录索引进入，**不要通读目录**。

> 与本仓既有体系的关系：任务分流与工作流注入由 Trellis 体系（`.trellis/workflow.md`，经 `.claude/settings.json` 的 SessionStart hook 注入）承担；本目录不重复该职责，专注于约定（规则）、参考（现状）与场景剧本。

## 2. 目录一览

| 目录 | 里面是什么 | 什么时候读 | 变更规则 |
|------|-----------|-----------|---------|
| `conventions/` | 项目约定（**规则**）：祈使句写的，能判定"这样写违规" | 写代码、写测试、调整模块前 | 只在有人做出决策时才变；改了要同步 `index-convention.md` |
| `workflows/` | **场景剧本**：分步流程、检查点、产出物 | 进入特定工作模式时按需加载 | 流程调整时更新，同步 `index-workflows.md` |
| `reference/` | 项目里的既定事实知识，供查询——架构、配置等 | 需要查现状时 | **内容易腐**，代码一改就可能过期；引用具体数字、行号、清单前先核对代码 |
| `adrs/` | 决策记录：当初为什么这么定，一文件一条 | 改动既有架构/设计前 | 已生效的**不可修改**，只能新增或标废弃；新增要同步 `index-adr.md` |
| `troubleshooting/` | 已验证的报错解法，一文件一条，带审核日期与到期提醒 | 构建/测试报错时 | 每条有独立的审核周期；永久解法实施后可删除；新增要同步 `index-troubleshooting.md` |
| `temp-memory/` | agent 在会话中记下的待办、决策、套路 | 会话开始（待办）、动既有设计前（决策）、写新代码前（套路） | **每条都有出口**，处理完即删；不长期停留 |

根目录另有索引文件，只列条目、不含正文：

- `index-convention.md` — `conventions/` 的入口，按场景查某条规则在哪份文件
- `index-workflows.md` — `workflows/` 的入口
- `index-adr.md` — 全部 ADR 的清单与状态
- `index-reference.md` — `reference/` 的入口
- `index-troubleshooting.md` — 全部排障条目的清单与审核状态

## 3. 主题 → 权威文件（单一权威 SSOT）

**一个主题只认一份权威文件。** 下表未列出的位置若出现同一主题的正文，一律视为待清理的副本；发现说法冲突时以本表为准。

| 主题 | 权威文件 | 其余位置怎么写 |
|------|----------|----------------|
| 构建 / 测试 / 发布命令清单 | `AGENTS.md` §2 | 只写"按改动范围如何选命令"，不复述清单 |
| 模块边界 / 依赖方向 / 存量例外白名单 | `conventions/project-boundaries.md` | `reference/architecture.md`、`AGENTS.md` 改指针 |
| Wails 绑定契约 / 前后端数据契约 / 兼容性 | `conventions/coding-api-contracts.md` | 评审清单、层差异段落改指针 |
| Vue 3 组件规范 / 响应式 / 样式隔离 / 状态共享（含 Pinia） | `conventions/coding-vue-style.md` | 评审清单改指针 |
| 网络层收口（transport 双通道）/ 路由 | `conventions/coding-api-integration.md` | — |
| 错误处理 / 容灾 | `conventions/coding-error-handling.md` | — |
| 测试策略与分层 | `conventions/testing-strategy.md` | 工作流改指针 |
| 构建质量门禁 / 已知门禁缺口（无 lint 现状） | `conventions/process-build-quality.md` | 索引、配置参考改指针 |
| 文档治理与归属 | `reference/docs-management.md` | 工作流改指针 |
| 工程结构与各层定位 | `reference/architecture.md` | `AGENTS.md` 摘要层改指针 |
| 配置取值现状（构建/环境配置作用域） | `reference/config-variables.md` | — |

> 表内路径除仓根 `AGENTS.md` 外，均相对 `docs/agent-resources/`。
> 与本仓既有规范沉淀（`docs/spec/` 与仓根 `AGENTS.md`「关键规则」清单）的分工：`docs/spec/` 记录**本项目踩坑沉淀的深度契约**（跨层签名、分期演进、事故机理），本目录沉淀**跨技术栈的通用工程规范**；两者互补，冲突时以 `docs/spec/` 为准（项目事实优先于通用规范）。

判定口径：
- **约定还是参考**：这句话**能不能判定某个改动违规**？能 → `conventions/`；不能 → `reference/`。
- **规则还是剧本**：可判定的**规则**（祈使句、不含步骤）放 `conventions/`；分步**操作**（阶段、检查点、产出物）放 `workflows/`。两者不重复同一段正文。

## 4. `conventions/` 命名规则

文件名**前缀即范畴**，前缀是唯一硬规则；后缀（`-style` / `-strategy` / `-contracts` 等）仅作描述，可自由取用。

| 前缀 | 范畴 |
|------|------|
| `coding-` | 编码规范：语言风格、组件风格、契约与兼容性、错误处理、状态管理 |
| `process-` | 流程规范：开发分工、构建与质量 |
| `project-` | 项目约束：模块职责、依赖方向、目录落位 |
| `testing-` | 测试策略 |

**不要通读整个目录。** `index-convention.md` 按"场景 → 文档"列了对照表，按当前任务挑 1-3 份读。各份之间会互相引用，顺着指针走即可。

## 5. 各目录细节

### `conventions/` — 规则

只放**能判定违规的规则**，祈使句写法，不含操作步骤（步骤在 `workflows/`）。四类前缀见上一节。新增或改名后同步 `index-convention.md`。

### `workflows/` — 场景剧本

按场景划分的行为指南，进入对应工作模式时按需加载。典型场景：开发新功能、修复 Bug、写文档、Code Review。

每份剧本定义该场景下的阶段、检查点与产出物，具体规则一律指向 `conventions/` 或 `reference/`，不复述。通过 `index-workflows.md` 索引。

### `adrs/` — 决策记录

一条决策一个文件，命名 `ADR-<三位编号>-<标题>.md`，编号顺序递增不复用，带状态机（提议 / 生效 / 废弃）。新增时复制 `_模板.md`，并把条目登记到 `index-adr.md`。

与 `conventions/` 的分工：约定说"该怎么做"，ADR 说"当初为什么这么定"。同一件事往往两边都有——约定给出可执行的规则，ADR 保留取舍过程。所以**改约定时先看有没有对应 ADR**，别推翻已生效决策；若是新取舍，补一条新 ADR 而不是改旧的。

### `reference/` — 参考

入口是根目录的 `index-reference.md`，按分类索引所有文档。

`reference/` 下允许有子目录（如 `internal-mechanisms/`），所有文档通过 `index-reference.md` 统一索引。**只描述现状，不承载规则**；凡能判定违规的内容一律归 `conventions/`。

### `troubleshooting/` — 排障库

一文件一条已验证的报错解法，通过 `index-troubleshooting.md` 渐进式加载。每条标注审核状态与到期日，过期未审 agent 会提醒。

**不要通读整个目录**。报错时先查索引，找到对应条目再打开单条文件。永久解法实施后可直接删除条目。

### `temp-memory/` — 临时会话记忆

| 文件 | 内容 | 出口 |
|------|------|------|
| `TEMP-MEM-activeContext.md` | 跨会话的待办、欠账、待人判断项 | 人解决后删除 |
| `TEMP-MEM-decisions.md` | 会话中识别到的决策及理由 | 有长期价值→升级为 ADR；否则删除 |
| `TEMP-MEM-patterns.md` | 会话中总结的本仓代码套路 | 确认成立→升级到 `conventions/`；否则删除 |

三份都有**积压阈值**，超过就要提醒人处理、agent 不自行处置；阈值与提醒话术见各文件。

这里只写团队共享的项目事实，**不写个人 session 进度**（在做什么、本次改了什么、什么还没提交）。

## 6. 新文件该放哪儿

| 要写的东西 | 放哪儿 | 附带动作 |
|-----------|--------|---------|
| 可判定违规的规则 | `conventions/`（带范畴前缀） | 同步 `index-convention.md` |
| 特定场景的分步剧本 | `workflows/` | 同步 `index-workflows.md` |
| 有长期影响的架构/设计决策及理由 | `adrs/` | 复制 `_模板.md`，同步 `index-adr.md` |
| 描述现状，供查阅 | `reference/` | 注明内容易腐 |
| 已验证的报错解法，附临时+永久解法 | `troubleshooting/` | 复制 `_模板.md`，同步 `index-troubleshooting.md` |
| 会话中的待办、决策、套路 | `temp-memory/` 对应文件 | 检查是否超积压阈值 |
| 本项目深度契约与踩坑机理 | `docs/spec/`（仓根 AGENTS.md 指引） | 按「规范沉淀规则」两级分流 |
