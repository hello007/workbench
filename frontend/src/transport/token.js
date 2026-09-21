/**
 * 浏览器访问模式访问令牌单点存取。
 *
 * serve 模式后端为每个进程生成随机令牌（data/web_token，见 server/token.go），
 * 浏览器首访经 token 输入门（tokenGate.js）取得后持久化到 localStorage，
 * 后续 RPC 请求带 Authorization: Bearer、WS 握手经 Sec-WebSocket-Protocol
 * 子协议携带。本模块是前端令牌读写唯一入口，rpc.js / events.js / tokenGate.js
 * 均经由它存取，禁止各自直碰 localStorage。
 *
 * localStorage 不可用（隐私模式/配额限制）时降级为模块内存态：当前页面内
 * 令牌仍有效（tokenGate 已持有值），仅刷新后需重新输入。
 *
 * 安全评估（XSS 面）：令牌存 localStorage 可被同源脚本读取，但本服务全部静态
 * 资产自托管、页面无第三方脚本注入面；且同源脚本一旦被注入即等价于拿到页面
 * 会话，可直接发起同样的 RPC 调用，localStorage 不构成额外提权面。传输侧不把
 * 令牌放进 URL（RPC 走 Authorization 头、WS 走 Sec-WebSocket-Protocol 子协议），
 * 规避浏览器历史/referer/服务端访问日志的泄漏通道。
 */

// localStorage 键名。令牌为机器级凭据，非用户身份，键名不带用户维度。
const STORAGE_KEY = 'workbench.web.token'

// 存储不可用时的内存兜底（模块级单例，页面生命周期内有效）。
let memoryToken = ''

/**
 * 读取当前访问令牌；未设置时返回空串。
 * @returns {string}
 */
export function getToken() {
  if (memoryToken) return memoryToken
  try {
    return window.localStorage.getItem(STORAGE_KEY) ?? ''
  } catch {
    // localStorage 读取异常（隐私模式等）：按未设置处理
    return ''
  }
}

/**
 * 持久化访问令牌（token 输入门提交时调用）。
 * @param {string} token 用户输入的令牌（调用方保证非空）
 */
export function setToken(token) {
  memoryToken = token
  try {
    window.localStorage.setItem(STORAGE_KEY, token)
  } catch {
    // 写入失败仅降级为内存态，不阻塞当前会话
  }
}

/**
 * 清除令牌（当前无调用方，供令牌重置/登出场景预留）。
 */
export function clearToken() {
  memoryToken = ''
  try {
    window.localStorage.removeItem(STORAGE_KEY)
  } catch {
    // 读取与写入同源异常，清除失败不抛出
  }
}
