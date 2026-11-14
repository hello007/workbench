# PRD：临时待办全屏编辑与提示词编辑弹窗的 E2E 交互覆盖

## 1. 背景与目标

「临时待办全屏编辑」（`TempTodoPanel.vue` 的 `el-dialog fullscreen`）与「提示词编辑弹窗」（`PromptLibraryPanel.vue` 的新建/编辑弹窗）在 2026-11-12 的 `11-12-todo-prompt-editor-ux` 任务中统一了「打开即聚焦正文/标题并把光标置于文本末尾」与 `Ctrl/Cmd+S` 保存的交互。该批交互当前只有组件单测覆盖，单测用 stub 替换 `el-dialog` 并手工 `$emit('opened'/'closed')`，无法证明以下真实浏览器链路：

1. Element Plus 真实 `@opened` 动画结束后聚焦是否生效、`document.activeElement` 是否确为目标输入框、`setSelectionRange` 是否真的把光标落在末尾；
2. `Ctrl+S` 在真实浏览器中先被组件 `preventDefault` 拦截（不触发浏览器「保存网页」）后是否完成落库；
3. 临时待办全屏弹窗的 `ESC` 依赖 Element Plus 的 document 级监听关闭弹窗，单测注释已明确「stub 不模拟，手动置 false 走 closed」——真实链路未验证；
4. `ESC`（取消整条编辑）与右上角 `X`（保留草稿）的分流只在真实关闭链路中才能区分。

**目标**：用 Playwright E2E 补齐上述四条，断言口径锚定用户可见结果与 Wails bound method 入参。

## 2. 现状调研

### 2.1 组件行为（真相源）

| 组件 | 打开聚焦 | 保存 | 关闭分流 |
|---|---|---|---|
| `TempTodoPanel.vue` 全屏弹窗 | `@opened` → `focusEditorDialog` → `focusInputAtEnd(dialogInputRef)`，聚焦正文 textarea 并定位末尾 | `onDialogKeydown` 命中 `isSaveShortcut` → `saveFromDialog`（`UpdateTempTodo` / `AddTempTodo`） | `ESC` 置 `editorDialogCanceled`，EP 默认关闭 → `@closed` 复位草稿退出编辑态；`X` /「返回编辑」不置标记 → 关闭弹窗、回到行内编辑、草稿保留 |
| `PromptLibraryPanel.vue` 编辑弹窗 | `@opened` → `focusEditorAtStart` → `focusInputAtEnd(titleInputRef)`，聚焦标题 input 并定位末尾（新建为空 → 位置 0） | `.prompt-editor-body` 容器级 `keydown` 命中 `isSaveShortcut` → `savePrompt`（`UpdatePrompt` / `AddPrompt`） | `ESC` / `X` /「取消」统一经 EP 默认关闭 → `@closed="resetEditorDraft"` 复位草稿，三者同语义 |

### 2.2 已有覆盖与缺口

| 层 | 现状 |
|---|---|
| 组件单测 | `TempTodoPanel.spec.js:878-947` 已用 `setSelectionRange` spy + 手工 emit 覆盖焦点、`Ctrl+S`、`ESC`、`X` 保留草稿；`PromptLibraryPanel.spec.js:441-500` 已覆盖新建/编辑聚焦与 `Ctrl+S` |
| E2E | `temp-todo-import.spec.js` 仅覆盖 Markdown 导入；`prompt-library.spec.js` 仅覆盖新建/复制/删除，未覆盖编辑弹窗的聚焦、`Ctrl+S`、关闭分流 |
| 缺口 | 真实 EP 动画后的聚焦与光标位置、真实 `page.keyboard` 触发的 `Ctrl+S` 与 `Escape`、真实 `X` 点击后的草稿保留判定 |

### 2.3 相关规范

- `docs/spec/temp-todo.md` §5（测试锚点）、§7（全屏编辑形态：焦点、关闭语义、快捷键）；
- `docs/spec/prompt-library.md` §4（编辑弹窗与键盘交互）；
- `docs/spec/e2e-testing.md` §6.1（键盘入口挂载竞态：先等挂载可见再按键）、§5（语义锚定位，禁 nth 硬编码）；
- `frontend/e2e/README.md`（新增用例约定：从 `./fixtures` import、`wailsOverrides` 覆盖、禁 `waitForTimeout`）。

## 3. 验收点

| 编号 | 验收点 | 断言 |
|---|---|---|
| A1 | 临时待办全屏弹窗打开后焦点在正文 textarea 且光标位于末尾 | `document.activeElement === textarea`；`selectionStart === selectionEnd === textarea.value.length`；值等于待办原文 |
| A2 | 全屏弹窗内 `Ctrl+S` 保存 | `UpdateTempTodo` 收到 `(id, 编辑后正文, 标签数组)`；弹窗关闭且行内编辑态退出 |
| A3 | 全屏弹窗 `ESC` 取消整条编辑 | 弹窗关闭；行内编辑 textarea 不复存在（回到展示态）；未调用 `UpdateTempTodo`；展示正文仍为原文 |
| A4 | 全屏弹窗 `X` 保留草稿 | 弹窗关闭；行内编辑 textarea 仍存在（编辑态保留）；textarea 值等于弹窗中的改动草稿 |
| B1 | 提示词编辑弹窗打开后焦点在标题输入框且光标位于标题末尾 | `document.activeElement` 为 `[data-field="title"]`；`selectionStart === title.length`；值为原标题 |
| B2 | 提示词新建弹窗焦点在标题且光标位置为 0（空串末尾） | `selectionStart === 0` |
| B3 | 提示词弹窗内 `Ctrl+S` 保存 | `UpdatePrompt` 收到 `(id, 标题, 正文)`；弹窗关闭 |
| B4 | 提示词弹窗 `ESC` 与 `X` 均为取消：关闭后草稿复位 | 关闭后重新打开同一提示词，标题/正文回到后端原值；未调用 `UpdatePrompt` |

## 4. 实现方案

### 4.1 文件落点

| 文件 | 内容 |
|---|---|
| `frontend/e2e/temp-todo-editor.spec.js`（新增） | A1–A4，覆盖全屏编辑焦点/光标、`Ctrl+S`、`ESC` 与 `X` 分流 |
| `frontend/e2e/prompt-library-editor.spec.js`（新增） | B1–B4；与 `prompt-library.spec.js`（新建/复制/删除业务链）分文件，只聚焦编辑弹窗的焦点与键盘交互 |

新增 spec 位于 `e2e/` 根目录，`frontend/vitest.config.js` 已显式 `exclude: ['e2e/**']`，无需改配置。

### 4.2 fixture 设计

- 临时待办：`ListTempTodos` 返回一条形状对齐 `model.TempTodo` 的条目（`id/text/tags/createdAt/completedAt: null`），正文取多字符中文，使「光标位于末尾」与「位置 0」可区分；`UpdateTempTodo` 用默认值即可（成功路径不消费返回值），断言经 `getWailsCalls` 读取调用入参。
- 提示词：`ListPrompts` 返回一条 `model.Prompt` 形状条目；`UpdatePrompt` 同样走调用记录断言。
- 每个用例独立 `describe` + `test.use({ wailsOverrides })`，避免状态串扰。

### 4.3 交互手法

- 打开入口：`openPanel(page, 'todo' | 'prompts')`（语义定位，禁 nth）；待办编辑入口 `[data-todo-id="..."] [data-action="edit"]` → `[data-action="expand-editor"]`；提示词编辑入口 `[data-prompt-id="..."] [data-action="edit"]`。
- 点击列表项图标按钮后 `page.mouse.move(0, 0)` 移开鼠标，规避 `el-tooltip`（`show-after=300ms`）浮层遮挡（先例：`prompt-library.spec.js:133`）。
- 键盘：`page.keyboard.press('Control+s')` / `('Escape')`；按键前先 `expect(输入框).toBeFocused()` 确认焦点已就位（对齐 §6.1 挂载竞态口径）。
- 关闭按钮：`.el-dialog__headerbtn`（先例：`sidebar-shortcuts.spec.js:129`）。
- 全程 `expect` 自动重试，禁 `waitForTimeout`。

## 5. 非目标

- 不覆盖保存在途关闭、会话代次竞争等并发语义（单测已覆盖，且 E2E 需 `__deferred__` 编排，收益低）；
- 不覆盖 `Ctrl+Enter` 兼容路径（`docs/spec/temp-todo.md` §7 保留兼容，主路径为 `Ctrl+S`）；
- 不改动任何产品代码；若 E2E 暴露真实缺陷，另立任务处理。

## 6. 风险

| 风险 | 应对 |
|---|---|
| 真实 EP 关闭动画使 `toBeHidden`/`toBeVisible` 时序抖动 | 全部使用自动重试断言，不写固定等待 |
| `Ctrl+S` 触发浏览器默认「保存网页」 | 组件已 `preventDefault`；若 Playwright 仍拦截，用例失败即暴露回归，属预期保护 |
| `el-dialog` 关闭后 DOM 驻留（`destroy-on-close` 默认 false） | 以可见性而非节点数断言弹窗开关；行内编辑态用 `toHaveCount(0)` 判存在性（v-if 真实移除） |
