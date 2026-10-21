/**
 * E2E 共享 fixtures：自动为每个测试注入 Wails mock（页面脚本执行前生效）。
 *
 * E2E 用例统一从本文件 import { test, expect }，无须手动 addInitScript。
 * mock 数据源为 src/test/wails-mock-defaults.js 的 E2E 合并表，
 * 与 vitest 全局兜底共用基础数据（见该文件头注释）。
 *
 * per-test / per-describe 覆盖：用例（或 describe）内 test.use({ wailsOverrides: {...} })
 * 覆盖指定方法的返回值；value 支持 wails-init.js 定义的描述符
 * （纯值 / {__error__} / {__sequence__}）。每个测试独享新页面，覆盖互不影响。
 *
 * 调用断言：getWailsCalls(page, method?) 读取页面内 window.__wailsCalls
 * 调用记录，用于断言「用户操作触发了正确的 Wails bound method 与参数」。
 */
import { test as base, expect } from '@playwright/test'
import { injectWailsMocks } from './wails-init'
import { WAILS_MOCK_E2E_RETURN_VALUES } from '../src/test/wails-mock-defaults'

const test = base.extend({
  /** 覆盖合并表中的方法返回值（{ 方法名: 值|描述符 }），默认空覆盖 */
  wailsOverrides: [{}, { option: true }],
  page: async ({ page, wailsOverrides }, use) => {
    await page.addInitScript(injectWailsMocks, {
      ...WAILS_MOCK_E2E_RETURN_VALUES,
      ...wailsOverrides
    })
    await use(page)
  }
})

/**
 * 读取页面内 mock 后端的调用记录，可选按方法名过滤。
 * @param {import('@playwright/test').Page} page
 * @param {string} [method] 只保留该 bound method 的记录；缺省返回全部
 * @returns {Promise<Array<{method: string, args: Array}>>} 按调用时间序
 */
async function getWailsCalls(page, method) {
  return page.evaluate(
    (m) => window.__wailsCalls
      .filter((c) => !m || c.method === m)
      .map((c) => ({ method: c.method, args: c.args })),
    method
  )
}

/**
 * 放行 `__deferred__` 描述符挂起的调用（FIFO 取最早挂起的一个），以 value resolve。
 * 用于先断言「在途中间态」（行级 loading / 信号量在途数 / 进度条），再放行结果的链路
 * （批量重试编排、素材购物栏合并导出等）。
 * @param {import('@playwright/test').Page} page
 * @param {string} key 描述符 `__deferred__` 的键名
 * @param {*} value resolve 载荷（纯 JSON，可结构化克隆）
 * @returns {Promise<boolean>} 是否存在挂起项（false = 无在途调用可放行）
 */
async function releaseDeferred(page, key, value) {
  return page.evaluate(({ key, value }) => {
    const bucket = (window.__wailsDeferred || {})[key] || []
    const entry = bucket.shift()
    if (!entry) return false
    entry.resolve(value)
    return true
  }, { key, value })
}

/**
 * 读取 `__deferred__` 指定键当前挂起（未放行）的调用数。
 * 断言信号量限流中间态（如并发上限 3 时第 4 个调用仍在排队）。
 * @param {import('@playwright/test').Page} page
 * @param {string} key 描述符 `__deferred__` 的键名
 * @returns {Promise<number>}
 */
async function pendingDeferredCount(page, key) {
  return page.evaluate((k) => ((window.__wailsDeferred || {})[k] || []).length, key)
}

/**
 * 页面上下文派发 Wails 事件（经 mock 的 runtime.EventsEmit 同步广播到全部已注册
 * handler，语义与真实后端 EventsEmit 推送一致，多组件共听同事件均收到）。
 * 用于驱动「事件先于 bound method resolve」的进度中间态（如素材购物栏导出进度，
 * 真实链路事件在合并期间推送，`__events__` 描述符的 setTimeout 派发晚于 resolve）。
 * @param {import('@playwright/test').Page} page
 * @param {string} event 事件名
 * @param {*} payload 事件载荷（纯 JSON，可结构化克隆）
 */
async function emitWailsEvent(page, event, payload) {
  await page.evaluate(([name, pl]) => window.runtime.EventsEmit(name, pl), [event, payload])
}

export { test, expect, getWailsCalls, releaseDeferred, pendingDeferredCount, emitWailsEvent }
