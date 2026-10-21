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
 * 4. 端口占用一览与行级防重（PortListPanel，2026-10-21 第二批）：空关键字默认视图
 *    扁平化一行一端口（按端口号升序）、两行同 PID 异端口 fixture 下终止任一行时
 *    同 PID 另一行同步进入在途态（pendingPids 按 PID 共享，行级防重跨扁平行生效）、
 *    StopProcess(pid, false) 参数断言
 * 5. 进程强杀链与失败分流（第一批 #4 留痕收口，ProcessManagerPanel terminate
 *    action==='kill'）：L3 确认框（标题「强制结束进程」+ danger 确认钮 + 不等待
 *    保存文案）、KillProcess(pid, killTree) 参数断言（进程树开关透传）、forced
 *    成功文案；失败分流——ProcessNotFound ∈ WARNING_CODES → warning toast
 *    （裸 message 无前缀）+ 自动刷新列表
 *
 * mock 数据源：ListProcesses/StopProcess/KillProcess 缺省已在 wails-mock-defaults.js
 * 基础表（形状对齐 model.ProcessInfo / ProcessStopResult）；终止链用 __deferred__
 * 挂起解析器驱动「等待期」中间态，失败分流用 {__error__, __code__} 描述符驱动
 * 结构化 reject（对齐 Wails ErrorFormatter 的 {code, message} AppError 形态）。
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

test.describe('工具箱：端口占用一览与行级防重', () => {
  // 两行同 PID 异端口：PortListPanel 扁平化一行一端口，pendingPids 按 PID 共享 →
  // 终止任一行，同 PID 另一行同步进入在途态（行级防重跨扁平行生效）
  const samePidRows = [
    {
      name: 'node.exe',
      pid: 4242,
      ports: [{ port: 8080, protocol: 'TCP', state: 'LISTENING', localAddr: '0.0.0.0:8080' }],
      memoryKB: 51200,
      isSelf: false,
      isSystem: false
    },
    {
      name: 'node.exe',
      pid: 4242,
      ports: [{ port: 9090, protocol: 'TCP', state: 'LISTENING', localAddr: '0.0.0.0:9090' }],
      memoryKB: 51200,
      isSelf: false,
      isSystem: false
    }
  ]
  test.use({
    wailsOverrides: {
      ListProcesses: samePidRows,
      StopProcess: { __deferred__: 'port-stop' }
    }
  })

  test('默认视图扁平化一行一端口，同 PID 行级防重同步在途 + StopProcess 参数', async ({ page }) => {
    const panel = await openToolbox(page)
    await panel.locator('.toolbox-nav-item').filter({ hasText: '端口占用' }).click()
    const view = page.locator('.port-list-panel')
    await expect(view).toBeVisible()
    await expect.poll(async () => (await getWailsCalls(page, 'ListProcesses')).length).toBe(1)

    // 扁平化：两行同 PID 进程 → 一行一端口，按端口号升序
    await expect(view.locator('.pl-meta')).toContainText('共 2 个监听端口')
    const rows = view.locator('.el-table__row')
    await expect(rows).toHaveCount(2)
    await expect(rows.nth(0)).toContainText('8080')
    await expect(rows.nth(1)).toContainText('9090')
    await expect(rows.nth(0)).toContainText('LISTENING')

    // 终止 8080 行 → 确认框（单次确认链；portCount=1 无连带端口文案）
    await rows.nth(0).getByRole('button', { name: '终止' }).click()
    const msgbox = page.locator('.el-message-box')
    await expect(msgbox).toBeVisible()
    await expect(msgbox).toContainText('确定结束进程「node.exe」（PID 4242）')
    await msgbox.getByRole('button', { name: '确定结束' }).click()

    // 行级防重按 PID：同 PID 两行同步在途（按钮禁用 + loading），防重复终止
    await expect(rows.nth(0).getByRole('button', { name: '终止' })).toBeDisabled()
    await expect(rows.nth(1).getByRole('button', { name: '终止' })).toBeDisabled()
    await expect.poll(async () => (await getWailsCalls(page, 'StopProcess')).length).toBe(1)
    // 端口面板固定 killTree=false（无进程树开关）
    expect((await getWailsCalls(page, 'StopProcess'))[0].args).toEqual([4242, false])

    // 放行优雅停止实测生效 → toast + 自动刷新列表，等待态解除
    await releaseDeferred(page, 'port-stop', { stopped: true, method: 'graceful' })
    await expect(page.locator('.el-message').filter({ hasText: '进程已正常退出' })).toBeVisible()
    await expect.poll(async () => (await getWailsCalls(page, 'ListProcesses')).length).toBe(2)
    await expect(rows.nth(1).getByRole('button', { name: '终止' })).toBeEnabled()
  })
})

test.describe('工具箱：进程强杀链（L3 确认 + KillProcess 参数）', () => {
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
      KillProcess: { __deferred__: 'pm-kill' }
    }
  })

  test('强杀 L3 确认 danger 按钮 + KillProcess(pid, killTree) 参数 + forced 文案刷新', async ({ page }) => {
    const panel = await openToolbox(page)
    await panel.locator('.toolbox-nav-item').filter({ hasText: '进程管理' }).click()
    const rows = page.locator('.process-manager-panel .el-table__row')
    await expect(rows).toHaveCount(2)

    const targetRow = rows.filter({ hasText: 'node.exe' })
    await targetRow.getByRole('button', { name: '强杀' }).click()

    // L3 确认框：标题「强制结束进程」+ danger 确认钮 + 强调不等待进程保存数据
    const msgbox = page.locator('.el-message-box')
    await expect(msgbox).toBeVisible()
    await expect(msgbox.locator('.el-message-box__title')).toContainText('强制结束进程')
    await expect(msgbox).toContainText('强制结束不会等待进程保存数据')
    const confirmBtn = msgbox.getByRole('button', { name: '确定结束' })
    await expect(confirmBtn).toHaveClass(/el-button--danger/)
    await confirmBtn.click()

    // KillProcess(pid, killTree)：进程树开关默认关闭
    await expect.poll(async () => (await getWailsCalls(page, 'KillProcess')).length).toBe(1)
    expect((await getWailsCalls(page, 'KillProcess'))[0].args).toEqual([4242, false])

    // 强杀成功文案（与停止链 graceful 文案「进程已正常退出」分流）+ 自动刷新
    await releaseDeferred(page, 'pm-kill', null)
    await expect(page.locator('.el-message').filter({ hasText: '进程已强制结束' })).toBeVisible()
    await expect.poll(async () => (await getWailsCalls(page, 'ListProcesses')).length).toBe(2)
    await expect(targetRow.getByRole('button', { name: '强杀' })).toBeEnabled()
  })
})

test.describe('工具箱：强杀失败分流（ProcessNotFound 自动刷新）', () => {
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
      // 结构化 reject {code, message}（对齐 Wails ErrorFormatter AppError 形态）：
      // ProcessNotFound ∈ WARNING_CODES → warning toast + 自动刷新列表
      KillProcess: { __error__: '目标进程已不存在，列表已过期', __code__: 'E_PROCESS_NOT_FOUND' }
    }
  })

  test('强杀失败 warning toast 无前缀 + 进程树开关透传 + 自动刷新', async ({ page }) => {
    const panel = await openToolbox(page)
    await panel.locator('.toolbox-nav-item').filter({ hasText: '进程管理' }).click()
    const rows = page.locator('.process-manager-panel .el-table__row')
    await expect(rows).toHaveCount(2)

    // 勾选「同时结束子进程（进程树）」→ killTree=true 透传 KillProcess
    await page.locator('.pm-tree-checkbox').click()

    const targetRow = rows.filter({ hasText: 'node.exe' })
    await targetRow.getByRole('button', { name: '强杀' }).click()
    const msgbox = page.locator('.el-message-box')
    await expect(msgbox).toBeVisible()
    await expect(msgbox.locator('.el-message-box__title')).toContainText('强制结束进程')
    await msgbox.getByRole('button', { name: '确定结束' }).click()

    // 失败分流：warning toast（裸 message，无「结束进程失败:」error 前缀）+ 自动刷新
    await expect(page.locator('.el-message--warning').filter({ hasText: '目标进程已不存在' })).toBeVisible()
    expect((await getWailsCalls(page, 'KillProcess'))[0].args).toEqual([4242, true])
    await expect.poll(async () => (await getWailsCalls(page, 'ListProcesses')).length).toBe(2)
  })
})
