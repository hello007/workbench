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
 *   - 本地存有合法令牌（64 位 hex）时经 Sec-WebSocket-Protocol 子协议携带
 *     （new WebSocket(url, [token])），与 server/ws.go 约定一致（服务端以
 *     upgrader.Subprotocols 回显选中子协议）；本地无令牌时裸连
 *     （new WebSocket(url)，不带 protocols），服务端经首访 HTTP 请求种下的
 *     wb_token cookie 认证（见 server/web.go issueSessionCookie），支持
 *     localStorage 清空后刷新页面的场景；
 *   - 注册表常驻：断线自动重连（指数退避，1s 起步、2 倍递增、30s 封顶），
 *     服务端向全部客户端广播，重连成功后事件自然恢复分发，无需重发订阅；
 *   - 断线重连续传（见 server/ws.go 续传契约）：事件帧携带 hub 级单调序号
 *     seq，本模块记录已收最大序号 lastSeq（内存态，多标签页各记各的）；
 *     重连 open 时若已有基准（lastSeq 非空）发送握手帧 {"resume": lastSeq}
 *     请求补发缺口，首连不发送（服务端从当前序号起广播，不重放历史）；
 *   - 补发帧与重连注册窗口内的实时广播共用服务端发送队列（FIFO 交错到达，
 *     补发集是窗口广播的超集），故握手后进入同步模式：序号帧先进缓冲不落地，
 *     收到 ws-synced 完成标记后按 seq 升序去重一次性落地，保证业务事件有序
 *     且不重不漏；缓冲量超限（服务端标记帧异常缺失的兜底）按断层处理；
 *   - ws-gap 断层标记帧（补发判定为缺口：服务端缓冲溢出或重启后序号归零）：
 *     放弃补发窗口，以服务端当前序号为新基准继续接收，经 connBanner 提示
 *     「部分更新已跳过」，由各面板既有冷启动/状态查询链路或手动刷新补齐状态；
 *   - ws-kicked 被踢单播控制帧（连接管控，服务端仅送达被踢连接、无 seq 不进
 *     续传缓冲）：分发层拦截转 connBanner 被踢态提示（「连接已被主机断开」），
 *     不进业务监听器、不推进序号基准；紧随的断线走既有静默自动重连链路；
 *   - 无 seq 字段的帧（老服务端兼容形态）直接分发，不推进序号基准；
 *   - 首次连接（页面生命周期内从未 open 成功）失败视为令牌缺失/失效
 *     （含裸连 cookie 认证 401），弹 token 输入门重试；已成功连接过的断线走
 *     静默退避重连（横幅提示，见 connBanner.js），连续失败达 3 次后终止静默
 *     循环，弹 token 门重新认证并横幅提示服务不可达；门被取消则停止重连，
 *     之后任意 EventsOn 注册或 RPC 401 弹门可再次拉起认证；若服务端轮换令牌，
 *     RPC 401 门会更新令牌，下一次重连即用新值；
 *   - 本地令牌非空但非 64 位 hex 形态时直接进 token 门重输，避免非法字符作为
 *     WebSocket 子协议同步抛 SyntaxError（裸连不带子协议无此风险，故仅拦截
 *     子协议通道）；
 *   - 无监听者时不重连，下次 EventsOn 注册时唤醒。
 */

import { getToken } from './token'
import { requestToken } from './tokenGate'
import { notifyConnected, notifyDisconnected, notifyUnreachable, notifyGap, notifyKicked } from './connBanner'

// 重连退避参数：基础 1s，指数递增，封顶 30s。
const RECONNECT_BASE_DELAY_MS = 1000
const RECONNECT_MAX_DELAY_MS = 30000
// 连续重连失败次数上限：达到后终止静默退避循环，转 token 门重新认证。
const RECONNECT_FAIL_LIMIT = 3
// 重连同步模式缓冲上限（帧数）：正常情况下服务端补发后必发 ws-synced 标记，
// 缓冲不会累积到该量级；超限说明标记帧异常缺失（服务端版本错配等），按断层
// 兜底处理，防止无界缓冲。服务端环形缓冲容量见 server/ws.go wsResumeBuffer。
const SYNC_BUFFER_LIMIT = 5000
// 服务端签发的访问令牌为 32 字节 hex（64 字符，见 server/token.go GenerateToken）。
// 用于拦截「非空但形态非法」的令牌：非法字符作为子协议传入会使 new WebSocket
// 同步抛 SyntaxError 未捕获，杀死重连链。空令牌不走此拦截——无令牌时裸连，
// 服务端经 wb_token cookie 认证（见 server/web.go issueSessionCookie）。
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
  // 已收到的最大事件序号（null = 本页面尚未收过任何带 seq 的帧）。内存态，
  // 多标签页各记各的；页面刷新后归 null，重连按新连接处理（不重放历史）。
  let lastSeq = null
  // 重连同步模式：resume 握手已发出、尚未收到 ws-synced 标记。期间序号帧
  // 进缓冲不落地，待标记后按 seq 升序去重一次性落地（保证业务事件有序）。
  let syncMode = false
  let syncBuffer = null

  /**
   * 确保建立 WS 连接（已连接/连接中/等待令牌时为 no-op）。
   */
  function ensureConnection() {
    if (socket || pendingGate) return
    connect()
  }

  /**
   * 发起一次 WS 连接。本地有合法令牌走子协议通道；无令牌走裸连（服务端经
   * wb_token cookie 认证）；令牌非空但形态非法（非 64 位 hex）先经 token 门
   * 取得合法值再连。
   */
  function connect() {
    if (socket || pendingGate) return
    const token = getToken()
    if (token && !TOKEN_PATTERN.test(token)) {
      // 令牌非服务端签发的 64 位 hex 形态：经 token 门取得/重输
      //（仅拦截子协议通道：非法字符作子协议会使 new WebSocket 同步抛
      // SyntaxError；裸连不带子协议，无令牌时直接裸连经 cookie 认证）
      waitGateThenConnect()
      return
    }
    // 有令牌：子协议携带；无令牌：裸连（第二参不传，服务端经 cookie 认证）
    socket = token ? new WebSocket(wsUrl, [token]) : new WebSocket(wsUrl)
    socket.onopen = () => {
      everOpened = true
      reconnectAttempts = 0
      if (lastSeq !== null) {
        // 断线重连：携带已收最大序号请求补发缺口（首连无基准不发送，
        // 服务端从当前序号起广播、不重放历史）。发送失败不单独处理——
        // 连接异常会随即触发 onclose 走既有重连/门链路。
        syncMode = true
        syncBuffer = new Map()
        try {
          socket.send(JSON.stringify({ resume: lastSeq }))
        } catch {
          /* 连接异常路径由 onclose 兜底 */
        }
      }
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
      // 续传控制帧（无 seq，服务端不广播给业务监听器）：分发层拦截处理
      if (frame.event === 'ws-gap') {
        handleGap(frame)
        return
      }
      if (frame.event === 'ws-synced') {
        handleSynced()
        return
      }
      // 被主机踢出（连接管控单播控制帧，仅送达被踢连接、无 seq 不进续传缓冲）：
      // 置被踢态横幅；紧随其后的 onclose 走既有静默重连链路，connBanner 被踢态
      // 保证通用断连文案不覆盖「已被主机断开」的解释
      if (frame.event === 'ws-kicked') {
        notifyKicked()
        return
      }
      if (typeof frame.seq === 'number' && Number.isFinite(frame.seq)) {
        receiveSequenced(frame)
        return
      }
      // 无 seq 帧（老服务端兼容形态）：直接分发，不推进序号基准
      dispatch(frame.event, Array.isArray(frame.data) ? frame.data : [])
    }
    socket.onclose = () => {
      socket = null
      notifyDisconnected()
      emitState(false)
      if (!everOpened) {
        // 首次握手未成功即断开：典型为令牌缺失/失效（含裸连 cookie 认证
        // 401——cookie 未种或服务端令牌已轮换），经门重试
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
   * 处理带序号的业务事件帧：实时模式落地并推进序号基准；同步模式（重连
   * 补发窗口）先进缓冲，待 ws-synced 标记后按序落地。
   * @param {{event: string, data: unknown[], seq: number}} frame
   */
  function receiveSequenced(frame) {
    if (syncMode) {
      if (lastSeq !== null && frame.seq <= lastSeq) {
        // 补发窗口内不应出现已收序号（防御：跨代际/异常服务端形态）
        return
      }
      // 同 Map 键去重：同一帧可能经「注册窗口实时广播」与「补发集」两条
      // 路径各到达一次（服务端同一 FIFO 队列交错），保留一份
      if (!syncBuffer.has(frame.seq)) {
        syncBuffer.set(frame.seq, frame)
      }
      if (syncBuffer.size > SYNC_BUFFER_LIMIT) {
        // 兜底：ws-synced 标记异常缺失（服务端版本错配等），缓冲量失控，
        // 按断层语义放弃等待，已缓冲帧按序落地后恢复实时分发
        flushSyncBuffer()
        notifyGap()
      }
      return
    }
    if (lastSeq !== null && frame.seq <= lastSeq) {
      // 重复帧（重连补发与实时广播的残余交叠）：丢弃
      return
    }
    lastSeq = frame.seq
    dispatch(frame.event, Array.isArray(frame.data) ? frame.data : [])
  }

  /**
   * 落地同步缓冲：按 seq 升序去重逐帧分发并推进序号基准。补发帧与注册窗口
   * 内的实时广播在服务端队列中交错，到达顺序不代表序号顺序，升序排序保证
   * 业务事件（如终端输出分块）按产生序消费。
   */
  function flushSyncBuffer() {
    syncMode = false
    const buffered = syncBuffer
    syncBuffer = null
    if (!buffered) return
    const seqs = [...buffered.keys()].sort((a, b) => a - b)
    for (const seq of seqs) {
      if (lastSeq !== null && seq <= lastSeq) continue
      const frame = buffered.get(seq)
      lastSeq = seq
      dispatch(frame.event, Array.isArray(frame.data) ? frame.data : [])
    }
  }

  /**
   * 处理补发完成标记（ws-synced）：同步缓冲按序落地，恢复实时分发。
   * 非同步模式收到（重复 resume 握手等异常形态）忽略。
   */
  function handleSynced() {
    if (!syncMode) return
    flushSyncBuffer()
  }

  /**
   * 处理断层标记帧（ws-gap）：补发判定为缺口（服务端缓冲溢出或重启后序号
   * 归零），放弃补发窗口，以服务端当前序号为新基准继续接收，并提示部分
   * 更新已跳过——各面板既有冷启动快照 / 状态查询链路或手动刷新负责补齐状态。
   * @param {{data: unknown[]}} frame data[0] 为服务端当前序号
   */
  function handleGap(frame) {
    syncMode = false
    syncBuffer = null
    const current = Array.isArray(frame.data) ? frame.data[0] : undefined
    lastSeq = typeof current === 'number' && current >= 0 ? current : 0
    notifyGap()
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
