# 临时待办行内编辑自适应高度（最多 15 行）

## Goal

行内编辑固定 3 行 textarea，用户展开看完长正文再点「编辑」时只能看到 3 行，需要反复滚动或再开全屏。改为按内容自动增高：内容不超过 15 行时全部显示，超过 15 行时最高 15 行并在框内滚动；全屏编辑保留作为超长内容的入口。

## Decision (ADR-lite)

**Context**：固定 `rows=3` 与「先展开阅读、再编辑」的实际使用路径冲突。
**Decision**：行内编辑改用 Element Plus `:autosize="{ minRows: 3, maxRows: 15 }"`，移除 `rows` 与 `resize`；新建卡片保持固定 3 行 + 可拖拽（不随输入变高）。
**Consequences**：autosize 由 JS 接管高度，与原生拖拽互斥，行内编辑失去手动拖拽（仍可全屏编辑）；新建卡行为不变，范围最小。

## Requirements

1. 行内编辑 textarea 改为 `:autosize="{ minRows: 3, maxRows: 15 }"`，删除 `:rows="3"` 与 `resize="vertical"`。
2. 新建卡片 textarea 与全屏弹窗 textarea 不改。
3. 「全屏编辑」按钮、草稿共用、保存/取消语义不变。

## Acceptance Criteria

- [ ] 正文 5 行的条目进入编辑：textarea 完整显示 5 行，无内部滚动
- [ ] 正文 30 行的条目进入编辑：textarea 显示约 15 行，剩余内容框内滚动
- [ ] 正文 1 行/空正文进入编辑：textarea 保持 3 行最小高度
- [ ] 新建卡片仍为固定 3 行、可拖拽
- [ ] 单测与构建通过

## Out of Scope

- 折叠展示、展开限高策略
- 新建卡片的输入高度行为
- 全屏编辑弹窗布局

## Technical Notes

- 文件：`frontend/src/components/TempTodoPanel.vue`（行内编辑 textarea 1 处）
- 文档：`docs/spec/temp-todo.md §7`、`README.md` 临时待办章节中「三行输入区」的表述需同步
- 规范：`docs/spec/frontend-visual-conventions.md` §6「autosize 与原生 resize 互斥」
- 验证：E2E 临时用例测量 `.todo-item textarea` 的 `clientHeight/scrollHeight`，覆盖 5 行与 30 行两种内容
