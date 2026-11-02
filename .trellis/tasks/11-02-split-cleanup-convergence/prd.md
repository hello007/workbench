# 延迟清理收敛与索引行同步（P2）

## Goal

`2446ca7` 的 `removeSplitArtifactsBeyond(dir, baseName, keepPage)` 存在三处缺口（两路审核实测）：

1. 旧 `%02d` 形态页序在 `1..keepPage` 内（如 `演变-01.pptx` 与新 `演变-001.pptx` 并存）不删除，与注释「旧 `%02d` 产物与越界页一并收敛」不符；相关测试只放 42/99 形成假阳性覆盖。
2. 超界页删除后不调用 `deleteIndexRowsForPaths`，源落点留下 lost 僵尸行（既有缺口，新函数正该修）。
3. 前缀比较大小写敏感：Windows 仅改大小写（`abc.pptx`→`Abc.pptx`）后 `abc-003.pptx` 不识别，超界页残留。

## Requirements

1. `removeSplitArtifactsBeyond` 签名扩展为 `(store *assetsStore, lib model.AssetLibrary, dir, baseName string, keepPage int, splitAt string)`：
   - 匹配 `<baseName>-<纯数字>.pptx`（Windows 下 base 比较折叠，经 `sameNameFold`；非 Windows 精确）；
   - 删除条件：页序 `> keepPage`，或页序 `<= keepPage` 但文件名不等于当前形态 `fmt.Sprintf("%s-%03d.pptx", baseName, seq)`（历史 `%02d`）；
   - 删除前经 `isSplitArtifactAfterSplitAt` mtime 守卫（晚于 `split_at+2s` 视为用户文件保留）；
   - 成功删除的路径批量调 `deleteIndexRowsForPaths`（库外自动静默）。
2. 调用点：核心成功后调用，source/cache 同口径；失败仅 Warn，不阻断记录写入。
3. 注释与 spec 口径统一：删干净「与注释不符」的表述；测试 fixture 必须包含页序 ≤ keepPage 的 `%02d` 用例。
4. 测试：
   - `演变-01.pptx`/`演变-02.pptx` + 新 3 位产物，keepPage=3 → 旧两位数被删、新产物保留；
   - 页序 > keepPage 删除 + files 行清理（源落点在库内）；
   - Windows 仅大小写改名后 `abc-003.pptx` 被识别删除（非 Windows skip）；
   - mtime 晚于 splitAt 的同类用户文件保留。
5. 文档：`docs/spec/pptx-split.md §9.3/§9.5/§9.7`。

## Acceptance Criteria

* [ ] 三条缺口均有用例锁定，回退到 `2446ca7` 实现时对应用例变红。
* [ ] 正常页（页序 ≤ keepPage 且为标准 `%03d` 形态）不被误删。
* [ ] `go test ./...` 与覆盖率门禁通过。

## Out of Scope

* cache 目录内非产物残留不清理（派生目录，保持不破坏性清空）。
