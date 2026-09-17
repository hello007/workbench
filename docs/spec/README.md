# 规范沉淀（docs/spec/）

> 本目录承接原 `.trellis/spec/` 的项目规范沉淀职能（该目录已于 2026-09-07 废弃移除）。

## 体系用途

存放本项目的**详细规范文档**：跨层契约、接口约定、签名同步规则等需要完整上下文（触发条件、签名对照、错误矩阵、正反示例）才能表达的内容。

## 两级分流规则

| 内容级别 | 沉淀位置 | 判断标准 |
|---|---|---|
| 详细契约 / 主题文档 | `docs/spec/<topic>.md` | 需要多节结构（触发条件、契约、测试要求等）才能说清 |
| 一两句级关键规则 | 项目根 `CLAUDE.md`「规范沉淀规则 → 关键规则」清单 | 每个 session 必知、一两句话可完整表达，附指向本目录详细文档的链接 |

执行 trellis 工作流 Phase 3.3（`trellis-update-spec`）时按上述规则分流写入，**禁止写入 `.trellis/spec/`**（该目录已废弃）。

## 文件索引

|文档|说明|
|---|---|
|[cross-layer-contracts.md](cross-layer-contracts.md)|Wails 绑定同步契约：App 方法签名变更须手动同步 `frontend/wailsjs/` 三处（App.js / App.d.ts / models.ts）；文件预览/保存的编码契约（UTF-8 / GBK）|
|[test-coverage-gate.md](test-coverage-gate.md)|测试覆盖率分层门禁：后端 model/server ≥80% + service ≥76% 基线 + util ≥40% 排除 pty_windows.go + 主包不设门禁；前端 ≥70% 硬失败 exclude wailsjs；CI 脚本 scripts/coverage-check.sh 契约|
|[test-stability.md](test-stability.md)|测试稳定性规范（Flaky 规避）：依赖文件系统 mtime 时序判失效的测试在 NTFS 上 flaky，改注入陈旧缓存（modTime 明确落后）驱动失效分支，不用 time.Sleep|
|[app-services-assembly.md](app-services-assembly.md)|AppServices 装配范式：`NewAppServices` 集中装配 + App 内嵌 `*AppServices` 字段提升保 133 委托方法与 Wails 绑定零 diff；AppServices 须在 package main；构造与生命周期副作用分离；新增 service 改动点 2 处|
|[logging-and-errors.md](logging-and-errors.md)|日志与错误处理规范：后端 slog + lumberjack 落盘 `data/logs/app.log`（禁 println）；AppError + Wails ErrorFormatter 结构化 error 跨层传递（源码核实 `CallbackMessage.Err any`）；前端 handleError 按 code 分流；新增错误码同步三处|
|[e2e-testing.md](e2e-testing.md)|E2E 测试规范（方案 C 混合架构）：前端 Playwright E2E（vite preview web 版 + mock Wails 后端，fixtures 注入）+ 后端 Go 集成测试（`//go:build integration` 标签隔离）；mock 单一数据源 `src/test/wails-mock-defaults.js`；`wailsjs/` 不入库，CI 须先 `wails generate module` 再 build|
|[perf-baseline.md](perf-baseline.md)|性能基线（v1.4 PR1）：Go benchmark（FileTree/ScanGitRepos/NewAppServices ns/op+B/op+allocs）+ 前端 bundle 体积（chunk 分布）+ MemStats 快照 + GUI 冷启动占位；PR4 优化前后对比的唯一数据依据；benchmark 不纳入覆盖率门禁|
|[security-scan.md](security-scan.md)|安全扫描（v1.4 PR1）：govulncheck（Go 调用链分析）+ npm audit（官方 registry 绕过 npmmirror）；CI security job `continue-on-error` 不阻塞 PR；已知漏洞清单（xlsx 无补丁接受风险、go-git/go-billy/x/net/标准库已修复）；go.mod `toolchain` directive 与标准库漏洞修复|
|[ai-structured-output.md](ai-structured-output.md)|AI 结构化输出契约（v1.5 AI epic PR1）：claude CLI `--json-schema` tool use 强制结构化，与自由文本 result 解耦；`AiFunction.OutputSchema` → `buildClaudeArgs` → `parseStreamLine` 提取 structured_output → `AiTaskRunResult/AiTaskState.StructuredOutput` 透传前端只渲染不解析；纯 prompt skill 模式（Command 空 + PromptTemplate 驱动，validateFunctions 放宽）|
|[design-tokens.md](design-tokens.md)|设计令牌体系（v1.6 视觉现代化）：双主题变量分层（:root 亮/html.dark 暗，非颜色变量不重复声明）；主色改须同步 Element Plus `--el-color-primary` 派生（light-3~9 实色阶，暗色 light-9 用 rgba 透明度）否则组件割裂；冷暖灰统一蓝灰 Slate 色相禁混；阴影禁纯黑须带背景色相；`--terminal-bg` 须与 `useTerminal.js` LIGHT/DARK_TERMINAL_THEME.background 严格一致（改背景色不动终端）；Geist 四字重梯度 + font-display:swap + fallback 链|

## 新增文档约定

- 命名：`<topic>.md`，topic 用英文小写连字符（如 `cross-layer-contracts.md`）
- 每个文档聚焦一个主题，标题下加一段摘要说明适用范围
- 新增后须同步更新本 README 的文件索引表，并在 `CLAUDE.md`「关键规则」清单补充对应的一句话入口（如有必要）

---

**迁移记录：** 2026-09-07 自 `.trellis/spec/backend/cross-layer-contracts.md` 经 `git mv` 迁入（保留 git 历史）；原 `.trellis/spec/` 下空模板与通用 guides 一并删除。

**最后更新：** 2026-09-16
