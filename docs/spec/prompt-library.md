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

## 4. 面板注册点

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

## 5. 测试点位

- `service/prompt_test.go`：增删改查、归一化（首行缩进保留）、排序、脏数据剔除、损坏留底、截断、重载持久化、Add/Update/Delete 三条落盘失败回滚。
- `service/settings_test.go`：`ShortcutOpenPromptLibrary` 的 Save→Get 往返 + 磁盘 JSON 键名断言（防 Go/前端键名漂移）。
- `frontend/src/utils/__tests__/promptPlaceholders.spec.js`：提取/替换的全部边界；默认值用例组（含冒号默认值、空默认保留、去空白、超长不识别、同名首次为准、纯空白视为未填、填写值不裁剪）与 Go `TestRenderPrompt_DefaultValue` 保持同一组语义用例，防跨语言漂移。
- `frontend/src/components/__tests__/PromptLibraryPanel.spec.js`：无占位符直复制、弹窗去重、部分填写、默认值 placeholder 提示、未填走默认值、填写覆盖默认值、复制原文保留语法、失败保持、新建校验、编辑、删除确认、搜索、展开、`E_PROMPT_NOT_FOUND` 刷新列表。
- `frontend/e2e/prompt-library.spec.js`：默认值链路的独立用例（新建 `{{页数:1}}` → 不填得默认值 → 填写覆盖 → 复制原文保留语法）。
- `frontend/src/views/__tests__/Home.spec.js`：自定义快捷键打开面板 / 空快捷键不劫持。
- `frontend/src/test/wails-mock-defaults.js`：四方法默认值（面板常驻挂载即拉取）。
