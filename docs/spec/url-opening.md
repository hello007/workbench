# 外链打开出口单点（ADR）

> 适用范围：前端所有「在系统默认浏览器打开 URL」的消费点。来源：任务 11-05-url（2026-11-05，真机验收发现终端 Ctrl+Click URL 落入内嵌 webview）。
> 关联：[terminal-multi-session.md](terminal-multi-session.md) §11.6（终端消费点契约）。

## 背景

终端 WebLinksAddon 最初用 `window.open(uri, '_blank', 'noopener,noreferrer')` 打开 URL。**Wails v2 WebView2 环境下 `window.open` 产生的是内嵌导航/新 webview 窗口，不经系统 shell 分发**——用户浏览器会话（登录态、插件）不可复用，WebView2 内嵌窗口无地址栏，体验割裂。仓内 GitInfo / FilePreviewRenderer / AiChatPanel 三处早已用 `runtime.BrowserOpenURL`（系统默认浏览器），终端是漏网点。

## 决策

**http(s) 外链打开禁 `window.open` / `location.href`，唯一出口 `runtime.BrowserOpenURL`，出口函数内做 http(s) 前置校验（`/^https?:\/\//i`）。**

- `BrowserOpenURL` 经 Wails runtime 调系统 shell 默认浏览器，桌面模式直通系统浏览器；
- serve 模式（浏览器通道）由 `frontend/src/transport/events.js` 的 bridge 等价实现（内部 window.open 新标签页——浏览器环境里「系统默认浏览器」就是浏览器自身，语义等价，属出口实现本身、豁免本 ADR 禁令）；
- http(s) 前置校验对齐 FilePreviewRenderer 先例，防任意协议直通（web-links addon 本身只识别 URL 形态，校验为纵深防御）；
- 已知例外：`vscode://file/...` 深链（CommandPalette「在编辑器打开」）为非 http(s) 深链场景，暂未收口，迁移时同样可走 `BrowserOpenURL`（OS opener 承载任意协议），后续任务处理。

## Wrong / Correct

Wrong（useTerminal.js 旧实现，已删）：

```javascript
function openUrlInBrowser(uri) {
  window.open(uri, '_blank', 'noopener,noreferrer')  // Wails WebView2 下内嵌打开
}
```

Correct：

```javascript
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

function openUrlInBrowser(uri) {
  if (!/^https?:\/\//i.test(uri)) return
  BrowserOpenURL(uri)
}
```

## 仓内参照（先例）

| 消费点 | 位置 | 校验形态 |
|---|---|---|
| AI 对话 markdown 外链 | AiChatPanel.vue（`handleLinkClick`） | `isExternalHref` 前置校验 |
| Git 仓库远程地址 | GitInfo.vue（`openRemoteUrl`） | `isHttpUrl` 前置校验 |
| HTML 预览 iframe postMessage 外链 | FilePreviewRenderer.vue（`onHtmlFrameMessage`） | `/^https?:\/\//i` 前置校验 |
| 终端 Ctrl+Click URL | useTerminal.js `openUrlInBrowser`（本 ADR 收口点） | `/^https?:\/\//i` 出口内守卫 |

## 回归必查

- 新增 URL 打开消费点 grep `window.open` / `location.href` 应零命中（`transport/events.js` serve 桥除外）；
- 非 http(s) URI（`file:` / `javascript:` / `vscode:` 等）不触发 `BrowserOpenURL`；
- 单测 stub：`vi.mock('wailsjs/runtime/runtime', () => ({ BrowserOpenURL: vi.fn() }))`（参照 GitInfo.spec.js），断言调用参数与协议拦截。
