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

const todayCost = {
  costUsd: 0.1234,
  taskCount: 2,
  inputTokens: 1200,
  outputTokens: 600,
  cacheReadTokens: 300,
  cacheCreationTokens: 150
}

const weekCost = {
  costUsd: 1.5,
  taskCount: 6,
  inputTokens: 4000,
  outputTokens: 1800,
  cacheReadTokens: 900,
  cacheCreationTokens: 400
}

// 形状对齐 model.OverviewSnapshot（含 T3 的 rangeKey / selected）
const overviewSnapshot = {
  generatedAt: Date.now(),
  range: 'week',
  repos: {
    pinnedTotal: 3,
    pendingCount: 2,
    staleCount: 1,
    staleDays: 7,
    missingCount: 1,
    noCommitCount: 1
  },
  ai: {
    today: todayCost,
    week: weekCost,
    rangeKey: 'week',
    selected: {
      cost: weekCost,
      successRate: 0.75,
      hasData: true,
      // 3 成功 / 1 失败 / 0 超时 / 0 取消，分母 4 = 75%
      successCount: 3,
      failedCount: 1,
      timeoutCount: 0,
      canceledCount: 0
    },
    successRate: 0.75,
    hasData: true,
    successCount: 3,
    failedCount: 1,
    timeoutCount: 0,
    canceledCount: 0
  },
  todos: {
    pendingCount: 3,
    oldestDays: 10,
    oldestId: 'todo-oldest',
    oldestText: '十天前的待办'
  }
}

// 切到「近 7 天」档的响应：仅 AI 域变化（成本 3.00 / 成功率 80%），仓库与待办不变。
const sevenDayCost = {
  costUsd: 3,
  taskCount: 8,
  inputTokens: 5000,
  outputTokens: 2200,
  cacheReadTokens: 1200,
  cacheCreationTokens: 500
}
const sevenDaySnapshot = {
  ...overviewSnapshot,
  range: '7d',
  ai: {
    ...overviewSnapshot.ai,
    rangeKey: '7d',
    selected: {
      cost: sevenDayCost,
      successRate: 0.8,
      hasData: true,
      successCount: 4,
      failedCount: 1,
      timeoutCount: 0,
      canceledCount: 1
    }
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
    // 默认档 week：成本主数值取 selected（本周），成功率为本周口径
    await expect(page.locator('.overview-card', { hasText: 'AI 成本' }).locator('.value-num')).toHaveText('$1.50')
    await expect(page.locator('.overview-card', { hasText: 'AI 成功率' }).locator('.value-num')).toHaveText('75.0%')

    // 口径断层标注
    await expect(page.locator('.overview-card', { hasText: '待收尾仓库' })).toContainText('失效仓库 1 个')
    await expect(page.locator('.overview-card', { hasText: '久未推进仓库' })).toContainText('无提交 1 个')
    await expect(page.locator('.overview-card', { hasText: 'AI 成本' })).toContainText('范围：本周')
    await expect(page.locator('.overview-card', { hasText: 'AI 成本' })).toContainText('历史保留上限：近 90 天 / 最多 2000 条')
  })

  test('进入面板拉取一次，点击刷新以 force=true 重拉', async ({ page }) => {
    await openPanel(page, 'overview')
    await expect(page.locator('.overview-card')).toHaveCount(5)

    const before = await getWailsCalls(page, 'GetOverviewSnapshot')
    expect(before).toHaveLength(1)
    // 默认档 week 作为初始入参
    expect(before[0].args).toEqual(['week', false])

    await page.getByRole('button', { name: '刷新' }).click()
    await expect
      .poll(async () => (await getWailsCalls(page, 'GetOverviewSnapshot')).length)
      .toBe(2)

    const calls = await getWailsCalls(page, 'GetOverviewSnapshot')
    expect(calls[1].args).toEqual(['week', true])
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

// 时间范围切档：只改 AI 域（M4/M5）数值与口径句，仓库/待办卡片不变。
test.describe('个人看板概览面板 · 时间范围切档', () => {
  test.use({
    wailsOverrides: {
      // 首次进入（week）与切档后（7d）各返回一份快照
      GetOverviewSnapshot: { __sequence__: [overviewSnapshot, sevenDaySnapshot] }
    }
  })

  test.beforeEach(async ({ page }) => {
    await page.goto('/')
  })

  test('切到近 7 天后 AI 卡片数值与口径句变化，仓库与待办不变', async ({ page }) => {
    await openPanel(page, 'overview')

    const costCard = page.locator('.overview-card', { hasText: 'AI 成本' })
    const successCard = page.locator('.overview-card', { hasText: 'AI 成功率' })
    const repoValue = page.locator('.overview-card', { hasText: '待收尾仓库' }).locator('.value-num')
    const todoValue = page.locator('.overview-card', { hasText: '待办积压' }).locator('.value-num')

    await expect(costCard.locator('.value-num')).toHaveText('$1.50')
    await expect(costCard).toContainText('范围：本周')
    await expect(successCard.locator('.value-num')).toHaveText('75.0%')
    await expect(repoValue).toHaveText('2')
    await expect(todoValue).toHaveText('3')

    await page.locator('.el-radio-button', { hasText: '近 7 天' }).click()

    await expect(costCard.locator('.value-num')).toHaveText('$3.00')
    await expect(costCard).toContainText('范围：近 7 天')
    await expect(successCard.locator('.value-num')).toHaveText('80.0%')

    // 仓库/待办为快照/全量语义，不随 AI 档位变化
    await expect(repoValue).toHaveText('2')
    await expect(todoValue).toHaveText('3')

    const calls = await getWailsCalls(page, 'GetOverviewSnapshot')
    expect(calls[calls.length - 1].args).toEqual(['7d', false])
  })
})
