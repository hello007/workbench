/**
 * 浏览器访问模式 transport 装配入口。
 *
 * 运行时探测：存在 Wails runtime（window.go.main.App）→ 桌面模式，什么都不装，
 * 行为零变化；否则视为浏览器（serve）模式，在 app.mount 前注入 polyfill：
 *   - window.go.main.App ← RPC Proxy（任意方法名 → POST /api/rpc）；
 *   - window.runtime      ← 事件桥（EventsOn 等 API 面 → WebSocket /ws）。
 *
 * 时序保证：wailsjs 生成 wrapper（App.js / runtime.js）均在方法调用时才解引用
 * window.go / window.runtime（非模块求值期），组件亦在 setup/onMounted 阶段才
 * 调 EventsOn，因此 main.js 在 createApp/mount 前调用本函数即可全链路生效。
 * main.js 的静态 import 会先于本函数执行完成，但被 import 的模块均不在模块
 * 求值期触碰 window.go/window.runtime（已逐一核对），顺序安全。
 */

import { createRpcProxy } from './rpc'
import { createEventBridge } from './events'

/**
 * 探测是否处于 Wails 桌面环境（生成绑定已注入 window.go.main.App）。
 * @returns {boolean}
 */
export function hasWailsBindings() {
  return typeof window !== 'undefined' && typeof window.go?.main?.App !== 'undefined'
}

/**
 * 浏览器模式 polyfill 安装入口（main.js 最早时机调用）。
 * @returns {boolean} 是否执行了安装（false = 桌面模式 no-op）
 */
export function installBrowserTransport() {
  if (hasWailsBindings()) return false
  window.go = { main: { App: createRpcProxy() } }
  window.runtime = createEventBridge({ wsUrl: buildWsUrl(window.location) })
  return true
}

/**
 * 由页面地址推导 WS 端点（serve 模式前端与 WS 同源，见 server/web.go）。
 * 抽出 location 参数便于覆盖 https→wss 分支的测试。
 * @param {{protocol: string, host: string}} location 形似 window.location 的对象
 * @returns {string}
 */
export function buildWsUrl(location) {
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${location.host}/ws`
}
