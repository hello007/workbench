---
title: 网络层与路由规范（transport 双通道）
scope: [api, integration, router]
targets: [api, transport, router, util]
---

# 网络层与路由规范（transport 双通道）

> **依赖定界**：本仓前端无 axios/fetch 直连业务后端的 HTTP 客户端；对后端的数据面调用走 **transport 层双通道**（桌面模式 = Wails 绑定；浏览器访问模式 = WebSocket + HTTP RPC）。路由使用已显式声明的 `vue-router`。

## 1. transport 层统一收口

- 所有后端调用统一经 `frontend/src/transport/`（`rpc.js`、`events.js`、`dom.js`、`token.js` 等）封装，禁止在页面或组件中绕过 transport 直连通道实现。
- **桌面 / 浏览器双模式分流**：通道实现必须在应用启动前安装（`installBrowserTransport()` 在 `createApp`/`mount` 前调用），模式判定与 polyfill 注入顺序不得调整——静态 import 的模块不得在模块求值期触碰 `window.go` / `window.runtime`。
- 通道级横切能力（错误桥、令牌门控、连接横幅）在 transport 层统一实现，业务层只消费，禁止在组件内重复实现通道探测。

## 2. 事件订阅规范

- 组件监听后端事件必须持有 `EventsOn` 返回的注销闭包，在组件卸载或 repoPath 切换时精准注销；禁止使用 `EventsOff('eventName')` 全局移除（会误杀其他组件的同名监听）。

## 3. Vue Router 路由设计规范

- **组件懒加载**：页面路由组件采用动态导入 `component: () => import(...)`，严禁在路由表静态全量导入视图页面。
- **元信息扩展**：路由元信息统一通过 `meta` 字段声明，便于路由守卫统一处理。
- **全局守卫规范**：全局前置守卫（`router.beforeEach`）逻辑保持简明，避免复杂异步等待导致白屏。
