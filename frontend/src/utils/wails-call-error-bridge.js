/**
 * Wails 调用错误桥（wails v2.12 → v2.16 运行时 reject 行为回归适配）。
 *
 * 根因（真机冒烟「加载幻灯片列表失败: [object Object]」定位链）：
 *  - 桌面端绑定方法失败时，main.go formatAppError（Wails ErrorFormatter）把错误
 *    格式化为结构化对象 {code, message}，经 dispatcher（v2.16
 *    internal/frontend/dispatcher/calls.go：CallbackMessage.Err any，json tag
 *    "error"）序列化进应答 `{"callbackid":"…","error":{…}}`，由 Windows 桥
 *    （frontend.go Callback()）以 `window.wails.Callback(<JSON 字符串>)` 注入页面。
 *  - wails v2.16 runtime 的 Callback()（internal/frontend/runtime/desktop/calls.js
 *    180-183 行，prod bundle 与 dev bundle 同构）对失败应答统一
 *    `new Error(message.error)` 后 reject（上游 issue #4379 的改动，自带测试只
 *    覆盖字符串错误形态）。message.error 经 JSON.parse 必为普通对象，永远不满足
 *    `instanceof Error`，故 `new Error(对象)` 的 message = String(对象) =
 *    "[object Object]"，code 丢失——全应用所有绑定方法的错误提示与 code 分流
 *    （warning/error）同时失效。
 *  - v2.12（升级前版本，2026-09-14 41c4b1a 升至 v2.16）行为是
 *    `reject(message.error)` 原样透传结构化对象；本项目 utils/error.js 的
 *    error.code / error.message 读取契约、WARNING_CODES 分流均基于该语义。
 *
 * 修复点：错误进入点收敛（单点覆盖全部绑定方法调用，非各调用点单补）。wails
 * runtime 在 window.wails 上暴露与 Callback 共享的 pending 回调表 callbacks
 * （desktop/main.js 与 dev bundle 均暴露同一对象）。本桥在应用启动早期包装
 * window.wails.Callback：应答中 error 为结构化对象时，按 runtime 同款收口协议
 * （clearTimeout + delete callbacks）直接以原对象 reject 对应 pending 调用，
 * 恢复 v2.12 语义；其余形态（字符串错误 / 成功应答 / 非法 JSON / 未注册回调）
 * 原样委托 runtime 自身实现，语义逐字不变。
 *
 * 生效面：桌面模式（window.wails 由 wails 注入，先于应用 bundle 执行）。浏览器
 * 访问模式仅 polyfill window.go / window.runtime（transport/index.js），不触碰
 * window.wails，本桥 no-op，浏览器通道错误形态由 transport/rpc.js 自行保证。
 */

/**
 * 安装调用错误桥。幂等（window.wails 上打标记，HMR/重复调用不叠加包装）；
 * window.wails.Callback 或 callbacks 回调表缺失（浏览器模式 / 测试 jsdom /
 * 未来 runtime 改面）时静默不装。
 */
export function installWailsCallErrorBridge() {
  if (typeof window === 'undefined') return
  const wails = window.wails
  if (!wails || typeof wails.Callback !== 'function' || !wails.callbacks) return
  if (wails.__callErrorBridgeInstalled) return
  wails.__callErrorBridgeInstalled = true

  const originalCallback = wails.Callback
  wails.Callback = function (incomingMessage) {
    // 仅接管「error 为结构化对象」形态：这是 ErrorFormatter 产出形态，也是
    // v2.16 new Error(obj) 唯一会破坏信息的形态。解析失败或其余形态全部委托
    // 原实现（非法 JSON 告警、「回调未注册」抛错语义保持原样）。
    try {
      const message = JSON.parse(incomingMessage)
      if (message && message.error !== null && typeof message.error === 'object') {
        const pending = wails.callbacks[message.callbackid]
        if (pending) {
          delete wails.callbacks[message.callbackid]
          if (pending.timeoutHandle) clearTimeout(pending.timeoutHandle)
          pending.reject(message.error) // v2.12 语义：reject 结构化对象本体
          return
        }
      }
    } catch {
      // 应答非合法 JSON（如被截断）：委托原实现走其既有告警与抛错路径
    }
    originalCallback.call(wails, incomingMessage)
  }
}
