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
 * - { __value__: v, __events__: [{ event, payload, delayMs? }] }：resolve __value__
 *   （缺省 null），并按序派发 Wails 事件（模拟 AI 任务 queued/started/output/done
 *   等异步事件流）。每次调用都完整派发（无状态消费），delayMs 缺省 0。
 *
 * 调用记录：每次 bound method 调用都会 push 到 window.__wailsCalls
 * （{method, args}），用例经 fixtures.js 的 getWailsCalls 断言「UI 操作触发了
 * 正确的 Wails 调用与参数」。
 *
 * 事件注册：window.runtime.EventsOn/EventsOnMultiple 将回调登记到
 * window.__wailsEventHandlers（按事件名分组），__events__ 派发时逐一调用，
 * EventsOff 移除该事件全部回调——语义对齐 Wails runtime。
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
        if (Array.isArray(value.__events__)) {
          // 模拟异步事件流：resolve __value__ 后按序经 setTimeout 派发（保持异步时序，
          // delayMs 支持用例编排「运行中→完成」等中间态断言窗口）
          value.__events__.forEach((ev) => {
            setTimeout(() => {
              const handlers = (window.__wailsEventHandlers || {})[ev.event] || []
              handlers.forEach((handler) => handler(ev.payload))
            }, ev.delayMs || 0)
          })
          return Promise.resolve(value.__value__ !== undefined ? value.__value__ : null)
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
  // BrowserOpenURL 等，见 wailsjs/runtime/runtime.js 转发表）。
  // EventsOn/EventsOnMultiple/EventsOnce 将回调登记到 window.__wailsEventHandlers
  // （按事件名分组），供 __events__ 描述符派发；EventsOff 移除该事件全部回调。
  window.__wailsEventHandlers = Object.create(null)
  const registerHandler = (name, callback, once) => {
    const handlers = (window.__wailsEventHandlers[name] = window.__wailsEventHandlers[name] || [])
    handlers.push(once ? (...args) => {
      const idx = handlers.indexOf(wrapped)
      if (idx >= 0) handlers.splice(idx, 1)
      callback(...args)
    } : callback)
    // once 包装后的函数名占位：闭包内自引用，splice 时按引用定位
    const wrapped = handlers[handlers.length - 1]
  }
  window.runtime = {
    EventsOn: (name, callback) => registerHandler(name, callback, false),
    EventsOnMultiple: (name, callback) => registerHandler(name, callback, false),
    EventsOnce: (name, callback) => registerHandler(name, callback, true),
    EventsOff: (name) => {
      delete window.__wailsEventHandlers[name]
    },
    EventsOffAll: () => {
      window.__wailsEventHandlers = Object.create(null)
    },
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
