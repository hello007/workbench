# 占位符默认值语法 `{{名称:默认值}}`

## Goal

为占位符增加默认值语法（如 `{{页数:1}}`）：用户未填写该项时用默认值替换，而不是保留原样。**前后端同步支持**——提示词面板（前端解析）与 AI 功能模板（Go `renderPrompt` / `BuildStagePrompt`）保持同一套语法与未填语义。

## Decision (ADR-lite)

**Context**：现有 `{{名称}}` 未填写时保留原样，对「页数/份数/称呼」这类有常用值的字段，用户每次都要手动填写；且 `docs/spec/prompt-library.md` 已承诺占位符语法与 AI 功能模板一致。

**Decision**：
- 语法：`{{名称:默认值}}`，以**首个半角冒号**分隔；默认值可含冒号、空格、中文，不可含花括号或换行。
- 默认值去首尾空白；名称去首尾空白后 ≤30 码点，默认值 ≤200 码点，超限整段按普通文本处理。
- 同名占位符出现不同默认值时，**以首次出现者为准**（两处统一）。
- 未填语义三级回退：**填写值（非空）→ 默认值（非空）→ 保留整段原样**。
- 提示词填写弹窗用 placeholder 提示默认值，输入框不预填；「复制原文」始终输出原文。
- Go 侧 form 的 Required 校验把模板中该字段的非空默认值视为已满足；file/text 单字段模式不扩展默认值语义（仍需参数值）。
- 不做转义、不支持全角冒号、不做历史正文自动迁移。

**Consequences**：
- 历史正文若存在 `{{a:b}}` 这类无意写法，行为从「保留原样」变为「未填时替换为 b」，属语法演进。
- 前后端各维护一份解析实现（跨语言无法共享代码），通过同一组用例表与文档约束防止漂移。

## Requirements

### 通用语法

1. 识别 `{{名称:默认值}}`：名称 = 首个半角冒号前内容；默认值 = 首个冒号后全部内容。
2. 名称/默认值均去首尾空白；名称 ≤30 码点，默认值 ≤200 码点；均不得含花括号或换行。
3. 默认值为空（`{{页数:}}`）视为无默认值，未填时保留整段原样。
4. 不满足规则者按普通文本处理（不识别、不替换）。
5. 同名占位符只保留首次出现的默认值定义。

### 前端（提示词面板）

6. `extractPlaceholderDefs(text)` 返回去重后的 `{ name, defaultValue }[]`；`extractPlaceholders` 保持返回名字数组（由其派生）。
7. `applyPlaceholders` 按「填写值 → 默认值 → 原样」替换；`{{名称}}` 旧行为不变。
8. 填写弹窗 placeholder 有默认值时追加「可留空，默认 X」；顶部提示与编辑弹窗 hint 同步语法说明。
9. 占位符徽标计数、输入项数量按名称去重，不受默认值影响。
10. 「复制原文」不做替换。

### 后端（Go AI 功能模板）

11. `renderPrompt` 改为解析式替换（正则匹配全部 `{{...}}`），按三级回退求值，与前端一致。
12. `BuildStagePrompt` 的 form Required 校验：参数为空但模板中该 key 有非空默认值时放行。
13. file/text 模式维持现状：参数值仍为必填，模板默认值不替代该必填校验。
14. `BuildFollowUpPrompt` 二级「无残留 `{{`」判定沿用，默认值替换后可正常采用。

## Acceptance Criteria

- [ ] `{{页数:1}}` 未填 → `1`；填写 `3` → `3`
- [ ] `{{名称}}` 未填 → `{{名称}}`
- [ ] `{{页数:}}` 未填 → 保留 `{{页数:}}`
- [ ] `{{时间:10:30}}` 未填 → `10:30`
- [ ] `{{页数:1}} … {{页数:5}}` 未填 → 两处均为 `1`
- [ ] `{{页数: 1 }}` 默认值去空白后为 `1`
- [ ] 超长名称/默认值不识别为占位符
- [ ] 前端：复制原文输出含 `{{页数:1}}` 的原文
- [ ] Go：`renderPrompt` 默认值用例与表单必填放行用例通过
- [ ] Go：file/text 必填行为不变（回归测试）
- [ ] 前端与 Go 新增/既有测试全绿；E2E `prompt-library.spec.js` 扩展默认值链路并通过
- [ ] `docs/spec/prompt-library.md` 与 AI 模板相关文档同步语法说明

## Technical Approach

### 前端

- `frontend/src/utils/promptPlaceholders.js`
  - 常量：`PLACEHOLDER_DEFAULT_MAX_LENGTH = 200`、`PLACEHOLDER_SEPARATOR = ':'`
  - 新增 `parsePlaceholder(raw)` 内部单点：拆名称/默认值并归一；
  - 新增 `extractPlaceholderDefs(text)`：按首次出现去重；
  - `extractPlaceholders` = defs.map(name)；
  - `applyPlaceholders` 替换回调用 parsePlaceholder，实现三级回退。
- `PromptLibraryPanel.vue`：placeholder 文案、顶部提示、编辑 hint、`v-pre` 提示。
- 文档：`docs/spec/prompt-library.md` §2/§3。

### 后端

- `service/ai_function.go`
  - 新增 `promptPlaceholderDef{Name, Default string}` 与 `parsePromptPlaceholders(template) []promptPlaceholderDef`（正则 `\{\{([^{}\r\n]+)\}\}`，同一组长度/字符规则）；
  - `renderPrompt` 改为 `ReplaceAllStringFunc`，回调内三级回退；
  - 新增 `templateDefaultByName(template) map[string]string`（供 Required 校验）；
  - form Required 校验改为「参数空且默认值为空」才报错。
- 测试：`service/ai_function_test.go` 覆盖默认值、覆盖优先级、空默认、含冒号、超长、同名首次、Required 放行、file/text 回归。

### 一致性守护

- 前端 `promptPlaceholders.spec.js` 与 Go `ai_function_test.go` 使用同一组语义用例（名称/默认值/期望输出三列），防两侧漂移。

## Out of Scope

- 默认值中的花括号转义语法。
- 全角冒号 `：`。
- 默认值类型校验（页码是否为正整数等），一律按字符串处理。
- 历史正文自动迁移或兼容开关。
- file/text 单字段模式的默认值替代必填。

## Technical Notes

- 前端：`frontend/src/utils/promptPlaceholders.js`、`PromptLibraryPanel.vue`、对应单测、`frontend/e2e/prompt-library.spec.js`
- 后端：`service/ai_function.go:955-1040`、`service/ai_function_test.go:103-120/537-610`
- 文档：`docs/spec/prompt-library.md` §2/§3、`docs/spec/ai-structured-output.md`
