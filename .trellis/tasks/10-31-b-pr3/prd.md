# B 方案 PR3：文档与规范沉淀

父任务：`10-31-b`（方案与决策见父任务 prd.md）

## 范围

1. `docs/spec/pptx-split.md`
   - 三落点对照表（路径布局 / 是否进索引 / 是否删源）。
   - replace 落点的**命名契约**：`<3位页序>-<源名主干>.pptx`，以及「该形态刻意不命中 `isPptxSplitDerivative`」这一隐式耦合警示（后续改判定函数会误伤）。
   - 删源时序、失败/取消回滚、`split_records` 的 `source-removed` 标记。
2. `docs/spec/assets-management.md`
   - 标注重映射规则（文件级复制到每页产物、页级 `(file_id,k)` → `(newID,1)`）。
   - 源记录直接删除（不走 lost）的决策与理由。
   - replace 后素材库条目规模变化（条目数 = 页数）与性能提示。
3. `docs/功能说明.md` §14.6 PPT 拆分：三落点用户可见说明与不可逆提示。
4. `README.md` 素材管理段落：补一句 replace 落点能力。
5. `CLAUDE.md` 关键规则表：新增一行（≤250 字符），钩子级要点 = 命名契约 + 删源前置条件 + 标注迁移 + 回滚，细节链接到 `pptx-split.md`。
6. `docs/API参考.md`：`SplitAssetFile` / `SplitAssetLibrary` 签名与说明更新。

## 验收

- [ ] 五处文档与实现一致，无过期表述（如「三落点」仍写「二选一」）。
- [ ] CLAUDE.md 新增行 ≤250 字符（钩子密集行 ≤350）。
- [ ] 若实现阶段产生新的跨层契约（如新错误码），按三处同步规则补齐。
