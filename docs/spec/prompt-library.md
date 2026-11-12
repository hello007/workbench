# 提示词管理（话术库）契约

> 侧边栏「提示词」面板：常用话术存取 + `{{名称}}` / `{{名称:默认值}}` 占位符复制（未填走默认值，无默认值保留原样）。与「临时待办」同属文本片段管理，但语义不同——提示词是可长期复用的文本资产，没有完成态。

## 1. 数据模型与持久化

| 层 | 位置 | 要点 |
|---|---|---|
| 模型 | `model/prompt.go` | `Prompt{ID, Title, Content, CreatedAt, UpdatedAt}`，时间均为 Unix 毫秒 |
| 服务 | `service/prompt.go` | 惰性加载 + 内存缓存；`List/Add/Update/Delete` |
| 桥接 | `app_prompt.go` | `ListPrompts / AddPrompt / UpdatePrompt / DeletePrompt` |
| 装配 | `app_services.go` | 字段 `promptSvc` + `NewAppServices` 构造各一行 |
| 存储 | `data/prompts.json` | `util.SaveJSON` 原子写；顶层对象 `{"prompts":[...]}` |

约束：

- 标题、正文均必填（`E_PROMPT_TITLE_REQUIRED` / `E_PROMPT_CONTENT_REQUIRED`），空正文无复制价值、空标题列表无法识别。
- 正文归一**只裁首尾空行与行尾空白**（`TrimLeft "\n"` + `TrimRight " \t\n"`），禁用 `strings.TrimSpace`——首行缩进是代码模板/嵌套列表的语义内容；空正文判定由调用方用 `TrimSpace` 兜底。
- 标题上限 100 rune、正文上限 20000 rune（与 `TempTodoTextMaxRunes` 同口径），超出截断并 Warn；前端正文输入框同步 `maxlength=20000` + 字数提示，避免静默截断。
- 编辑保留 `CreatedAt`，刷新 `UpdatedAt`；加载时 `UpdatedAt <= 0` 回填 `CreatedAt`（老数据）。
- 加载剔除无 ID / 空标题 / 空正文的脏条目并逐条记 Warn（`index/id/reason`）；文件损坏走 `util.BackupCorruptFile` 留底并降级空列表。
- 落盘失败三条写路径均回滚内存态：`Add` 截断切片、`Update` 还原整条、`Delete` 插回原位。
- 列表按 `CreatedAt` 降序，同毫秒按 ID 兜底，保证跨端渲染顺序一致。

## 2. 占位符规则

语法与 AI 功能模板一致（Go 侧 `renderPrompt` / `BuildStagePrompt`），解析全部在前端完成，后端只存原文。

支持两种写法：`{{名称}}` 与 `{{名称:默认值}}`（如 `{{页数:1}}`）。

识别规则（`frontend/src/utils/promptPlaceholders.js`）：

- `{{` 与 `}}` 之间去首尾空白后非空；
- 名称与默认值均不含 `{` `}`，不含换行；
- 名称与默认值以**首个半角冒号**分隔，故 `{{时间:10:30}}` 的默认值是 `10:30`，默认值可含冒号、空格与中文；
- 名称去首尾空白后长度 ≤ 30 个 Unicode 码点；默认值去首尾空白后长度 ≤ 200 码点；
- 默认值为空（`{{页数:}}`）视为无默认值；不填时保留整段原样；
- 不满足者（`{{}}`、`{{   }}`、跨行、名称或默认值超长）按普通文本处理，不弹窗、不替换；
- 同名占位符去重后只生成一个输入框，替换时全部出现处一起替换；同名出现不同默认值时**以首次出现者为准**（前后端一致）；
- 不做转义语法，不支持全角冒号 `：`。

入口函数：`parsePlaceholder(raw)` 为「拆名称 / 默认值」单点，`extractPlaceholderDefs(text)` 返回去重后的 `{ name, defaultValue }[]`，`extractPlaceholders(text)` 由其派生名字数组（徽标计数与输入框数量按名称去重）。

### 2.1 未填语义（三级回退，前后端一致）

1. **填写值**：去除首尾空白后非空 → 用填写值替换（输出保留用户原内容，不裁剪首尾空白）；
2. **默认值**：该名称有非空默认值 → 用默认值替换；
3. **保留整段原样**：`{{名称}}` 与 `{{名称:默认值}}` 均原样保留，漏填在结果里一眼可见。

「未填」统一判定为 `strings.TrimSpace(value) == ""`（Go）/ `String(value).trim() === ''`（前端），纯空白视为未填。

### 2.2 兼容性说明

历史正文若存在 `{{a:b}}` 这类无意写法，行为从「保留原样」变为「未填时替换为 b」，属语法演进；不做历史正文自动迁移，也不提供兼容开关。

## 3. 复制交互

- 无占位符：直接 `navigator.clipboard.writeText`，toast 提示，不弹窗。
- 有占位符：弹窗按去重名字逐个渲染「标签 + 自适应多行输入框」；有默认值的输入框 placeholder 追加「可留空，默认 X」，输入框**不预填**默认值（留空才走默认值）。
- 两个出口：「复制原文」与「替换并复制」；「复制原文」始终输出原文，不做任何替换。
- 未填写（undefined / null / 空串 / 纯空白）的占位符按 §2.1 三级回退：有默认值用默认值，无默认值保留 `{{名称}}` 原样。
- 写入失败提示「复制失败，请检查剪贴板权限」，弹窗保持打开。
- 保存/删除命中 `E_PROMPT_NOT_FOUND`（列表已过期，条目被删除）时除 warning 提示外**必须重新拉取列表**：保存场景同时关闭编辑弹窗，避免用户对已不存在的条目反复重试。
- 弹窗尺寸：新建/编辑 `min(1040px, 94vw)`，正文输入 18 行且编辑区 `min-height: 360px`；填写弹窗 `min(820px, 92vw)`（新建/编辑对齐 `frontend-visual-conventions.md` §5「尺寸分档」的「长文编辑器」行，填写弹窗对齐同节 min() 响应式策略）。
- 填写弹窗在输入项下方渲染「实际内容预览」（`data-preview`），随 `fillValues` 实时更新。预览与「替换并复制」共用 `segmentPlaceholders(text, values)` 分段单点，`applyPlaceholders` 即各段文本拼接，保证复制结果与预览不漂移。
- 预览按分段 kind 用 `v-for` 渲染 `<span>`（**禁 `v-html`**）：
  - `filled`（填写值）：`--primary-bg` 底 + `--primary-color` 字；
  - `default`（默认值）：同 filled 配色 + `--primary-color` 虚线边框，`title="使用默认值"`；
  - `unfilled`（未填且无默认值）：`--text-tertiary` 字 + `--border-light` 虚线边框，保留 `{{名称}}` 原文，`title="未填写，保留占位符"`；
  - `text`：原样输出，不加样式类。
- 预览容器 `white-space: pre-wrap`、`word-break: break-word`、`max-height: 32vh` + `overflow-y: auto` + `overscroll-behavior: contain`（滚到边界不链式传给外层弹窗），背景 `--bg-tertiary`、边框 `--border-light`、圆角 `--radius-md`；颜色全部取主题变量，浅色/深色自动适配。
- 暗色主题下 `filled` / `default` 文字提亮为 `--primary-light`（`--primary-color` 叠 `--primary-bg` 仅约 2.4:1）：按本仓「非正文 token ≥ 3:1」口径（提亮后约 3.5:1），**不追求正文 WCAG AA 4.5:1**——高亮是替换片段的标记而非正文，与 CodeMirror 深色 token 同口径（详见 `frontend-visual-conventions.md` §6「Vue scoped 样式」小节）。

## 4. 编辑弹窗与键盘交互

- 新建/编辑共用同一弹窗，`@opened` 后聚焦标题输入框并把光标置于文本末尾（`focusInputAtEnd`，`frontend/src/utils/focusInput.js`）；新建为空、编辑时接着原标题继续输入。
- 标题与正文外层容器监听 `Ctrl+S` / `Cmd+S`（`isSaveShortcut`），命中后 `preventDefault` 并调用 `savePrompt`，避免触发浏览器「保存网页」；两个输入框共用容器级监听。
- 弹窗关闭（`ESC`、右上角 X、「取消」）统一走 `@closed="resetEditorDraft"`，复位 `editingId` 与标题/正文草稿，重新打开不带入上次内容；保存成功关闭同样复位（幂等）。
- 保存发起时记录 `editorSession`（开始/结束编辑递增）；保存请求在途时若已关闭并重新打开，回调只刷新列表，不关闭新弹窗、不清空新草稿；`saving` 期间重复保存直接忽略。
- 前端空标题/空正文校验、`E_PROMPT_NOT_FOUND` 关闭并刷新列表的既有语义不变。

## 5. 面板注册点

新增一级面板须同步（缺任一则入口不可达或构建失败）：

| 位置 | 改动 |
|---|---|
| `frontend/src/store/settings.js` | `SIDEBAR_PANELS` 增项；`DEFAULTS.openPromptLibrary`；`shortcutOpenPromptLibrary` ref + 加载/保存/冲突检查 |
| `frontend/src/components/ActivityBar.vue` | `panelIconMap` 图标映射；第 10 项起 tooltip 不拼 `Alt+N` |
| `frontend/src/views/Home.vue` | `v-show` 挂载 + 自定义快捷键分支 |
| `frontend/src/components/SettingsPanel.vue` | 快捷键标签、可配置列表、默认判定、重置、录制写回 |
| `model/settings.go` | `ShortcutOpenPromptLibrary` 字段（json key 须与前端 `shortcutOpenPromptLibrary` **逐字一致**，不一致时保存被 Go 静默丢弃） |
| `frontend/wailsjs/` | App.js / App.d.ts / models.ts 三处手工同步 |

`Alt+1..9` 是固定映射，只覆盖前 9 个面板；第 10 个面板起没有固定快捷键，可达性由自定义快捷键补位（默认空，不占键）。

## 6. 测试点位

- `service/prompt_test.go`：增删改查、归一化（首行缩进保留）、排序、脏数据剔除、损坏留底、截断、重载持久化、Add/Update/Delete 三条落盘失败回滚。
- `service/settings_test.go`：`ShortcutOpenPromptLibrary` 的 Save→Get 往返 + 磁盘 JSON 键名断言（防 Go/前端键名漂移）。
- `frontend/src/utils/__tests__/promptPlaceholders.spec.js`：提取/替换的全部边界；默认值用例组（含冒号默认值、空默认保留、去空白、超长不识别、同名首次为准、纯空白视为未填、填写值不裁剪）与 Go `TestRenderPrompt_DefaultValue` 保持同一组语义用例，防跨语言漂移；`segmentPlaceholders` 分段（普通文本、填写值、默认值、未填、混合、换行、无效占位符）及 `applyPlaceholders` 与分段拼接一致性。
- `frontend/src/components/__tests__/PromptLibraryPanel.spec.js`：无占位符直复制、弹窗去重、部分填写、默认值 placeholder 提示、未填走默认值、填写覆盖默认值、复制原文保留语法、失败保持、新建校验、编辑、删除确认、搜索、展开、`E_PROMPT_NOT_FOUND` 刷新列表；预览区三类高亮与输入实时更新、复制原文不受预览影响；打开后聚焦标题且光标在末尾、`Ctrl/Cmd+S` 保存并阻止默认、关闭复位草稿。
- `frontend/src/utils/__tests__/focusInput.spec.js`：原生元素与 ElInput 实例/ stub 根元素解析、光标定位末尾、长文本滚动到底、`Ctrl/Cmd+S` 组合键矩阵（含 Alt/Shift 不命中）。
- `frontend/e2e/prompt-library.spec.js`：默认值链路的独立用例（新建 `{{页数:1}}` → 不填得默认值 → 填写覆盖 → 复制原文保留语法）；预览区可见性、`ph-unfilled`/`ph-default`/`ph-filled` 文本与切换断言。
- `frontend/src/views/__tests__/Home.spec.js`：自定义快捷键打开面板 / 空快捷键不劫持。
- `frontend/src/test/wails-mock-defaults.js`：四方法默认值（面板常驻挂载即拉取）。
