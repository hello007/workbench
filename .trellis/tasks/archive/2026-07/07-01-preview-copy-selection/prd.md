# 文件预览区支持复制选中文本（Ctrl+C 与右键复制菜单）

## Goal

在右栏（ContentPanel 操作面板）的文件预览态下，用户可以**直接复制鼠标选中的文本内容**：方式一 Ctrl+C（必须），方式二右键菜单"复制"（可选，已纳入）。当前选中文本后按 Ctrl+C 复制不出内容，根因是全局快捷键拦截过宽，本任务修复该问题并补充右键复制菜单。

## What I already know

* 右栏文件预览父组件为 `frontend/src/components/ContentPanel.vue`（预览区 DOM `:124-174`）。
* 文本/代码预览渲染器为 `frontend/src/components/FilePreviewRenderer.vue`，使用 **CodeMirror 6**（只读，`EditorState.readOnly.of(true)`，**未** 用 `EditorView.editable.of(false)`，内容可选中），markdown 走 `markdown-it + highlight.js` 的 `<pre><code>`。
* 预览组件本身**无** `user-select:none`、**无** `@contextmenu`、**无** copy/keydown 事件拦截；全局 `style.css`/`app.css` 也无 `user-select:none`。选中能力本身正常。
* **根因**：`frontend/src/views/Home.vue` 的全局 keydown 处理器 `handleGlobalKeydown`（`:356-395`）在 Ctrl+C 分支只排除了 `INPUT`/`TEXTAREA`（`:382-383`），未排除 CodeMirror 的 `contenteditable`、markdown 的 `<pre>/<code>`、PDF iframe、以及 `window.getSelection()` 非空的情况。命中 `:385-387` 后执行 `e.preventDefault()` + `handleCopy(selectedNode.value)` → `CopyToSystemClipboard(data.path)`，把**文件路径**覆盖进系统剪贴板，导致选中文本无法复制。
* 该根因在历史任务 `06-25-clipboard-fix` 的 PRD 中已被标记为"场景 A / Out of Scope，未修"。
* 文本复制现有 4 处重复实现（`navigator.clipboard.writeText` + `ElMessage`）：FileTreePanel/CommitHistory/GitInfo/ContentPanel，**无公共封装**。
* 右键菜单有成熟自研模式：`<ul class="context-menu">`，全局 CSS 已就绪（`style.css:183-278`）；`DirectoryTree.vue` 最精简、`FileTreePanel.vue` 最全（含视口边界检测 + 多菜单互斥关闭），可直接复用。
* Windows 剪贴板踩坑经验（`06-25-clipboard-fix`）：剪贴板为独占资源、`EmptyClipboard` 会清空他应用内容、避免 `window.focus` 抢锁。文本复制走 `navigator.clipboard.writeText`（浏览器原生，不经 Go/Win32，不触碰系统剪贴板锁）。

## Assumptions (temporary)

* 修复应面向"选中文本"的通用判定，而非仅针对 CodeMirror。
* 右键菜单仅在可选中文本的预览态（文本/代码 CodeMirror、markdown）生效；图片/office/pdf 预览不纳入右键复制。

## Open Questions

* （已全部解决，见 Decision）

## Requirements (evolving)

* **方式一（Ctrl+C 修复，必须）**：修复 `Home.vue` 全局 Ctrl+C 拦截过宽——当存在选中文本时，不劫持 Ctrl+C，交还浏览器原生复制。
* **方式二（右键复制菜单，已纳入）**：在文件预览态右键弹出菜单，含两项——"复制"（选中文本时可用，未选中时禁用）与"全选"（选中预览区全部文本）。复用项目自研 `<ul class="context-menu">` 模式与全局 CSS。
* 文本复制统一走 `navigator.clipboard.writeText`，不新增 Go/Win32 调用，避免系统剪贴板锁回归。

## Acceptance Criteria (evolving)

* [ ] 在文本/代码（CodeMirror）预览区选中文本，Ctrl+C 复制得到的是**选中文本**而非文件路径。
* [ ] 在 markdown 预览的 `<pre>` 代码块选中文本，Ctrl+C 正常复制选中文本。
* [ ] 修复不影响原有"文件树选中文件节点 + Ctrl+C 复制文件路径到系统剪贴板（供资源管理器粘贴）"的功能。
* [ ] 预览态右键出现菜单，含"复制"与"全选"两项；"复制"在未选中文本时禁用。
* [ ] "全选"点击后选中预览区全部文本（CodeMirror 选中全文、markdown 选中整个正文）。
* [ ] 右键菜单带视口边界检测，不溢出窗口边缘；与文件树/目录树右键菜单互斥（开一个关其他）。

## Definition of Done (team quality bar)

* 前端测试（Vitest）覆盖关键分支：选中文本时不劫持 Ctrl+C、无选中/焦点在文件树时保持原行为。
* `cd frontend && npm run build` / lint 通过。
* 不引入剪贴板抢锁回归（文本复制走 `navigator.clipboard`，不新增 Go 调用）。
* 确认是否需要更新 README.md / docs。

## Technical Approach

### 方式一：Ctrl+C 拦截修复（Home.vue）

* 在 `handleGlobalKeydown` 的 Ctrl+C 分支前，增加判定：`if (window.getSelection().toString()) return` —— 只要页面上有选中文本，就放行，让浏览器执行原生复制。
* 理由：`getSelection()` 是最通用的判定，覆盖 CodeMirror contenteditable 与 markdown `<pre>/<code>`，且与文件树节点复制不冲突（文件树节点是点击高亮选中，不产生文本选区）。
* **实现时验证点**：CodeMirror 6 的 contenteditable 选区，`window.getSelection().toString()` 是否能取到选中文本。若取不到，补充"target 落在 `.file-preview-body` 内或为 contenteditable"的二级判定。
* 保持原有 INPUT/TEXTAREA 排除逻辑不动。

### 方式二：右键复制菜单（实现位置：FilePreviewRenderer.vue）

* 在 `FilePreviewRenderer.vue` 的预览根容器绑定 `@contextmenu`，`preventDefault` 后按自研模式弹出菜单（放此处而非 ContentPanel，因可直接访问 CodeMirror view 实例与 markdown DOM，全选逻辑最直接）。
* 复用 `DirectoryTree.vue` 的菜单骨架：`contextMenu` reactive state（visible/x/y）+ 视口边界检测 + 全局 contextmenu 监听实现多菜单互斥关闭。
* 菜单项两项，配置驱动便于后续扩展：
  * **复制**：`window.getSelection().toString()` 为空时 `.is-disabled`；点击 `navigator.clipboard.writeText` → `ElMessage.success`。
  * **全选**：CodeMirror 调 `selectAll` 命令（`@codemirror/commands`，需 view 实例）或 `dispatch({ selection: EditorSelection.range(0, doc.length) })`；markdown 走 Selection API（`getSelection().selectAllChildren(bodyEl)`）。
* 仅在文本/代码（CodeMirror）与 markdown 预览分支挂载该菜单；image/office/pdf 分支不挂载。

## Decision (ADR-lite)

* **Context**：用户在文件预览区选中文本无法复制，且历史任务已识别但未修。
* **Decision**：本次同时交付 Ctrl+C 拦截修复（方式一）与右键复制菜单（方式二）。方式一修根因、保体验底线；方式二复用现有自研菜单模式，边际成本低、贴合用户右键复制习惯。
* **Decision（菜单项）**：右键菜单含"复制"+"全选"两项；不提供"复制全部内容"（与"全选+复制"重叠，避免过度设计）。
* **Consequences**：改动集中在 `Home.vue` 与 `FilePreviewRenderer.vue` 两个前端文件，不触碰 Go/Win32 剪贴板链路，无系统剪贴板锁回归风险。后续若需收敛 4 处重复的复制函数为公共 composable，另立任务。

## Out of Scope (explicit)

* 不改动文件树复制文件路径（CF_HDROP）的 Go/Win32 链路。
* 不重构全局剪贴板状态管理（`clipboard` reactive 对象）。
* 不抽取公共 `useClipboard` composable 收敛 4 处重复——本次保持改动聚焦，另立任务。
* 图片/office/pdf 预览的右键复制不纳入（这些类型无"选中文本"语义）。

## Technical Notes

* 关键文件：
  * `frontend/src/views/Home.vue`（`:356-395` 全局 keydown，`:382-387` Ctrl+C 分支，`:454-461` handleCopy）
  * `frontend/src/components/FilePreviewRenderer.vue`（CodeMirror 初始化 `:316-338`）
  * `frontend/src/components/ContentPanel.vue`（预览区 `:124-174`，待加右键菜单）
  * `frontend/src/components/DirectoryTree.vue` / `FileTreePanel.vue`（右键菜单参考实现）
  * `frontend/src/style.css`（`:183-278` 右键菜单全局样式）
* 参考：`.trellis/tasks/archive/2026-06/06-25-clipboard-fix/prd.md`（"场景 A / 可能的修复方向"）。
