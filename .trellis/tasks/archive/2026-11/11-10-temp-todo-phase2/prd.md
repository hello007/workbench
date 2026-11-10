# 临时待办工作区二期（Temp Todo Phase 2）

## Goal

一期已落地「剪贴板一键落条 → 打标签 → 复制 → 完成归档」闭环，但记录入口仍局限于系统剪贴板：用户在 AI 回复、代码预览中读到值得留存的内容时，需要先复制再按快捷键，多一步操作且容易丢上下文；同时待办数据只能停留在面板内，无法回流到 AI 对话或用户既有的 Markdown 笔记体系。

本期补齐「来源侧采集 → 使用侧回流」两端：**选中文本右键存为待办**、**条目一键转提示词**、**导出 Markdown 到用户笔记目录**。记录入口复用一期 `AddTempTodo`，不新增记录类 service 方法。

## Requirements

### R1 选中文本右键「存为待办」

| 项 | 定义 |
|---|---|
| 入口一 | `AiChatPanel` 的 AI 回复气泡（`.chat-msg-md`），选中文本后右键弹出菜单 |
| 入口二 | `FilePreviewRenderer` 的 text / markdown 预览，在既有「复制 / 全选 / 行归属」菜单追加「存为待办」 |
| 选区来源 | 当前 `window.getSelection()` 文本，且选区锚点必须落在对应预览区内（防止误捕获区外选区）；右键导致 DOM 选区被清除时回退到最近一次选区缓存（沿用 `FilePreviewRenderer` 的 `lastSelectedText` 机制） |
| 落条行为 | 调既有 `AddTempTodo(text, [])`，**不弹标签输入框**；成功 `ElMessage.success('已存为待办')`，失败走 `handleError` |
| 禁用态 | 未选中文本时菜单项置灰（`is-disabled`），点击 no-op |
| 语义锚 | 新菜单项必须带 `data-action="save-todo"`；禁止 `mousedown + click` 双绑同一 handler |
| 面板同步 | 落条成功后通知待办面板刷新（见 R4） |

### R2 条目「转提示词」

| 项 | 定义 |
|---|---|
| 入口 | 待办区与已完成区条目行内动作新增「转提示词」（`data-action="to-prompt"`，图标 `ChatDotRound`） |
| 通信 | `uiStore` 新增一次性信箱 `todoPromptRequest = { text, seq }` + `pushTodoPrompt(text)`，语义对齐 `terminalPendingCmd` / `traceBlameRequest` |
| 消费 | `AiChatPanel` watch 信箱，消费后置 null；切换 `uiStore.activePanel = 'ai-chat'`、聚焦输入框 |
| 填充语义 | **追加**而非覆盖：输入框为空时直接填入正文，非空时以换行分隔追加到末尾，避免丢失用户已输入内容 |
| 重复点击 | 依赖 `seq` 单调递增，保证同一条目连续两次点击都能生效 |

### R3 导出 Markdown

| 项 | 定义 |
|---|---|
| 入口 | `TempTodoPanel` 工具条「导出」按钮（`data-action="export"`） |
| 目录选择 | `App.OpenDirectoryDialog('选择待办导出目录')`，空串（用户取消）为 no-op |
| 文件 | 目标目录下 `<YYYY-MM-DD>.md`（按导出当天日期），文件不存在时新建并写入一级标题 |
| 追加 | 文件已存在时在末尾追加；追加前若文件末尾无换行先补一个换行，避免与既有内容粘连 |
| 格式 | 见下方「导出格式」 |
| 范围 | 全部条目（待办 + 已完成），已完成条目标注完成状态 |
| 空列表 | 前端按钮在无条目时禁用；后端防御性返回空路径，前端提示「暂无可导出的待办」 |
| 失败 | 目录不存在 / 不可写 / 写入失败返回错误码，前端 `handleError` 提示，不残留半截内容（单次追加写，失败即整体未写） |
| 浏览器模式 | `isBrowserChannel()` 为真时不弹目录对话框，提示「该功能仅桌面模式可用」；后端方法加入 `RPCHiddenMethods`，浏览器通道不可达 |
| 成功提示 | `ElMessage.success` 展示写入文件的绝对路径 |

#### 导出格式（流水式带时间）

```markdown
# 2026-11-10

- 13:44 这条待办正文，多行内容第二行缩进对齐  #重构 #AI提示词
- 15:20 已完成的一条  #想法（已完成 16:02）
```

| 规则 | 说明 |
|---|---|
| 标题 | 新建文件首行 `# YYYY-MM-DD` + 空行；已存在文件不重复写标题 |
| 条目前缀 | `- HH:MM `（24 小时制，取待办 `createdAt` 本地时间） |
| 多行正文 | 第二行起以两个空格缩进，保持在同一个列表项内 |
| 标签 | 正文后空两格接 `#标签`，按条目标签顺序输出 |
| 已完成 | 行尾追加 `（已完成 HH:MM）`，取 `completedAt` 本地时间 |
| 顺序 | 与面板列表一致：待办在前（创建时间倒序），已完成在后（完成时间倒序） |

### R4 面板数据同步

`TempTodoPanel` 为 `v-show` 常驻挂载，仅在 `onMounted` 加载一次。新增跨组件写入入口（R1）后需要刷新：

| 项 | 定义 |
|---|---|
| 通信 | `uiStore` 新增 `tempTodoChangeSeq` + `notifyTempTodoChanged()` |
| 消费 | `TempTodoPanel` watch `tempTodoChangeSeq`，变化时调既有 `load()`；面板自身的增删改仍走本地 `load()` 调用 |
| 不采用 | 不引入后端事件推送（一期已定 `TempTodoService` 无 EventSink），不做切面板监听（重复读盘） |

## Acceptance Criteria

- [ ] AI 回复中选中文本右键，出现「存为待办」；点击后待办列表新增该条，换行保留
- [ ] 代码预览 / Markdown 预览中选中文本右键，「存为待办」可点击并落条
- [ ] 未选中文本时「存为待办」置灰，点击无副作用
- [ ] 两个右键入口的「存为待办」均带 `data-action="save-todo"`，单测断言 click 只触发一次
- [ ] 待办面板处于打开状态时，从 AI 面板右键落条后切回可见新条目（无需重启或手动刷新）
- [ ] 点击待办条目「转提示词」，AI 对话面板被激活、输入框被填入正文且获得焦点
- [ ] 输入框已有内容时，「转提示词」以换行追加，不覆盖原内容
- [ ] 点击「导出」选择目录后，当天 `.md` 文件末尾出现新增条目，格式符合约定
- [ ] 同一天连续导出两次，标题不重复，条目顺序追加
- [ ] 用户取消目录选择时不产生任何文件与提示
- [ ] 浏览器模式下点击「导出」提示「该功能仅桌面模式可用」，不发起请求

## Definition of Done

- 后端单测：`ExportMarkdown` 覆盖新建文件、追加到已有文件、末尾无换行补换行、空列表、非法目录、目录内已有当日文件
- 前端单测：`TempTodoPanel` 转提示词与导出编排（含浏览器模式分支）、`AiChatPanel` 右键存待办与信箱消费、`FilePreviewRenderer` 新菜单项（含禁用态与单击一次）
- 覆盖率：前端 ≥70%、service ≥76%（现门禁）
- 契约：新增 App 方法 `ExportTempTodosMarkdown` 须同步 `frontend/wailsjs/` 三处（App.js / App.d.ts / models.ts 无需改，无新 model 类型）
- 新错误码三处同步：`model/app_error.go` + `frontend/src/utils/error.js` 的 `ErrorCode`（`WARNING_CODES` 不收录，本码为 error 级）
- `main.go` 的 `RPCHiddenMethods` 追加 `ExportTempTodosMarkdown`
- 面板根元素 flex 约束链不变，深浅色主题视觉一致
- README 按需补充功能说明

## Technical Approach

```
入口层
├─ AiChatPanel（assistant 气泡 @contextmenu）
│    └─ 读取选区 → AddTempTodo(text, []) → notifyTempTodoChanged()
├─ FilePreviewRenderer（既有 contextMenu 增项）
│    └─ 同上
└─ TempTodoPanel
     ├─ 转提示词 → uiStore.pushTodoPrompt(text) → AiChatPanel watch 消费 → inputText 追加
     └─ 导出 → isBrowserChannel? 提示 : OpenDirectoryDialog → ExportTempTodosMarkdown(dir)

后端
App.ExportTempTodosMarkdown(dir) → TempTodoService.ExportMarkdown(dir)
     → 校验目录 → 全量读取 → 拼装 Markdown → 追加写 <dir>/<YYYY-MM-DD>.md → 返回绝对路径
```

### 后端接口

| 方法 | 说明 |
|---|---|
| `TempTodoService.ExportMarkdown(dir string) (string, error)` | 全量导出到 `dir` 下当天 `YYYY-MM-DD.md` 并追加，返回文件绝对路径；目录非法返回 `E_TEMP_TODO_EXPORT_FAILED` |
| `App.ExportTempTodosMarkdown(dir string) (string, error)` | App 层委托，加入 `RPCHiddenMethods` |

### 新增错误码

| 码 | 级别 | 触发 |
|---|---|---|
| `E_TEMP_TODO_EXPORT_FAILED` | error | 导出目录为空 / 非绝对路径 / 不是目录 / 写入失败 |

### 前端新增状态

| 位置 | 内容 |
|---|---|
| `store/ui.js` | `todoPromptRequest` + `pushTodoPrompt`；`tempTodoChangeSeq` + `notifyTempTodoChanged` |
| `AiChatPanel.vue` | 右键菜单状态机（visible/x/y/canCopy）、选区读取、信箱消费、输入框追加 |
| `FilePreviewRenderer.vue` | 菜单模板追加一项、`onMenuCommand('saveTodo')` |
| `TempTodoPanel.vue` | 导出按钮与流程、转提示词按钮、`tempTodoChangeSeq` watch |

## Decision (ADR-lite)

**Context**：三个能力横跨「读懂内容的位置」与「记录内容的位置」，如果每个入口各自持有状态，面板数据与 AI 输入框会反复分叉。

**Decision**：
1. 记录入口复用 `AddTempTodo`，不新增记录类 service 方法——记录语义在一期已收敛，二期只扩来源。
2. 跨组件动作统一走 `uiStore` 一次性信箱（`{ payload, seq }` + 消费即清空），沿用 `terminalPendingCmd` / `traceBlameRequest` 两个先例，不引入事件总线或后端推送。
3. 导出落盘放在后端——前端 `SaveFile` 是覆盖写，无法满足「追加到当天文件」；同时后端校验目录可避免前端拼路径的编码差异。
4. 浏览器模式明确降级为「仅桌面模式可用」，并将导出方法加入 `RPCHiddenMethods`，避免浏览器通道获得任意路径写文件能力。

**Consequences**：导出不支持选择自定义文件名（固定当日日期），用户如需按主题归档可在笔记内自行整理；转提示词为纯文本追加，不自动带入标签与时间上下文。

## Out of Scope

- 系统级全局热键、托盘常驻、多端同步
- 富文本 / 附件 / 图片
- 标签管理界面、标签树、标签统计
- 重复内容检测、优先级、拖拽排序
- 转 Trellis 任务、其他导出格式（CSV / JSON / PDF）
- 导出后自动清空待办、按搜索过滤导出
- 导出可自定义文件名或按主题归档

## Technical Notes

- `app_temp_todo.go:18` `AddTempTodo(text, tags)` 现签名可直接复用。
- `FilePreviewRenderer.vue:1116` `contextMenu` 状态、`:1154` `onContextMenu`、`:1216` `onMenuCommand`、`:223` 菜单模板为本次扩展点；`lastSelectedText` 选区缓存机制沿用。
- `AiChatPanel.vue:165` 消息区、`:196` assistant 气泡为右键挂载点；`:983` `inputText` 为转提示词落点；组件在 `Home.vue:89` 以 `v-show` 常驻。
- `ui.js:40` `terminalPendingCmd`、`:57` `traceBlameRequest` 为信箱实现范本；`TerminalPanel.vue:558` `consumePendingCmd` 为消费范式。
- `TempTodoPanel.vue:397` `onMounted` 单次加载，`load()` 为统一刷新入口。
- `app_preview.go:68` `OpenDirectoryDialog`、`:40` `SaveFileDialog`、`:31` `SaveFile`。
- `transport/index.js:29` `isBrowserChannel()` 为浏览器模式判定。
- `main.go:258` `RPCHiddenMethods` 现清单。
- 写入格式约定：用户笔记目录 `D:\工作\Typora\00.TODO\碎碎念`，文件名按日 `YYYY-MM-DD.md`，条目为流水式 `- HH:MM 正文  #标签`。
- 追加写不适用于 `util.SaveJSON`（该约束仅限 `data/` 下 JSON 持久化），本处为用户笔记目录的文本追加。
