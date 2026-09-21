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
})
