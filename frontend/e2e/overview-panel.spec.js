/**
 * 个人看板「概览」面板 E2E（mock 驱动）。
 *
 * 覆盖四条链路：
 * 1. 入口可达：活动栏 data-panel-id="overview" 可点击，面板渲染 5 张卡片
 *    （M1/M2 仓库状态、M4/M5 AI 任务、M3 待办积压）。
 * 2. 数值渲染：GetOverviewSnapshot fixture（形状对齐 model.OverviewSnapshot）
 *    映射到卡片主数值与口径标注（失效仓库单列、无提交单列、AI 90 天/2000 条）。
 * 3. 刷新：点击「刷新」按钮以 force=true 重拉（进入面板默认 force=false）。
 * 4. 下钻：M1 → 状态看板；M3 → 临时待办；M4 → AI 面板并打开历史抽屉。
 *
 * mock 前置：默认表已含空快照 GetOverviewSnapshot，本 spec override 具体快照。
 */
import { test, expect, getWailsCalls, openPanel } from './fixtures'

// 形状对齐 model.OverviewSnapshot
const overviewSnapshot = {
  generatedAt: Date.now(),
  range: 'today',
  repos: {
    pinnedTotal: 3,
    pendingCount: 2,
    staleCount: 1,
    staleDays: 7,
    missingCount: 1,
    noCommitCount: 1
  },
  ai: {
    today: {
      costUsd: 0.1234,
      taskCount: 2,
      inputTokens: 1200,
      outputTokens: 600,
      cacheReadTokens: 300,
      cacheCreationTokens: 150
    },
    week: {
      costUsd: 1.5,
      taskCount: 6,
      inputTokens: 4000,
      outputTokens: 1800,
      cacheReadTokens: 900,
      cacheCreationTokens: 400
    },
    successRate: 0.75,
    hasData: true
  },
  todos: {
    pendingCount: 3,
    oldestDays: 10,
    oldestId: 'todo-oldest',
    oldestText: '十天前的待办'
  }
}

test.describe('个人看板概览面板', () => {
  test.use({ wailsOverrides: { GetOverviewSnapshot: overviewSnapshot } })

  // 每个用例独享新页面（fixtures 注入 mock 后由用例显式导航）
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
  })

  test('活动栏入口可打开并渲染 5 张卡片与口径标注', async ({ page }) => {
    await openPanel(page, 'overview')

    const cards = page.locator('.overview-card')
    await expect(cards).toHaveCount(5)
    // 主数值断言收敛到 .value-num 精确文本：卡片内还有 pin 总数/失效数/阈值等数字，
    // 整卡 toContainText('1') 会被「无提交 1 个」等旁证文本假绿（见 e2e-testing.md §5.3）。
    await expect(page.locator('.overview-card', { hasText: '待收尾仓库' }).locator('.value-num')).toHaveText('2')
    await expect(page.locator('.overview-card', { hasText: '久未推进仓库' }).locator('.value-num')).toHaveText('1')
    await expect(page.locator('.overview-card', { hasText: '待办积压' }).locator('.value-num')).toHaveText('3')
    await expect(page.locator('.overview-card', { hasText: 'AI 成本' }).locator('.value-num')).toHaveText('$0.12')
    await expect(page.locator('.overview-card', { hasText: 'AI 成功率' }).locator('.value-num')).toHaveText('75.0%')

    // 口径断层标注
    await expect(page.locator('.overview-card', { hasText: '待收尾仓库' })).toContainText('失效仓库 1 个')
    await expect(page.locator('.overview-card', { hasText: '久未推进仓库' })).toContainText('无提交 1 个')
    await expect(page.locator('.overview-card', { hasText: 'AI 成本' })).toContainText('近 90 天')
  })

  test('进入面板拉取一次，点击刷新以 force=true 重拉', async ({ page }) => {
    await openPanel(page, 'overview')
    await expect(page.locator('.overview-card')).toHaveCount(5)

    const before = await getWailsCalls(page, 'GetOverviewSnapshot')
    expect(before).toHaveLength(1)
    expect(before[0].args).toEqual(['', false])

    await page.getByRole('button', { name: '刷新' }).click()
    await expect
      .poll(async () => (await getWailsCalls(page, 'GetOverviewSnapshot')).length)
      .toBe(2)

    const calls = await getWailsCalls(page, 'GetOverviewSnapshot')
    expect(calls[1].args).toEqual(['', true])
  })

  test('M1 下钻切到状态看板', async ({ page }) => {
    await openPanel(page, 'overview')
    await page.locator('.overview-card', { hasText: '待收尾仓库' }).click()
    await expect(page.locator('.dashboard-view')).toBeVisible()
  })

  test('M3 下钻切到临时待办', async ({ page }) => {
    await openPanel(page, 'overview')
    await page.locator('.overview-card', { hasText: '待办积压' }).click()
    await expect(page.locator('.temp-todo-panel')).toBeVisible()
  })

  test('M4 下钻切到 AI 面板并打开历史抽屉', async ({ page }) => {
    await openPanel(page, 'overview')
    await page.locator('.overview-card', { hasText: 'AI 成本' }).click()
    await expect(page.locator('.ai-function-panel')).toBeVisible()
    await expect(page.locator('.ai-history-dialog')).toBeVisible()
  })
})
