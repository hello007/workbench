# 终端交互修复：URL 默认浏览器与多行粘贴预览

## Goal

修复 11-05-xterm-addons 真机验收发现的两点交互问题：URL 打开落入内嵌 webview、多行粘贴确认框缺内容预览。

## What I already know（调研结论）

| 项 | 现状 | 位置 |
|---|---|---|
| URL 打开 | `openUrlInBrowser` 用 `window.open(uri, '_blank', 'noopener,noreferrer')`——Wails WebView2 环境在内嵌 webview 新开，不走系统默认浏览器 | useTerminal.js:180-182 |
| 仓内标准方案 | `BrowserOpenURL`（wailsjs/runtime/runtime）走系统默认浏览器，AiChatPanel.vue:740 / GitInfo.vue:163 / FilePreviewRenderer.vue:410 三处成熟先例（http(s) 协议前置校验模式） | 同左 |
| 粘贴确认框 | `ElMessageBox.confirm(message, title, opts)` message 为纯字符串，无待粘贴内容预览 | useTerminal.js:289-293 |
| MessageBox VNode | ElMessageBox message 支持 VNode（h() 渲染）；弹层挂 document.body 下，scoped 样式不可达，用 inline style + CSS 变量（设计令牌在 :root 生效） | EP 2.14 能力 |
| 测试 stub 模式 | `vi.mock('wailsjs/runtime/runtime', () => ({ BrowserOpenURL: vi.fn() }))` 既有模式（GitInfo.spec.js:20-22） | 同左 |

## Requirements

### R1 URL 走系统默认浏览器

- `openUrlInBrowser` 改调 `BrowserOpenURL(uri)`（import 自 `wailsjs/runtime/runtime`），删除 `window.open`。
- 协议校验：handler 内前置 `https?://` 校验（对齐 FilePreviewRenderer 先例），非 http(s) 不打开（防任意协议直通）。web-links addon 本身只识别 URL 形态，校验为纵深防御。
- Ctrl/Cmd 修饰键守卫不变。
- serve 模式（浏览器通道）`BrowserOpenURL` 由 transport/events.js:396 桥接等价实现，无新增适配。

### R2 多行粘贴确认框带内容预览

- 确认框 message 改 VNode：确认文案 + 预览块。
- 预览规则：取待粘贴文本前 5 行、单行截 120 字符、总 400 字符上限；超限尾部加省略标记与「共 N 行」摘要行。
- 预览块样式：等宽字体（Cascadia Code 栈，与终端一致）、`var(--bg-tertiary)` 底 + `var(--border-color)` 边 + `var(--text-secondary)` 字，inline style 写入（弹层不受 scoped 影响），预读 `white-space: pre-wrap` 保换行形态。
- 确认后行为不变（`terminal.paste(text)` 原文写入，预览截断不影响实际粘贴内容）。

## Acceptance Criteria

- [ ] 真机 Ctrl+Click URL 打开系统默认浏览器，不再落入内嵌 webview
- [ ] 非 http(s) URI 不触发打开
- [ ] 粘贴多行弹窗显示预览块（≤5 行/400 字符），超限显示省略与总行数
- [ ] 预览块深浅两主题下可读（设计令牌联动）
- [ ] 确认粘贴写入完整原文（预览截断不影响）
- [ ] vitest 通过：BrowserOpenURL stub 断言调用与协议拦截；预览块渲染断言（行数/截断/摘要）
- [ ] `npm run build` 通过

## Definition of Done

- 单测更新（useTerminal.spec.js：URL 打开 stub 断言、粘贴预览 VNode 断言）
- vitest + build 全绿
- docs/spec/terminal-multi-session.md §11 若形成新契约点则补两行（URL 打开出口单点、粘贴预览规则）

### R3 规范沉淀：外链打开出口单点（ADR）

- 新建 `docs/spec/url-opening.md`：ADR 形态（背景 → 决策 → Wrong/Correct）——**外链打开禁 `window.open` / `location.href`，唯一出口 `runtime.BrowserOpenURL`（http(s) 前置校验）**；机理（Wails WebView2 下 window.open 内嵌导航/新 webview，不经系统 shell）下沉该文档，仓内三先例（AiChatPanel/GitInfo/FilePreviewRenderer）列为参照。
- CLAUDE.md 关键规则表补一行（≤250 字符）链接该文档。
- 文档索引：CLAUDE.md 文档索引表与 docs/spec/README.md（若其收录清单机制适用）登记。

## Out of Scope

- 文件路径打开链路不动（OpenWithDefaultApp 本就走系统默认程序）
- 不新增设置项
- 不动 paste 拦截机制（捕获阶段 DOM paste 不变）

## Technical Notes

- `window.open` 在 Wails v2 WebView2 的行为：默认内嵌导航/新 webview，未经后端 shell 分发；`BrowserOpenURL` 经 Wails runtime 调系统默认浏览器，是仓内唯一认可的 URL 出口（transport/events.js 有浏览器通道等价桥）。
- ElMessageBox VNode message 在 EP 2.14 实测可用；样式必须 inline（弹层 teleport 到 body）。
- 粘贴预览提取函数建议抽纯函数（`buildPastePreview(text)` → {lines, truncated, totalLines}）便于直测。
