# Research: 前端无界内存驻留候选面盘点

- **Query**: 盘点 frontend/src/ 无界内存驻留候选面（Vue3 + Pinia + Element Plus，WebView2 常驻进程，驻留 = 应用生命周期内不还）
- **Scope**: internal（仅读代码，不改代码）
- **Date**: 2026-09-27

## 背景约束

`views/Home.vue:7-87` 确认：主三栏 / AiFunctionPanel / StatsView / DashboardView / AiChatPanel 全部 `v-show` 常驻挂载，应用生命周期内不卸载。因此组件内 `ref` 数据一旦加载即驻留，除非存在显式重置路径。Git 元数据面板（Branches/Remotes/Tags/Submodules/Info）在 ContentPanel lazy tab 内，未访问不挂载。

---

## 1. 组件级大数组状态

| 面 | 文件:行 | 数据结构 | 增长操作 | 清理现状 | 风险 |
|---|---|---|---|---|---|
| 提交历史数组 | `components/CommitHistory.vue:252`（`commits` ref([])，PAGE_SIZE=20 于 250 行） | commit 对象数组，每条含 `files[]`、sha、author、message | 「加载更多」`loadMore` → `loadCommits(false)` → `push` 追加，**无页数上限** | 切仓库 watch repoPath（529 行）→ `loadCommits(true)` 整组替换（369 行 `commits.value = []` + 重灌）；push/pull 后同样重置（305-311）；卸载随组件销毁。会话内单仓库持续 loadMore 只增不减 | **高**（千条级 × 每条 files 数组；操作频率中） |
| AI 审查结果 | `CommitHistory.vue:316-317`；`LocalChanges.vue:267-269` | `reviewIssues[]` + `reviewSummary` 字符串 | 每次 RunAiFunction('code-review') done 覆盖 | `resetAiState` 清空（CommitHistory:420-431 / LocalChanges:408-425，切仓库 + 卸载双入口） | 低 |
| 本地变更列表 | `components/LocalChanges.vue:238`（`changes` ref([])） | 变更文件对象数组 | `loadChanges` 全量替换 | 切仓库 watch（643-649 行）清空重载 | 低 |
| 推送长输出 | `LocalChanges.vue:254`（`pushResultOutput`）；`BranchSyncBar.vue:108` | 长字符串（>200 字符才入） | 每次推送覆盖 | 弹窗关闭不清空，驻留至下次推送覆盖；量级小 | 低 |
| AI 功能任务 Tab | `components/AiFunctionPanel.vue:244`（`tasks` ref([])） | task 对象：`output`（后端 256KB 截断后末尾窗口）、`tableExtracted`（后端预解析表格全量）、`metrics`、`followUps`、`prompt` | 每次运行：主段同 functionId 空闲任务 splice 原位替换（374-385）；**followUp 段始终新开 Tab push 追加，数组无上限** | 仅用户手动 `closeTask`（689-703，连带 `RemoveAiTask` 清后端 runtime）。组件 v-show 常驻不卸载 → 不关 Tab 即终生驻留 | **高**（单 task 可含数百 KB output + 表格数据；重度 AI 用户累积） |
| 已打开功能 Tab | `AiFunctionPanel.vue:246`（`openFuncs` ref([])） | AiFunction 快照数组 | 点卡片 push | 手动关 Tab filter 移除（682） | 低 |
| 会话消息 | `store/aiChat.js:120`（`chatMessages` ref([])） | 消息数组，每条含 markdown 长文本 content | 发送乐观追加（261-264）、选中会话整组替换（182） | 切会话 `selectChatSession` 整组替换旧引用可 GC（180-182）；切目录/空态 `resetChatSessionSelection` 清空（161-165）。**驻留面 = 当前会话全量消息，store 应用生命周期常驻** | **中**（单会话长对话可数 MB；切走即释放，但当前会话不还） |
| 选择题已答标记 | `store/aiChat.js:405`（`answeredChatQuestions` ref({})） | `key = sessionId:taskId` → true | 每次提交选择题答案写入 | **无清理路径**（注释明示仅内存态、重启才清）；值为 true 字节级小，条数随会话数单调增 | 低 |
| 会话/目录/模板列表 | `store/aiChat.js:43,114,363` | 列表（不含消息） | 重载整组替换 | 替换式，无累积 | 低 |
| 在途任务流式缓冲 | `store/aiChat.js:230,340`（`chatTask.reply +=`） | 流式回复累积字符串 | output 事件追加 | done 后 `chatTask = null`（353） | 低 |
| 任务历史面板 | `components/AiTaskHistoryPanel.vue:171,175`（`list`/`stats`） | 历史记录数组 + 聚合对象 | 打开面板/切筛选整组替换（204-209） | destroy-on-close 弹窗，每次打开重载替换 | 低 |
| diff 弹窗双栏数据 | `components/FileDiffDialog.vue:136-139`（`left`/`right`/`fileGroups` ref([])） | **每行一个 `{kind,no,text}` 对象**，双栏行数 ≈ diff 行数 × 2；range 模式 `fileGroups` 每文件再各存 left/right | 打开弹窗 `loadDiff` → `parseDiff`/`parseRangeDiff` 解析整段 diff 文本 | 关闭弹窗**不清空**（watch 仅 `visible=true` 时加载，365-370；el-dialog `destroy-on-close` 只销毁 slot DOM，script 状态驻留）；下次打开 loadDiff 开头先清空重灌（316-318）。驻留面 = 最近一次 diff | **中**（range 大 diff 数 MB → 行对象数十万级；操作频率中） |
| 文件预览内容 | `components/ContentPanel.vue:536-560`（`filePreview` ref，含 `content`/`base64`） | 文本 content 或图片/Office base64（数 MB 级） | 每次预览 `previewFile` 整对象替换（839-854） | 切文件即替换旧引用；`clearPreview` 显式清空（1048-1065，Home.onNodeSelect 驱动）。驻留面 = 当前单文件 | 中低（base64 约为原文件 1.33 倍，单份驻留） |
| Excel 解析产物 | `components/FilePreviewRenderer.vue:734`（`xlsxSheets` ref([])） | 每 sheet：`rows` 二维数组 + `dataRows` 对象数组（单元格值全量） | `renderXlsx` 解析 base64 后整组赋值（781） | watch 切换非 office 类型清空（823-827）；`renderOfficeBySubType` 重渲染前清空（803） | 中低（大表 × 对象膨胀；随预览切换释放） |
| 批量拉取结果 | `ContentPanel.vue:583`（`pullResults`） | 每仓库结果对象数组 | pull-progress 事件 push（1077） | `startBatchPull` 重置（1030） | 低 |
| Git 元数据 | `GitBranches.vue:110`、`GitRemotes.vue:125`、`GitTags.vue:104`、`GitSubmodules.vue:142` | branches/remotes/tags/submodules 列表 | 加载整组替换；watch repoPath 重载（GitRemotes:258 等） | 替换式，无累积；量级数百内 | 低 |
| 仓库统计 | `views/StatsView.vue:62,79`（`stats`/`repoInfo`） | 单个聚合对象（trend/heatmap/contributors/dirLineStats 数组） | loadStats 替换（requestSeq 防竞态 66-69） | 替换式；RepoStatsChart 纯 props + computed，vue-echarts 单实例常驻（v-show），option 替换不累积 | 中低（echarts 单实例 + 中等量级 series） |
| 命令面板搜索结果 | `composables/useCommandPalette.js:8,12`（`fileResults`/`contentGroups`） | 结果数组 | 每次搜索替换 | 替换式 | 低 |
| 工作区共享态 | `store/workspace.js:16-27`（selectedNode/latestCommit/clipboard） | 小对象 | 选中/提交时覆盖 | `clearClipboard`（32-37） | 低 |
| 目录/收藏/设置 store | `store/directory.js:16`、`store/favorites.js:12`、`store/settings.js:226-272` | 列表 + 标量（`shellConfigsCache` 列表替换） | 重载替换 | 替换式，量级小 | 低 |

## 2. Pinia store 级无界数组与无清理路径汇总

| store | 无界面 | 清理路径 | 结论 |
|---|---|---|---|
| aiChat | `chatMessages`（单会话驻留）、`answeredChatQuestions`（无界单调增） | 切会话替换 / 无 | aiChat 是唯一有两条驻留面的 store |
| directory / favorites / workspace / ui / settings | 无无界数组（均列表替换或小对象） | — | 无风险 |

## 3. 事件监听器盘点（EventsOn 全量）

**规范实现（返回闭包精准注销，符合 docs/spec/cross-layer-contracts.md）**：

| 位置 | 事件 | 注销 |
|---|---|---|
| `composables/useTerminal.js:163,172` | terminal-output / terminal-exit | `destroyTerminal` 摘除（114 行注释明示本实例闭包） |
| `components/AiFunctionPanel.vue:752-755` | ai-task:queued/started/output/done | onBeforeUnmount 逐闭包调用（757-760） |
| `components/AiChatPanel.vue:948-951` | chat-task:queued/started/output/done | onBeforeUnmount（963-972，先注册后异步加载防 await 窗口泄漏） |
| `components/CommitHistory.vue:542` | ai-task:done | onUnmounted 调闭包（551-554） |
| `components/LocalChanges.vue:653` | ai-task:done | onBeforeUnmount 调闭包（660-663） |

**偏差（EventsOff 全局清同名，未用闭包）**——三处当前均为唯一监听者，功能无误伤，属规范偏差而非泄漏：

| 位置 | 事件 | 现状 |
|---|---|---|
| `components/AiTaskHistoryPanel.vue:370,373` | ai-task:archived | `EventsOn` 返回值未接收，onBeforeUnmount `EventsOff('ai-task:archived')` |
| `components/ContentPanel.vue:1076,1080` | pull-progress / pull-complete | setup 顶层注册（1092，非 onMounted），`cleanupPullEvents` 用 EventsOff（1067-1071），onBeforeUnmount 调用（1094） |
| `components/UpdateDialog.vue:104,113` | update:download-progress | EventsOn 未存闭包，onBeforeUnmount EventsOff |

## 4. 定时器 / Observer

| 位置 | 类型 | 清理现状 | 风险 |
|---|---|---|---|
| `components/TerminalPanel.vue:439,456-457` | ResizeObserver | onBeforeUnmount `disconnect()` | 无（机制健全） |
| `components/CommitHistory.vue:347,546` | filterTimer（300ms 防抖） | onUnmounted clearTimeout | 无 |
| `composables/useSessionState.js:169,206-216` | saveTimer（2s 防抖）+ beforeunload | `dispose()` clearTimeout + 移除监听 | 无 |
| `components/CommandPalette.vue:281,287` | searchTimer（300ms 防抖） | 新输入时 clearTimeout；**无 onUnmounted 清理**（防抖窗口内卸载则回调多触发一次，一次性） | 低 |
| `transport/events.js:160` | reconnectTimer（指数退避） | 用户取消令牌时 clearTimeout（144）；registry 空不调度 | 无（基础设施） |
| `transport/connBanner.js:80` | hideTimer | 58/68 行 clearTimeout | 无 |
| `components/FileTreePanel.vue:1038,1080,1163,1468,1483`、`DirectoryTree.vue:488`、`ContentPanel.vue:997` | setTimeout 一次性（DOM 等待/focus） | 一次性自清 | 无 |
| 全局 setInterval | 无生产代码命中 | — | 无 |
| `components/FilePreviewRenderer.vue:1052-1058` | document×3 + window message 监听 | onBeforeUnmount 全部 removeEventListener（1061-1066） | 无 |

终端为受控面：`useTerminalTabs.js` MAX_TERMINAL_TABS=8 软上限，每 tab 一个 xterm 实例（scrollback 默认 1000 行，`useTerminal.js:85,131`），`destroyTerminal` → `term.dispose()` + `CloseTerminal` + 闭包注销（253-274）；TerminalPanel 卸载 destroyAll + observer disconnect（455-462）。

## 5. 既有释放机制清单（勿重复建议）

| 机制 | 调用点 |
|---|---|
| `CancelAiTask` | `AiFunctionPanel.vue:672`（手动取消）；`CommitHistory.vue:423`、`LocalChanges.vue:411,415`（resetAiState：切仓库 + 卸载双入口，释放并发槽位与 claude 子进程）；`store/aiChat.js:283`（CancelChatTask） |
| `InvalidateCommitHistoryCache`（前端） | `CommitHistory.vue:309`（push/pull 后）、`394`（手动刷新） |
| EventsOn 闭包注销范式 | 上表 5 处（useTerminal / AiFunctionPanel / AiChatPanel / CommitHistory / LocalChanges） |
| `RemoveAiTask` | `AiFunctionPanel.vue:697`（closeTask 时清后端 runtime） |
| FilePreviewRenderer 资源释放 | `cmView.destroy()`（649/819/1069）、xlsxSheets 清空（738/803/825）、docx innerHTML 清空 |
| resetAiState 双入口模式 | CommitHistory:420 / LocalChanges:408（切仓库 watch + 卸载钩子均调用） |

---

## Top 候选测量面排序（建议先进测量脚本）

1. **AiFunctionPanel `tasks` 数组**（`AiFunctionPanel.vue:244`）— 无上限累积 + v-show 常驻 + 单 task 含 256KB 截断 output 与 tableExtracted 表格数据，仅手动关 Tab 释放。重度 AI 使用场景主嫌疑。
2. **CommitHistory `commits` 数组**（`CommitHistory.vue:252`）— loadMore 无页数上限，千条提交 × files 数组；会话内单仓库只增不减（切仓库有重置）。
3. **store/aiChat `chatMessages` + `answeredChatQuestions`**（`store/aiChat.js:120,405`）— 当前会话 markdown 长文本终生驻留 + 已答标记无界单调增（后者量小）。
4. **FileDiffDialog `left/right/fileGroups`**（`FileDiffDialog.vue:136-139`）— 关闭弹窗后最近一次 range 大 diff 以双栏行对象形态驻留（膨胀 2-4 倍），下次打开才释放。
5. **ContentPanel `filePreview.base64` + FilePreviewRenderer `xlsxSheets`**（`ContentPanel.vue:536`、`FilePreviewRenderer.vue:734`）— Office/图片单份 base64（原文件 1.33 倍）+ Excel 全量解析对象，随切换释放但当前文件不还。

## Caveats

- 行号基于当前工作区快照（2026-09-27，master 00103d1），后续改动会漂移。
- `tasks[].output` 具体截断窗口大小（256KB 为截断标记阈值）未逐一核实后端实现，测量时以后端 `service/ai_task.go` 实际写入为准。
- vue-echarts 内部 canvas 实例与 el-table 大数据量的渲染层驻留未深入（属第三方组件内部，前端不可直接控制），测量脚本如需覆盖建议单独观测 StatsView 打开前后的 heap 差值。
