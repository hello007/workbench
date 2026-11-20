# 修复 FileHistoryDialog 覆盖率插桩下的用例超时

## Goal

`frontend/src/components/__tests__/FileHistoryDialog.spec.js:229`「initialSha 超定位上限未命中」用例在 `npm run test:coverage`（v8 插桩 + 全量并行）下偶发 5s 超时，裸跑 `npm test` 稳定通过。目标是消除覆盖率插桩下的偶发失败，**零放松断言强度**（`GetFileCommitHistory` 调用 25 次、`ElMessage.info` 精确文案断言原样保留）。

## Requirements

1. 为目标用例增加**用例级超时覆盖** `{ timeout: 20000 }`，沿用仓内先例 `BranchCompareDiffViewer.spec.js:75` 的写法。
2. 目标用例的 `vi.waitFor` 显式传 `{ timeout: 15000, interval: 50 }`，消除轮询默认 1s 超时在插桩/并行下的第二处隐患。
3. 断言内容零改动：调用次数 25、`ElMessage.info` 文案全等、`not.toContain(target)` 负断言全部保留。
4. 不修改 `frontend/vitest.config.js` 全局 `testTimeout`；不改动 `FileHistoryDialog.vue`；不改动其他 spec。
5. 等待方式维持 `vi.waitFor` 轮询，禁 `sleep` / `waitForTimeout` 式等待。
6. 沉淀规范：在 `docs/spec/test-stability.md` 增补「前端 v8 插桩放大单用例耗时」小节，并同步 `AGENTS.md` 关键规则行（若超出既有行长度上限则只做最小钩子）。

## Acceptance Criteria

* [ ] `npm run test:coverage` 连跑 3 次全绿（含 70% 四指标门禁）
* [ ] `npm test` 全绿
* [ ] diff 可证断言内容零放松（仅新增超时 / 轮询参数）
* [ ] 全量覆盖率跑中该用例耗时稳定低于 20000ms，且超出 5000ms 不再是失败原因
* [ ] `docs/spec/test-stability.md` 已补充该 flaky 类别与推荐写法

## Definition of Done

* 相关单测通过，覆盖率门禁不下降（前端四指标 ≥70%）
* 规范文档更新且索引/关键规则无悬空引用
* 不主动提交（提交与推送给由用户触发，走 fetch + rebase origin/master）

## Technical Approach

### 根因（实测）

该用例驱动组件扫描满 500 条上限（20 条/页 × 25 页），`locateCommit` 的 `while` 循环每轮 `await loadHistory(false)` 后 `commits.value.push(...page)`，jsdom 下累计渲染 6500 行；v8 插桩与全量并行进一步放大耗时。

| 运行形态 | 该用例耗时 |
|---|---|
| 单文件、无覆盖率 | 1356 ms |
| 单文件、v8 覆盖率 | 2699 ms |
| 全量并行、v8 覆盖率 | 4793 / 5494 / 6274 / 6453 ms（多次实测，均逼近或越过 5000ms 上限） |

同文件相邻用例「逐页扫描至命中」仅扫 2 页（覆盖率下 53 ms），不在风险面。

### 方案

超时是**环境放大**而非逻辑缺陷，正确做法是把边界显式写在该用例上，而非全局放宽或改写组件：

* `it('...', { timeout: 20000 }, async () => {...})`：以最坏实测 6.45s 为基线，留约 3× 余量。
* `vi.waitFor(fn, { timeout: 10000, interval: 50 })`：保证轮询上限高于扫描链最坏实测值约 2 倍，用例级超时为外层兜底，避免「用例超时未到、轮询先死」。
* 不改 `vitest.config.js` 全局 `testTimeout`：全局放宽会把其他用例的真实变慢一起掩盖，且本仓其余前端用例均在 100 ms 量级，无信号表明存在系统性放大。

## Decision (ADR-lite)

**Context**：覆盖率插桩后单一用例耗时从 1.36s 涨到 4.79s，贴近 vitest 默认 5s 上限，属环境敏感的 flaky，不是产品缺陷。

**Decision**：采用用例级双层超时（用例 `{ timeout: 20000 }` + `vi.waitFor({ timeout: 15000, interval: 50 })`），零改断言；并将该类别沉淀至 `docs/spec/test-stability.md`。

**Consequences**：
* 正面——只影响该用例，其他用例的超时守卫保持默认强度；断言强度不变，仍能拦住「无限扫描」回归。
* 负面——单次失败的最长暴露时间从 5s 变为 20s；由于扫描链本身受插桩放大，这是可接受成本。
* 后续——若前端再出现「插桩放大」类 flaky，按同一范式（用例级超时 + 显式轮询参数）处理，禁全局放宽。

## Out of Scope

* `FileHistoryDialog.vue` 业务逻辑改动与性能优化（如虚拟滚动、分页渲染）
* `frontend/vitest.config.js` 全局 `testTimeout` 调整
* 其他 spec、E2E、后端测试

## Technical Notes

* `frontend/src/components/FileHistoryDialog.vue:120-194`：`loadHistory`（逐页追加）与 `locateCommit`（500 条上限循环）
* `frontend/src/components/__tests__/FileHistoryDialog.spec.js:227-251`：目标用例
* `frontend/src/components/__tests__/BranchCompareDiffViewer.spec.js:75`：用例级超时先例
* `frontend/src/transport/__tests__/tokenGate.spec.js:5`：文件级 `vi.setConfig({ testTimeout })` 先例（本次不采用，影响面大于用例级）
* `frontend/vitest.config.js`：未设置 `testTimeout`，取 vitest 默认 5000ms；覆盖率门禁四指标 70%
* `docs/spec/test-stability.md`：现有 Flaky 规范覆盖 Go 侧 mtime 时序，缺前端插桩放大条目
