/**
 * 外部 diff 工具集成 E2E（mock 后端，不触发真实外部进程）。
 *
 * 覆盖点（验收范围：按钮态 + 配置持久化链路）：
 * 1. 设置面板「外部 diff 工具」区渲染：预设下拉 / 路径 / 参数模板输入
 * 2. 修改路径后 blur：SaveSettings 调用携带 diffToolName/diffToolPath/diffToolArgs 三字段
 * 3. diff 弹窗「用外部工具打开」按钮：未配置置灰；已配置可用（仅验证态，不点击，
 *    避免真实外部进程；打开链路的参数正确性由组件单测与后端单测覆盖）
 *
 * diff 数据来自 E2E 合并表 GetLocalChanges 默认值（src/app.js 等）；
 * 已配置态经 test.use({ wailsOverrides }) 覆盖 GetSettings 注入 diffToolPath。
 */
import { test, expect, getWailsCalls } from './fixtures'
import { openGitRepoPage, visibleDialog, GIT_FLOW_OVERRIDES } from './git-fixtures'

/** 已配置外部 diff 工具的 GetSettings 覆盖（形状对齐 model.AppSettings 的 diffTool 字段） */
const DIFF_TOOL_CONFIGURED = {
  diffToolName: 'winmerge',
  diffToolPath: 'C:\\Program Files\\WinMerge\\WinMergeU.exe',
  diffToolArgs: '{left} {right}'
}

test.describe('外部 diff 工具集成', () => {
  test.describe('设置面板配置区', () => {
    test('设置区渲染预设/路径/参数模板三项', async ({ page }) => {
      await page.setViewportSize({ width: 1920, height: 1080 })
      await page.goto('/')
      await expect(page).toHaveURL(/#\/$/)

      // ActivityBar 倒数第二个图标为设置（最后一个为终端切换）
      await page.locator('.activity-bar-item').nth(-2).click()
      const settingsDialog = visibleDialog(page, '设置')
      await expect(settingsDialog).toBeVisible()

      const diffSection = settingsDialog.locator('.settings-section-title', { hasText: '外部 diff 工具' })
      await expect(diffSection).toBeVisible()
      await expect(settingsDialog.locator('.diff-tool-preset-select')).toBeVisible()
      await expect(settingsDialog.locator('input[placeholder*="WinMergeU.exe"]')).toBeVisible()
      await expect(settingsDialog.locator('input[placeholder="{left} {right}"]')).toBeVisible()
    })

    test('修改路径后 blur：SaveSettings 携带三个 diffTool 字段', async ({ page }) => {
      await page.setViewportSize({ width: 1920, height: 1080 })
      await page.goto('/')
      await page.locator('.activity-bar-item').nth(-2).click()

      const pathInput = visibleDialog(page, '设置').locator('input[placeholder*="WinMergeU.exe"]')
      await pathInput.fill('C:\\tools\\WinMergeU.exe')
      await pathInput.blur()

      const calls = await getWailsCalls(page, 'SaveSettings')
      expect(calls.length).toBeGreaterThan(0)
      const saved = calls.at(-1).args[0]
      expect(saved.diffToolPath).toBe('C:\\tools\\WinMergeU.exe')
      // 全量覆盖写须携带其余两个字段，避免清空既有配置
      expect(typeof saved.diffToolName).toBe('string')
      expect(typeof saved.diffToolArgs).toBe('string')
    })
  })

  test.describe('diff 弹窗按钮态', () => {
    test.describe('未配置', () => {
      test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

      test('未配置时按钮置灰', async ({ page }) => {
        await openGitRepoPage(page)
        await page.getByRole('tab', { name: '本地变动' }).click()

        const row = page.locator('.el-table__row', { hasText: 'src/app.js' }).first()
        await row.dblclick()
        const dialog = visibleDialog(page, '文件差异')
        await expect(dialog).toBeVisible()

        const btn = dialog.getByRole('button', { name: '用外部工具打开' })
        await expect(btn).toBeDisabled()
        // 悬停展示引导提示（tooltip 挂 body）
        await btn.hover()
        await expect(page.getByText('未配置外部 diff 工具，请在设置中配置')).toBeVisible()
      })
    })

    test.describe('已配置', () => {
      test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES, GetSettings: DIFF_TOOL_CONFIGURED } })

      test('已配置时按钮可用（仅验证态，不点击执行）', async ({ page }) => {
        await openGitRepoPage(page)
        await page.getByRole('tab', { name: '本地变动' }).click()

        const row = page.locator('.el-table__row', { hasText: 'src/app.js' }).first()
        await row.dblclick()
        const dialog = visibleDialog(page, '文件差异')
        await expect(dialog).toBeVisible()

        const btn = dialog.getByRole('button', { name: '用外部工具打开' })
        await expect(btn).toBeEnabled()
      })
    })
  })
})
