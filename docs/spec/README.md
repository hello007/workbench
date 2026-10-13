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
|[data-atomic-write.md](data-atomic-write.md)|data JSON 落盘原子写契约：全部 data JSON 持久化必须走 `util.SaveJSON`（CreateTemp 同目录 temp → fsync → os.Rename 替换，Windows 为 MoveFileEx REPLACE_EXISTING），禁新增 os.WriteFile 直写与自建 temp+rename（saveChatJSON/atomicWriteConfig 已收敛委托）；崩溃残留 `.<目标名>.tmp-*` 点文件无害不自动清扫；原子写不解决整体覆盖丢数据（last-writer-wins），手改 data 文件前仍须关 workbench.exe|
|[app-services-assembly.md](app-services-assembly.md)|AppServices 装配范式：`NewAppServices` 集中装配 + App 内嵌 `*AppServices` 字段提升保 133 委托方法与 Wails 绑定零 diff；AppServices 须在 package main；构造与生命周期副作用分离；新增 service 改动点 2 处|
|[logging-and-errors.md](logging-and-errors.md)|日志与错误处理规范：后端 slog + lumberjack 落盘 `data/logs/app.log`（禁 println）；AppError + Wails ErrorFormatter 结构化 error 跨层传递（源码核实 `CallbackMessage.Err any`）；前端 handleError 按 code 分流；新增错误码同步三处|
|[e2e-testing.md](e2e-testing.md)|E2E 测试规范（方案 C 混合架构）：前端 Playwright E2E（vite preview web 版 + mock Wails 后端，fixtures 注入）+ 后端 Go 集成测试（`//go:build integration` 标签隔离）；mock 单一数据源 `src/test/wails-mock-defaults.js`；`wailsjs/` 不入库，CI 须先 `wails generate module` 再 build|
|[perf-baseline.md](perf-baseline.md)|性能基线（v1.4 PR1）：Go benchmark（FileTree/ScanGitRepos/NewAppServices ns/op+B/op+allocs）+ 前端 bundle 体积（chunk 分布）+ MemStats 快照 + GUI 冷启动占位；PR4 优化前后对比的唯一数据依据；benchmark 不纳入覆盖率门禁|
|[security-scan.md](security-scan.md)|安全扫描（v1.4 PR1）：govulncheck（Go 调用链分析）+ npm audit（官方 registry 绕过 npmmirror）；CI security job `continue-on-error` 不阻塞 PR；已知漏洞清单（xlsx 无补丁接受风险、go-git/go-billy/x/net/标准库已修复）；go.mod `toolchain` directive 与标准库漏洞修复|
|[ai-structured-output.md](ai-structured-output.md)|AI 结构化输出契约（v1.5 AI epic PR1）：claude CLI `--json-schema` tool use 强制结构化，与自由文本 result 解耦；`AiFunction.OutputSchema` → `buildClaudeArgs` → `parseStreamLine` 提取 structured_output → `AiTaskRunResult/AiTaskState.StructuredOutput` 透传前端只渲染不解析；纯 prompt skill 模式（Command 空 + PromptTemplate 驱动，validateFunctions 放宽）|
|[design-tokens.md](design-tokens.md)|设计令牌体系（v1.6 视觉现代化）：双主题变量分层（:root 亮/html.dark 暗，非颜色变量不重复声明）；主色改须同步 Element Plus `--el-color-primary` 派生（light-3~9 实色阶，暗色 light-9 用 rgba 透明度）否则组件割裂；冷暖灰统一蓝灰 Slate 色相禁混；阴影禁纯黑须带背景色相；`--terminal-bg` 须与 `useTerminal.js` LIGHT/DARK_TERMINAL_THEME.background 严格一致（改背景色不动终端）；Geist 四字重梯度 + font-display:swap + fallback 链|
|[frontend-visual-conventions.md](frontend-visual-conventions.md)|前端页面风格开发规范（v1.6 视觉现代化，与 design-tokens 互补）：配色按语义选色（primary/success/warning/danger/info）+ 蓝灰三档中性色阶（bg/text/border）；VSCode 式三列布局（ActivityBar 48px + FileTreePanel + ContentPanel）+ 面板 nav+content 二分；组件视觉模式（nav active 左侧指示条 `left:0` 防 overflow 裁剪禁负 left、卡片 hover `--shadow-sm`→`--shadow-md` 层级、section-title 字重梯度 h1-h3 600/`-0.01em` + h4-h6 500、kbd Geist 字体族、行内 style 禁用迁语义类）；四档间距/圆角内紧外松就近归并；弹窗 `min(像素,视口比例)` 响应式尺寸分档（设置 960×620、命令面板 720×480、小弹窗 420-600）；本文指南层引用变量名不重述取值|
|[browser-channel.md](browser-channel.md)|浏览器访问通道契约（v1.7 浏览器模式 epic）：service 层事件唯一出口 EventSink（wailsEventSink/WSHub/multicastSink 三实现 + sinkHolder 切换防竞态）；/api/rpc reflect 翻译层协议与 E_RPC_* 错误矩阵；WS 帧格式与升级前认证；token 三通道与热轮换语义；polyfill 注入时序（wrapper 调用时才解引用 window.go）；serve 模式 wailsRuntimeUnavailable 守卫（runtime.EventsEmit/对话框对非 Wails ctx 会 log.Fatalf 杀进程）|
|[terminal-multi-session.md](terminal-multi-session.md)|终端多会话契约（v1.7 终端优化 epic）：xterm 多实例（禁重挂载、隐藏容器 fit 静默失败、激活 refit、EventsOn 闭包注销）；tab 状态与实例分离（MAX_TERMINAL_TABS=8 前端单一来源）；快照 v2 多 tab 归一化（WorkDir 兼容降级、幂等、置空防复写）；Vue watch 多源引用比较陷阱（getter 投影）；全屏 z-index 层级表 1500 + fullscreen⇒visible 不变式 + ESC 冒泡过滤；外观设置热更契约（el-input-number 必带 :value-on-clear）|
|[process-management.md](process-management.md)|进程管理契约（工具箱「进程管理」）：三个 Wails 方法签名与 `mode` 禁用具名 string（MISSING_EXPORT 坑）；`IsSelf`/`IsSystem` 后端填充前端只消费；自我保护三层且**判据只用 PID**（E2E 改名 workbench-e2e.exe 致进程名判据失效）；**Windows 优雅停止能否生效取决于目标进程是否响应、无法按「有无窗口」预判**（本机实测 ping 响应退出 / cmd /k 拒绝且存活，推翻常见结论），故统一「发请求 → 5s 轮询实测存活 → 未退转强杀」，`SUCCESS` 只代表信号送达不等于已退出；默认视图状态无法识别时保守保留防中文系统空列表；netstat/tasklist 解析三硬约束 + 失败须拼 stderr + 终止命令必带超时|
|[pptx-split.md](pptx-split.md)|PPT 按页拆分契约（工具箱「PPT 拆分」）：纯 Go OOXML 最小编辑集（sldIdLst/rels/Content_Types/删 slide+notesSlide/剥离 custShowLst+extLst/跨页链接清理）与页序三跳映射（slide 文件名 ≠ 页序）；XML 编辑用「解析定位 + 字节删除」禁 Encoder 重序列化（保真度）；OPC 关系 Target 相对关系源 part 目录（非 `_rels/` 目录，实测踩坑）；取消复用 E_GIT_CANCELLED 不新增错误码；事件 `pptx-split:*` 仅 PptSplitDialog 监听（右键入口 Promise 直返规避共听面）；fixture 程序化构造禁依赖本机 Office|
|[assets-management.md](assets-management.md)|素材管理契约（侧边栏「素材管理」）：SQLite 只用 modernc.org/sqlite 纯 Go 零 CGO；FTS5 中文检索五契约（切分同源 splitFtsText 单点、detail=full 必须保留、standalone 表事务双写、trigram 禁用——2 字词静默 0 命中、弃 snippet() 改原文窗口）；单连接池自锁坑（SetMaxOpenConns(1) 下 rows 未 Close 再查 = 永久死锁）；FTS rowid 编码（文件 +id/幻灯片 -id）；派生产物排除按真实拆分布局 `<源文件名>/<源文件名>-NN.pptx` 同名文件夹判定（禁凭空目录名）；user_version 迁移 + Go 回填；事件 assets-scan:* 带 libraryId；standalone FTS 表不支持 'delete-all' 命令|

## 新增文档约定

- 命名：`<topic>.md`，topic 用英文小写连字符（如 `cross-layer-contracts.md`）
- 每个文档聚焦一个主题，标题下加一段摘要说明适用范围
- 新增后须同步更新本 README 的文件索引表，并在 `CLAUDE.md`「关键规则」清单补充对应的一句话入口（如有必要）

---

**迁移记录：** 2026-09-07 自 `.trellis/spec/backend/cross-layer-contracts.md` 经 `git mv` 迁入（保留 git 历史）；原 `.trellis/spec/` 下空模板与通用 guides 一并删除。

**最后更新：** 2026-10-13
