# E2E 测试关键流程

## Goal

WorkBench 后端单测覆盖率 model/server ≥80%、service ≥76%，前端 Vitest 878 用例 ≥70%，但关键用户流程（提交/合并/分支管理）无端到端回归保护——功能越多越需要回归护栏。引入 E2E 测试覆盖关键流程，防止重构与新功能破坏用户核心路径。

## Requirements

* E2E 覆盖 3 个关键流程：Git 提交/推送、分支管理（增删改/切换）、合并/变基
* 前端 Playwright E2E：驱动 vite preview web 版 + mock Wails 后端（复用 setup.js mock 模式）
* 后端 Go 集成测试：扩展 go test，service 链 + 临时 git 仓库 fixture
* 不破坏现有单测与构建流程
* E2E 可在本地与 CI（GitHub Actions）运行

## Acceptance Criteria

* [ ] Playwright + vite preview + mock 后端脚手架落地，可本地运行
* [ ] 3 个关键流程各有前端 E2E 用例且通过
* [ ] 后端集成测试覆盖同样 3 个流程（临时 git 仓库 fixture）且通过
* [ ] GitHub Actions ci.yml 落地：push/PR 触发，Linux runner 跑 go test + npm test + E2E
* [ ] 不破坏现有 go test / npm test / wails build
* [ ] E2E 稳定非 flaky（连续多次运行结果一致）

## Definition of Done

* E2E 测试稳定通过（非 flaky）
* 现有单测与构建不受影响
* CLAUDE.md / 路线图同步
* spec 文档更新（E2E 规范）

## Technical Approach

**方案 C 混合**（用户已确认 2026-09-13）：

* 前端：Playwright 驱动 vite preview（web 版），Wails 后端用 setup.js mock 模式替换——复用现有单测 mock 基础设施
* 后端：Go 集成测试（go test 扩展），t.TempDir 建临时 git 仓库 fixture，测 service 层调用链
* 契约一致性：mock 行为与真实 Go 服务靠 docs/spec/cross-layer-contracts.md 纪律对齐
* CI：GitHub Actions Linux runner（remote Gitee 自动同步 GitHub，Actions 在 GitHub 侧执行，现有 release.yml 即此模式）

## Decision (ADR-lite)

**Context**: Wails v2.12.0 桌面应用需 E2E 回归护栏。研究 [research/e2e-approach.md] 核实：Wails v2 无官方 E2E 支持；Playwright 驱动真实 WebView2 桌面应用技术可行（CDP 注入 + connectOverCDP，微软官方背书）但 CI 须 Windows、CDP flaky、社区无 Wails 成熟范例、侵入性高（注入环境变量）。项目已有 setup.js mock Wails bound method 基础设施可复用。

**Decision**: 采用方案 C 混合——前端 Playwright E2E（vite preview + mock Wails 后端）+ 后端 Go 集成测试（临时 git 仓库 fixture）。CI 走 GitHub Actions Linux runner（Gitee 自动同步 GitHub）。不破坏现有 go test / npm test / wails build。

**Consequences**:
- 非真"端到端"——前后端交界有缝隙，mock 与真实 Go 行为契约漂移靠 spec 纪律保障
- 维护两套 fixture（前端 mock 数据 + 后端临时 git 仓库）
- Approach A 真桌面 CDP 列为未来可选增强（低优先，需 PoC 验证 connectOverCDP 稳定性）

## Out of Scope (explicit)

* 全量 E2E 覆盖（仅关键流程）
* 后端单测 / 前端单测改动（保持现状）
* 性能测试
* Approach A 真桌面 CDP PoC（未来任务）
* submodule 管理、文件树操作、AI 功能触发的 E2E（本期排除，后续按需追加）

## Research References

* [`research/e2e-approach.md`](research/e2e-approach.md) — Wails v2 无官方 E2E；Playwright 驱动 WebView2 技术可行（CDP 注入）但 CI 成本高 flaky；推荐 Approach C 混合（前端 Playwright mock + 后端 Go 集成测试），Linux CI，复用 setup.js mock 基础设施

## Technical Notes

* Wails v2.12.0，WebView2 Windows；remote Gitee（自动同步 GitHub），Actions 在 GitHub 侧执行
* 现有测试基线：go test -race 全绿、npm test 878
* 现有 CI：仅 release.yml（tag 触发发版，Windows runner）；无 push/PR 测试 CI，本期新增 ci.yml
* 相关 spec：[test-coverage-gate.md](docs/spec/test-coverage-gate.md) / [test-stability.md](docs/spec/test-stability.md)

## Implementation Plan (small PRs)

* PR1: Playwright 脚手架——依赖、playwright.config、vite preview 集成、mock 后端复用 setup.js、1 条冒烟用例
* PR2: 前端 E2E 三流程用例（提交/推送、分支管理、合并/变基）+ mock fixture 扩展
* PR3: 后端 Go 集成测试——临时 git 仓库 fixture + 三流程 service 链测试
* PR4: GitHub Actions ci.yml + 文档同步（CLAUDE.md 常用命令、docs/测试策略.md、E2E spec）
