/**
 * 浏览器模式事件通道：实现 window.runtime 的事件 API 面，经 WebSocket /ws
 * 接收后端广播（帧格式 {"event":"<名>","data":[...]}，见 server/ws.go）。
 *
 * 对齐 Wails v2 desktop runtime 语义（组件调用方式零改动）：
 *   - EventsOnMultiple(name, cb, max) 返回注销闭包（精准移除本监听器）；
 *     max=-1 不限次数，max=1 即 EventsOnce（触发一次后自动移除）；
 *   - EventsOn(name, cb) = EventsOnMultiple(name, cb, -1)；
 *   - EventsOff(name, ...names) 按名移除该事件全部监听器（全局语义，
 *     业务组件侧以 EventsOn 闭包精准注销为主，见 cross-layer-contracts.md）；
 *   - 额外对齐 BrowserOpenURL（GitInfo/FilePreviewRenderer 实际使用），
 *     浏览器下等价为新标签页打开。
 *
 * 连接管理：
 *   - 令牌经 Sec-WebSocket-Protocol 子协议携带（new WebSocket(url, [token])），
 *     与 server/ws.go 约定一致（服务端以 upgrader.Subprotocols 回显选中子协议）；
 *   - 注册表常驻：断线自动重连（指数退避，1s 起步、2 倍递增、30s 封顶），
 *     服务端向全部客户端广播，重连成功后事件自然恢复分发，无需重发订阅；
 *   - 首次连接（页面生命周期内从未 open 成功）失败视为令牌缺失/失效，
 *     弹 token 输入门重试；已成功连接过的断线走静默退避重连（横幅提示，
 *     见 connBanner.js），连续失败达 3 次后终止静默循环，弹 token 门重新
 *     认证并横幅提示服务不可达；门被取消则停止重连，之后任意 EventsOn
 *     注册或 RPC 401 弹门可再次拉起认证；若服务端轮换令牌，RPC 401 门
 *     会更新令牌，下一次重连即用新值；
 *   - 建连前校验令牌为 64 位 hex（服务端签发形态），非法令牌直接进 token
 *     门重输，避免非法字符作为 WebSocket 子协议同步抛 SyntaxError；
 *   - 无监听者时不重连，下次 EventsOn 注册时唤醒。
 */

import { getToken } from './token'
import { requestToken } from './tokenGate'
import { notifyConnected, notifyDisconnected, notifyUnreachable } from './connBanner'

// 重连退避参数：基础 1s，指数递增，封顶 30s。
const RECONNECT_BASE_DELAY_MS = 1000
const RECONNECT_MAX_DELAY_MS = 30000
// 连续重连失败次数上限：达到后终止静默退避循环，转 token 门重新认证。
const RECONNECT_FAIL_LIMIT = 3
// 服务端签发的访问令牌为 32 字节 hex（64 字符，见 server/token.go GenerateToken）。
// 建连前校验：无令牌或形态非法时不发起 WebSocket——非法字符作为子协议传入
// 会使 new WebSocket 同步抛 SyntaxError 未捕获，杀死重连链。
const TOKEN_PATTERN = /^[0-9a-f]{64}$/i

/**
 * 创建事件桥实例。index.js 以默认参数创建单例挂到 window.runtime；
 * 工厂形态便于测试隔离（每个用例独立 registry 与连接状态）。
 * @param {{wsUrl: string}} options WS 端点地址（ws(s)://host/ws）
 */
export function createEventBridge({ wsUrl }) {
  // 事件注册表：事件名 → 监听记录 Set。常驻不随断线清空（重连重订阅语义）。
  const registry = new Map()
  // 连接状态监听者（onConnectionChange 注册），状态翻转时通知。
  const connListeners = new Set()
  let socket = null
  // 正在等待 token 输入门结果（防止重复弹门/重复建连）。
  let pendingGate = false
  let reconnectTimer = null
  let reconnectAttempts = 0
  // 页面生命周期内是否成功 open 过（首次连接失败走 token 门、其后走静默重连的分界）。
  let everOpened = false

  /**
   * 确保建立 WS 连接（已连接/连接中/等待令牌时为 no-op）。
   */
  function ensureConnection() {
    if (socket || pendingGate) return
    connect()
  }

  /**
   * 发起一次 WS 连接。无令牌时先经 token 门取得再连。
   */
  function connect() {
    if (socket || pendingGate) return
    const token = getToken()
    if (!TOKEN_PATTERN.test(token)) {
      // 无令牌或令牌非服务端签发的 64 位 hex 形态：经 token 门取得/重输
      waitGateThenConnect()
      return
    }
    socket = new WebSocket(wsUrl, [token])
    socket.onopen = () => {
      everOpened = true
      reconnectAttempts = 0
      notifyConnected()
      emitState(true)
    }
    socket.onmessage = (event) => {
      let frame
      try {
        frame = JSON.parse(event.data)
      } catch {
        return
      }
      if (!frame || typeof frame.event !== 'string') return
      dispatch(frame.event, Array.isArray(frame.data) ? frame.data : [])
    }
    socket.onclose = () => {
      socket = null
      notifyDisconnected()
      emitState(false)
      if (!everOpened && getToken()) {
        // 首次握手未成功即断开：典型为令牌缺失/失效，经门重试
        waitGateThenConnect()
        return
      }
      if (reconnectAttempts >= RECONNECT_FAIL_LIMIT) {
        // 连续重连失败达阈值：终止静默退避循环，转 token 门重新认证，
        // 并经横幅给出「服务不可达」解释（终结无提示的无限重连）
        reconnectAttempts = 0
        notifyUnreachable()
        waitGateThenConnect()
        return
      }
      scheduleReconnect()
    }
  }

  /**
   * 弹 token 门（模块级去重），取得令牌后重建连接；用户取消时停止重连。
   */
  function waitGateThenConnect() {
    if (pendingGate) return
    pendingGate = true
    requestToken().then(() => {
      pendingGate = false
      // 门取得新令牌后失败计数清零，重新获得完整退避重连机会
      reconnectAttempts = 0
      connect()
    }).catch(() => {
      // 用户取消令牌输入：解除挂起态、停掉未决退避定时器并保留不可达横幅，
      // 终结静默重连循环；之后任一组件 EventsOn 注册或 RPC 401 弹门可再次拉起认证
      pendingGate = false
      clearTimeout(reconnectTimer)
      reconnectTimer = null
      notifyUnreachable()
    })
  }

  /**
   * 按指数退避调度静默重连。无监听者时不重连（下次注册时唤醒）。
   */
  function scheduleReconnect() {
    if (reconnectTimer !== null) return
    if (registry.size === 0) return
    const delay = Math.min(RECONNECT_BASE_DELAY_MS * 2 ** reconnectAttempts, RECONNECT_MAX_DELAY_MS)
    reconnectAttempts++
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      // 退避等待期间监听者可能已全部注销，触发时再判一次，
      // 守卫「无监听者不建连」契约（下次注册时唤醒）
      if (registry.size === 0) return
      connect()
    }, delay)
  }

  /**
   * 分发一帧事件给注册表中的监听者，回调以 ...data 变参形态触发
   * （与桌面 EventsEmit 推送、E2E __events__ 派发形态一致）。
   * @param {string} name 事件名
   * @param {unknown[]} data 帧数据数组
   */
  function dispatch(name, data) {
    const records = registry.get(name)
    if (!records) return
    for (const record of [...records]) {
      if (record.removed) continue
      record.callback(...data)
      if (record.remaining > 0) {
        record.remaining--
        if (record.remaining === 0) {
          // EventsOnce 等有限次监听触发完毕后自动移除
          removeRecord(name, records, record)
        }
      }
    }
  }

  /**
   * 移除单条监听记录；事件名的记录集为空时连键一并移除，
   * 保证「无任何监听者」判定（静默重连守卫）准确。
   */
  function removeRecord(name, records, record) {
    record.removed = true
    records.delete(record)
    if (records.size === 0 && registry.get(name) === records) {
      registry.delete(name)
    }
  }

  /**
   * 注册监听器（对齐 Wails EventsOnMultiple 语义），返回精准注销闭包。
   * @param {string} name 事件名
   * @param {(...data: unknown[]) => void} callback 回调
   * @param {number} maxCallbacks 最大触发次数（-1 不限）
   * @returns {() => void} 注销闭包（仅移除本监听器）
   */
  function onMultiple(name, callback, maxCallbacks) {
    if (typeof callback !== 'function') return () => {}
    let records = registry.get(name)
    if (!records) {
      records = new Set()
      registry.set(name, records)
    }
    const record = { callback, remaining: maxCallbacks, removed: false }
    records.add(record)
    ensureConnection()
    return () => removeRecord(name, records, record)
  }

  /**
   * 按名移除事件全部监听器（Wails v2 EventsOff 全局语义，支持多事件名）。
   * @param {string} name
   * @param {...string} additionalNames
   */
  function off(name, ...additionalNames) {
    for (const n of [name, ...additionalNames]) {
      registry.delete(n)
    }
  }

  /**
   * 监听连接状态翻转。回调收到 true（已连接）/false（已断开）。
   * @param {(connected: boolean) => void} callback
   * @returns {() => void} 注销闭包
   */
  function onConnectionChange(callback) {
    if (typeof callback !== 'function') return () => {}
    connListeners.add(callback)
    return () => {
      connListeners.delete(callback)
    }
  }

  /**
   * 通知全部连接状态监听者。
   * @param {boolean} connected
   */
  function emitState(connected) {
    for (const listener of [...connListeners]) {
      listener(connected)
    }
  }

  return {
    EventsOnMultiple: onMultiple,
    EventsOn: (name, callback) => onMultiple(name, callback, -1),
    EventsOnce: (name, callback) => onMultiple(name, callback, 1),
    EventsOff: off,
    EventsOffAll: () => registry.clear(),
    BrowserOpenURL: (url) => {
      window.open(url, '_blank', 'noopener')
    },
    onConnectionChange
  }
}
