import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

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
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await Promise.resolve()
    expect(resolved).toBe(false)
  })
})
