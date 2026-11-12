# 提示词面板 Playwright E2E 用例

## Goal

「提示词」侧边栏面板功能已于 2026-11-11 交付，组件单测齐全，但缺少端到端用例。本任务补一条 Playwright E2E，覆盖「新建 → 占位符替换复制 → 原文复制 → 删除」完整用户链路，防止前端装配、活动栏入口、Wails mock 契约与对话框交互在集成层回归。

## Requirements

1. 从 `frontend/e2e/fixtures.js` 导入 `{ test, expect }`，复用既有 Wails mock 注入。
2. 经活动栏语义锚 `[data-panel-id="prompts"]` 打开「提示词」面板，禁止 `nth` 索引定位入口。
3. 新建一条正文含 `{{客户名称}}` 的提示词，保存后列表出现该条目。
4. 点击复制：出现「填写占位符」弹窗与「客户名称」输入框；填写后点「替换并复制」，断言剪贴板内容为替换结果。
5. 再次点击复制：不填写，点「复制原文」，断言剪贴板内容为含 `{{客户名称}}` 的原文。
6. 删除该条目：确认后列表项消失。
7. 断言关键 Wails 调用参数：`AddPrompt` 收到标题与正文原文、`DeletePrompt` 收到条目 ID。
8. 全程使用 `expect` 可见性断言等待，禁止 `waitForTimeout` / 固定 sleep。

## Acceptance Criteria

- [ ] 新增 `frontend/e2e/prompt-library.spec.js`，覆盖上述 1–6 步完整链路。
- [ ] 剪贴板断言读取真实 `navigator.clipboard.readText()` 结果，非 mock 内部记录。
- [ ] 「替换并复制」后剪贴板等于占位符替换后的文本；「复制原文」后等于含 `{{客户名称}}` 的原文。
- [ ] 删除确认弹窗按 `docs/spec/confirm-dialog.md` 规范定位（限定 `.el-message-box` 容器，不裸选全局按钮）。
- [ ] `npm run e2e -- prompt-library.spec.js` 本地通过。
- [ ] `npm test`（vitest）不受影响（用例位于 `e2e/`，已被 `test.exclude` 排除）。
- [ ] `npm run build` 通过。

## Definition of Done

- E2E 用例新增并通过本地运行。
- 相关既有测试（vitest 全量或受影响范围）与前端构建通过。
- 不引入 `waitForTimeout`、不硬编码面板索引、不直连 `@playwright/test`。
- README / spec 仅在约定发生变化时更新；本任务不新增目录，故 vitest `test.exclude` 无需改动。

## Technical Approach

- **mock 状态机**：面板常驻挂载于 `Home.vue`（`v-show`），挂载即调用 `ListPrompts`。用 `test.use({ wailsOverrides })` 给 `ListPrompts` 配 `__rules__` 状态机：
  - `__default__: []`（初始空列表）
  - `after: 'AddPrompt' → [PROMPT]`
  - `after: 'DeletePrompt' → []`
  规则按声明序评估、后命中覆盖前命中，删除时两规则同时命中、后声明的删除态生效，与调用次数无关。
- **剪贴板权限**：`test.use({ permissions: ['clipboard-read', 'clipboard-write'] })`，在 `localhost` 安全上下文下读取真实剪贴板。
- **定位**：面板 `.prompt-panel`、新建 `[data-action="create"]`、编辑弹窗按标题「新建提示词」过滤、输入 `[data-field="title"]` / `[data-field="content"] textarea`、保存 `[data-action="save"]`、条目 `[data-prompt-id="prompt-e2e"]`、复制 `[data-action="copy"]`、占位符输入 `[data-fill-name="客户名称"] textarea`、替换复制 `[data-action="copy-filled"]`、原文复制 `[data-action="copy-original"]`。
- **删除确认**：`page.locator('.el-message-box').getByRole('button', { name: '删除' })`，规避 EP 关闭后实例驻留 DOM 带来的选择器歧义。
- **mock 数据源**：`src/test/wails-mock-defaults.js` 已含 `ListPrompts` / `AddPrompt` / `DeletePrompt` 条目，形状对齐 `model.Prompt`；本用例的动态状态经用例级 `wailsOverrides` 表达，不修改默认表，避免影响其他用例。

## Decision (ADR-lite)

**Context**：提示词面板已有完整单测，缺少跨「活动栏 → 面板装配 → Wails 契约 → 浏览器剪贴板」的集成验证。
**Decision**：新增独立 E2E spec，以规则状态机表达列表增删，真实读取剪贴板断言复制结果。
**Consequences**：用例依赖 Chromium 剪贴板权限；若浏览器权限策略变化，需改为在页面内拦截 `navigator.clipboard.writeText`，但会降低断言真实性，暂不采用。

## Out of Scope

- 编辑（`UpdatePrompt`）链路、搜索过滤、长正文展开/收起。
- 后端 service 单测与组件单测补强（已覆盖）。
- 浏览器通道（serve 模式）下的提示词面板验证。
- 修改 `wails-mock-defaults.js` 默认值（本任务不需要）。

## Technical Notes

- 面板实现：`frontend/src/components/PromptLibraryPanel.vue`
- 占位符替换：`frontend/src/utils/promptPlaceholders.js`（留空保留原样）
- 活动栏语义锚：`frontend/src/components/ActivityBar.vue`（`data-panel-id`）
- 挂载点：`frontend/src/views/Home.vue`（`v-show="uiStore.activePanel === 'prompts'"`）
- 相关契约：[`docs/spec/e2e-testing.md`](../../../docs/spec/e2e-testing.md)、[`docs/spec/confirm-dialog.md`](../../../docs/spec/confirm-dialog.md)、[`docs/spec/prompt-library.md`](../../../docs/spec/prompt-library.md)
- vitest `exclude` 已包含 `e2e/**`（`frontend/vitest.config.js`），新增 spec 文件无需调整。
