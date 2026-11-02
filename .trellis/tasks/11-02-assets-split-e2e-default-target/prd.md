# 修复 assets-split E2E 默认落点断言过期（P1，仅测试代码）

## Goal

`frontend/e2e/assets-split.spec.js` 两例仍按「素材库缓存目录」为默认落点断言，与 `798fc2f` 起的 `replace` 默认不一致，导致 E2E 稳定 2 失败。修复断言与注释，使 E2E 全绿。

## 背景

* 前端默认落点已改为 `replace`：`frontend/src/components/AssetsPanel.vue:1526`（`ref('replace')`）与 `:1543`（重开复位）。
* 单元测试与规范已同步：`AssetsPanel.spec.js:3164/3182`、`docs/spec/pptx-split.md:98/103`。
* 仅 E2E spec 未同步，属测试债务，产品行为无缺陷。

## Requirements

1. 修改 `frontend/e2e/assets-split.spec.js:42` 用例：
   * 默认断言改为「就地替换（删除源文件，推荐）」，`.split-replace-warning` 可见、`.split-cache-hint` 计数 0。
   * 显式点选「素材库缓存目录」后提交，保留原有 `target='cache'` 调用参数断言（维持缓存落点主链覆盖）。
   * 同步修正用例内「默认缓存落点」类注释。
2. 修改 `frontend/e2e/assets-split.spec.js:145` 用例：
   * 首次打开默认断言改为「就地替换」、`.split-replace-warning` 可见。
   * 「取消后重开」断言改为「回到就地替换默认、强确认未勾选、提交禁用」，删除对 `.split-replace-warning` 计数 0 的断言。
   * 同步修正用例名与注释中「恢复缓存目录默认落点」措辞。
3. 不改动产品代码、单元测试与后端。

## Acceptance Criteria

* [ ] `cd frontend && npx playwright test e2e/assets-split.spec.js` 全部通过。
* [ ] `cd frontend && npm run e2e` 全部通过（124 例）。
* [ ] `cd frontend && npm test` 仍通过（2275 例）。
* [ ] `git diff` 仅涉及 `frontend/e2e/assets-split.spec.js`。

## Out of Scope

* 不修改 `AssetsPanel.vue`、单元测试、后端与规范文档（现有口径已正确）。
* 不处理 `vitest.config.js` 的 CJS/ESM 加载告警。
