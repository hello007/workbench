# v1.4 平台加固：性能 / 稳定性 / 文档 / 技术债

## Goal

v1.3.0 AI 功能全量交付后，进入平台加固阶段。在协作、云集成、国际化三块暂缓的前提下，将其余路线图剩余项（技术债、稳定性/崩溃恢复、文档完善、性能优化）整合为单一 epic 任务，按步分 PR 推进，提升 WorkBench 的运行质量、可维护性与上手门槛。

推进顺序（已确认）：**技术债 → 稳定性 → 文档 → 性能**。技术债先打底降后续风险并建立性能基线，稳定性内在质量紧随，文档外在产出，性能据基线数据决策优化项收尾。

## What I already know

### 启动与运行时

* `main.go`：`NewApp` → `SettingsService.Load` → `wails.Run`，启动链路极简
* `app.go startup`：纯构造 `NewAppServices` + 轻量副作用（`CleanupDiffTempDir`、`StartHistoryCleanup`、`CheckPendingUpdate`），Go 侧启动优化空间有限，瓶颈在 WebView2 初始化与前端 bundle 体积
* `shutdown`：关闭 terminal / aiFunc service，无工作状态快照

### 测试现状

* 后端 50 个 `_test.go`，无 `testutil.go` / `helpers_test.go`，fixture 与辅助函数各文件重复
* 前端已有 `src/test/wails-mock-defaults.js`（vitest/E2E 共用单一数据源）+ `src/test/setup.js`
* 分层覆盖率门禁已建（model/server ≥80%、service ≥76%、util ≥40%、前端 ≥70%，见 `docs/spec/test-coverage-gate.md`）
* 集成测试 `//go:build integration`、E2E 方案 C 混合架构均已落地

### 文档现状

* 功能文档齐全：功能说明 / 开发工作流 / 部署说明 / 开发规范 / 常见问题 / 路线图 / 测试策略 / project-context
* spec 沉淀：cross-layer-contracts / app-services-assembly / logging-and-errors / e2e-testing / test-coverage-gate / test-stability
* 缺：用户快速入门指南、架构设计文档、API 文档、贡献指南
* 历史遗留：`docs/plans/` 60+ 文件（2025-04 起）、`docs/superpowers/plans|specs/` 旧目录，未归档清理

### 依赖

* 实际 Go 工具链 `go1.26.2`（`go env GOVERSION` 确认），`go.mod` 声明 `go 1.24.0` 为最低兼容线，GOTOOLCHAIN=auto，无矛盾
* 关键依赖：Wails v2.12.0、go-git v5.18.0、golang.org/x/sys v0.38.0、lumberjack v2.2.1
* npm：vite 8.0.10、vitest 4.1.5、element-plus 2.13.7、vue 3.5.33、pinia 4.0.3，整体较新
* 无自动化安全扫描 / 依赖审计

### 稳定性

* 全局错误处理已建（AppError + ErrorFormatter + slog 落盘，见 `docs/spec/logging-and-errors.md`）
* 无崩溃恢复：未持久化 UI 状态（当前打开仓库、激活 tab、未提交表单），崩溃后冷启动
* 更新服务已有 pending update 机制（`CheckPendingUpdate`），可复用其持久化模式

## Requirements

### PR1：技术债清理 + 性能基线（先打底）

* **测试重构**：抽后端测试辅助。按包就近建 `*_test_helper.go` 或 `util/testutil`，提取重复 fixture 工厂、临时仓库构造、断言助手；与前端 `wails-mock-defaults.js` 单一数据源模式对齐
* **历史文档归档**：`docs/plans/` 60+ 与 `docs/superpowers/` 旧目录归档至 `docs/archive/`（git 历史可追溯，不删内容只挪位），更新文档索引
* **依赖升级**：Go minor/patch 升级（`go get -u` 谨慎，逐包验证）、npm `npm update`、Wails 跟官方；major 版本单独评估不在本轮
* **安全扫描接入**：Go `govulncheck` + npm `npm audit`，纳入 CI（ci.yml 加 job），已知漏洞清单与处置
* **性能基线建立**：冷启动耗时（WebView2 初始化 + Go startup 各阶段 + 前端首屏）、内存占用快照、大型仓库（1000+ 文件）文件树加载耗时；记录到 `docs/spec/perf-baseline.md`，作为 PR4 优化决策依据
* **覆盖率门禁复核**：升级后复核分层门禁数值是否需调整，同步 `docs/spec/test-coverage-gate.md`

### PR2：稳定性 — 崩溃恢复

* **UI 状态快照**：持久化当前打开的仓库、激活视图/tab、目录树展开状态、终端会话标识；崩溃/异常退出后下次启动恢复
* **快照写入策略**：复用 settings.json 持久化模式，独立 `data/session.json`；debounce 写入避免高频 IO；shutdown 正常退出写最终快照
* **恢复交互**：启动检测到会话快照时恢复 UI 状态；可选提示用户"已恢复上次会话"（轻量 toast，非阻塞）
* **边界**：仅恢复 UI 状态，不恢复未提交代码/未保存编辑器内容（超出 MVP）；终端会话不真实复用进程，仅恢复标识与工作目录
* **错误兜底**：快照文件损坏/版本不匹配时降级为冷启动，不阻塞启动

### PR3：文档完善

* **用户面向**：`docs/快速入门.md`——安装、首次配置、核心流程（开仓/提交/分支/AI 功能）截图步骤
* **开发面向**：`docs/架构设计.md`（分层 model/service/util + Wails 桥接 + 前端 Pinia store 拆分）、`docs/贡献指南.md`（环境/分支策略/提交规范/PR 流程/spec 沉淀规则）
* **API 文档**：按需——Wails 绑定方法清单（App.js/App.d.ts 自动生成 + 关键方法语义说明），或引用 codegraph 索引
* **README 同步**：CLAUDE.md 项目要求，README.md 与新增文档交叉引用
* **路线图同步**：勾选本轮完成项，更新最后更新日期与文档版本

### PR4：性能优化（据 PR1 基线决策）

* **启动时间**：据基线数据——若前端 bundle 过大走代码分割/懒加载非关键路由；若 Go startup 阶段有阻塞走延迟加载非关键 service
* **内存使用**：据基线——文件树节点上限、大文件分块读取、及时释放大对象
* **大型仓库**：据基线——文件树加载瓶颈优化（虚拟滚动已有，补增量加载/深度限制调优）
* **回归保障**：优化后重跑 PR1 性能基线对比，量化收益写入 `docs/spec/perf-baseline.md`
* **约束**：每项优化须有基线数据支撑，禁止盲目优化

## Acceptance Criteria

### PR1

* [ ] 后端测试辅助提取完成，至少 3 个包的重复 fixture 收敛，测试全绿零回归
* [ ] `docs/plans/` 与 `docs/superpowers/` 归档至 `docs/archive/`，文档索引更新
* [ ] Go/npm 依赖 minor/patch 升级完成，`go test ./...` + `npm test` + E2E 全绿
* [ ] `govulncheck` + `npm audit` 接入 CI，已知漏洞清单归档
* [ ] `docs/spec/perf-baseline.md` 建立，含冷启动/内存/1000+ 文件树三项基线数据
* [ ] 覆盖率门禁复核，`test-coverage-gate.md` 同步（如数值变更）

### PR2

* [ ] `data/session.json` UI 状态快照写入/读取，debounce + shutdown 双触发
* [ ] 崩溃后启动恢复打开仓库/激活 tab/目录树展开状态
* [ ] 快照损坏降级冷启动，不阻塞启动（有测试覆盖）
* [ ] 终端会话标识恢复，工作目录正确（不复用进程有说明）
* [ ] 单测 + E2E 覆盖恢复路径

### PR3

* [ ] `docs/快速入门.md` 完成，含核心流程步骤
* [ ] `docs/架构设计.md` + `docs/贡献指南.md` 完成
* [ ] API 文档按需完成（绑定方法清单或 codegraph 引用）
* [ ] README.md 与新文档交叉引用同步
* [ ] 路线图勾选本轮完成项，日期/版本号更新

### PR4

* [ ] 每项优化有 PR1 基线数据支撑（before/after 量化）
* [ ] 冷启动/内存/文件树至少一项可量化改善
* [ ] 优化后重跑基线对比写入 `docs/spec/perf-baseline.md`
* [ ] 测试全绿零回归

## Definition of Done

* 测试新增/更新（单测/集成/E2E 按需），覆盖率门禁零回归
* lint / typecheck / CI 全绿
* 文档与 README 同步更新（CLAUDE.md 项目要求）
* spec 沉淀按 `docs/spec/` 两级分流规则
* 风险项考虑回滚/灰度

## Technical Approach

* **epic 单任务分 PR**：不拆独立 subtask，`09-14-v1-4` 下按 PR1-4 序列推进，每 PR 独立提交验证
* **测试重构**：就近建包级 `*_test_helper.go`（Go 测试辅助不跨包导出更安全），跨包共用入 `util/testutil`
* **崩溃恢复**：复用 settings.json 持久化模式 + update service pending 机制经验；`data/session.json` 独立文件避免与设置耦合
* **性能优化**：严格先测后优，PR1 基线是 PR4 前置依赖
* **文档归档**：`git mv` 保留历史，仅挪目录不删内容

## Decision (ADR-lite)

**Context**: v1.3.0 后剩余路线图项分散于性能/稳定性/文档/技术债四域，协作/云集成/国际化暂缓，需确定推进策略与边界。

**Decision**:
1. 整合为单一 epic `09-14-v1-4`，分 PR1-4 序列推进，不拆 subtask
2. 顺序：技术债 → 稳定性 → 文档 → 性能（技术债打底 + 建性能基线，性能据基线收尾）
3. 协作/云集成/国际化三块路线图标 ⏸️ 暂缓 v1.4+
4. Git 边缘项（推送结果面板、三向合并）排除本轮，正交于平台加固主题
5. 崩溃恢复 MVP 限 UI 状态快照，不含未提交代码/编辑器内容恢复

**Consequences**:
* 优势：单 epic 聚焦平台加固，PR 序列清晰，性能优化有数据支撑避免盲目
* 风险：epic 跨度大周期长，需每 PR 严格收口防 scope 蔓延；测试重构可能触及大量测试文件
* 后续：协作/云集成/国际化 v1.4+ 独立启动；Git 边缘项按需独立成项

## Out of Scope

* 协作功能团队工作空间模板（路线图已标 ⏸️ 暂缓 v1.4+）
* GitHub / GitLab 云集成（路线图已标 ⏸️ 暂缓 v1.4+）
* 多语言 i18n 支持（路线图已标 ⏸️ 暂缓 v1.4+）
* Git 推送结果独立面板（路线图标边缘项/暂不实现）
* Git 三向合并支持（diff 工具后续迭代，可独立成项）
* 崩溃恢复中的未提交代码/编辑器未保存内容恢复（超 MVP）
* major 依赖版本升级（单独评估）

## Technical Notes

* 启动优化先测后优：WebView2 冷启动耗时 + 前端 bundle 体积 + Go startup 各阶段耗时，建基线再决策
* 崩溃恢复可复用 settings.json 持久化模式 + update service pending 机制
* 测试重构抽 `util/testutil` 或各包 `xxx_test_helper.go`，与现有 mock-defaults 模式对齐
* `docs/plans/` 与 `superpowers/` 历史目录 `git mv` 至 `docs/archive/` 保留历史
* 依赖升级分批：Go minor/patch 季度、npm 月度、Wails 跟官方；major 单独评估
* `go.mod` 的 `go 1.24.0` 为最低兼容声明，实际工具链 `go1.26.2`，GOTOOLCHAIN=auto，无矛盾
