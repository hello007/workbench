# 待办与提示词面板编辑焦点及快捷键体验

## Goal

「临时待办」与「提示词」两个侧边栏面板的新建/编辑入口当前缺少一致的键盘体验：部分场景打开后光标不落在输入框、已有内容时光标停在开头，且只能通过鼠标点「保存」或 `Ctrl+Enter` 保存，`ESC` 也无法取消行内编辑。本次统一为：打开编辑即聚焦第一个可编辑框并把光标置于末尾；`Ctrl+S`（macOS `Cmd+S`）保存；`ESC` 取消编辑。

## Requirements

1. **焦点落位**：新建/编辑打开后，焦点自动进入第一个可编辑文本框，光标位于文本末尾；长文本同时滚动到底部。
   - 临时待办：新建卡片、行内编辑、全屏编辑弹窗（正文 textarea）。
   - 提示词：新建/编辑弹窗（标题 input）。
2. **保存快捷键**：`Ctrl+S` / `Cmd+S` 触发保存，覆盖临时待办的新建、行内编辑、全屏编辑，以及提示词编辑弹窗；拦截浏览器默认「保存网页」。
3. **兼容既有快捷键**：临时待办保留 `Ctrl+Enter` 保存。
4. **ESC 取消编辑**：
   - 临时待办新建卡片、行内编辑：`ESC` 丢弃草稿并退出编辑态。
   - 临时待办全屏编辑弹窗：`ESC` 取消整条编辑（关闭弹窗并丢弃草稿、退出编辑态）；弹窗右上角 X 与「返回编辑」按钮维持「关闭弹窗、保留草稿」。
   - 提示词编辑弹窗：`ESC` 关闭并丢弃草稿（关闭时统一复位，含 X 与「取消」）。
5. **无回归**：保存中/保存失败的既有语义、全屏弹窗「保存在途时关闭」的复位逻辑、占位符填写弹窗与导入弹窗行为均不变。

## Acceptance Criteria

- [ ] 临时待办新建：打开后焦点在正文 textarea，光标在末尾
- [ ] 临时待办行内编辑：打开后焦点在正文 textarea，光标在原文末尾
- [ ] 临时待办全屏编辑：弹窗打开后焦点在正文 textarea，光标在原文末尾
- [ ] 提示词新建/编辑：弹窗打开后焦点在标题 input，编辑时光标在原标题末尾
- [ ] 三个临时待办编辑场景按 `Ctrl+S` / `Cmd+S` 均触发保存且浏览器默认行为被阻止
- [ ] 提示词编辑弹窗按 `Ctrl+S` / `Cmd+S` 触发保存
- [ ] 临时待办新建/行内编辑按 `ESC` 退出编辑态且草稿丢弃
- [ ] 临时待办全屏弹窗按 `ESC` 取消整条编辑；点 X 仍返回行内编辑且草稿保留
- [ ] 提示词弹窗按 `ESC` 关闭后草稿与编辑态复位
- [ ] 新增/更新单测覆盖上述焦点、快捷键、取消语义，现有单测与 E2E 全绿
- [ ] `npm run build` 通过

## Technical Approach

- 新增 `frontend/src/utils/focusInput.js`：
  - `resolveInputElement(comp)`：依次取组件暴露的 `textarea` / `input`，再回退组件根 DOM 内的 `textarea, input`；
  - `focusInputAtEnd(comp)`：`focus()` → `setSelectionRange(len, len)` → `scrollTop = scrollHeight`。原生 DOM 与 EP `ElInput` 实例均可传入，测试 stub 也可命中。
- `TempTodoPanel.vue`：
  - 行内编辑正文 textarea 增加函数 ref（`v-for` 内避免数组 ref）；
  - `startCreate` / `startEdit` 在 `nextTick` 后调用 `focusInputAtEnd`；
  - 新建容器、行内编辑容器绑定 `@keydown="onEditorKeydown"`：`Ctrl/Cmd+S` 保存、`ESC` 取消草稿；
  - 全屏弹窗内容区绑定 `@keydown="onDialogKeydown"`：`Ctrl/Cmd+S` 保存、`ESC` 置取消标记；`@closed` 依据「保存完成 / 取消 / 返回编辑」三态处理；
  - `focusEditorDialog` 改为 `focusInputAtEnd`。
- `PromptLibraryPanel.vue`：
  - 标题 `el-input` 增加 ref，弹窗 `@opened` 时聚焦并定位末尾；
  - 编辑弹窗内容包裹容器绑定 `@keydown` 处理 `Ctrl/Cmd+S`；
  - 弹窗 `@closed` 统一复位草稿与 `editingId`（保存成功关闭同样复位，无副作用）。
- 测试：
  - 新增 `focusInput.spec.js`；
  - 升级两面板 spec 的 `el-input` stub（聚焦转发到原生元素）与 `el-dialog` stub（`nextTick` 后 emit `opened`、关闭 emit `closed`）；
  - 用例覆盖焦点与光标、`Ctrl+S`/`Cmd+S`、`ESC` 三态。
- 文档：`docs/spec/temp-todo.md`、`docs/spec/prompt-library.md` 各补一条键盘交互约定。

## Decision (ADR-lite)

**Context**：保存快捷键存在两种候选（沿用 `Ctrl+Enter` 或新增 `Ctrl+S`），全屏弹窗 ESC 语义也存在「返回编辑」与「取消编辑」两种取舍。

**Decision**：`Ctrl+S` / `Cmd+S` 为标准保存键，`Ctrl+Enter` 保留兼容；行内/新建 `ESC` 取消编辑；全屏弹窗 `ESC` 取消整条编辑，X 与「返回编辑」维持保留草稿。原因为 `ESC` 在用户心智中代表「放弃当前操作」，而 X/返回是显式的「暂离」。

**Consequences**：全屏弹窗关闭路径由两态变三态，需在 `@closed` 中区分保存完成、ESC 取消、普通关闭；测试需分别锁定。

## Out of Scope

- 占位符填写弹窗、导入预览弹窗的键盘快捷键。
- 全局自定义快捷键配置项新增（保存键不做可配置）。
- 浏览器访问模式下的额外键盘差异处理。

## Technical Notes

- 现有实现：`frontend/src/components/TempTodoPanel.vue`、`frontend/src/components/PromptLibraryPanel.vue`
- 全局快捷键入口 `frontend/src/views/Home.vue` 的 `handleGlobalKeydown` 未占用 `Ctrl+S`，输入框聚焦时会放行自定义快捷键，因此保存/取消必须绑定在组件内。
- EP 2.14.5 `el-dialog` 默认 `close-on-press-escape=true`，ESC 由 document 冒泡监听处理；组件内先收到的 keydown 可设置取消标记。
- 单测为 stub 环境，焦点断言需对原生元素 spy `focus` / `setSelectionRange`，或 `attachTo` 后断言 `document.activeElement`。
