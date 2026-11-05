<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->

> **本仓覆盖层纠偏**（2026-10-29 追加，位于管理块外，`trellis update` 不会覆盖）：块内 `.trellis/spec/` 描述已失效——该目录已于 2026-09-07 废弃移除，规范沉淀实际位于 `docs/spec/`（索引见 docs/spec/README.md）与 CLAUDE.md「关键规则」清单，勿按块内旧路径查找或写入规范。

---

# 项目约定（AI 编程 Harness 导览）

> 以下内容为 AI 编程规范体系（2026-11-05 初始化），与 Trellis 块及仓根 CLAUDE.md 并存：任务分流走 Trellis，深度契约走 `docs/spec/` 与 CLAUDE.md「关键规则」，本文承载**通用工程导览**（概览、命令、架构速查、规范入口）。
> 文中路径一律用反引号标注，表示**按需读取**——需要时再打开，不要一次性全读。

## 1. 项目概览

- **项目名称**：WorkBench - 开发者工作台
- **项目类型**：Wails 桌面应用（前后端同仓，另支持浏览器访问模式）
- **技术栈**：Go 1.26.10 + Wails v2.16.0；Vue 3.5 + Element Plus 2.14.5 + Pinia 4.1 + Vue Router 5.4 + Vite 8（JavaScript，非 TypeScript）
- **交付形态**：Windows / Linux 桌面可执行文件（Linux 需 `-tags webkit2_41`）
- **版本管理**：版本号真相源在 `wails.json`，发版走 `/release` skill（`scripts/release.sh`）

**影响面说明**：
- App 方法签名与 `model/` 导出 struct 是前后端契约真相源，变更必须同步前端消费面。
- `frontend/wailsjs/` 与 `frontend/dist/` 为生成产物不入库，任何层禁止手改。

## 2. 常用命令

**执行前提**（构建拓扑闭环，必须成组执行）：
- 工作目录：Go 命令从**仓根**执行，前端命令在 `frontend/` 下执行；
- **干净检出后**，任何 `go build` / `go vet` / `go test` / `npm run build` 之前必须先：
  ```bash
  wails generate module                    # 生成 frontend/wailsjs/
  mkdir -p frontend/dist/assets && echo '<!doctype html>' > frontend/dist/index.html && echo '/* placeholder */' > frontend/dist/assets/app.js
  ```
  否则 `//go:embed all:frontend/dist` 报 no matching files found。

**常用命令**：

```bash
# 本地运行 / 调试
wails dev

# 构建应用
wails build                      # Linux 加 -tags webkit2_41

# Go 静态检查与测试（仓根）
go vet ./...
go test ./...
go test -tags=integration ./...

# 前端构建与测试（frontend/ 下）
npm run build
npm test
npm run test:coverage
npm run e2e                      # Playwright E2E，首次先 npm run e2e:install

# 依赖一致性（仓根，零安装零网络）
node scripts/check-dep-consistency.mjs
```

本仓无 lint / format 命令（现状守护决策，门禁以 CI 为准，详见 `docs/agent-resources/conventions/process-build-quality.md`）。

## 3. 架构速查

```
Vue 组件 → transport（双通道收口）
   ├─ 桌面模式：Wails 绑定（wailsjs 生成）
   └─ 浏览器模式：WebSocket + HTTP RPC（server/ 通道）
        ↓
Go 桥接层（app_*.go）→ AppServices（app_services.go）→ service/ 业务层
        ↓
model/（契约真相源）、util/（工具）、server/（浏览器通道）
```

依赖方向严格单向向下；跨层契约与存量例外以 `docs/agent-resources/conventions/project-boundaries.md` 与 `docs/spec/` 为准。详细架构见 `docs/agent-resources/reference/architecture.md`。

## 4. 开发规范

通用工程规范在 `docs/agent-resources/conventions/`（入口：`docs/agent-resources/index-convention.md`），写代码前按需读取。

**体系分工**（本仓三层规范，冲突时优先级从高到低）：
1. 仓根 `CLAUDE.md`「关键规则」+ `docs/spec/` — 本项目踩坑沉淀的深度契约（最优先）
2. `docs/agent-resources/conventions/` — 跨技术栈通用工程规范
3. `.trellis/workflow.md` — 任务分流与流程

## 5. 硬性约束

### Git
- **不主动提交、不主动推送**：agent 不得未经触发自行执行 `git commit` / `git push`；完成任务后停止并报告，提交由用户决定。
- **用户显式触发提交流程时必须执行，不得推回**：用户明确指定提交方式时按其要求完成。
- **push 始终手动**；共享 master 禁 `--force`，推送前 fetch + rebase（细则见 `docs/开发规范.md`）。

### 安全与兼容性
- **绝不提交敏感凭据**：不提交真实密码、Token、生产证书或私钥；内网地址与凭据不得写入本仓库（远端为 GitHub）。
- **向后兼容**：绑定方法、`model/` struct、配置 key、错误码变更默认保持向后兼容。
- **禁止降级检查**：严禁为了测试通过而删除校验逻辑、跳过安全检查或吞没异常。

### 工作流程（任务分流）

**任务分流由 Trellis 承担**：每回合 workflow-state 注入判定（问答直答 / 建任务走 `trellis-brainstorm` / 内联小修须用户明示）。场景级分步剧本（开发、修 Bug、写文档、Code Review）按需读 `docs/agent-resources/workflows/`（索引：`index-workflows.md`）。

### 代码搜索与检索
- 代码结构、调用链、影响面优先使用 CodeGraph（`codegraph_*` 工具）；
- 文本与关键字检索使用 grep；
- 已知具体文件直接使用阅读工具按需查看。

## 6. 常见坑点

1. **生成产物缺失假失败**：干净检出直接跑 `go build`/`vite build` 因 `wailsjs/`、`dist/` 缺失报错——先按 §2 前置链生成，不是代码问题。
2. **wailsjs 手改失效**：`frontend/wailsjs/` 是生成产物，手改下次 generate 即被覆盖；契约变更必须改 Go 侧真相源。
3. **中文编码双重乱码**：前端还原 base64 二进制文本时裸 `atob` 会把中文搞乱，须走项目 `decodeBase64Utf8`。
4. **模式分流时机**：桌面/浏览器双模式判定依赖启动前安装顺序（`installBrowserTransport()` 在 mount 前），调整 import 顺序可能静默破坏浏览器模式。

其余已知坑查阅 `docs/agent-resources/index-troubleshooting.md`（初始为空，典型排障与解法应沉淀至此，避免重复排查）。

## 7. 该读哪份文件

`docs/agent-resources/` 下按**内容性质**分类，都入库共享、agent 通用，按需读取（总导览与 SSOT 裁判表：`docs/agent-resources/README.md`）：

| 类别 | 位置 | 性质 |
|------|------|------|
| 约定 | `docs/agent-resources/conventions/` + `index-convention.md` | 该怎么做。祈使句，能判定违规 |
| 决策记录 | `docs/agent-resources/adrs/` + `index-adr.md` | 当初为什么这么定。不可变，状态机 |
| 参考 | `docs/agent-resources/reference/` + `index-reference.md` | 现状是什么。客观描述，易腐 |
| 排障库 | `docs/agent-resources/troubleshooting/` + `index-troubleshooting.md` | 已验证报错解法，带审核到期 |
| 工作流 | `docs/agent-resources/workflows/` + `index-workflows.md` | 怎么做某类事。场景剧本 |
| 临时会话记忆 | `docs/agent-resources/temp-memory/` | 待办/决策/套路。每条都有出口 |

常用入口：
- 查通用代码规范 → `docs/agent-resources/index-convention.md`
- 查本项目深度契约 → `docs/spec/` + 仓根 CLAUDE.md「关键规则」
- 查看待办与欠账 → `docs/agent-resources/temp-memory/TEMP-MEM-activeContext.md`
- 选择场景剧本 → `docs/agent-resources/index-workflows.md`
