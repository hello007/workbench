/**
 * Git 子模块管理流程 E2E（mock 后端，测「UI 链路正确性」）。
 *
 * 覆盖点：
 * 1. 子模块列表渲染：路径/短 SHA/状态标签四色映射（干净/已修改/未初始化）+ 未初始化行无「切换跟踪分支」按钮（detached 才有）
 * 2. 添加子模块 -> AddSubmodule(repoPath, url, path, branch) 参数正确 + 成功提示 + 列表刷新
 * 3. 行内更新（目标子模块回显）-> UpdateSubmodules(repoPath, mode, recursive, init, subPath) 参数正确
 * 4. 工具栏全量初始化 -> UpdateSubmodules(repoPath, 'checkout', true, true, '')
 * 5. 删除子模块 -> 确认弹窗 -> RemoveSubmodule(repoPath, path) 参数正确
 * 6. 异常路径：UpdateSubmodules 失败 -> 错误提示出现
 *
 * 子模块列表来自 wails-mock-defaults.js E2E 合并表的 GetSubmodules 默认值
 * （libs/logger=干净 / libs/sdk=已修改 / libs/vendored=未初始化）。
 */
import { test, expect, getWailsCalls } from './fixtures'
import { openGitRepoPage, visibleDialog, E2E_REPO_PATH, GIT_FLOW_OVERRIDES } from './git-fixtures'

test.describe('Git 子模块管理流程', () => {
  test.describe('子模块列表渲染', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('列表渲染：路径、短 SHA 与状态标签按四色映射展示', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '子模块' }).click()

      const card = page.locator('.submodules-container')
      const loggerRow = card.locator('.submodule-row', { hasText: 'libs/logger' })
      const sdkRow = card.locator('.submodule-row', { hasText: 'libs/sdk' })
      const vendoredRow = card.locator('.submodule-row', { hasText: 'libs/vendored' })

      // 三行均渲染，短 SHA 展示
      await expect(loggerRow).toBeVisible()
      await expect(loggerRow.getByText('a1b2c3d4')).toBeVisible()
      await expect(sdkRow).toBeVisible()
      await expect(vendoredRow).toBeVisible()

      // 状态标签：干净 / 已修改 / 未初始化
      await expect(loggerRow.getByText('干净')).toBeVisible()
      await expect(sdkRow.getByText('已修改')).toBeVisible()
      await expect(vendoredRow.getByText('未初始化')).toBeVisible()

      // url 展示
      await expect(card.getByText('https://github.com/demo/logger.git')).toBeVisible()

      // 挂载即加载子模块列表
      const calls = await getWailsCalls(page, 'GetSubmodules')
      expect(calls).toEqual([{ method: 'GetSubmodules', args: [E2E_REPO_PATH] }])
    })
  })

  test.describe('添加子模块', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('填写 url/path/branch 后添加：AddSubmodule 参数正确，成功后刷新列表', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '子模块' }).click()

      await page.getByRole('button', { name: '添加子模块' }).click()
      const dialog = visibleDialog(page, '添加子模块')
      await expect(dialog).toBeVisible()

      await dialog.getByPlaceholder('https://example.com/repo.git').fill('https://github.com/demo/utils.git')
      await dialog.getByPlaceholder('如 libs/sub-repo').fill('libs/utils')
      await dialog.getByPlaceholder('如 main').fill('main')
      await dialog.getByRole('button', { name: '添加' }).click()

      await expect(page.locator('.el-message', { hasText: '子模块已添加' })).toBeVisible()
      await expect(dialog).toBeHidden()

      const addCalls = await getWailsCalls(page, 'AddSubmodule')
      expect(addCalls).toEqual([
        {
          method: 'AddSubmodule',
          args: [E2E_REPO_PATH, 'https://github.com/demo/utils.git', 'libs/utils', 'main']
        }
      ])
      // 列表刷新：挂载 + 添加成功后各一次
      expect(await getWailsCalls(page, 'GetSubmodules')).toHaveLength(2)
    })
  })

  test.describe('更新子模块', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('行内更新：目标子模块回显，UpdateSubmodules 收到 (repoPath, mode, recursive, init, subPath)', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '子模块' }).click()

      await page
        .locator('.submodule-row', { hasText: 'libs/logger' })
        .getByRole('button', { name: '更新' })
        .click()
      const dialog = visibleDialog(page, '更新子模块')
      await expect(dialog).toBeVisible()
      // 目标子模块回显（只读）
      await expect(dialog.locator('input[disabled]')).toHaveValue('libs/logger')

      // 切换更新模式为 merge
      await dialog.locator('.mode-select').click()
      await page.getByRole('option', { name: '合并（merge）' }).click()
      await dialog.getByRole('button', { name: '更新' }).click()

      await expect(page.locator('.el-message', { hasText: '子模块更新成功' })).toBeVisible()
      await expect(dialog).toBeHidden()

      const updateCalls = await getWailsCalls(page, 'UpdateSubmodules')
      expect(updateCalls).toEqual([
        { method: 'UpdateSubmodules', args: [E2E_REPO_PATH, 'merge', true, true, 'libs/logger'] }
      ])
      // 列表刷新：挂载 + 更新成功后各一次
      expect(await getWailsCalls(page, 'GetSubmodules')).toHaveLength(2)
    })

    test('工具栏全量初始化：走 update --init --recursive（checkout 模式）', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '子模块' }).click()

      await page.getByRole('button', { name: '初始化' }).click()

      await expect(page.locator('.el-message', { hasText: '子模块初始化成功' })).toBeVisible()

      const initCalls = await getWailsCalls(page, 'UpdateSubmodules')
      expect(initCalls).toEqual([
        { method: 'UpdateSubmodules', args: [E2E_REPO_PATH, 'checkout', true, true, ''] }
      ])
    })
  })

  test.describe('删除子模块', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('行内删除：确认弹窗后 RemoveSubmodule 参数正确', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '子模块' }).click()

      await page
        .locator('.submodule-row', { hasText: 'libs/logger' })
        .getByRole('button', { name: '删除' })
        .click()

      // 二次确认弹窗（标题「删除子模块」，确认按钮「删除」）
      const messageBox = page.locator('.el-message-box')
      await expect(messageBox).toContainText('确定删除子模块「libs/logger」')
      await messageBox.getByRole('button', { name: '删除' }).click()

      await expect(page.locator('.el-message', { hasText: '子模块已删除' })).toBeVisible()

      const removeCalls = await getWailsCalls(page, 'RemoveSubmodule')
      expect(removeCalls).toEqual([{ method: 'RemoveSubmodule', args: [E2E_REPO_PATH, 'libs/logger'] }])
      // 列表刷新：挂载 + 删除成功后各一次
      expect(await getWailsCalls(page, 'GetSubmodules')).toHaveLength(2)
    })
  })

  test.describe('更新失败链路', () => {
    test.use({
      wailsOverrides: {
        ...GIT_FLOW_OVERRIDES,
        UpdateSubmodules: { __error__: 'submodule not initialized' }
      }
    })

    test('UpdateSubmodules 失败：错误提示出现且列表不刷新', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '子模块' }).click()

      await page
        .locator('.submodule-row', { hasText: 'libs/logger' })
        .getByRole('button', { name: '更新' })
        .click()
      const dialog = visibleDialog(page, '更新子模块')
      await dialog.getByRole('button', { name: '更新' }).click()

      await expect(
        page.locator('.el-message--error', { hasText: '更新子模块失败: submodule not initialized' })
      ).toBeVisible()

      // 失败后列表不刷新（仍只有挂载时一次调用）
      expect(await getWailsCalls(page, 'GetSubmodules')).toHaveLength(1)
    })
  })
})
