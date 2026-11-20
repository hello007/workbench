/**
 * window 级监听注册计数 helper（E2E 唯一入口）。
 *
 * 背景（docs/spec/e2e-testing.md §12.1）：全屏 ESC 类用例需要观测「组件是否在 window 上
 * 注册/注销了监听」，历史实现是各用例内联覆写 window.addEventListener 并断言绝对值
 * toBe(0)。绝对基线曾因新增合法注册点（FolderTree 菜单监听）而误报，因此本 helper
 * 只提供**相对增量**读取面：先取 `listenerSnapshot` 作基线，操作后再取
 * `listenerDelta`，断言「相对增加了 N 次」而非「当前共 N 个」。
 *
 * API 刻意不提供「断言绝对值」的便捷入口（不导出 toBe 式方法、不 import expect），
 * 纯数据返回，断言留在 spec。E2E 内联自写 addEventListener 覆写由
 * `scripts/check-e2e-listener-hooks.mjs` 静态拦截。
 *
 * 口径声明（读增量前必知）：
 * - 计数是 add/remove 的**调用次数**，不是有效注册数：原生对同 type+listener+capture
 *   的重复 add 会去重、对未注册监听的 remove 为 no-op，这里仍各计 1 次
 *   （TerminalPanel.vue 全屏 watch 的 immediate 挂载即产生一次 no-op remove；用例
 *   基线取在挂载之后，故不影响断言）。
 * - 统计随 document 生命周期：每次 goto/reload 后由 init script 重新归零，
 *   基线快照与增量必须在同一 document 内读取，跨导航取 delta 会得到负值。
 *
 * 用法（安装必须在 page.goto 之前，与既有用例一致）：
 *
 *   import { test, expect } from './fixtures'
 *   import { installListenerCounter, listenerSnapshot, listenerDelta } from './listenerCounter'
 *
 *   await installListenerCounter(page, ['keydown'])
 *   const panel = await openTerminal(page)     // 内部 page.goto('/')
 *   const base = await listenerSnapshot(page)  // { keydown: { added, removed } }
 *   await panel.locator('.fullscreen-btn').click()
 *   expect((await listenerDelta(page, base)).keydown.added).toBe(1)
 *
 * 注入幂等：页面内以 `window.__listenerCounterInstalled` 守卫，同一 page 重复调用不会
 * 重复包装；重复调用传入的新事件类型会合并进跟踪集合（不重置已有计数）。
 */

/**
 * 注入监听计数器。必须在 `page.goto` 之前调用（addInitScript 只对后续导航生效）。
 *
 * 覆写 window.addEventListener / window.removeEventListener，按事件类型累加
 * `window.__listenerStats[type] = { added, removed }`；未跟踪的类型直接透传不计数。
 * 原方法以 bind(window) 保存后调用，保持 this 语义与浏览器原生行为一致。
 *
 * @param {import('@playwright/test').Page} page
 * @param {string[]} [types=['keydown']] 需要计数的事件类型（支持多类型，按类型分组互不干扰）
 */
async function installListenerCounter(page, types = ['keydown']) {
  await page.addInitScript((trackedTypes) => {
    window.__listenerStats = window.__listenerStats || {}
    window.__listenerTrackedTypes = window.__listenerTrackedTypes || []
    for (const type of trackedTypes) {
      if (!window.__listenerTrackedTypes.includes(type)) {
        window.__listenerTrackedTypes.push(type)
      }
      if (!window.__listenerStats[type]) {
        window.__listenerStats[type] = { added: 0, removed: 0 }
      }
    }
    // 幂等守卫：同一页面只包装一次，重复 addInitScript（重复调用/多导航）不叠加计数
    if (window.__listenerCounterInstalled) return
    window.__listenerCounterInstalled = true

    const originalAdd = window.addEventListener.bind(window)
    const originalRemove = window.removeEventListener.bind(window)
    window.addEventListener = function (type, listener, options) {
      if (window.__listenerTrackedTypes.includes(type)) {
        window.__listenerStats[type].added++
      }
      return originalAdd(type, listener, options)
    }
    window.removeEventListener = function (type, listener, options) {
      if (window.__listenerTrackedTypes.includes(type)) {
        window.__listenerStats[type].removed++
      }
      return originalRemove(type, listener, options)
    }
  }, types)
}

/**
 * 读取当前累计计数（浅拷贝，避免用例持有页面内对象引用）。
 * @param {import('@playwright/test').Page} page
 * @returns {Promise<Record<string, {added: number, removed: number}>>}
 */
async function listenerSnapshot(page) {
  return page.evaluate(() => {
    const out = {}
    for (const [type, stats] of Object.entries(window.__listenerStats || {})) {
      out[type] = { added: stats.added, removed: stats.removed }
    }
    return out
  })
}

/**
 * 计算当前计数相对基线的增量：`{ [type]: { added, removed } }`。
 * 基线必须是 `listenerSnapshot(page)` 的返回值，且覆盖当前出现的全部类型——缺基线、
 * 基线非对象、或基线缺少某类型统计时直接抛错（不退化按 0 起算，避免掩盖
 * 「installListenerCounter 安装晚于 goto」或跨导航引用旧基线这两类错误）。
 * @param {import('@playwright/test').Page} page
 * @param {Record<string, {added: number, removed: number}>} base listenerSnapshot 返回值
 * @returns {Promise<Record<string, {added: number, removed: number}>>}
 */
async function listenerDelta(page, base) {
  if (!base || typeof base !== 'object' || Array.isArray(base)) {
    throw new Error(
      `listenerDelta: 基线无效，需传入 listenerSnapshot(page) 的返回值，实际收到 ${JSON.stringify(base)}`
    )
  }
  const current = await listenerSnapshot(page)
  const out = {}
  for (const [type, stats] of Object.entries(current)) {
    if (!base[type]) {
      throw new Error(
        `listenerDelta: 基线缺少监听计数类型 "${type}"——installListenerCounter 须在 page.goto 之前安装该类型，且基线与增量须在同一 document 内读取`
      )
    }
    out[type] = {
      added: stats.added - base[type].added,
      removed: stats.removed - base[type].removed
    }
  }
  return out
}

export { installListenerCounter, listenerSnapshot, listenerDelta }
