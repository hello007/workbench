/**
 * 素材批量评分 E2E（第八批 SRS §2.6 P0 主链路，mock 驱动，矩阵候选 #13）。
 *
 * 覆盖：素材面板入口（活动栏）→ 文件视图勾选 3 项 → 浮动操作条「评分」→
 * 弹窗内 el-rate 设 4 星 → 提交 → mock 混合明细（2 成功 1 失败）→ 部分失败
 * 不中断语义：弹窗内切结果视图（成功 2 项 / 失败 1 项 + 失败行 = 后端拼装
 * label + message + danger tag）+ succeeded>0 照常上报 completed 触发刷新链
 * （ListAssetFiles 第二次重拉）+ 选中保留（PRD D5，数据变更点位非语境变更）
 * + BatchSetAssetRating 调用参数断言（库 + 目标数组 slideNum||0 归一 + 星级）。
 *
 * 形态复刻 assets-batch-tag.spec.js 先例；mock 数据源为 wails-mock-defaults.js
 * E2E 合并表 + 本文件 overrides（形状对齐 model.AssetLibrary / AssetFileEntry /
 * AssetBatchRatingResult，item 含后端拼装 label 字段）。
 * 本 spec 位于既有 e2e/ 通道目录，vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls } from './fixtures'

// 素材面板基础数据：单库 + 三个 ok 状态 pptx 文件；ListAssetFiles 用 __sequence__
// 描述符两帧驱动——首帧（挂载加载）无评分、第二帧（批量 completed 刷新链重拉）
// 前 11/12 带 4 星评分，验证「刷新链执行」来自批量完成而非初始数据；
// AssetThumbsAvailable=false 走降级路径（E2E 环境无 PowerPoint，缩略图面不参与）
const libFixture = { id: 'lib-e2e', name: 'E2E 素材库', rootPath: 'D:/e2e-assets', createdAt: '2026-10-15 10:00:00', updatedAt: '2026-10-15 10:00:00' }
const makeFile = (id, dir, title, rating) => ({
  id,
  path: `${dir}/${title}.pptx`,
  size: 1024 * 1024,
  modTime: '2026-10-14 10:00:00',
  status: 'ok',
  title,
  note: '',
  rating,
  tags: [],
  createdAt: '2026-10-14 10:00:00',
  updatedAt: '2026-10-14 10:00:00'
})

test.describe('素材管理批量评分', () => {
  test.use({
    wailsOverrides: {
      ListAssetLibraries: [libFixture],
      GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 3, slideCount: 6, failedCount: 0, encryptedCount: 0 },
      ListAssetFiles: {
        __sequence__: [
          [makeFile(11, '方案A', '城商行方案', 0), makeFile(12, '方案B', '获客方案', 0), makeFile(13, '方案C', '流失方案', 0)],
          [makeFile(11, '方案A', '城商行方案', 4), makeFile(12, '方案B', '获客方案', 4), makeFile(13, '方案C', '流失方案', 0)]
        ]
      },
      AssetThumbsAvailable: false,
      // 形状对齐 model.AssetBatchRatingResult：混合明细（2 成功 1 失败，每条目
      // 一事务逐条提交不回滚），失败条目 label 为后端拼装定位文案
      BatchSetAssetRating: {
        total: 3,
        succeeded: 2,
        failed: 1,
        items: [
          { fileId: 11, slideNum: 0, status: 'ok', message: '', label: '方案A/城商行方案.pptx' },
          { fileId: 12, slideNum: 0, status: 'ok', message: '', label: '方案B/获客方案.pptx' },
          { fileId: 13, slideNum: 0, status: 'error', message: '索引记录不存在', label: '方案C/流失方案.pptx' }
        ]
      }
    }
  })

  test('批量评分主链路：勾选 3 项→设 4 星→提交→混合明细失败行染 + completed 刷新 + 选中保留', async ({ page }) => {
    await page.goto('/')

    // 活动栏第 5 项（nth(4)）切换到素材管理面板；切面板后鼠标移离防 tooltip
    // 遮挡画廊首卡（时序坑先例见 assets-batch-tag.spec.js 同款注释）
    await page.locator('.activity-bar-item').nth(4).click()
    await page.mouse.move(400, 400)
    await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
    const panel = page.locator('.assets-panel')
    await expect(panel).toBeVisible()
    const cards = panel.locator('.asset-card')
    await expect(cards).toHaveCount(3)

    // 勾选三张文件卡：浮动操作条浮出（第八批批量评分入口挂此条）
    await cards.nth(0).locator('.asset-card-check').click()
    await cards.nth(1).locator('.asset-card-check').click()
    await cards.nth(2).locator('.asset-card-check').click()
    const bar = panel.locator('.asset-selection-bar')
    await expect(bar).toBeVisible()
    await expect(bar.locator('.selection-count')).toHaveText('已选 3 项')

    // 打开批量评分弹窗
    await bar.getByRole('button', { name: '评分', exact: true }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '批量评分' })
    await expect(dialog).toBeVisible()
    await expect(dialog.locator('.batch-rating-scope')).toContainText('将对选中的 3 项设置星级')

    // 设 4 星（el-rate 第 4 颗星；hint 回显当前星级语义）
    await dialog.locator('.el-rate__item').nth(3).click()
    await expect(dialog.locator('.batch-rating-hint')).toHaveText('4 星')

    // 提交：混合明细（有失败）→ 弹窗不关闭，切结果视图
    await dialog.getByRole('button', { name: '提交' }).click()
    const result = dialog.locator('.batch-rating-result')
    await expect(result).toBeVisible()
    await expect(result.locator('.summary-ok')).toHaveText('成功 2 项')
    await expect(result.locator('.summary-failed')).toHaveText('失败 1 项')

    // 失败行染：danger tag + 后端拼装 label（第一数据源）+ 失败原因
    const failedRow = result.locator('.batch-rating-failed-row')
    await expect(failedRow).toHaveCount(1)
    await expect(failedRow.locator('.el-tag').filter({ hasText: '失败' })).toBeVisible()
    await expect(failedRow.locator('.failed-target')).toHaveText('方案C/流失方案.pptx')
    await expect(failedRow.locator('.failed-msg')).toHaveText('索引记录不存在')

    // 调用参数断言：单一入口混合目标（文件形态 slideNum 缺省经 ||0 归一）+ 星级
    const calls = await getWailsCalls(page, 'BatchSetAssetRating')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual([
      'lib-e2e',
      [
        { fileId: 11, slideNum: 0 },
        { fileId: 12, slideNum: 0 },
        { fileId: 13, slideNum: 0 }
      ],
      4
    ])

    // succeeded > 0 照常上报 completed：宿主刷新链重拉底册（ListAssetFiles 第二次）
    await expect.poll(async () => (await getWailsCalls(page, 'ListAssetFiles')).length).toBe(2)

    // 选中保留（PRD D5：数据变更点位非语境变更，支持对失败项单独重试）
    await expect(bar).toBeVisible()
    await expect(bar.locator('.selection-count')).toHaveText('已选 3 项')

    // 关闭结果视图（限定页脚作用域——EP 头部关闭钮的可访问名同样含「关闭」）
    await dialog.locator('.el-dialog__footer').getByRole('button', { name: '关闭' }).click()
    await expect(dialog).not.toBeVisible()
  })
})
