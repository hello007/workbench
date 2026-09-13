/**
 * 文件树操作流程 E2E（mock 后端，测「UI 链路正确性」）。
 *
 * 覆盖点：
 * 1. 文件树渲染：选中工作目录后懒加载根节点（src 目录 + README.md 文件）
 * 2. 空白区右键新建文件 -> CreateFile(parentPath, name, '') 参数正确 + 成功提示 + 树刷新
 * 3. 空白区右键新建文件夹 -> CreateDirectory(parentPath, name) 参数正确
 * 4. 节点右键重命名 -> RenameFile(oldPath, newName) 参数正确 + 刷新父目录
 * 5. 节点右键删除 -> 确认弹窗 -> DeleteFile(path) 参数正确 + 刷新父目录
 * 6. 异常路径：CreateFile 失败 -> 错误提示出现且树不刷新
 *
 * 文件树数据来自 wails-mock-defaults.js E2E 合并表的 GetFileTree 默认值
 * （demo-repo 根下 src 目录 + README.md 文件）。
 */
import { test, expect, getWailsCalls } from './fixtures'
import { E2E_REPO_PATH, GIT_REPO_DIRECTORY, GIT_FLOW_OVERRIDES, visibleDialog } from './git-fixtures'

/** 文件树用例公共前置：打开首页并选中工作目录，等待文件树根节点渲染 */
async function openFileTreePage(page) {
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.goto('/')
  const dirItem = page.locator('.dir-item', { hasText: GIT_REPO_DIRECTORY.name })
  await expect(dirItem).toBeVisible()
  await dirItem.click()
  // 文件树懒加载完成：根级节点可见即代表 GetFileTree(rootPath) 链路贯通
  await expect(page.locator('.el-tree-node', { hasText: 'README.md' })).toBeVisible()
}

test.describe('文件树操作流程', () => {
  test.describe('文件树渲染', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('选中目录后懒加载根节点：目录与文件均渲染', async ({ page }) => {
      await openFileTreePage(page)

      await expect(page.locator('.el-tree-node', { hasText: 'src' })).toBeVisible()
      await expect(page.locator('.el-tree-node', { hasText: 'README.md' })).toBeVisible()

      // 挂载即按选中目录路径加载根节点
      const treeCalls = await getWailsCalls(page, 'GetFileTree')
      expect(treeCalls).toEqual([{ method: 'GetFileTree', args: [E2E_REPO_PATH] }])
    })
  })

  test.describe('新建文件', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('空白区右键新建文件：CreateFile 参数正确，成功后刷新树', async ({ page }) => {
      await openFileTreePage(page)

      // 右键空白区（树容器）唤起空白菜单
      await page.locator('.tree-content').click({ button: 'right' })
      await page
        .locator('.context-menu-item')
        .filter({ hasText: '新建文件', hasNotText: '新建文件夹' })
        .click()

      const dialog = visibleDialog(page, '新建文件')
      await expect(dialog).toBeVisible()
      await dialog.getByPlaceholder('例如: main.go').fill('main.go')
      await dialog.getByRole('button', { name: '确定' }).click()

      await expect(page.locator('.el-message', { hasText: '文件创建成功' })).toBeVisible()
      await expect(dialog).toBeHidden()

      const createCalls = await getWailsCalls(page, 'CreateFile')
      expect(createCalls).toEqual([{ method: 'CreateFile', args: [E2E_REPO_PATH, 'main.go', ''] }])
      // 树刷新：根节点加载 + 创建成功后 refreshNode 各一次
      const treeCalls = await getWailsCalls(page, 'GetFileTree')
      expect(treeCalls).toHaveLength(2)
      expect(treeCalls[1]).toEqual({ method: 'GetFileTree', args: [E2E_REPO_PATH] })
    })
  })

  test.describe('新建文件夹', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('空白区右键新建文件夹：CreateDirectory 参数正确', async ({ page }) => {
      await openFileTreePage(page)

      await page.locator('.tree-content').click({ button: 'right' })
      await page.locator('.context-menu-item', { hasText: '新建文件夹' }).click()

      const dialog = visibleDialog(page, '新建文件夹')
      await expect(dialog).toBeVisible()
      await dialog.getByPlaceholder('例如: src').fill('docs')
      await dialog.getByRole('button', { name: '确定' }).click()

      await expect(page.locator('.el-message', { hasText: '文件夹创建成功' })).toBeVisible()

      const createCalls = await getWailsCalls(page, 'CreateDirectory')
      expect(createCalls).toEqual([{ method: 'CreateDirectory', args: [E2E_REPO_PATH, 'docs'] }])
    })
  })

  test.describe('重命名', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('节点右键重命名：RenameFile 收到 (oldPath, newName)，成功后刷新父目录', async ({ page }) => {
      await openFileTreePage(page)

      await page
        .locator('.el-tree-node', { hasText: 'README.md' })
        .click({ button: 'right' })
      await page.locator('.context-menu-item', { hasText: '重命名' }).click()

      const dialog = visibleDialog(page, '重命名')
      await expect(dialog).toBeVisible()
      // 原名称回显
      await expect(dialog.getByPlaceholder('请输入新名称')).toHaveValue('README.md')

      await dialog.getByPlaceholder('请输入新名称').fill('ABOUT.md')
      await dialog.getByRole('button', { name: '确定' }).click()

      await expect(page.locator('.el-message', { hasText: '重命名成功' })).toBeVisible()
      await expect(dialog).toBeHidden()

      const renameCalls = await getWailsCalls(page, 'RenameFile')
      expect(renameCalls).toEqual([
        { method: 'RenameFile', args: [`${E2E_REPO_PATH}/README.md`, 'ABOUT.md'] }
      ])
      // 刷新父目录：根节点加载 + 重命名成功后各一次
      const treeCalls = await getWailsCalls(page, 'GetFileTree')
      expect(treeCalls).toHaveLength(2)
    })
  })

  test.describe('删除', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('节点右键删除：确认弹窗后 DeleteFile 参数正确', async ({ page }) => {
      await openFileTreePage(page)

      await page
        .locator('.el-tree-node', { hasText: 'README.md' })
        .click({ button: 'right' })
      await page.locator('.context-menu-item', { hasText: '删除' }).click()

      // 二次确认弹窗（ElMessageBox）
      const messageBox = page.locator('.el-message-box')
      await expect(messageBox).toContainText('确定要删除 "README.md" 吗')
      await messageBox.getByRole('button', { name: '确定' }).click()

      await expect(page.locator('.el-message', { hasText: '删除成功' })).toBeVisible()

      const deleteCalls = await getWailsCalls(page, 'DeleteFile')
      expect(deleteCalls).toEqual([{ method: 'DeleteFile', args: [`${E2E_REPO_PATH}/README.md`] }])
      // 刷新父目录：根节点加载 + 删除成功后各一次
      const treeCalls = await getWailsCalls(page, 'GetFileTree')
      expect(treeCalls).toHaveLength(2)
    })
  })

  test.describe('新建文件失败链路', () => {
    test.use({
      wailsOverrides: {
        ...GIT_FLOW_OVERRIDES,
        CreateFile: { __error__: 'file already exists' }
      }
    })

    test('CreateFile 失败：错误提示出现且树不刷新', async ({ page }) => {
      await openFileTreePage(page)

      await page.locator('.tree-content').click({ button: 'right' })
      await page
        .locator('.context-menu-item')
        .filter({ hasText: '新建文件', hasNotText: '新建文件夹' })
        .click()
      const dialog = visibleDialog(page, '新建文件')
      await dialog.getByPlaceholder('例如: main.go').fill('main.go')
      await dialog.getByRole('button', { name: '确定' }).click()

      await expect(
        page.locator('.el-message--error', { hasText: '创建失败: file already exists' })
      ).toBeVisible()

      // 失败后树不刷新（仍只有根节点加载一次调用）
      expect(await getWailsCalls(page, 'GetFileTree')).toHaveLength(1)
    })
  })
})
