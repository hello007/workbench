/**
 * Git 分支管理流程 E2E（mock 后端，测「UI 链路正确性」）。
 *
 * 覆盖点：
 * 1. 分支列表渲染：本地分支展示、当前分支标记、当前分支禁删、远程分支不进列表
 * 2. 新建分支 -> CreateBranch 参数正确 + 成功提示 + 列表刷新
 * 3. 重命名分支 -> RenameBranch(old, new) 参数正确
 * 4. 删除非当前分支 -> DeleteBranch 安全删除（force=false）
 * 5. 切换分支（ContentPanel 切换分支弹窗）-> CheckoutBranch(path, branch, isRemote)
 * 6. 异常路径：CreateBranch 失败 -> 错误提示出现且列表不刷新
 *
 * 分支列表来自 wails-mock-defaults.js E2E 合并表的 GetBranches 默认值
 * （main=当前 / feature/login / origin/main=远程）。
 */
import { test, expect, getWailsCalls } from './fixtures'
import { openGitRepoPage, visibleDialog, E2E_REPO_PATH, GIT_FLOW_OVERRIDES } from './git-fixtures'

test.describe('Git 分支管理流程', () => {
  test.describe('分支列表渲染', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('本地分支渲染：当前分支有标记且禁删，远程分支不展示', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '分支' }).click()

      const card = page.locator('.git-branches-card')
      const mainRow = card.locator('.branch-row', { hasText: 'main' })
      const featureRow = card.locator('.branch-row', { hasText: 'feature/login' })

      // 本地分支展示 + 当前分支标记
      await expect(mainRow).toBeVisible()
      await expect(mainRow.getByText('当前')).toBeVisible()
      await expect(featureRow).toBeVisible()

      // 当前分支禁用删除（模板 disabled，不触达 deleteBranch）
      await expect(mainRow.getByRole('button', { name: '删除' })).toBeDisabled()
      await expect(featureRow.getByRole('button', { name: '删除' })).toBeEnabled()

      // 仅展示本地分支：远程分支不渲染
      await expect(card.getByText('origin/main')).toHaveCount(0)

      // 挂载即加载分支列表
      const branchCalls = await getWailsCalls(page, 'GetBranches')
      expect(branchCalls).toEqual([{ method: 'GetBranches', args: [E2E_REPO_PATH] }])
    })
  })

  test.describe('新建分支', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('新建分支：CreateBranch 参数正确，成功后刷新列表', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '分支' }).click()

      await page.getByRole('button', { name: '新建分支' }).click()
      const dialog = visibleDialog(page, '新建分支')
      await expect(dialog).toBeVisible()

      await dialog.getByPlaceholder('从当前 HEAD 创建，如 feature/x').fill('feature/e2e-new')
      await dialog.getByRole('button', { name: '创建' }).click()

      await expect(page.locator('.el-message', { hasText: '分支创建成功' })).toBeVisible()
      // 对话框关闭
      await expect(dialog).toBeHidden()

      const createCalls = await getWailsCalls(page, 'CreateBranch')
      expect(createCalls).toEqual([{ method: 'CreateBranch', args: [E2E_REPO_PATH, 'feature/e2e-new'] }])
      // 列表刷新：挂载 + 创建成功后各一次
      expect(await getWailsCalls(page, 'GetBranches')).toHaveLength(2)
    })
  })

  test.describe('重命名分支', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('重命名分支：RenameBranch 收到 (old, new) 两个分支名', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '分支' }).click()

      await page
        .locator('.branch-row', { hasText: 'feature/login' })
        .getByRole('button', { name: '重命名' })
        .click()
      const dialog = visibleDialog(page, '重命名分支')
      await expect(dialog).toBeVisible()
      // 原分支名只读回显（el-input 渲染为 input value 而非文本节点）
      await expect(dialog.locator('input[disabled]')).toHaveValue('feature/login')

      await dialog.getByPlaceholder('输入新分支名').fill('feature/login2')
      await dialog.getByRole('button', { name: '重命名' }).click()

      await expect(page.locator('.el-message', { hasText: '分支已重命名' })).toBeVisible()

      const renameCalls = await getWailsCalls(page, 'RenameBranch')
      expect(renameCalls).toEqual([
        { method: 'RenameBranch', args: [E2E_REPO_PATH, 'feature/login', 'feature/login2'] }
      ])
    })
  })

  test.describe('删除分支', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('删除非当前分支：默认走安全删除（force=false）', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '分支' }).click()

      // 安全删除成功（未完全合并的二次确认弹窗不出现）
      await page
        .locator('.branch-row', { hasText: 'feature/login' })
        .getByRole('button', { name: '删除' })
        .click()

      await expect(page.locator('.el-message', { hasText: '分支已删除' })).toBeVisible()

      const deleteCalls = await getWailsCalls(page, 'DeleteBranch')
      expect(deleteCalls).toEqual([{ method: 'DeleteBranch', args: [E2E_REPO_PATH, 'feature/login', false] }])
      // 列表刷新：挂载 + 删除成功后各一次
      expect(await getWailsCalls(page, 'GetBranches')).toHaveLength(2)
    })
  })

  test.describe('切换分支（ContentPanel 弹窗）', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('选择分支后切换：CheckoutBranch 收到 (path, branch, isRemote)', async ({ page }) => {
      await openGitRepoPage(page)

      await page.locator('.git-actions').getByRole('button', { name: '切换分支' }).click()
      const dialog = visibleDialog(page, '切换分支')
      await expect(dialog).toBeVisible()
      // 弹窗回显当前分支
      await expect(dialog.locator('.branch-name')).toHaveText('main')

      // 打开分支下拉并选择非当前本地分支
      await dialog.locator('.branch-select').click()
      await page.getByRole('option', { name: 'feature/login' }).click()
      await dialog.getByRole('button', { name: '切换' }).click()

      await expect(page.locator('.el-message', { hasText: '已切换到分支: feature/login' })).toBeVisible()

      const checkoutCalls = await getWailsCalls(page, 'CheckoutBranch')
      expect(checkoutCalls).toEqual([
        { method: 'CheckoutBranch', args: [E2E_REPO_PATH, 'feature/login', false] }
      ])
    })
  })

  test.describe('新建分支失败链路', () => {
    test.use({
      wailsOverrides: {
        ...GIT_FLOW_OVERRIDES,
        CreateBranch: { __error__: 'branch already exists' }
      }
    })

    test('CreateBranch 失败：错误提示出现且列表不刷新', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '分支' }).click()

      await page.getByRole('button', { name: '新建分支' }).click()
      const dialog = visibleDialog(page, '新建分支')
      await dialog.getByPlaceholder('从当前 HEAD 创建，如 feature/x').fill('feature/login')
      await dialog.getByRole('button', { name: '创建' }).click()

      await expect(
        page.locator('.el-message--error', { hasText: '创建分支失败: branch already exists' })
      ).toBeVisible()

      // 失败后列表不刷新（仍只有挂载时一次调用）
      expect(await getWailsCalls(page, 'GetBranches')).toHaveLength(1)
    })
  })
})
