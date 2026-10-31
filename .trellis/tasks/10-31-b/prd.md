# 素材拆分 B 方案：就地替换 + 单页产物进索引

## Goal

在 PPT 按页拆分中新增第三种落点「就地替换」：拆分到源文件旁的同名文件夹后**删除原文件**，且单页产物以可索引命名进入素材库，使每个页面成为独立素材，可单独删除、移动、重命名、调整顺序与标注。

现有两种落点（缓存目录 `data/assets/<libId>/derived/`、源目录同名文件夹）产物一律被 `isPptxSplitDerivative` 排除，不进索引。本方案新增的落点产物**必须进索引**，因此不能沿用现有命名。

## Background（现状）

- 拆分核心：`SplitPptxPerPageWithProgress` 输出 `<源名主干>-<3位页序>.pptx`（`service/pptx_split.go:140`）。
- 排除规则：`isPptxSplitDerivative(rel)` 判定「父目录名 + `-` + 纯数字」形态即视为派生副本，扫描时不索引（`service/assets_scan.go:37-57`）。
- 落点编排：缓存目录经「源副本代拆」实现（先复制源到目标父目录，拆完删副本），源目录直调核心（`service/assets_split.go:300-330`）。
- 索引与标注：`files`（path 唯一）、`slides`（file_id, slide_num）、`slide_tags`/`slide_meta`（file_id, slide_num）、`thumb_cache`。

## Requirements

### R1 落点三态

- 拆分落点扩为三态：`cache`（默认，现状）/ `source`（现状）/ `replace`（新增，拆完删源）。
- App 绑定 `SplitAssetFile`、`SplitAssetLibrary` 增加落点参数（枚举或字符串常量），并同步 `frontend/wailsjs` 三处与浏览器通道可用性。
- 三态的产物布局：
  | 落点 | 产物路径 | 进索引 |
  |---|---|---|
  | cache | `data/assets/<libId>/derived/<镜像源目录>/<源名主干>/<源名主干>-NNN.pptx` | 否（现状） |
  | source | `<源目录>/<源名主干>/<源名主干>-NNN.pptx` | 否（现状） |
  | replace | `<源目录>/<源名主干>/<NNN>-<源名主干>.pptx` | **是** |

### R2 可索引命名（replace 落点）

- 页序前缀命名 `<3位页序>-<源名主干>.pptx`：`stem` 前缀为数字而非父目录名，不命中 `isPptxSplitDerivative`，扫描后自然成为独立文件素材。
- 不改动 `isPptxSplitDerivative` 规则本身（现有两态产物继续被排除）。
- 已知边界：用户把产物重命名为 `<源名主干>-001.pptx` 形态会命中排除、下次扫描退出索引——需在界面提示「不要改成与原文件同名」。
- 页序以文件名字典序表达（`001` 起三位），用户拖动重排后需自行重命名编号；素材库内排序按修改时间/名称/星级，不依赖页序前缀。

### R3 删源安全语义

- 仅当该文件**全部页拆分成功**且逐页产物 `Stat` 校验通过，才删除源文件。
- 任一页失败 / 取消 / 产物校验失败 → **保留源**，返回失败明细。
- 删除失败（文件被 PowerPoint/WPS 占用、只读、权限）→ 保留源 + 明确错误（`E_ASSETS_SPLIT_SOURCE_DELETE_FAILED`，新增码三处同步），可重试。
- 单文件入口与库批量入口同口径；批量中单文件失败不影响其他文件。

### R4 标注与标签迁移

- 拆分成功、删源前，把源文件的用户资产迁移到各页产物：
  - 文件级标签 / 星级 / 备注 → 复制到**每个**产物（每页都带源文件标签与评分）。
  - 页级标注（`slide_tags` / `slide_meta`，键 `(file_id, slide_num=k)`）→ 迁移到第 k 页产物的 `slide_num=1`。
- 迁移在事务内完成，失败则整文件不删源（回退已迁移数据）。
- 迁移完成后删除源 `files` 行（级联清 slides/标注），避免遗留 lost 噪音（**本方案选择直接删记录，不做 lost**——标注已迁移，lost 只增加手工清理负担）。此决策需在 PRD 评审时确认。

### R5 取消与失败回滚

- replace 落点下，取消或部分失败时**删除已产出的单页文件**（否则会进索引，与仍在的源形成重复素材）。
- 前两落点保持现状（取消保留已产出，幂等重拆覆盖）。

### R6 重拆与状态

- replace 成功后源已不存在，`split_records` 标记 `source-removed`，「重拆」入口对该记录禁用。
- 拆分成败均写记录（含落点、产物目录、页数、是否删源）。

### R7 前端

- 拆分确认弹窗三选一，replace 附不可逆强确认：「原始文件将被删除，单页文件保存于 `<源名主干>/` 目录，且会作为独立素材进入素材库」。
- 批量拆分同口径；结果汇总区分「已删除源 / 保留源（失败）/ 已回滚」。
- 拆分完成后触发 `refreshDataAfterMutation`（新增素材需要重扫加载）。

## 设计要点

- **命名与排除规则的解耦**：不新增排除白名单、不改判定函数，靠「页序前缀」命名区分。代价是规则隐式依赖命名形态，须在规范文档写死。
- **索引规模**：1208 页源库若全部 replace，将产生 1208 条素材、1208 个单页 pptx。单页文件解析与缩略图（1 页/文件）成本低，但 `ListAssetFiles` 分页、统计、检索规模随之上升；需在性能基线中登记。
- **缩略图**：单页文件仍是 pptx，走既有 COM 链路；按需 hd 预取逻辑（`thumbPrefetchPages`）对 1 页文件自然退化为单页。
- **与购物栏/合并导出的兼容**：产物是普通素材，可正常收集与合并，无需改动。

## 子任务分工

| 子任务 | 范围 |
|---|---|
| `10-31-b-pr1` | 后端：落点三态、可索引命名、删源、标注迁移、回滚、错误码、单测 |
| `10-31-b-pr2` | 前端：三选项弹窗与强确认、批量同口径、结果明细、wailsjs 同步、单测 |
| `10-31-b-pr3` | 文档与规范：`pptx-split.md`、`assets-management.md`、功能说明/README、CLAUDE.md 规则行 |

依赖顺序：PR1 → PR2 → PR3。

## Out of Scope

- 前两种落点产物的索引化（保持排除）。
- 产物之间的页序重排可视化（依赖文件名，不做 UI 拖拽重命名）。
- 拆分后自动重建合并（撤销入口）；用户可用现有合并导出重建单一文件。
- 跨库移动、产物去重。

## Risks

- 删源不可逆：误判「全部成功」会丢原文件。缓解 = 逐页产物校验 + 失败零删除 + 强确认文案。
- 命名形态与排除规则隐式耦合：后续若调整 `isPptxSplitDerivative` 会误伤 replace 产物。
- 索引规模膨胀：大规模替换后素材库条目数等于总页数，需评估分页与统计性能。

## Technical Notes

- 关键文件：`service/assets_split.go`、`service/pptx_split.go`、`service/assets_scan.go`（`isPptxSplitDerivative`）、`service/assets_db.go`（标注写入）、`app_assets.go`、`frontend/src/components/AssetsPanel.vue`、`frontend/src/components/AssetSplitDialog*`（若存在）、`model/assets.go`。
- 相关规范：`docs/spec/pptx-split.md`、`docs/spec/assets-management.md`、`docs/spec/cross-layer-contracts.md`、`docs/spec/test-coverage-gate.md`。
