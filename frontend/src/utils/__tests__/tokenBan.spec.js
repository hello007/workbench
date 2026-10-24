import { describe, it, expect, vi, beforeEach } from 'vitest'
import { kickTokenConnections } from '../tokenBan'

vi.mock('../../../wailsjs/go/main/App', () => ({
  KickConnection: vi.fn()
}))

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }
}))

/** 等待逐连接踢出的微任务链收敛（for-await 每轮一个 await） */
async function flushAll() {
  for (let i = 0; i < 8; i++) {
    await Promise.resolve()
  }
}

describe('kickTokenConnections（封禁联动踢出编排）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  /** 连接明细 fixture（形状对齐 useDeviceConnections 的 connections 条目） */
  const conn = (id, tokenId) => ({ id, ip: `10.0.0.${id}`, tokenId, tokenName: '', connectedAt: new Date().toISOString() })

  it('按 tokenID 过滤该令牌全部在途连接，逐个 KickConnection（多标签页多连接全覆盖）', async () => {
    const { KickConnection } = await import('../../../wailsjs/go/main/App')
    KickConnection.mockResolvedValue()
    const connections = [conn(1, 't1'), conn(2, 't2'), conn(3, 't1'), conn(4, 't3'), conn(5, 't1')]
    await kickTokenConnections(connections, 't1')
    await flushAll()
    expect(KickConnection).toHaveBeenCalledTimes(3)
    expect(KickConnection).toHaveBeenNthCalledWith(1, 1)
    expect(KickConnection).toHaveBeenNthCalledWith(2, 3)
    expect(KickConnection).toHaveBeenNthCalledWith(3, 5)
  })

  it('单个连接踢出失败不中断其余（fail-soft），收集后统一 warning 提示失败数', async () => {
    const { KickConnection } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    KickConnection.mockRejectedValueOnce({ code: 'E_CONNECTION_NOT_FOUND', message: '连接不存在' })
      .mockResolvedValueOnce()
      .mockResolvedValueOnce()
    const connections = [conn(1, 't1'), conn(2, 't1'), conn(3, 't1')]
    await kickTokenConnections(connections, 't1')
    await flushAll()
    // 首个失败后其余连接照常逐个踢出
    expect(KickConnection).toHaveBeenCalledTimes(3)
    expect(KickConnection).toHaveBeenNthCalledWith(2, 2)
    expect(KickConnection).toHaveBeenNthCalledWith(3, 3)
    // 失败收敛为一条 warning（1/3），不抛错不打断调用方
    expect(ElMessage.warning).toHaveBeenCalledTimes(1)
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('1/3'))
    expect(ElMessage.error).not.toHaveBeenCalled()
  })

  it('全部成功时不产生 warning', async () => {
    const { KickConnection } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    KickConnection.mockResolvedValue()
    await kickTokenConnections([conn(1, 't1'), conn(2, 't1')], 't1')
    await flushAll()
    expect(ElMessage.warning).not.toHaveBeenCalled()
  })

  it('无该令牌在途连接时不调用、不提示（浏览器通道 connections 恒空的自然形态）', async () => {
    const { KickConnection } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    await kickTokenConnections([conn(1, 't2')], 't1')
    await flushAll()
    expect(KickConnection).not.toHaveBeenCalled()
    expect(ElMessage.warning).not.toHaveBeenCalled()
  })

  it('connections 非数组（undefined/异常载荷）时安全 no-op', async () => {
    const { KickConnection } = await import('../../../wailsjs/go/main/App')
    await kickTokenConnections(undefined, 't1')
    await flushAll()
    expect(KickConnection).not.toHaveBeenCalled()
  })
})
