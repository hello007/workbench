/**
 * Git 合并 / 变基流程 E2E（mock 后端，测「UI 链路正确性」）。
 *
 * 覆盖点：
 * 1. 快进合并：Merge(path, target, 'ff') 参数正确、输出展示、成功提示、目标输入清空
 * 2. 变基：切操作类型后 Rebase(path, target) 参数正确
 * 3. 合并冲突：Merge 正常返回但 GetConflictState 报冲突 -> 冲突面板出现；
 *    中止 -> AbortMerge(path) 后面板收起（经 __sequence__ 驱动冲突态三段变化）
 * 4. 异常路径：Merge 失败 -> 错误提示出现且无输出展示
 *
 * 冲突态语义对齐真实后端：合并冲突时 git 以 exit 1 结束但后端已吸收为正常返回，
 * 冲突与否由 GetConflictState 查询决定（见 GitMerge.execute 注释）。
 */
import { test, expect, getWailsCalls } from './fixtures'
import { openGitRepoPage, E2E_REPO_PATH, GIT_FLOW_OVERRIDES } from './git-fixtures'

// 冲突态三段序列：面板挂载(1) 无冲突 -> 执行后查询(2) 合并冲突 -> 中止后查询(3) 无冲突
const CONFLICT_THEN_CLEAN = {
  __sequence__: [
    { type: 'none', files: [] },
    { type: 'merge', files: ['src/app.js'] },
    { type: 'none', files: [] }
  ]
}

test.describe('Git 合并/变基流程', () => {
  test.describe('合并主链路', () => {
    test.use({
      wailsOverrides: { ...GIT_FLOW_OVERRIDES, Merge: 'Fast-forward merged feature/login' }
    })

    test('快进合并：Merge 参数正确，输出展示后目标输入清空', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '合并/变基' }).click()

      // 默认操作类型为「合并」
      await expect(page.getByRole('radio', { name: '合并' })).toBeChecked()

      const targetInput = page.getByPlaceholder('输入要合并的目标分支，如 feature/x')
      await targetInput.fill('feature/login')
      await page.getByRole('button', { name: '执行' }).click()

      // 输出展示 + 成功提示 + 输入清空
      await expect(page.locator('.last-output')).toHaveText('Fast-forward merged feature/login')
      await expect(page.locator('.el-message', { hasText: '操作完成' })).toBeVisible()
      await expect(targetInput).toHaveValue('')

      const mergeCalls = await getWailsCalls(page, 'Merge')
      expect(mergeCalls).toEqual([
        { method: 'Merge', args: [E2E_REPO_PATH, 'feature/login', 'ff'] }
      ])
      // 冲突态查询：面板挂载 + 执行后各一次，均为无冲突
      expect(await getWailsCalls(page, 'GetConflictState')).toHaveLength(2)
    })
  })

  test.describe('变基', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('变基到目标分支：Rebase 参数正确', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '合并/变基' }).click()

      // 切操作类型为「变基」，目标输入占位符随类型切换
      // （el-radio 原生 input 被样式隐藏，点击样式化 label 触发选中）
      await page.locator('.el-radio', { hasText: '变基' }).click()
      const targetInput = page.getByPlaceholder('输入变基目标分支，如 main')
      await targetInput.fill('main')
      await page.getByRole('button', { name: '执行' }).click()

      await expect(page.locator('.last-output')).toHaveText('Rebase completed')
      await expect(page.locator('.el-message', { hasText: '操作完成' })).toBeVisible()

      const rebaseCalls = await getWailsCalls(page, 'Rebase')
      expect(rebaseCalls).toEqual([{ method: 'Rebase', args: [E2E_REPO_PATH, 'main'] }])
      expect(await getWailsCalls(page, 'Merge')).toEqual([])
    })
  })

  test.describe('合并冲突处理', () => {
    test.use({
      wailsOverrides: {
        ...GIT_FLOW_OVERRIDES,
        Merge: 'Auto-merging src/app.js\nCONFLICT (content): Merge conflict in src/app.js',
        GetConflictState: CONFLICT_THEN_CLEAN
      }
    })

    test('合并冲突：冲突面板出现，中止后调 AbortMerge 且面板收起', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '合并/变基' }).click()

      await page.getByPlaceholder('输入要合并的目标分支，如 feature/x').fill('feature/login')
      await page.getByRole('button', { name: '执行' }).click()

      // 冲突面板出现：警告标记 + 冲突类型 + 冲突文件 + 操作按钮
      // （冲突文件限定 .conflict-row 定位，避免命中 .last-output 里的 Merge 输出文本）
      await expect(page.getByText('冲突未解决')).toBeVisible()
      await expect(page.getByText('合并冲突')).toBeVisible()
      await expect(page.locator('.conflict-row .conflict-file', { hasText: 'src/app.js' })).toBeVisible()
      await expect(page.getByRole('button', { name: '继续' })).toBeVisible()

      // 中止：AbortMerge 调用 + 面板收起 + 提示
      await page.getByRole('button', { name: '中止' }).click()
      await expect(page.locator('.el-message', { hasText: '已中止操作' })).toBeVisible()
      await expect(page.getByText('冲突未解决')).toBeHidden()

      const abortCalls = await getWailsCalls(page, 'AbortMerge')
      expect(abortCalls).toEqual([{ method: 'AbortMerge', args: [E2E_REPO_PATH] }])
      // 冲突态查询：挂载 + 执行后 + 中止后各一次
      expect(await getWailsCalls(page, 'GetConflictState')).toHaveLength(3)
    })
  })

  test.describe('合并失败链路', () => {
    test.use({
      wailsOverrides: {
        ...GIT_FLOW_OVERRIDES,
        Merge: { __error__: 'refusing to merge unrelated histories' }
      }
    })

    test('Merge 失败：错误提示出现且无输出展示', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '合并/变基' }).click()

      await page.getByPlaceholder('输入要合并的目标分支，如 feature/x').fill('feature/login')
      await page.getByRole('button', { name: '执行' }).click()

      await expect(
        page.locator('.el-message--error', { hasText: '操作失败: refusing to merge unrelated histories' })
      ).toBeVisible()
      // 失败无输出展示，且面板未进入冲突态
      await expect(page.locator('.last-output')).toHaveCount(0)
      await expect(page.getByText('冲突未解决')).toHaveCount(0)
    })
  })
})
