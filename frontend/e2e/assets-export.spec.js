/**
 * 素材管理元数据导出 E2E（第九批 SRS §2.6.4 主链路，mock 驱动）。
 *
 * 覆盖：素材面板工具行「导出」→ 弹窗范围预览（全库 = stats 合计，浏览语境无选中
 * 无检索不触发预览查询）→ 填 CSV 目标 → 提交成功（toast + 关闭 + 调用参数断言：
 * formats/pathMode/overwrite/范围字段）→ 目标已存在确认型链路（后端确认错误 →
 * ElMessageBox 覆盖确认 → overwrite=true 重调 → 恒拒下 warning toast + 弹窗保持）。
 * mock 数据源为 wails-mock-defaults.js E2E 合并表 + 本文件 overrides（形状对齐
 * model.AssetLibrary / AssetLibraryStats / AssetFileEntry / AssetExportResult）。
 * 本 spec 位于既有 e2e/ 通道目录，vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls } from './fixtures'

const libFixture = { id: 'lib-e2e', name: 'E2E 素材库', rootPath: 'D:/e2e-assets', createdAt: '2026-10-16 10:00:00', updatedAt: '2026-10-16 10:00:00' }
const makeFile = (id, title) => ({
  id,
  path: `${title}.pptx`,
  size: 1024 * 1024,
  modTime: '2026-10-15 10:00:00',
  status: 'ok',
  title,
  note: '',
  rating: 0,
  tags: [],
  createdAt: '2026-10-15 10:00:00',
  updatedAt: '2026-10-15 10:00:00'
})
const baseOverrides = {
  ListAssetLibraries: [libFixture],
  GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 2, slideCount: 4, failedCount: 0, encryptedCount: 0 },
  ListAssetFiles: [makeFile(11, '城商行方案'), makeFile(12, '获客方案')],
  AssetThumbsAvailable: false
}

const openPanel = async (page) => {
  await page.goto('/')
  // 活动栏第 5 项（nth(4)）切换到素材管理面板；鼠标移离防 tooltip 遮挡画廊
  //（时序坑见 assets-batch-tag.spec 同款注释）
  await page.locator('.activity-bar-item').nth(4).click()
  await page.mouse.move(400, 400)
  await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
  await expect(page.locator('.assets-panel')).toBeVisible()
}

test.describe('素材管理元数据导出：全库导出主链路', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      // 形状对齐 model.AssetExportResult
      ExportAssetMetadata: { libraryId: 'lib-e2e', csvPath: '', mdDir: '', rows: 6, mdFiles: 0 }
    }
  })

  test('范围预览=库统计合计，提交成功 toast + 关闭 + 调用参数', async ({ page }) => {
    await openPanel(page)

    // 工具行「导出」入口（浏览语境无选中无检索：范围 = 全库，预览 = stats 合计 2+4）
    await page.locator('.assets-panel').getByRole('button', { name: '导出' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '导出元数据' })
    await expect(dialog).toBeVisible()
    await expect(dialog.locator('.export-scope')).toContainText('将导出 6 条')
    await expect(dialog.locator('.export-scope')).toContainText('全库')

    // 填 CSV 目标（真实 EP el-input 内层为 .el-input__inner；MD 未勾选其输入禁用）
    await dialog.locator('.el-input__inner').first().fill('D:/e2e-export/out.csv')
    await dialog.locator('.el-dialog__footer').getByRole('button', { name: '导出' }).click()

    // 成功 toast + 弹窗关闭
    await expect(page.locator('.el-message').filter({ hasText: '已导出 6 条元数据' })).toBeVisible()
    await expect(dialog).not.toBeVisible()

    // 调用参数断言：范围字段（浏览语境选中/检索皆空）+ partial 形状
    const calls = await getWailsCalls(page, 'ExportAssetMetadata')
    expect(calls.length).toBe(1)
    const opts = calls[0].args[1]
    expect(opts.csvPath).toBe('D:/e2e-export/out.csv')
    expect(opts.formats).toEqual(['csv'])
    expect(opts.pathMode).toBe('relative')
    expect(opts.overwrite).toBe(false)
    expect(opts.fileIds).toEqual([])
    expect(opts.slideTargets).toEqual([])
    expect(opts.keyword).toBe('')
    // 预览阶段无 SearchAssets 调用（全库范围直接用 stats 合计，不发预览查询）
    const searchCalls = await getWailsCalls(page, 'SearchAssets')
    expect(searchCalls.length).toBe(0)
  })
})

test.describe('素材管理元数据导出：目标已存在确认链路', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      // 恒拒 override（E2E 描述符无「先错后成」序列形态）：确认型错误码驱动
      // ElMessageBox 覆盖确认链路；重调仍拒在组件内闭环再次弹确认框
      //（runExport 递归语义：确认型错误恒弹覆盖确认，非 handleError 分流）
      ExportAssetMetadata: {
        __error__: '导出目标已存在：D:/e2e-export/out.csv。确认覆盖将重写同名内容',
        __code__: 'E_ASSETS_EXPORT_TARGET_EXISTS'
      }
    }
  })

  test('确认框覆盖写入重调 overwrite=true，恒拒下再次弹确认框且导出弹窗保持', async ({ page }) => {
    await openPanel(page)

    await page.locator('.assets-panel').getByRole('button', { name: '导出' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '导出元数据' })
    await expect(dialog).toBeVisible()
    await dialog.locator('.el-input__inner').first().fill('D:/e2e-export/out.csv')
    await dialog.locator('.el-dialog__footer').getByRole('button', { name: '导出' }).click()

    // 确认框弹出（消息含后端目标路径），点「覆盖写入」→ overwrite=true 重调。
    // EP 2.14 关闭后的 message-box 实例仍驻留 DOM 且二次弹出后同名选择器命中
    // 2 元素——首框取 .first()、重调后的新框取 .last()（DOM 追加序 = 弹出序）
    const msgbox = page.locator('.el-message-box')
    await expect(msgbox.first()).toBeVisible()
    await expect(msgbox.first()).toContainText('导出目标已存在：D:/e2e-export/out.csv')
    await msgbox.first().getByRole('button', { name: '覆盖写入' }).click()

    // 重调仍拒（mock 恒拒）：确认型错误在组件内闭环再次弹覆盖确认框（EP 2.14 起
    // 关闭动画期间重开正常显示，覆盖链路可重复），导出弹窗保持打开
    await expect(msgbox.last()).toBeVisible()
    await expect(msgbox.last()).toContainText('导出目标已存在：D:/e2e-export/out.csv')
    await expect(dialog).toBeVisible()

    // 两次调用：首次 overwrite=false，确认后重调 overwrite=true
    const calls = await getWailsCalls(page, 'ExportAssetMetadata')
    expect(calls.length).toBe(2)
    expect(calls[0].args[1].overwrite).toBe(false)
    expect(calls[1].args[1].overwrite).toBe(true)
    expect(calls[1].args[1].csvPath).toBe('D:/e2e-export/out.csv')
  })
})
