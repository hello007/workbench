# 临时待办契约（侧边栏「临时待办」）

> 剪贴板快速记录、选中文本存档、转提示词、Markdown 导出与导入的跨层契约。
> 最后更新：2026-11-11 · 来源任务：11-09-temp-todo-workspace（一期）、11-10-temp-todo-phase2（二期导出）、11-10-markdown（三期导入）、11-11-temp-todo-editor-dialog（全屏编辑）、11-11-temp-todo-ui-polish（视觉重构）

---

## 1. 数据模型与存储

| 项 | 契约 |
|---|---|
| 模型 | `model.TempTodo{ID, Text, Tags, CreatedAt(ms), CompletedAt *int64}`；`CompletedAt` 用指针，nil = 待办区 |
| 导入草稿 | `model.TempTodoDraft{Text, Tags, CreatedAt}`；`CreatedAt <= 0` 表示取导入时刻，ID 由 service 生成 |
| 文件 | `data/temp_todos.json`，顶层 `{"todos": [...]}`，全局共享不随工作目录切换 |
| 落盘 | 每次写操作整体重写，必须走 `util.SaveJSON`（原子写），禁 `os.WriteFile` 直写 |
| 保留策略 | 已完成条目保留 30 天且最多 500 条，`List()` 读取时清理并落盘；待办区永不清理 |
| 读取降级 | 文件损坏经 `util.BackupCorruptFile` 留底后降级空列表；无 ID 或空正文的脏条目剔除 |

## 2. 导出格式契约（二期）

`ExportMarkdown(dir)` 追加写入 `dir/<YYYY-MM-DD>.md`，返回绝对路径；目录须为绝对路径且存在，空列表返回空路径。

```
# 2026-11-10

- 15:00 未完成条目  #标签
- 13:44 多行正文
  第二行  #重构（已完成 14:05）
```

| 规则 | 说明 |
|---|---|
| 条目行 | `- HH:MM 正文`，标签以两空格分隔并逐项 `#` 前缀 |
| 多行正文 | 第二行起缩进**恰好两空格**（`indentTempTodoText`） |
| 已完成标记 | 行尾追加 `（已完成 HH:MM）`（全角括号） |
| 追加语义 | 空文件先写一级日期标题；既有文件末尾无换行先补换行 |
| 顺序 | 复用 `List()`：待办在前（创建时间降序）、已完成在后（完成时间降序） |

## 3. 导入解析契约（三期）

解析是前端纯函数 `parseTempTodoMarkdown(raw, { now, fileName, existingTexts })`（`frontend/src/utils/tempTodoImport.js`），后端只负责批量写入，不重复解析。

### 3.1 行识别矩阵

| 行形态 | 处理 |
|---|---|
| `- [ ] 正文` / `- [ ] HH:MM 正文` | 待办 |
| `- HH:MM 正文  #标签` | 待办（`HH:MM` 须合法且后跟空白） |
| `- [x] …` / `- [X] …` | 跳过 |
| 行尾 `（已完成 HH:MM）` 或 `(已完成 HH:MM)` | 跳过（导出格式的已完成条目） |
| `- 普通列表项`、标题、段落、空行 | 跳过，不报错 |
| 两空格（或一个 tab）缩进的非空行 | 拼回上一条待办的正文；无宿主则跳过 |

补充规则：

- 顶层列表项允许行首 0–3 空格缩进；4+ 空格按续行处理。
- 续行只剥掉**恰好两个空格**（或一个 tab），更深缩进属于正文自身相对缩进。
- 复选框只剥离标记本身，不吞其后空白——`- [ ]   #标签` 仍须能识别出行尾标签。
- 标签从行尾反复剥离 `\s+#(\S+)`；正文中间的 `#123` 保留。
- 剥离后正文为空的条目跳过。

### 3.2 创建时间

| 场景 | 取值 |
|---|---|
| 行内有 `HH:MM`，文件名匹配 `YYYY-MM-DD.md` | 文件名日期 + 行内时刻（本地时区） |
| 行内有 `HH:MM`，文件名不匹配或非法日期 | 导入当天 + 行内时刻 |
| 组合结果晚于当前时刻 | 钳到当前时刻（前端钳一次，`AddMany` 再兜底一次） |
| 无行内时间 | 前端传 `CreatedAt=0`，后端填导入时刻 |

### 3.3 重复判定

- 判定键：正文**删除所有空白字符**（含全角空格）后全等。
- 比较范围：现有全部条目（待办 + 已完成）与文件内前序条目；文件内自身重复保留第一条。
- 重复项默认不写入；预览提供「重复项也导入」开关，默认关闭。

### 3.4 预览与写入

- 预览 `el-dialog`：汇总「将导入 N 条 / 跳过重复 M 条」，N 随开关实时变化。
- 列表最多渲染 200 条（`IMPORT_PREVIEW_LIMIT`），超出提示但不影响全量写入。
- 无可导入条目时确认按钮禁用；解析结果为空提示「未找到可导入的待办行」。
- 确认后调用 `AddTempTodos(items)` 一次落盘；成功后清空搜索关键字并刷新列表。

### 3.5 已知取舍（非缺陷）

| 边界 | 行为 | 原因 |
|---|---|---|
| Markdown 代码围栏内的 `- ` 行 | 会被识别为待办 | 纯行级解析，未实现 fence 状态机 |
| 正文含空行的多行条目 | 导出→导入不可逆（空行结束条目） | 导出把空行写成两空格续行，导入端以空行断句 |
| 续行以列表语法开头 | 按新的顶层条目处理 | 行首缩进 ≤3 即顶层，与 Markdown 语义一致 |

## 4. 批量写入与错误码

| 项 | 契约 |
|---|---|
| 入口 | `App.AddTempTodos(items []model.TempTodoDraft) (int, error)` → `TempTodoService.AddMany` |
| 归一化 | 逐条复用 `normalizeTempTodoText` / `normalizeTempTodoTags`；空正文跳过且不报错 |
| 落盘 | 全部条目 append 后**一次** `saveLocked`；失败整体回滚内存态并返回 `E_TEMP_TODO_IMPORT_FAILED` |
| 浏览器通道 | `AddTempTodos` 为纯数据面方法，与 `AddTempTodo` 同口径，**不进 `RPCHiddenMethods`**；导入入口在前端用 `isBrowserChannel()` 前置拦截，文件对话框与 `ReadFileBytes` 不可达 |

错误码现状（`model/app_error.go` 临时待办域）：4 个 warning（clipboard empty / no text / text required / not found）+ 3 个 error（clipboard failed / export failed / import failed）。

## 5. 测试锚点

| 层 | 断言点 |
|---|---|
| 解析纯函数 | 两类待办行、跳过矩阵、续行拼接与缩进保留、标签提取、文件名日期与钳制、去空白去重（含文件内第三条）、CRLF、空输入 |
| service | `AddMany` 归一化/跳过空条目/`CreatedAt<=0` 取当前/未来钳制/一次落盘；落盘失败返回 `E_TEMP_TODO_IMPORT_FAILED` 且内存回滚 |
| 组件 | 导入入口 `data-action="import"`、浏览器降级提示、预览汇总与开关联动、确认调用参数、空文件提示、全部重复禁用确认 |
| 组件（全屏编辑） | `data-action="expand-editor"` 打开弹窗、正文与标签沿用草稿、保存入参、`Ctrl+Enter` 保存、空正文与保存失败不关闭弹窗、关闭后草稿保留、保存成功退出编辑态、保存在途关闭弹窗后仍复位 |
| 组件（条目展示） | 截断才显示「展开」、按钮切换展开/收起文案、未截断无按钮且点击正文不改变展开态；截断判定纯函数覆盖未布局/相等/1px 容差 |
| 组件（常驻操作） | 四个 `data-action` 图标在未 hover 时仍渲染（防回归到 hover-only） |
| E2E | `frontend/e2e/temp-todo-import.spec.js`：真实面板链路预览汇总 3/2、开关切到 5、确认后 `AddTempTodos` 入参 3/5 条且字段对齐 `TempTodoDraft`；fixture 文件名取历史日期规避未来时刻钳制 |

## 6. 前端实现单点

| 单点 | 位置 | 约束 |
|---|---|---|
| `parseTempTodoMarkdown` | `frontend/src/utils/tempTodoImport.js` | 纯函数、无 Vue 依赖；新增行格式先改此单点并补单测 |
| `indentTempTodoText` | `service/temp_todo.go` | 导出续行缩进；改动须同步导入端的 `CONTINUATION_RE` |
| `normalizeTempTodoText` / `normalizeTempTodoTags` | `service/temp_todo.go` | 单条与批量共用，禁在导入链路另起归一化 |
| `appendTempTodoMarkdown` | `service/temp_todo.go` | 导出追加写入的唯一出口 |
| `isTextOverflowing` | `frontend/src/utils/todoTextOverflow.js` | 截断判定唯一入口；jsdom 无布局，单测直接传模拟元素，禁在组件内另写判定 |

## 7. 全屏编辑形态

| 项 | 契约 |
|---|---|
| 入口 | 新建卡片与行内编辑 footer 各一个「全屏编辑」按钮（`data-action="expand-editor"`），行为一致 |
| 实现 | `el-dialog fullscreen`；弹窗外壳样式写非 scoped 块并以 `.el-dialog.todo-editor-dialog` 圈定，禁裸改全局 `.el-dialog`；容器 `padding: 0`（EP 2.14.5 自带 16px，不归零则 header/footer 背景内缩、贴不到边） |
| 草稿 | 弹窗与行内编辑共用 `draftText` / `draftTags`，不复制、不回写；弹窗只是「放大视图」 |
| 关闭语义 | `X` / `ESC` /「返回编辑」只关闭弹窗、草稿保留；仅保存成功（`editorDialogSaveDone`）经 `@closed` 复位编辑态 |
| 保存 | 行内 `saveDraft` 与弹窗 `saveFromDialog` 共用 `persistDraft`；空正文与保存失败均不关闭弹窗；保存在途被 ESC/X 关闭时 `@closed` 已错过标记，保存完成后由 `saveFromDialog` 就地复位（防残留草稿再次保存出新条目） |
| 快捷键 | 弹窗内 `Ctrl+Enter` 保存 |
| 布局 | body 纵向 flex 约束链（`.el-dialog__body` → `.editor-dialog-body` → textarea 撑满）；标签输入与 footer 固定不随正文滚动 |

## 8. 条目展示契约（11-11 视觉重构）

| 项 | 契约 |
|---|---|
| 折叠 | 正文默认 `-webkit-line-clamp: 3`；截断提示靠浏览器省略号 +「展开」按钮，**不使用底部渐隐遮罩**（20px 渐隐会压住第三行）；`has-overflow` 仅用于底部 8px 留白 |
| 截断判定 | 单点 `isTextOverflowing`：`clientHeight <= 0` 视为未截断（面板隐藏或未布局），`scrollHeight > clientHeight + 1` 才算截断 |
| 展开 | `display:block` + `max-height:50vh` + `overflow-y:auto`；按钮文案「展开 / 收起」，`data-action="toggle-expand"` |
| 点击 | 正文点击**不再**切换展开（避免与选中文字复制冲突）；展开唯一入口是显式按钮 |
| 测量时机 | 可见时 `onMounted`、列表/关键词/展开态变化、`activePanel` 切回 `todo`、容器宽度变化（ResizeObserver；隐藏态 0 宽跳过） |
| 布局 | header 与 body 用 `padding-inline: max(var(--spacing-md), calc((100% - 1000px) / 2))` 限宽居中，背景仍铺满全宽，不改 DOM 结构 |
| 操作按钮 | 四图标常驻 meta 行右端，默认 `--text-tertiary`，悬停卡片或图标提亮，删除悬停转危险色；`data-action` 保持 copy/to-prompt/edit/delete |
| header | 单行；副标题为 `kbd` 键帽，< 900px 隐藏；导入/导出图标化 + tooltip；搜索框 `flex: 0 1 240px`、最小 160px |
| 卡片 | 轻卡片：`--border-light` 描边，hover 升 `--border-color` + `--shadow-sm`（不整圈主色）；已完成卡换 `--bg-tertiary` 与次级文字色 |
