# 修复目录选择双触发

## Goal

工作目录列表项同时绑定 `@mousedown` 与 `@click` 两个事件到同一 `handleSelect`，单击一次目录会向父组件 `Home.onDirectorySelect` 发出两次 `select` 事件，导致目录切换逻辑（保存/恢复树状态、清空预览、切换选中节点）重复执行，存在重复 IO 与 `nextTick` 竞态。本任务收敛为单次触发，并用单元测试锁死。

## What I already know

* 触发点：`frontend/src/components/DirectoryTree.vue:27-28`，`dir-item` 同时绑定 `@mousedown="handleSelect(dir.id)"` 与 `@click="handleSelect(dir.id)"`。
* `handleSelect` 仅 `emit('select', dirId)`，无去重；父级 `Home.vue:185 onDirectorySelect` 会保存当前树状态、清空预览、赋值 `selectedNode`、`await nextTick()` 后恢复新目录树状态。
* `@mousedown` 由提交 `cddcedd`（2026-05-20「修复工作目录节点点击事件」）新增，同时保留 `@click`，无提交说明细节。
* 同构组件 `AiChatPanel.vue:39` 的拖拽目录列表只用 `@click`，拖拽排序正常，可作对照。
* 现有测试 `DirectoryTree.spec.js`「点击目录应该 emit select」仅断言 `select` 事件存在与首个参数，未断言次数，故未捕获双触发。
* 拖拽排序依赖 `vue-draggable-plus`（~0.6.1，基于 SortableJS），SortableJS 在拖拽结束后会抑制紧随的 click。

## Assumptions (temporary)

* 拖拽排序场景下是否需要「拖拽即选中」属于产品偏好，需要用户确认。
* 选中同一目录重复点击无业务必要（刷新有其他入口）。

## Open Questions

* 无。已按推荐方案收敛（见 Decision）。

## Requirements

* 单击目录项只产生一次 `select` 事件：删除 `dir-item` 上的 `@mousedown`，仅保留 `@click`。
* 选择行为（切换工作目录、树状态恢复）不发生回归。
* 补充单测断言事件次数。

## Acceptance Criteria

* [x] 单击目录项 `emitted('select')` 长度恰为 1。
* [x] `mousedown` 单独触发不再 emit `select`。
* [x] 拖拽排序仍可用（不改动 VueDraggable 配置）。
* [x] 前端单测全绿，覆盖率门禁不下降。

## Decision (ADR-lite)

**Context**：`dir-item` 同时监听 `mousedown` 与 `click`，单击一次即两次 emit；`mousedown` 由 2026-05 提交 `cddcedd` 追加且保留了原有 `click`。

**Decision**：删除 `@mousedown`，保留 `@click` 作为唯一选中入口，与同构组件 `AiChatPanel.vue` 的目录列表保持一致。

**Consequences**：单次点击只触发一次切换；SortableJS 拖拽结束后会抑制紧随 click，因此拖动排序后不再自动切换选中目录（视为可接受，排序与选中属两个独立操作）。若后续产品要求「拖拽即选中」，应改为在 `onDragEnd` 中显式处理，而非恢复双事件绑定。

## Definition of Done (team quality bar)

* 单测新增/调整，断言事件次数
* `npm test` 全绿
* 无文档需要更新（行为修复）

## Out of Scope (explicit)

* 工作目录列表的其他交互重构（双击重命名、多选等）。
* `Home.onDirectorySelect` 逻辑本身的重构。

## Technical Notes

* 关键文件：`frontend/src/components/DirectoryTree.vue`、`frontend/src/views/Home.vue`、`frontend/src/components/__tests__/DirectoryTree.spec.js`
* 历史提交：`cddcedd`（新增 mousedown）、`512b529`（迁移 vue-draggable-plus）、`7310273`（Pinia 迁移）
