# 提示词管理（话术库）

## Goal

为 WorkBench 增加「提示词管理」能力：把常用话术（提示词 / 模板文本）长期保存下来，支持新增、编辑、删除、检索与一键复制；当文本中含有占位符时，复制前弹窗让用户录入占位符取值，也可跳过录入直接复制原文。定位为「可复用的文本资产库」，与「临时待办」的完成态待办语义不同。

## Requirements

### 数据与列表

- 每条提示词：标题 + 正文（纯文本，保留换行）。
- 新增、编辑、删除；无「完成 / 归档」状态。
- 列表按创建时间倒序；关键字搜索同时匹配标题与正文（大小写不敏感）。
- 不做标签、分组、Markdown 导入导出、复制后置顶。
- 删除需二次确认（ElMessageBox 单次确认形态）。

### 占位符

- 语法 `{{名称}}`，与仓库 AI 功能模板（`renderPrompt`）保持一致。
- 宽松单行规则：`{{` 与 `}}` 之间去掉首尾空白后非空、不含 `{` `}`、不含换行、长度 ≤30 即识别；中文、英文、数字、下划线、连字符、空格均允许。
- `{{}}`、`{{   }}`、跨行花括号、名字超过 30 字符：不识别为占位符，按普通文本处理。
- 同名占位符去重后只生成一个输入框，替换时全部出现处一起替换。
- 不做转义语法。

### 复制交互

- 正文不含占位符：点击复制直接写剪贴板，toast 提示，不弹窗。
- 正文含占位符：弹窗列出全部占位符，每项一行标签 + 自适应高度多行输入框。
- 底部两个按钮：「复制原文」与「替换并复制」。
- 未填写的占位符在替换时**保留 `{{名称}}` 原样**（漏填可被发现，且与「完全不填 = 原文」一致）。
- 复制成功后 toast 提示并关闭弹窗；不记忆上一次输入。
- 复制统一走 `navigator.clipboard.writeText`（仓库既有先例），不新增后端剪贴板方法。

### 入口与快捷键

- 新增独立侧边栏面板（`SIDEBAR_PANELS` 增 `prompts`，第 10 项），新增 `PromptLibraryPanel.vue`。
- `Alt+1..9` 映射前 9 个面板的现状不变；第 10 项不占固定 `Alt+数字` 快捷键。
- 为提示词面板提供可自定义快捷键（`matchShortcut` 链路，与「快速记待办」同构），默认空，由用户在设置页录制。

## Acceptance Criteria

- [ ] 活动栏出现第 10 个入口，点击打开提示词面板；`Alt+1..9` 仍映射前 9 个面板。
- [ ] 新增 → 列表即时出现，重启应用后仍在。
- [ ] 编辑标题/正文生效并持久化，更新 `updatedAt`。
- [ ] 删除需确认，确认后列表消失且数据文件同步。
- [ ] 关键字搜索可命中标题命中项与正文命中项，清空关键字恢复全量。
- [ ] 无占位符文本：点击复制直接进剪贴板，不弹窗。
- [ ] 含占位符文本：弹窗展示全部占位符，同名只出现一次。
- [ ] 全部填写后点「替换并复制」，结果中所有出现处均被替换。
- [ ] 部分填写后复制，未填项保留 `{{名称}}`。
- [ ] 「复制原文」复制未经替换的原文。
- [ ] `{{}}`、`{{   }}`、跨行花括号、超 30 字符名字不触发弹窗。
- [ ] 自定义快捷键可打开提示词面板；录制 `Alt+数字` 被既有冲突检查拦截。

## Definition of Done

- 后端 service 单测覆盖增删改查、持久化与异常分支。
- 前端占位符解析抽为独立模块并单测（边界用例见上）。
- 前端组件单测覆盖：无占位符直复制、有占位符弹窗、部分填写、复制原文、搜索过滤、删除确认。
- 覆盖率满足分层门禁（后端 service ≥76%、前端 ≥70%）。
- 面板注册、快捷键、`wailsjs` 三处同步完成。
- 功能完成后确认是否需要更新 `README.md`。

## Technical Approach

- **后端**：新增 `model/prompt.go`（`Prompt{ID, Title, Content, CreatedAt, UpdatedAt}`）与 `service/prompt.go`（`List/Add/Update/Delete`，`util.SaveJSON` 原子写 `data/prompts.json`）；`app_prompt.go` 暴露 App 方法；`app_services.go` 装配（2 处）；若浏览器通道需要访问则补 `web_serve.go` 与 `RPCHiddenMethods` 判定。
- **占位符解析放在前端**：新增 `frontend/src/utils/promptPlaceholders.js`，导出 `extractPlaceholders(text)` 与 `applyPlaceholders(text, values)` 两个纯函数，便于单测；后端不参与模板渲染。
- **前端面板**：`PromptLibraryPanel.vue` 复用待办的视觉语言（搜索框、列表项、内联/弹窗编辑），但独立实现，不改动 `TempTodoPanel.vue`。
- **注册点位**：`store/settings.js` 的 `SIDEBAR_PANELS`、默认快捷键字段、`ActivityBar.vue` 图标映射、`Home.vue` 面板挂载与快捷键分支、`frontend/wailsjs/` 三处手工同步。

## Decision (ADR-lite)

**Context**：侧边栏已有 9 个面板且 `Alt+1..9` 占满；提示词与临时待办都属文本片段管理，存在「合并入口」与「独立入口」两种取舍。

**Decision**：取独立第 10 面板（方案 A），不合并进 1335 行的 `TempTodoPanel.vue`；可达性由可自定义快捷键补位。占位符沿用仓库既有 `{{key}}` 语法并采用宽松单行规则，解析放前端。

**Consequences**：入口数量继续增长，第 10 项无固定 `Alt+数字` 快捷键；但两类数据与组件解耦，后续加标签/分组不会互相牵连。若未来面板数继续增长，需要单独评估「面板分组 / 折叠」方案。

## Out of Scope

- 提示词版本历史 / 回收站。
- 占位符默认值、条件、循环等模板引擎能力。
- 云同步、跨设备共享、AI 自动生成提示词。
- 标签、分组、导入导出。

## Technical Notes

- 已核实文件：`service/temp_todo.go`、`app_temp_todo.go`、`frontend/src/components/TempTodoPanel.vue`、`frontend/src/components/ActivityBar.vue`、`frontend/src/views/Home.vue`、`frontend/src/store/settings.js`、`service/ai_function.go`。
- 关键规则约束：新增 service 须在 `app_services.go` 两处登记；`wailsjs` 三处手工同步；`data/` 持久化必须走 `util.SaveJSON`；复制文本走 `navigator.clipboard.writeText`。
