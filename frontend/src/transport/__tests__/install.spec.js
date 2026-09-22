import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { hasWailsBindings, installBrowserTransport, buildWsUrl } from '../index'

// 隔离 token 门与横幅的真实实现（install 出的事件桥连接失败会触发它们）
vi.mock('../tokenGate', () => ({
  requestToken: vi.fn(() => new Promise(() => {}))
}))
vi.mock('../connBanner', () => ({
  notifyConnected: vi.fn(),
  notifyDisconnected: vi.fn(),
  notifyUnreachable: vi.fn()
}))

/** 可编程 WebSocket 桩：仅记录构造参数 */
class FakeWebSocket {
  static instances = []
  constructor(url, protocols) {
    this.url = url
    this.protocols = protocols
    FakeWebSocket.instances.push(this)
  }
}

describe('transport 探测与装配', () => {
  beforeEach(() => {
    FakeWebSocket.instances = []
    delete window.go
    delete window.runtime
    // 预置合法形态令牌（64 位 hex）：注册监听即建连，不经 token 门（门已被 mock 挂起）
    window.localStorage.clear()
    window.localStorage.setItem('workbench.web.token', 'a'.repeat(64))
    vi.stubGlobal('WebSocket', FakeWebSocket)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('wails 环境不安装：返回 false 且不覆写 window.runtime（桌面零变化）', () => {
    const sentinelRuntime = { EventsOn: () => {} }
    window.go = { main: { App: { ListDirectory: () => {} } } }
    window.runtime = sentinelRuntime
    expect(hasWailsBindings()).toBe(true)
    expect(installBrowserTransport()).toBe(false)
    expect(window.runtime).toBe(sentinelRuntime)
  })

  it('无 wails 绑定时安装 RPC Proxy 与事件桥（API 面齐全）', () => {
    expect(hasWailsBindings()).toBe(false)
    expect(installBrowserTransport()).toBe(true)
    // wailsjs 生成 wrapper 调用形态：window.go.main.App[方法名](args)
    expect(typeof window.go.main.App.ListDirectory).toBe('function')
    for (const api of ['EventsOn', 'EventsOnMultiple', 'EventsOnce', 'EventsOff', 'EventsOffAll', 'BrowserOpenURL', 'onConnectionChange']) {
      expect(typeof window.runtime[api]).toBe('function')
    }
  })

  it('WS 端点由页面地址同源推导（http → ws）', () => {
    installBrowserTransport()
    window.runtime.EventsOn('x', () => {})
    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(FakeWebSocket.instances[0].url).toBe(`ws://${window.location.host}/ws`)
  })

  it('buildWsUrl https 页面推导 wss 端点', () => {
    expect(buildWsUrl({ protocol: 'https:', host: 'demo.example.com' })).toBe('wss://demo.example.com/ws')
    expect(buildWsUrl({ protocol: 'http:', host: '127.0.0.1:36115' })).toBe('ws://127.0.0.1:36115/ws')
  })
})
