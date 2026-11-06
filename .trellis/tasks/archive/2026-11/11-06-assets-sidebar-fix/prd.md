# 素材管理侧边栏体验优化与问题修复

## Goal

修复「素材管理」面板（`AssetsPanel.vue` 及其复用的 `FolderTree.vue`）在用中暴露的 6 项问题，分索引侧（Office 临时文件误入库、解析失败原因不可查）与交互侧（文件夹树打开目录/粘贴串台/收起回弹/右键缺失、文件视图无法删单文件）两类，使用户能自证问题原因、自管库内文件，不必依赖开发者排查。

## 背景事实（代码勘察结论，已核实）

| 序号 | 现象 | 代码事实 |
|---|---|---|
| 1 | `~$` 锁文件被索引 | `service/assets_scan.go:1132 collectPptxEntries` 仅按 `.pptx` 后缀 + 用户排除规则 + `isPptxSplitDerivative` 过滤；`~$xxx.pptx` 有 `.pptx` 后缀会被收集，内容为锁文件非 zip → 判 `corrupted` |
| 2 | 解析失败只有计数 | `files` 表无错误字段；失败原因仅进 `log/slog`（`assets_scan.go:1075`）；`AssetScanResult.Failed` 只计数；`AssetsPanel.vue:435` 为纯文本，不可点 |
| 3a | 文件夹树无「在资源管理器打开」 | `FolderTree.vue:93-97` 菜单仅 create/rename/annotate/rebuild/delete；后端 `App.OpenInExplorer`（`app_external.go:19`）已存在，素材树未接 |
| 3b | 素材面板 Ctrl+V 串到工作目录树 | 素材域无粘贴实现；`Home.vue:586-589` 全局快捷键无条件读 `workspaceStore.selectedNode` 调 `handlePaste`，不区分当前面板 |
| 4 | 文件夹树无法收起子目录 | `FolderTree.vue:232 onNodeCollapse` 只剔除自身 key、未剔除已展开子孙；EP 2.14.5 `default-expanded-keys` 回放 `expand(子孙, autoExpandParent=true)` 强制祖先链展开 → 收起即回弹。「全部展开/全部收起/双击」三路径已接 `syncExpandedKeysFromStore` 可达面过滤，**点击箭头路径未接** |
| 5 | 文件视图无法删单文件 | 素材域只有 `DeleteAssetFolder`（磁盘递归删 + `markFileLost`），无单文件删除方法；`AssetListView.vue` 行级无操作入口，详情侧边栏亦无删除 |
| 6 | 文件夹树只有 `⋯` 无右键 | `FolderTree.vue` 仅有 `el-dropdown trigger="click"`；命令集与禁用判定（`canCreateChild/canRename/canAnnotate/canRebuild/canDelete`）已成型可复用 |

**关键关联**：问题 2 的「解析失败 1」极可能就是问题 1 的 `~$` 锁文件。两项必须一并验证，验收时以「库内存在锁文件时 failed 计数不增加」为联合判据。

## Decision（ADR-lite）

| 决策 | 结论 | 理由 |
|---|---|---|
| D1 单文件删除语义 | 物理删除磁盘文件 + 索引行 `markFileLost` 收口 | 与 `DeleteAssetFolder` 同构（磁盘删 + lost 收口），标注/评分保留待复活，不引入第二套删除契约 |
| D2 素材面板粘贴 | 仅修「不串台」，素材域内无粘贴动作 | 实现真实粘贴属新功能（后端复制方法 + 重名/跨盘/权限处理），超出本次修复定位 |
| D3 失败原因 | 落库（`files.parse_error`）+ 统计行可点弹窗 | 只做内存态则用户重启后原因丢失，恰是本条抱怨核心 |
| D4 交付节奏 | 单任务分 3 个 PR | 索引侧 / 树交互 / 文件删除三段，共享一次 PRD 与验收 |
| D5 `~$` 判定口径 | 内置硬编码，不走用户可配置规则 | 锁文件是 Office 运行时产物，无「需要索引」的合理场景 |
| D6 存量锁文件记录处理 | 扫描对账时**删除索引行**，不标 `excluded` | 锁文件无用户资产（不可打标签评分的有效内容）；标 excluded 会污染统计「已排除 N」与文件夹树「已排除」语义。磁盘文件原地不动，无数据损失 |
| D7 右键菜单 | 与 `⋯` 共用同一命令集与禁用资格判定 | 复用 `can*` 单点，禁写第二套口径 |

## Requirements

### PR1 索引侧

- **R1.1** 收集层忽略文件名以 `~$` 开头的 `.pptx`（内置规则，与既有派生排除并列）
- **R1.2** 扫描对账时清除存量 `~$` 记录的索引行（含 slides / file_tags 级联），且不得因「excluded 复活」逻辑回流
- **R1.3** `files` 表新增 `parse_error` 列（schema v12 迁移），解析失败时写入根因文本（含 `assetParseError.kind` 分类语义）
- **R1.4** `model.AssetFileEntry` 暴露 `ParseError`，文件视图/详情可见
- **R1.5** 前端统计行「解析失败 N」可点击，弹窗列出失败文件（相对路径 + 原因 + 复制全部），数据源复用宿主已持有的 `ListAssetFiles` 全量清单（前端按 `status === 'failed'` 筛选，零新增接口）

### PR2 文件夹树交互

- **R2.1** 节点菜单（`⋯` 与右键共用）新增「在资源管理器中打开」，目标为节点对应磁盘目录；根节点「全部素材」打开库根
- **R2.2** 素材面板激活（`uiStore.activePanel === 'assets'`）时，全局 Ctrl+V 不再向工作目录文件树投递粘贴
- **R2.3** 点击折叠箭头收起目录后保持收起（接入 `syncExpandedKeysFromStore` 可达面过滤）
- **R2.4** 节点右键弹出与 `⋯` 相同的命令项，禁用项与 tooltip 说明同源

### PR3 文件视图删除

- **R3.1** 后端新增 `DeleteAssetFile(libraryID, fileID)`：磁盘删除 + 索引 `markFileLost`；处理中/拆分派生产物/路径越界拒绝；返回可用提示
- **R3.2** 前端详情侧边栏提供「删除文件」入口，二次确认后执行，成功后走统一刷新入口 `refreshDataAfterMutation`
- **R3.3** App 方法签名与 wailsjs 三处手工同步（`App.js` / `App.d.ts`；无新 model struct 时不涉 `models.ts`）

## Acceptance Criteria

- [x] 库内放置 `~$客户端架构-v1.1.pptx` 后执行扫描：`failed` 计数不因该文件增加，文件视图/检索/统计均不出现该文件
- [x] 存量库中已入库的 `~$` 记录在一次扫描后被移除，统计「文件」数下降，磁盘文件保持原样
- [x] 解析失败文件在统计行可点开弹窗，显示路径与其具体原因文本，可一键复制
- [x] 早期已失败（`parse_error` 为空）的记录在弹窗中显示明确的降级文案，不显示空白
- [x] 素材树节点右键与 `⋯` 菜单项完全一致，禁用态与 tooltip 一致；「在资源管理器中打开」打开正确目录
- [x] 素材面板激活时按 Ctrl+V，工作目录文件树不产生任何粘贴动作
- [x] 含已展开子孙的目录点击折叠箭头后保持收起，刷新树数据后仍保持收起
- [x] 文件视图选中单个 PPT 后可删除，二次确认生效；删除后文件从视图消失、目录树徽标递减、标注保留为 lost
- [x] 拆分派生产物/处理中文件删除被拒绝并给出可读提示

## Definition of Done

- 后端：`go vet ./...`、`go test ./...` 通过；新增逻辑配单测（临时文件判定、⚠翻转为 ok 的防复活、parse_error 写入与读取、删除守卫）
- 前端：`npm test` 通过；新增交互配单测（右键菜单命令一致性、Ctrl+V 面板分流、折叠收起不回弹、删除确认链）
- 覆盖率：后端 service ≥76% 基线、model/server ≥80%；前端 ≥70%
- 文档：`docs/spec/assets-management.md` 增补「内置锁文件忽略」「解析失败原因可查」「单文件删除」三节；README 若有功能清单需同步
- 契约：wailsjs 手工同步完成，`npm run build` 无 MISSING_EXPORT

## Technical Approach

### PR1

1. `service/assets_scan.go` 新增 `isOfficeLockFileName(name string) bool`（`strings.HasPrefix(name, "~$")`），判定点与 `isPptxSplitDerivative` 相邻，收集层在 `.pptx` 后缀判定后调用。
2. 存量对账（现有 excluded 判定循环）：对 `~$` 路径直接删除索引行并计数（新增 `AssetScanResult` 字段如 `Removed` 复用或新增 `LockFilesRemoved`，倾向复用 `Removed` 避免模型扩张），同步 `deleteIndexRowsForPaths` 清理 FTS。
3. **防复活**：`fileHasSlides` 复活门槛只对 `lost/excluded` 生效；锁文件行走删除路径，需确保「磁盘存在 + 索引缺失」不会在下一轮被当作新文件收集——收集层已忽略，天然闭环。
4. schema v12：`ALTER TABLE files ADD COLUMN parse_error TEXT NOT NULL DEFAULT ''`，走 `migrateSchemaV12`（列存在性预判 + 单事务），基线 `CREATE TABLE files` 同步加列。
5. `setFileOutcome` 增加 `parseErr string` 参数写入；`replaceFileSlides` 成功路径清空该列（重解析成功必须归零，防陈旧原因残留）。
6. 前端弹窗：`AssetsPanel` 新增失败清单弹窗，数据源为已加载的 `fileEntries` 过滤 `status==='failed'`；空原因显示「原因未记录（该文件在旧版本中失败，重新扫描可获取）」。

### PR2

1. `FolderTree.vue` 抽出 `nodeMenuItems(data)` computed，`⋯` 下拉与右键菜单 `v-for` 同源；新增 `open-explorer` 命令与 emit。
2. 右键：节点 `@contextmenu.prevent.stop` 记录 `{ x, y, data }`，渲染绝对定位菜单（对齐 `FileTreePanel` 自建 context-menu 形态），空白区点击/滚动/Esc 关闭。
3. `Home.vue handleGlobalKeydown`：Ctrl+V 分支前置 `uiStore.activePanel === 'assets'` 判定直接 return（不 preventDefault，保留浏览器默认行为无副作用）。
4. `onNodeCollapse` 改为调用 `syncExpandedKeysFromStore()`，与另三路径同收口。

### PR3

1. `service/assets_folders.go` 或新文件增 `DeleteAssetFile`：`resolveLibraryRelPath` 校验 → 拒绝派生/processing → `os.Remove` → `markFileLost`（同事务/锁语义对齐 `DeleteAssetFolder`）。
2. `app_assets.go` 增 App 方法；`web_serve.go` 无需 `SetEventSink`（无新事件）。
3. wailsjs `App.js` / `App.d.ts` 手工补声明。
4. 前端详情侧边栏按钮 + `ElMessageBox.confirm` 单次确认形态，成功走 `refreshDataAfterMutation`。

## Out of Scope

- 素材面板内实现真实文件粘贴（复制/移动剪贴板文件）
- 用户可配置的「文件名规则」排除（`~$` 为内置判定）
- 工作目录文件树（`FileTreePanel.vue`）的既有行为改动
- 批量删除文件（仅单文件，批量删除留待后续）
- 失败文件的一键重试解析（本期只做「可见 + 可复制」）

## Technical Notes

- 规范入口：`docs/spec/assets-management.md`（§8.7 刷新统一入口、§11.2 lost 状态机、§12 排除规则）、`docs/spec/el-tree-expand-state.md`、`docs/spec/cross-layer-contracts.md`、`docs/spec/confirm-dialog.md`、`docs/spec/logging-and-errors.md`
- 组件共享范围：`FolderTree.vue` 仅被 `AssetsPanel.vue` 与自身测试引用（已核实），可安全演进
- 失败文件当前已在文件视图可见（`listFileEntries` 不过滤 status），弹窗复用该清单不新增接口
- 风险：D6 删除索引行不可逆；判据为「文件名以 `~$` 开头」，用户自建同名文件概率接近零，且磁盘文件不动

## 实施记录（2026-11-06）

- PR1 索引侧：`isOfficeLockFileName` + 收集层短路 + 存量 `removeLockFileRecords`；schema v12 `files.parse_error` + `parseErrorText`（按字符截断 500）+ 成功路径清空；`model.AssetFileEntry.ParseError`、`AssetScanResult.LockFilesRemoved`；前端统计行可点弹窗（路径 + 原因 + 复制全部）+ 详情「失败原因」行。
- PR2 树交互：`nodeMenuItems` 单点供 `⋯` 与右键菜单同源；右键菜单复用全局 `.context-menu` 样式；`onNodeCollapse` 改 `nextTick(syncExpandedKeysFromStore)`（EP `node-collapse` 早于 expanded 落值的时序坑）；`Home.vue` Ctrl+V 按 `uiStore.activePanel === 'assets'` 分流。
- PR3 删除：`DeleteAssetFile`（派生判定先于 processing；磁盘删 + `markFileLost`）+ App 方法 + wailsjs 同步；详情「删除文件」单次确认 + `closeDetail`/`clearSelection` + 统一刷新入口。
- 验证：`go vet ./...` 无问题；`go test ./service/ ./model/` 全绿（service 覆盖率 80.9%，model 98.3%）；`npm test` 119 文件 2643 用例通过（前端覆盖率 86.56%）；`npm run build` 通过。
- 文档：`docs/spec/assets-management.md` §16、`docs/spec/el-tree-expand-state.md` §5、README 功能行同步。
