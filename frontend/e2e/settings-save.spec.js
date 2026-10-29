/**
 * 设置面板保存整体提交链 E2E（settings store 合并写语义，mock 驱动）。
 *
 * 覆盖：
 * 1. 整体提交链：修改单项（Obsidian 路径）→ SaveSettings 收到「磁盘基底 + 本组覆盖」
 *    的完整 payload（未改段原样保留——整体提交语义下漏段 = 清掉其他全部设置）
 * 2. 「失败基底 null 禁保存」不变式：GetSettings reject 时保存链在读取基底即中止，
 *    SaveSettings 永不发出（空壳整体提交清段风险的后端读取侧防线）
 * 3. 保存后热更：主题切换（store 合并写 + App.vue watch 即时应用 html.dark）+
 *    终端字号变更（设置页与终端工具栏共享 store 字段 → 已开终端热更 + ResizeTerminal
 *    通知 PTY）
 */
import { test, expect, getWailsCalls } from './fixtures'

/** 磁盘基底设置（形状对齐 model.AppSettings，前端消费字段子集） */
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

/** 打开设置弹窗（活动栏设置图标，nth(7)） */
async function openSettings(page) {
  await page.locator('.activity-bar-item').nth(7).click()
  const dialog = page.locator('.settings-dialog')
  await expect(dialog).toBeVisible()
  return dialog
}

test.describe('设置：整体提交链（基底 + 本组覆盖）', () => {
  test.use({ wailsOverrides: { GetSettings: baseSettings } })

  test('修改单项后 SaveSettings 收到完整 payload 且未改段原样保留', async ({ page }) => {
    await page.goto('/')
    const dialog = await openSettings(page)

    // 修改 Obsidian 路径（失焦触发 @change → saveGeneralSettings 合并写）
    const obsidianInput = dialog.locator('.settings-item').filter({ hasText: 'Obsidian 程序路径' }).locator('input')
    await obsidianInput.fill('D:/tools/Obsidian/Obsidian.exe')
    await obsidianInput.press('Tab')

    // 整体提交：SaveSettings 恰一次，payload = 基底 + 覆盖字段
    await expect.poll(async () => (await getWailsCalls(page, 'SaveSettings')).length).toBe(1)
    const payload = (await getWailsCalls(page, 'SaveSettings'))[0].args[0]
    expect(payload.obsidianPath).toBe('D:/tools/Obsidian/Obsidian.exe')
    // 未配置的 tessoa 路径随整体提交原样保留（不因缺字段静默丢失）
    expect(payload.tessoaPath).toBe('')
    // 未改段原样保留（整体提交语义下任一缺段 = 静默清空该段设置）
    expect(payload.themeMode).toBe('light')
    expect(payload.searchExcludeDirs).toEqual(['node_modules', 'dist'])
    expect(payload.searchExcludeFiles).toEqual(['.log'])
    expect(payload.terminalFontSize).toBe(14)
    expect(payload.diffToolPath).toContain('WinMergeU.exe')
    expect(payload.defaultShell).toBe('powershell')
  })
})

test.describe('设置：GetSettings 失败基底禁保存不变式', () => {
  test.use({
    wailsOverrides: {
      GetSettings: { __error__: 'settings.json 读取失败' }
    }
  })

  test('基底读取失败时保存链中止，SaveSettings 永不发出（空壳提交防线）', async ({ page }) => {
    await page.goto('/')
    const dialog = await openSettings(page)

    // 失败基底：面板回退默认值（Obsidian 输入为空）
    const obsidianInput = dialog.locator('.settings-item').filter({ hasText: 'Obsidian 程序路径' }).locator('input')
    await expect(obsidianInput).toHaveValue('')

    // 触发保存链：GetSettings 再读基底（失败）→ 链路在保存前中止
    const readsBefore = (await getWailsCalls(page, 'GetSettings')).length
    await obsidianInput.fill('D:/tools/Obsidian/Obsidian.exe')
    await obsidianInput.press('Tab')

    // 标记：保存链的基底读取已发生（失败），此后 SaveSettings 恒为零次
    await expect.poll(async () => (await getWailsCalls(page, 'GetSettings')).length).toBeGreaterThan(readsBefore)
    expect((await getWailsCalls(page, 'SaveSettings')).length).toBe(0)
  })
})

test.describe('设置：保存后热更生效', () => {
  test.use({ wailsOverrides: { GetSettings: baseSettings } })

  test('主题切换即时应用 html.dark，终端字号热更联动 ResizeTerminal', async ({ page }) => {
    await page.goto('/')

    // 终端面板先展开（字号热更的消费端在线，首个 tab 会话建立）
    await page.locator('.activity-bar-item').nth(8).click()
    const terminal = page.locator('.terminal-panel')
    await expect(terminal.locator('.shell-badge')).toBeVisible()
    await expect(terminal.locator('.font-size-value')).toHaveText('14')

    // 设置弹窗切暗色主题（store saveTheme 合并写保存链）
    const dialog = await openSettings(page)
    await dialog.locator('.el-radio').filter({ hasText: '暗色' }).click()

    // 主题热更即时生效（App.vue watch resolvedTheme → html.dark class）
    await expect.poll(() => page.evaluate(() => document.documentElement.classList.contains('dark'))).toBe(true)
    // SaveSettings 收到 themeMode: 'dark'（基底 + 本组覆盖）
    const themeSaved = (await getWailsCalls(page, 'SaveSettings')).some((c) => c.args[0]?.themeMode === 'dark')
    expect(themeSaved).toBe(true)

    // 关闭设置弹窗（ESC），终端工具栏 A+ 步进（与设置页共享 store 字段）
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
    await terminal.locator('.font-step-btn').filter({ hasText: 'A+' }).click()

    // 字号热更：工具栏数值 14 → 15（store clamp 界内步进），已开终端实时生效
    await expect(terminal.locator('.font-size-value')).toHaveText('15')
    // PTY 尺寸同步：useTerminal watch → resize() → ResizeTerminal IPC
    await expect.poll(async () => (await getWailsCalls(page, 'ResizeTerminal')).length).toBeGreaterThanOrEqual(1)
    // 保存链：SaveSettings 收到 terminalFontSize: 15
    const appearanceSaved = (await getWailsCalls(page, 'SaveSettings')).some((c) => c.args[0]?.terminalFontSize === 15)
    expect(appearanceSaved).toBe(true)
  })
})
