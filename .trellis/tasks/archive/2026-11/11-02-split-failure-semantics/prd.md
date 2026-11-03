# 拆分失败语义与清理可收敛性（P1）

## Goal

三处失败语义缺陷：

1. **source 落点核心失败前已删旧页**：`splitOneFileWithHash` 在调用拆分核心**之前**执行 `removeSplitArtifacts(outDir, baseName)`（cache 落点前置 `os.RemoveAll(outDir)`）。核心失败/取消时上一次拆分成果已被删，与「延迟清理保存旧成果」契约矛盾（replace 落点早已改为延迟清理）。
2. **Windows 仅大小写改名 replace 重拆必失败**：`isReplaceArtifactName` 用大小写敏感的 `HasPrefix`/`HasSuffix`，`snapshotReplaceArtifactNames` 不把 `001-Abc.pptx` 计入 `abc` 的历史产物；NTFS 上 `fileExists(001-abc.pptx)` 为真，判定「非历史产物」后回滚，重拆永久失败。
3. **删除失败后记录被覆盖、无法自动收敛**：`cleanupSplitArtifactsInDirByPage` 删除失败仅 Warn 并 `continue`，随后 upsert 覆盖 `artifact_stem`/`output_dir`，残留页不再被任何记录引用，注释与日志所称「下次重拆收敛」不成立。

## Requirements

1. **延迟清理**：source/cache 落点核心成功后，按本次 `result.PageCount` 删除目录内 `页序 > PageCount` 的旧产物（`removeSplitArtifactsBeyond(dir, baseName, pageCount)` 单点，形态 `<base>-<纯数字>.pptx`）；核心前不再删除任何旧页。核心失败/取消时旧页原样保留。cache 落点同样不再前置 `RemoveAll`（`重拆禁 RemoveAll 整目录` 既有约束）。
2. **大小写折叠**：`isReplaceArtifactName` 的前缀/后缀判定改为大小写不敏感（`strings.EqualFold` 语义，保留原始切片取页序）；`snapshotReplaceArtifactNames`、`firstReplaceArtifactName` 自动受益。补 Windows 用例：`abc.pptx` 上次拆分产物 `001-Abc.pptx`（或 `001-abc.pptx`），改名/重拆后不再因大小写误判失败。
3. **删除失败可观测且尝试收敛**：单页删除失败先重试一次；仍失败则 Warn 明确「残留需人工清理，本次重拆不再自动收敛」（不再声称下次收敛）。不改记录写入策略（保留本次新记录）。
4. 测试：
   - 核心失败/取消：source 旧产物完整保留（现状已有部分断言，补 cache 前置不再清空断言）。
   - 页数减少：成功删除超界旧页；页数增加保留全部。
   - 大小写仅差异的 replace 重拆（Windows 执行、非 Windows skip）。
   - 删除失败降级文案（可注入 chmod/只读文件，或仅注释级验证；若难构造则记录）。
5. 文档：`docs/spec/pptx-split.md §9.4/§9.5`、`CLAUDE.md` 相关行同步（延迟清理覆盖 source/cache；大小写折叠）。

## Acceptance Criteria

* [ ] 核心失败/取消后 source 旧产物与索引行完整保留（不再前置删除）。
* [ ] 页数减少重拆：超出本次页数的旧页删除，新页保留。
* [ ] Windows 大小写折叠后 replace 重拆成功且不误留旧页。
* [ ] 删除失败日志不再承诺自动收敛。
* [ ] `go test ./...` 通过，覆盖率不降。

## Technical Approach

* 新增 `removeSplitArtifactsBeyond(dir, baseName string, keepPage int)`：`removeSplitArtifacts` 改造为带 `keepPage` 版本，仅删 `seq > keepPage`；保留旧函数签名或统一调用。
* 在核心成功后（拿到 `result.PageCount`）调用；cache 落点同口径（不再 `RemoveAll`）。
* `isReplaceArtifactName` 用 `strings.ToLower` 比较，切片按原串长度取。
* 删除失败：`os.Remove` 失败后 `time.Sleep(50ms)` 重试一次（不引入新依赖）。

## Out of Scope

* 核心写盘同名覆盖导致的「失败时部分旧页已被覆盖」——彻底解决需核心写临时目录再原子移动，成本大，另立任务。
