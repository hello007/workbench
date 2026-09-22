import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { createEventBridge } from '../events'
import { requestToken } from '../tokenGate'
import { notifyConnected, notifyDisconnected, notifyUnreachable } from '../connBanner'

// 隔离 token 门与横幅的真实实现（DOM/持久化副作用），仅验证事件桥对它们的调用契约
vi.mock('../tokenGate', () => ({
  requestToken: vi.fn()
}))
vi.mock('../connBanner', () => ({
  notifyConnected: vi.fn(),
  notifyDisconnected: vi.fn(),
  notifyUnreachable: vi.fn()
}))

// 服务端签发令牌为 32 字节 hex（64 字符），建连前按该形态校验（见 events.js TOKEN_PATTERN）
const TOK = 'a'.repeat(64)
const TOK2 = 'b'.repeat(64)
const TOK_NEW = 'c'.repeat(64)

/** 门提交指定令牌：模拟 tokenGate 已持久化新令牌后 resolve */
function mockGateResolves(token) {
  requestToken.mockImplementation(() => {
    window.localStorage.setItem('workbench.web.token', token)
    return Promise.resolve(token)
  })
}

/** 可编程 WebSocket 桩：记录实例，允许测试手工驱动 open/close/收帧 */
class FakeWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  static instances = []

  constructor(url, protocols) {
    this.url = url
    this.protocols = protocols
    this.readyState = FakeWebSocket.CONNECTING
    this.onopen = null
    this.onmessage = null
    this.onclose = null
    FakeWebSocket.instances.push(this)
  }

  /** 模拟服务端握手完成 */
  open() {
    this.readyState = FakeWebSocket.OPEN
    if (this.onopen) this.onopen()
  }

  /** 模拟连接断开（含握手失败） */
  close() {
    this.readyState = FakeWebSocket.CLOSED
    if (this.onclose) this.onclose({ code: 1006 })
  }

  /** 模拟服务端推送一帧事件 */
  receive(frame) {
    if (this.onmessage) this.onmessage({ data: JSON.stringify(frame) })
  }
}

function lastSocket() {
  return FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
}

/** 刷新微任务队列（token 门 Promise 链用，不依赖真实定时器） */
async function flushMicrotasks() {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve()
  }
}

describe('事件桥（window.runtime shim）', () => {
  beforeEach(() => {
    FakeWebSocket.instances = []
    vi.clearAllMocks()
    // 缺省挂起 Promise：防止未显式安排门结果的用例因 undefined.then 抛错
    requestToken.mockReturnValue(new Promise(() => {}))
    vi.stubGlobal('WebSocket', FakeWebSocket)
    window.localStorage.clear()
    window.localStorage.setItem('workbench.web.token', TOK)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('注册即按子协议通道建连：url 为 /ws、protocols 携带令牌', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    bridge.EventsOn('terminal-output', () => {})
    expect(FakeWebSocket.instances).toHaveLength(1)
    const socket = lastSocket()
    expect(socket.url).toBe('ws://test/ws')
    // 与 server/ws.go 约定一致：令牌经 Sec-WebSocket-Protocol 子协议携带
    expect(socket.protocols).toEqual([TOK])
    // 已在连接中时重复注册不重复建连
    bridge.EventsOn('terminal-exit', () => {})
    expect(FakeWebSocket.instances).toHaveLength(1)
  })

  it('帧到达按事件名分发并以变参展开 data', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const calls = []
    bridge.EventsOn('e1', (...data) => calls.push(data))
    const socket = lastSocket()
    socket.open()
    socket.receive({ event: 'e1', data: [1, 'a', true] })
    expect(calls).toEqual([[1, 'a', true]])
    // 未注册事件不分发、不抛错
    socket.receive({ event: 'other', data: [] })
    expect(calls).toEqual([[1, 'a', true]])
  })

  it('同事件多监听者全部收到分发', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const a = vi.fn()
    const b = vi.fn()
    bridge.EventsOn('e', a)
    bridge.EventsOn('e', b)
    lastSocket().open()
    lastSocket().receive({ event: 'e', data: ['x'] })
    expect(a).toHaveBeenCalledWith('x')
    expect(b).toHaveBeenCalledWith('x')
  })

  it('非法帧被忽略：非 JSON、缺事件名、data 非数组', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const cb = vi.fn()
    bridge.EventsOn('e', cb)
    const socket = lastSocket()
    socket.open()
    socket.onmessage({ data: 'not json' })
    socket.receive({ data: [1] })
    socket.receive({ event: 'e', data: 'str' })
    expect(cb).toHaveBeenCalledTimes(1)
    expect(cb).toHaveBeenCalledWith()
  })

  it('EventsOnce 触发一次后自动移除', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const cb = vi.fn()
    bridge.EventsOnce('e', cb)
    const socket = lastSocket()
    socket.open()
    socket.receive({ event: 'e', data: [1] })
    socket.receive({ event: 'e', data: [2] })
    expect(cb).toHaveBeenCalledTimes(1)
    expect(cb).toHaveBeenCalledWith(1)
  })

  it('EventsOnMultiple 按 maxCallbacks 限次后自动移除', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const cb = vi.fn()
    bridge.EventsOnMultiple('e', cb, 2)
    const socket = lastSocket()
    socket.open()
    socket.receive({ event: 'e', data: [] })
    socket.receive({ event: 'e', data: [] })
    socket.receive({ event: 'e', data: [] })
    expect(cb).toHaveBeenCalledTimes(2)
  })

  it('注销闭包仅移除本监听器（多组件共听语义）', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const a = vi.fn()
    const b = vi.fn()
    const offA = bridge.EventsOn('e', a)
    bridge.EventsOn('e', b)
    offA()
    offA() // 幂等，二次注销不抛错
    lastSocket().open()
    lastSocket().receive({ event: 'e', data: [] })
    expect(a).not.toHaveBeenCalled()
    expect(b).toHaveBeenCalledTimes(1)
  })

  it('EventsOff 按名移除该事件全部监听器，不影响其他事件', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const a = vi.fn()
    const b = vi.fn()
    bridge.EventsOn('e1', a)
    bridge.EventsOn('e1', vi.fn())
    bridge.EventsOn('e2', b)
    lastSocket().open()
    bridge.EventsOff('e1')
    lastSocket().receive({ event: 'e1', data: [] })
    lastSocket().receive({ event: 'e2', data: ['keep'] })
    expect(a).not.toHaveBeenCalled()
    expect(b).toHaveBeenCalledWith('keep')
  })

  it('EventsOffAll 清空注册表，EventsOff 多事件名一并移除', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const a = vi.fn()
    const b = vi.fn()
    const c = vi.fn()
    bridge.EventsOn('e1', a)
    bridge.EventsOn('e2', b)
    lastSocket().open()
    bridge.EventsOff('e1', 'e2')
    lastSocket().receive({ event: 'e1', data: [] })
    lastSocket().receive({ event: 'e2', data: [] })
    expect(a).not.toHaveBeenCalled()
    expect(b).not.toHaveBeenCalled()
    // EventsOffAll 清空全部剩余注册（含后注册的 e3）
    bridge.EventsOn('e3', c)
    bridge.EventsOffAll()
    lastSocket().receive({ event: 'e3', data: [] })
    expect(c).not.toHaveBeenCalled()
  })

  it('分发过程中被同事件先序监听者注销的监听者不再接收本次分发', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const b = vi.fn()
    let offB = null
    bridge.EventsOn('e', () => offB())
    offB = bridge.EventsOn('e', b)
    lastSocket().open()
    lastSocket().receive({ event: 'e', data: [] })
    // 先序监听者执行时注销了后序监听者：快照迭代跳过已移除记录
    expect(b).not.toHaveBeenCalled()
  })

  it('callback 非函数时返回空注销闭包且不建连', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const off = bridge.EventsOn('e', null)
    expect(typeof off).toBe('function')
    expect(() => off()).not.toThrow()
    expect(FakeWebSocket.instances).toHaveLength(0)
  })

  it('重连后按常驻注册表恢复分发（指数退避 1s 起步）', () => {
    vi.useFakeTimers()
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const cb = vi.fn()
    bridge.EventsOn('e', cb)
    const s1 = lastSocket()
    s1.open()
    s1.close()
    // 断开后按 1s 退避重连
    vi.advanceTimersByTime(999)
    expect(FakeWebSocket.instances).toHaveLength(1)
    vi.advanceTimersByTime(1)
    expect(FakeWebSocket.instances).toHaveLength(2)
    // 新连接 open 后事件继续按注册表分发（服务端广播语义，无需重发订阅）
    const s2 = lastSocket()
    s2.open()
    s2.receive({ event: 'e', data: ['after-reconnect'] })
    expect(cb).toHaveBeenCalledWith('after-reconnect')
  })

  it('断连触发 onConnectionChange 翻转与横幅通知，注销后不再通知', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    // 连接由事件注册驱动建立，onConnectionChange 本身不建连
    bridge.EventsOn('e', () => {})
    const listener = vi.fn()
    const off = bridge.onConnectionChange(listener)
    const socket = lastSocket()
    socket.open()
    expect(listener).toHaveBeenCalledWith(true)
    expect(notifyConnected).toHaveBeenCalled()
    socket.close()
    expect(listener).toHaveBeenCalledWith(false)
    expect(notifyDisconnected).toHaveBeenCalled()
    off()
    socket.open()
    socket.close()
    expect(listener).toHaveBeenCalledTimes(2)
  })

  it('onConnectionChange 传非函数返回安全空闭包', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const off = bridge.onConnectionChange(null)
    expect(() => off()).not.toThrow()
  })

  it('重连退避指数递增：1s/2s 起步，达到失败阈值前按退避节奏重连', () => {
    vi.useFakeTimers()
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    bridge.EventsOn('e', () => {})
    lastSocket().open()
    // 第 1 轮断开 → 1s 后重连
    lastSocket().close()
    vi.advanceTimersByTime(999)
    expect(FakeWebSocket.instances).toHaveLength(1)
    vi.advanceTimersByTime(1)
    expect(FakeWebSocket.instances).toHaveLength(2)
    lastSocket().close()
    // 第 2 轮断开 → 2s 后重连
    vi.advanceTimersByTime(1999)
    expect(FakeWebSocket.instances).toHaveLength(2)
    vi.advanceTimersByTime(1)
    expect(FakeWebSocket.instances).toHaveLength(3)
    lastSocket().close()
    // 第 3 轮断开 → 4s 后重连（连续失败第 3 次，之后转门，见阈值用例）
    vi.advanceTimersByTime(3999)
    expect(FakeWebSocket.instances).toHaveLength(3)
    vi.advanceTimersByTime(1)
    expect(FakeWebSocket.instances).toHaveLength(4)
    lastSocket().close()
  })

  it('首次连接失败（从未 open）触发 token 门重试而非静默重连', async () => {
    vi.useFakeTimers()
    // 门提交：模拟 tokenGate 已持久化新令牌后 resolve
    mockGateResolves(TOK2)
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    bridge.EventsOn('e', () => {})
    // 首连持合法令牌直接建连，不经门
    expect(requestToken).not.toHaveBeenCalled()
    // 握手失败即断开：令牌失效场景，弹门而非退避
    lastSocket().close()
    expect(requestToken).toHaveBeenCalledTimes(1)
    // 门未决期间不重复弹门、不建连（第一个门 Promise 仍挂起）
    requestToken.mockReturnValue(new Promise(() => {}))
    await flushMicrotasks()
    expect(FakeWebSocket.instances).toHaveLength(2)
    expect(lastSocket().protocols).toEqual([TOK2])
    vi.advanceTimersByTime(60000)
    expect(FakeWebSocket.instances).toHaveLength(2)
  })

  it('无本地令牌时裸连（服务端经 cookie 认证）：不弹门、不传子协议', () => {
    window.localStorage.removeItem('workbench.web.token')
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    bridge.EventsOn('e', () => {})
    // 首访 HTTP 请求已种 wb_token 会话 cookie，WS 裸连由服务端经 cookie 认证，
    // 无需本地令牌、不经 token 门
    expect(requestToken).not.toHaveBeenCalled()
    expect(FakeWebSocket.instances).toHaveLength(1)
    const socket = lastSocket()
    expect(socket.url).toBe('ws://test/ws')
    // new WebSocket(url) 第二参不传：protocols 为 undefined（非子协议通道）
    expect(socket.protocols).toBeUndefined()
  })

  it('裸连失败（从未 open）转 token 门，门提交令牌后转子协议通道', async () => {
    vi.useFakeTimers()
    window.localStorage.removeItem('workbench.web.token')
    mockGateResolves(TOK2)
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    bridge.EventsOn('e', () => {})
    expect(lastSocket().protocols).toBeUndefined()
    // 裸连握手失败（cookie 未种/失效 → 服务端 401 拒绝）：转 token 门
    lastSocket().close()
    expect(requestToken).toHaveBeenCalledTimes(1)
    await flushMicrotasks()
    // 门提交令牌后以子协议通道重建连接
    expect(FakeWebSocket.instances).toHaveLength(2)
    expect(lastSocket().protocols).toEqual([TOK2])
    vi.advanceTimersByTime(60000)
    expect(FakeWebSocket.instances).toHaveLength(2)
  })

  it('裸连成功后断线走静默重连（仍为裸连形态），不经 token 门', () => {
    vi.useFakeTimers()
    window.localStorage.removeItem('workbench.web.token')
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const cb = vi.fn()
    bridge.EventsOn('e', cb)
    const s1 = lastSocket()
    s1.open()
    s1.close()
    // 曾成功连接：断线走 1s 退避静默重连，不弹门
    expect(requestToken).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1000)
    expect(FakeWebSocket.instances).toHaveLength(2)
    expect(lastSocket().protocols).toBeUndefined()
    lastSocket().open()
    lastSocket().receive({ event: 'e', data: ['after'] })
    expect(cb).toHaveBeenCalledWith('after')
  })

  it('令牌非 64 位 hex 形态时不建 WS，直接进 token 门重输（防子协议 SyntaxError）', async () => {
    mockGateResolves(TOK)
    window.localStorage.setItem('workbench.web.token', 'bad-token!')
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    bridge.EventsOn('e', () => {})
    // 非法令牌拦截在 new WebSocket 之前：不建连、直接弹门
    expect(FakeWebSocket.instances).toHaveLength(0)
    expect(requestToken).toHaveBeenCalledTimes(1)
    await flushMicrotasks()
    // 门取得合法令牌后正常建连，子协议携带新令牌
    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(lastSocket().protocols).toEqual([TOK])
  })

  it('曾连接后连续 3 次重连失败：终止静默循环转 token 门并提示服务不可达', async () => {
    vi.useFakeTimers()
    // 门实现须在弹门前就绪（beforeEach 的挂起实现被覆盖为提交新令牌即 resolve）
    mockGateResolves(TOK2)
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    bridge.EventsOn('e', () => {})
    lastSocket().open()
    // 断开后 1s/2s/4s 退避重连，每次连上即断（重连失败）
    lastSocket().close()
    vi.advanceTimersByTime(1000)
    expect(FakeWebSocket.instances).toHaveLength(2)
    lastSocket().close()
    vi.advanceTimersByTime(2000)
    expect(FakeWebSocket.instances).toHaveLength(3)
    lastSocket().close()
    vi.advanceTimersByTime(4000)
    expect(FakeWebSocket.instances).toHaveLength(4)
    lastSocket().close()
    // 连续失败达阈值（3 次）：不再调度退避，弹门重新认证 + 不可达横幅
    expect(requestToken).toHaveBeenCalledTimes(1)
    expect(notifyUnreachable).toHaveBeenCalled()
    // 门未决期间不再静默重连
    vi.advanceTimersByTime(60000)
    expect(FakeWebSocket.instances).toHaveLength(4)
    // 门提交新令牌后失败计数清零并重建连接
    await flushMicrotasks()
    expect(FakeWebSocket.instances).toHaveLength(5)
    expect(lastSocket().protocols).toEqual([TOK2])
  })

  it('token 门被取消后停止重连并保留不可达横幅，不再静默循环', async () => {
    vi.useFakeTimers()
    // 用户取消令牌输入：门 reject（实现须在弹门前就绪）
    requestToken.mockReturnValue(Promise.reject({ code: 'E_RPC_INTERNAL', message: '已取消访问令牌输入' }))
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    bridge.EventsOn('e', () => {})
    lastSocket().open()
    // 连续 3 次重连失败达阈值，弹门
    lastSocket().close()
    vi.advanceTimersByTime(1000)
    lastSocket().close()
    vi.advanceTimersByTime(2000)
    lastSocket().close()
    vi.advanceTimersByTime(4000)
    lastSocket().close()
    expect(requestToken).toHaveBeenCalledTimes(1)
    await flushMicrotasks()
    // 取消路径同样给出不可达横幅
    expect(notifyUnreachable).toHaveBeenCalled()
    // 取消后不再重连，之后重新注册监听可再次拉起建连
    vi.advanceTimersByTime(60000)
    expect(FakeWebSocket.instances).toHaveLength(4)
    mockGateResolves(TOK_NEW)
    bridge.EventsOn('e2', () => {})
    await flushMicrotasks()
    expect(FakeWebSocket.instances).toHaveLength(5)
  })

  it('注册表清空后断开不再重连，重新注册时唤醒建连', () => {
    vi.useFakeTimers()
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const off = bridge.EventsOn('e', () => {})
    lastSocket().open()
    off()
    lastSocket().close()
    vi.advanceTimersByTime(60000)
    expect(FakeWebSocket.instances).toHaveLength(1)
    bridge.EventsOn('e2', () => {})
    expect(FakeWebSocket.instances).toHaveLength(2)
  })

  it('BrowserOpenURL 浏览器下等价为新标签页打开', () => {
    const bridge = createEventBridge({ wsUrl: 'ws://test/ws' })
    const openMock = vi.fn()
    vi.stubGlobal('open', openMock)
    bridge.BrowserOpenURL('https://example.com')
    expect(openMock).toHaveBeenCalledWith('https://example.com', '_blank', 'noopener')
  })
})
