# brainstorm: spec 沉淀迁移出 trellis 目录

## Goal

将 `.trellis/spec/` 下已沉淀的项目知识迁到项目正式文档位置（`docs/`），并让未来的 `trellis-update-spec` 沉淀写入非 trellis 位置（`CLAUDE.md` 或其他合适位置），在**不修改 trellis 源码**的约束下实现。终态：`.trellis/spec/` 理论为空目录、可删除。

## What I already know

### 现有 spec 内容盘点（共 10 文件 / 1020 行）

| 文件 | 行数 | 状态 |
|---|---|---|
| `backend/cross-layer-contracts.md` | 121 | **已填**：Wails 绑定同步契约（App 方法签名 / models.ts 同步），高价值 |
| `backend/error-handling.md` | 139 | 空模板（"To be filled by the team"） |
| `backend/database-guidelines.md` | 51 | 空模板 |
| `backend/directory-structure.md` | 51 | 空模板 |
| `backend/logging-guidelines.md` | 51 | 空模板 |
| `backend/quality-guidelines.md` | 51 | 空模板 |
| `backend/index.md` | 39 | 索引（含 Pre-Development Checklist） |
| `guides/code-reuse-thinking-guide.md` | 105 | 通用思维指南（非项目特定） |
| `guides/cross-layer-thinking-guide.md` | 322 | 通用思维指南（非项目特定） |
| `guides/index.md` | 87 | 索引 |

真正需要迁移的项目沉淀只有 `cross-layer-contracts.md` 一个文件。

### 引用 `.trellis/spec/` 的机制（迁移/删除的连带影响面）

| 引用点 | 性质 | 删除 spec 后影响 |
|---|---|---|
| `.trellis/workflow.md` | **官方定制层**（trellis 明示 "All customization is done by editing this file; the scripts are parsers only"） | 多处段落指引读/写 `.trellis/spec/`，需改写指向 |
| `.claude/skills/trellis-update-spec/SKILL.md` | trellis 安装产物（升级可能覆盖） | skill 指令固定写 `.trellis/spec/` |
| `.claude/skills/trellis-before-dev/SKILL.md` | trellis 安装产物 | 开发前 `cat .trellis/spec/...` 会落空 |
| `implement.jsonl` / `check.jsonl` 注入 | 自由路径列表 | 天然支持指向 docs/ 路径，无影响 |
| `.trellis/scripts/get_context.py` 等 | **trellis 源码（不可改）** | 若硬编码 spec 路径需确认行为 |

### 流程事实（与用户描述的差异）

- 用户描述："执行 finish-work 后自动执行 trellis-update-spec"
- 实际流程（workflow.md Phase 3）：`trellis-implement` → `trellis-check` → **`trellis-update-spec`（Phase 3.3，commit 之前）** → commit（Phase 3.4）→ `/trellis:finish-work`（仅归档 + journal，不含 update-spec）
- 即 update-spec 在 finish-work **之前**执行，非之后

### 其他事实

- `.trellis/config.yaml` 无 spec 路径配置项；hooks 仅支持任务生命周期（after_create/start/finish/archive），无路径重定向能力
- `docs/` 目录已存在，含 8 个项目文档（开发规范.md、测试策略.md 等）
- 项目 CLAUDE.md 已有文档索引表

## Assumptions (temporary)

- 继续使用 trellis 工作流其余部分（tasks / journal / finish-work），仅 spec 体系外迁
- `.claude/skills/`、`.claude/commands/` 下的 trellis 文件不动（升级可能覆盖）

## Decisions

- **D1 源码边界**：`.trellis/scripts/` 不可改；`workflow.md`（官方定制层）、`CLAUDE.md`、`docs/` 可改；`.claude/` 下 trellis 安装产物不动。
- **D2 迁移目标**：`docs/` 新建子目录（默认 `docs/spec/`），保留独立文件粒度，未来沉淀沿用同一体系。
- **D3 分流策略**：两级分流——一两句级关键规则直接进 CLAUDE.md；详细契约/主题文档进 `docs/spec/`。
- **D4 非沉淀内容**：guides/ 通用思维指南与空模板文件直接删除，不迁移。
- **D5 scripts 安全性（已验证）**：`.trellis/scripts/` 对 `.trellis/spec/` 无硬依赖（get_context.py / task.py / hooks 均无读取），删目录不影响任务流程。

## Open Questions

（无——全部已决）

## Requirements

1. `cross-layer-contracts.md`（121 行 Wails 跨层契约）完整迁至 `docs/spec/`，用 `git mv` 保留历史
2. 其余 spec 文件（空模板 ×5、index ×2、guides ×3）删除；`.trellis/spec/` 目录随之移除
3. `docs/spec/README.md` 新建：体系用途说明 + 文件索引
4. CLAUDE.md 新增「规范沉淀规则」一节（两级分流 + 路径覆盖声明），并在文档索引表加 `docs/spec/` 入口
5. workflow.md 中所有引导读/写 `.trellis/spec/` 的正文段落改指向 `docs/spec/`（不动 `[required · once]` 步骤标记与 `[workflow-state:*]` 块，规避回归测试约束）
6. 实施后完整跑一遍 task 流程（create → start → archive）验证无 spec 相关报错

## Acceptance Criteria

- [ ] `docs/spec/cross-layer-contracts.md` 内容与原文一致（git mv 无修改）
- [ ] `.trellis/spec/` 不存在，`git status` 干净
- [ ] CLAUDE.md 含沉淀规则：详细文档进 docs/spec/、关键规则进 CLAUDE.md、禁止写 .trellis/spec/
- [ ] workflow.md 无残留的 `.trellis/spec` 写入/读取引导（Phase 1.3 jsonl 指引、Phase 3.3、Trellis System 章节）
- [ ] task 流程 create → start → archive 试跑通过，无 spec 缺失报错
- [ ] CLAUDE.md 文档索引含 docs/spec/ 入口

## Technical Approach

**双保险机制（升级免疫）**：

1. **CLAUDE.md 规则层（主）**：项目 CLAUDE.md 是每次会话必读且 trellis 升级不会覆盖的项目文件。写入沉淀分流规则后，即使 trellis-update-spec skill 指令仍指向 `.trellis/spec/`，CLAUDE.md 的 OVERRIDE 声明使其优先。
2. **workflow.md 引导层（辅）**：官方定制层。改写正文中的 spec 路径引导（jsonl curation、Phase 3.3 描述、Trellis System 说明），使主流程自然指向新位置。

**风险与兜底**：

| 风险 | 兜底 |
|---|---|
| trellis 升级覆盖 `.claude/` skills，update-spec 又指回旧路径 | CLAUDE.md 规则常驻（项目文件，升级免疫） |
| before-dev skill `cat .trellis/spec/...` 落空 | workflow.md 已改指引；CLAUDE.md 规则指路 docs/spec/ |
| jsonl 注入引用旧路径 | jsonl 是自由路径列表，Phase 1.3 指引改为 docs/spec/ |
| scripts 隐性依赖 spec | 已验证零依赖（D5）；AC 含流程试跑 |

**Implementation Plan**：

- 步骤 1：`git mv .trellis/spec/backend/cross-layer-contracts.md docs/spec/`；删除其余 spec 文件与目录
- 步骤 2：新建 `docs/spec/README.md`（索引）
- 步骤 3：CLAUDE.md 加「规范沉淀规则」节 + 索引入口
- 步骤 4：workflow.md 改写 spec 路径引导（正文 only）
- 步骤 5：验证——task 流程试跑 + grep 确认无残留引导 + git 状态检查

## Definition of Done (team quality bar)

- 文档链接全部有效（CLAUDE.md 索引、docs 内部引用）
- 迁移后原内容可追溯（git 历史 + 迁移说明）
- README.md / CLAUDE.md 按项目要求确认是否更新

## Out of Scope (explicit)

- 不改 `.trellis/scripts/` 下任何脚本
- 不动 trellis 任务/日志体系（tasks / workspace / journal）
- （待定）guides 通用指南的内容翻译或重写

## Technical Notes

- 已查文件：`.trellis/config.yaml`、`.trellis/workflow.md`（Customizing 章节）、`.claude/commands/trellis/finish-work.md`（全文 66 行）、`.trellis/spec/` 全目录
- workflow.md "Customizing Trellis (for forks)" 明示：定制通过编辑 workflow.md 完成；改 `[required · once]` 标记须同步改 `[workflow-state:*]` tag 块，否则回归测试不过
- finish-work 拒绝脏工作树（`.trellis/workspace/` 与 `.trellis/tasks/` 之外），docs/ 改动属正常提交范围
