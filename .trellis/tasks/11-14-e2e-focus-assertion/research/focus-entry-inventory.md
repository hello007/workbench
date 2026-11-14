# 「打开即聚焦」入口全量清单与覆盖缺口评估

> 排查日期：2026-11-14 · 方法：`grep -rn "setSelectionRange|\.focus()|autofocus" frontend/src` 并补扫可选链形态 `focus?.()`（字面量 grep 不命中，先例 `FileTreePanel.vue:829`、`FolderTree.vue:543`、`AiChatPanel.vue:1109/1196/1311`），逐点读上下文判定是否属「打开/呼出入口」。
> 结论先说：**全仓唯一的「末尾」光标落位单点是 `frontend/src/utils/focusInput.js` 的 `focusInputAtEnd`（focus + setSelectionRange(end) + scrollTop）**；`AiChatPanel.vue:1195`（skill 补全）为**指定位置**落位，是该单点之外的唯一例外。其余 `focus()` 调用是各自手写的弹窗/输入框聚焦，语义分三档：末尾、全选、仅聚焦。

**不在本清单的命中点（非输入控件 caret 语义）**：`TerminalPanel.vue:336`（搜索条关闭回焦）、`:479`（tab 首次 init 后）、`:495`（活动 tab 就绪 refit 后）、`useTerminal.js:526`（`focus()` 封装）四处均为 xterm 终端实例焦点回焦，目标是恢复键盘输入而非文本 caret，归 `terminal-multi-session.md` 域；`focusInput.js:44/46/47` 为全仓聚焦单点实现本身，非入口。

## 1. 入口清单

图例：覆盖列 ✅=已有焦点断言；⚠️=有用例但只做功能断言（fill/submit），无焦点断言；❌=无用例。

| # | 入口 | 代码位置 | 聚焦方式 | 光标语义 | 覆盖 |
|---|------|----------|----------|----------|------|
| 1 | 临时待办 · 全屏编辑弹窗 | `TempTodoPanel.vue:291`（`@opened="focusEditorDialog"`）→ `TempTodoPanel.vue:688` | `@opened` + `focusInputAtEnd` | 末尾 | ✅ `temp-todo-editor.spec.js:70,95`（可鉴别范式） |
| 2 | 临时待办 · 新建卡片 | `TempTodoPanel.vue:646`（`startCreate`） | `nextTick` + `focusInputAtEnd` | 末尾（值为空） | ⚠️ 仅行内编辑器存在性断言 |
| 3 | 临时待办 · 行内编辑 | `TempTodoPanel.vue:656`（`startEdit`） | `nextTick` + `focusInputAtEnd` | 末尾 | ⚠️ 同上 |
| 4 | 提示词 · 编辑/新建弹窗 | `PromptLibraryPanel.vue:87`（`@opened="focusEditorAtStart"`）→ `:283` | `@opened` + `focusInputAtEnd` | 末尾 | ✅ `prompt-library-editor.spec.js:83,114`（不可鉴别，已标残余风险） |
| 5 | 命令面板 | `CommandPalette.vue:453`（`watch(visible)`）→ `:461`（`focus()`） | `nextTick` + `focus()` | 无（搜索框为空） | ✅ 间接 `terminal-tabs.spec.js:211`（作 ESC 用例前置） |
| 6 | 文件树 · 新建文件/文件夹弹窗 | `FileTreePanel.vue:1722` | `setTimeout(100)` + `focus()` | 末尾（值为空） | ⚠️ `file-tree.spec.js:62,92`（仅 fill + 提交） |
| 7 | 文件树 · 重命名弹窗 | `FileTreePanel.vue:1765` | `setTimeout(100)` + `focus()` + `select()` | **全选** | ⚠️ `file-tree.spec.js:113`（无焦点断言；功能链完整：回显 + fill + 提交 + 参数断言 + 刷新次数） |
| 8 | 文件树 · 复制到弹窗 | `FileTreePanel.vue:1850` | `setTimeout(100)` + `focus()` | 末尾（默认名） | ❌ |
| 9 | 收藏目录树 · 新增目录弹窗 | `DirectoryTree.vue:495` | `setTimeout(100)` + `focus()` | 末尾（值为空） | ❌ |
| 10 | 收藏目录树 · 重命名弹窗 | `DirectoryTree.vue:557` | `nextTick` + `focus()` + `select()` | **全选** | ❌ |
| 11 | AI 对话 · 添加目录弹窗 | `AiChatPanel.vue:615` | `nextTick` + `focus()` | 末尾（值为空） | ❌ |
| 12 | AI 对话 · 重命名目录弹窗 | `AiChatPanel.vue:658` | `nextTick` + `focus()` + `select()` | **全选** | ❌ |
| 13 | AI 对话 · 待办转提示词信箱 | `AiChatPanel.vue:1109`（`watch(todoPromptRequest)`） | `nextTick` + `focus()` | 末尾（追加文本后） | ❌ |
| 14 | AI 对话 · 模板填充后回焦 | `AiChatPanel.vue:1311`（`applyTemplate`） | `nextTick` + `focus()` | 末尾（整段替换后） | ❌ |
| 15 | AI 对话 · skill 补全光标落位 | `AiChatPanel.vue:1185-1198`（`confirmSkillCompletion`） | `nextTick` + `setSelectionRange(caret)` + `focus()` | **指定位置** | ❌（单测仅 `AiChatPanel.spec.js:1729` 前置造 caret，全 spec 无 `selectionStart/End` 断言） |
| 16 | 内容面板 · 克隆仓库弹窗 | `ContentPanel.vue:1220` | `setTimeout(100)` + `focus()` | 末尾（值为空） | ❌ |
| 17 | 内容面板 · 分支下拉展开 | `ContentPanel.vue:702` | `nextTick` + `focus()` | 无（下拉） | ❌ |
| 18 | 设置 · 快捷键录制 | `SettingsPanel.vue:1510` | `nextTick` + `focus()` | 无（容器） | ❌（`sidebar-shortcuts.spec.js` 无焦点断言） |
| 19 | 终端搜索条 | `TerminalSearchBar.vue:84`（`onMounted`）；初值 `TerminalSearchBar.vue:66`（`ref(props.initialQuery)`，`TerminalPanel` 传回 `searchQuery`） | `onMounted` + `focus()` | 末尾（`initialQuery` 回填，可能非空） | ❌ |
| 20 | 文件树 · 筛选框（截断哨兵点击） | `FileTreePanel.vue:828`（`focusTreeFilter`，`nextTick` + `focus?.()`） | `nextTick` + `focus()` | 无（筛选框初值空） | ✅ `file-tree.spec.js:263` |
| 21 | 浏览器 token 门禁 | `transport/tokenGate.js:99` | `focus()` | 末尾（值为空） | ❌（serve 模式专属，桌面 E2E 通道不覆盖） |
| 22 | 预览 · 全选文本（菜单命令） | `FilePreviewRenderer.vue:1231` | `cmView.focus()` | 全选 | n/a（非「打开」语义） |
| 23 | 素材管理 · 文件夹树 新建/重命名弹层 | `FolderTree.vue:542-543`（`focusDialogInput`，`@opened` → `nextTick` + `focus?.()`；时点约束见同文件 `:537-542` 注释） | `@opened` + `nextTick` + `focus()` | 末尾（新建值空 / 重命名回显原名，无 `select()`） | ❌（单测 `FolderTree.spec.js:218,226` 断 activeElement；无 E2E） |

## 2. 鉴别力分档

| 档 | 入口 | 判定依据 | 说明 |
|---|---|---|---|
| **可鉴别**（有加固价值） | #7 #10 #12 重命名弹窗 | 值非空 + `select()` 产生 `(0, len)` 选区，与「纯 focus 的 `(len, len)`」形态不同 | 天然可鉴别，**无需**「置 0 → 复用重开」技巧；一条用例可证 `select()` 生效 |
| **不可鉴别**（重新赋值/空值） | #6 #8 #9 #11 #16 #19 #21 #2 #3 #13 #14 #23 | 每次打开都重新赋值（或值恒空；#19 为节点重建 + `initialQuery` 可能非空）→ 浏览器默认末尾与 `setSelectionRange` 同形 | 只可断 `toBeFocused`；caret 入参下沉单测 |
| **不可鉴别（已记录）** | #4 | `@closed` 复位草稿 → 重开重新赋 `draftTitle`，节点即使复用值也已重写 | 残余风险已在 `prompt-library-editor.spec.js` 文件头 `:15-19` 声明（用例 :76-87 为基线断言） |
| **光标语义为指定位置** | #15 | `setSelectionRange(caret)` 落点位与默认末尾不同，可鉴别 | 前置条件是补全后 token 后置空格，`caret` 非末尾 |

## 3. 是否值得补 E2E

### 3.1 建议补（P2，3 条同构用例可并为 1 个 spec）

| 入口 | 理由 | 断言设计 |
|---|---|---|
| #7 文件树重命名 | 使用频率最高；`select()` 失败会让用户误删原名的代价最高（改名场景下默认不选中，用户继续输入即变成「原名+新输入」） | 打开后断 `selectionStart === 0 && selectionEnd === value.length`，一次即证 |
| #10 收藏目录重命名 | 同构，同一断言模板 | 同上 |
| #12 AI 对话重命名目录 | 同构，同一断言模板 | 同上 |

> 三条入口的聚焦实现各自独立（无单点工具），因此「一处修好、另一处回归」不会被其它用例发现——这是补它们的核心理由。

### 3.2 可补可不补（P3，仅 `toBeFocused`）

- #13 待办转提示词信箱：跨面板跳转 + 聚焦是用户可见的一次性动作，`toBeFocused` 有价值；但属「不可鉴别」档，收益止于「焦点没丢」。
- #15 skill 补全光标：E2E 可读真实 `selectionStart`（单测只断入参），但需先造 skill 列表 mock，成本中等；当前单测覆盖已够。
- #19 终端搜索条：呼出即聚焦，仅 `toBeFocused`，与命令面板同型；命令面板已间接覆盖，重复价值低。

### 3.3 不建议补

| 入口 | 理由 |
|---|---|
| #6 #9 #11 #16 #21（空值新建类弹窗，#8 除外） | 值为空时 caret 唯一位置，断言零鉴别力；「弹窗打开后焦点在输入框」用户价值低，且 `setTimeout(100)` 的时序已由 `toBeFocused` 自动重试天然吸收。**若未来某入口改为回显非空默认值，应随该改动升级为可鉴别断言** |
| #8（文件树复制到弹窗） | 打开即回填非空默认名（`FileTreePanel.vue:1844` `defaultCopyToName()`），且每次打开都重新赋值 → 浏览器默认末尾与 `setSelectionRange` 同形，不可鉴别；仅可断 `toBeFocused`（值非空故不属于「空值新建类」） |
| #2 #3（临时待办行内/新建） | 新建值恒空；行内编辑值非空但**关闭路径不复用节点**（退出编辑即 `v-if` 移除），无法构造复用重开态 |
| #14 模板填充 | 整段替换赋值 → 光标必在末尾，实现无可选项，无回归面 |
| #17 #18（分支下拉/快捷键录制） | 聚焦对象非输入控件，无 caret 语义 |
| #23（素材管理文件夹树弹层） | 新建值恒空；重命名仅 `focus()`，无 `select()` / `setSelectionRange`（无 caret 实现可断言）；焦点已由组件单测 `FolderTree.spec.js:218,226` 覆盖 |
| #5 #20 | 已有焦点断言，重复新增无增益 |
| #22 | 菜单命令非「打开」入口 |

## 4. 观察项（不构成缺陷，仅记录）

1. **聚焦时序实现不统一**：`focusInputAtEnd` 走 `@opened`（EP 打开动画结束），#6/#8/#9/#16 走 `setTimeout(100)`，#11/#12/#17/#18 走 `nextTick`，#23 走 `@opened` + `nextTick`（规避 EP focus-trap 夺焦，组件注释有实测记录）。`setTimeout(100)` 在低配机动画未结束时聚焦会被后续动画抢焦点，但目前无用户反馈，且 E2E 侧 `toBeFocused` 重试可吸收；本任务不改产品代码，仅登记。
2. **重命名弹窗的 `select()` 与 `focusInputAtEnd` 是两条独立实现**：前者不滚动到底（长文件名场景无影响），后者不选中。若后续统一为单点工具，需保留「全选」这一形态参数，勿一律改为末尾——否则用户改名体验退化。
