/**
 * 会话快照恢复 E2E：验证「mock 后端返回快照 → 前端启动恢复 UI 状态」链路。
 *
 * 覆盖 PR2 崩溃恢复 MVP：GetSessionState 注入 activePanel='ai' 快照，
 * 首页启动后经 restoreSession → applySessionState 写回 ui store，
 * AiFunctionPanel（v-show=activePanel==='ai'）由隐藏变可见。
 *
 * 不验证 selectedDirectoryId 恢复：该路径依赖 GetDirectories 返回含目标 id 的目录列表，
 * 且 onDirectorySelect 触发文件树重载，复杂度高；单元侧 useSessionState.spec.js 已覆盖
 * applySessionState 与 restoreSession 逻辑，本用例聚焦跨层绑定 + 真实浏览器渲染。
 */
import { test, expect } from './fixtures'

test.describe('会话快照恢复：注入 activePanel=ai 快照', () => {
  // 注入快照：activePanel=ai（覆盖 wails-mock-defaults.js 默认 null 冷启动）
  test.use({
    wailsOverrides: {
      GetSessionState: {
        selectedDirectoryId: '',
        activePanel: 'ai',
        terminal: { visible: false, height: 0, workDir: '' }
      }
    }
  })

  test('启动时按快照恢复活动面板（AI 功能页可见，三栏隐藏）', async ({ page }) => {
    await page.goto('/')

    // AI 功能面板经 v-show=activePanel==='ai' 变可见（恢复写回 ui store 后）
    await expect(page.locator('.ai-function-panel')).toBeVisible()

    // 三栏主布局（directory/toolbox 激活时才显示）应隐藏
    await expect(page.locator('.splitpanes-container')).toBeHidden()
  })
})

test.describe('会话快照恢复：无快照走默认冷启动', () => {
  // 默认 GetSessionState 返回 null（wails-mock-defaults.js 基线），无覆盖

  test('默认 activePanel=directory，三栏可见、AI 面板隐藏', async ({ page }) => {
    await page.goto('/')

    await expect(page.locator('.splitpanes-container')).toBeVisible()
    await expect(page.locator('.ai-function-panel')).toBeHidden()
  })
})
