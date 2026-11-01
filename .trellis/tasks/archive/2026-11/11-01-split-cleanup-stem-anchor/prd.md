# 重拆清理产物组基名锚定（审核收口）

## Goal

`cleanupSplitArtifactsInDirByPage`（commit `739d85c`）改为「主干分组 + 页覆盖证据」后，两路代码审核确认仍有两处误删缺口：①`pageCount==1` 豁免使单页记录的任意唯一同形候选即判产物组（单页源 + 产物缺失 + 用户 `笔记-001.pptx` 仍被删）；②仅按覆盖页数选组，用户在同一目录放多份同主干文件可覆盖反超产物组（记录 3 页、产物剩 1 页、用户 2 份 → 用户组被删）。本任务用「产物目录基名锚定」根除此两处，并收口审核提出的日志/过滤/测试缺口。

## What I already know

* 产物目录布局单点 `splitSourceOutputDir` = `filepath.Join(Dir(src), splitSourceBaseName(rel))`，即 **`filepath.Base(record.outputDir)` == 拆分时源的命名主干**，source/replace 同布局。
* 产物命名主干同样是拆分时源主干 → 正常目录的产物组主干与 `filepath.Base(outputDir)` 一致（改名重链后记录键换了新名，但目录与产物主干都还是旧名）。
* 目录级迁移 `migrateSplitRecordsTx` 重算 `output_dir` 到新落点、文件级搬迁 `resolveRelocatedSplitOutDir` 改指新目录时，基名与旧产物主干可能不一致 → 走「零删除 + Warn」安全侧。
* 现有实现见 `service/assets_split.go`：`collectSplitArtifactsByPage` / `dominantSplitArtifactStem` / `parseSplitArtifactName`。
* 审核结论：索引行断言非假绿（`deleteIndexRowsForPaths` 只查 `files`）；缺口是缩略图/幻灯片级联未断言、`replace` 用户文件用例用错文件名、2 页缺首页新行为无用例锁定、「清理在删源前」无时序用例。

## Requirements

1. 产物组选择改为基名锚定：候选组主干须与 `filepath.Base(dir)` **EqualFold** 相等才可入选；命中组直接选中，不再与覆盖页数最多的组竞争；无匹配组 → 零删除 + Warn（不可判定）。
2. `pageCount==1` 仍要求基名匹配（不再有「任意单文件即产物」豁免）；单页正常目录（`旧名/旧名-001.pptx`）仍清理。
3. `collectSplitArtifactsByPage` 的 `os.ReadDir` 错误：`os.IsNotExist` 静默；其余（权限拒绝等）`Logger().Warn` 后返回。
4. 候选过滤改为 `e.Type().IsRegular()`（排除符号链接/junction/子目录）。
5. 目录删除失败口径统一：`ENOENT`/`ENOTEMPTY` 静默，其余 Warn（主分支与空目录收敛分支一致）。
6. 注释修正：`assets_split.go:471`/`:759` 调用点断行错位；`cleanupSplitArtifactsInDirByPage` 头注释补基名锚定规则。
7. 测试补齐：
   - 单页记录 + 用户 `笔记-001.pptx`（目录基名不同）→ 文件与索引行保留。
   - 单页记录 + 正常产物 `旧名-001.pptx`（目录基名 `旧名`）→ 删除。
   - 用户同主干多文件反超（记录 3 页，产物剩 `旧名-001`，用户 `笔记-002/003`）→ 只删 `旧名-001`，用户文件保留。
   - `replace` 用户文件改为 `001-笔记.pptx`（真正同形态）并断言保留。
   - 覆盖不足但基名匹配（2 页记录、目录 `旧名` 内只剩 `旧名-002.pptx`）→ 该文件删除（基名证据充分）；与「无基名匹配」的保留用例区分。
   - 目录内同名 `.pptx` 子目录 → 不动（`IsRegular` 固化）。
   - 缩略图与幻灯片级联：复用同包既有 `insertThumbLedgerRow`/`countThumbCacheRows`（若存在）断言被删页缩略图清理；不存在则补最小断言或记录缺口。
   - 集成时序：`replace` 拆分成功但移入回收站失败 → 旧目录已清理、新产物保留、源保留（锁定「清理在删源前」）。
   - 2 页源缺首页（旧产物仅剩页 2，目录基名与产物主干一致）→ 页 2 与其索引行被清理（基名匹配即产物组，覆盖不足不豁免）；保留原 3 页用例的清理断言。

## Acceptance Criteria

* [ ] 单页源 + 用户自建同页序文件的误删场景被用例锁定为「保留」。
* [ ] 用户同主干多文件反超场景被用例锁定为「用户文件保留、产物组照常清理」。
* [ ] 正常目录（source 与 replace）旧产物仍被清理，索引行同步；`pageCount==1` 正常目录亦清理。
* [ ] 基名不匹配（目录被改名）→ 零删除 + Warn。
* [ ] `go test ./...` 通过；覆盖率门禁 service ≥76% 不降。
* [ ] `docs/spec/pptx-split.md §9.5` 与 `CLAUDE.md` 关键规则行（钩子密集档 ≤350 字符）同步且与实现逐条一致。

## Definition of Done

* 上述测试全部落地并通过，含回退验证（旧实现下至少新增误删用例变红）。
* spec / CLAUDE.md 无漂移；CLAUDE 行只留不变式钩子，机理细节在 spec。
* 无新增静默失败路径（权限错误必须 Warn）。

## Technical Approach

`dominantSplitArtifactStem(byStem, pageCount, dir)` 增加 `dir` 参数：

1. `anchor := filepath.Base(dir)`。
2. 遍历组，仅 `strings.EqualFold(stem, anchor)` 的组为候选（每组仍至少 1 页；若一组内覆盖页超过 pageCount 不裁剪，保持去重页序计数）。
3. 命中 → 返回该主干；未命中 → `("", false)` 由调用方 Warn「目录基名与产物主干不匹配，跳过清理」。
4. 删除循环不变（仍要求同页序唯一且主干 == 产物组主干）。

取舍：目录被用户改名后旧产物不再自动清理（安全侧，人工可处理）；换取「用户文件误删」路径消除。

## Decision (ADR-lite)

**Context**：记录未持久化产物主干（不改表结构），只能由目录证据反推；覆盖数法存在用户文件反超路径，单页豁免无任何判别力。
**Decision**：以 `filepath.Base(outputDir)` 作为旧产物主干锚点（布局单点保证，改名重链后基名仍是旧主干），失配即不可判定。
**Consequences**：两类误删路径消除；目录改名/搬迁后失配零删除，残留需人工清理；实现不新增表列。残余风险仅「用户文件主干 == 产物目录名」，实际不可达，spec 记录。

## Out of Scope

* 不新增 `split_records` 产物主干/哈希列。
* 不改 `findSplitArtifactByPage` 的单页定位语义。
* 不改前端。

## Technical Notes

* 代码：`service/assets_split.go`（`collectSplitArtifactsByPage` / `dominantSplitArtifactStem` / `cleanupSplitArtifactsInDirByPage`）、`service/assets_split_cleanup_renamed_test.go`。
* spec：`docs/spec/pptx-split.md §9.5`、`CLAUDE.md` 第 123 行。
* 审核结论来源：commit `739d85c` 两路子代理审核（逻辑 + 测试）。
