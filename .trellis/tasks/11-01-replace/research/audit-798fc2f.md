# 提交 798fc2f 审核结论（本轮修复依据）

审核范围：`798fc2f fix(assets): 拆分记录随目录迁移，修复产物二次拆分`。

## 需修复

1. **中：产物容器/单页产物移动后 `output_dir` 失配**
   - `migrateSplitRecordsTx` 只按 `source_rel_path` 命中旧前缀，未处理 `output_dir` 在被移动前缀下的记录。
   - 场景：replace 容器 `方案A/城商行方案/` 被拖动/重命名后，记录仍指向旧目录；`filterReplaceArtifactSources` 漏剔 → 产物被再拆；`splitOneFileReplace` 的 `ownsOutDir` 判定同时失效。
   - 单页产物可被 `MoveAssetFiles` 单独拖出容器，记录无法表达产物新位置。
2. **低：默认落点文案残留 cache**
   - `docs/spec/pptx-split.md:98`、`docs/spec/assets-management.md:447`、`docs/API参考.md:515`、`frontend/src/components/AssetsPanel.vue:14,950-953`、`model/assets.go:393`、`service/assets_split.go:28,1311`、`service/assets_folders.go:648`、`service/assets_split_replace_review2_test.go:17`。
   - 需区分：前端弹窗默认 `replace`；后端 `normalizeSplitTarget("")` 仍归一 `cache`。
3. **低：Wails 绑定注释**
   - `app_assets.go:102,120-121` 仍写「拆分记录拒绝目录操作」，与已解除守卫相反。
4. **低：功能说明不一致**
   - `docs/功能说明.md:556` 目录移动漏「拆分记录」；`:554` 称 replace 容器显示「派生」并禁管理，实际不命中 `isPptxSplitDerivative`。
5. **低：测试缺口**
   - 回滚用例未断言 `folder_tags`；
   - 缺目录边界负例（`a` 不迁 `ab/`）、LIKE 元字符目录名（`50%off`）、嵌套前缀端到端、容器 `output_dir` 迁移与幂等。

## 已确认无问题

事务边界、单连接池 rows 纪律、嵌套前缀降序、冲突四态、大小写剥离、三入口源键迁移、cache 落点保持、幂等、前端强确认。

## 残余风险（Out of Scope）

`RelocateAssetFile` 重链不改拆分记录；Linux 大小写同名目录被 LIKE 一并命中；仅记录无 files 行时 `ReplaceAssetPathPrefix` 仍返回无命中。
