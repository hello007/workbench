# Research: EP 2.14.5 确认型弹窗行为回归排查——15 处 A 类消费点核对

- **Query**: EP 2.14.5 升级后确认型错误恒弹 confirm（递归语义）链路逐处核对：组件行为 / vitest 断言一致性 / E2E 可达性与选择器风险
- **Scope**: internal（frontend/src/components + __tests__ + e2e）
- **Date**: 2026-10-20

## 核对判据与背景定性

**版本事实（已实证）**：`frontend/package.json` 声明 `element-plus: ^2.13.7`（caret 漂移），`node_modules/element-plus/package.json` 实际版本 **2.14.5**。

**组件权威行为（确认基准）**：「确认型错误恒弹 confirm（回归语义）」——catch 判确认型错误码 → `ElMessageBox.confirm` → 确认后重调 → 若仍失败且仍是确认型错误则再弹（组件内闭环，非 handleError 分流）。**该闭环仅存在于递归重调形态**；15 处中仅 #1（AssetExportDialog.runExport）是递归形态。其余 14 处为两类形态：

| 形态 | 消费点 | EP 2.13.7→2.14 行为差异暴露面 |
|---|---|---|
| 递归恒弹（确认后再失败仍确认型 → 再弹） | #1 | 第二次 confirm 在 2.13.7 关闭动画期重开异常 → 曾被误断言为 handleError 分流；2.14 起正常 |
| 顺序两段 confirm（第一段关闭后立即弹第二段） | #7/#8/#9（Obsidian 两段链）、#11（删除会话：在途取消确认→删除确认） | 同「关闭后立即重开」形态；2.13.7 下第二段可能异常 reject 落静默 catch，2.14 起正常 |
| 单次确认（确认后动作单次，失败走 handleError，无再弹） | #2/#3/#4/#5/#6/#10/#12/#13/#14/#15 | 无重开场景，EP 差异不适用；仅 E2E 选择器面与单实例问题 |

**EP 2.14 起的 E2E 断言面变化**：message-box 关闭后实例驻留 DOM（`display:none` 不移除），同链路弹第二次时裸 `.el-message-box` 命中 2 元素，Playwright strict mode 失败。两种已落地适配范式：`.first()`/`.last()`（DOM 追加序 = 弹出序，assets-export.spec.js）与 `:visible` + `filter({ hasText })`（assets-backup-rules.spec.js）。

**mock 屏蔽机制（对背景定性的一处修正）**：`frontend/src/test/setup.js` 只全局 mock Wails 绑定，**不 mock element-plus**；ElMessageBox mock 由各 spec 文件 `vi.mock('element-plus', ...)` 文件级工厂提供（如 AiChatPanel.spec.js:52、FileTreePanel.spec.js:18、AssetsPanel.spec.js:273）。效果等价：EP 运行时 confirm 被完整替换为 vi.fn()，运行时漂移被屏蔽，vitest 断言以组件代码分支为准。

**判定基点**：当前工作区 clean，bb7a1fa 下 vitest + E2E 全绿 = EP 2.14.5 下既有断言零破坏已实证。核对目标 = 找现存漂移与守护缺口，非重新验证全绿。

## 汇总表

| # | 消费点 | 触发条件 | 二次弹框 | vitest 断言 | E2E | 结论 |
|---|---|---|---|---|---|---|
| 1 | AssetExportDialog.vue:235 | E_ASSETS_EXPORT_TARGET_EXISTS | 有（递归恒弹） | 一致 | 有（bb7a1fa 已修，.first/.last） | 一致（已修复先例） |
| 2 | AssetsPanel.vue:2425 | E_ASSETS_RELINK_MISMATCH | 无（确认后单次重调，forceError→handleError） | 一致（spec:4738） | 有（assets-relink.spec.js） | 一致 |
| 3 | AssetsPanel.vue:3074 | E_ASSETS_FOLDER_NOT_EMPTY | 无（force=true 单次重调） | 一致（spec:779） | 无 | 一致，无 E2E 守护（低险） |
| 4 | LocalChanges.vue:368 | 无上游状态驱动（非错误驱动） | 无 | 一致（spec:274/288/302） | 无（仅覆盖有上游链） | 一致，无 E2E 守护（低险） |
| 5 | GitBranches.vue:214 | -d 删除失败（任意错误，不判码） | 无（强删单次，失败→handleGitError） | 一致（spec:206） | 无（仅覆盖 -d 成功） | 一致，无 E2E 守护（中险） |
| 6 | FileTreePanel.vue:1376 | 删除前主动确认（非错误驱动） | 无 | 一致（spec:798-845） | 有（file-tree.spec.js） | 一致 |
| 7 | DirectoryTree.vue:672+680 | vault 未注册状态驱动 | 有（顺序两段） | 无用例 | 无 | 组件一致，双面无守护 |
| 8 | ContentPanel.vue:817+825 | 同 #7 | 有（顺序两段） | 无用例 | 无 | 组件一致，双面无守护 |
| 9 | FileTreePanel.vue:1506+1514 | 同 #7 | 有（顺序两段） | 无用例 | 无 | 组件一致，双面无守护 |
| 10 | AiChatPanel.vue:426 | chatInFlight 状态驱动 | 无 | 一致（spec:275-313） | 无 | 一致，无 E2E 守护（低险） |
| 11 | AiChatPanel.vue:612 | 删除会话（在途时前置 #10） | 有（条件性顺序两段） | 一致（spec:907-941 明确断言 times(2)） | 无 | 一致，无 E2E 守护 |
| 12 | AiChatPanel.vue:889 | 删除模板主动确认 | 无 | 一致（spec:836-854） | 无 | 一致，无 E2E 守护（低险） |
| 13 | AiTaskHistoryPanel.vue:291/313 | 删除单条/批量清理主动确认 | 无 | 一致（spec:190-209） | 无 | 一致，无 E2E 守护（低险） |
| 14 | PortListPanel.vue:150+166 | 终止前确认（isSystem 强确认/通用二选一） | 无 | 一致（spec:139-196） | 无 | 一致，无 E2E 守护（低险） |
| 15 | ProcessManagerPanel.vue:164+184 | 同 #14 同构 | 无 | 一致（spec:157-349） | 无 | 一致，无 E2E 守护（低险） |

**统计**：现存需修漂移 0 处；组件代码与 vitest 断言一致 15/15（其中 12 处有 vitest 用例，#7/#8/#9 三处 Obsidian 链无用例）；E2E 覆盖 3 处（#1/#2/#6），无 E2E 守护 12 处。

## 逐处核对细节

### 1. AssetExportDialog.vue:235 —— E_AssetsExport_TARGET_EXISTS 覆盖确认（已修复先例）

- **组件权威行为**（runExport，226-249 行）：递归形态。`runExport(overwrite)` → `doSubmit` 抛 `code === AssetsExportTargetExists` → confirm「覆盖确认」（warning / 覆盖写入）→ 用户取消 `return` 静默 → 确认后 `runExport(true)` **递归重调**；递归再失败且仍是确认型 → **再次弹框**（恒弹闭环，组件内）；非确认型 → `handleError`。`abandoned` 守卫两处（放弃后失败静默 231 行、确认期间放弃不重调 243 行）。
- **vitest**（AssetExportDialog.spec.js）：19 行文件级 mock `confirm: vi.fn(() => Promise.resolve(true))`；173 行「确认型错误 → confirm（含后端消息）→ overwrite=true 重调成功」；194 行取消 reject 不重调；328 行恒拒形态用例。与组件分支一致。
- **E2E**（assets-export.spec.js:89-130）：bb7a1fa 已修复——旧断言「恒拒下 warning 分流」改为「恒拒下再次弹确认框」，选择器 `msgbox.first()`（首框）/`msgbox.last()`（重调后的新框，DOM 追加序 = 弹出序），并断言 ExportAssetMetadata 两次调用参数 false→true。
- **结论**：一致。bb7a1fa 修复的是 **E2E 断言侧漂移**（旧断言与组件递归权威语义不符），组件代码从未漂移。本处即「确认型错误恒弹」判定的先例范式。

### 2. AssetsPanel.vue:2425 —— AssetsRelinkMismatch 强制重链确认

- **组件权威行为**（relocateWithForceConfirm，2414-2454 行）：`run(false)` 失败判 `code === AssetsRelinkMismatch`（非该码直接 handleError）→ confirm「强制重链确认」（warning / 强制重链）→ 取消静默 return → 确认后 `run(true)` **单次**重调；`forceError` → `handleError('强制重链失败: ')`（**不递归、不二次弹框**）。成功按 reparsed/hashMatched/skippedHashCheck 三态分型 toast + `refreshAfterMaintain`。
- **vitest**（AssetsPanel.spec.js:4738-4761）：mock 先 reject 确认型码再 resolve，断言 confirm 入参（含文案/标题/type/confirmButtonText）+ `toHaveBeenLastCalledWith(..., true)` force 重调 + 强制重链分型 toast。与代码一致。
- **E2E**（assets-relink.spec.js:77-108）：恒拒 override 驱动，断言首框弹出 → 点「强制重链」→ warning `.el-message`（重调失败走 handleError 分流）+ 两次调用参数 false→true。**该 spec 注释「重调仍拒走 handleError warning 分流」与组件单次重调语义一致**——此链路 confirm 只弹一次，无 EP 关闭动画重开问题，裸 `page.locator('.el-message-box')`（95 行）单实例安全。
- **结论**：一致。

### 3. AssetsPanel.vue:3074 —— 目录删除 catch → 空目录确认重试

- **组件权威行为**（onDeleteFolder，3056-3092 行）：前端已知非空（recursiveCount>0）先预确认（3061）→ `DeleteAssetFolder(force=count>0)`；catch 判 `code === AssetsFolderNotEmpty` → confirm 后端计数文案（3074，连同内容删除）→ `DeleteAssetFolder(force=true)` **单次**重调；重调失败 → throw → 外层 catch（`'cancel'/'close'` 豁免）→ handleError。无第二次 confirm。
- **vitest**（AssetsPanel.spec.js:764-777 预确认 force=true；779-793 确认型拒绝二次确认后 force=true 重调，断言两次调用参数）。一致。
- **E2E**：无覆盖（assets 系列 spec 均未触达目录删除链）。
- **结论**：一致；无 E2E 守护。链路单次 confirm + 单次重调，EP 漂移不适用，补守护收益低，不强求。

### 4. LocalChanges.vue:368 —— 推送无上游确认

- **组件权威行为**（doPush，360-401 行）：**状态驱动而非错误驱动**——`HasUpstream` 返回 false → confirm「设置并推送」→ 取消 info + return → 确认 `PushRepo(setUpstream=true)`。内层 `catch (e)`（380 行）是 **HasUpstream 探测失败的豁免分支**（不弹框按常规推送 + warning），confirm 本身不在 catch 内。`PushRepo` 失败 → handleGitError。无二次 confirm。
- **vitest**（LocalChanges.spec.js:274-286 确认 → PushRepo(true)；288-300 取消 → 不推送；302-313 探测失败 → 常规推送）。三分支全覆盖，一致。
- **E2E**：commit-push.spec.js 断言「CommitFiles→HasUpstream→PushRepo 调用序」（有上游 mock 场景）；无上游 confirm 分支未覆盖。
- **结论**：一致；无 E2E 守护（单次弹框，EP 漂移不适用，收益低）。

### 5. GitBranches.vue:214 —— 分支删除失败强删确认

- **组件权威行为**（deleteBranch，203-229 行）：`DeleteBranch(force=false)` 失败（**catch 任意错误，不判错误码**，注释指向 not fully merged）→ confirm「强制删除确认」（`.then(() => true).catch(() => false)` 形态）→ 取消 return → 确认 `DeleteBranch(force=true)` 单次；强删失败 → 外层 catch → handleGitError。无二次 confirm。
- **vitest**（GitBranches.spec.js:179-204 -d 成功不弹确认；206 起 -d 失败 + 确认 → force=true）。一致。
- **E2E**（branch.spec.js:101-121）：仅覆盖安全删除成功（并断言二次确认弹窗不出现）；「-d 失败 → 强删确认」链未覆盖（E2E 描述符无「先错后成」序列形态，需恒拒 override + 强删失败断言，构造可行但未做）。
- **结论**：一致；无 E2E 守护。该链是确认型错误驱动的两段调用（-d 失败弹框），若未来补 E2E 须注意重调后无第二次弹框（单实例安全），按收益定可暂缓。

### 6. FileTreePanel.vue:1376 —— 文件删除主动确认

- **组件权威行为**（handleDeleteAt，1374-1407 行）：**主动确认而非错误驱动**——删除前 confirm「警告」→ confirm 自带 try/catch（取消 return）→ `DeleteFile`；失败/返回 falsy → ElMessage.error。无二次 confirm。
- **vitest**（FileTreePanel.spec.js:798-845）：确认后调用 DeleteFile + confirm 入参断言（含标题「警告」）；取消不调用；失败提示。一致。
- **E2E**（file-tree.spec.js:153-156 树内删除；319-321 截断层外文件右键删除全链）：均断言确认框文案 + 点「确定」。单次 confirm，裸 `.el-message-box` 单实例安全。
- **结论**：一致，有守护。

### 7. DirectoryTree.vue:672+680 —— Obsidian vault 两段链

- **组件权威行为**（handleOpenObsidian，665-714 行）：`OpenInObsidian` 返回 `not-registered`（状态驱动）→ **第一段 confirm**（自动注册并打开 / 打开仓库管理器，`distinguishCancelAndClose: true`）→ 确认后**立即弹第二段 confirm**（预告信任提示，`dangerouslyUseHTMLString`）→ 确认 `AutoRegisterAndOpen`；第二段取消静默（695-697）；第一段 `cancel` → 复制路径 + 打开仓库管理器（699-707）；`close` → 静默。**顺序两段 confirm = EP 2.13.7 关闭动画期重开异常的历史暴露面**；EP 2.14 下两段均正常显示，但同链路双实例驻留 → 若补 E2E 裸 `.el-message-box` 会 strict mode 冲突。
- **vitest**（DirectoryTree.spec.js）：**无用例**（仅 40-42 行 Wails 方法 mock 定义；249-292 是 handleDelete 目录删除链，与 Obsidian 无关）。
- **E2E**：grep 全部 spec 无 obsidian 命中。
- **结论**：组件代码一致无漂移；vitest 与 E2E 双面无守护。该链是三处同构链中唯一「第二段 confirm 在 EP 2.13.7 下曾可能异常 reject 落静默 catch 而测试不可知」的形态；EP 2.14.5 下行为正常，补守护按收益定（E2E 补须用 `.first()/.last()` 或 `:visible`+filter）。

### 8. ContentPanel.vue:817+825 —— 同构 Obsidian 两段链

- 与 #7 逐字节同构（808-859 行，入口参数来自 `workspaceStore.selectedNode.path`）。
- **vitest**（ContentPanel.spec.js）：仅 569-581「Esc 有修改时二次确认」用例（对应 921 行编辑退出确认，非 Obsidian 链）；Obsidian 链无用例。**E2E**：无。
- **结论**：同 #7——组件一致，双面无守护。

### 9. FileTreePanel.vue:1506+1514 —— 同构 Obsidian 两段链

- 与 #7 逐字节同构（1499-1548 行）。**vitest**（FileTreePanel.spec.js）grep obsidian/AutoRegister 无用例；**E2E** 无。
- **结论**：同 #7——组件一致，双面无守护。

### 10. AiChatPanel.vue:426 —— 在途任务统一确认（confirmDiscardInFlight）

- **组件权威行为**（423-441 行）：`chatInFlight` 状态驱动单次 confirm（取消任务并继续 / 留在当前）→ 取消 return false → 确认后 `cancelChatTask`；取消失败 → handleError → return false（不继续动作，防带在途任务继续）。被切换目录/切换会话/新建/删除四处复用。无二次 confirm。
- **vitest**（AiChatPanel.spec.js:275-313 切换确认与取消失败不切换；926-941 删除会话前置取消确认被拒不删除）。一致。
- **E2E**：ai-chat.spec.js 有「会话切换」describe（普通切换），在途任务确认链未覆盖。
- **结论**：一致；无 E2E 守护（单次弹框，EP 漂移不适用）。

### 11. AiChatPanel.vue:612 —— 删除会话（条件性顺序两段 confirm）

- **组件权威行为**（handleDeleteSession，603-626 行）：在途任务属于该会话时先走 #10 的 confirmDiscardInFlight（**第一段**）→ 关闭后**立即弹第二段**删除确认（612）→ 确认 `deleteChatSession`；删除失败 → handleError。**条件性顺序两段 = EP 关闭动画重开风险面**（与 #7 同形态，仅条件触发）。
- **vitest**（AiChatPanel.spec.js:907-924）：**明确断言 `confirm` 被调用 `toHaveBeenCalledTimes(2)`**（「两次确认：先取消任务再删除」）+ CancelChatTask/DeleteChatSession 调用；926-941 首段被拒则两段均不发生。与代码一致，双 confirm 语义已有守护。
- **E2E**：ai-chat.spec.js 无删除会话用例。
- **结论**：一致；无 E2E 守护。若补 E2E 须用多实例安全选择器（双 confirm 同链）。

### 12. AiChatPanel.vue:889 —— 删除模板确认

- **组件权威行为**（887-906 行）：单次确认 → `removeChatTemplate`；失败 handleError。无二次。
- **vitest**（AiChatPanel.spec.js:836-854 确认调用 + 取消拒绝）。一致。**E2E**：无。
- **结论**：一致；无 E2E 守护（单次弹框，收益低）。

### 13. AiTaskHistoryPanel.vue:291 与 313 —— 删除历史 / 批量清理

- **组件权威行为**：removeOne（289-309）与 clearOld（311-328）各自独立单次 confirm；确认后调用删除/清理接口，失败 ElMessage.error（组件内自处理，不走 handleError）。无二次 confirm。
- **vitest**（AiTaskHistoryPanel.spec.js:190-199 removeOne 调用断言；201-209 clearOld 调用断言；confirm 由文件级 mock 默认 resolve 放行）。一致。**E2E**：无（AI 任务历史面板无 spec）。
- **结论**：一致；无 E2E 守护（低险）。

### 14. PortListPanel.vue:150 与 166 —— 终止确认（isSystem 强确认 / 通用确认二选一）

- **组件权威行为**（requestConfirm，143-180 行）：`isSystem`（后端下发标志）→ error 级红色强确认（150，危险操作/仍然结束）；否则 warning 级通用确认（166，含多端口连带提示）。**同一次终止只弹其一，非叠加**。terminate（186-212）：确认后 `StopProcess(pid, false)`；失败 handleError；ProcessNotFound 自动刷新。无第二次 confirm。
- **vitest**（PortListPanel.spec.js:139-155 error 级强确认入参断言；157-172 warning 级文案含连带提示；174-184 单端口无连带提示；186-196 取消不调用）。一致。**E2E**：无（工具箱无进程/端口 spec）。
- **结论**：一致；无 E2E 守护（工具箱 E2E 整体缺失属既有事实，低险）。

### 15. ProcessManagerPanel.vue:164 与 184 —— 同构强杀二段选择链

- **组件权威行为**（requestConfirm，159-194 行）：与 #14 同构——isSystem → L2 error 级强确认（164）；否则 L3 通用确认（184，kill 文案更强硬）。terminate（203-234）：确认后 stop/kill 单次调用，失败 handleError，ProcessNotFound 刷新。无第二次 confirm。
- **vitest**（ProcessManagerPanel.spec.js:157 强杀文案断言；230-236 强制结束文案；338-349 取消不进入等待态）。一致。**E2E**：无。
- **结论**：一致；无 E2E 守护（低险）。

## B 类核对：E2E 全量 message-box 断言面

grep `frontend/e2e/` 全部 17 个 spec，`.el-message-box` 断言共 6 个文件 12 处：

| 文件:行 | 链路 | 选择器形态 | 多实例风险 |
|---|---|---|---|
| assets-export.spec.js:115 | 覆盖确认（递归恒弹） | `.first()` / `.last()` | 已适配（bb7a1fa 先例） |
| assets-backup-rules.spec.js:167+172 | 删除库升级确认（顺序两段） | `:visible` + `filter(hasText)` | 已适配（先例之二） |
| assets-backup-rules.spec.js:136 | 还原确认（单次） | 裸 | 无（单实例） |
| assets-relink.spec.js:69/95/143/170 | 重链/前缀替换/清理（各单次） | 裸 | 无（各链单次 confirm） |
| file-tree.spec.js:154/319 | 文件删除确认（单次） | 裸 | 无 |
| submodule.spec.js:142 | 子模块删除确认（单次） | 裸 | 无 |

**结论**：B 类裸选择器 9 处全部是单次 confirm 链（确认后无同链路第二框），单实例下 strict mode 无冲突；两段 confirm 链（assets-export、assets-backup-rules）均已用多实例安全选择器适配。**现存无裸选择器多实例风险**。B 类单次确认行为本身不受 EP 漂移影响，仅断言面已核对完毕。

## EP 组件既有断言面（E2E spec 清单）

17 个 E2E spec 文件（bb7a1fa 下全绿 = EP 2.14.5 下既有断言零破坏实证）及其覆盖的 EP 组件类型：

| spec | 覆盖 EP 组件类型 |
|---|---|
| smoke / session-restore | 面板渲染、活动栏 |
| branch / merge / commit-push / submodule / repo-config | el-dialog 弹窗、el-table、el-dropdown、el-tabs |
| diff-tool | 设置表单、el-dialog |
| file-tree | el-tree 文件树、右键菜单、message-box |
| ai-function / ai-chat | 面板、el-dialog、下拉 |
| assets-split / assets-batch-tag / assets-relink / assets-backup-rules / assets-folder-drag / assets-export | el-dialog、el-dropdown 维护菜单、message-box、FolderTree 拖拽、el-table |

**一句话结论**：EP 2.14.5 下 17 个 spec 全绿，既有断言面（弹窗/下拉/树/表格/message-box）零破坏已实证；EP 行为差异唯一暴露点（关闭动画期重开 + 实例驻留 DOM）已由 assets-export（`.first()/.last()`）与 assets-backup-rules（`:visible`+filter）两处先例消化，后续新补两段 confirm 链 E2E 时复用其一即可。

## Caveats / 未决

- #7/#8/#9（Obsidian 两段链）vitest 与 E2E 双面无守护，是 15 处中唯一「EP 2.13.7 曾可能异常 reject 落静默 catch 而任何测试不可知」的形态；组件代码本身一致，是否补守护按收益决策（不强制）。
- #5（分支强删确认）E2E 缺口构造需「恒拒 override」形态（E2E 描述符无先错后成序列），可行但未做；vitest 已守护，风险可控。
- LocalChanges.vue:368 在任务清单中标注为「catch 内 confirm」，实测为「无上游状态驱动 confirm + HasUpstream 探测失败豁免 catch」，本表按实际行为登记。
- 任务背景中「vitest 层 ElMessageBox.confirm 全局 mock（frontend/src/test/setup.js）」经核为文件级 `vi.mock('element-plus')`（setup.js 仅 mock Wails 绑定），判定结论不受影响。
