# Research: E2E 覆盖缺口矩阵（五域盘点）

- **Query**: 盘点 E2E 覆盖缺口矩阵：五域 × 现有 E2E 覆盖 × 组件消费链 × mock 可行性 × 风险档，产出 Top 20 候选用例
- **Scope**: internal（frontend/e2e、frontend/src、wails-mock-defaults.js、git log 活跃度）
- **Date**: 2026-10-21

---

## 1. 盘点基线

### 1.1 现有 E2E 分布（17 spec / 72 用例，已核实）

| 域 | spec | 用例数 | 覆盖主题 |
|---|---|---|---|
| 文件树 | file-tree.spec.js | 16 | 渲染/新建/重命名/删除/失败链/目录筛选/2000 截断哨兵/覆盖层右键 |
| Git | submodule 6 / repo-config 6 / branch 6 / commit-push 5 / merge 4 / diff-tool 4 / session-restore 2 / smoke 1 | 34 | 子模块、配置导入导出、分支、提交推送、合并冲突、diff 工具、崩溃恢复 |
| 素材 | backup-rules 4 / relink 4 / folder-drag 4 / export 2 / batch-tag 1 / split 1 | 16 | 备份还原+排除规则、丢失重链+前缀替换、文件夹拖拽、元数据导出、批量打标签、PPT 拆分 |
| AI | ai-function 4 / ai-chat 2 | 6 | 功能运行成功/失败/取消流、对话闭环、会话切换 |

另有 `e2e-desktop/`（真桌面通道）：crash-flag、session-persist 两 spec，独立于本盘点。

### 1.2 mock 基础设施现状

- 单一数据源：`frontend/src/test/wails-mock-defaults.js`（基础表 + E2E extra 表，合并注入 `window.go.main.App`），共 **176 个方法有缺省值**。
- 描述符能力（`e2e/wails-init.js`）：`{__error__, __code__}`（结构化 reject）、`{__sequence__}`（按序返回）、`{__value__, __events__, __preEvents__}`（resolve + 事件派发，支持 delayMs）——素材 `assets-*:progress/done/error`、AI `ai-task:*`/`chat-task:*`、终端 `terminal-output` 等事件链均可 mock。
- 调用断言：`getWailsCalls(page, method)` 读取 `window.__wailsCalls`，参数级断言现成。
- **66 个绑定方法无 mock 缺省**（共 246 个绑定方法）。与五域缺口直接相关的缺省项：
  - 终端：`CreateTerminal` / `WriteTerminalInput` / `ResizeTerminal` / `CloseTerminal` / `ChangeTerminalDir`
  - 看板：`GetRepoFilterList` / `RefreshRepoFilterList` / `SaveRepoMeta` / `CleanMissingRepoMeta`
  - AI 历史：`GetAiTaskHistory` / `GetAiTaskHistoryStats` / `ExportAiTaskHistoryCSV` / `ClearAiTaskHistory` / `GetAiTaskOutput` / `RunAiFollowUp` / `GetDiscoveredSkills` / `RefreshDiscoveredSkills`
  - Git 次级：`GetRemotes` / `AddRemote` / `RemoveRemote` / `FetchRepo` / `GetTags` / `CreateTag` / `DeleteTag` / `PushTag` / `CherryPick` 全家 / `GetFileDiff` / `GetRangeDiff` / `DiscardChanges` / `CloneRepo` / `InitSubmodules` / `GetRepoReadme` / `SearchFiles` / `GetFileTreeRecursive`

### 1.3 近 5 周组件改动频次（风险档依据，git log 统计）

| 文件 | 次数 | 文件 | 次数 |
|---|---|---|---|
| AssetsPanel.vue | 18 | CommitHistory.vue | 5 |
| SettingsPanel.vue | 12 | AiChatPanel.vue | 5 |
| FileTreePanel.vue | 10 | settings.js (store) | 5 |
| Home.vue | 7 | DirectoryTree.vue | 4 |
| ToolboxPanel.vue | 7 | CommandPalette.vue | 4 |
| TerminalPanel.vue | 7 | AssetSelectionBar.vue | 4 |
| ActivityBar.vue | 6 | DashboardView.vue | 3 |

---

## 2. 五域缺口矩阵

图例：覆盖 = 有 E2E 用例数；风险 = 高/中/低（近期改动 × 跨组件事件链 × mock 盲区）；mock 复杂度 = 极低/低/中/高。

### 2.1 工具箱域（7 面板，2026-10-19 Panel 化重构）— **E2E 零覆盖**

| 面板 | 核心用户链路 | Wails 方法 | mock 缺省 | 真实依赖可 mock 性 | mock 复杂度 | 风险 |
|---|---|---|---|---|---|---|
| ToolboxPanel 外壳 | 左 nav 选工具 → 右侧 `component :is` 卸载重建切换；激活时三栏隐藏（独占一级面板） | 无（纯编排） | — | 无后端依赖 | 极低 | **高**（7 次改动，重构核心不变式无守护） |
| ProcessManagerPanel | 关键字/模式查进程 → 列表 → 优雅停止 5s 轮询 → 强杀确认 | ListProcesses / StopProcess / KillProcess | 有（空列表 / forced） | 真实进程枚举不触——mock 列表形状已定型（ProcessInfo） | 低 | **高**（停止链路三段语义刚收敛） |
| PortListPanel | 端口占用一览（`ListProcesses('','')` 扁平化）→ 行级防重终止 | ListProcesses / StopProcess | 有 | 同上 | 极低 | 中 |
| EnvVarPanel | 三来源枚举 → 脱敏标记 → 点击临时显值 | ListEnvVars | 有（完整三组 fixture，含 sensitive 项） | 无 | 极低 | 低 |
| HostsPanel | 只读解析 → 行类别过滤 → 复制 | GetHosts | 有（entry/comment/unknown/blank 四类 fixture） | 真实 hosts 文件不触 | 极低 | 低 |
| JsonYamlToolPanel | JSON/YAML 转换校验 | 无（纯前端） | — | 无 | 极低 | 低 |
| PptSplitPanel | 单文件拆分 → 批量拆分 → 取消 → 进度/完成事件 | SplitPptxPerPage / SplitPptxFolder / CancelPptxSplit + `pptx-split:*` 事件 | 有（空结果） | 真实 OOXML 在后端，不触；事件经 `__events__` 描述符派发 | 低 | **高**（三批近期改动：文件夹批量/工具箱面板化/素材集成） |
| CopyToPanel | 选目标目录 → CopyTo | CopyTo | 有 | 真实文件系统不触 | 低 | 低 |

mock 盲区说明：PPT 拆分真实链路的 OOXML 产物校验不可在 E2E 覆盖（后端域，Go 测试已覆盖）；E2E 价值在**面板编排与事件消费**（进度条、取消按钮态、错误 toast、批量汇总 Items 渲染）。

### 2.2 终端域（多 tab）— **E2E 零覆盖**

| 链路 | 组件/composable | Wails 方法 | mock 缺省 | mock 复杂度 | 风险 |
|---|---|---|---|---|---|
| tab 创建 → 输出事件流 → 输入回写 | TerminalPanel + useTerminal + useTerminalTabs | CreateTerminal / WriteTerminalInput / ResizeTerminal / CloseTerminal / ChangeTerminalDir + `terminal-output` / `terminal-exit` 事件 | **全无，需补缺省** | **中**：5 方法补 `null`/id 返回 + `__events__` 派发 `terminal-output`；xterm.js 真实渲染在 headless Chromium 可用但 fit 依赖布局尺寸，断言宜聚焦 DOM/状态而非像素 | **高**（7 次改动；terminal-multi-session.md 坑密度全项目最高：v-show 保 DOM、tab watch getter 投影、事件闭包注销、z-index 1500 全屏层） |
| tab 切换/关闭生命周期 | useTerminalTabs | 同上 | 同上 | 中 | **高**（「关闭非活动 tab」是唯一能暴露 watch 投影缺陷的路径） |
| 全屏层开合 + ESC | TerminalPanel | 无 | — | 低（纯前端，但 z-index/ESC 过滤逻辑脆） | 中 |
| 会话快照恢复（terminal 字段） | useSessionState + session-restore.spec | GetSessionState / SaveSessionState | 有（null = 冷启动） | 已有 2 用例覆盖，终端字段指针形态未覆盖 | 中 |

mock 盲区说明：真实 pty 在后端，E2E 只能验前端编排；`terminal-output` 事件载荷 `(sessionId, output)` 双参形态需对齐 useTerminal 消费签名。

### 2.3 AI 对话域 — 6 用例（薄面）

| 链路 | 组件 | Wails 方法 | mock 缺省 | mock 复杂度 | 风险 |
|---|---|---|---|---|---|
| 会话目录 CRUD + 选中过滤 | AiChatPanel + aiChat store | AddChatDirectory / ListChatDirectories / UpdateChatDirectory / RemoveChatDirectory / ReorderChatDirectories | 有（空/true） | 低 | 中（store 3 次改动） |
| 模板管理（增删改查） | AiChatPanel | AddChatTemplate / ListChatTemplates / UpdateChatTemplate / RemoveChatTemplate | 有 | 低 | 中 |
| 权限模式/模型设置 | AiChatPanel | GetChatSettings / SaveChatSettings | 有 | 极低 | 低 |
| 取消在途对话 | AiChatPanel | CancelChatTask + chat-task 事件 | 有 | 低（复用 RunChat `__preEvents__` 形态造在途态） | 中 |
| 选择题 chat-question 块渲染分流 | AiChatPanel | RunChat 事件流 | 有（RunChat 描述符） | 低（done 前插入 question 块 output 事件） | 中（headless AskUserQuestion 契约近期改动） |
| claudeSessionId 回写 + --resume 续聊 | AiChatPanel | GetChatSession（含消息数组） | 有（null 需 override 完整对象） | 低 | 中 |

已覆盖：对话闭环（RunChat 事件流气泡渲染）、会话切换。

### 2.4 导航中心 / 看板 / 设置域 — **E2E 零覆盖**（smoke 仅 1 条）

| 链路 | 组件 | Wails 方法 | mock 缺省 | mock 复杂度 | 风险 |
|---|---|---|---|---|---|
| 命令面板：内容搜索 → 结果组渲染 → 点击跳转 | CommandPalette + useCommandPalette | ContentSearch | 有（空数组；`timedOut: true` 标志组形态注释明示按需 override） | 低 | **高**（4 次改动；2026-10-08 批量超时重试编排刚落地：searchVersion 版本守卫/信号量 3/banner 进度态/retryingRepoPaths 预占，全前端编排零 E2E） |
| 命令面板：超时目录批量重试 | useCommandPalette | ContentSearchDirs | 有 | 低（前 N 次 pending resolver + 后续 resolve 形态有 spec 先例记录） | **高**（同上） |
| 全局看板：状态渲染 + 手动刷新 + 钉选互锁 | DashboardView | GetDashboardStatuses / RefreshDashboardStatuses / GetDashboardPinned / AddDashboardPin / RemoveDashboardPin / IsDashboardPinned / GetRepoFilterList | 有（extra 表空数组） | 低 | 中（3 次改动） |
| 设置：主题/字体/终端外观修改 → 保存 | SettingsPanel + settings store | GetSettings / SaveSettings | 有 | 低 | **高**（12 次改动全项目第二；整体提交语义「GetSettings 失败基底 null 禁保存」有 spec 警示；外观热更联动终端 refit 跨组件） |
| 设置：网络访问分区 | SettingsPanel | GetWebServeConfig / SetWebServeConfig / GetWebServeToken / RegenerateWebToken | 有 | 极低 | 低 |
| 最近访问 / 收藏分组 | DirectoryTree + useRecentAccess | GetFavorites / AddFavorite / UpdateFavoriteGroup / ReorderDirectories | 有 | 低 | 中 |
| 统计页图表渲染 | StatsView | GetRepoStats | 有（extra 表完整 fixture 含行数字段） | 极低（ECharts 真实浏览器可用，有先例注释） | 低 |

### 2.5 素材域 — 16 用例，但薄面大

已覆盖：备份/还原/排除规则（4）、丢失重链/前缀替换/清理（4）、文件夹拖拽单卡/多选/目录级（4）、元数据导出（2）、批量打标签（1）、PPT 拆分（1）。

| 薄面链路 | 组件 | Wails 方法 | mock 缺省 | mock 复杂度 | 风险 |
|---|---|---|---|---|---|
| 购物栏：页收集 → 抽屉 → 拖拽排序 → 合并导出进度链 | AssetsPanel + AssetCartDrawer + AssetSelectionBar（溯源入口） | ListAssetCartItems / AddAssetCartItems / ReorderAssetCart / MergeAssetCartToPptx / CancelAssetCartExport + `assets-cart:export-progress/done/error` 三事件 | 有（全部） | **低-中**：方法缺省齐全、事件描述符现成；拖拽排序 vuedraggable 需 dispatchEvent 构造（folder-drag.spec 有 HTML5 DnD 先例）；导出进度需 `__events__` + delayMs | **高**（AssetsPanel 18 次改动全项目最高；三事件跨组件广播共听面；条目键三处同构易漂移） |
| 批量评分弹窗主链 | AssetBatchRatingDialog（宿主 execute 分派） | BatchSetAssetRating | 有（全零明细） | 极低（与已有 batch-tag.spec 同构，可复制形态） | 中 |
| facet 多维筛选参数换算 | AssetFilterPopover + AssetsPanel | SearchAssets（参数断言） | 有 | 极低（`getWailsCalls` 参数级断言现成；换算单点在模块导出函数） | 中（mtime 纳秒区间/止日末毫秒锚点易回归） |
| 检索排序档位切换 | AssetsPanel | SearchAssets（sortBy/sortDir 透传断言） | 有 | 极低 | 中 |
| 保存检索条件 upsert + 异库调取重映射 | AssetsPanel | ListAssetSavedSearches / SaveAssetSavedSearch / DeleteAssetSavedSearch | 有（空列表） | 低 | 中 |
| 幻灯片标注保存双代次守卫 | AssetsPanel（onSaveSlideMeta） | UpdateAssetSlideMeta + ListAssetSlides | 有 | **中**：需 `__sequence__` 控制 ListAssetSlides 返回序模拟旧代次响应（P1 踩坑：miss 回填/resetSlideDraft 时序） | 中 |
| 缩略图画廊懒加载 + 失败重试恰一轮 | AssetsPanel + AssetThumb | GetAssetThumb / AssetThumbsAvailable / RetryAssetThumb + `assets-thumbs:progress/done` | 有 | 中（base64 小串可 mock；失败→done 自动重试 let 标记链需事件序列） | 中 |
| 标签管理树：拖拽移动 + 失败回滚 | AssetTagManagerDialog + AssetTagTreeSelect | ListAssetTags / MoveAssetTag / DeleteAssetTag / CreateAssetTag | 有（MoveAssetTag null，reject 需 override） | 中（el-tree node-drop 落点换算 + 乐观更新回滚重拉） | 中（D8 拖拽近期落地） |
| 标签树选择器三处复用语境 | AssetTagTreeSelect | ListAssetTags | 有 | 低 | 中（fetchSeq 代次守卫） |
| 预览层：翻页/hd 切换/越界关层 | AssetPreviewOverlay | GetAssetThumb | 有 | 中（z-index 局部语境 jsdom 测不出，E2E 是唯一自动验证面） | 中 |

---

## 3. mock 可行性评估要点（候选链路共性）

1. **真实进程枚举（进程管理/端口占用）**：完全可 mock——ListProcesses 返回 ProcessInfo 数组，形状已在 mock 表定型（注释含字段对齐说明）；StopProcess 三段语义（优雅→轮询→强杀）是前端展示层消费 `{stopped, method}`，注入 `{stopped: false}` + `__sequence__` 可驱动「转强杀」UI 分支。
2. **真实 OOXML/COM（PPT 拆分、PPT 预览、缩略图、跳转）**：后端域不触 E2E；前端可测面 = 事件进度消费（`__events__` 派发）、降级态（ConvertPptxForPreview 默认缺省即降级形态）、取消按钮态（Cancel 返回 false 幂等）。PPT 预览 viewer 全链（pdfjs 拉 /preview-pdf 端点）**不可 E2E**（dev 代理 + preview 服务均无该路由），仅测降级提示 + 「用默认程序打开」按钮。
3. **真实 pty（终端）**：不可 mock 后端，可 mock 编排——5 方法补缺省 + terminal-output 事件双参载荷；断言聚焦 tab DOM 状态与调用序列，不聚焦渲染像素。
4. **拖拽交互（购物栏排序/标签树/文件夹树）**：Playwright 原生 HTML5 DnD 不可靠，dispatchEvent 构造 dragstart/drop 是既有先例（folder-drag.spec.js）；vuedraggable（购物栏）与 el-tree（标签树）落点换算不同，后者需 `node-drop` 事件参数构造，复杂度中。
5. **ElMessageBox 递归/两段链**：强删除备份失败升级确认已有 spec 先例（backup-rules 第 4 用例），`.first()/.last()` DOM 序范式照抄即可。
6. **vitest 被 mock 屏蔽的面**（E2E 独有价值）：EP 运行时弹窗层叠、z-index、ECharts/xterm 真渲染、事件广播多组件共听、vuedraggable 真排序。

---

## 4. Top 20 候选用例（风险 × 价值排序）

| # | 候选用例（一句话链路） | mock 复杂度 | 涉及组件 / 方法 | 风险依据 |
|---|---|---|---|---|
| 1 | 命令面板搜索超时标志组渲染 + 单目录重试移除标志：输入 `::关键词` → ContentSearch 返回 `{timedOut:true}` 组 → warning 行 → 点击重试 ContentSearchDirs 成功后标志组消失 | 低 | CommandPalette / useCommandPalette / ContentSearch、ContentSearchDirs | 2026-10-08 新逻辑零 E2E；mock 缺省注释明示该形态 |
| 2 | 命令面板批量重试编排：≥2 超时组 → banner「重试全部」→ 前 3 个 pending resolver 捕获排队中间态 → 逐个 resolve → 进度 done/total 更新 → 版本守卫失效路径丢弃写回 | 低 | useCommandPalette / ContentSearchDirs | 信号量 3 + searchVersion 守卫纯前端编排，回归即静默丢结果 |
| 3 | 终端 tab 生命周期：新建 tab → 切换（v-show 保 DOM）→ 关闭非活动 tab → EventsOn 闭包注销（terminal-output 不再写入已销毁实例） | 中 | TerminalPanel / useTerminalTabs / CreateTerminal 等 5 方法补缺省 + terminal-output 事件 | 7 次改动 + watch getter 投影唯一暴露路径 |
| 4 | 工具箱进程管理主链：输入关键字搜索 → 列表渲染 → 优雅停止（StopProcess `{stopped:true,method:'graceful'}`）→ 强杀确认弹窗（KillProcess）→ 失败 toast 分流 | 低 | ProcessManagerPanel / ListProcesses、StopProcess、KillProcess | Panel 化重构后零守护；停止三段语义刚收敛 |
| 5 | 素材购物栏收集→合并导出事件链：卡片加入（徽标计数）→ 抽屉打开 → MergeAssetCartToPptx resolve + `assets-cart:export-progress/done` 事件 → 进度条→完成 toast → 失败事件 → error 分流 | 低-中 | AssetsPanel / AssetCartDrawer / MergeAssetCartToPptx + 三事件 `__events__` | AssetsPanel 18 次改动最高频；跨组件事件共听面 |
| 6 | 设置面板保存整体提交链：改外观项 → SaveSettings 收到完整 settings 对象（含未改段）→ GetSettings reject 时保存按钮禁用（「失败基底 null 禁保存」不变式） | 低 | SettingsPanel / settings store / GetSettings、SaveSettings | 12 次改动第二高频；整体提交清段风险有 spec 警示 |
| 7 | 工具箱外壳切换不变式：激活工具箱 → 三栏隐藏（`.main-panes` v-show 仅 directory）→ 切换 7 工具 `component :is` 卸载重建（前工具 onBeforeUnmount 触发）→ 退回目录三栏恢复 | 极低 | ToolboxPanel / Home.vue | 2026-10-19 重构核心；纯编排零 mock |
| 8 | PPT 拆分面板批量链：选目录 → SplitPptxFolder resolve + `pptx-split:progress` 事件 → 逐文件进度 → done 汇总 Items（含失败明细）渲染 → CancelPptxSplit 按钮态切换 | 低 | PptSplitPanel / SplitPptxFolder、CancelPptxSplit + 事件 | 三批近期改动；事件消费面零守护 |
| 9 | 全局看板渲染与钉选：GetDashboardStatuses 列表渲染 → 手动刷新 Refresh → 钉选 Add/Remove 互锁（IsDashboardPinned）→ 过滤器选目录 GetRepoFilterList | 低 | DashboardView / 7 方法 | 零覆盖；mock 缺省现成，低成本快赢 |
| 10 | AI 对话会话目录 CRUD 与选中过滤：新增目录 → 列表选中 → ListChatSessions 按 directoryId 过滤 → 重命名/删除确认弹窗 | 低 | AiChatPanel / aiChat store / 5 目录方法 | 方法缺省齐全；store 3 次改动 |
| 11 | 幻灯片标注保存双代次守卫：保存 → await 刷新期间检索态变化（`__sequence__` 注入旧代次 ListAssetSlides）→ 断言不回填旧值、表单不回退 | 中 | AssetsPanel onSaveSlideMeta / UpdateAssetSlideMeta、ListAssetSlides | P1 踩坑记录（二次保存写回旧数据），唯一能自动守卫的面 |
| 12 | facet 筛选参数换算：打开 popover → 勾评分档 + 相对时间档 + 大小区间 → SearchAssets 收到正确 ratings/mtimeFrom/mtimeTo/sizeFrom/sizeTo（getWailsCalls 断言） | 极低 | AssetFilterPopover / SearchAssets | 纳秒区间/止日末毫秒锚点换算纯函数易回归 |
| 13 | 批量评分弹窗主链：选中 3 条目 → 弹窗设星 → BatchSetAssetRating 明细（成功+失败混合）→ 结果列染失败行 → completed 不清选中 | 极低 | AssetBatchRatingDialog / BatchSetAssetRating | 与 batch-tag.spec 同构可复制；v8 新功能零 E2E |
| 14 | 检索排序档位切换：工具行下拉改 mtime 降序 → applySearch → SearchAssets 收到 `{sortBy:'mtime',sortDir:'desc'}` 透传 | 极低 | AssetsPanel / SearchAssets | v8 新功能；参数断言一步到位 |
| 15 | 端口占用一览与终止：空关键字默认视图扁平化为一行一端口 → 行级防重（同 PID 多端口共享在途态）→ 终止确认 → StopProcess | 极低 | PortListPanel / ListProcesses、StopProcess | 与进程管理同构；扁平化逻辑无守护 |
| 16 | hosts/环境变量只读渲染与脱敏：GetHosts 四行类别过滤开关 → ListEnvVars sensitive 项打码 → 点击临时显值 | 极低 | HostsPanel / EnvVarPanel | fixture 完整现成，最低成本快赢 |
| 17 | 缩略图画廊懒加载 + 失败重试恰一轮：画廊挂载 GetAssetThumb tier=0 逐卡 → 失败卡 → `assets-thumbs:done` 事件 → 自动 RetryAssetThumb 恰一次（let 标记）→ 切库复位 | 中 | AssetsPanel / AssetThumb / GetAssetThumb、RetryAssetThumb + 事件 | 重试「恰一轮」时序无自动守护 |
| 18 | 标签树拖拽移动 + 失败回滚：拖子标签到另一父 → node-drop 落点换算 → MoveAssetTag resolve 树重排 / reject 回滚重拉恢复原位 | 中 | AssetTagManagerDialog / AssetTagTreeSelect / MoveAssetTag（override reject） | D8 拖拽新落地；乐观更新回滚无守护 |
| 19 | 保存检索条件 upsert 与异库调取重映射：保存当前条件（重名拒绝）→ 切库 → 调取 → tagIds 失效按 tagNames 重映射 → miss 丢弃计数提示 | 低 | AssetsPanel / ListAssetSavedSearches、SaveAssetSavedSearch | v7 功能零 E2E；重映射丢弃逻辑纯前端 |
| 20 | 文件树 pptx 预览降级链：点击 pptx 节点 → ConvertPptxForPreview 默认降级态 → 渲染器降级提示 + 「用默认程序打开」按钮出现 → OpenWithDefaultApp 触发 | 极低 | FilePreviewRenderer / FileTreePanel / ConvertPptxForPreview（缺省即降级） | 2026-10-20 新功能；降级分支 mock 即缺省，近零成本 |

排序依据：#1-#8 均为「近期高频改动（≥5 次/或两周内新功能）× 零 E2E × mock 低复杂度」；#9-#16 为零覆盖低成本面（快赢批次）；#17-#20 为中复杂度或新功能冒烟。

### 4.1 本批落地标注（2026-10-21，PRD Top 5 链路实施后）

| 候选 # | 状态 | 落地载体 |
|---|---|---|
| 1（单目录重试） | **本批落地** | command-palette-retry.spec.js 用例 1（warning 行 + 参数断言 + 零命中移除） |
| 2（批量重试编排） | **本批落地** | command-palette-retry.spec.js 用例 2（信号量 3 在途上限 + banner done/total 三态）+ 用例 3（searchVersion 版本守卫丢弃 + worker 提前终止） |
| 3（终端 tab 生命周期） | **本批落地（部分）** | terminal-tabs.spec.js 3 用例（冷启动 / 新建切换 v-show 保 DOM / 关闭非活动 tab 会话销毁 + 再展开不重复建会话）；EventsOn 闭包注销的缓冲区写入断言未做（xterm 缓冲区内容不在断言面，以 CloseTerminal 调用为销毁代理） |
| 4（进程管理主链） | **本批落地（停止链）** | toolbox-panel.spec.js 用例 3（isSelf 硬禁 + 确认链 + 行级防重 + graceful toast + 刷新）；KillProcess 强杀链与失败 toast 分流未展开（同构，留后续） |
| 5（购物栏事件链） | **本批落地（主链）** | assets-cart.spec.js 3 用例（加页入栏 + 参数断言 / 溯源跳转 / 导出进度事件 + SkippedItems 报告 + 保留不清空）；实施中发现并修复真实缺陷：locateCartEntry 消费 AssetCartItem 不存在的 filePath 字段（vitest fixture 形状漂移掩盖），改由 srcPath 剥离库根还原 |
| 6（设置保存链） | **本批落地** | settings-save.spec.js 3 用例（整体提交 payload 含未改段 / GetSettings reject 禁保存不变式 / 主题 + 终端字号热更联动 ResizeTerminal） |
| 7（工具箱外壳不变式） | **本批落地** | toolbox-panel.spec.js 用例 1（三栏互斥）+ 用例 2（component :is 卸载重建枚举） |
| 8-20 | **留后续批次** | 按原排序依据与 mock 可行性按需启动（#8 PPT 拆分批量链 / #15-#16 快赢面 / #12-#14 素材参数断言面等） |

本批新增基础设施（后续候选用例可直接复用）：`{__deferred__}` 挂起解析器描述符 + `releaseDeferred`/`pendingDeferredCount`（fixtures.js）+ `emitWailsEvent`（EventsEmit 同步广播，wails-init.js）；终端 5 方法 mock 缺省已入 wails-mock-defaults.js 基础表。范式沉淀见 docs/spec/e2e-testing.md §6。

---

## 5. 契约对照（写用例时生效）

- 统一从 `e2e/fixtures.js` import `{ test, expect, getWailsCalls }`（+ 本批新增 `releaseDeferred` / `pendingDeferredCount` / `emitWailsEvent`）；禁 `waitForTimeout`。
- mock 缺省只进 `wails-mock-defaults.js` 两表（基础表/E2E extra 表）；用例级差异走 `test.use({ wailsOverrides: {...} })`。
- 新建 E2E 子目录（如分域拆 spec）不触发 vitest exclude 问题（`e2e/**` 已整体排除）；新建**通道**目录才需同步 `vitest.config.js`。
- ElMessageBox 递归/两段链断言用 `.first()/.last()`（DOM 追加序 = 弹出序）。
- 最佳参照：mock 驱动形态抄 `assets-export.spec.js`（事件 + 描述符）、`file-tree.spec.js`（右键菜单 + 参数断言）、`assets-folder-drag.spec.js`（dispatchEvent DnD）、`assets-backup-rules.spec.js`（两段 confirm 链）。

## Caveats / Not Found

- JsonYamlToolPanel 确认纯前端（零 wailsjs 导入），无后端 mock 面。
- PPT 预览 viewer 全链（pdfjs 拉 /preview-pdf）在 `vite preview` E2E 通道**结构性不可测**（该路由由 Wails dev 中间件/AssetServer 提供），候选 #20 仅覆盖降级分支。
- 「导航中心」按功能说明对应命令面板 + 最近访问/收藏链路；若另有所指（如独立视图）需主会话澄清。
- e2e-desktop 通道（真桌面）2 spec 不在缺口盘点范围。
