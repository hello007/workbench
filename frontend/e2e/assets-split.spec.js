/**
 * 素材管理 PPT 拆分 E2E（SRS §2.2.3 库级批量拆分主链路，mock 驱动）。
 *
 * 覆盖：素材面板入口（活动栏）→ 库级「拆分全部 PPT」→ 落点确认弹窗（radio 默认
 * 素材库缓存目录、切源目录出现自动排除索引警示）→ 开始拆分 → mock 拆分完成 →
 * 汇总提示 + SplitAssetLibrary 调用参数断言（getWailsCalls）。
 * mock 数据源为 wails-mock-defaults.js E2E 合并表 + 本文件 overrides（形状对齐
 * model.AssetLibrary / AssetFileEntry / AssetSplitBatchResult）。
 */
import { test, expect, getWailsCalls } from './fixtures'

test.describe('素材管理 PPT 拆分', () => {
  // 素材面板基础数据：单库 + 两个 ok 状态 pptx 文件（画廊可渲染、拆分入口可用）；
  // AssetThumbsAvailable=false 走降级路径（E2E 环境无 PowerPoint，缩略图面不参与）
  // （test.use 须在 describe 层声明，test 函数体内调用非法）
  const assetsOverrides = {
    ListAssetLibraries: [
      { id: 'lib-e2e', name: 'E2E 素材库', rootPath: 'D:/e2e-assets', createdAt: '2026-10-14 10:00:00', updatedAt: '2026-10-14 10:00:00' }
    ],
    GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 2, slideCount: 4, failedCount: 0, encryptedCount: 0 },
    ListAssetFiles: [
      { id: 11, path: '方案A/城商行方案.pptx', size: 1024 * 1024, modTime: '2026-10-13 10:00:00', status: 'ok', title: '城商行方案', note: '', rating: 0, tags: [], createdAt: '2026-10-13 10:00:00', updatedAt: '2026-10-13 10:00:00' },
      { id: 12, path: '方案B/获客方案.pptx', size: 2 * 1024 * 1024, modTime: '2026-10-13 11:00:00', status: 'ok', title: '获客方案', note: '', rating: 0, tags: [], createdAt: '2026-10-13 11:00:00', updatedAt: '2026-10-13 11:00:00' }
    ],
    AssetThumbsAvailable: false,
    // 形状对齐 model.AssetSplitBatchResult：两个文件全部拆分成功（无失败明细弹窗）
    SplitAssetLibrary: {
      total: 2,
      split: 2,
      skipped: 0,
      failed: 0,
      items: [
        { relPath: '方案A/城商行方案.pptx', status: 'split', message: '' },
        { relPath: '方案B/获客方案.pptx', status: 'split', message: '' }
      ]
    }
  }

  test.use({ wailsOverrides: assetsOverrides })

  test('库级拆分主链路：入口→落点确认弹窗→mock 拆分完成→汇总提示', async ({ page }) => {
    await page.goto('/')

    // 活动栏第 5 项（nth(4)）切换到素材管理面板
    await page.locator('.activity-bar-item').nth(4).click()
    const panel = page.locator('.assets-panel')
    await expect(panel).toBeVisible()
    await expect(panel.locator('.asset-card')).toHaveCount(2)

    // 库级拆分入口 → 确认弹窗（落点 radio 默认「素材库缓存目录（推荐）」）
    await panel.getByRole('button', { name: '拆分全部 PPT' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '拆分 PPT' })
    await expect(dialog).toBeVisible()
    const radios = dialog.locator('.el-radio')
    await expect(radios).toHaveCount(2)
    await expect(dialog.locator('.el-radio.is-checked')).toContainText('素材库缓存目录')
    // 默认落点为缓存目录说明（非源目录警示）
    await expect(dialog.locator('.split-cache-hint')).toBeVisible()
    await expect(dialog.locator('.split-source-warning')).toHaveCount(0)

    // 切「源文件所在目录」：自动排除索引警示出现
    await radios.filter({ hasText: '源文件所在目录' }).click()
    await expect(dialog.locator('.split-source-warning')).toContainText('已自动排除索引')
    await expect(dialog.locator('.split-cache-hint')).toHaveCount(0)

    // 切回默认落点发起拆分（确认按钮进行中态防重复提交）
    await radios.filter({ hasText: '素材库缓存目录' }).click()
    await dialog.getByRole('button', { name: '开始拆分' }).click()

    // mock 拆分完成：Promise 分支汇总提示弹出 + 确认弹窗关闭（EP el-dialog 关闭后
    // DOM 保留仅隐藏，断言不可见而非 count 0）
    await expect(page.locator('.el-message').filter({ hasText: '拆分完成' })).toBeVisible()
    await expect(dialog).not.toBeVisible()

    // 调用参数断言：库级批量 + 缓存目录落点（outputToSourceDir=false）
    const calls = await getWailsCalls(page, 'SplitAssetLibrary')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e', false])
  })
})
