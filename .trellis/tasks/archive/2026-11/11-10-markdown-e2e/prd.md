# 临时待办 Markdown 导入 E2E

## Goal

为三期「临时待办 Markdown 导入」补 Playwright E2E，覆盖预览汇总数字、重复开关联动计数、确认后 `AddTempTodos` 入参三条契约，把「解析纯函数 + 组件单测」已锁定的行为在真实浏览器链路（面板挂载 → 文件对话框 → ReadFileBytes → 解析 → 预览 → 落库调用）上再钉一层。

## Requirements

- 新增 `frontend/e2e/temp-todo-import.spec.js`，从 `./fixtures` import `{ test, expect, getWailsCalls, openPanel }`。
- per-test 经 `test.use({ wailsOverrides })` 注入：`ListTempTodos`（一条已存待办）、`OpenFileDialog`（固定路径）、`ReadFileBytes`（fixture Markdown 的 base64）。
- fixture Markdown 文件名取 `2020-01-05.md`（历史日期，规避行内时间晚于当前时刻的钳制），内容同时覆盖：行内时间待办、带标签待办、续行多行正文、导出格式已完成行（跳过）、普通列表项（跳过）、与已存待办重复行、文件内自身重复行。
- 用例一：预览汇总「将导入 3 条 / 跳过重复 2 条」；切换「重复项也导入」后「将导入 5 条」且跳过重复块消失；再切回恢复 3/2。
- 用例二：默认（过滤重复）确认导入 → 断言 `AddTempTodos` 入参为 3 条，字段 `text/tags/createdAt` 与 `model.TempTodoDraft` 一致，`createdAt` 为「文件名日期 + 行内时刻」的本地毫秒时间戳。
- 用例三：打开「重复项也导入」后确认 → 入参 5 条（含重复项），验证开关真实作用于写库载荷。

## Acceptance Criteria

- [ ] 新增 spec 文件可被 `npx playwright test e2e/temp-todo-import.spec.js` 单独执行并通过。
- [ ] 三条用例覆盖用户指定的三项断言（汇总数字、开关后计数、`AddTempTodos` 入参）。
- [ ] fixture 形状与 `model.TempTodoDraft`（`text`/`tags`/`createdAt`）严格对齐，不携带前端内部字段（如 `duplicate`）。
- [ ] 全程无 `waitForTimeout`/固定 sleep，仅用 web-first 断言与 `getWailsCalls`。
- [ ] 不修改生产代码；不改动既有 E2E 用例。

## Definition of Done

- 新增 E2E 通过（本地 Chromium）。
- 相关规范文档按需同步（`docs/spec/temp-todo.md` 测试锚点补 E2E 行，若适用）。
- README 无需改动（`e2e/README.md` 已用 `e2e/*.spec.js` 概括文件职责）。

## Technical Approach

- 面板激活走 `openPanel(page, 'todo')`（`data-panel-id` 语义锚），先断言已存待办文本可见，确保 `ListTempTodos` 已 resolve 再点导入。
- 导入按钮 `[data-action="import"]`；预览对话框 `.todo-import-dialog`；开关 `.import-switch`；确认按钮 `[data-action="import-confirm"]`。
- 预览计数断言用 `dialog.locator('.import-summary')` 文本正向断言；开关后「跳过重复」块用 `toHaveCount(0)` 结构性断言。
- `createdAt` 期望值在 spec 内用 `new Date(2020, 0, 5, HH, MM).getTime()` 计算（Node 与浏览器同一本地时区；Playwright 未设 `timezoneId`）。
- 三个 describe 共用同一份 override 基底，仅 `AddTempTodos` 返回值按用例调整（3 / 5），使成功提示与入参一致。

## Decision (ADR-lite)

**Context**：组件单测已覆盖解析与确认链路，但真实浏览器下的「常驻面板挂载 + 文件桥接 + 对话框交互」未被 E2E 守护。
**Decision**：新增独立 spec，以 `wailsOverrides` 替代原生文件对话框与文件读取，不引入真实文件系统依赖。
**Consequences**：E2E 与单测形成双层守护；fixture 日期需避开未来钳制逻辑，注释中说明原因。

## Out of Scope

- 浏览器模式降级提示、空文件提示、全部重复禁用确认等分支（组件单测已覆盖，且不属于本次指定断言）。
- `parseTempTodoMarkdown` 逻辑变更、生产代码修改。
- 真实桌面通道（e2e-desktop）验证。

## Technical Notes

- 相关文件：`frontend/e2e/fixtures.js`、`frontend/e2e/repo-config.spec.js`（同类文件桥接先例）、`frontend/src/components/TempTodoPanel.vue`（模板锚点与 computed）、`frontend/src/utils/tempTodoImport.js`（解析规则）、`model/temp_todo.go`（`TempTodoDraft` 形状）。
- 解析规则：行首 0–3 空格为顶层项；续行恰两空格；行尾 `#标签` 反复剥离；已完成标记与 `[x]` 跳过；重复判定为删空白后全等（现有全部条目 + 文件内前序）。
- 预览计数：`importWritableCount` 随开关变化，`importSkippedCount` 开关打开时恒 0。
