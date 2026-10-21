/**
 * 素材缩略图画廊懒加载与失败自动补恰一轮 E2E（assets-management.md §8.4，mock 驱动，
 * 矩阵候选 #17）。
 *
 * 覆盖：画廊挂载逐卡 GetAssetThumb(tier=0)（可视区懒加载）→ assets-thumbs:progress
 * 事件收集失败文件（failed 增量判定 + currentFile）→ 进度条中间态 → assets-thumbs:done
 * 事件 → 失败集按底册反查 fileId 自动 RetryAssetThumb 恰一次（thumbAutoRetried 标记）
 * → 下一批 progress/done 不再自动重试（恰一轮语义：损坏类必然再失败不放大）。
 *
 * 事件经 fixtures.js 的 emitWailsEvent 驱动（页面上下文调 mock runtime.EventsEmit
 * 同步广播，载荷形状对齐真实后端进度推送 {libraryId, done, total, failed,
 * currentFile}）；mock 数据源为 wails-mock-defaults.js E2E 合并表（GetAssetThumb
 * 缺省空串 = 未生成「无预览」占位、RetryAssetThumb null 安全 no-op）。
 * 本 spec 位于既有 e2e/ 通道目录，vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls, emitWailsEvent } from './fixtures'

const libFixture = { id: 'lib-e2e', name: 'E2E 素材库', rootPath: 'D:/e2e-assets', createdAt: '2026-10-21 10:00:00', updatedAt: '2026-10-21 10:00:00' }
const makeFile = (id, dir, title) => ({
  id,
  path: `${dir}/${title}.pptx`,
  size: 1024 * 1024,
  modTime: '2026-10-14 10:00:00',
  status: 'ok',
  title,
  note: '',
  rating: 0,
  tags: [],
  createdAt: '2026-10-14 10:00:00',
  updatedAt: '2026-10-14 10:00:00'
})

test.describe('素材缩略图懒加载与失败自动补', () => {
  test.use({
    wailsOverrides: {
      ListAssetLibraries: [libFixture],
      GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 2, slideCount: 2, failedCount: 0, encryptedCount: 0, lostCount: 0, excludedCount: 0 },
      ListAssetFiles: [makeFile(11, '方案A', '城商行方案'), makeFile(12, '方案B', '获客方案')],
      // 能力可用语境（重试链为可用态行为；返回空串 = 单卡生成失败「无预览」占位）
      AssetThumbsAvailable: true,
      GetAssetThumb: '',
      RetryAssetThumb: null
    }
  })

  test('画廊逐卡懒加载 tier=0，失败卡经 done 事件自动重试恰一轮', async ({ page }) => {
    await page.goto('/')
    await page.locator('.activity-bar-item').nth(4).click()
    await page.mouse.move(400, 400)
    await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
    const panel = page.locator('.assets-panel')
    await expect(panel).toBeVisible()
    const cards = panel.locator('.asset-card')
    await expect(cards).toHaveCount(2)

    // 画廊挂载逐卡可视区懒加载：GetAssetThumb(libraryId, fileId, slideNum=1, tier=0)
    await expect.poll(async () => (await getWailsCalls(page, 'GetAssetThumb')).length).toBeGreaterThanOrEqual(2)
    const thumbCalls = await getWailsCalls(page, 'GetAssetThumb')
    for (const call of thumbCalls) {
      expect(call.args).toEqual(['lib-e2e', expect.any(Number), 1, 0])
    }
    // 返回空串 = 未生成：卡面落「无预览」占位（失败占位形态）
    await expect(cards.nth(0).locator('.asset-card-thumb')).toContainText('无预览')

    // 批次一：progress 事件报一卡失败（failed 增量判定收集 currentFile），
    // 进度条中间态可见
    await emitWailsEvent(page, 'assets-thumbs:progress', {
      libraryId: 'lib-e2e', done: 1, total: 2, failed: 1, currentFile: '方案A/城商行方案.pptx'
    })
    const progress = panel.locator('.thumbs-progress')
    await expect(progress).toBeVisible()
    await expect(progress.locator('.scan-progress-text')).toHaveText('生成缩略图 1/2')

    // done 事件：失败集按底册 path 反查 fileId 自动重试，恰一次调用
    await emitWailsEvent(page, 'assets-thumbs:done', { libraryId: 'lib-e2e' })
    await expect(progress).toBeHidden() // 终态复位进度条
    let retryCalls = await getWailsCalls(page, 'RetryAssetThumb')
    expect(retryCalls.length).toBe(1)
    expect(retryCalls[0].args).toEqual(['lib-e2e', 11, '方案A/城商行方案.pptx'])

    // 批次二：再一轮 progress/done（thumbAutoRetried 已置位）——不再自动重试，
    // 恰一轮语义（损坏类必然再失败，一轮即止不放大）
    await emitWailsEvent(page, 'assets-thumbs:progress', {
      libraryId: 'lib-e2e', done: 1, total: 2, failed: 1, currentFile: '方案B/获客方案.pptx'
    })
    await expect(progress).toBeVisible()
    await emitWailsEvent(page, 'assets-thumbs:done', { libraryId: 'lib-e2e' })
    await expect(progress).toBeHidden()
    retryCalls = await getWailsCalls(page, 'RetryAssetThumb')
    expect(retryCalls.length).toBe(1)
  })
})
