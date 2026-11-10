# 源码触点清单（二期实现前探查）

> 本文件记录实现所需的既有代码位置与可复用范式，避免实现阶段重复全仓搜索。

## 1. 一期复用面

| 位置 | 现状 | 二期用途 |
|---|---|---|
| `app_temp_todo.go:18` | `AddTempTodo(text string, tags []string) (model.TempTodo, error)` | 两个右键入口直接调用，零新增记录方法 |
| `service/temp_todo.go:69` | `List()` 读取时执行保留策略 | 导出前复用同一份内存数据 |
| `service/temp_todo.go:235` | `loadLocked` 惰性加载 + 损坏留底 | 导出方法内部沿用 |
| `service/temp_todo.go:275` | `saveLocked` 走 `util.SaveJSON` | 导出不写 JSON，不涉及 |
| `frontend/src/components/TempTodoPanel.vue:397` | `onMounted` 单次 `load()` | 需补外部变更刷新 |
| `frontend/src/components/TempTodoPanel.vue:380` | `quickAddFromClipboard` | 面板内已有刷新范式：动作后调 `load()` |

## 2. 右键菜单范式

| 位置 | 现状 |
|---|---|
| `FilePreviewRenderer.vue:1116` | `contextMenu = reactive({visible,x,y,canCopy})` |
| `FilePreviewRenderer.vue:1128` | `lastSelectedText` 选区缓存（右键清除 DOM 选区时回退） |
| `FilePreviewRenderer.vue:1134` | `readSelectionInPreview()` 限定选区在 CodeMirror / markdown 容器内 |
| `FilePreviewRenderer.vue:1154` | `onContextMenu`：仅 `kind === 'text'`，`preventDefault + stopPropagation`，视口边界回退 |
| `FilePreviewRenderer.vue:1186` | `closeContextMenu` |
| `FilePreviewRenderer.vue:1216` | `onMenuCommand(command)` 分发 |
| `FilePreviewRenderer.vue:223-247` | 菜单模板（复制 / 全选 / 行归属） |

`AiChatPanel.vue` 当前无右键菜单，需新建同类状态机；挂载点选 assistant 气泡 `.chat-msg-md`（`AiChatPanel.vue:196-204`），避免 user 气泡误触发。

## 3. 信箱范式

| 位置 | 现状 |
|---|---|
| `store/ui.js:40-51` | `terminalPendingCmd` + `pushTerminalPendingCmd`，seq 单调递增 |
| `store/ui.js:57-67` | `traceBlameRequest` + `pushTraceBlameRequest` |
| `TerminalPanel.vue:558-600` | `consumePendingCmd()`：取 `seq` 消费，失败路径置 null 放弃 |

## 4. 对话框与落盘

| 位置 | 现状 |
|---|---|
| `app_preview.go:68` | `OpenDirectoryDialog(title)`，serve 模式经 `wailsRuntimeUnavailable` 拒绝 |
| `app_preview.go:40` | `SaveFileDialog(defaultFilename, filters)` |
| `app_preview.go:31` | `SaveFile(filePath, content, encoding)` 覆盖写 |
| `AssetsPanel.vue:2153` | `pickExportMdDir = () => OpenDirectoryDialog('选择 Markdown 导出目录')` |
| `transport/index.js:29` | `isBrowserChannel()` 判定浏览器通道 |

## 5. 错误码与契约

| 位置 | 现状 |
|---|---|
| `model/app_error.go:132-140` | 临时待办域五个错误码（4 warning + 1 error） |
| `frontend/src/utils/error.js:45-52` | 临时待办域错误码映射 + `WARNING_CODES` |
| `main.go:258` | `RPCHiddenMethods` 现清单 |
| `frontend/wailsjs/go/main/App.js` / `App.d.ts` | App 绑定三处同步点之二（`models.ts` 无新类型时不动） |

## 6. 测试

| 位置 | 说明 |
|---|---|
| `frontend/src/components/__tests__/TempTodoPanel.spec.js` | 一期面板单测（`quickAddFromClipboard` 等） |
| `frontend/src/components/__tests__/AiChatPanel.spec.js` | AI 面板单测，需 mock `AddTempTodo` |
| `frontend/src/components/__tests__/FilePreviewRenderer` 相关 spec | 右键菜单既有用例 |
| `service/temp_todo_test.go` | 一期 service 单测，导出用例追加于此 |

## 7. 约束摘录

- 新入口带 `data-action`，禁 `mousedown + click` 双绑（响应一次点击只 emit 一次）。
- 浏览器通道：新增纯桌面数据面方法须同步 `RPCHiddenMethods`。
- 新错误码三处同步（`model/app_error.go` + `error.js` 的 `ErrorCode` / `WARNING_CODES`）。
- `util.SaveJSON` 原子写约束仅覆盖 `data/` 下 JSON，不适用于用户目录 Markdown 追加。
