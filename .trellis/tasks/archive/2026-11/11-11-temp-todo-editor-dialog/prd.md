# 临时待办编辑全屏弹窗

## Goal

临时待办的正文可能较长（提示词片段、多段笔记），但现有新建卡片与行内编辑器的 `el-input type="textarea" :rows="3"` 只能显示三行，长文无法整体浏览与修改。本次在保留现有 3 行编辑形态的前提下，新增全屏编辑入口，让用户在大面积区域内一次性编辑正文与标签。

## Requirements

1. 保留现有新建卡片与行内编辑形态，布局与行为不变。
2. 两处编辑态 footer 各新增一个「全屏编辑」按钮（图标 + 文字），行为一致。
3. 全屏形态采用 `el-dialog fullscreen`：正文编辑区纵向撑满可用高度，标签输入与操作按钮固定在底部。
4. 弹窗与现有编辑态共用同一份草稿态（`draftText` / `draftTags` / `editingId` / `creating`），不产生第二份内容。
5. 弹窗是「放大视图」：`X` / `ESC` / 底部「返回编辑」只关闭弹窗并回到原编辑态，草稿原样保留；仅「保存」落库并退出编辑。
6. 弹窗内支持 `Ctrl+Enter` 保存；保存成功后关闭弹窗、退出编辑态、刷新列表。
7. 空正文沿用「待办内容不能为空」提示，弹窗保持打开；保存失败时弹窗保持打开、内容不丢并提示错误。

## Acceptance Criteria

- [ ] 新建卡片与行内编辑器均显示「全屏编辑」按钮，点击打开全屏弹窗
- [ ] 弹窗内正文区域随窗口高度自适应，长文可整体浏览编辑，不出现三行挤压
- [ ] 弹窗内修改正文 / 标签后关闭，原编辑态草稿同步保留
- [ ] 弹窗内保存调用 `AddTempTodo`（新建）或 `UpdateTempTodo`（编辑），成功后退出编辑态并刷新列表
- [ ] 正文为空或保存失败时弹窗不关闭
- [ ] 原有 3 行编辑、导入预览、剪贴板快捷记录、搜索过滤行为零回归
- [ ] 暗色主题下弹窗背景、标题栏、底栏配色与现有面板一致

## Definition of Done

- 组件单测覆盖：按钮打开弹窗、草稿双向同步、`Ctrl+Enter` 保存参数、空内容不关闭、保存失败不关闭、关闭保留草稿
- `npm run build`、`npm test` 通过
- 手工验证亮 / 暗主题与窗口缩放
- 行为变化同步 `docs/spec/temp-todo.md`

## Technical Approach

- 仅改 `frontend/src/components/TempTodoPanel.vue`，无后端、无契约变更。
- 新增 `editorDialogVisible` 响应式开关；弹窗内 `el-input` 直接 `v-model` 同一份 `draftText` / `draftTags`，天然双向同步。
- `saveDraft` 改造为返回布尔（成功 `true`），弹窗保存处据此决定是否关闭；行内保存行为不变。
- 弹窗 `:close-on-click-modal="false"` 防误触，`ESC` 保留；`append-to-body` 关闭以让 scoped 样式可达，内部 `.el-dialog__body` 用 `:deep()` 建立 `flex: 1; min-height: 0` 约束链。
- 语义锚：按钮 `data-action="expand-editor"`，弹窗保存 `data-action="editor-save"`，便于单测与 E2E 定位。

## Decision (ADR-lite)

**Context**：长文本编辑需要更大的可视面积，但现有 3 行编辑对短内容足够，不应强制改变默认体验。

**Decision**：新增按需打开的全屏 `el-dialog` 作为「放大视图」，与行内编辑共享草稿；关闭不丢内容，仅保存落库。

**Consequences**：改动集中在前端单组件；全屏弹窗在暗色主题下需显式配色覆盖；不引入 Markdown 预览或自动保存等额外能力，后续如需再加。

## Out of Scope

- Markdown 预览 / 富文本编辑
- 正文自动保存、草稿跨会话持久化
- 弹窗内复制、转提示词等条目级操作（仍只在列表条目上提供）
- 浏览器通道差异化处理（全屏弹窗为纯前端交互，两通道一致）
- E2E 用例（本次以组件单测 + 手工验证覆盖）

## Technical Notes

- 组件：`frontend/src/components/TempTodoPanel.vue`
- 现状：新建与行内编辑共用一份草稿态（`draftText` / `draftTags` / `editingId` / `creating`），`saveDraft` 统一落库（`AddTempTodo` / `UpdateTempTodo`）
- 正文展示为 2 行 clamp，编辑 textarea `rows=3`、`resize=vertical`
- 导入预览已使用 `el-dialog`（`width="min(640px, 80vw)"`）
- 项目无 `el-dialog fullscreen` 先例；终端面板全屏为自绘 fixed 层（z-index 1500），不复用
- Element Plus 全屏 dialog 默认白底，暗色主题必须覆盖 `.el-dialog` / header / body / footer 背景
- 交互取舍依据：长文本编辑瓶颈是纵向可视行数，全屏/近全屏模态是同类工具（Trello 卡片详情、Jira 事项编辑）的通行做法
