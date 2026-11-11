# 修复临时待办折叠态第三行被遮挡

## Goal

上一轮视觉重构给折叠正文加了 20px 底部渐隐，实测行高 20.8px、三行高度 62px，渐隐几乎覆盖第三行整片高度，视觉上像被模糊阴影压住。去掉渐隐、改用 `-webkit-line-clamp` 自带省略号 +「展开」按钮表达截断，并给正文底部留出呼吸空间。

## What I already know

- 正文 13px / 行高 20.8px，折叠三行高度 62px，省略号已正常渲染。
- `.todo-text-wrap.has-overflow::after` 高度 20px，是遮挡来源。
- 正文底部到「展开」按钮仅 4px（`.todo-main` 的 `gap: var(--spacing-xs)`）。
- `has-overflow` class 当前条件为 `overflowIDs.has(id) && !expandedIDs.has(id)`。

## Decision (ADR-lite)

**Context**：渐隐与省略号/展开按钮功能重复，且 20px 高度压住第三行。
**Decision**：删除 `::after` 渐隐；`has-overflow` 改为仅由 `overflowIDs` 驱动（展开态保留），并给 `.todo-text-wrap.has-overflow` 加 `padding-bottom: var(--spacing-sm)`（8px）。
**Consequences**：第三行完整可见，正文与「展开/收起」按钮间距约 12px；底部不再有阴影条；短条目（未截断）不受影响。

## Requirements

1. 删除 `.todo-text-wrap.has-overflow::after` 渐隐规则。
2. `.todo-text-wrap.has-overflow` 增加 `padding-bottom: var(--spacing-sm)`。
3. 模板两处 `has-overflow` 条件去掉 `&& !expandedIDs.has(id)`。
4. 其余折叠/展开行为、按钮文案、`data-action` 不变。

## Acceptance Criteria

- [ ] 折叠态第三行完整可见，底部无渐变/阴影遮罩
- [ ] 正文底部与「展开」按钮间距 ≥ 8px（实测应约 12px）
- [ ] 未截断短条目无额外底部留白与按钮
- [ ] 展开/收起行为与文案不变
- [ ] `TempTodoPanel.spec.js` 全通过，`npm run build` 通过

## Out of Scope

- 折叠行数（仍为 3 行）、展开限高策略
- 其他面板或卡片样式

## Technical Notes

- 文件：`frontend/src/components/TempTodoPanel.vue`（模板 2 处、样式 1 处）
- 规范：`docs/spec/frontend-visual-conventions.md`、`docs/spec/temp-todo.md §8`
- 复测：沿用 Playwright 临时测量用例，断言 `::after` 高度为 0、正文底到按钮间距 ≥ 8
