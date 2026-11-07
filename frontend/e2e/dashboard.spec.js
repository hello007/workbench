/**
 * 全局状态看板 E2E（DashboardView 渲染 + 钉选互锁链，mock 驱动）。
 *
 * 覆盖四条链路：
 * 1. 状态列表渲染：GetDashboardStatuses fixture（形状对齐 model.RepoStatus，含
 *    dirtyCount/lastCommitUnix/stashCount 工作树三字段）映射分支 / 工作区（N 处改动）/
 *    最后提交（相对时间）/ Stash（计数）/ 未推送（ahead）/ 未拉取（behind）标签列
 * 2. 手动刷新：圆形刷新按钮触发 GetDashboardStatuses 重拉（RefreshDashboardStatuses
 *    无前端消费方，手动刷新走 loadStatuses → GetDashboardStatuses）
 * 3. 钉选互锁：添加弹窗打开即按选中工作目录扫描（GetRepoFilterList 参数断言），
 *    候选项逐个 IsDashboardPinned 标记；切目录重扫；加入看板 AddDashboardPin 后
 *    再次打开弹窗，IsDashboardPinned 经 __sequence__ 翻转 true → 候选项禁用 +
 *    「已关注」标记（勾选禁用互锁，防重复关注）
 * 4. 取消关注：表格行「取消关注」→ RemoveDashboardPin 参数断言 + 状态重拉
 *
 * mock 前置：GetRepoFilterList 基础表缺省 []（DashboardView.vue:218 消费链
 * Proxy null 会 TypeError），本 spec override 具体清单。
 */
import { test, expect, getWailsCalls, openPanel } from './fixtures'

// 形状对齐 model.Directory（与 e2e/git-fixtures.js GIT_REPO_DIRECTORY 同构）
const dashboardDirs = [
  { id: 'dir-main', name: '主工作区', path: 'D:/e2e-demo', isDefault: true, isGitRepo: false, hasRemote: false },
  { id: 'dir-alt', name: '备用工作区', path: 'E:/repos', isDefault: false, isGitRepo: false, hasRemote: false }
]

// 形状对齐 model.RepoStatus：两行覆盖 有改动/干净 与 ahead/behind 非零标签面，
// 含工作树三字段（dirtyCount / lastCommitUnix / stashCount）
const dashboardStatuses = [
  {
    path: 'D:/e2e-demo/demo-repo',
    name: 'demo-repo',
    branch: 'main',
    dirty: true,
    dirtyCount: 3,
    ahead: 2,
    behind: 0,
    hasUpstream: true,
    detached: false,
    lastCommitUnix: Math.floor(Date.now() / 1000) - 3000, // 50 分钟前 → 相对时间
    stashCount: 5,
    isRepo: true,
    missing: false,
    error: ''
  },
  {
    path: 'E:/repos/legacy',
    name: 'legacy',
    branch: 'dev',
    dirty: false,
    dirtyCount: 0,
    ahead: 0,
    behind: 3,
    hasUpstream: true,
    detached: false,
    lastCommitUnix: Math.floor(Date.now() / 1000) - 86400 * 40, // 40 天前 → 转 zh-CN 日期
    stashCount: 0,
    isRepo: true,
    missing: false,
    error: ''
  }
]

// 形状对齐 model.RepoFilterItem（添加弹窗候选；hasRemote false 项渲染「无远程」标记）
const repoFilterItems = [
  { name: 'demo-repo', path: 'D:/e2e-demo/demo-repo', hasRemote: true, isGitRepo: true },
  { name: 'tools', path: 'D:/e2e-demo/tools', hasRemote: false, isGitRepo: true }
]

// 按表头文本解析数据列下标（thead th 与 tbody td 同序）：列插入/调序时断言自动跟随表头，
// 避免 td.nth 硬编码下标错位漂移（如错命中「未推送」列的 dim 0 造成假绿）
async function columnIndexOf(table, headerText) {
  const headers = table.locator('thead th')
  const count = await headers.count()
  for (let i = 0; i < count; i++) {
    if ((await headers.nth(i).innerText()).trim() === headerText) {
      return i
    }
  }
  throw new Error(`状态表未找到表头列: ${headerText}`)
}

test.describe('全局状态看板：渲染与钉选互锁', () => {
  // IsDashboardPinned 按扫描序翻转：三次扫描（打开 / 切目录 / 加入后重开）× 2 候选 =
  // 前 4 次未关注 false，加入看板后的重开扫描 2 次均 true（序列耗尽 hold last）
  test.use({
    wailsOverrides: {
      GetDirectories: dashboardDirs,
      GetDashboardStatuses: dashboardStatuses,
      GetRepoFilterList: repoFilterItems,
      IsDashboardPinned: { __sequence__: [false, false, false, false, true, true] }
    }
  })

  test('状态渲染 + 手动刷新 + 钉选 Add/IsPinned 翻转/Remove + 扫描参数断言', async ({ page }) => {
    await page.goto('/')
    // 状态看板（活动栏 data-panel-id="dashboard" 语义定位）
    await openPanel(page, 'dashboard')
    const view = page.locator('.dashboard-view')
    await expect(view).toBeVisible()

    // 1. 状态列表渲染：两行 fixture → 分支 / 工作区 / 最后提交 / Stash / 未推送 / 未拉取
    await expect(view.locator('.status-table .el-table__row')).toHaveCount(2)
    const table = view.locator('.status-table')
    const stashCol = await columnIndexOf(table, 'Stash')
    const commitCol = await columnIndexOf(table, '最后提交')
    const repoRow = view.locator('.status-table .el-table__row').filter({ hasText: 'demo-repo' })
    await expect(repoRow).toContainText('main')
    await expect(repoRow).toContainText('3 处改动') // dirtyCount 3 → warning 工作区
    await expect(repoRow).toContainText('50 分钟前') // lastCommitUnix 50 分钟前 → 相对时间
    await expect(repoRow).toContainText('2') // ahead 2 → danger 未推送
    await expect(repoRow.locator('td').nth(stashCol)).toContainText('5') // Stash 列计数
    const legacyRow = view.locator('.status-table .el-table__row').filter({ hasText: 'legacy' })
    await expect(legacyRow).toContainText('干净')
    await expect(legacyRow).toContainText('3') // behind 3 → warning 未拉取
    // 40 天前 → 转 zh-CN 日期（YYYY/M/D 含 / 分隔；相对时间「N 天前」不含 /，正断言即排除）
    await expect(legacyRow.locator('td').nth(commitCol)).toContainText('/')
    await expect(legacyRow.locator('td').nth(stashCol)).toContainText('0') // Stash 零值 dim 0
    await expect.poll(async () => (await getWailsCalls(page, 'GetDashboardStatuses')).length).toBe(1)

    // 2. 手动刷新（圆形按钮）→ GetDashboardStatuses 重拉
    await view.locator('.dashboard-header .el-button.is-circle').click()
    await expect.poll(async () => (await getWailsCalls(page, 'GetDashboardStatuses')).length).toBe(2)

    // 3. 添加弹窗：打开即按选中工作目录（loadDirectories 自动选 default = dir-main）扫描
    await view.getByRole('button', { name: '添加仓库' }).click()
    const addDialog = page.locator('.el-dialog:visible').filter({ hasText: '添加关注仓库' })
    await expect(addDialog).toBeVisible()
    await expect.poll(async () => (await getWailsCalls(page, 'GetRepoFilterList')).length).toBe(1)
    expect((await getWailsCalls(page, 'GetRepoFilterList'))[0].args).toEqual(['dir-main'])

    // 候选列表渲染：未关注项可勾选，无远程项带「无远程」标记
    const demoCheckbox = addDialog.locator('.el-checkbox').filter({ hasText: 'demo-repo' })
    const toolsCheckbox = addDialog.locator('.el-checkbox').filter({ hasText: 'tools' })
    await expect(demoCheckbox).toBeVisible()
    await expect(demoCheckbox).not.toHaveClass(/is-disabled/)
    await expect(toolsCheckbox).toContainText('无远程')

    // 过滤器切目录 → 重扫 GetRepoFilterList('dir-alt')（IsDashboardPinned 仍未关注）
    await addDialog.locator('.el-select__wrapper').click()
    await page.locator('.el-select-dropdown:visible .el-select-dropdown__item').filter({ hasText: '备用工作区' }).click()
    await expect.poll(async () => (await getWailsCalls(page, 'GetRepoFilterList')).length).toBe(2)
    expect((await getWailsCalls(page, 'GetRepoFilterList'))[1].args).toEqual(['dir-alt'])

    // 勾选候选 → 加入看板：AddDashboardPin 逐路径调用 + toast + 状态重拉
    await demoCheckbox.click()
    await addDialog.getByRole('button', { name: '加入看板' }).click()
    await expect(page.locator('.el-message').filter({ hasText: '已添加 1 个仓库' })).toBeVisible()
    await expect(addDialog).toBeHidden()
    expect((await getWailsCalls(page, 'AddDashboardPin')).map((c) => c.args)).toEqual([['D:/e2e-demo/demo-repo']])
    await expect.poll(async () => (await getWailsCalls(page, 'GetDashboardStatuses')).length).toBe(3)

    // IsDashboardPinned 翻转（加入后重开弹窗扫描 → true）：候选项禁用 + 「已关注」标记
    await view.getByRole('button', { name: '添加仓库' }).click()
    await expect(addDialog).toBeVisible()
    await expect.poll(async () => (await getWailsCalls(page, 'GetRepoFilterList')).length).toBe(3)
    await expect(demoCheckbox).toHaveClass(/is-disabled/)
    await expect(demoCheckbox).toContainText('已关注')
    await expect(demoCheckbox.locator('input')).toBeDisabled()
    await addDialog.getByRole('button', { name: '取消' }).click()
    await expect(addDialog).toBeHidden()

    // 4. 取消关注：表格行操作 → RemoveDashboardPin 参数断言 + 状态重拉 + toast
    await repoRow.getByRole('button', { name: '取消关注' }).click()
    await expect(page.locator('.el-message').filter({ hasText: '已取消关注' })).toBeVisible()
    expect((await getWailsCalls(page, 'RemoveDashboardPin')).map((c) => c.args)).toEqual([['D:/e2e-demo/demo-repo']])
    await expect.poll(async () => (await getWailsCalls(page, 'GetDashboardStatuses')).length).toBe(4)
  })
})
