# Research: 第二批 E2E 用例清单建议（候选 #8-#20 复核）

- **Query**: 复核 E2E 覆盖矩阵候选 #8-#20 时效性与可 mock 性，产出第二批用例清单建议（约 12 条）
- **Scope**: internal（coverage-matrix.md、frontend/e2e 24 spec、wails-mock-defaults.js、wails-init.js、组件源码、git log 3065efd..HEAD）
- **Date**: 2026-10-21
- **矩阵基线**: `.trellis/tasks/archive/2026-10/10-21-e2e-coverage/research/coverage-matrix.md`（§4 Top 20 / §4.1 落地标注）

---

## 1. 时效性总述（复核窗口：3065efd..HEAD）

**第一批落地（3065efd，五高价值链路 15 用例）之后，前端源码改动合计仅 2 个文件：**

| 文件 | 改动 | 与候选 #8-#20 的关系 |
|---|---|---|
| `frontend/src/components/FilePreviewRenderer.vue` | +6 行：mermaid `initialize` 增 `layout:'dagre'` + `look:'classic'` 视觉保持双键（mermaid 12 升级配套，098a3c1） | 该组件是候选 #20 的宿主，但改动点仅涉 markdown 分支的 mermaid 渲染配置，**与 #20 断言面（pptx 降级分支）零交集** |
| `frontend/package.json` / `package-lock.json` | mermaid 12.2.0 + npm audit fix 依赖变更 | 不涉组件行为 |

另新增 `frontend/e2e/markdown-mermaid.spec.js`（mermaid 12 升级专项冒烟，不占用矩阵编号）。

**结论：#8-#20 涉及的全部组件（PptSplitPanel / DashboardView / AiChatPanel / AssetsPanel / PortListPanel / HostsPanel / EnvVarPanel / AssetTagManagerDialog / AssetFilterPopover / AssetBatchRatingDialog / TerminalPanel）自矩阵定稿（同日）以来零改动，矩阵 §1.3 改动频次与风险档全部继续有效。** 全部候选时效结论 = **未变**。

---

## 2. 候选复核表（#8-#20 逐条 + 2 个无编号 PRD 倾向项）

mock 缺省核对基准：`frontend/src/test/wails-mock-defaults.js` 基础表 + E2E extra 表（2026-10-21 现状，含第一批补入的终端 5 方法）。既有覆盖核对基准：`frontend/e2e/` 24 个 spec 全量 grep。

| # | 候选 | 时效结论 | 可 mock 性 | 既有覆盖 | 建议 |
|---|---|---|---|---|---|
| 8 | PPT 拆分面板批量链（SplitPptxFolder + `pptx-split:batch:progress` 事件 + CancelPptxSplit） | 未变 | **可行**：`SplitPptxFolder`（`{dirPath,total,items}` 缺省在表）、`CancelPptxSplit:false`；进度事件载荷 `{index,total,currentFile}` 按 dirPath 过滤（PptSplitPanel.vue:218），用 `__deferred__` 挂起 resolve + `emitWailsEvent` 驱动中间态（assets-cart.spec 先例范式）；汇总来自 Promise 返回值非事件（PptSplitPanel.vue:92） | 无（assets-split.spec 测素材域 SplitAssetLibrary；toolbox-panel.spec 用例 2 仅路过挂载） | **落地**（高险：三批近期改动事件消费面零守护） |
| 9 | 全局看板渲染与钉选互锁 | 未变 | **可行但有一个缺省缺口**：`GetDashboardPinned/[]`、`Add/RemoveDashboardPin/true`、`IsDashboardPinned/false`、`GetDashboardStatuses/RefreshDashboardStatuses/[]` 均在 extra 表；**`GetRepoFilterList` 两表均无**——Proxy 兜底 resolve null，DashboardView.vue:218 `const items = await GetRepoFilterList(dirId)` 后直接遍历，null 会 TypeError。落地前置：mock 表补 `GetRepoFilterList: []`（+ 触及过滤保存链时补 `SaveRepoMeta`/`CleanMissingRepoMeta`） | 无 | **落地**（PRD 倾向；mock 补一处缺省即低成本快赢） |
| 10 | AI 会话目录 CRUD 与选中过滤 | 未变 | 可行：5 目录方法缺省齐全（`AddChatDirectory:null`、`ListChatDirectories:[]`、Update/Remove/Reorder 均 true） | **部分覆盖**：ai-chat.spec 用例 1 已含「添加目录」入口链；选中按 directoryId 过滤、重命名/删除确认、拖拽排序未覆盖 | **候补**（增量价值中等，可并入 AI 选择题用例同 spec 追加） |
| 11 | 幻灯片标注保存双代次守卫 | 未变 | 可行：`UpdateAssetSlideMeta:null`、`ListAssetSlides:{items,total}` 齐全；旧代次响应注入用 `__sequence__` 描述符 | 无 | **落地**（P1 真实踩坑「二次保存写回旧数据」唯一自动守卫面） |
| 12 | facet 筛选参数换算（ratings/mtime 纳秒区间/sizeFrom/sizeTo） | 未变 | **可行（最低成本）**：`SearchAssets` 缺省注释明示 facet 字段经参数断言消费；换算单点 `AssetFilterPopover` 模块导出函数；`getWailsCalls` 参数级断言现成 | 无 | **落地**（PRD 倾向；与 #14 同 spec 两用例） |
| 13 | 批量评分弹窗主链（混合明细 + 失败行染 + 不清选中） | 未变 | **可行**：`BatchSetAssetRating:{total,succeeded,failed,items}` 缺省在表（注释明示 label 后端拼装）；与 assets-batch-tag.spec 同构可复制 | 无（batch-tag 是打标签，非评分） | **落地**（PRD 倾向；v8 新功能零 E2E） |
| 14 | 检索排序档位切换（sortBy/sortDir 透传） | 未变 | 可行：`SearchAssets` 缺省注释明示排序键透传断言；`{sortBy:'mtime',sortDir:'desc'}` 参数断言一步到位 | 无 | **落地**（与 #12 合并一条清单项） |
| 15 | 端口占用一览（扁平化 + 行级防重） | 未变 | 可行：`ListProcesses:[]`、`StopProcess` 缺省在表；扁平化逻辑纯前端 | 无（toolbox-panel.spec 用例 3 测进程管理面板终止主链，未触 PortListPanel） | **落地**（与 #4 停止链同构，成本低） |
| 16 | hosts/环境变量只读渲染与脱敏 | 未变 | **可行（fixture 完整现成）**：`GetHosts` 四行类别（entry/comment/unknown/blank）+ `ListEnvVars` 三组含 2 个 sensitive 项均在基础表 | 无 | **候补首位**（成本最低但风险亦低；12 条满额时的第一替补） |
| 17 | 缩略图画廊懒加载 + 失败重试恰一轮 | 未变 | 可行：`GetAssetThumb:''`（无预览占位契约注释在表）、`RetryAssetThumb:null`、`AssetThumbsAvailable:false`、`assets-thumbs:*` 事件描述符现成；失败→done 自动重试「恰一轮」let 标记链用事件序列驱动 | 无 | **落地**（重试时序无自动守护） |
| 18 | 标签树拖拽移动 + 失败回滚 | 未变 | 可行：`ListAssetTags` 两级 fixture 在表、`MoveAssetTag:null`（reject 场景 `{__error__}` override）；el-tree `node-drop` 落点参数构造复杂度中 | 无（D8 拖拽新落地零守护） | **落地**（乐观更新回滚唯一自动守卫面） |
| 19 | 保存检索条件 upsert + 异库调取重映射 | 未变 | 可行：`ListAssetSavedSearches:[]`、`SaveAssetSavedSearch:null`、`DeleteAssetSavedSearch:null` 齐全（注释明示形状与 override 要点）；重映射丢弃计数纯前端逻辑 | 无 | **落地**（v7 功能零 E2E） |
| 20 | 文件树 pptx 预览降级链 | **未变**（宿主 FilePreviewRenderer 有 +6 行 mermaid 改动，与 pptx 降级分支零交集） | **可行（mock 缺省即降级态）**：`ConvertPptxForPreview` 缺省注释明示 `E_PPTX_PREVIEW_UNAVAILABLE` 降级形态；`OpenWithDefaultApp:true` 在表 | 无（markdown-mermaid.spec 仅覆盖 markdown 分支） | **落地**（近零成本：降级分支 mock 即缺省） |
| —（矩阵 §2.3 表行） | AI 对话选择题 chat-question 卡片渲染分流 | 未变（AiChatPanel 零改动） | **可行**：RunChat `__events__` 注入含 ` ```chat-question JSON``` ` 围栏的 output 事件（围栏格式单点 `utils/chatMarkdown.js` fence 渲染分流，lang='chat-question' 且 JSON 合法出卡片）；交互全 DOM（事件委托点击 option → selected → submit → `chat-question-answered` 置灰 + 发送选择消息） | 无（vitest 仅覆盖 chatMarkdown 围栏解析纯函数，交互链未覆盖） | **落地**（PRD 倾向） |
| —（矩阵 §2.2 表行） | 终端全屏层开合 + 全局 ESC 过滤 | 未变（TerminalPanel 零改动） | **可行（纯前端零 mock）**：断言面全 DOM——全屏按钮切 `is-fullscreen` class（z-index 1500）、ESC 退出、EP 弹层内 ESC 不退全屏（`.el-overlay` 目标过滤）、`defaultPrevented` 过滤、非全屏期 keydown 监听随态挂载/卸载 | 无 | **落地**（PRD 倾向；真实 Chromium 键盘/层叠是 jsdom 盲区，E2E 独有价值最高的一条） |

---

## 3. 第二批建议清单（12 条）

排序原则：PRD 倾向五项全保 → 高险零守护面 → 快赢参数断言面。spec 归属遵循「同域同文件追加、专项链路新文件」。

| 序 | 用例（操作路径 → 核心断言） | spec 归属 | mock 要点 |
|---|---|---|---|
| 1 | 终端全屏 ESC 过滤链：点全屏按钮 → `.terminal-panel.is-fullscreen` 出现（z-index 1500）→ ESC 退出恢复停靠 → 打开 EP 弹窗后按 ESC 仅关弹窗全屏保持 → 关闭全屏后 keydown 监听注销 | 追加 `terminal-tabs.spec.js`（新 describe） | 零 override（纯前端）；ESC 用 `page.keyboard.press('Escape')`；弹窗用现成入口（如命令面板）制造 `.el-overlay` 语境 |
| 2 | AI 选择题卡片交互：发送消息 → RunChat output 事件携带 ` ```chat-question JSON``` ` 围栏 → 卡片渲染（题目/选项/禁用提交钮）→ 点选 option（selected 态）→ 提交钮启用 → 点击后卡片置灰（chat-question-answered）→ 选择答案作为消息发出 | 追加 `ai-chat.spec.js`（新 describe） | RunChat override：`__preEvents__` queued/started + `__events__` output（text 含围栏）+ done；围栏 JSON 形状对齐 `chatMarkdown.js` 解析契约（question/options/multi 字段名以该文件为准） |
| 3 | PPT 拆分批量链：选目录 → SplitPptxFolder 挂起 → `pptx-split:batch:progress` 事件推进度（{index,total,currentFile}）→ 放行 PptxBatchResult（含 1 条 error 明细）→ 汇总渲染成功/失败明细 → 在途取消按钮态 | 新 `pptx-split-panel.spec.js`（事件序列复杂，独立文件防 toolbox-panel 膨胀） | `SplitPptxFolder: {__deferred__:'pptx-batch'}` + `releaseDeferred` 放行 `{dirPath,total,items:[...]}`；进度经 `emitWailsEvent`（事件先于 resolve 时序）；CancelPptxSplit 缺省 false 即可断言调用参数 |
| 4 | 看板渲染与钉选互锁：状态列表渲染（GetDashboardStatuses fixture）→ 手动刷新 → 钉选 Add → IsDashboardPinned 翻转 → 取消钉 Remove → 过滤器选目录触发 GetRepoFilterList | 新 `dashboard.spec.js` | **前置：wails-mock-defaults 补 `GetRepoFilterList: []`**（当前两表均无，Proxy null 会在 DashboardView.vue:218 消费链 TypeError）；钉选链方法缺省已齐 |
| 5 | facet 筛选参数换算：打开筛选 popover → 勾评分档 + 相对时间档 + 大小区间 → SearchAssets 收到 ratings/mtimeFrom/mtimeTo/sizeFrom/sizeTo 精确值（含止日末毫秒锚点） | 新 `assets-filter-sort.spec.js`（用例 1） | SearchAssets 缺省即可（参数断言消费）；期望值按 AssetFilterPopover 模块导出换算逻辑预计算 |
| 6 | 排序档位透传：工具行下拉切 mtime 降序 → applySearch → SearchAssets 收到 `{sortBy:'mtime',sortDir:'desc'}` | 同上（用例 2） | 同上，零额外 override |
| 7 | 批量评分主链：选中 3 条目 → 弹窗设 4 星 → 提交 → BatchSetAssetRating 收混合明细（2 成功 1 失败）→ 失败行染 + completed 不清选中 | 新 `assets-batch-rating.spec.js`（复制 batch-tag 形态） | `BatchSetAssetRating` override：`{total:3,succeeded:2,failed:1,items:[...]}`（item 含后端拼装 label 字段）；目标收集链复用 batch-tag 的 targets 构造 |
| 8 | 幻灯片标注保存双代次守卫：保存标注 → await 刷新期间 `__sequence__` 注入旧代次 ListAssetSlides 响应 → 断言不回填旧标注、表单不回退旧值 | 新 `assets-slide-meta.spec.js` | `ListAssetSlides: {__sequence__:[旧代次页, 新代次页]}`；UpdateAssetSlideMeta 缺省 null；断言面=保存后 selected 引用与表单值 |
| 9 | 端口占用一览与终止：无关键字默认视图扁平化一行一端口（同 PID 双端口两行）→ 终止其中一行 → 同 PID 另一行同步进入在途态（行级防重）→ 确认 → StopProcess 参数 | 追加 `toolbox-panel.spec.js`（同面板域 describe） | ListProcesses override 两行同 PID 异端口 fixture；StopProcess 缺省 `{stopped:true,method:'forced'}`；确认链 `.first()/.last()` 范式照抄用例 3 |
| 10 | 缩略图懒加载 + 失败重试恰一轮：画廊挂载逐卡 GetAssetThumb(tier=0) → 一卡失败占位 → `assets-thumbs:done` 事件 → 自动 RetryAssetThumb **恰一次**（let 标记复位语义）→ 再次 done 不再重试 | 新 `assets-thumbs.spec.js` | GetAssetThumb override 按序返回（一卡空串=失败态）；重试链用 `__sequence__` 或事件序列驱动；断言 RetryAssetCalls 恰 1 次 |
| 11 | 标签树拖拽移动 + 失败回滚：管理弹窗拖子标签到另一父 → MoveAssetTag resolve → 树重排；reject 场景 → 乐观更新回滚重拉恢复原位 | 新 `assets-tag-tree.spec.js` | resolve 场景 MoveAssetTag 缺省 null；reject 场景 override `{__error__:'移动失败'}`；el-tree node-drop 需 dispatchEvent 构造落点参数（folder-drag.spec DnD 先例 + 落点换算走 `resolveTagMoveDrop` 单点） |
| 12 | 保存检索条件 upsert + 异库重映射：保存当前条件（含 facet 快照）→ 重名拒绝排除自身 → 切库 → 调取 → tagIds 失效按 tagNames 重映射 miss 丢弃并计数提示 | 新 `assets-saved-search.spec.js` | ListAssetSavedSearches override 跨库 fixture（tagNames/tagIds 同序成对）；SaveAssetSavedSearch override 返回带 ID 对象；DeleteAssetSavedSearch 缺省 null |

**候补（按序替补）**：#16 hosts/env 只读渲染与脱敏（fixture 完整现成，最低成本）；#10 AI 目录 CRUD 补全（选中过滤 + 重命名/删除确认，可并入清单 2 的 spec）。

**落地前置事项汇总**（唯一 mock 缺省缺口）：`wails-mock-defaults.js` 基础表补 `GetRepoFilterList: []`（清单 4 前置，其余 11 条方法缺省已全部就绪）；清单 1 若要顺带断言「非全屏期 ESC 监听注销」，见 §5.1 保留面评估的 mock 契约补齐前提。

---

## 4. PRD 倾向复核

| PRD 倾向面 | 对应候选条目 | 时效结论 |
|---|---|---|
| 素材 facet 多维筛选 | #12（清单 5） | 未变：AssetFilterPopover 换算单点与 SearchAssets 参数契约自矩阵定稿零改动；mock 缺省注释已明示 facet 字段断言消费方式 |
| 批量评分 | #13（清单 7） | 未变：BatchSetAssetRating 缺省形状（含 label 拼装说明）在表；与既有 batch-tag.spec 同构形态可直接复制 |
| 终端 z-index 层叠与全局 ESC | 矩阵 §2.2 表行（未进 Top 20 编号；本报告清单 1 补入） | 未变：TerminalPanel 零改动；ESC 过滤三规则（defaultPrevented / `.el-overlay` 目标 / xterm textarea）注释完整（TerminalPanel.vue:398-407），断言面可逐条 DOM 表达 |
| 看板 pin 链 | #9（清单 4） | 未变：DashboardView 零改动；唯一前置是补 `GetRepoFilterList` 缺省 |
| AI 对话选择题卡片 | 矩阵 §2.3 表行（未进 Top 20 编号；本报告清单 2 补入） | 未变：AiChatPanel 与 chatMarkdown.js 均零改动；围栏渲染分流契约（lang='chat-question' + JSON 校验）稳定，vitest 已覆盖解析纯函数，E2E 增量在真实交互链 |

---

## 5. 两保留面评估

### 5.1 EventsOn 闭包注销缓冲区断言（第一批 #3 留痕）

**结论：可行，但前提是先补 E2E mock 层的 EventsOn 返回值契约（本次不动手，留作实施前置）。**

事实链（已逐一核实）：

1. 真实 Wails runtime 的 `EventsOn` 返回「注销本监听器」闭包，前端组件普遍消费：AssetsPanel 12 个（4573-4584）、useTerminal 2 个（163/172）、AiChatPanel 4 个（948-951）、PptSplitPanel 4 个（337-340）、AiFunctionPanel 4 个（752-755）、CommitHistory / LocalChanges 各 1 个。
2. 组件卸载调用形态为 truthy 防御：`offProgress && offProgress()`（AssetsPanel.vue:4591-4602 同构 12 行）。
3. **E2E mock（e2e/wails-init.js:147-149）的 EventsOn/EventsOnMultiple 返回 undefined**——与真实 runtime 契约不一致。后果：E2E 中组件卸载不炸（truthy 防御兜住）但**注销从未发生**，`window.__wailsEventHandlers` 只增不减。vitest 侧 mock 是一致的（`EventsOn: vi.fn(() => vi.fn())`，useTerminal.spec.js:49-50 注释明示「与真实 Wails runtime 一致」）——E2E mock 是唯一偏差点。
4. mock 注册表 `window.__wailsEventHandlers` 已存在（按事件名分组数组），`page.evaluate` 一行可读 handler 数量；wails-mock.spec.js 已自测多 handler 广播与 EventsOnce 自移除，但未覆盖闭包注销。

补齐后断言面（实施时）：

- `EventsOn/EventsOnMultiple` 返回按引用 splice 本 handler 的闭包（对齐 transport/events.js 浏览器桥的精准注销语义）；
- 关闭非活动 tab（或切走卸载任意监听组件）后断言 `__wailsEventHandlers['terminal-output'].length` 回落；
- 补强断言：注销后 `emitWailsEvent` 派发同名事件，已销毁实例无副作用（无 CloseTerminal 误调、无 DOM 写入）。

不改 mock 则该断言不可行：注销路径在 E2E 中是死路（offX 恒 undefined），断言 handler 数回落必然失败，断言「不回落」则是假绿。

### 5.2 KillProcess 强杀链挂点（第一批 #4 留痕）

**结论：挂点存在，可实施，建议本批纳入（可并入清单 9 同 spec）。**

「pty 事件驱动缺 bound method 挂点」的顾虑经核实**不成立**——进程管理域无 pty 依赖，强杀链是纯 bound method 请求-响应控制流（ProcessManagerPanel.vue:203-235 `terminate`）：

- 强杀入口：`action === 'kill'` → L3 确认框（标题「强制结束进程」、确认钮 danger 样式，164-188 行两级 confirm）→ `KillProcess(row.pid, killTree.value)` → success toast「进程已强制结束」→ `loadList()` 刷新。
- 停止链 forced 分支：`StopProcess` 返回 `method !== 'graceful'` 即落「已强制结束」文案（217-222 行）；`stopped` 字段前端未消费，mock 值任意。
- 失败分流：`StopProcess` reject → handleError + `code === ProcessNotFound` 时自动刷新列表（225-230 行）——用 `{__error__, __code__}` 描述符可驱动（错误码常量名以 `frontend/src/utils/error.js` 为准）。

第一批 #4 已覆盖停止主链 graceful 分支 + isSelf 硬禁 + 行级防重 + 确认链；本条是同构补全：强杀 L3 确认 + KillProcess 参数断言 + forced 文案 + 失败 toast/自动刷新分流，全部 DOM 与 `getWailsCalls` 可表达。

---

## 6. Caveats / Not Found

- `GetRepoFilterList` / `SaveRepoMeta` / `CleanMissingRepoMeta` 是 #8-#20 相关方法中仅有的 mock 缺省缺口（矩阵 §1.2 已登记，本次复核确认仍未补）；Proxy 兜底 null 对 DashboardView 是硬错误，落地清单 4 前必须补。
- 选择题围栏 JSON 的精确字段名（question/options/multi 等）未在本次逐字段核对，写用例时以 `frontend/src/utils/chatMarkdown.js` 解析函数为唯一契约源。
- PortListPanel 扁平化消费的 ListProcesses 行形状（ports 数组元素 port/protocol/state 字段）以 `model.ProcessInfo` / models.ts 为准，mock 表注释已载明。
- 禁改边界遵守：本报告未改动 `frontend/e2e/` 下任何文件，§5.1 的 mock 契约补齐与 §3 清单 4 的缺省补充均为建议事项，待主会话决策后由实施批次执行。
