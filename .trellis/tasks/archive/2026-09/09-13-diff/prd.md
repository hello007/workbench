# 外部 diff 工具集成

## Goal

在 WorkBench 中集成外部 diff 工具：设置面板可配置外部 diff 工具（预设模板 + 自定义命令行），FileDiffDialog（覆盖工作区 / 提交 / 区间三种 diff 场景）支持一键用外部工具打开左右版本文件。落地路线图「差异工具集成」项，并顺带修正路线图文档漂移。

## Requirements

### 1. 设置面板「外部 diff 工具」配置区

* 新增配置项（`model.AppSettings` 加字段）：
  * `diffToolName` — 预设名：`beyondcompare` / `winmerge` / `vscode` / `custom`
  * `diffToolPath` — 可执行文件路径
  * `diffToolArgs` — 参数模板，支持 `{left}` `{right}` 占位符
* 预设模板选中即填充默认路径 + 参数模板（均可编辑）：
  * Beyond Compare：`BComp.exe` / `{left} {right}`
  * WinMerge：`WinMergeU.exe` / `{left} {right}`
  * VSCode diff：`code` / `--diff --wait {left} {right}`
  * 自定义：用户自行填写
* 不做「自动检测已安装工具」（ADR 决策）；路径用 `el-input` + 文件选择器
* 持久化走现有 `SettingsService` Load/Save，前端 settingsStore 模式与现有设置项一致

### 2. FileDiffDialog「用外部工具打开」按钮

* workspace / commit 单文件模式：弹窗头部按钮
* range 多文件模式：每个文件组标题行小按钮（ADR 决策 1A）
* 工具未配置（路径为空）：按钮置灰 + tooltip「未配置外部 diff 工具，请在设置中配置」
* 点击调用后端 `OpenInExternalDiff`，左右文件由后端生成临时路径传给工具

### 3. 后端打开逻辑（挂现有 service）

* App 新方法 `OpenInExternalDiff`（放 external 域，`app_external.go`），实现挂 `FileOperationService` 或 `GitService`（按依赖归属：需要读 git 内容 + 启进程 + 读设置，倾向新方法放 `FileOperationService`，git 内容获取复用 `GitService` 既有能力或直接 `git show`）
* 流程：按 mode 解析左右版本来源 → `git show <rev>:<file>` 取旧版内容（workspace 左侧 = HEAD 版本）→ 写临时文件（保留原文件名，分 left/right 子目录）→ 按参数模板拼命令 → `util.HideCommandWindow` 启动
* 临时文件：`os.TempDir()/workbench-diff/<唯一子目录>/left/<文件名>` + `right/<文件名>`；启动后不主动删（工具可能 detach），创建前清理旧残留目录
* range 模式：前端逐文件调用，传 `file` + `baseSha`/`headSha`
* 未配置 → 返回 `model.AppError{ErrCodeDiffToolNotConfigured}`；路径无效/启动失败 → `ErrCodeDiffToolLaunchFailed`（新错误码同步 `model/app_error.go` 常量表 + `frontend/src/utils/error.js` ErrorCode/WARNING_CODES）
* Windows 隐藏控制台窗口复用 `util/exec_windows.go` 既有模式

### 4. 路线图文档漂移修正

* E2E 段「三关键流程」更新为现状六流程
* 「全局错误处理」勾选已完成（09-12 交付）
* 「差异工具集成」本任务完成后勾选
* `docs/功能说明.md` 补外部 diff 工具说明

## Acceptance Criteria

* [ ] 后端单测：配置解析（预设模板映射/参数模板渲染）、命令拼装、临时文件写入与清理
* [ ] 前端单测：按钮态（未配置置灰/已配置可用）、预设切换填充逻辑、设置持久化字段
* [ ] 前端 878 单测零回归，覆盖率门禁（后端/前端阈值）达标
* [ ] E2E：仅测按钮态与配置持久化，不触发外部进程
* [ ] `npm run build` 通过（wailsjs 绑定 App.js / App.d.ts / models.ts 三处手动同步）
* [ ] 新错误码双侧同步（app_error.go + error.js）
* [ ] docs/功能说明.md、docs/路线图.md 同步（含三关键流程→六流程、全局错误处理勾选）

## Definition of Done

* 后端 `go test ./...`、前端 `npm test` 全绿，lint 干净
* 文档更新（功能说明 / 路线图 / README 如需）
* 覆盖率门禁零回归

## Decision (ADR-lite)

**Context**：三个边界需决策——range 场景按钮粒度、是否自动检测已安装工具、可选第二任务安排。

**Decision**：
1. range 模式每文件组标题行加按钮（功能完整，成本可控）
2. 不做自动检测；预设选中即填默认路径 + 文件选择器兜底
3. 仓库列表配置导入导出另开任务，本次不做

**Consequences**：range 场景逐文件调用后端，打开多个外部窗口由用户控制；无探测代码，换机器/非默认安装路径需手动选一次；第二任务独立走 trellis 流程。

## Out of Scope

* macOS / Linux 平台适配验证（仅 Windows 主验证）
* 图片/二进制文件外部 diff
* merge 工具（三方合并）集成
* 仓库列表配置导入导出（另开任务）

## Technical Notes

* 关键文件：
  * 后端：service/git.go（diff 层）、service/settings.go、model/settings.go、model/app_error.go、service/fileoperation.go（进程启动先例）、util/exec_windows.go、app_external.go、app_services.go（如需注入）
  * 前端：frontend/src/components/FileDiffDialog.vue、SettingsPanel.vue、settings store、utils/error.js、wailsjs 三处绑定
* 临时文件不还原二进制：二进制文件禁用按钮（与内置 diff 二进制提示一致）
* VSCode `--wait` 使进程阻塞至窗口关闭，便于感知结束；BC/WinMerge 默认阻塞。启动即返回，不等待进程退出（goroutine Wait 仅用于错误日志）
* wailsjs 不入库，CI 先 `wails generate module`；本地改动需手动同步三处（cross-layer-contracts 规范）
* git 取旧版内容：workspace 左侧 = `HEAD:<file>`；commit 左侧 = `<sha>^:<file>`（root commit 用空树 `4b825dc642cb6eb9a060e54bf8d69288fbee4904` 或 git diff 空树约定）；range 左 = `baseSha:<file>`，右 = `headSha:<file>`（或工作区文件）
