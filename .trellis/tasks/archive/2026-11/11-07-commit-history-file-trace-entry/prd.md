# 提交历史 → 文件级追溯入口衔接

## Goal

补全追溯网络的反向链路：仓库提交历史（CommitHistory）展开的变更文件、提交 diff（FileDiffDialog）中的文件，可一键进入该文件的提交历史（FileHistoryDialog）或行归属（blame），形成「提交 → 文件 → 行」与既有「仓库 → 文件 → 行」的双向闭环。

## Requirements

1. **三处右键入口**（点击语义均不变）：
   - CommitHistory 提交详情变更文件 tag：右键菜单「查看文件历史」「行归属」；点击仍打开该文件 diff
   - FileDiffDialog commit 模式头部文件名：同款右键菜单
   - FileDiffDialog range 模式分组头文件名：同款右键菜单
2. **「查看文件历史」**：打开 FileHistoryDialog，`fileAbsPath = repoPath + relPath`（绝对路径换算），`initialSha` = 所在提交（commit 模式 = props.sha；range 模式 = headSha；workspace/LocalChanges 模式 = 空）
3. **「行归属」**：`CheckFileExists(fileAbsPath)` 点击时判定——存在 → 切换选中节点到该文件 → previewFile 打开预览 → FilePreviewRenderer `autoBlame` 自动开启 blame；不存在 → toast「文件已不在工作树，已转入文件历史追溯」+ 直接打开 FileHistoryDialog（带 initialSha）
4. **autoBlame 遵守既有 blame 入口条件**：json/jsonc/json5（派生视图）与 markdown/HTML 渲染视图不自动开启，此类文件点「行归属」降级为 toast 提示 + 打开文件历史
5. **后端新增 `CheckFileExists(path) bool`**（轻量 os.Stat 语义）；须同步 RPCHiddenMethods 清单（纯桌面数据面方法）+ wailsjs 两处（App.js / App.d.ts，bool 返回无 models.ts 需求）

## Acceptance Criteria

- [ ] CommitHistory 变更文件右键可进 FileHistoryDialog 且定位到所在提交（initialSha 高亮生效）
- [ ] FileDiffDialog commit/range 两形态文件右键可进 FileHistoryDialog 且定位（range 定位到 headSha）
- [ ] 「行归属」对工作树存在的文件：切预览 + 自动开启 blame 标注
- [ ] 「行归属」对已删除文件：降级 toast + 转入 FileHistoryDialog，追溯链不中断
- [ ] json/jsonc/json5 与 markdown/HTML 渲染视图文件：不自动开 blame，降级提示
- [ ] FileDiffDialog 4 处挂载点（LocalChanges / CommitHistory×2 / FileHistoryDialog 内嵌）右键功能均可用
- [ ] CheckFileExists 后端单测（存在/不存在/空路径/目录）+ wailsjs 同步
- [ ] 前端单测覆盖右键菜单、降级分支、autoBlame 条件；`npm test` / `npm run build` 绿；`go vet` / `go test` 绿

## Definition of Done

- 单测覆盖新增入口逻辑与降级分支（含语义锚 data-action）
- 覆盖率门禁：前端 ≥70%（后端 CheckFileExists 在主包，无门禁）
- docs/spec 更新：file-level-history.md 增补「提交历史入口」节、git-blame.md 增补 autoBlame 契约、browser-channel.md 记录 RPCHiddenMethods 变更
- README.md 按文档语言约定确认是否需更新

## Technical Approach

### 路径口径（勘察结论，rename 天然正确）

`commit.files` 来自 `--name-only`（commit_history_cli.go，无 `-M` 但 Git ≥2.9 默认 rename 检测开启）→ rename 提交只列新路径 = pathAtCommit 语义，拼接仓库根即正确，无 rename 特判。`diff.renames=false` 边缘下列旧路径：FileHistoryDialog --follow 对旧路径仍可追溯，blame 走存在性降级，逻辑自洽。

### 组件改动面

| 组件 | 改动 |
|---|---|
| CommitHistory.vue | file tag `@contextmenu` 右键菜单；自挂 FileHistoryDialog（对齐 FileTreePanel.vue:500 先例）；「行归属」经联动通道发请求 |
| FileDiffDialog.vue | commit 头部 + range 分组头文件名右键菜单；自挂 FileHistoryDialog（对齐 FilePreviewRenderer 先例）；「行归属」经联动通道 |
| FilePreviewRenderer.vue | 新增 `autoBlame` prop，内容加载完成后自动开启 blame（遵守 blameAvailable 条件，消费后复位） |
| ContentPanel/Home | 消费联动请求：切 workspaceStore.selectedNode 为文件节点 → previewFile → autoBlame 传入 |
| 公共 | 抽右键菜单公共实现（自绘 `.context-menu` + `data-action` 语义锚，对齐 FileTreePanel 先例）；joinPath 工具（基于/对齐 utils/pathMatch.js normalizePath） |
| 后端 | `CheckFileExists`（app_filetree.go 或 app_git.go）；RPCHiddenMethods 同步 |

### 联动请求通道

FileDiffDialog 有 4 处挂载（LocalChanges / CommitHistory×2 / FileHistoryDialog 内嵌），emit 链穿透成本高 → 倾向独立单例 store/composable（模式对齐 uiStore 信箱先例：seq 单调递增、消费即清），CommitHistory 与 FileDiffDialog 写请求、ContentPanel/Home 层 watch 消费。实现时定稿。

### 风险点（实现阶段核实）

1. workspaceStore.selectedNode 切换链：原 watch(selectedNode) 自动预览已移除、由 Home.onNodeSelect 主动调 previewFile（ContentPanel.vue:1158 注释）——直改 store 可能绕过 Home 侧逻辑（latestCommit 更新等），需核实 Home.onNodeSelect 全貌后选择接入点
2. FilePreviewRenderer autoBlame 触发时机：预览内容加载完成前 blame 调用会竞态（blameSeq 复位链），须挂内容就绪之后
3. FileHistoryDialog 内嵌 FileDiffDialog 的「行归属」：嵌套三层（历史弹窗→diff→联动），若消费链复杂可 MVP 豁免此嵌套实例（记录即可）

## Decision (ADR-lite)

### D1: blame 展示形态 = 联动文件预览（2026-11-07 已确认）

**Context**：blame 唯一展示形态在 FilePreviewRenderer（CodeMirror 行尾标注），ContentPanel 仓库/文件形态互斥。
**Decision**：点击「行归属」→ 切换选中节点到该文件（Home.onNodeSelect 链路）→ previewFile 打开预览 → FilePreviewRenderer 新增 autoBlame prop 自动开启 blame。
**Consequences**：复用全部 blame 既有能力（行数守卫/零 SHA 禁跳转/标注点击→FileHistoryDialog 定位），零重复实现；提交历史 tab 被切走（追溯心智连续，可接受）；改动面跨 CommitHistory → ContentPanel/Home → FilePreviewRenderer 三层。

### D2: 入口交互形态 = 右键菜单为主（2026-11-07 已确认）

**Context**：CommitHistory file tag 点击语义已占用（打开该文件 diff）；tag 空间紧张；FileTreePanel 已有文件节点右键先例。
**Decision**：三处入口统一右键菜单，点击语义不变（tag 点击仍开 diff）；tooltip 弥补可发现性。
**Consequences**：与既有右键心智一致；点击链路零回归风险；三处共用公共菜单实现。

### D3: 存在性判定 = 新增 CheckFileExists + 点击时判定（2026-11-07 已确认）

**Context**：无轻量文件存在性绑定；blame 工作树语义要求文件存在。
**Decision**：新增 `CheckFileExists(path) bool`（os.Stat），点击「行归属」时判定（非展开详情时批量），不存在则降级 toast + 转 FileHistoryDialog。
**Consequences**：一个 bool 绑定换完整降级链路；判定零额外批量 IO；降级文案「文件已不在工作树，已转入文件历史追溯」。

## Out of Scope

- 后端缓存/解析改动（复用既有 GetFileCommitHistory / GetFileBlame）
- blame 渲染形态重构、独立 blame 弹窗
- CommitHistory `--name-status` 状态标记（A/M/D/R）改造——当前 --name-only 口径已满足本任务
- 目录级追溯（--follow 仅支持单一路径）

## Technical Notes

- 关键 spec：`docs/spec/file-level-history.md`（路径口径 §1）、`docs/spec/git-blame.md`（initialSha §5、blame 工作树语义 §2、入口条件 §4）、`docs/spec/browser-channel.md`（RPCHiddenMethods）、`docs/spec/terminal-chat-link.md`（uiStore 信箱先例）
- 先例代码：FileTreePanel.vue:500（FileHistoryDialog 挂载 + 自绘右键菜单）、FilePreviewRenderer.vue:1221-1241（initialSha 跳转 + 自挂 FileHistoryDialog）、ContentPanel.vue:978（previewFile 签名）、ContentPanel.vue:1158（自动预览移除注释）、utils/pathMatch.js（normalizePath）
- `commit.files` 后端契约：commit_history_cli.go:64-81（--name-only 口径）、parseCommitLogFiles:253
- 项目约定：列表项禁 mousedown+click 双绑（右键为独立事件不冲突）；单测/E2E 语义锚 data-action；wailsjs 手改三处规则（本任务 bool 返回仅两处）
