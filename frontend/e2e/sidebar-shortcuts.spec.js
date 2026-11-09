/**
 * 侧边栏快捷键与顺序自定义 E2E（活动栏 Alt+数字切换 + 设置区顺序调整链路）。
 *
 * 覆盖：
 * 1. 默认顺序下 Alt+1..9 切换对应面板（活动栏 is-active 断言），Alt+9 指向第 9
 *    个面板「临时待办」
 * 2. 磁盘持久化顺序恢复：GetSettings 带 sidebarOrder 时活动栏 DOM 顺序与
 *    Alt+编号指向同步跟随（重启保留链路）
 * 3. 设置区顺序调整：上移 → SaveSettings payload 收到新顺序 + 活动栏即时重排 +
 *    关闭设置后 Alt+1 指向新顺序第一个面板（编号跟随顺序）
 * 4. 恢复默认顺序一键回退
 * 5. 面板 tooltip 带编号提示（「工作目录 (Alt+1)」拼接）
 *
 * 键盘挂载竞态（e2e-testing.md §6.1）：goto 返回早于 Home onMounted 的全局
 * keydown 注册，「按键 + 断言」须整体包进 expect(fn).toPass() 重试（切换面板
 * 幂等，重复按键安全）。
 */
import { test, expect, getWailsCalls, activityPanel, activityAction } from './fixtures'

/** 磁盘基底设置（形状对齐 model.AppSettings，前端消费字段子集；对齐 settings-save.spec） */
const baseSettings = {
  themeMode: 'light',
  defaultShell: 'powershell',
  gpuDisabled: false,
  obsidianPath: '',
  tessoaPath: '',
  searchExcludeDirs: ['node_modules', 'dist'],
  searchExcludeFiles: ['.log'],
  terminalFontSize: 14,
  terminalFontFamily: '',
  terminalScrollback: 1000,
  diffToolName: 'winmerge',
  diffToolPath: 'C:\\Program Files\\WinMerge\\WinMergeU.exe',
  diffToolArgs: '{left} {right}'
}

const DEFAULT_ORDER = ['directory', 'ai', 'ai-chat', 'toolbox', 'assets', 'dashboard', 'stats', 'branchcompare', 'todo']

/** 打开设置弹窗并切到快捷键 tab（导航语义定位 hasText，禁 nth） */
async function openShortcutsTab(page) {
  await activityAction(page, 'settings').click()
  const dialog = page.locator('.settings-dialog')
  await expect(dialog).toBeVisible()
  await dialog.locator('.settings-nav-item').filter({ hasText: '快捷键' }).click()
  return dialog
}

test.describe('侧边栏 Alt+数字快捷键（默认顺序）', () => {
  test.use({ wailsOverrides: { GetSettings: baseSettings } })

  test('Alt+1..9 切换对应面板，Alt+9 指向临时待办', async ({ page }) => {
    await page.goto('/')
    // 挂载确认（§6.1：keydown 注册晚于 goto load 事件，先等首项可见再按键）
    await expect(activityPanel(page, 'directory')).toBeVisible()

    // Alt+2 → AI 功能（默认顺序第 2 个）；按键 + 断言整体重试（切换幂等）
    await expect(async () => {
      await page.keyboard.press('Alt+2')
      await expect(activityPanel(page, 'ai')).toHaveClass(/is-active/)
    }).toPass()

    await expect(async () => {
      await page.keyboard.press('Alt+3')
      await expect(activityPanel(page, 'ai-chat')).toHaveClass(/is-active/)
    }).toPass()

    await expect(async () => {
      await page.keyboard.press('Alt+8')
      await expect(activityPanel(page, 'branchcompare')).toHaveClass(/is-active/)
    }).toPass()

    // Alt+9（默认顺序第 9 个）→ 临时待办
    await expect(async () => {
      await page.keyboard.press('Alt+9')
      await expect(activityPanel(page, 'todo')).toHaveClass(/is-active/)
    }).toPass()

    // 回到 Alt+8 验证双向切换不漂移
    await expect(async () => {
      await page.keyboard.press('Alt+8')
      await expect(activityPanel(page, 'branchcompare')).toHaveClass(/is-active/)
    }).toPass()
    await expect(activityPanel(page, 'todo')).not.toHaveClass(/is-active/)
  })
})

test.describe('侧边栏顺序：磁盘恢复（GetSettings 带 sidebarOrder）', () => {
  test.use({
    wailsOverrides: {
      GetSettings: { ...baseSettings, sidebarOrder: ['stats', ...DEFAULT_ORDER.filter(id => id !== 'stats')] }
    }
  })

  test('活动栏顺序与 Alt+编号恢复磁盘顺序', async ({ page }) => {
    await page.goto('/')
    // 磁盘顺序恢复：首项为 stats（仓库统计）
    await expect(page.locator('[data-panel-id]').first()).toHaveAttribute('data-panel-id', 'stats')
    await expect(activityPanel(page, 'stats')).toBeVisible()
    // Alt+1 指向磁盘顺序第一个面板
    await expect(async () => {
      await page.keyboard.press('Alt+1')
      await expect(activityPanel(page, 'stats')).toHaveClass(/is-active/)
    }).toPass()
  })
})

test.describe('侧边栏顺序：设置区上移调整', () => {
  test.use({ wailsOverrides: { GetSettings: baseSettings } })

  test('SaveSettings 收到新顺序 + 活动栏重排 + Alt+1 跟随新顺序', async ({ page }) => {
    await page.goto('/')
    await expect(activityPanel(page, 'directory')).toBeVisible()

    const dialog = await openShortcutsTab(page)
    // 第一行（工作目录）下移 → [ai, directory, ...]；语义定位行内按钮
    await page.locator('.sidebar-order-item').first().getByText('下移').click()

    // SaveSettings payload 收到新顺序（持久化链路）
    await expect.poll(async () => (await getWailsCalls(page, 'SaveSettings')).length).toBe(1)
    const payload = (await getWailsCalls(page, 'SaveSettings'))[0].args[0]
    expect(payload.sidebarOrder[0]).toBe('ai')
    expect(payload.sidebarOrder[1]).toBe('directory')
    expect(payload.sidebarOrder).toHaveLength(9)

    // 活动栏即时重排（store 响应式跨组件）
    await expect(page.locator('[data-panel-id]').first()).toHaveAttribute('data-panel-id', 'ai')

    // 关闭设置弹窗后 Alt+1 指向新顺序第一个（编号跟随顺序）
    await dialog.locator('.el-dialog__headerbtn').click()
    await expect(dialog).not.toBeVisible()
    await expect(async () => {
      await page.keyboard.press('Alt+1')
      await expect(activityPanel(page, 'ai')).toHaveClass(/is-active/)
    }).toPass()
  })
})

test.describe('侧边栏顺序：恢复默认', () => {
  test.use({
    wailsOverrides: {
      GetSettings: { ...baseSettings, sidebarOrder: ['stats', ...DEFAULT_ORDER.filter(id => id !== 'stats')] }
    }
  })

  test('一键回退默认排列并持久化', async ({ page }) => {
    await page.goto('/')
    await expect(activityPanel(page, 'stats')).toBeVisible()

    const dialog = await openShortcutsTab(page)
    await dialog.getByText('侧边栏顺序').waitFor()
    await dialog.getByRole('button', { name: '恢复默认' }).click()

    // 活动栏回默认排列（首项 directory）
    await expect(page.locator('[data-panel-id]').first()).toHaveAttribute('data-panel-id', 'directory')
    await expect.poll(async () => (await getWailsCalls(page, 'SaveSettings')).length).toBe(1)
    const payload = (await getWailsCalls(page, 'SaveSettings'))[0].args[0]
    expect(payload.sidebarOrder).toEqual(DEFAULT_ORDER)
  })
})

test.describe('侧边栏 tooltip 编号提示', () => {
  test.use({ wailsOverrides: { GetSettings: baseSettings } })

  test('面板 hover tooltip 拼接 Alt 编号（默认首项「工作目录 (Alt+1)」）', async ({ page }) => {
    await page.goto('/')
    const entry = activityPanel(page, 'directory')
    await expect(entry).toBeVisible()

    await entry.hover()
    const tooltip = page.locator('.el-popper').filter({ hasText: '工作目录 (Alt+1)' })
    await expect(tooltip).toBeVisible()

    // 鼠标移离防 tooltip 遮挡后续交互（活动栏 tooltip 先例约束）
    await page.mouse.move(400, 400)
    await expect(tooltip).not.toBeVisible()
  })
})
