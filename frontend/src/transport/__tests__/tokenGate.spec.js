import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// 全量并行跑时该文件首载 tokenGate 模块图可能超出默认 5s 用例超时（worker
// 过载抖动，逻辑本身毫秒级），放宽本文件用例超时消除偶发失败
vi.setConfig({ testTimeout: 20000 })

/**
 * 每个用例重新加载模块：tokenGate.js 以模块级 gatePromise 做并发去重，
 * 重载保证用例间互不污染。
 */
async function freshGate() {
  vi.resetModules()
  return await import('../tokenGate')
}

/** 提交指定令牌值（模拟用户输入 + input 事件 + 点击） */
async function submitToken(value) {
  const input = document.querySelector('.wb-token-gate-input')
  const submit = document.querySelector('.wb-token-gate-submit')
  input.value = value
  input.dispatchEvent(new Event('input'))
  submit.click()
}

/** 排空微任务队列（finally 链传播 rejection 需多轮微任务） */
async function flushMicrotasks() {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve()
  }
}

describe('token 输入门', () => {
  beforeEach(() => {
    window.localStorage.clear()
    document.body.innerHTML = ''
  })

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('requestToken 弹出浮层，提交非空令牌后 resolve 去除首尾空白并持久化', async () => {
    const gate = await freshGate()
    const promise = gate.requestToken()
    const backdrop = document.querySelector('.wb-token-gate-backdrop')
    expect(backdrop).toBeTruthy()
    expect(document.querySelector('.wb-token-gate-title').textContent).toContain('访问令牌验证')
    // 空输入时提交按钮禁用
    expect(document.querySelector('.wb-token-gate-submit').disabled).toBe(true)
    await submitToken('  tok-1  ')
    await expect(promise).resolves.toBe('tok-1')
    expect(window.localStorage.getItem('workbench.web.token')).toBe('tok-1')
    expect(document.querySelector('.wb-token-gate-backdrop')).toBeNull()
  })

  it('并发调用去重：多个等待方共享同一 Promise 与同一浮层', async () => {
    const gate = await freshGate()
    const p1 = gate.requestToken()
    const p2 = gate.requestToken()
    expect(p1).toBe(p2)
    expect(document.querySelectorAll('.wb-token-gate-backdrop')).toHaveLength(1)
    await submitToken('tok')
    await expect(p1).resolves.toBe('tok')
    await expect(p2).resolves.toBe('tok')
  })

  it('resolve 后去重状态复位，再次调用弹出新浮层（样式注入幂等）', async () => {
    const gate = await freshGate()
    const p1 = gate.requestToken()
    await submitToken('tok-1')
    await p1
    const p2 = gate.requestToken()
    expect(document.querySelectorAll('.wb-token-gate-backdrop')).toHaveLength(1)
    // 两次弹门只注入一份样式
    expect(document.querySelectorAll('#wb-transport-style')).toHaveLength(1)
    await submitToken('tok-2')
    await expect(p2).resolves.toBe('tok-2')
  })

  it('空输入点击提交不 resolve（按钮禁用兜底）', async () => {
    const gate = await freshGate()
    const promise = gate.requestToken()
    let resolved = false
    promise.then(() => {
      resolved = true
    })
    await submitToken('   ')
    // 刷新微任务后仍未 resolve
    await Promise.resolve()
    expect(resolved).toBe(false)
    expect(document.querySelector('.wb-token-gate-backdrop')).toBeTruthy()
  })

  it('空输入按 Enter 同样不 resolve（onSubmit 空值守卫）', async () => {
    const gate = await freshGate()
    const promise = gate.requestToken()
    let resolved = false
    promise.then(() => {
      resolved = true
    })
    const input = document.querySelector('.wb-token-gate-input')
    input.value = '   '
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    await Promise.resolve()
    expect(resolved).toBe(false)
    expect(document.querySelector('.wb-token-gate-backdrop')).toBeTruthy()
  })

  it('Enter 键等价点击提交', async () => {
    const gate = await freshGate()
    const promise = gate.requestToken()
    const input = document.querySelector('.wb-token-gate-input')
    input.value = 'tok-enter'
    input.dispatchEvent(new Event('input'))
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    await expect(promise).resolves.toBe('tok-enter')
  })

  it('键盘事件 key 非 Enter 不触发提交', async () => {
    const gate = await freshGate()
    const promise = gate.requestToken()
    let resolved = false
    promise.then(() => {
      resolved = true
    })
    const input = document.querySelector('.wb-token-gate-input')
    input.value = 'tok-x'
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }))
    await Promise.resolve()
    expect(resolved).toBe(false)
    expect(document.querySelector('.wb-token-gate-backdrop')).toBeTruthy()
  })

  it('ESC 取消：浮层关闭、令牌未持久化，Promise 以可分流错误形态 reject', async () => {
    const gate = await freshGate()
    const promise = gate.requestToken()
    // 焦点在输入框内按 ESC，keydown（bubbles）经冒泡到达浮层触发取消
    const input = document.querySelector('.wb-token-gate-input')
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await expect(promise).rejects.toEqual({
      code: 'E_RPC_INTERNAL',
      message: '已取消访问令牌输入'
    })
    expect(document.querySelector('.wb-token-gate-backdrop')).toBeNull()
    expect(window.localStorage.getItem('workbench.web.token')).toBeNull()
  })

  it('关闭按钮取消等价 ESC，取消后可重新弹门并正常提交', async () => {
    const gate = await freshGate()
    const p1 = gate.requestToken()
    document.querySelector('.wb-token-gate-close').click()
    await expect(p1).rejects.toEqual({
      code: 'E_RPC_INTERNAL',
      message: '已取消访问令牌输入'
    })
    // 取消后去重态复位，再次调用弹出新浮层
    const p2 = gate.requestToken()
    expect(document.querySelectorAll('.wb-token-gate-backdrop')).toHaveLength(1)
    await submitToken('tok-2')
    await expect(p2).resolves.toBe('tok-2')
  })

  it('非 ESC 键不触发取消，浮层保留', async () => {
    const gate = await freshGate()
    const promise = gate.requestToken()
    let rejected = false
    promise.catch(() => {
      rejected = true
    })
    const input = document.querySelector('.wb-token-gate-input')
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }))
    await flushMicrotasks()
    expect(rejected).toBe(false)
    expect(document.querySelector('.wb-token-gate-backdrop')).toBeTruthy()
  })

  it('提交后 ESC 监听已随浮层注销，再次按键无副作用', async () => {
    const gate = await freshGate()
    const promise = gate.requestToken()
    await submitToken('tok-ok')
    await expect(promise).resolves.toBe('tok-ok')
    // 提交后按 ESC：浮层已移除且监听已注销，不抛错也不产生新的浮层状态
    expect(() => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))).not.toThrow()
    expect(document.querySelector('.wb-token-gate-backdrop')).toBeNull()
  })
})
