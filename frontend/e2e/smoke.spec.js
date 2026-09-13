/**
 * 冒烟用例：验证「vite preview web 版 + mock Wails 后端」E2E 链路贯通。
 *
 * 断言首页核心布局渲染：侧边栏活动栏 + 三栏主布局（工作目录 / 文件树 / 内容面板）。
 * 首页挂载路径会真实调用 mock 后端（GetDirectories / GetSettings / GetAppVersion 等），
 * 任一环节断裂（构建失败、mock 注入失败、组件渲染中断）都会导致用例失败。
 */
import { test, expect } from './fixtures'

test.describe('冒烟：vite preview + mock Wails 后端链路', () => {
  test('应用首页核心布局渲染', async ({ page }) => {
    await page.goto('/')

    // hash 路由就位（router/index.js 使用 createWebHashHistory，首页为 #/）
    await expect(page).toHaveURL(/#\/$/)

    // 侧边栏活动栏渲染
    const activityBar = page.locator('.activity-bar')
    await expect(activityBar).toBeVisible()

    // 三栏主布局容器渲染
    const mainPanes = page.locator('.splitpanes-container')
    await expect(mainPanes).toBeVisible()

    // 左栏：工作目录面板（挂载时经 directoryStore 调 GetDirectories，mock 返回 []）
    await expect(page.locator('.directory-tree-panel')).toBeVisible()
    await expect(page.locator('.directory-tree-panel .dir-toolbar-title')).toHaveText('工作目录')

    // 中栏：文件树面板
    await expect(page.locator('.file-tree-aside')).toBeVisible()

    // 右栏：内容面板
    await expect(page.locator('.content-panel')).toBeVisible()
  })
})
