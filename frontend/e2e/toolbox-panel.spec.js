/**
 * 工具箱 Panel 化主链 E2E（2026-10-19 工具箱全窗口面板化重构不变式，mock 驱动）。
 *
 * 覆盖：
 * 1. 全窗口面板切换互斥：活动栏激活工具箱 → 三栏区（.main-panes）隐藏、面板占满；
 *    关闭 → 三栏恢复（独占一级面板语义，与 AiFunctionPanel 等同构）
 * 2. 左侧清单工具切换：component :is 卸载重建语义（ProcessManagerPanel 每次切入
 *    重新 onMounted 枚举进程，切走即卸载）
 * 3. 进程管理终止主链：isSelf 硬禁、停止确认框（单次确认链）、StopProcess 参数
 *    （pid + killTree）、等待期行级防重（该行按钮禁用 + loading）、优雅停止生效
 *    toast（method graceful）+ 终止后列表刷新
 *
 * mock 数据源：ListProcesses/StopProcess/KillProcess 缺省已在 wails-mock-defaults.js
 * 基础表（形状对齐 model.ProcessInfo / ProcessStopResult）；终止链用 __deferred__
 * 挂起解析器驱动「等待期」中间态。
 */
import { test, expect, getWailsCalls, releaseDeferred } from './fixtures'

/** 打开工具箱面板（活动栏第 4 项 nth(3)），移开鼠标防 tooltip 遮挡 */
async function openToolbox(page) {
  await page.goto('/')
  await page.locator('.activity-bar-item').nth(3).click()
  await page.mouse.move(400, 400)
  const panel = page.locator('.toolbox-panel')
  await expect(panel).toBeVisible()
  return panel
}

test.describe('工具箱：全窗口面板切换互斥', () => {
  test('激活工具箱三栏隐藏，关闭后三栏恢复且活动态流转', async ({ page }) => {
    const panel = await openToolbox(page)

    // 面板化互斥不变式：激活时三栏区隐藏（v-show 仅 directory 可见）
    await expect(page.locator('.main-panes')).toBeHidden()
    // 活动栏 active 态跟随
    await expect(page.locator('.activity-bar-item').nth(3)).toHaveClass(/is-active/)
    // 左侧功能清单 7 工具，默认选中首个「拷贝到」
    const nav = panel.locator('.toolbox-nav-item')
    await expect(nav).toHaveCount(7)
    await expect(nav.filter({ hasText: '拷贝到' })).toHaveClass(/is-active/)

    // 关闭 → 切回 directory，三栏恢复
    await panel.locator('.toolbox-close').click()
    await expect(panel).toBeHidden()
    await expect(page.locator('.main-panes')).toBeVisible()
    await expect(page.locator('.activity-bar-item').nth(0)).toHaveClass(/is-active/)
  })
})

test.describe('工具箱：工具切换卸载重建', () => {
  test.use({
    wailsOverrides: {
      // 环境变量面板挂载即枚举（默认 fixture 三组形态），与进程管理互为切换对
      ListProcesses: []
    }
  })

  test('切入进程管理挂载枚举，切走卸载再切回重新挂载枚举', async ({ page }) => {
    const panel = await openToolbox(page)
    const nav = panel.locator('.toolbox-nav-item')

    // 切到进程管理：component :is 挂载 → onMounted 即枚举（1 次）
    await nav.filter({ hasText: '进程管理' }).click()
    await expect(page.locator('.process-manager-panel')).toBeVisible()
    await expect.poll(async () => (await getWailsCalls(page, 'ListProcesses')).length).toBe(1)

    // 切到环境变量：进程面板卸载（v-if 卸载重建，非 v-show 驻留）
    await nav.filter({ hasText: '环境变量' }).click()
    await expect(page.locator('.env-var-panel')).toBeVisible()
    await expect(page.locator('.process-manager-panel')).toHaveCount(0)

    // 切回进程管理：重新挂载再次枚举（等价原弹窗 destroy-on-close + @open 语义）
    await nav.filter({ hasText: '进程管理' }).click()
    await expect(page.locator('.process-manager-panel')).toBeVisible()
    await expect.poll(async () => (await getWailsCalls(page, 'ListProcesses')).length).toBe(2)
  })
})

test.describe('工具箱：进程管理终止主链', () => {
  // 形状对齐 model.ProcessInfo：isSelf/isSystem 由后端填充（PID + 父进程链判据），
  // 前端只消费标志不自判
  const processRows = [
    { name: 'workbench.exe', pid: 1000, ports: [], memoryKB: 0, isSelf: true, isSystem: false },
    {
      name: 'node.exe',
      pid: 4242,
      ports: [{ port: 8080, protocol: 'TCP', state: 'LISTENING' }],
      memoryKB: 51200,
      isSelf: false,
      isSystem: false
    }
  ]
  test.use({
    wailsOverrides: {
      ListProcesses: processRows,
      // 挂起解析器：断言行级防重中间态后放行优雅停止结果
      StopProcess: { __deferred__: 'pm-stop' }
    }
  })

  test('isSelf 硬禁 + 停止确认链 + 等待期行级防重 + 优雅停止生效刷新', async ({ page }) => {
    const panel = await openToolbox(page)
    await panel.locator('.toolbox-nav-item').filter({ hasText: '进程管理' }).click()

    const table = page.locator('.process-manager-panel')
    const rows = table.locator('.el-table__row')
    await expect(rows).toHaveCount(2)

    // 本应用行：isSelf 硬禁（L1，后端标志前端只消费），两按钮均不可点
    const selfRow = rows.filter({ hasText: 'workbench.exe' })
    await expect(selfRow.getByRole('button', { name: '停止' })).toBeDisabled()
    await expect(selfRow.getByRole('button', { name: '强杀' })).toBeDisabled()

    // 非本应用行 → 停止：确认框（单次确认链，裸选择器安全）
    const targetRow = rows.filter({ hasText: 'node.exe' })
    await targetRow.getByRole('button', { name: '停止' }).click()
    const msgbox = page.locator('.el-message-box')
    await expect(msgbox).toBeVisible()
    await expect(msgbox).toContainText('确定结束进程「node.exe」（PID 4242）')
    await msgbox.getByRole('button', { name: '确定结束' }).click()

    // 等待期行级防重：该行两按钮禁用 + loading（最坏 11s 在途窗口防重复触发）
    await expect(targetRow.getByRole('button', { name: '停止' })).toBeDisabled()
    await expect(targetRow.getByRole('button', { name: '强杀' })).toBeDisabled()
    await expect.poll(async () => (await getWailsCalls(page, 'StopProcess')).length).toBe(1)
    // 调用参数：pid + 进程树开关（默认关闭 = false）
    expect((await getWailsCalls(page, 'StopProcess'))[0].args).toEqual([4242, false])

    // 放行优雅停止实测生效（method graceful）→ 成功 toast + 终止后自动刷新列表
    await releaseDeferred(page, 'pm-stop', { stopped: true, method: 'graceful' })
    await expect(page.locator('.el-message').filter({ hasText: '进程已正常退出' })).toBeVisible()
    await expect.poll(async () => (await getWailsCalls(page, 'ListProcesses')).length).toBe(2)

    // 等待态解除：按钮恢复可用
    await expect(targetRow.getByRole('button', { name: '停止' })).toBeEnabled()
  })
})
