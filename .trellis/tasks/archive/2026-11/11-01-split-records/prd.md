# split_records 路径随目录迁移

## Goal

目录重命名、目录移动、路径前缀替换后，`split_records` 的 `source_rel_path` 与 `output_dir` 随同一事务迁移，使 `filterReplaceArtifactSources`、`listReplaceSplitOutputs` 与单文件「产物二次拆分拒绝」在新路径下立即生效，消除历史 replace 单页产物被再次批量拆分产生「页的页」嵌套素材的问题。

同时把拆分弹窗的默认落点由 `cache` 改为 `replace`（仍需勾选强确认后才可提交）。

## Background

- `split_records` 以 `source_rel_path` 为唯一键；`output_dir` 存绝对路径。
- 三个目录级入口目前只迁移 `files.path` 与 `folder_meta` / `folder_tags` 键：
  - `RenameAssetFolder` / `MoveAssetFolder` → `moveAssetFolderCore`；
  - `ReplaceAssetPathPrefix`。
- 目录重命名/移动对 `source_removed=0` 的拆分记录直接拒绝（`splitRecordExistsByPathPrefix`），但界面无删除拆分记录入口，形成死路；`source_removed=1` 的 replace 留痕放行却不迁移，正是本次 bug 来源。
- 既有同事务级联先例：`replaceFilePaths` 内 `folder_meta` / `folder_tags` 键改写，SQLite `substr` 按字符口径（`length(?)`）避免中文目录名字节偏移截断。

## Requirements

1. **三入口全覆盖**：`RenameAssetFolder`、`MoveAssetFolder`、`ReplaceAssetPathPrefix` 在更新目录索引的同一事务内迁移拆分记录。
2. **解除目录守卫**：移除 `moveAssetFolderCore` 对 `source_removed=0` 记录的拒绝；三类记录（cache / source / replace）全部随目录迁移，目录可移动范围不再被历史记录锁死。
3. **source_rel_path 迁移**：目录边界语义 = 恰等于旧前缀，或以「旧前缀 + `/`」开头；LIKE 转义复用 `escapeLikePattern`；前缀剥离大小写不敏感（复用 `strings.EqualFold`，不新增第二套路径比较）。
4. **output_dir 迁移**：
   - `target=source` / `target=replace`：产物位于库根内，按「库根 + 迁移后相对路径」重算为 `Join(库根, 新源目录, 新源名主干)`，禁止字符串盲替换；
   - `target=cache`：产物位于 `data/assets/<库>/derived`，物理目录不随用户目录迁移，`output_dir` 保持原绝对路径（仍可打开当前产物）；登记为限制。
5. **幂等**：重复迁移零变更（改写后前缀不再命中；大小写变体命中时按改写结果与原值相等跳过）。
6. **冲突策略**：迁移后目标键已存在记录时，保留 `source_removed=1` 的一条（两条同类时保留既有行），删除落败行并 `slog.Warn`；迁移门禁不因冲突中断。
7. **事务原子性**：拆分记录迁移与 `files.path`、`folder_meta`、`folder_tags` 同一事务；任一步失败整体回滚。
8. **不改变表结构**：`split_records` 列、`target` / `source_removed` 语义不变；schema 无迁移。
9. **前端默认落点**：拆分确认弹窗默认选中 `replace`（`splitTarget` 初始值与 `resetSplitDialog` 复位值同步），强确认勾选逻辑不变。

## Acceptance Criteria

- [ ] 目录重命名后，replace 记录 `source_rel_path` 与 `output_dir` 均指向新路径；`filterReplaceArtifactSources` 在新目录下把产物全部剔除；`listReplaceSplitOutputs` 返回新 `output_dir`；单文件对产物执行 replace 拆分返回「不能再次拆分」。
- [ ] 目录移动后，cache / source / replace 三类记录 `source_rel_path` 全部迁移；source/replace 的 `output_dir` 重算；cache 的 `output_dir` 保持原值。
- [ ] `ReplaceAssetPathPrefix` 在旧前缀与实际路径存在 ASCII 大小写差异时，记录正确迁移，不产生重复路径段。
- [ ] 中文目录名迁移无截断（记录路径完整）。
- [ ] 空目录（无 `files` 行）仍有拆分记录时迁移照常执行。
- [ ] 重复执行迁移零变更；冲突场景保留 `source_removed=1` 记录且不中断。
- [ ] 迁移中途失败（测试注入 SQLite 触发器）时 `split_records`、`files.path`、`folder_meta` 全部回滚。
- [ ] 前端拆分弹窗打开即选中 `replace`，未勾选强确认时提交按钮禁用。
- [ ] `go test ./...` 全通过；覆盖率门禁不降。

## Technical Approach

### 存储层（service/assets_db.go）

- 新增迁移载荷：

```go
type splitRecordMigration struct {
    libraryRoot string // 库根绝对路径（output_dir 重算基准）
    oldPrefix   string // 旧目录相对路径（斜杠）
    newPrefix   string // 新目录相对路径
}
```

- `replaceFilePaths` 增加 `splitMig splitRecordMigration` 参数（`oldPrefix == ""` 跳过，防御预留）。事务内顺序：`files.path` 批量更新 → `folder_meta` / `folder_tags` 键级联 → 拆分记录迁移。
- `migrateSplitRecordsTx`：
  1. 事务内查询命中记录（`source_rel_path = ? OR source_rel_path LIKE ? ESCAPE '\'`），结果集收齐并 `Close` 后再发写语句（单连接池自锁纪律）；
  2. Go 侧剥离前缀（大小写不敏感）得到新相对路径，`source/replace` 重算 `output_dir`；
  3. 目标键已存在则按冲突策略取舍（保留 `source_removed=1`，同类保留既有行），落败行删除，`Logger().Warn`；
  4. `UPDATE split_records SET source_rel_path = ?, output_dir = ? WHERE source_rel_path = ?`。
- 新增前缀剥离单点 `stripDirPathPrefix(rel, prefix) (suffix string, ok bool)`：
  - 恰等（`EqualFold`）返回空后缀；
  - `len(rel) > len(prefix)` 且 `rel[len(prefix)] == '/'` 且 `EqualFold(rel[:len(prefix)], prefix)` 返回后缀；
  - `assetPathRenamesForPrefix` 同步改用该单点（现有 `strings.TrimPrefix` 在 ASCII 大小写差异下会把整条旧路径当作后缀，导致新路径重复拼接）。
- `output_dir` 重算：`filepath.Join(root, filepath.FromSlash(path.Dir(newRel)), splitSourceBaseName(newRel))`；目录为根时 `path.Dir` 返回 `.`，走 `Join` 归一。
- 不新增路径比较口径：`output_dir` 判定和落点构造均基于库根，cache / source / replace 由 `target` 显式分流。

### 服务层

- `moveAssetFolderCore`：删除 `splitRecordExistsByPathPrefix` 拒绝段；调用 `replaceFilePaths(..., splitRecordMigration{lib.RootPath, rel, newRel})`。
- `ReplaceAssetPathPrefix`：调用 `replaceFilePaths(..., splitRecordMigration{lib.RootPath, oldNorm, newNorm})`。
- 同步更新 `RenameAssetFolder` / `MoveAssetFolder` / `moveAssetFolderCore` / `ReplaceAssetPathPrefix` 函数头注释，删除「宁严勿错 / 无重链机制」旧口径。
- `MoveAssetFiles` 单文件移动保持现状（`moveFilePath` 已改 `source_rel_path`；产物目录物理不移动，`output_dir` 不改）。

### 前端

- `frontend/src/components/AssetsPanel.vue`：`splitTarget` 初始值 `'cache'` → `'replace'`；`resetSplitDialog` 复位值同步；弹窗注释与落点说明同步（默认推荐改为就地替换，强确认门槛保留）。
- 同步更新前端单测中对默认落点的断言（`SplitAssetLibrary` / `SplitAssetFile` 默认参数）。

## Decision (ADR-lite)

**Context**：目录迁移后拆分记录路径失配；现有守卫对 `source_removed=0` 记录形成无删除入口的死路；cache 产物物理目录不随用户目录移动。
**Decision**：解除守卫并做全量记录迁移；source/replace 的 `output_dir` 按库根重算，cache 保持原绝对路径；同事务迁移，冲突时保留 replace 留痕。
**Consequences**：目录操作不再被历史记录锁死；cache 记录与新源路径不再镜像，下次重拆会在新 derived 路径再生成一份（旧产物仍可打开，属登记限制）；表结构不变。

## Out of Scope

- 不迁移 `data/assets/<库>/derived` 物理目录，不做派生目录合并/回滚。
- 不新增「删除拆分记录」用户入口。
- 不改变 `MoveAssetFiles` 单文件移动的 `output_dir` 语义。
- 不调整 `split_records` 表结构或 `target` / `source_removed` 语义。
- 不重构拆分落点解析、FTS、缩略图链路。

## Test Plan

- `service/assets_split_migrate_test.go`（新增）：
  - 目录重命名 + replace 记录：路径/落点迁移，`filterReplaceArtifactSources` 剔除产物，单文件拒拆；
  - 目录移动 + 三类记录：cache 落点保持、source/replace 重算；
  - 中文目录名迁移；
  - 空目录（仅记录、无 files 行）；
  - 幂等（二次迁移零变更）；
  - 冲突（既有 `source_removed=1` 保留，落败行删除）；
  - 大小写差异前缀替换；
  - 触发器注入失败 → 整体回滚。
- 前端 `AssetsPanel.spec.js`：默认落点 `replace` 断言。
- 全量：`go test ./...`、`cd frontend && npm test`、覆盖率门禁。

## Technical Notes

- 入口：`service/assets_folders.go`（`RenameAssetFolder:309`、`MoveAssetFolder:343`、`moveAssetFolderCore:522`）；`service/assets_relink.go`（`ReplaceAssetPathPrefix:328`、`assetPathRenamesForPrefix:282`）；`service/assets_db.go`（`replaceFilePaths:618`、`escapeLikePattern:593`、`listReplaceSplitOutputs:2567`）。
- 判定消费：`service/assets_split.go`（`filterReplaceArtifactSources:1267`、`isReplaceArtifactOutput:1285`、单文件拒拆 `splitOneFileReplace:527`、`splitOutputDir:1313`、`samePathFold:1300`）。
- 默认落点：`frontend/src/components/AssetsPanel.vue:1524`、`resetSplitDialog:1539`。
- 既有先例：`replaceFilePaths` 的 `folder_meta` / `folder_tags` 键级联；`moveFilePath` 的 `split_records` 源路径改写。
- 文档同步：`docs/spec/assets-management.md` §8.7（删除「残余限制」，写入迁移契约）；`CLAUDE.md` 关键规则相关行按两档分流更新。
