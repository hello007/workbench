# 临时待办三期：从 Markdown 导入待办

## Goal

给「临时待办」面板增加 Markdown 导入能力：用户选择一份 .md 文件，系统解析其中的待办行，预览将导入条数与疑似重复项，确认后批量落为临时待办。解决「待办在笔记工具里写过一遍，切回 WorkBench 还得手动重录」的重复劳动，并与二期导出的流水式格式形成可逆闭环。

## What I already know

### 既有实现（一期 / 二期）

* 数据面：`service/temp_todo.go`，`TempTodoService` 惰性加载 + 内存缓存 + 整体原子落盘（`util.SaveJSON`），文件 `data/temp_todos.json`。
* 模型：`model.TempTodo{ID, Text, Tags, CreatedAt(ms), CompletedAt *int64}`。
* 写入单条：`Add(text, tags)`，内部 `normalizeTempTodoText`（换行归一、去首尾空白、20000 rune 截断）+ `normalizeTempTodoTags`（去空、去重）。
* 导出：`ExportMarkdown(dir)` 追加到 `dir/<YYYY-MM-DD>.md`，条目格式 `- HH:MM 正文  #标签`；已完成行尾追加 `（已完成 HH:MM）`；多行正文第二行起缩进两空格。
* App 绑定：`app_temp_todo.go` 七个方法；`ExportTempTodosMarkdown` 在 `main.go` `RPCHiddenMethods` 中屏蔽（浏览器通道不可用）。
* 前端：`frontend/src/components/TempTodoPanel.vue`，工具条含「导出」（`data-action="export"`）与「新建」（`data-action="create"`）；浏览器模式经 `isBrowserChannel()` 前置提示「该功能仅桌面模式可用」。
* 文件能力：`OpenFileDialog(title, filters)`（`app_preview.go:54`），serve 模式经 `wailsRuntimeUnavailable` 前置拒绝；`ReadFileBytes` 返回 base64，前端文本还原须走 `utils/base64.js` 的 `decodeBase64Utf8`（禁裸 atob）。
* 错误码现状：`E_TEMP_TODO_*` 六枚（`model/app_error.go:136-141`），新增错误码须三处同步（`app_error.go` + `error.js` 的 ErrorCode/WARNING_CODES）。
* 无 `docs/spec/temp-todo.md`，一期二期规范未沉淀为独立 spec 文档。

## Decisions (ADR-lite)

* **Q1 创建时间日期归属（已定：A）**：文件名基名匹配 `YYYY-MM-DD.md` 时取文件名日期，否则取导入当天；「日期 + 行内时刻」晚于当前时刻时钳到当前时刻。无行内时间的条目取导入时刻。
* **Q2 疑似重复项（已定：A）**：重复项默认跳过并在预览中单独列出；预览顶部提供「重复项也导入」开关，默认关闭；文件内自身重复按重复处理，保留第一条。
* **Q3 解析边界（已定：A）**：严格对齐导出格式、可逆——① 两空格缩进续行拼回同一待办；② 行尾 `（已完成 HH:MM）` 的已完成行跳过；③ 正文 Markdown 语法原样保留，只做首尾空白清理。
* **Q4 解析位置（技术决策）**：解析在前端纯函数（预览、单测友好），后端新增批量写入方法一次落盘，复用既有归一化，不新增记录语义。

## Requirements

### R1 入口与文件选择

* 「临时待办」面板工具条新增「导入」按钮，`data-action="import"`，位置在「导出」与「新建」之间。
* 点击后：浏览器模式（`isBrowserChannel()`）提示「该功能仅桌面模式可用」并直接返回；桌面模式弹 `OpenFileDialog('选择待办 Markdown 文件', [{ DisplayName: 'Markdown 文件', Pattern: '*.md' }])`。
* 用户取消（空路径）静默返回。
* 读取文件走 `ReadFileBytes` + `decodeBase64Utf8`；读取失败经 `handleError` 提示。

### R2 行解析规则

逐行处理（按文件物理顺序），全程不报错，无法识别的行静默跳过：

1. 去掉行尾 `\r`。
2. 顶层列表项匹配 `^- +(.+)$`（行首至多 3 个空格缩进视为顶层）；
3. 列表项内容分流：
   * `[ ] ` 开头 → 待办，剥离复选框后继续解析内容中的时间前缀；
   * `[x] ` / `[X] ` 开头 → 跳过（已完成）；
   * 以合法 `HH:MM ` 开头（`00:00`–`23:59`）→ 待办，取出行内时间；
   * 其余列表行（如 `- 普通列表项`）→ 跳过。
4. 行尾已完成标记 `（已完成 HH:MM）` 或 `(已完成 HH:MM)` → 整条跳过（导出格式里已完成条目的形态）。
5. 标签提取：从正文尾部反复剥离 `\s+#(\S+)`，得到标签数组；剥离后正文再 TrimSpace。正文中间的 `#` 保留（如 `#123` issue 号不被当作标签的唯一条件是它不在行尾连续段）。
6. 续行：当前正在构建的待办之后，以两个及以上空格（或制表符）开头且非列表项的非空行，去掉两空格缩进后以换行拼入该待办正文；空行或非缩进行结束当前待办。无宿主的续行跳过。
7. 正文归一交给后端 `normalizeTempTodoText`（换行归一、去首尾空白、20000 rune 截断），前端解析阶段只做结构切分。

### R3 创建时间

* 行内时间 + 日期：日期优先取文件名中的 `YYYY-MM-DD`，否则取导入当天；组合晚于当前时刻时钳到当前时刻。
* 无行内时间（`- [ ]` 行）：创建时间取后端导入时刻（前端传 `CreatedAt=0`，由后端填充）。
* 时间参数由前端解析后传给后端，后端对 `CreatedAt<=0` 兜底为 `now()`。

### R4 重复判定

* 判定键：正文删除所有空白字符（含全角空格 `　`）后全等。
* 与现有全部条目（待办 + 已完成）比较；文件内部自身重复同样命中，保留第一条。
* 重复项在预览中单独列出并标注「已存在」。

### R5 预览与写入

* 解析结果为空（0 条待办行）时 `ElMessage.info('未找到可导入的待办行')`，不打开预览。
* 预览用 `el-dialog`：顶部汇总「将导入 N 条 / 跳过重复 M 条」，提供开关「重复项也导入」（默认关闭，切换后汇总与列表实时更新）；列表展示每条的时间、正文、标签，最多渲染 200 条并提示「仅展示前 200 条」。
* 无任何可导入条目时确认按钮禁用。
* 确认后调用后端 `AddTempTodos` 一次落盘；成功后提示「已导入 N 条」，关闭预览、清空搜索关键字、刷新列表。
* 写入失败经 `handleError` 提示，条目一条都不落（后端整体回滚内存态）。

### R6 后端批量写入

* 新增 `model.TempTodoDraft{Text string; Tags []string; CreatedAt int64}`（`CreatedAt=0` 表示取当前时刻）。
* `TempTodoService.AddMany(items []model.TempTodoDraft) (int, error)`：加载 → 逐条归一化（空正文条目跳过）→ 生成 ID → 追加 → 一次 `saveLocked`；落盘失败整体回滚内存态并返回错误；返回实际新增条数。
* App 层新增 `AddTempTodos(items []model.TempTodoDraft) (int, error)`；同步 `frontend/wailsjs/` 三处（App.js / App.d.ts / models.ts）。
* 创建时间晚于当前时刻的条目由 service 再钳一次（纵深防御，前端已钳）。
* 新增错误码 `E_TEMP_TODO_IMPORT_FAILED`（error 级），三处同步。

### R7 浏览器通道

* 前端入口已拦截，文件对话框与读文件均不可达。
* `AddTempTodos` 为纯数据面方法（只新增待办），不加入 `RPCHiddenMethods`，与 `AddTempTodo` 保持一致。
* 不新增凭据、路径、错误码之外的浏览器可见信息。

## Acceptance Criteria

* [x] 工具条出现「导入」按钮（`data-action="import"`），导出/新建按钮位置与既有风格一致。
* [x] 浏览器模式下点击「导入」提示仅桌面模式可用，不发起文件对话框。
* [x] 混合内容文件（标题、段落、`- [x]`、`- 普通列表`、`- [ ] 待办`、`- 15:30 待办 #标签`、已完成导出行、多行正文）解析结果：只导入两类待办行，已完成行与其它行零导入、零报错。
* [x] 多行正文还原为同一条待办，续行内容保留换行。
* [x] 行尾 `#标签` 被提取为 Tags，正文不含标签文本；正文中间 `#123` 保留。
* [x] 文件名 `2026-11-09.md` 的行内 `08:30` 生成 `2026-11-09 08:30` 的创建时间；晚于当前时刻则钳到当前时刻。
* [x] 预览准确报出「将导入 N 条 / 跳过重复 M 条」；开关切换后数字实时变化。
* [x] 确认后一次落盘，列表出现新条目；`data/temp_todos.json` 仅被整体重写一次。
* [x] 全部为重复项时确认按钮禁用。
* [x] 空文件 / 纯标题文件提示「未找到可导入的待办行」。

## Technical Approach

```
TempTodoPanel.vue
  └─ 点击导入 → OpenFileDialog → ReadFileBytes → decodeBase64Utf8
       → utils/tempTodoImport.js  parseTempTodoMarkdown(text, { now, fileName, existing })
            → { items: [{ text, tags, createdAt }], skipped: [...] }
       → el-dialog 预览（汇总 + 列表 + 「重复项也导入」开关）
       → App.AddTempTodos(items) → TempTodoService.AddMany → 一次 SaveJSON
       → load() + 清空关键字
```

* 前端新增纯函数模块 `frontend/src/utils/tempTodoImport.js`：只做结构解析与去重标记，无副作用、无 Vue 依赖，便于单测。
* 后端 `AddMany` 与 `Add` 共用归一化函数；不使用 `Add` 循环（避免 N 次写盘）。
* 预览组件内嵌在 `TempTodoPanel.vue`（单文件，无需新组件）；遵循 `el-dialog` 既有用法与 `data-action` 语义锚。

## Out of Scope

* 导入 .txt / .json / 其他笔记格式。
* `- [x]` 已完成后导入为已完成态。
* 导入时覆盖 / 合并已有条目的编辑历史。
* 浏览器模式的文件上传导入。
* 从 URL / 剪贴板粘贴 Markdown 解析（后续可复用解析函数扩展）。
* 增量导入记录（记住上次导入的文件与偏移）。

## Technical Notes

* 主要文件：`service/temp_todo.go`、`service/temp_todo_test.go`、`app_temp_todo.go`、`model/temp_todo.go`、`model/app_error.go`、`frontend/src/components/TempTodoPanel.vue`、`frontend/src/components/__tests__/TempTodoPanel.spec.js`、`frontend/src/utils/tempTodoImport.js`（新增）、`frontend/src/utils/error.js`、`frontend/wailsjs/` 三处。
* 文本还原：`decodeBase64Utf8`（`frontend/src/utils/base64.js`），禁裸 atob。
* 落盘：`util.SaveJSON` 原子写，禁 `os.WriteFile` 直写。
* 前端单测禁 `waitForTimeout`；语义锚用 `data-action`；写 spec 目录须同步 vitest `test.exclude`（仅当新增 e2e 目录时）。
* 前端覆盖率门禁 ≥70%、service ≥76%：新增解析模块与 `AddMany` 须自带单测，避免整体掉线。
* 文档：实现后确认 README 是否需补一句功能说明；若解析规则有长期价值，沉淀 `docs/spec/temp-todo.md`（可选）。
