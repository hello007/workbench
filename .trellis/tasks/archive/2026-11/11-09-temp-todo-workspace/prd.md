# 临时待办工作区（Temp Todo Workspace）

## Goal

在与 AI 对话、阅读代码、跨工具工作时，用户会持续产出「临时待办、提示词片段、值得记住的一句话」。当前用记事本手记，**缺时间、缺标签、缺归档**，导致内容很快被遗忘。

本任务在 WorkBench 内新增一级侧边栏面板「临时待办」，提供四个核心动作：**剪贴板一键落条（应用内快捷键）、打标签、一键复制、勾选完成归档**，并支持在已完成区逐条清理。目标是让「记下来」的摩擦趋近于零，让「找回来」有时间和标签两条线索。

## Requirements

### 1. 面板与布局

| 项 | 定义 |
|---|---|
| 入口 | 一级侧边栏面板，`id = todo`，标签「临时待办」，编号顺延为 `Alt+9` |
| 结构 | 顶部工具条（搜索框 / 新建按钮 / 待办·已完成计数）→ 待办列表 → 已完成折叠区 |
| 待办列表 | 按创建时间倒序（最新在上）；每条显示勾选框、正文（最多 2 行，超出省略）、创建时间、标签 |
| 已完成区 | 默认折叠；展开后按完成时间倒序；每条显示取消勾选、正文、完成时间、删除按钮 |
| 行内操作 | 悬浮显示「复制 / 编辑 / 删除」；删除待办需二次确认 |
| 空状态 | 无条目时给出「按 Ctrl+Alt+N 从剪贴板快速记录」引导 |

### 2. 记录

| 项 | 定义 |
|---|---|
| 快捷记录 | 应用内快捷键触发，读取系统剪贴板文本直接落一条待办；成功后提示前 20 字预览，**不强制切换面板** |
| 快捷键键位 | 默认 `Ctrl+Alt+N`，纳入设置区「快捷键」可自定义项（与命令面板、终端同级） |
| 手动新建 | 面板内「新建」进入编辑态，输入多行文本后保存 |
| 空白校验 | 全空白文本拒绝保存并提示 |
| 剪贴板异常 | 剪贴板为空 → 提示「剪贴板为空，未记录」；剪贴板内是文件（无文本格式）→ 提示「剪贴板不含文本」；两种情况均不落条 |

### 3. 元数据与编辑

| 项 | 定义 |
|---|---|
| 时间 | `createdAt` 必填；完成后写 `completedAt`；列表显示相对时间（如「3 小时前」），悬浮显示绝对时间 |
| 标签 | 自由文本标签数组（输入即建，逗号分隔）；编辑态下可增删；不做标签管理器、不做标签树 |
| 关联 | 仅标签／关键字，**不自动附带工作目录、不存文件路径、不存 AI 会话 ID**（来源可能是 WorkBench 外的工具） |
| 编辑 | 正文与标签均可事后修改；编辑不改变 `createdAt` |

### 4. 状态流转与清理

| 项 | 定义 |
|---|---|
| 完成 | 勾选后置 `completedAt`，条目进入已完成区 |
| 取消完成 | 已完成区取消勾选 → 回到待办区，清空 `completedAt` |
| 删除待办 | 二次确认后物理删除 |
| 删除已完成 | 逐条一键删除，**无需二次确认**（已完成的删除成本低、可重建） |
| 保留策略 | 已完成条目仅保留最近 **30 天**，另设硬上限 **500 条**；超出在读取时自动清理并落盘 |

### 5. 检索

| 项 | 定义 |
|---|---|
| 搜索 | 顶部搜索框对**正文 + 标签**做不区分大小写包含匹配，前端过滤，两类列表同时受控 |
| 排序 | 不提供自定义排序，固定按时间倒序 |

### 6. 持久化与跨模式

| 项 | 定义 |
|---|---|
| 存储 | `data/temp_todos.json`，统一走 `util.SaveJSON` 原子写 |
| 读剪贴板 | 后端新增 `util.ReadClipboardText`（Windows 读 `CF_UNICODETEXT`，非 Windows 走既有 clipboard 工具链），由 service 一次性完成「读剪贴板 + 落条」 |
| 浏览器模式 | 与桌面模式同一条后端链路（后端在本机运行），前端零分叉；该方法**不加入** `RPCHiddenMethods` |
| 事件 | 本服务不向前端推送事件，无需 `SetEventSink` |

## Acceptance Criteria

- [ ] 面板出现在活动栏第 9 位，`Alt+9` 可切换，设置区可调整顺序且编号跟随
- [ ] 复制一段文本后在非输入态按 `Ctrl+Alt+N`，待办列表新增该条，提示包含前 20 字预览
- [ ] 剪贴板为空时按快捷键，提示「剪贴板为空，未记录」且列表无新增
- [ ] 快捷键可在设置区改为其他组合，重启后保持；与已有快捷键冲突时被拒绝
- [ ] 新建/编辑条目可保存多行文本与多个标签，刷新或重启应用后仍在
- [ ] 点击条目复制按钮，系统剪贴板内容等于该条正文
- [ ] 勾选后条目从待办区移入已完成区并显示完成时间；取消勾选可回退
- [ ] 已完成条目删除后立即消失，重启后不再出现
- [ ] 已完成条目超过 30 天或超过 500 条时，读取后自动清理过期项
- [ ] 搜索框输入关键字，只保留正文或标签命中的条目
- [ ] 相对时间文案正确，悬浮显示绝对时间

## Definition of Done

- 后端：`service.TodoService` 单测覆盖增删改查、完成/取消、30 天与 500 条清理边界、剪贴板为空与无文本两条失败分支
- 前端：面板组件单测覆盖快捷键记录、复制、勾选流转、删除、搜索过滤；`store/settings.js` 新增快捷键字段的解析与冲突用例
- 覆盖率：service ≥76%、前端 ≥70%（现有门禁）
- 契约：新增 App 方法、`model.TempTodo` 字段、Settings 快捷键字段须同步 `frontend/wailsjs/` 三处
- 新增错误码三处同步（`app_error.go` + 前端 `ErrorCode` / `WARNING_CODES`）
- 面板根元素具备完整 flex 约束链，深浅色主题下视觉一致
- README 按需补充功能说明

## Technical Approach

```
TempTodoPanel.vue（侧边栏面板）
   ├─ 列表/编辑/搜索：纯前端状态 + transport 调用
   ├─ 复制：navigator.clipboard.writeText（沿用现有组件做法）
   └─ 快捷键：Home.vue handleGlobalKeydown → matchShortcut(shortcutQuickTodo)
        → AddTodoFromClipboard()（App 层）
              → service.TodoService.AddFromClipboard()
                    → util.ReadClipboardText() + 落条 + SaveJSON
```

### 数据模型

```go
type TempTodo struct {
    ID          string   `json:"id"`
    Text        string   `json:"text"`
    Tags        []string `json:"tags"`
    CreatedAt   int64    `json:"createdAt"`   // Unix 毫秒
    CompletedAt *int64   `json:"completedAt"` // nil = 未完成
}
```

`CompletedAt` 用指针，避免 `omitempty` 对 0 值失效导致前端误判完成态（参见会话快照 `TerminalSnapshot` 同类决策）。

### Service 接口

| 方法 | 说明 |
|---|---|
| `ListTodos()` | 返回全量，读取时顺带执行保留策略清理 |
| `AddTodo(text, tags)` | 手动新建 |
| `AddTodoFromClipboard()` | 读剪贴板 + 落条，一次调用完成 |
| `UpdateTodo(id, text, tags)` | 改正文/标签 |
| `SetTodoCompleted(id, completed)` | 完成/取消完成 |
| `DeleteTodo(id)` | 物理删除 |

写操作统一「改内存 → `util.SaveJSON` 落盘 → 返回」，失败回滚内存态并返回错误。

### 快捷键键位决策

选 `Ctrl+Alt+N` 而非 `Ctrl+Shift+N`：后者在浏览器模式下会被 Chrome/Edge 吞掉（打开隐身窗口），而 WorkBench 支持浏览器访问模式；`Ctrl+Alt+N` 在浏览器与 WebView2 下均无默认动作。

## Decision (ADR-lite)

**Context**：记录动作发生在「刚看完 AI 输出」这类稍纵即逝的时刻，任何多余点击都会让人退回记事本。

**Decision**：
1. 记录入口用**应用内快捷键**（非系统全局热键）——该时刻 WorkBench 通常在前台，全局热键需后端常驻注册、双平台实现且易与用户热键冲突，收益不抵成本。
2. 面板放**一级侧边栏**而非「工具箱」——它是持续使用的列表，与工具箱里的一次性工具定位不同。
3. 关联收敛为**标签**——来源可能是 WorkBench 外的工具，自动采工作目录意义有限；文件路径与会话跳转引入失效处理与跨面板链路。

**Consequences**：应用最小化时无法记录（可接受）；列表宽度受限，长文需折叠展示（点开或编辑态看全文）；标签无管理界面，重复标签靠使用习惯自然收敛。

## Out of Scope

- 系统级全局热键、托盘常驻
- AI 对话/文件树/代码预览中选中文本右键「存为待办」
- 剪贴板多行批量拆条导入
- 重复内容检测与去重
- 导出 Markdown、转 Trellis 任务、转 AI 提示词模板
- 标签管理界面、标签树、标签统计
- 提醒 / 到期通知、优先级、拖拽排序
- 多端同步、云存储、富文本与附件

## Technical Notes

**必须同步的接入点**

| 位置 | 改动 |
|---|---|
| `frontend/src/store/settings.js` | `SIDEBAR_PANELS` 追加 `{id:'todo', label:'临时待办'}`；`DEFAULTS` 加 `quickTodo`；快捷键 getter/setter |
| `frontend/src/components/ActivityBar.vue` | `panelIconMap` 加 `todo` 图标（内联 SVG，`fill="currentColor"`） |
| `frontend/src/views/Home.vue` | 面板渲染映射 + `handleGlobalKeydown` 快捷键分支 |
| `frontend/src/components/SettingsPanel.vue` | 快捷键列表加「从剪贴板记为待办」可自定义项 |
| `app_services.go` | `AppServices` 加 `todoSvc` 字段 + `NewAppServices` 构造行（2 处） |
| `app_*.go` | 新增委托方法 |
| `frontend/wailsjs/` | `App.js` / `App.d.ts` / `models.ts` 三处手动同步 |
| `app_error.go` + `frontend/src/utils/error.js` | 新增错误码（剪贴板空/无文本） |

**风险与约束**

- 剪贴板读取**仅在快捷键触发时读一次**，禁常驻轮询、禁窗口 `focus` 自动读——历史事故见 `.trellis/tasks/archive/2026-06/06-25-clipboard-fix`（`OpenClipboard` 高频抢占导致其他应用复制粘贴偶发失效）。
- 快捷键分支须沿用现有守卫：输入框/对话框/终端聚焦时放行，避免打断输入与终端快捷键。
- 面板常驻可见时，window 级监听须随使用态挂载（`FolderTree` 先例）；若面板用 `v-show` 常驻，须核对监听数量基线断言取相对增量。
- `util.ReadClipboardText` 非 Windows 分支需与 `clipboard_other.go` 现有工具探测（xclip/wl-paste）保持一致，失败返回可识别错误而非空串。
