import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

/**
 * 每个用例重新加载模块：connBanner.js 以模块级状态（everConnected/横幅节点）
 * 跨调用记忆，重载保证用例间互不污染。
 */
async function freshBanner() {
  vi.resetModules()
  return await import('../connBanner')
}

describe('连接状态横幅', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('首次成功连接前断开不弹横幅（初始化路径由 token 门覆盖）', async () => {
    const banner = await freshBanner()
    banner.notifyDisconnected()
    expect(document.querySelector('.wb-conn-banner')).toBeNull()
  })

  it('首次成功连接不弹横幅', async () => {
    const banner = await freshBanner()
    banner.notifyConnected()
    expect(document.querySelector('.wb-conn-banner')).toBeNull()
  })

  it('连接后断开弹出 warning 横幅并常驻直至恢复', async () => {
    vi.useFakeTimers()
    const banner = await freshBanner()
    banner.notifyConnected()
    banner.notifyDisconnected()
    const el = document.querySelector('.wb-conn-banner')
    expect(el).toBeTruthy()
    expect(el.textContent).toBe('与后端连接已断开，正在自动重连…')
    expect(el.className).toContain('wb-conn-banner--warning')
    // 断连横幅不自动消失
    vi.advanceTimersByTime(60000)
    expect(document.querySelector('.wb-conn-banner')).toBeTruthy()
  })

  it('恢复时弹 success 提示并在 3s 后自动消失', async () => {
    vi.useFakeTimers()
    const banner = await freshBanner()
    banner.notifyConnected()
    banner.notifyDisconnected()
    banner.notifyConnected()
    const el = document.querySelector('.wb-conn-banner')
    expect(el.textContent).toBe('连接已恢复')
    expect(el.className).toContain('wb-conn-banner--success')
    vi.advanceTimersByTime(2999)
    expect(document.querySelector('.wb-conn-banner')).toBeTruthy()
    vi.advanceTimersByTime(1)
    expect(document.querySelector('.wb-conn-banner')).toBeNull()
  })

  it('连续断开提示复用同一节点不重复弹', async () => {
    const banner = await freshBanner()
    banner.notifyConnected()
    banner.notifyDisconnected()
    banner.notifyDisconnected()
    expect(document.querySelectorAll('.wb-conn-banner')).toHaveLength(1)
  })

  it('notifyUnreachable 展示服务不可达横幅并常驻直至恢复', async () => {
    vi.useFakeTimers()
    const banner = await freshBanner()
    banner.notifyUnreachable()
    const el = document.querySelector('.wb-conn-banner')
    expect(el.textContent).toBe('服务不可达，请确认 WorkBench 正在运行')
    expect(el.className).toContain('wb-conn-banner--warning')
    // 不可达横幅与断连横幅同语义：不自动消失，常驻直至重连成功
    vi.advanceTimersByTime(60000)
    expect(document.querySelector('.wb-conn-banner')).toBeTruthy()
  })

  it('曾连接后不可达，重连成功显示恢复提示并在 3s 后自动消失', async () => {
    vi.useFakeTimers()
    const banner = await freshBanner()
    banner.notifyConnected()
    banner.notifyUnreachable()
    banner.notifyConnected()
    const el = document.querySelector('.wb-conn-banner')
    expect(el.textContent).toBe('连接已恢复')
    expect(el.className).toContain('wb-conn-banner--success')
    vi.advanceTimersByTime(3000)
    expect(document.querySelector('.wb-conn-banner')).toBeNull()
  })

  it('初始化阶段不可达后重连成功，横幅静默消失不误报恢复', async () => {
    vi.useFakeTimers()
    const banner = await freshBanner()
    banner.notifyUnreachable()
    // 从未成功连接过：重连成功走 hide 收尾，不弹「连接已恢复」
    banner.notifyConnected()
    expect(document.querySelector('.wb-conn-banner')).toBeNull()
  })
})
