/**
 * 素材库索引设置与备份还原 E2E（素材管理第十批 SRS §2.7.3 / §2.7.5 主链路，mock 驱动）。
 *
 * 覆盖：索引设置（排除规则弹窗打开 → 编辑目录/后缀规则 → 保存 → UpdateAssetLibraryRules
 * 调用参数与成功 toast）→ 备份（一键快照调用与落点 toast）→ 还原（清单弹层打开 +
 * 行渲染 → 还原确认 → RestoreAssetLibrary 调用参数）→ 删除备份失败升级确认链
 * （E_ASSETS_DELETE_BACKUP_FAILED 恒拒 → error 语义强确认 → allowNoBackup=true 重调）。
 * mock 数据源为 wails-mock-defaults.js E2E 合并表 + 本文件 overrides（形状对齐
 * model.AssetLibrary / AssetBackupEntry / AssetScanResult）。
 * 本 spec 位于既有 e2e/ 通道目录，vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls } from './fixtures'

const libFixture = {
  id: 'lib-e2e',
  name: 'E2E 素材库',
  rootPath: 'D:/e2e-assets',
  createdAt: '2026-10-18 10:00:00',
  updatedAt: '2026-10-18 10:00:00',
  excludeDirs: [],
  excludeExts: []
}
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
const backupFixture = {
  libraryId: 'lib-e2e',
  fileName: '20261017-090000.db',
  path: 'D:/data/assets/backups/lib-e2e/20261017-090000.db',
  sizeBytes: 2048 * 1024,
  createdAt: '2026-10-17 09:00:00'
}

const baseOverrides = {
  ListAssetLibraries: [libFixture],
  GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 2, slideCount: 3, failedCount: 0, encryptedCount: 0, lostCount: 0, excludedCount: 0 },
  ListAssetFiles: [makeFile(11, '城商行方案'), makeFile(12, '获客方案')],
  AssetThumbsAvailable: false
}

// 打开素材管理面板并等待就绪
const openAssetsPanel = async (page) => {
  await page.goto('/')
  await page.locator('.activity-bar-item').nth(4).click()
  await page.mouse.move(400, 400)
  await expect(page.locator('.assets-panel')).toBeVisible()
  await expect(page.locator('.assets-stats-line')).toBeVisible()
}

test.describe('素材库索引设置与备份还原：排除规则编辑', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      UpdateAssetLibraryRules: { total: 2, new: 0, updated: 0, unchanged: 0, failed: 0, encrypted: 0, denied: 0, excluded: 2, lost: 0, revived: 0, removed: 0 }
    }
  })

  test('添加目录与后缀规则保存，UpdateAssetLibraryRules 收归一参数并提示汇总', async ({ page }) => {
    await openAssetsPanel(page)

    await page.getByRole('button', { name: '索引设置' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '索引设置' })
    await expect(dialog).toBeVisible()

    // 目录规则输入行：填入后回车加入
    const dirInput = dialog.locator('.rules-field').filter({ hasText: '排除文件夹' }).locator('input')
    await dirInput.fill('*草稿*')
    await dirInput.press('Enter')
    await expect(dialog.locator('.el-tag').filter({ hasText: '*草稿*' })).toBeVisible()

    // 后缀规则输入行：填 tmp 形态，展示归一带点值
    const extInput = dialog.locator('.rules-field').filter({ hasText: '忽略后缀' }).locator('input')
    await extInput.fill('tmp')
    await extInput.press('Enter')
    await expect(dialog.locator('.el-tag').filter({ hasText: '.tmp' })).toBeVisible()

    await dialog.getByRole('button', { name: '保存并重新扫描' }).click()

    // 后端同步重扫返回带 excluded 汇总：toast 分型
    await expect(page.locator('.el-message').filter({ hasText: '新排除 2 个文件' })).toBeVisible()

    const calls = await getWailsCalls(page, 'UpdateAssetLibraryRules')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e', ['*草稿*'], ['.tmp']])
  })
})

test.describe('素材库索引设置与备份还原：手动备份', () => {
  test.use({
    wailsOverrides: { ...baseOverrides, BackupAssetLibrary: backupFixture }
  })

  test('点击即调 BackupAssetLibrary，成功提示含落点路径', async ({ page }) => {
    await openAssetsPanel(page)

    await page.getByRole('button', { name: '备份', exact: true }).click()
    await expect(page.locator('.el-message').filter({ hasText: '备份完成' })).toBeVisible()

    const calls = await getWailsCalls(page, 'BackupAssetLibrary')
    expect(calls).toHaveLength(1)
    expect(calls[0].args).toEqual(['lib-e2e'])
  })
})

test.describe('素材库索引设置与备份还原：还原链', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      ListAssetLibraryBackups: [backupFixture]
    }
  })

  test('备份清单弹层渲染 → 选份确认 → RestoreAssetLibrary 收条目绝对路径', async ({ page }) => {
    await openAssetsPanel(page)

    await page.getByRole('button', { name: '还原' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '备份还原' })
    await expect(dialog).toBeVisible()
    // 清单行渲染（时间/文件名/大小）
    await expect(dialog.locator('.el-table')).toContainText('2026-10-17 09:00:00')
    await expect(dialog.locator('.el-table')).toContainText('20261017-090000.db')
    await expect(dialog.locator('.el-table')).toContainText('2 MB')

    // 行内还原 → 确认弹窗明示覆盖后果 → 确认执行
    await dialog.getByRole('button', { name: '还原' }).click()
    const msgbox = page.locator('.el-message-box')
    await expect(msgbox).toBeVisible()
    await expect(msgbox).toContainText('覆盖当前库全部索引与标注')
    await msgbox.getByRole('button', { name: '还原' }).click()

    await expect(page.locator('.el-message').filter({ hasText: '已还原备份' })).toBeVisible()
    const calls = await getWailsCalls(page, 'RestoreAssetLibrary')
    expect(calls).toHaveLength(1)
    expect(calls[0].args).toEqual(['lib-e2e', 'D:/data/assets/backups/lib-e2e/20261017-090000.db'])
  })
})

test.describe('素材库索引设置与备份还原：删除备份失败升级确认', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      // 恒拒 override（E2E 描述符无「先错后成」序列形态）：确认型错误码驱动
      // 升级强确认链路；重调仍拒走 handleError warning 分流（确认型码不拼前缀）
      DeleteAssetLibrary: {
        __error__: '删除前自动备份失败（磁盘空间不足），已取消本次删除。',
        __code__: 'E_ASSETS_DELETE_BACKUP_FAILED'
      }
    }
  })

  test('error 语义强确认后 allowNoBackup=true 重调', async ({ page }) => {
    await openAssetsPanel(page)

    await page.getByRole('button', { name: '删除库' }).click()
    // 首个确认（删除素材库）→ 确认后后端拒绝。EP messagebox 关闭动画期间旧 DOM
    // 残留（display:none 不移除），恒用 :visible 过滤当前活动框防 strict mode 冲突
    let msgbox = page.locator('.el-message-box:visible')
    await expect(msgbox).toBeVisible()
    await msgbox.getByRole('button', { name: '删除' }).click()

    // 升级强确认（error 语义 + 无恢复点文案）
    msgbox = page.locator('.el-message-box:visible').filter({ hasText: '无备份删除确认' })
    await expect(msgbox).toBeVisible()
    await expect(msgbox).toContainText('删除后无法恢复')
    await msgbox.getByRole('button', { name: '仍要删除' }).click()

    // 重调仍拒（mock 恒拒）：确认型码 warning 分流提示
    await expect(page.locator('.el-message').filter({ hasText: '删除前自动备份失败' })).toBeVisible()

    const calls = await getWailsCalls(page, 'DeleteAssetLibrary')
    expect(calls.length).toBe(2)
    expect(calls[0].args).toEqual(['lib-e2e', false])
    expect(calls[1].args).toEqual(['lib-e2e', true])
  })
})
