/**
 * 分支对比面板 E2E（mock 后端，只读 batch 1 冒烟）。
 *
 * 覆盖点：
 * 1. 活动栏切换到「分支对比」面板，挂载后种子当前工作目录 → 面板可见
 * 2. 面板可见且 A/B 可解析时自动对比：统计条 + 对齐提交表渲染（含疑似拣选徽标）
 * 3. 点击文件树文件：diff 查看器渲染双栏内容
 *
 * 数据源：wails-mock-defaults.js 基础表的 GetBranchCompareAlignment /
 * GetBranchCompareFileTree / GetBranchCompareFileDiff 默认值（覆盖 matched-hash /
 * matched-patch-id / a-only / b-only 四种行形态 + 六种文件状态 + 一段最小 diff），
 * 仓库上下文经 git-fixtures.js 的 GIT_FLOW_OVERRIDES 注入。
 */
import { test, expect, getWailsCalls } from './fixtures'
import { E2E_REPO_PATH, GIT_FLOW_OVERRIDES } from './git-fixtures'

// GetGitInfo 基础表默认 {}（isRepo 缺省 = 非 git 仓库空态），面板用例覆盖为 git 仓库
const BRANCH_COMPARE_OVERRIDES = {
  ...GIT_FLOW_OVERRIDES,
  GetGitInfo: { isRepo: true, branch: 'main', path: E2E_REPO_PATH }
}

// 打开首页并切到分支对比面板（分支 A=main（当前）/ B=origin/main 由 mock 分支表推导，
// 面板可见即触发自动对比）
async function openBranchComparePage(page) {
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.goto('/')
  await expect(page).toHaveURL(/#\/$/)
  // 活动栏分支对比图标（data-activity=branchcompare 稳定定位）
  await page.locator('[data-activity="branchcompare"]').click()
  await expect(page.locator('.bc-view')).toBeVisible()
}

test.describe('分支对比面板', () => {
  test.use({ wailsOverrides: { ...BRANCH_COMPARE_OVERRIDES } })

  test('切换面板自动对比：统计条与对齐提交表渲染，参数正确', async ({ page }) => {
    await openBranchComparePage(page)

    // 统计条（R4）：A 独有 / 匹配（含疑似拣选）/ B 独有 / 共同历史
    await expect(page.locator('.stats-line')).toContainText('A 独有 1')
    await expect(page.locator('.stats-line')).toContainText('匹配 2')
    await expect(page.locator('.stats-line')).toContainText('疑似拣选 1')
    await expect(page.locator('.stats-line')).toContainText('B 独有 1')
    await expect(page.locator('.stats-line')).toContainText('merge-base 之下共同历史 12')

    // 对齐提交表（R3）：4 行对齐结果渲染，匹配徽标区分匹配方式
    const rows = page.locator('.bc-left .el-table__row')
    await expect(rows).toHaveCount(4)
    await expect(rows.first()).toContainText('aaa1111')
    await expect(page.locator('.badge-patch-id', { hasText: '疑似拣选' })).toBeVisible()

    // 对比调用参数：种子仓库路径 + 默认分支（main 当前 / origin/main 远程默认启发式）
    const alignCalls = await getWailsCalls(page, 'GetBranchCompareAlignment')
    expect(alignCalls).toEqual([
      { method: 'GetBranchCompareAlignment', args: [E2E_REPO_PATH, 'main', 'origin/main'] }
    ])
  })

  test('点击文件树文件：diff 查看器渲染双栏内容', async ({ page }) => {
    await openBranchComparePage(page)

    // 文件树渲染分支差异文件（默认 6 个文件），点击新增的 login.js（节点展示名取路径末段）
    const fileNode = page.locator('.tree-node-file', { hasText: 'login.js' })
    await expect(fileNode).toBeVisible()
    await fileNode.click()

    // diff 查看器（R6）渲染默认 unified diff 的双栏内容
    await expect(page.locator('.bc-diff-area .diff-col-left')).toContainText('context line')
    await expect(page.locator('.bc-diff-area .diff-col-right')).toContainText('new line')

    const diffCalls = await getWailsCalls(page, 'GetBranchCompareFileDiff')
    expect(diffCalls).toEqual([
      { method: 'GetBranchCompareFileDiff', args: [E2E_REPO_PATH, 'main', 'origin/main', 'src/auth/login.js'] }
    ])
  })
})
