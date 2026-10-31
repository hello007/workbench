# 素材拆分 B 方案审核缺陷修复

父任务：`10-31-b`（B 方案 PR1/PR2/PR3 已完成并归档）
来源：三份独立子 agent 审核报告（后端 5708beb / 前端 037a154 / 文档 d89d0dd），关键项已人工复核。

## Goal

修复审核发现的 1 项数据安全问题、后端与前端中风险缺陷，并校正 PR3 文档与实现不一致处，使「就地替换」落点的删源承诺、索引一致性与文档口径全部成立。

## Requirements

### R1 回收站安全（P0，`util/recycle_windows.go`）

- 现状：`FOF_ALLOWUNDO | FOF_NOCONFIRMATION | FOF_SILENT`，回收站关闭、文件超配额或网络盘时 Shell 静默永久删除且返回 0，违背「可恢复删除」承诺。
- 改法：标志改为 `FOF_ALLOWUNDO | FOF_WANTNUKEWARNING | FOF_NOERRORUI`，去掉 `FOF_NOCONFIRMATION` 与 `FOF_SILENT`。回收站不可用时 Shell 弹永久删除警告；用户取消 → `fAnyOperationsAborted != 0` → 返回错误、源保留。
- 抽 `recycleFlags()` 纯函数并单测断言标志组合（不含 `fofNoConfirmation`、含 `fofWantNukeWarning`）。
- Windows 实机验证：回收站关闭、超配额两场景不得静默删源。

### R2 源文件 TOCTOU 防护（`service/assets_split.go`）

- 核心拆分前记录源 `Stat`（size + mtime）；移入回收站前复核。不一致 → 回滚产物与索引行，返回失败「源文件在拆分期间被修改，请重试」。
- 复核点位于删源前取消检查点之后、`MoveToRecycleBin` 之前。

### R3 标注迁移与删源行同事务收口（`service/assets_db.go`）

- 现状：迁移事务与 `deleteFileRecord` 之间隔删源，窗口内 `BatchSetRating` / `BatchUpdateFileTags` 写入随源行删除丢失。
- 改法：新增 `finalizeReplaceSource(srcFileID, targets)` 单事务——重读源标注并幂等迁移到产物，随后显式清 FTS/thumb_cache 并删源 `files` 行，同一事务提交。
- 拆分链路：回收站移入成功后调用该单点替代「先 migrate、后 delete」两段；原 `migrateSplitAnnotations` 保留供失败前预迁移或改为内部复用（禁两套 SQL 口径）。

### R4 v8 迁移幂等与原子（`service/assets_schema.go` / `assets_db.go`）

- v8 两条 `ALTER TABLE ADD COLUMN` 改为 Go 层 `ensureSplitRecordColumn`（`pragma_table_info` 判存在），并在单一事务内执行 + 同事务写 `user_version`。
- 崩溃/半迁移后重跑不再报 `duplicate column name`。

### R5 源文件从回收站还原后的重拆语义（`service/assets_split.go`）

- replace 增量四验增加 `!record.sourceRemoved` 条件：记录标记已删源但源文件重新出现时不再 `skipped`，走完整重拆（清旧产物、重建索引、再次移入回收站）。
- 保留「源不存在且入口前置校验拒绝」的现状。

### R6 replace 落点排除规则校验（`service/assets_split.go`）

- 建索引前用库排除 matcher 判定产物相对路径；命中排除规则 → 回滚产物并返回明确错误，禁写入随即被扫描转 excluded 的索引行。
- matcher 复用扫描链路单点，禁新写一套 glob 判定。

### R7 前端刷新与文案（`AssetsPanel.vue`）

- replace 刷新链移入 `finally`：条件 `seq === splitSeq && target === 'replace'`，覆盖删源失败、批量取消/系统性错误等 catch 路径（`refreshDataAfterMutation` 内部吞错，不双弹）。
- 批量弹窗目录名回显改「各源文件同名目录」，禁用字面量「源文件同名」冒充目录名。
- replace 警示区补提示：产物勿重命名为 `<源主干>-001.pptx` 形态（单文件用实际主干，批量用通用描述）。
- 单文件 replace 目录回显优先取 `entry.path` 的 basename 去扩展名，避免依赖 `title`。

### R8 测试

- 后端：回收站标志函数；TOCTOU 复核失败回滚；`finalizeReplaceSource` 原子性与幂等；v8 半迁移重跑；源还原后重拆；排除命中拒绝。
- 前端：replace 失败/取消仍刷新；批量文案；离开 replace 清勾选；E2E replace 用例改用 `target:'replace', sourceRemoved:true` 形状 mock，断言「已删除源」汇总。
- 覆盖率门禁不变。

### R9 旧产物清理边界（`service/assets_split.go`）

- `removeReplaceArtifacts` / `rollbackReplaceArtifacts` 仅删除本次或历史产物，不误删用户自建文件：
  - 重拆清理：只删已在 `files` 索引中的路径（成功拆分的旧产物必已索引）。
  - 回滚：核心调用前快照目录内匹配产物的文件名集合，回滚只删快照之后新增的文件。
- 崩溃残留的未索引产物保留原样（下次扫描收录或人工清理），不做模式盲删。

### R10 文档校正

- `README.md` / `docs/功能说明.md`：把「原始文件只读不修改」限定为扫描/检索/缩略图链路，注明就地替换是显式删源例外。
- `docs/spec/pptx-split.md`：
  - 删除「前端据 sourceRemoved 禁用重拆」表述（前端未实现，且 R5 允许还原后重拆）。
  - 9.3 表拆分「删源前取消检查点 → 回滚」与「回收站移入失败 → 不回滚」两行。
  - 批量失败语义限定：业务失败逐项入 Items；DB/标注事务错误与取消整体中止。
  - 「界面提示不要改名」改为已实现后的准确文案。
- `docs/spec/assets-management.md` / `perf-baseline.md`：
  - 删除 `processScanFile` 写「缩略图记账」（实际不写，缩略图后续补缺/按需生成）。
  - `ListAssetFiles` 改为全量清单口径；分页指向 `ListAssetSlides`。
  - `deleteFileRecord` 改为「显式清 FTS/thumb_cache + 外键级联清 slides/file_tags/slide_tags/slide_meta」。
- `docs/spec/logging-and-errors.md`：三分类判定去掉 `Code`；`model/assets.go` 注释同步。
- 路径表述补 `-p` 例外（`replaceArtifactBase`），覆盖 pptx-split 表格、assets-management、功能说明。
- `docs/API参考.md`：同步删除「前端禁用重拆」与 `target` 决定重拆禁用的描述。
- `CLAUDE.md` 规则行如受影响同步（保持 ≤250 字符）。

## Out of Scope

- 批量取消时保留已完成 Items（后端 abort 语义不变，仅前端补刷新）。
- 跨库移动、产物去重、页序可视化重排。
- 回收站实现的非 Windows 平台行为变更（Linux Trash 已满足）。

## Acceptance

- [x] `go test ./...` 全绿，覆盖率门禁通过（model 98.2% / server 90.5% / service 80.8% / util 62.2%）。
- [x] 前端 `npm test` 全绿（2266 用例），覆盖率 ≥70%（实测语句 85.94%）；E2E replace 用例覆盖「已删除源」分支。
- [ ] 回收站不可用时源文件不被永久删除（Windows 实机两场景）——标志已改并由单测守护，实机回收站关闭/超配额场景待人工验证。
- [x] 源文件拆分期间被修改 → 源保留、产物回滚。
- [x] v8 半迁移库可重开。
- [x] 文档与代码逐条一致，无「只读不修改」「前端禁用重拆」「ListAssetFiles 分页」等过期表述。

## Risks

- `FOF_WANTNUKEWARNING` 会弹系统对话框：后台拆分中弹窗体验需真机确认；若不可接受，改为删源前主动探测回收站可用性（需额外 Shell API 调研）。
- 同事务重迁移增加一次标注读，开销可忽略（单文件级）。
- R9 快照法依赖目录可读；不可读时回滚退化为仅删已知列表。
