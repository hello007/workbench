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
import { test, expect, getWailsCalls, activityPanel } from './fixtures'
import { E2E_REPO_PATH, GIT_REPO_DIRECTORY, GIT_FLOW_OVERRIDES, visibleDialog } from './git-fixtures'
import { makeLargeDirNodes } from '../src/test/wails-mock-defaults'

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

/**
 * 大目录筛选用例前置：GetFileTree 被 makeLargeDirNodes(2050) 覆盖（默认值无
 * README.md，不能复用 openFileTreePage 的 README 等待），改等首个树节点渲染。
 */
async function openLargeDirTreePage(page) {
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.goto('/')
  const dirItem = page.locator('.dir-item', { hasText: GIT_REPO_DIRECTORY.name })
  await expect(dirItem).toBeVisible()
  await dirItem.click()
  await expect(page.locator('.el-tree-node', { hasText: 'chunk-0.dat' })).toBeVisible()
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

// ---- 目录内按名筛选（截断优化配套，perf-baseline §15.6 盲区补齐）----
// 大目录 override：根层 2050 项文件（触发 2000 截断 + 哨兵），chunk-2049.dat
// 位于树中不可见的 2000 名外，仅筛选可定位。
test.describe('目录内按名筛选', () => {
  const largeDirOverrides = {
    ...GIT_FLOW_OVERRIDES,
    GetFileTree: makeLargeDirNodes(2050)
  }
  test.use({ wailsOverrides: largeDirOverrides })

  test('筛选命中 2000 名外文件：覆盖层渲染，点击文件保持筛选态', async ({ page }) => {
    await openLargeDirTreePage(page)

    // 工具栏筛选框输入（无目录点击，作用域默认工作目录根层）
    const filterInput = page.locator('.tree-filter-input input')
    await filterInput.fill('chunk-2049')

    // 覆盖层激活（树视图被替换），防抖 + GetFileTree 后命中渲染
    const overlay = page.locator('.tree-filter-overlay')
    await expect(overlay).toBeVisible()
    await expect(overlay.locator('.tree-filter-summary')).toContainText('命中 1 项')
    await expect(overlay.locator('.filter-result-name', { hasText: 'chunk-2049.dat' })).toBeVisible()

    // 筛选触发了对作用域路径的全量拉取（根层加载 + 筛选各一次）
    const treeCalls = await getWailsCalls(page, 'GetFileTree')
    expect(treeCalls[1]).toEqual({ method: 'GetFileTree', args: [E2E_REPO_PATH] })

    // 点击命中文件：emit select 预览（ContentPanel 链路），筛选态保持
    await overlay.locator('.filter-result-node', { hasText: 'chunk-2049.dat' }).click()
    await expect(overlay).toBeVisible()
    const previewCalls = await getWailsCalls(page, 'PreviewFile')
    expect(previewCalls[0].args[0]).toBe(`${E2E_REPO_PATH}/chunk-2049.dat`)
  })

  test('清空关键词退出筛选：覆盖层消失树视图回归', async ({ page }) => {
    await openLargeDirTreePage(page)

    const filterInput = page.locator('.tree-filter-input input')
    await filterInput.fill('chunk-2')
    await expect(page.locator('.tree-filter-overlay')).toBeVisible()

    await filterInput.fill('')
    await expect(page.locator('.tree-filter-overlay')).toHaveCount(0)
    await expect(page.locator('.el-tree-node', { hasText: 'chunk-0.dat' })).toBeVisible()
  })

  test('ESC 退出筛选', async ({ page }) => {
    await openLargeDirTreePage(page)

    const filterInput = page.locator('.tree-filter-input input')
    await filterInput.fill('chunk-2')
    await expect(page.locator('.tree-filter-overlay')).toBeVisible()

    await filterInput.press('Escape')
    await expect(page.locator('.tree-filter-overlay')).toHaveCount(0)
  })

  test('截断哨兵点击聚焦筛选框（2000 名外文件定位入口）', async ({ page }) => {
    await openLargeDirTreePage(page)

    // 2050 项截断后哨兵为末节点（第 2001 项），Playwright 自动滚动到可见后点击
    const hint = page.locator('.el-tree-node .truncation-hint-node')
    await expect(hint).toContainText('已显示前 2000 项')
    await hint.click()

    await expect(page.locator('.tree-filter-input input')).toBeFocused()
    // 哨兵点击不改变选中态：未触发文件预览（仅聚焦）
    expect(await getWailsCalls(page, 'GetFileTree')).toHaveLength(1)
  })

  // ---- 覆盖层右键菜单（09-27-filetree-filter-overlay-context-menu）----
  // GetFileTree 用 __sequence__ 三段：树根层加载 -> 筛选拉取 -> 操作后 runFilter
  // 重拉（第三段反映操作后状态：目标项已删/已更名）。mock 后端无真实文件系统，
  // 操作后列表变化全靠序列第三段表达。test.use 仅 describe 级合法，故每用例
  // 独立子 describe 包裹注入各自的序列 override。
  const without2049 = () => makeLargeDirNodes(2050).filter(n => n.name !== 'chunk-2049.dat')
  const seqOverrides = (afterOpNodes) => ({
    ...GIT_FLOW_OVERRIDES,
    GetFileTree: {
      __sequence__: [makeLargeDirNodes(2050), makeLargeDirNodes(2050), afterOpNodes]
    }
  })

  test.describe('覆盖层右键菜单', () => {
    test.describe('命中文件右键弹节点菜单', () => {
      test.use({ wailsOverrides: seqOverrides(makeLargeDirNodes(2050)) })

      test('复用树右键菜单（含重命名/删除项）', async ({ page }) => {
        await openLargeDirTreePage(page)

        const filterInput = page.locator('.tree-filter-input input')
        await filterInput.fill('chunk-2049')
        const overlay = page.locator('.tree-filter-overlay')
        await expect(overlay.locator('.filter-result-name', { hasText: 'chunk-2049.dat' })).toBeVisible()

        // 命中行右键弹现有 context-menu（非空白区语义），含文件操作项
        await overlay.locator('.filter-result-node', { hasText: 'chunk-2049.dat' }).click({ button: 'right' })
        const menu = page.locator('.context-menu')
        await expect(menu).toBeVisible()
        await expect(menu.locator('.context-menu-item', { hasText: '重命名' })).toBeVisible()
        await expect(menu.locator('.context-menu-item', { hasText: '删除' })).toBeVisible()
        // 文件节点菜单不含目录专属项（防错用目录语义）
        await expect(menu.locator('.context-menu-item', { hasText: '新建文件' })).toHaveCount(0)
      })
    })

    test.describe('截断层外文件右键删除全链', () => {
      test.use({ wailsOverrides: seqOverrides(without2049()) })

      test('确认后 DeleteFile 参数正确，覆盖层保语境刷新', async ({ page }) => {
        await openLargeDirTreePage(page)

        const filterInput = page.locator('.tree-filter-input input')
        await filterInput.fill('chunk-2049')
        const overlay = page.locator('.tree-filter-overlay')
        const hitRow = overlay.locator('.filter-result-node', { hasText: 'chunk-2049.dat' })
        await expect(hitRow).toBeVisible()

        // 右键 -> 删除 -> 确认弹窗
        await hitRow.click({ button: 'right' })
        await page.locator('.context-menu-item', { hasText: '删除' }).click()
        const messageBox = page.locator('.el-message-box')
        await expect(messageBox).toContainText('确定要删除 "chunk-2049.dat" 吗')
        await messageBox.getByRole('button', { name: '确定' }).click()

        await expect(page.locator('.el-message', { hasText: '删除成功' })).toBeVisible()

        const deleteCalls = await getWailsCalls(page, 'DeleteFile')
        expect(deleteCalls).toEqual([{ method: 'DeleteFile', args: [`${E2E_REPO_PATH}/chunk-2049.dat`] }])

        // 筛选语境保连续：显式失效父目录缓存 + 重跑筛选
        // InvalidateFileTreeCache 双调幂等：refreshAfterFilterOp 显式 1 次 +
        // refreshNode store.root 兜底命中（真实 el-tree 根层已加载）再 1 次；
        // GetFileTree 不锁次数（refreshNode 树重载与 runFilter 并行为内部实现细节），
        // 保语境 + 命中刷新语义由下方覆盖层断言承载
        const invalidateCalls = await getWailsCalls(page, 'InvalidateFileTreeCache')
        expect(invalidateCalls).toEqual([
          { method: 'InvalidateFileTreeCache', args: [E2E_REPO_PATH] },
          { method: 'InvalidateFileTreeCache', args: [E2E_REPO_PATH] }
        ])
        const treeCallsAfterDelete = await getWailsCalls(page, 'GetFileTree')
        expect(treeCallsAfterDelete.length).toBeGreaterThanOrEqual(3)
        treeCallsAfterDelete.forEach(call => expect(call.args).toEqual([E2E_REPO_PATH]))

        // 覆盖层仍激活且命中列表已刷新（目标项消失，非退筛选回树）
        await expect(overlay).toBeVisible()
        await expect(overlay.locator('.filter-result-node', { hasText: 'chunk-2049.dat' })).toHaveCount(0)
        await expect(overlay.locator('.tree-filter-summary')).toContainText('命中 0 项')
      })
    })

    test.describe('命中文件右键重命名全链', () => {
      const renamedNodes = () => makeLargeDirNodes(2050)
        .filter(n => n.name !== 'chunk-2049.dat')
        .concat([{
          ...makeLargeDirNodes(1)[0],
          name: 'renamed-2049.dat',
          path: `${E2E_REPO_PATH}/renamed-2049.dat`,
          id: `${E2E_REPO_PATH}/renamed-2049.dat`
        }])
      test.use({ wailsOverrides: seqOverrides(renamedNodes()) })

      test('RenameFile 参数正确，覆盖层保语境刷新', async ({ page }) => {
        await openLargeDirTreePage(page)

        const filterInput = page.locator('.tree-filter-input input')
        await filterInput.fill('chunk-2049')
        const overlay = page.locator('.tree-filter-overlay')
        const hitRow = overlay.locator('.filter-result-node', { hasText: 'chunk-2049.dat' })
        await expect(hitRow).toBeVisible()

        // 右键 -> 重命名 -> 对话框提交
        await hitRow.click({ button: 'right' })
        await page.locator('.context-menu-item', { hasText: '重命名' }).click()
        const dialog = visibleDialog(page, '重命名')
        const nameInput = dialog.locator('input').nth(1)
        await expect(nameInput).toHaveValue('chunk-2049.dat')
        await nameInput.fill('renamed-2049.dat')
        await dialog.getByRole('button', { name: '确定' }).click()

        await expect(page.locator('.el-message', { hasText: '重命名成功' })).toBeVisible()

        const renameCalls = await getWailsCalls(page, 'RenameFile')
        expect(renameCalls).toEqual([{ method: 'RenameFile', args: [`${E2E_REPO_PATH}/chunk-2049.dat`, 'renamed-2049.dat'] }])

        // 覆盖层仍激活，命中列表反映更名后状态（关键词 'chunk-2049' 不再命中新名 -> 空态）
        await expect(overlay).toBeVisible()
        await expect(overlay.locator('.filter-result-node', { hasText: 'chunk-2049.dat' })).toHaveCount(0)
      })
    })

    test.describe('覆盖层截断哨兵右键', () => {
      test.use({ wailsOverrides: seqOverrides(makeLargeDirNodes(2050)) })

      test('不弹菜单（哨兵过滤面不回归）', async ({ page }) => {
        await openLargeDirTreePage(page)

        // 'chunk' 命中全部 2050 项 -> 覆盖层复用截断（2000 + 哨兵）
        const filterInput = page.locator('.tree-filter-input input')
        await filterInput.fill('chunk')
        const overlay = page.locator('.tree-filter-overlay')
        await expect(overlay.locator('.truncation-hint-node')).toBeVisible()

        await overlay.locator('.truncation-hint-node').click({ button: 'right' })
        await expect(page.locator('.context-menu')).toHaveCount(0)
      })
    })

    test.describe('覆盖层右键粘贴保语境', () => {
      // GetFileTree 序列三段：树根层 -> 筛选拉取 -> 粘贴后 runFilter 重拉（列表不变，
      // mock 后端无真实文件系统）；剪贴板注入一个源路径，CopyItem resolve 成功
      test.use({
        wailsOverrides: {
          ...seqOverrides(makeLargeDirNodes(2050)),
          ReadFromSystemClipboard: JSON.stringify({ paths: [`${E2E_REPO_PATH}/clip-src.dat`], isCut: false }),
          CopyItem: 'ok'
        }
      })

      test('CopyItem 参数正确，覆盖层保语境刷新（命中项仍在）', async ({ page }) => {
        await openLargeDirTreePage(page)

        const filterInput = page.locator('.tree-filter-input input')
        await filterInput.fill('chunk-2049')
        const overlay = page.locator('.tree-filter-overlay')
        const hitRow = overlay.locator('.filter-result-node', { hasText: 'chunk-2049.dat' })
        await expect(hitRow).toBeVisible()

        // 右键命中文件 -> 粘贴（目标目录 = 命中文件所在目录，即工作目录根）
        await hitRow.click({ button: 'right' })
        await page.locator('.context-menu-item', { hasText: '粘贴' }).click()

        await expect(page.locator('.el-message', { hasText: '粘贴成功' })).toBeVisible()

        const copyCalls = await getWailsCalls(page, 'CopyItem')
        expect(copyCalls).toEqual([{ method: 'CopyItem', args: [`${E2E_REPO_PATH}/clip-src.dat`, E2E_REPO_PATH] }])

        // 筛选语境保连续：覆盖层仍激活，命中列表重拉后命中项仍在（非退筛选回树）
        await expect(overlay).toBeVisible()
        await expect(overlay.locator('.filter-result-node', { hasText: 'chunk-2049.dat' })).toBeVisible()
        await expect(overlay.locator('.tree-filter-summary')).toContainText('命中 1 项')
      })
    })

    test.describe('覆盖层右键新建保语境', () => {
      // 命中项为根层目录（chunk-dir，根层含子目录符合 GetFileTree 单层契约）。
      // 新建产物落命中目录内部（createParentData = chunk-dir），与筛选作用域（根层）
      // 不同层，故 runFilter 重拉（序列第三段）不含产物——保语境断言为「覆盖层仍激活
      // + 命中项仍在」，产物可见性由 runFilter 语义决定（同层时出现，见 vitest 用例）
      const dirNode = () => ({
        id: `${E2E_REPO_PATH}/chunk-dir`,
        name: 'chunk-dir',
        path: `${E2E_REPO_PATH}/chunk-dir`,
        type: 'directory',
        isGitRepo: false,
        hasRemote: false,
        hasChildren: true,
        isLeaf: false
      })
      test.use({
        wailsOverrides: {
          ...GIT_FLOW_OVERRIDES,
          GetFileTree: {
            // 三段语义：树根层（2050 文件 + 根层目录）-> 筛选拉取（同层重拉，命中目录）
            // -> 新建后 runFilter 重拉（根层单层，产物在 chunk-dir 内部不出现）
            __sequence__: [
              makeLargeDirNodes(2050).concat([dirNode()]),
              [dirNode()],
              [dirNode()]
            ]
          }
        }
      })

      test('CreateDirectory 参数正确，覆盖层保语境刷新（命中项仍在）', async ({ page }) => {
        await openLargeDirTreePage(page)

        const filterInput = page.locator('.tree-filter-input input')
        await filterInput.fill('chunk-dir')
        const overlay = page.locator('.tree-filter-overlay')
        await expect(overlay.locator('.filter-result-node', { hasText: 'chunk-dir' })).toBeVisible()

        // 右键命中目录 -> 新建文件夹（父目录 = 命中目录自身）
        await overlay.locator('.filter-result-node', { hasText: 'chunk-dir' }).click({ button: 'right' })
        await page.locator('.context-menu-item', { hasText: '新建文件夹' }).click()

        const dialog = visibleDialog(page, '新建文件夹')
        await dialog.getByPlaceholder('例如: src').fill('new-chunk-dir')
        await dialog.getByRole('button', { name: '确定' }).click()

        await expect(page.locator('.el-message', { hasText: '文件夹创建成功' })).toBeVisible()

        const createCalls = await getWailsCalls(page, 'CreateDirectory')
        expect(createCalls).toEqual([{ method: 'CreateDirectory', args: [`${E2E_REPO_PATH}/chunk-dir`, 'new-chunk-dir'] }])

        // 筛选语境保连续：覆盖层仍激活（非退筛选回树），命中列表经 runFilter 重拉后命中项仍在
        await expect(overlay).toBeVisible()
        await expect(overlay.locator('.filter-result-node', { hasText: 'chunk-dir' })).toBeVisible()
      })
    })
  })

})

// 聚合视图（虚拟工作区）：根层来自后端 GetAggregatedRootNodes（收藏目录平铺 +
// 失效占位），后续展开/预览/编辑复用绝对路径链路
test.describe('聚合视图（虚拟工作区）', () => {
  const AGG_DIR_A = 'D:/e2e-demo/项目A'
  const AGG_DIR_GONE = 'D:/e2e-demo/已删除目录'

  // 收藏条目形状对齐 model.Favorite：目录类收藏进聚合树，失效收藏产出 Missing 占位
  const AGG_FAVORITES = [
    { path: AGG_DIR_A, alias: '常用项目A', group: '默认' },
    { path: AGG_DIR_GONE, alias: '', group: '默认' }
  ]

  const AGG_ROOT_NODES = [
    {
      id: AGG_DIR_A,
      name: '项目A',
      path: AGG_DIR_A,
      type: 'directory',
      isGitRepo: false,
      hasRemote: false,
      hasChildren: true,
      isLeaf: false,
      missing: false
    },
    {
      id: AGG_DIR_GONE,
      name: '已删除目录',
      path: AGG_DIR_GONE,
      type: 'directory',
      isGitRepo: false,
      hasRemote: false,
      hasChildren: false,
      isLeaf: false,
      missing: true
    }
  ]

  test.use({
    wailsOverrides: {
      ...GIT_FLOW_OVERRIDES,
      GetFavorites: AGG_FAVORITES,
      GetAggregatedRootNodes: AGG_ROOT_NODES
    }
  })

  /** 打开普通文件树后切换聚合视图，等待收藏根层渲染 */
  async function openAggregatedView(page) {
    await openFileTreePage(page)
    await page.locator('[data-action="toggle-aggregated-view"]').click()
    await expect(page.locator('.el-tree-node', { hasText: '常用项目A' })).toBeVisible()
  }

  test('切换聚合视图：根层走 GetAggregatedRootNodes，别名覆盖显示名且路径顺序保持', async ({ page }) => {
    await openAggregatedView(page)

    // 收藏 alias 覆盖后端路径末段（后端只知路径）
    await expect(page.locator('.el-tree-node', { hasText: '常用项目A' })).toBeVisible()
    // 正常目录不标灰
    await expect(
      page.locator('.el-tree-node', { hasText: '常用项目A' }).locator('.custom-tree-node.missing-node')
    ).toHaveCount(0)

    // 只断言首次加载的入参顺序（渲染顺序由上方节点可见性断言覆盖）：
    // 未来若新增收藏变化触发的重载，恰好一次调用的强断言会假失败
    const aggCalls = await getWailsCalls(page, 'GetAggregatedRootNodes')
    expect(aggCalls).toHaveLength(1)
    expect(aggCalls[0].args[0]).toEqual([AGG_DIR_A, AGG_DIR_GONE])
  })

  test('失效收藏：节点标灰 + tooltip 提示，右键菜单仅保留移除收藏与复制路径', async ({ page }) => {
    await openAggregatedView(page)

    const missingNode = page.locator('.el-tree-node', { hasText: '已删除目录' })
    await expect(missingNode).toBeVisible()
    await expect(missingNode.locator('.custom-tree-node.missing-node')).toBeVisible()

    await missingNode.click({ button: 'right' })
    const menu = page.locator('.context-menu')
    await expect(menu).toBeVisible()
    // 失效节点的文件操作对不存在路径无意义，仅移除收藏/复制路径两项
    await expect(menu.locator('.context-menu-item')).toHaveCount(2)
    await expect(menu.getByText('移除收藏')).toBeVisible()
    await expect(menu.getByText('复制路径')).toBeVisible()
  })

  test('切回目录树：恢复工作目录根节点（两模式树状态互不串扰）', async ({ page }) => {
    await openAggregatedView(page)
    const toggle = page.locator('[data-action="toggle-aggregated-view"]')

    await toggle.click()
    await expect(page.locator('.el-tree-node', { hasText: 'README.md' })).toBeVisible()
    await expect(page.locator('.el-tree-node', { hasText: '常用项目A' })).toHaveCount(0)
  })

  // 增删工作目录后的刷新（emit('change') → loadDirectories）不得隐式改写仍有效的
  // 选中：修复前无条件跳默认/首个，聚合下造成选中态与聚合模式错配（高亮跳变、
  // clipboard 被清、快照污染），普通树则触发 treeKey 变化重建 el-tree。
  // test.use 仅 describe 级合法：注入「选中目录非首位且无默认标记」的列表，
  // 使修复前的隐式跳变（跳到列表首个）能被高亮断言捕获。
  test.describe('增删工作目录后聚合保持', () => {
    // 排在首位、无默认标记：修复前 loadDirectories 刷新后会选中它（隐式跳变）
    const SECOND_DIRECTORY = {
      id: 'dir-e2e-second',
      name: 'second-dir',
      path: 'D:/e2e-demo/second-dir',
      isDefault: false,
      isGitRepo: false,
      hasRemote: false
    }

    test.use({
      wailsOverrides: {
        ...GIT_FLOW_OVERRIDES,
        GetDirectories: [SECOND_DIRECTORY, GIT_REPO_DIRECTORY],
        GetFavorites: AGG_FAVORITES,
        GetAggregatedRootNodes: AGG_ROOT_NODES
      }
    })

    test('删除工作目录刷新列表：聚合不退出、高亮保持原选中', async ({ page }) => {
      await openAggregatedView(page)

      // 左侧 DirectoryTree 右键非选中目录 → 删除 → 单次确认弹窗（confirm-dialog
      // 单次确认链，裸选择器安全）。mock 的 GetDirectories 为静态值：删除后刷新
      // 返回的列表仍含两项目——选中保持判定只依赖「选中 id 是否仍在列表」，与
      // 列表是否收缩正交，静态列表恰好隔离了列表内容因素
      await page.locator('.dir-item', { hasText: 'second-dir' }).click({ button: 'right' })
      const menu = page.locator('.context-menu')
      await expect(menu).toBeVisible()
      await menu.locator('.context-menu-item', { hasText: '删除' }).click()
      const msgbox = page.locator('.el-message-box')
      await expect(msgbox).toBeVisible()
      await msgbox.getByRole('button', { name: '确定' }).click()

      // 刷新确实发生（启动加载之外至少一次 emit('change') → loadDirectories）
      const dirCalls = await getWailsCalls(page, 'GetDirectories')
      expect(dirCalls.length).toBeGreaterThanOrEqual(2)

      // 聚合未退出：收藏根层仍在、toggle 按钮文案未翻回「目录树」
      await expect(page.locator('.el-tree-node', { hasText: '常用项目A' })).toBeVisible()
      await expect(page.locator('[data-action="toggle-aggregated-view"]')).toHaveText('聚合视图')

      // 选中未跳变：高亮仍在 demo-repo，未落到列表首位的 second-dir
      await expect(page.locator('.dir-item--active', { hasText: 'demo-repo' })).toBeVisible()
      await expect(page.locator('.dir-item--active', { hasText: 'second-dir' })).toHaveCount(0)
    })
  })

  test('展开收藏目录：按该目录绝对路径懒加载子节点（复用既有 GetFileTree 链路）', async ({ page }) => {
    await openAggregatedView(page)

    await page.locator('.el-tree-node', { hasText: '常用项目A' }).click()

    const treeCalls = await getWailsCalls(page, 'GetFileTree')
    expect(treeCalls).toContainEqual({ method: 'GetFileTree', args: [AGG_DIR_A] })
  })

  // ---- 收藏外部变更联动（⑦）----
  // 两个用例需要不同的 GetFavorites 动态行为（RemoveFavorite 后收缩 / 打开命令面板后改别名），
  // 且 test.use 仅 describe 级合法，故各自子 describe 注入完整 override 集。

  test.describe('取消收藏后聚合根消失', () => {
    test.use({
      wailsOverrides: {
        ...GIT_FLOW_OVERRIDES,
        // RemoveFavorite 之后收藏列表与聚合根层都只保留未取消的一项（__rules__ 按
        // 「方法此前是否被调用」切换，不受重建触发次数影响）
        GetFavorites: {
          __rules__: [{ after: 'RemoveFavorite', value: [AGG_FAVORITES[1]] }],
          __default__: AGG_FAVORITES
        },
        GetAggregatedRootNodes: {
          __rules__: [{ after: 'RemoveFavorite', value: [AGG_ROOT_NODES[1]] }],
          __default__: AGG_ROOT_NODES
        }
      }
    })

    test('右键取消收藏：该根节点自动消失，其余根保留', async ({ page }) => {
      await openAggregatedView(page)

      await page.locator('.el-tree-node', { hasText: '常用项目A' }).click({ button: 'right' })
      const menu = page.locator('.context-menu')
      await expect(menu).toBeVisible()
      await menu.getByText('取消收藏').click()

      // 收藏列表变化后聚合根层自动重建：取消项消失，另一项仍在
      await expect(page.locator('.el-tree-node', { hasText: '常用项目A' })).toHaveCount(0)
      await expect(page.locator('.el-tree-node', { hasText: '已删除目录' })).toBeVisible()

      const removeCalls = await getWailsCalls(page, 'RemoveFavorite')
      expect(removeCalls).toEqual([{ method: 'RemoveFavorite', args: [AGG_DIR_A] }])
    })

    test('命令面板取消收藏（外部入口）：仅靠收藏 watch 重建根层，节点自动消失', async ({ page }) => {
      await openAggregatedView(page)

      // 命令面板的取消收藏不经 FileTreePanel.handleRemoveFavorite 的显式刷新，
      // 只能由 favorites deep watch 触发重建——这是 ⑦ 的核心回归面。
      const palette = page.locator('.command-palette-dialog')
      await expect(async () => {
        await page.keyboard.press('Control+p')
        await expect(palette).toBeVisible()
      }).toPass({ timeout: 5000 })
      // '@' 前缀进入收藏模式，空查询列出全部收藏（含别名）
      await palette.getByPlaceholder(/搜索文件、目录/).fill('@')
      const favRow = palette.locator('.result-item', { hasText: '常用项目A' })
      await expect(favRow).toBeVisible()
      await favRow.locator('.remove-fav-btn').click()
      await page.keyboard.press('Escape')

      // 去抖窗口后聚合根层重建：取消项消失，另一项仍在
      await expect(page.locator('.el-tree-node', { hasText: '常用项目A' })).toHaveCount(0)
      await expect(page.locator('.el-tree-node', { hasText: '已删除目录' })).toBeVisible()

      const removeCalls = await getWailsCalls(page, 'RemoveFavorite')
      expect(removeCalls).toEqual([{ method: 'RemoveFavorite', args: [AGG_DIR_A] }])
    })
  })

  test.describe('收藏别名修改后显示名更新', () => {
    test.use({
      wailsOverrides: {
        ...GIT_FLOW_OVERRIDES,
        // 首次调用（FileTreePanel 挂载）返回原别名；命令面板打开后再次 loadFavorites
        // 返回改过别名的列表（模拟外部入口修改别名）
        GetFavorites: {
          __sequence__: [
            AGG_FAVORITES,
            [{ ...AGG_FAVORITES[0], alias: '项目A-新名' }, AGG_FAVORITES[1]]
          ]
        },
        GetAggregatedRootNodes: AGG_ROOT_NODES
      }
    })

    test('别名变化触发聚合根重建：显示名更新且展开态保持', async ({ page }) => {
      await openAggregatedView(page)

      // 展开收藏根：子节点就绪后作为「展开态保持」的锚点。
      // 子节点用 .el-tree-node__content 定位：.el-tree-node 的 hasText 会命中包含
      // 子孙文本的祖先节点，strict mode 下会解析出多个元素。
      await page.locator('.el-tree-node', { hasText: '常用项目A' }).click()
      await expect(page.locator('.el-tree-node__content', { hasText: 'README.md' })).toBeVisible()

      // 命令面板 visible watch 会 loadFavorites（收藏列表外部刷新的既有入口）；
      // 挂载竞态按 e2e-testing §6.1 范式：按键 + 面板可见整体重试
      await expect(activityPanel(page, 'directory')).toBeVisible()
      await expect(async () => {
        await page.keyboard.press('Control+p')
        await expect(page.locator('.command-palette-dialog')).toBeVisible()
      }).toPass({ timeout: 5000 })
      await page.keyboard.press('Escape')

      // 去抖后根层重建：显示名取新别名，且已展开的子节点随展开态恢复仍在
      await expect(page.locator('.el-tree-node__content', { hasText: '项目A-新名' })).toBeVisible()
      await expect(page.locator('.el-tree-node__content', { hasText: 'README.md' })).toBeVisible()

      const favCalls = await getWailsCalls(page, 'GetFavorites')
      expect(favCalls.length).toBeGreaterThanOrEqual(2)
    })
  })
})
