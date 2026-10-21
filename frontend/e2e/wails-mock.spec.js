/**
 * Wails mock 基建自证用例：验证 wails-init.js 事件派发语义对齐真实 Wails runtime。
 *
 * 派发收敛在单点 dispatch（快照拷贝 + 正序遍历 + per-handler try/catch），
 * 两处硬化点（docs/spec/e2e-testing.md §6 登记）：
 * - 单个 handler 抛错不中断其余同名 handler、不上抛（真实 runtime 事件派发
 *   不因订阅者异常中断）；
 * - EventsOnce 派发期间 splice 自移除只影响 live 数组、不影响本次遍历序列，
 *   同事件多 handler 时不再因索引左移跳过后续 handler，且触发一次后自移除、
 *   同事件再派发不再收到。
 * 另覆盖 EventsOn 返回的注销闭包（2026-10-21 第二批基建收口）：按引用 splice
 * 本 handler，对齐真实 Wails runtime 的「EventsOn 返回取消函数」契约——组件
 * 卸载精准注销自身监听器，多组件共听同事件不误删其余监听。
 * 不经应用组件、直接驱动 window.runtime 与 window.__probe，属 mock 基建层断言。
 */
import { test, expect, emitWailsEvent } from './fixtures'

test.describe('Wails mock 事件派发基建', () => {
  test('多 handler 含抛错 handler 时其余仍按注册序收到事件', async ({ page }) => {
    await page.goto('/')

    // 注册 3 个同名 handler：第 2 个抛错，第 1/3 个把 payload push 到 window.__probe
    await page.evaluate(() => {
      window.__probe = []
      window.runtime.EventsOn('e2e-mock-probe', (payload) => {
        window.__probe.push(['first', payload])
      })
      window.runtime.EventsOn('e2e-mock-probe', () => {
        throw new Error('boom')
      })
      window.runtime.EventsOn('e2e-mock-probe', (payload) => {
        window.__probe.push(['third', payload])
      })
    })

    // 派发不因第 2 个 handler 抛错中断/上抛（emitWailsEvent 正常 await 返回）
    await emitWailsEvent(page, 'e2e-mock-probe', { n: 1 })

    const probe = await page.evaluate(() => window.__probe)
    expect(probe).toEqual([
      ['first', { n: 1 }],
      ['third', { n: 1 }]
    ])
  })

  test('EventsOnce 触发一次后自移除，且自移除不让后续注册的 handler 被跳过', async ({ page }) => {
    await page.goto('/')

    // once 刻意先注册：派发期间其 splice 自移除使 live 数组左移，
    // 若按 live 数组 forEach 索引推进，后面的 always handler 会被跳过
    await page.evaluate(() => {
      window.__probeOnce = []
      window.runtime.EventsOnce('e2e-mock-once', (payload) => {
        window.__probeOnce.push(['once', payload])
      })
      window.runtime.EventsOn('e2e-mock-once', (payload) => {
        window.__probeOnce.push(['always', payload])
      })
    })

    await emitWailsEvent(page, 'e2e-mock-once', { turn: 1 })
    // 第二次派发：once 已自移除不再收到，always 持续收到
    await emitWailsEvent(page, 'e2e-mock-once', { turn: 2 })

    const probe = await page.evaluate(() => window.__probeOnce)
    expect(probe).toEqual([
      ['once', { turn: 1 }],
      ['always', { turn: 1 }],
      ['always', { turn: 2 }]
    ])
  })

  test('EventsOn 返回的注销闭包按引用移除本 handler，其余监听不受影响', async ({ page }) => {
    await page.goto('/')

    // 注册 2 个同名 handler，保留第一个返回的 off 闭包（组件卸载即调用的形态）
    await page.evaluate(() => {
      window.__probeOff = []
      window.__offFirst = window.runtime.EventsOn('e2e-mock-off', (payload) => {
        window.__probeOff.push(['first', payload])
      })
      window.runtime.EventsOn('e2e-mock-off', (payload) => {
        window.__probeOff.push(['second', payload])
      })
    })
    expect(await page.evaluate(() => window.__wailsEventHandlers['e2e-mock-off'].length)).toBe(2)

    // 调第一个返回的 off 闭包：仅移除自身，注册表数量回落为 1
    await page.evaluate(() => window.__offFirst())
    expect(await page.evaluate(() => window.__wailsEventHandlers['e2e-mock-off'].length)).toBe(1)

    // 派发仅剩者收到，已注销的 handler 不再收到
    await emitWailsEvent(page, 'e2e-mock-off', { n: 7 })
    expect(await page.evaluate(() => window.__probeOff)).toEqual([['second', { n: 7 }]])
  })
})
