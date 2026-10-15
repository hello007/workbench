import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

import { installWailsCallErrorBridge } from '../wails-call-error-bridge'

// wails v2.16 desktop/calls.js Callback() 的忠实镜像（含上游 new Error 包装行为），
// 作为 fake 原实现驱动桥接层：证明包装后恢复 v2.12 的结构化 reject 语义。
function makeV216RuntimeCallback(callbacks) {
  return function originalCallback(incomingMessage) {
    const message = JSON.parse(incomingMessage)
    const pending = callbacks[message.callbackid]
    if (!pending) {
      throw new Error(`Callback '${message.callbackid}' not registered!!!`)
    }
    clearTimeout(pending.timeoutHandle)
    delete callbacks[message.callbackid]
    if (message.error) {
      const err = message.error instanceof Error ? message.error : new Error(message.error)
      pending.reject(err)
    } else {
      pending.resolve(message.result)
    }
  }
}

// 注册一个 pending 调用（等价 runtime Call() 存入 callbacks 表的形态）
function registerPending(callbacks, callbackID) {
  let resolve
  let reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  callbacks[callbackID] = { timeoutHandle: 123, resolve, reject }
  return { promise, resolve, reject }
}

describe('wails-call-error-bridge', () => {
  let callbacks
  let originalCallback

  const callWith = (payload) => window.wails.Callback(JSON.stringify(payload))

  beforeEach(() => {
    callbacks = {}
    originalCallback = makeV216RuntimeCallback(callbacks)
    window.wails = { Callback: originalCallback, callbacks }
  })

  afterEach(() => {
    delete window.wails
    vi.restoreAllMocks()
  })

  describe('桌面模式（window.wails 存在）', () => {
    beforeEach(() => {
      installWailsCallErrorBridge()
    })

    it('结构化对象错误：以原对象 reject（code/message 完整，v2.12 语义），不经 new Error 压成 [object Object]', async () => {
      const { promise } = registerPending(callbacks, 'main.App.ListAssetSlides-1')
      callWith({
        callbackid: 'main.App.ListAssetSlides-1',
        error: { code: 'E_ASSETS_LIB_NOT_FOUND', message: '素材库不存在或已删除' }
      })
      await expect(promise).rejects.toEqual({
        code: 'E_ASSETS_LIB_NOT_FOUND',
        message: '素材库不存在或已删除'
      })
    })

    it('结构化对象错误：pending 回调按 runtime 同款协议收口（出表 + clearTimeout）', async () => {
      const clearTimeoutSpy = vi.spyOn(window, 'clearTimeout')
      const { promise } = registerPending(callbacks, 'main.App.SearchAssets-2')
      callWith({ callbackid: 'main.App.SearchAssets-2', error: { message: '失败' } })
      await expect(promise).rejects.toEqual({ message: '失败' })
      expect(callbacks['main.App.SearchAssets-2']).toBeUndefined()
      expect(clearTimeoutSpy).toHaveBeenCalledWith(123)
    })

    it('字符串错误：委托原实现，reject Error 实例且 message 原样保留', async () => {
      const { promise } = registerPending(callbacks, 'main.App.Parse-3')
      callWith({ callbackid: 'main.App.Parse-3', error: 'error parsing arguments: count mismatch' })
      await expect(promise).rejects.toThrow('error parsing arguments: count mismatch')
    })

    it('成功应答：委托原实现正常 resolve', async () => {
      const { promise } = registerPending(callbacks, 'main.App.ListAssetSlides-4')
      callWith({ callbackid: 'main.App.ListAssetSlides-4', result: { items: [], total: 0 } })
      await expect(promise).resolves.toEqual({ items: [], total: 0 })
    })

    it('error 为 null（成功应答的序列化形态）：委托原实现 resolve，不误判为对象错误', async () => {
      const { promise } = registerPending(callbacks, 'main.App.ListAssetSlides-5')
      callWith({ callbackid: 'main.App.ListAssetSlides-5', error: null, result: 'ok' })
      await expect(promise).resolves.toBe('ok')
    })

    it('结构化错误但回调未注册：委托原实现按其既有语义抛错', () => {
      expect(() => callWith({ callbackid: 'ghost-6', error: { message: 'x' } })).toThrow(
        "Callback 'ghost-6' not registered!!!"
      )
    })

    it('非法 JSON 应答：委托原实现抛错（原实现自带告警路径）', () => {
      expect(() => window.wails.Callback('{not-json')).toThrow()
    })

    it('安装幂等：重复安装不叠加包装（original 引用不变）', () => {
      const wrapped = window.wails.Callback
      installWailsCallErrorBridge()
      expect(window.wails.Callback).toBe(wrapped)
      expect(window.wails.__callErrorBridgeInstalled).toBe(true)
    })
  })

  describe('非桌面模式（window.wails 缺失或形态不全）', () => {
    it('window.wails 不存在（浏览器访问模式）：静默不装不抛错', () => {
      delete window.wails
      expect(() => installWailsCallErrorBridge()).not.toThrow()
    })

    it('callbacks 回调表缺失（未来 runtime 改面）：静默不装不抛错', () => {
      window.wails = { Callback: vi.fn() }
      expect(() => installWailsCallErrorBridge()).not.toThrow()
      expect(window.wails.__callErrorBridgeInstalled).toBeUndefined()
    })
  })
})
