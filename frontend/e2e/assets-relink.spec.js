/**
 * 素材文件丢失与重链 E2E（第九批 SRS §2.6.5 主链路，mock 驱动）。
 *
 * 覆盖：lost 呈现（统计行丢失数 + 卡片丢失标签 + 详情「重新定位」入口）→ 手动
 * 重链主链路（文件对话框选库内新位置 → 哈希匹配直复活 toast → Relocate 调用参数
 * 断言）→ 哈希不等确认链路（E_ASSETS_RELINK_MISMATCH → ElMessageBox 强制重链
 * 确认 → force=true 重调）→ 前缀替换弹窗（预览命中数 + 二次确认 → 执行调用参数）
 * → 清理丢失（确认弹窗列出记录数 → Clear 调用）。
 * mock 数据源为 wails-mock-defaults.js E2E 合并表 + 本文件 overrides（形状对齐
 * model.AssetLibrary / AssetLibraryStats / AssetFileEntry / AssetRelocateResult /
 * AssetPrefixReplaceResult）。
 * 本 spec 位于既有 e2e/ 通道目录，vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls } from './fixtures'

const libFixture = { id: 'lib-e2e', name: 'E2E 素材库', rootPath: 'D:/e2e-assets', createdAt: '2026-10-17 10:00:00', updatedAt: '2026-10-17 10:00:00' }
const makeFile = (id, title, status = 'ok') => ({
  id,
  path: `${title}.pptx`,
  size: 1024 * 1024,
  modTime: '2026-10-15 10:00:00',
  status,
  title,
  note: '',
  rating: 0,
  tags: [],
  createdAt: '2026-10-15 10:00:00',
  updatedAt: '2026-10-15 10:00:00'
})
const baseOverrides = {
  ListAssetLibraries: [libFixture],
  GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 2, slideCount: 3, failedCount: 0, encryptedCount: 0, lostCount: 1 },
  ListAssetFiles: [makeFile(11, '城商行方案'), makeFile(12, '已移走方案', 'lost')],
  AssetThumbsAvailable: false
}

const openLostDetail = async (page) => {
  await page.goto('/')
  // 活动栏第 5 项（nth(4)）切换到素材管理面板；鼠标移离防 tooltip 遮挡画廊
  await page.locator('.activity-bar-item').nth(4).click()
  await page.mouse.move(400, 400)
  await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
  await expect(page.locator('.assets-panel')).toBeVisible()
  // lost 呈现：统计行丢失数 + 卡片丢失标签
  await expect(page.locator('.assets-stats-line')).toContainText('丢失 1')
  const lostCard = page.locator('.asset-card').filter({ hasText: '已移走方案' })
  await expect(lostCard.locator('.el-tag').filter({ hasText: '丢失' })).toBeVisible()
  // 点击丢失卡进详情，「重新定位」入口可见
  await lostCard.click()
  await expect(page.locator('.assets-detail')).toBeVisible()
  return page.getByRole('button', { name: '重新定位' })
}

test.describe('素材文件丢失与重链：手动重链主链路', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      RelocateAssetFile: { fileId: 12, oldPath: '已移走方案.pptx', newPath: '新目录/已移走方案.pptx', hashMatched: true, skippedHashCheck: false, reparsed: false },
      OpenFileDialog: 'D:/e2e-assets/新目录/已移走方案.pptx'
    }
  })

  test('lost 卡片标记 + 重新定位：哈希匹配直复活 toast + 调用参数', async ({ page }) => {
    const relocateBtn = await openLostDetail(page)
    await relocateBtn.click()

    // 哈希匹配分型 toast（不触发强制确认框）
    await expect(page.locator('.el-message').filter({ hasText: '已重链：文件内容与原记录一致' })).toBeVisible()
    await expect(page.locator('.el-message-box')).not.toBeVisible()

    const calls = await getWailsCalls(page, 'RelocateAssetFile')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e', 12, 'D:/e2e-assets/新目录/已移走方案.pptx', false])
  })
})

test.describe('素材文件丢失与重链：哈希不等确认链路', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      // 恒拒 override（E2E 描述符无「先错后成」序列形态）：确认型错误码驱动
      // ElMessageBox 强制重链确认链路；重调仍拒走 handleError warning 分流
      RelocateAssetFile: {
        __error__: '目标文件内容与原记录不一致（哈希校验未通过，可能是不同文件或已被修改）。强制重链将按新文件重新解析，已有标签与标注保留。',
        __code__: 'E_ASSETS_RELINK_MISMATCH'
      },
      OpenFileDialog: 'D:/e2e-assets/另一份.pptx'
    }
  })

  test('强制重链确认 → force=true 重调，恒拒下 warning 提示', async ({ page }) => {
    const relocateBtn = await openLostDetail(page)
    await relocateBtn.click()

    const msgbox = page.locator('.el-message-box')
    await expect(msgbox).toBeVisible()
    await expect(msgbox).toContainText('目标文件内容与原记录不一致')
    await msgbox.getByRole('button', { name: '强制重链' }).click()

    // 重调仍拒（mock 恒拒）：warning 分流提示（确认型码不拼前缀）
    await expect(page.locator('.el-message').filter({ hasText: '目标文件内容与原记录不一致' })).toBeVisible()

    const calls = await getWailsCalls(page, 'RelocateAssetFile')
    expect(calls.length).toBe(2)
    expect(calls[0].args[3]).toBe(false)
    expect(calls[1].args[3]).toBe(true)
  })
})

test.describe('素材文件丢失与重链：前缀替换与清理丢失', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      PreviewAssetPathPrefixReplace: 2,
      ReplaceAssetPathPrefix: {
        libraryId: 'lib-e2e', oldPrefix: '旧目录', newPrefix: '新目录',
        replaced: 2, revived: 2, reparsed: 0, stillLost: 0,
        scan: { total: 2, new: 0, updated: 0, unchanged: 2, failed: 0, encrypted: 0, denied: 0, lost: 0, revived: 0, removed: 0 }
      },
      ClearLostAssetFiles: 1
    }
  })

  test('前缀替换：预览命中数 + 二次确认 + 执行调用参数 + 结果 toast', async ({ page }) => {
    await page.goto('/')
    await page.locator('.activity-bar-item').nth(4).click()
    await page.mouse.move(400, 400)
    await expect(page.locator('.assets-panel')).toBeVisible()

    // 工具行「维护」→「路径前缀替换」（EP 下拉菜单挂 body）
    await page.locator('.assets-panel').getByRole('button', { name: '维护' }).click()
    await page.getByRole('menuitem', { name: '路径前缀替换…' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '路径前缀替换' })
    await expect(dialog).toBeVisible()

    // 填新旧前缀 → 防抖预览命中 2（mock 恒返 2）
    await dialog.locator('.el-input__inner').first().fill('旧目录')
    await dialog.locator('.el-input__inner').nth(1).fill('新目录')
    await expect(dialog.locator('.prefix-preview')).toContainText('命中 2 条索引记录')

    // 提交 → 二次确认（命中数 + 新旧前缀）→ 确认执行
    await dialog.locator('.el-dialog__footer').getByRole('button', { name: '替换' }).click()
    const msgbox = page.locator('.el-message-box')
    await expect(msgbox).toBeVisible()
    await expect(msgbox).toContainText('旧前缀「旧目录」命中 2 条记录')
    await expect(msgbox).toContainText('新前缀「新目录」')
    await msgbox.getByRole('button', { name: '替换' }).click()

    // 三态结果 toast + 弹窗关闭
    await expect(page.locator('.el-message').filter({ hasText: '前缀替换完成：更新 2 条路径，恢复 2' })).toBeVisible()
    await expect(dialog).not.toBeVisible()

    const previewCalls = await getWailsCalls(page, 'PreviewAssetPathPrefixReplace')
    expect(previewCalls.length).toBe(1)
    expect(previewCalls[0].args).toEqual(['lib-e2e', '旧目录', '新目录'])
    const calls = await getWailsCalls(page, 'ReplaceAssetPathPrefix')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e', '旧目录', '新目录'])
  })

  test('清理丢失：确认弹窗列出记录数 → Clear 调用 + 结果 toast', async ({ page }) => {
    await page.goto('/')
    await page.locator('.activity-bar-item').nth(4).click()
    await page.mouse.move(400, 400)
    await expect(page.locator('.assets-panel')).toBeVisible()

    await page.locator('.assets-panel').getByRole('button', { name: '维护' }).click()
    await page.getByRole('menuitem', { name: /清理丢失记录（1）*/ }).click()

    const msgbox = page.locator('.el-message-box')
    await expect(msgbox).toBeVisible()
    await expect(msgbox).toContainText('将删除 1 条丢失记录')
    await msgbox.getByRole('button', { name: '清理' }).click()

    await expect(page.locator('.el-message').filter({ hasText: '已清理 1 条丢失记录' })).toBeVisible()
    const calls = await getWailsCalls(page, 'ClearLostAssetFiles')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e'])
  })
})
