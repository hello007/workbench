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

export { test, expect, getWailsCalls }
