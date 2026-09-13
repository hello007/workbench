/**
 * Playwright E2E 的 Wails mock 注入函数。
 *
 * 本模块导出的 injectWailsMocks 会被 Playwright addInitScript 序列化后在
 * 浏览器页面上下文执行（早于应用任何脚本），因此函数体必须自包含：
 * 只能访问入参 returnValues 与浏览器全局，不得引用本文件的其他导入。
 *
 * 真实 Wails 桌面环境由 Go 侧注入两个页面全局对象：
 * - window.go.main.App —— bound method 容器（frontend/wailsjs/go/main/App.js
 *   的每个导出函数都转发到 window['go']['main']['App']['<方法名>']）
 * - window.runtime     —— Wails runtime API（frontend/wailsjs/runtime/runtime.js
 *   的 EventsOn / BrowserOpenURL 等都转发到 window.runtime.<方法名>）
 *
 * vite preview 纯浏览器环境两者均缺失，任何常驻组件（Home.vue 挂载的
 * ActivityBar / FileTreePanel / ContentPanel / AiFunctionPanel 等）在
 * setup / onMounted 同步调用即抛 TypeError，首页渲染中断，故必须补齐。
 *
 * 返回值描述符（returnValues 的 value 形态）：
 * - 纯 JSON 值（含数组）：每次调用 resolve 该值
 * - { __error__: string, __code__?: string }：reject。带 __code__ 时 reject 结构化
 *   {code, message}（对齐 Wails ErrorFormatter 的 AppError 形态），否则 reject Error
 * - { __sequence__: [v1, v2, ...] }：按序返回，序列耗尽后恒返回最后一个值
 *
 * 调用记录：每次 bound method 调用都会 push 到 window.__wailsCalls
 * （{method, args}），用例经 fixtures.js 的 getWailsCalls 断言「UI 操作触发了
 * 正确的 Wails 调用与参数」。
 */

/**
 * 在页面上下文注入 Wails mock。
 * @param {Record<string, unknown>} returnValues 方法名 -> 默认返回值/描述符（纯 JSON，可结构化克隆）
 */
export function injectWailsMocks(returnValues) {
  // bound method 调用记录（按时间序），供用例断言调用链
  window.__wailsCalls = []

  /**
   * 为指定方法名生成 mock 实现：记录调用 + 按描述符决定 resolve/reject。
   * @param {string} method
   */
  const methodFor = (method) => {
    const hasOwn = Object.prototype.hasOwnProperty.call(returnValues, method)
    const value = hasOwn ? returnValues[method] : null
    return (...args) => {
      window.__wailsCalls.push({ method: method, args: args })
      if (
        value && typeof value === 'object' && !Array.isArray(value)
      ) {
        if (value.__error__ !== undefined) {
          const err = value.__code__ !== undefined
            ? { code: value.__code__, message: value.__error__ }
            : new Error(value.__error__)
          return Promise.reject(err)
        }
        if (Array.isArray(value.__sequence__)) {
          const seq = value.__sequence__
          // 序列耗尽后恒返回最后一个值（hold last），空序列回退 null
          const next = seq.length > 1 ? seq.shift() : (seq[0] === undefined ? null : seq[0])
          return Promise.resolve(next)
        }
      }
      return Promise.resolve(value)
    }
  }

  // 登记表内方法 + Proxy 兜底表外方法（同样记录调用、resolve null：
  // 新增组件引用未登记方法时链路不断裂，只需在 wails-mock-defaults.js
  // 为影响 UI 断言的方法补具体返回值）
  const methodCache = Object.create(null)
  for (const method of Object.keys(returnValues)) {
    methodCache[method] = methodFor(method)
  }
  const appProxy = new Proxy(methodCache, {
    get(target, prop) {
      if (typeof prop !== 'string') return undefined
      if (!Object.prototype.hasOwnProperty.call(target, prop)) {
        target[prop] = methodFor(prop)
      }
      return target[prop]
    }
  })
  window.go = { main: { App: appProxy } }

  // window.runtime stub：覆盖前端实际消费的 runtime API（EventsOn/EventsOff/
  // BrowserOpenURL 等，见 wailsjs/runtime/runtime.js 转发表）
  window.runtime = {
    EventsOn: () => {},
    EventsOnMultiple: () => {},
    EventsOnce: () => {},
    EventsOff: () => {},
    EventsOffAll: () => {},
    EventsEmit: () => {},
    BrowserOpenURL: () => {},
    WindowReload: () => {},
    LogPrint: () => {},
    LogTrace: () => {},
    LogDebug: () => {},
    LogInfo: () => {},
    LogWarning: () => {},
    LogError: () => {}
  }
}
