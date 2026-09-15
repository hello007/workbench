# 推送结果面板

## Goal

当前 `PushRepo` 已返回 git stdout，但前端 `LocalChanges.vue` 仅以 `ElMessage.success` toast 展示，超 200 字符截断丢信息。需补独立结果面板，长输出完整展示，闭环 roadmap「查看推送结果」边缘项。对齐既有 Pull 结果弹窗范式（`ContentPanel.vue` `singlePullResult` ElDialog）。

## Requirements

- 推送成功且 output.length > 200：弹 ElDialog 完整展示 git stdout，不截断。
- output.length ≤ 200 或空：保留 `ElMessage.success` toast（与 Pull 200 阈值范式一致，短输出不打断流）。
- 面板内容：纯原始 stdout `<pre>` 等宽文本 + 复制按钮（`navigator.clipboard.writeText`）。
- 面板形态：独立 `PushResultDialog.vue` 组件，`v-model` 控制可见性，`output` prop 传入。
- 宿主 `LocalChanges.vue` 引入组件，`doPush` 成功分支按阈值设 ref。
- 推送失败仍走 `handleGitError` toast（错误链路不变）。
- 提交并推送流同样适用（commit → push 成功 → 阈值判断弹面板）。

## Acceptance Criteria

- [ ] 长输出（>200 字符）推送成功后弹 Dialog，完整可见无截断。
- [ ] 短输出（≤200）与空输出走 toast，行为同现状不回归。
- [ ] 复制按钮点击后剪贴板含完整 output。
- [ ] Dialog 关闭按钮可用，`append-to-body` 遮罩正确。
- [ ] 现有 6 个推送测试用例不回归；新增长输出弹面板 + 复制按钮用例。

## Definition of Done

- 前端单测覆盖：长输出弹 Dialog / 短输出 toast / 复制按钮 / Dialog 关闭。
- Lint / typecheck / CI 绿。
- `docs/功能说明.md` Git 集成章节补推送结果说明；`docs/路线图.md` 推送结果项勾选。

## Technical Approach

**复用 Pull 范式，独立组件化**：
- 新增 `frontend/src/components/PushResultDialog.vue`：ElDialog + `<pre class="push-result-output">` + 复制按钮（ElButton + `CopyDocument` icon）。
- 样式复用 Pull 的 `.pull-result-output`（max-height 400px overflow-y auto，等宽）。复制按钮为 Push 独有增强。
- `LocalChanges.vue`：加 `pushResultVisible` ref + `pushResultOutput` ref，`doPush` 成功分支 `if (text.length > 200) { pushResultOutput.value = text; pushResultVisible.value = true }` 替代现有 `ElMessage.success(text.slice(0,200)+'...')`。
- mock 数据源 `wails-mock-defaults.js:138` `PushRepo` 保留现有多行 output 样例。

**偏离 Pull parity 声明**：
- Pull 用内联 ElDialog（`ContentPanel.vue` template 内）；Push 用独立组件文件（预留推送历史扩展入口）。
- Pull 无复制按钮；Push 加复制按钮（低成本增强，长输出场景用户需拷贝诊断信息）。

## Decision (ADR-lite)

**Context**: Push 长输出被 toast 截断；需结果展示闭环。Pull 已有 200 阈值 ElDialog 范式可参照。

**Decision**: 对齐 Pull 200 阈值模式（短 toast / 长面板），但用独立 `PushResultDialog.vue` 组件而非内联，并加复制按钮。

**Consequences**: 与 Pull 实现细节非完全对称（独立文件 + 复制按钮），但交互范式一致（阈值 + ElDialog + pre）。预留历史扩展点。后续若 Pull 也需独立组件化可统一。

## Out of Scope

- 推送失败错误面板（错误仍走 handleError toast）。
- 推送历史记录归档（仅即时结果，独立组件预留入口不实现）。
- 推送进度流式展示（git push 非交互式 stdout 一次性返回）。
- 结构化解析 ref 行（`main -> main` 拆表）— 留后续。
- 后端 Push 签名调整。
- Fetch 结果面板（Fetch 现状 toast，parity 需求未提）。

## Technical Notes

- 后端：`service/git.go:616` `Push(repoPath, setUpstream) (string, error)` / `app_git.go:599` `PushRepo`。
- 前端宿主：`frontend/src/components/LocalChanges.vue:355-392` `doPush`（现有 200 截断 toast 在 379-384）。
- Pull 参照范式：`frontend/src/components/ContentPanel.vue:348-359`（Dialog 模板）/ `:588-589`（ref）/ `:661-667`（阈值逻辑）/ `:1422-1425`（`.pull-result-output` 样式）。
- mock：`frontend/src/test/wails-mock-defaults.js:138`。
- 测试：`frontend/src/components/__tests__/LocalChanges.spec.js`（现有 6 用例，`推送超长输出截断展示` 需改断言为弹 Dialog）。
- spec：`docs/spec/logging-and-errors.md`（错误分流）、`docs/spec/cross-layer-contracts.md`（Wails 绑定，本任务后端无改不触发）。
