# 大小写折叠平台守卫统一（P1 数据丢失）

## Goal

`2446ca7` 为修复 Windows 仅大小写改名引入的 `EqualFold` 折叠未做平台守卫：

* `isReplaceArtifactName` 在 Linux（大小写敏感）把用户 `001-ABC.pptx` 当成 `001-abc.pptx` 的历史产物，收入 `snapshotReplaceArtifactNames`；随后备份让位、成功后 `removeReplaceArtifactBackups` 删除备份 → **用户文件内容丢失**。
* `foldNameInSet`、`resolveExistingDirCase` 同样无守卫，可能把用户文件/兄弟目录折叠命中。

`samePathFold` 已有 `runtime.GOOS == "windows"` 守卫，是正确口径。

## Requirements

1. 新增单点 `sameNameFold(a, b string) bool`：`a == b` 或（`runtime.GOOS == "windows"` 且 `strings.EqualFold`），与 `samePathFold` 同口径；文件内注释说明「Windows 文件系统大小写不敏感，Linux 折叠会误判不同文件」。
2. `isReplaceArtifactName`：精确匹配优先；仅 Windows 下追加一次折叠匹配（`strings.ToLower` 双方后复用精确逻辑），返回语义不变。
3. `foldNameInSet`：改为调用 `sameNameFold`；集合遍历保持确定性（命中即 true，无需取具体值）。若现有调用需要返回命中的 key，改为返回 bool + 调用方自行处理。
4. `resolveExistingDirCase`：Linux（非 Windows）直接返回原 `dir`，不读父目录。
5. 测试：
   - 非 Windows：`isReplaceArtifactName("001-ABC.pptx", "abc")` 为 false；`resolveExistingDirCase` 原样返回。
   - Windows（`runtime.GOOS` 分支，测试内 skip 非 Windows）：折叠命中 true；`foldNameInSet` 表驱动。
   - `sameNameFold` 直测三态（相等 / Windows 折叠 / 非 Windows 不等）。
6. 文档：`docs/spec/pptx-split.md §9.5` 明确「折叠仅在 Windows 语义下启用」；`CLAUDE.md` 相关行同步（若涉及）。

## Acceptance Criteria

* [ ] Linux 上大小写变体用户文件不再被识别为历史产物（用例在非 Windows 断言 false）。
* [ ] Windows 大小写改名重拆仍成功（既有用例保持绿）。
* [ ] 折叠逻辑全仓只经 `sameNameFold` / `samePathFold`，无裸 `EqualFold` 用于文件名/路径判定。
* [ ] `go test ./...` 与覆盖率门禁通过。

## Out of Scope

* 不改 `splitStemInCandidates` 的折叠（它用于产物主干归属，输入均为产物形态候选，且已在 Windows 语义下使用）——若确认存在同类风险，记录后另立任务。
