import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { createRpcProxy, invokeRpc } from '../rpc'
import { clearToken } from '../token'

/**
 * token 状态（模块级内存兜底 + localStorage）经 clearToken 复位；
 * tokenGate 去重态在 resolve 后于 finally 自行复位，用例间无残留。
 */

/** 构造形似 fetch Response 的最小应答对象 */
function jsonResponse(body, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body
  }
}

describe('RPC 通道', () => {
  let fetchMock

  beforeEach(() => {
    clearToken()
    document.body.innerHTML = ''
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('方法调用翻译为 POST /api/rpc {method, args} 并 resolve data', async () => {
    setStoredToken('tok-1')
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true, data: { id: 7 } }))
    const proxy = createRpcProxy()
    await expect(proxy.ListDirectory('D:/x', true)).resolves.toEqual({ id: 7 })
    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/rpc')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual({ method: 'ListDirectory', args: ['D:/x', true] })
    expect(init.headers.Authorization).toBe('Bearer tok-1')
    expect(init.headers['Content-Type']).toBe('application/json')
  })

  it('未存储令牌时请求不带 Authorization 头', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true, data: null }))
    await createRpcProxy().GetVersionInfo()
    expect(fetchMock.mock.calls[0][1].headers.Authorization).toBeUndefined()
  })

  it('ok=false 业务失败 reject {code, message}（与桌面 ErrorFormatter 形态一致）', async () => {
    setStoredToken('tok-1')
    fetchMock.mockResolvedValueOnce(jsonResponse({
      ok: false,
      error: { code: 'E_GIT_IN_PROGRESS', message: '该仓库有 Git 操作进行中，请稍后重试' }
    }))
    await expect(createRpcProxy().Merge('/repo', 'main', 'ff')).rejects.toEqual({
      code: 'E_GIT_IN_PROGRESS',
      message: '该仓库有 Git 操作进行中，请稍后重试'
    })
  })

  it('ok=false 且 error 无 code 时兜底 E_RPC_INTERNAL', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: false, error: { message: '未知错误' } }))
    await expect(createRpcProxy().X()).rejects.toEqual({
      code: 'E_RPC_INTERNAL',
      message: '未知错误'
    })
  })

  it('HTTP 401 触发 token 输入门，输入后携带新令牌重试成功', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ error: '未授权访问：请携带访问令牌' }, 401))
      .mockResolvedValueOnce(jsonResponse({ ok: true, data: 'done' }))
    const promise = createRpcProxy().ReadFileBytes('D:/a.txt')
    // 首次 401 后弹出输入门（重试前仅发生一次请求）
    await vi.waitFor(() => {
      expect(document.querySelector('.wb-token-gate-backdrop')).toBeTruthy()
    })
    expect(fetchMock).toHaveBeenCalledOnce()
    // 输入令牌提交 → 自动重试
    const input = document.querySelector('.wb-token-gate-input')
    input.value = 'tok-new'
    input.dispatchEvent(new Event('input'))
    document.querySelector('.wb-token-gate-submit').click()
    await expect(promise).resolves.toBe('done')
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls[1][1].headers.Authorization).toBe('Bearer tok-new')
    // 令牌已经门持久化，后续请求直接复用
    expect(window.localStorage.getItem('workbench.web.token')).toBe('tok-new')
    expect(document.querySelector('.wb-token-gate-backdrop')).toBeNull()
  })

  it('HTTP 401 后取消令牌输入：取消错误经通道透传 reject，浮层关闭', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ error: '未授权访问：请携带访问令牌' }, 401))
    const promise = createRpcProxy().ReadFileBytes('D:/a.txt')
    await vi.waitFor(() => {
      expect(document.querySelector('.wb-token-gate-backdrop')).toBeTruthy()
    })
    // ESC 取消输入门（焦点在输入框内经冒泡触发）：reject {code, message} 交由
    // 业务组件 handleError 分流，下一次任意 RPC 401 可再次触发门（不永久锁死认证入口）
    const input = document.querySelector('.wb-token-gate-input')
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await expect(promise).rejects.toEqual({
      code: 'E_RPC_INTERNAL',
      message: '已取消访问令牌输入'
    })
    expect(document.querySelector('.wb-token-gate-backdrop')).toBeNull()
  })

  it('协议层 HTTP 拒绝（405）reject writeRPCError 结构中的 code/message', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(
      { ok: false, error: { code: 'E_RPC_BAD_REQUEST', message: '仅支持 POST 请求' } },
      405
    ))
    await expect(createRpcProxy().ListDirectory()).rejects.toEqual({
      code: 'E_RPC_BAD_REQUEST',
      message: '仅支持 POST 请求'
    })
  })

  it('HTTP 非 200 且 body 为 {error:"文本"} 形态时兜底 E_RPC_INTERNAL', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ error: 'boom' }, 500))
    await expect(createRpcProxy().X()).rejects.toEqual({
      code: 'E_RPC_INTERNAL',
      message: 'boom'
    })
  })

  it('200 响应体非法 JSON 时 reject 响应格式异常', async () => {
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => {
        throw new Error('invalid json')
      }
    })
    await expect(createRpcProxy().X()).rejects.toEqual({
      code: 'E_RPC_INTERNAL',
      message: '响应格式异常'
    })
  })

  it('网络失败 reject E_RPC_INTERNAL 与可读消息', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('fetch failed'))
    await expect(createRpcProxy().X()).rejects.toEqual({
      code: 'E_RPC_INTERNAL',
      message: '网络请求失败: fetch failed'
    })
    // 非 Error 形态的失败原因（无 message 属性）按字符串兜底
    fetchMock.mockRejectedValueOnce('boom')
    await expect(createRpcProxy().X()).rejects.toEqual({
      code: 'E_RPC_INTERNAL',
      message: '网络请求失败: boom'
    })
  })

  it('HTTP 非 200 且 error 对象无 message 时回退状态码文案', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ error: {} }, 503))
    await expect(createRpcProxy().X()).rejects.toEqual({
      code: 'E_RPC_INTERNAL',
      message: '请求失败 (HTTP 503)'
    })
  })

  it('ok=false 且 error 无 message 时兜底「未知错误」', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: false, error: { code: 'E_X' } }))
    await expect(createRpcProxy().X()).rejects.toEqual({
      code: 'E_X',
      message: '未知错误'
    })
  })

  it('invokeRpc 参数数组与 Symbol 属性访问行为正确', async () => {
    const proxy = createRpcProxy()
    expect(proxy[Symbol.toPrimitive]).toBeUndefined()
    expect(fetchMock).not.toHaveBeenCalled()
    // invokeRpc 可独立调用（等价于 proxy 方法调用的底层入口）
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true, data: 42 }))
    await expect(invokeRpc('Ping', [])).resolves.toBe(42)
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ method: 'Ping', args: [] })
  })
})

/** 直写 localStorage 令牌（仅本文件用，避免引入模块级内存态干扰） */
function setStoredToken(value) {
  window.localStorage.setItem('workbench.web.token', value)
}
