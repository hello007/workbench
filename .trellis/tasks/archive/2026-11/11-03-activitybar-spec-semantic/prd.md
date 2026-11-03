# ActivityBar 组件测试语义化

## Goal

将 `ActivityBar.spec.js` 中依赖渲染下标（`findAll('.activity-bar-item')[n]`）的高亮与点击用例，改为基于既有 `data-panel-id` / `data-action` 语义属性定位，消除活动栏新增入口时的索引漂移维护成本，与 E2E 已落地的语义定位口径一致。

## Requirements

* 顶部 8 个高亮用例改为语义定位：按 `data-panel-id` 断言活跃项唯一且 ID 正确。
* 8 个点击用例改为 `[data-panel-id="<id>"]` / `[data-action="settings"]` 直接定位，不再经索引数组。
* 保留图标总数断言（10 = 8 面板 + 设置 + 终端），该断言表达"入口数量"语义而非位置。
* 删除用例内"第 N 个图标（index=N）"的下标注释；仅在语义属性总览用例保留渲染序说明。
* 生产组件不加新属性：`data-panel-id`（面板项）与 `data-action`（设置/终端/设备）已由 E2E 改造提供。

## Acceptance Criteria

* [ ] `frontend/src/components/__tests__/ActivityBar.spec.js` 前两组用例（高亮/点击）零 `items[n]` 下标断言。
* [ ] 高亮断言形如「活跃面板唯一且 `data-panel-id` 等于目标值」，不再逐个断言其他下标非活跃。
* [ ] `cd frontend && npx vitest run src/components/__tests__/ActivityBar.spec.js` 全绿。
* [ ] ActivityBar 组件源码无改动（仅测试文件变更）。

## Definition of Done

* 单测全绿，前端覆盖率门禁不回归。
* 不新增测试依赖、不改生产代码。
* 无需更新 README（纯测试可维护性改造）。

## Technical Approach

在 `describe('ActivityBar')` 内提供局部辅助：

```js
const panel = (wrapper, id) => wrapper.find(`[data-panel-id="${id}"]`)
const activePanelIds = (wrapper) =>
  wrapper.findAll('[data-panel-id].is-active').map((w) => w.attributes('data-panel-id'))
```

高亮用例统一断言 `expect(activePanelIds(wrapper)).toEqual([expectedId])`；点击用例统一 `await panel(wrapper, id).trigger('click')`。设置入口用 `wrapper.find('[data-action="settings"]')`。

## Decision (ADR-lite)

**Context**: E2E 已完成语义定位改造（2026-11-03，Session 209），组件单测仍按位置索引，活动栏新增入口即集体错位。

**Decision**: 复用现有 `data-*` 属性做单测语义定位，不引入 `data-testid`，不做组件改造。

**Consequences**: 测试与 DOM 语义绑定，新增面板无需改高亮/点击用例；若未来重命名 `data-panel-id` 需同步单测与 E2E（已有单一属性口径，成本可控）。

## Out of Scope

* 多设备指示、封禁、趋势等 `devices-*` 用例的定位方式（按行内容断言，无面板索引漂移问题）。
* 新增终端入口点击用例。
* 改动 `ActivityBar.vue` 或 E2E 用例。

## Technical Notes

* 组件：`frontend/src/components/ActivityBar.vue`（面板项 `:data-panel-id="item.id"`；设置 `data-action="settings"`；终端 `data-action="terminal"`；设备 `data-action="devices"`）。
* 测试：`frontend/src/components/__tests__/ActivityBar.spec.js` 第 114–271 行为本次改造面。
* 既有语义属性用例：同文件「每个入口带稳定语义属性」已断言 8 个 `data-panel-id` 与 2 个 `data-action`。
