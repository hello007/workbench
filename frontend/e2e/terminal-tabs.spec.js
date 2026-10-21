/**
 * 终端多 tab 生命周期 E2E（terminal-multi-session.md 契约的前端编排面，mock 驱动）。
 *
 * 真实 pty 在后端，E2E 只验前端编排（tab 状态机 + 调用序列），断言聚焦 tab 列表 UI
 * 与面板状态，不深入 xterm 缓冲区内容（xterm 在浏览器 E2E 真实渲染，非 jsdom）。
 *
 * 覆盖：
 * 1. 展开面板冷启动：首个 tab 自动创建（CreateTerminal 参数断言）+ 单 tab 徽章模式
 * 2. 新建第二 tab + 激活切换：tab-strip 出现、isActive 流转、双容器 v-show 保 DOM
 *    （容器不销毁，仅 display 切换）、切换不新建会话
 * 3. 关闭非活动 tab（useTerminalTabs closeTab splice + 快照 getter 投影唯一暴露路径）：
 *    CloseTerminal 收到被关 tab 的 sessionId（__sequence__ 分配）、列表收缩退徽章模式；
 *    收起再展开面板不重复建会话（冷启动判定 tabs.length === 0 短路——末 tab 无关闭
 *    入口（D1 设计），「关闭全部 → 冷启动重建」不可达，冷启动路径由用例 1 覆盖）
 *
 * mock 前置：终端 5 方法缺省已入 wails-mock-defaults.js 基础表（单一数据源）；
 * 本 spec 用 __sequence__ 让 CreateTerminal 按调用序分配可区分的 sessionId。
 */
import { test, expect, getWailsCalls } from './fixtures'

const shellOverrides = {
  // 平台 Shell 列表（Windows 形态）：默认 Shell 解析收敛首项 powershell，
  // 终端兜底目录 fallbackTerminalDir() 依首项判定返回 C:\
  GetShellConfigs: [
    { type: 'powershell', displayName: 'PowerShell' },
    { type: 'cmd', displayName: 'CMD' }
  ],
  // 按调用序分配 sessionId（序列耗尽后 hold last），供 CloseTerminal 参数断言对号
  CreateTerminal: { __sequence__: ['session-1', 'session-2', 'session-3'] }
}

/** 展开终端面板并等待首个 tab 会话建立 */
async function openTerminal(page) {
  await page.goto('/')
  // 活动栏最后一项（nth(8)）为终端开关
  await page.locator('.activity-bar-item').nth(8).click()
  const panel = page.locator('.terminal-panel')
  await expect(panel).toBeVisible()
  await expect.poll(async () => (await getWailsCalls(page, 'CreateTerminal')).length).toBe(1)
  return panel
}

test.describe('终端多 tab：冷启动首个 tab', () => {
  test.use({ wailsOverrides: shellOverrides })

  test('展开面板自动创建首个会话，单 tab 徽章模式（无 tab-strip）', async ({ page }) => {
    const panel = await openTerminal(page)

    // 单 tab 徽章模式（D1：< 2 tab 隐藏 tab 栏）
    await expect(panel.locator('.shell-badge')).toBeVisible()
    await expect(panel.locator('.shell-name')).toHaveText('PowerShell')
    // 目录继承文件树当前目录（空）→ 平台兜底 C:\（首项 powershell 判定）
    await expect(panel.locator('.path-text')).toHaveText('C:\\')
    await expect(panel.locator('.tab-strip')).toHaveCount(0)

    // 首会话创建调用参数：目录兜底 + 设置页默认 Shell（GetSettings 空对象 → 平台首项）
    const calls = await getWailsCalls(page, 'CreateTerminal')
    expect(calls.length).toBe(1)
    expect(calls[0].args[0]).toBe('C:\\')
    expect(calls[0].args[1]).toBe('powershell')
  })
})

test.describe('终端多 tab：新建与激活切换', () => {
  test.use({ wailsOverrides: shellOverrides })

  test('新建第二 tab 出现 tab-strip，激活流转且双容器 v-show 保 DOM', async ({ page }) => {
    const panel = await openTerminal(page)

    // 新建 tab：继承目录 + 默认 Shell，新 tab 自动激活
    await panel.locator('.add-tab-btn').click()
    const tabStrip = panel.locator('.tab-strip')
    await expect(tabStrip).toBeVisible()
    const tabs = panel.locator('.terminal-tab')
    await expect(tabs).toHaveCount(2)
    await expect(tabs.nth(1)).toHaveClass(/is-active/)
    await expect(tabs.nth(1).locator('.tab-title')).toHaveText('PowerShell')

    // 第二个会话建立（ CreateTerminal 累计 2 次）
    await expect.poll(async () => (await getWailsCalls(page, 'CreateTerminal')).length).toBe(2)

    // v-show 保 DOM（禁重挂载不变式）：两个容器都存在，仅活动容器可见
    const containers = panel.locator('.terminal-container')
    await expect(containers).toHaveCount(2)
    await expect(containers.nth(1)).toBeVisible()
    await expect(containers.nth(0)).toBeHidden()

    // 切回第一个 tab：isActive 流转 + 容器 DOM 不销毁（display 切换）
    await tabs.nth(0).click()
    await expect(tabs.nth(0)).toHaveClass(/is-active/)
    await expect(tabs.nth(1)).not.toHaveClass(/is-active/)
    await expect(containers).toHaveCount(2)
    await expect(containers.nth(0)).toBeVisible()
    await expect(containers.nth(1)).toBeHidden()

    // 切换走 refit 路径不新建会话（CreateTerminal 仍为 2 次）
    expect((await getWailsCalls(page, 'CreateTerminal')).length).toBe(2)
  })
})

test.describe('终端多 tab：关闭生命周期与冷启动', () => {
  test.use({ wailsOverrides: shellOverrides })

  test('关闭非活动 tab 会话销毁且列表收缩退徽章模式，再展开不重复建会话', async ({ page }) => {
    const panel = await openTerminal(page)

    // 建第二 tab（激活态为 tab-2）
    await panel.locator('.add-tab-btn').click()
    const tabs = panel.locator('.terminal-tab')
    await expect(tabs).toHaveCount(2)
    await expect.poll(async () => (await getWailsCalls(page, 'CreateTerminal')).length).toBe(2)

    // 关闭非活动 tab（第 1 个）——activeId 不变、仅 splice 的路径（快照镜像
    // getter 投影唯一暴露点）：列表收缩为单 tab，tab 栏隐藏退徽章模式（D1），
    // 徽章展示剩余会话（PowerShell / C:\）
    await tabs.nth(0).hover()
    await tabs.nth(0).locator('.tab-close').click()
    await expect(panel.locator('.tab-strip')).toHaveCount(0)
    await expect(panel.locator('.shell-badge')).toBeVisible()
    await expect(panel.locator('.shell-name')).toHaveText('PowerShell')
    // 被关 tab 的会话销毁（CloseTerminal 收到其 sessionId，剩余 tab 会话保留）
    expect(await getWailsCalls(page, 'CloseTerminal')).toEqual([
      { method: 'CloseTerminal', args: ['session-1'] }
    ])

    // 收起再展开面板：tabs.length 非空短路冷启动分支，快照恢复 0 个 →
    // 不重复建会话（CreateTerminal 仍 2 次，无历史 tab 复原）
    await page.locator('.activity-bar-item').nth(8).click()
    await expect(panel).toBeHidden()
    await page.locator('.activity-bar-item').nth(8).click()
    await expect(panel.locator('.shell-badge')).toBeVisible()
    await expect(panel.locator('.tab-strip')).toHaveCount(0)
    expect((await getWailsCalls(page, 'CreateTerminal')).length).toBe(2)
    expect((await getWailsCalls(page, 'CloseTerminal')).length).toBe(1)

    // 再新建 tab：第 3 个会话建立，tab 栏重新出现且新 tab 激活
    await panel.locator('.add-tab-btn').click()
    await expect(panel.locator('.terminal-tab')).toHaveCount(2)
    await expect(panel.locator('.terminal-tab').nth(1)).toHaveClass(/is-active/)
    await expect.poll(async () => (await getWailsCalls(page, 'CreateTerminal')).length).toBe(3)
  })
})
