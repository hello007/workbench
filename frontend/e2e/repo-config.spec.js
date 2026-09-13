/**
 * 仓库列表配置导入导出 E2E（mock 文件对话框 + 导入预览-决策链路）。
 *
 * 桥接契约：文件对话框（SaveFileDialog/OpenFileDialog）与文件读写（SaveFile/
 * ReadFileBytes）均经后端桥接（见 docs/spec/cross-layer-contracts.md），E2E 以
 * wailsOverrides 注入返回值替代原生对话框，覆盖：
 * 1. 工具栏入口渲染：导出/导入两个圆形按钮可见
 * 2. 取消路径：保存对话框取消（返回空串）不落盘；导入选文件取消静默关闭
 * 3. 导出链路：ExportRepoConfig() 取 manifest 文本 → SaveFileDialog(默认名, filters)
 *    参数正确 → SaveFile(path, text, 'utf-8') 落盘
 * 4. 导入预览-决策链路：OpenFileDialog 选文件 → ReadFileBytes →
 *    PreviewRepoConfigImport 展示预览（新增/冲突/非法段落）→ 冲突项改决策
 *    「覆盖本机」→ 确认导入 → ApplyRepoConfigImport(jsonText, decisions) 参数正确 →
 *    结果汇总（新增/覆盖/跳过/失败计数）渲染 → 完成关闭
 * 5. 空预览：无可导入项时确认按钮禁用
 */
import { test, expect, getWailsCalls } from './fixtures'

const MANIFEST_TEXT = JSON.stringify({
  manifestVersion: 1,
  exportedAt: '2026-09-13T00:00:00Z',
  directories: [
    { name: '导入新目录', path: 'D:/ws/new-dir', isDefault: false },
    { name: '导入冲突目录', path: 'D:/ws/conflict', isDefault: true }
  ],
  favorites: [{ path: 'D:/ws/fav', alias: '收藏', group: '默认', createdAt: 1 }]
})

/** 导入预览 payload：与 MANIFEST_TEXT 对齐（1 新增目录 + 1 冲突目录 + 1 新增收藏） */
const PREVIEW_PAYLOAD = {
  newDirectories: [{ name: '导入新目录', path: 'D:/ws/new-dir', isDefault: false }],
  conflictDirectories: [{ name: '导入冲突目录', path: 'D:/ws/conflict', isDefault: true }],
  newFavorites: [{ path: 'D:/ws/fav', alias: '收藏', group: '默认', createdAt: 1 }],
  conflictFavorites: [],
  invalid: []
}

/** 公共前置：打开首页，等待工作目录面板与工具栏按钮渲染 */
async function openHomePage(page) {
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.goto('/')
  await expect(page.locator('.directory-tree-panel')).toBeVisible()
  await expect(page.locator('button[title="导出仓库列表配置"]')).toBeVisible()
  await expect(page.locator('button[title="导入仓库列表配置"]')).toBeVisible()
}

test.describe('仓库列表配置导入导出：入口与取消路径（默认 mock）', () => {
  // 基础表 SaveFileDialog/OpenFileDialog 默认返回空串 = 用户取消
  test('工具栏导出/导入按钮渲染', async ({ page }) => {
    await openHomePage(page)
  })

  test('导出：保存对话框取消时不落盘', async ({ page }) => {
    await openHomePage(page)
    await page.locator('button[title="导出仓库列表配置"]').click()

    // 用户取消：无成功提示、无 SaveFile 调用
    await expect(page.locator('.el-message--success')).toHaveCount(0)
    expect(await getWailsCalls(page, 'SaveFile')).toHaveLength(0)
    // 后端导出文本已被调用（先取文本后选路径）
    expect(await getWailsCalls(page, 'ExportRepoConfig')).toHaveLength(1)
  })

  test('导入：选文件取消时静默关闭对话框', async ({ page }) => {
    await openHomePage(page)
    await page.locator('button[title="导入仓库列表配置"]').click()

    // OpenFileDialog 返回空串 → 对话框自动关闭，不读文件
    const dialog = page.locator('.repo-import-dialog')
    await expect(dialog).not.toBeVisible()
    expect(await getWailsCalls(page, 'ReadFileBytes')).toHaveLength(0)
  })
})

test.describe('仓库列表配置导入导出：导出与导入主链路', () => {
  test.use({
    wailsOverrides: {
      SaveFileDialog: 'D:/e2e-export/repo_config.json',
      OpenFileDialog: 'D:/e2e-import/repo_config.json',
      ReadFileBytes: { base64: Buffer.from(MANIFEST_TEXT, 'utf-8').toString('base64') },
      PreviewRepoConfigImport: PREVIEW_PAYLOAD,
      ApplyRepoConfigImport: { added: 0, overwritten: 1, skipped: 1, failed: 0, failedReasons: [] }
    }
  })

  test('导出链路：ExportRepoConfig → SaveFileDialog 选路径 → SaveFile 按 UTF-8 落盘', async ({ page }) => {
    await openHomePage(page)
    await page.locator('button[title="导出仓库列表配置"]').click()
    await expect(page.locator('.el-message--success')).toBeVisible()

    expect(await getWailsCalls(page, 'ExportRepoConfig')).toEqual([{ method: 'ExportRepoConfig', args: [] }])
    expect(await getWailsCalls(page, 'SaveFileDialog')).toEqual([
      { method: 'SaveFileDialog', args: ['repo_config.json', [{ DisplayName: 'JSON 文件', Pattern: '*.json' }]] }
    ])

    const saveCalls = await getWailsCalls(page, 'SaveFile')
    expect(saveCalls).toHaveLength(1)
    expect(saveCalls[0].args[0]).toBe('D:/e2e-export/repo_config.json')
    expect(saveCalls[0].args[1]).toContain('"manifestVersion":1')
    expect(saveCalls[0].args[2]).toBe('utf-8')
  })

  test('导入链路：选文件 → 预览 → 冲突改覆盖 → 确认导入 → 结果汇总', async ({ page }) => {
    await openHomePage(page)
    await page.locator('button[title="导入仓库列表配置"]').click()

    // 预览对话框渲染：三类段落可见
    const dialog = page.locator('.repo-import-dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog.getByText('将新增工作目录（1）')).toBeVisible()
    await expect(dialog.getByText('冲突工作目录（1）')).toBeVisible()
    await expect(dialog.getByText('将新增收藏（1）')).toBeVisible()

    // 桥接调用参数：OpenFileDialog → ReadFileBytes → PreviewRepoConfigImport
    expect(await getWailsCalls(page, 'OpenFileDialog')).toEqual([
      { method: 'OpenFileDialog', args: ['选择仓库列表配置文件', [{ DisplayName: 'JSON 文件', Pattern: '*.json' }]] }
    ])
    const readCalls = await getWailsCalls(page, 'ReadFileBytes')
    expect(readCalls[0].args[0]).toBe('D:/e2e-import/repo_config.json')
    const previewCalls = await getWailsCalls(page, 'PreviewRepoConfigImport')
    expect(previewCalls[0].args[0]).toBe(MANIFEST_TEXT)

    // 冲突目录改决策为「覆盖本机」（默认跳过）
    const conflictRow = dialog.locator('.preview-conflict', { hasText: '导入冲突目录' })
    await conflictRow.locator('.el-radio', { hasText: '覆盖本机' }).click()

    // 确认导入 → 结果汇总（mock 返回 1 覆盖 / 1 跳过）
    await dialog.getByRole('button', { name: '确认导入' }).click()
    await expect(dialog.getByText('覆盖 1')).toBeVisible()
    await expect(dialog.getByText('跳过 1')).toBeVisible()

    // 执行调用参数：导入文本 + 冲突决策表（新增项无决策、收藏默认跳过不入表）
    const applyCalls = await getWailsCalls(page, 'ApplyRepoConfigImport')
    expect(applyCalls).toHaveLength(1)
    expect(applyCalls[0].args[0]).toBe(MANIFEST_TEXT)
    expect(applyCalls[0].args[1]).toEqual({
      directories: { 'D:/ws/conflict': 'overwrite' },
      favorites: {}
    })

    // 完成关闭
    await dialog.getByRole('button', { name: '完成' }).click()
    await expect(dialog).not.toBeVisible()
  })
})

test.describe('仓库列表配置导入导出：空预览（全部非法/空配置）', () => {
  test.use({
    wailsOverrides: {
      OpenFileDialog: 'D:/e2e-import/empty.json',
      ReadFileBytes: { base64: '' }
    }
  })

  test('空预览时确认按钮禁用', async ({ page }) => {
    await openHomePage(page)
    await page.locator('button[title="导入仓库列表配置"]').click()

    const dialog = page.locator('.repo-import-dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog.getByText('无可导入项')).toBeVisible()
    await expect(dialog.getByRole('button', { name: '确认导入' })).toBeDisabled()
  })
})
