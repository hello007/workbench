# replace 产物容器移动收口

## Goal

收口审核发现的「中」级缺口：replace 产物容器目录被重命名/移动后，`split_records.output_dir` 不随之迁移，导致批量拆分再次把单页产物当源（「页的页」）；禁止单页产物被单独移动；同步清扫默认落点与目录守卫的陈旧文档/注释，补齐审核指出的测试缺口。

## Requirements

1. **产物容器目录迁移**：目录重命名/移动（RenameAssetFolder / MoveAssetFolder）与 `ReplaceAssetPathPrefix` 在同一事务内，除现有 `source_rel_path` 前缀迁移外，额外迁移 `output_dir` 位于被移动目录下的记录：
   - 仅 `target=source` / `target=replace`；
   - `output_dir` 转相对库根路径后按目录边界剥离旧前缀，改写为 `Join(库根, 新相对路径 + 后缀)`；
   - `target=cache` 的 derived 路径在库根外，天然不命中，保持原值；
   - 字符串比较复用 `stripDirPathPrefix`（EqualFold），不做盲目字符串替换。
2. **同键记录合并**：源键与 `output_dir` 同时命中时只落一次 UPDATE；源键迁移仍按旧路径长度降序处理嵌套链式腾位。
3. **单页产物禁止单独移动**：`MoveAssetFiles` 对 replace 产物（`isReplaceArtifactOutput` 命中）拒绝，提示整体移动所在产物容器目录；批量内计入 failed 明细，不中断其余文件。
4. **幂等与冲突**：重复迁移零变更；源键目标已存在时沿用现有冲突策略（保留 `source_removed=1`）。
5. **文档清扫**：
   - 前端默认落点 = `replace`；后端空串归一仍为 `cache`，两处需分别写明；
   - `app_assets.go` 目录操作注释删除「拆分记录拒绝」旧口径；
   - `docs/功能说明.md` 目录移动补「拆分记录同事务迁移」，replace 容器可整体移动、单页产物不可单独移动；
   - `docs/spec/pptx-split.md`、`docs/spec/assets-management.md`、`docs/API参考.md`、`model/assets.go`、`service/assets_split.go`、`AssetsPanel.vue` 注释同步。
6. **测试补齐**：容器重命名/移动迁移、单页产物移动拒绝、回滚断言 `folder_tags`、目录边界负例（`a` 不迁 `ab/`）、LIKE 元字符目录名（`50%off`）、嵌套前缀端到端、`output_dir` 只迁移记录的幂等。

## Acceptance Criteria

- [ ] replace 容器目录重命名/移动后，`split_records.output_dir` 指向新位置；`filterReplaceArtifactSources` 剔除其中产物；对产物单文件 replace 拆分返回「不能再次拆分」。
- [ ] 移动含容器目录的上级目录（多级嵌套）时，容器记录的 `output_dir` 一并迁移。
- [ ] `ReplaceAssetPathPrefix` 目录前缀替换同样迁移 `output_dir`。
- [ ] `MoveAssetFiles` 移动 replace 单页产物返回明确失败，文件与记录均不动。
- [ ] cache 记录 `output_dir` 迁移前后保持原值。
- [ ] 重复迁移零变更；目录边界不误伤同头缀目录；LIKE 元字符目录名正确迁移。
- [ ] 回滚用例断言 `files.path` / `folder_meta` / `folder_tags` / `split_records` 均未变。
- [ ] 文档/注释无「默认 cache」「拒绝目录操作」残留。
- [ ] `go test ./...` 全通过；覆盖率门禁不降；前端测试通过。

## Technical Approach

### 存储层

- 扩展 `migrateSplitRecordsTx`：一次读取全部 `split_records`（表体量小），逐条计算：
  - `newRel`：`source_rel_path` 命中旧前缀时改写（沿用现状与长度降序）；
  - `newOutDir`：`target=source/replace` 且 `output_dir` 在库根内、相对路径命中旧前缀时改写；源键也命中时优先按「库根 + 新源目录 + 新源主干」重算（现有契约），避免两套公式冲突；
  - 两者均未变则跳过；仅 `output_dir` 变时直接 UPDATE（无唯一键冲突面）；源键变时沿用现有冲突四态处理。
- `output_dir` 相对化：`filepath.Rel(libraryRoot, outDir)`，`..` 前缀或错误视为库外跳过；随后 `stripDirPathPrefix(rel, oldPrefix)`。
- 不新增路径比较口径。

### 服务层

- `MoveAssetFiles`：批量入口一次性取 `listReplaceSplitOutputs`，传入 `moveOneAssetFile`；磁盘 rename 前判定 `isReplaceArtifactOutput(oldAbs, outputs)`，命中即 `fail("该文件是就地替换生成的单页产物，请整体移动其所在产物目录")`。
- 目录重定位无需额外改动（迁移在 `replaceFilePaths` 内完成）。

### 前端/文档

- 仅注释与文案同步，不改交互；强确认门槛不变。

## Out of Scope

- `RelocateAssetFile` 重链与拆分记录联动（残余风险，另行登记）。
- 单页产物跨容器移动后的记录拆分（已选择拒绝移动）。
- Linux 大小写同名目录被一并命中的既有行为。
- `split_records` 表结构变更。

## Technical Notes

- `service/assets_db.go` `migrateSplitRecordsTx` / `replaceFilePaths`
- `service/assets_folders.go` `moveAssetFolderCore` / `MoveAssetFiles` / `moveOneAssetFile`
- `service/assets_split.go` `filterReplaceArtifactSources` / `isReplaceArtifactOutput` / `splitOneFileReplace` / `samePathFold`
- 审核报告：`798fc2f` 提交审核（中 1 项、低 4 项）
- 文档：`docs/spec/pptx-split.md`、`docs/spec/assets-management.md`、`docs/API参考.md`、`docs/功能说明.md`、`CLAUDE.md` 相关规则行
