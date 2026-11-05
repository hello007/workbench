---
name: development
title: 开发/修改功能
description: 新功能开发与现有功能修改的标准工作流
---

# 开发工作流

适用于新功能开发、现有功能修改、代码重构等编码类任务。

> 本仓任务分流（是否建 Trellis 任务）由 Trellis workflow-state 每回合注入承担，本剧本在任务已确定进入实现后使用。

## 核心原则

- 先理解需求，再动手改代码
- 修改前先分析影响范围（优先使用 CodeGraph）
- 不执行 git commit / git push（除非用户明确要求）
- 改动范围和需求严格对齐，不顺手优化无关代码
- 优先假定环境正常：直接执行目标命令，失败后依据报错信息定位排查；不前置运行环境检测、依赖预检等防御性检查

## 阶段一：开发与验证（agent 执行）

### 第一步：理解需求与分析代码

1. 明确需求边界：要改什么、不改什么
2. 使用 CodeGraph 或代码搜索探索相关模块和符号
3. 找到入口点、调用链、跨模块/跨栈依赖关系（Go 桥接层 ↔ 前端 transport ↔ 组件）
4. 识别可能受影响的下游（Wails 绑定契约、前端消费组件、事件监听方）

### 第二步：制定方案

1. 明确改动涉及哪些模块、哪些文件
2. 如果有多种实现方案，说明取舍并选择最简单的
3. 涉及 Wails 绑定方法签名或 `model/` struct 变更时，遵循契约同步规则（见 `docs/agent-resources/conventions/coding-api-contracts.md`）
4. 对不明确的地方主动提问，不要假设

### 第三步：修改代码

沿依赖方向从**最底层**开始，自底向上修改（依赖边界见 `docs/agent-resources/conventions/project-boundaries.md`）：

1. Go 侧：`util/` → `service/` → `model/` → `app*.go` 桥接层
2. 前端侧：`utils/` → `transport/` → `composables/`、`store/` → `components/` → `views/`、`router/`

功能开发过程中不编写测试，测试统一在第四步集中完成。开发过程中的自查使用编译类命令快速验证，不运行全量测试。

例外：关键组件（复杂算法、易错边界、状态机逻辑）允许在开发过程中先编写最小验证性测试快速反馈，以简为限——不追求完备、不为此执行完整构建，最终仍以第四步的集中编写与验证为准。

### 第四步：编写测试（功能代码全部完成后集中进行）

- 测试集中编写：功能代码全部改完后，一次性编写本批次全部测试，随后统一验证；不在功能开发过程中穿插测试编写与测试运行
- 新代码必须有测试覆盖核心路径（分层与命名规范见 `docs/agent-resources/conventions/testing-strategy.md`；Go 侧辅助函数入 `util/testutil`，禁止各 `_test.go` 重复定义）
- 修改已有功能时，补充对应的测试用例

### 第五步：编译测试验证

按改动范围选择命令（完整清单见仓根 `AGENTS.md` §2）：

```bash
# Go 改动
go vet ./...
go test ./...
# 前端改动（frontend/ 下）
npm run build
npm test
```

内层循环加速（只跑目标用例，不能代替完整验证）：

```bash
go test ./service/ -run TestXxx -v          # Go 单包单用例
cd frontend && npx vitest run src/**/Xxx.spec.js  # 前端单文件
```

最终验证必须跑全量门禁命令。内层循环以批次为单位执行，不逐个用例单独运行。

### 第六步：收尾自查

本仓无格式化工具，无需格式化步骤；对照 `docs/agent-resources/conventions/process-build-quality.md` 的门禁清单确认相关项已通过。

## 阶段二：任务完成后（agent 只提醒，不代劳）

编译测试通过后**停下**，报告结果，把下面这些**作为提醒说给用户**——用户没有明确要求，就不要动手。

### 阶段二 agent 不做的事

不跑完整门禁、不敲 `git add` / `commit` / `push`、不替用户改正式文档。

用户明确说了才做对应那一条，**只做说到的那一条，不顺手带上其余步骤**。

### 提醒清单

| 情况 | 提醒用户做什么 |
|------|---------------|
| 新增能力、或改了对外接口 / 配置项 / 错误码 | 同步正式文档（`docs/` 与 README.md，见仓根 CLAUDE.md 文档索引） |
| 沉淀了新的关键规则或深度契约 | 按「规范沉淀规则」分流到 CLAUDE.md 关键规则行或 `docs/spec/` |
| 上面都处理完了 | 提交代码（由用户手动执行或显式指定提交方式） |

### 文档同步判断规则

- 新增模块/能力 → 补指南类文档
- 改了对外接口、配置项、错误码 → 更新对应文档
- 内部重构、无对外变化 → 一般不需要

工具怎么选见 `docs/agent-resources/reference/docs-management.md`。
