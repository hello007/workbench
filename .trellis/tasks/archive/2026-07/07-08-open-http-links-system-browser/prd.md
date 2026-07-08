# 使用系统默认浏览器打开 http 链接

## Goal

应用内点击 http(s) 链接（如 Git 仓库远程地址 github.com 的 http url）时，当前在 Wails 内置 webview 中打开，体验差且无法复用用户浏览器会话。改为调用系统默认浏览器打开，并将该规则沉淀为项目开发规范，避免未来重现反模式。

## Requirements

- GitInfo 仓库远程地址（http(s)）点击后由系统默认浏览器打开，不再在 webview 内打开。
- 复用 `FilePreviewRenderer` 已验证范式：`import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'`，`@click` 触发。
- 在 `docs/开发规范.md` 新增"前端交互-外部链接打开"小节，记录规则与反面模式。
- 补充 `GitInfo.spec.js` 点击行为单测。

## Acceptance Criteria

- [ ] 点击 GitInfo 中 http(s) 仓库地址，系统默认浏览器打开对应 URL，应用内 webview 不发生导航。
- [ ] ssh/git 协议远程地址仍显示为纯文本，点击无副作用。
- [ ] `GitInfo.spec.js` 覆盖：http(s) 地址点击调用 `BrowserOpenURL`；非 http 地址不触发。
- [ ] `cd frontend && npm test` 通过。
- [ ] `wails build` 构建通过。
- [ ] `docs/开发规范.md` 新增"外部链接打开"规范小节。
- [ ] 确认 `README.md` 是否需更新（按项目约定）。

## Definition of Done

- 单测更新并通过（`cd frontend && npm test`）
- `wails build` 构建通过
- 规范文档更新
- 回归 markdown 预览外链打开未受影响

## Technical Approach

**代码改动（`frontend/src/components/GitInfo.vue`）**：

- `el-link` 去掉 `:href` 与 `target="_blank"`，改为 `@click="openRemoteUrl(gitInfo.remoteUrl)"`。
- `import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'`。
- 新增 `openRemoteUrl(url)`：判空 + `isHttpUrl` 校验后调 `BrowserOpenURL(url)`。
- 保留 `isHttpUrl` 控制 `v-if`（非 http 仍渲染纯文本 span）。

**规范沉淀（`docs/开发规范.md`）**：在"前端错误处理"后新增"### 外部链接打开"小节，含规则、反面模式、示例代码。

## Decision (ADR-lite)

**Context**：`GitInfo` 仓库地址用 `el-link :href target="_blank"` 导致 webview 内打开；`FilePreviewRenderer` 已有 `BrowserOpenURL` 正确范式但未抽象。

**Decision**：仅修 `GitInfo` 就地复用范式，不抽取通用工具函数（YAGNI——当前两处语义不同：`FilePreviewRenderer` 含多协议判断 + 相对路径解析，`GitInfo` 仅 http(s)）；规范沉淀到 `docs/开发规范.md`（项目开发规范权威位置，人与 AI 均可参考）。

**Consequences**：未来若新增 commit/PR 链接需手动复用范式；规范文档约束未来不再出现 webview 打开外链的反模式。`CommandPalette` 的 `vscode://` 协议暂不在范围内。

## Out of Scope

- 抽取通用 `openExternalUrl` 工具函数（当前两处语义不同，YAGNI）。
- `CommandPalette.vue` 的 `vscode://` 协议 `window.open`（非 http，本次不处理）。
- `FilePreviewRenderer` 已正确处理，不改动。

## Technical Notes

- 参照范式：`FilePreviewRenderer.vue:221` import，`:815` `BrowserOpenURL(href)`。
- 单测 mock 范式：`FilePreviewRenderer.spec.js:29-31` `BrowserOpenURL: vi.fn()`。
- 历史 related task：`.trellis/tasks/archive/2026-07/07-06-fix-md-relative-link/prd.md`（markdown 相对/外部链接处理）。
