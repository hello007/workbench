/**
 * 素材管理批量打标签 E2E（第六批 SRS §2.4 主链路，mock 驱动）。
 *
 * 覆盖：素材面板入口（活动栏）→ 文件视图勾选 2 项 → 浮动操作条「批量打标签」→
 * 弹窗内标签树下拉选标签（添加区）→ 提交 → mock 批量成功 → 成功 toast + 弹窗
 * 关闭 + completed 刷新链（ListAssetFiles 序列第二帧带标签，条目卡片渲染标签）
 * + 选中保留（PRD D5）+ BatchUpdateAssetFileTags 调用参数断言（getWailsCalls）。
 * mock 数据源为 wails-mock-defaults.js E2E 合并表 + 本文件 overrides（形状对齐
 * model.AssetLibrary / AssetFileEntry / AssetBatchTagResult / AssetTagListItem）。
 * 本 spec 位于既有 e2e/ 通道目录，vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls } from './fixtures'

test.describe('素材管理批量打标签', () => {
  // 素材面板基础数据：单库 + 两个 ok 状态 pptx 文件；ListAssetFiles 用 __sequence__
  // 描述符两帧驱动——首帧（挂载加载）无标签、第二帧（批量 completed 刷新链重拉）
  // 带「重点客户」标签，验证「标签出现在条目上」来自刷新链而非初始数据；
  // AssetThumbsAvailable=false 走降级路径（E2E 环境无 PowerPoint，缩略图面不参与）
  const libFixture = { id: 'lib-e2e', name: 'E2E 素材库', rootPath: 'D:/e2e-assets', createdAt: '2026-10-15 10:00:00', updatedAt: '2026-10-15 10:00:00' }
  const keyTag = { id: 1, name: '重点客户', color: '#2563eb', parentId: 0 }
  const makeFile = (id, title, tags) => ({
    id,
    path: `方案${id === 11 ? 'A' : 'B'}/${title}.pptx`,
    size: 1024 * 1024,
    modTime: '2026-10-14 10:00:00',
    status: 'ok',
    title,
    note: '',
    rating: 0,
    tags,
    createdAt: '2026-10-14 10:00:00',
    updatedAt: '2026-10-14 10:00:00'
  })
  const assetsOverrides = {
    ListAssetLibraries: [libFixture],
    GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 2, slideCount: 4, failedCount: 0, encryptedCount: 0 },
    ListAssetFiles: {
      __sequence__: [
        [makeFile(11, '城商行方案', []), makeFile(12, '获客方案', [])],
        [makeFile(11, '城商行方案', [keyTag]), makeFile(12, '获客方案', [keyTag])]
      ]
    },
    AssetThumbsAvailable: false,
    // 形状对齐 model.AssetBatchTagResult：两文件全部写入成功（无失败明细视图）
    BatchUpdateAssetFileTags: {
      total: 2,
      succeeded: 2,
      failed: 0,
      items: [
        { fileId: 11, status: 'ok', message: '' },
        { fileId: 12, status: 'ok', message: '' }
      ]
    }
  }

  test.use({ wailsOverrides: assetsOverrides })

  test('批量打标签主链路：勾选 2 项→弹窗选标签→提交成功→条目渲染标签且选中保留', async ({ page }) => {
    await page.goto('/')

    // 活动栏第 5 项（nth(4)）切换到素材管理面板
    await page.locator('.activity-bar-item').nth(4).click()
    // 切面板后鼠标移离活动栏：el-tooltip（show-after 300ms）因点击后鼠标驻留会
    // 弹出「素材管理」提示并 teleport 到 body——其位置恰覆盖画廊首卡复选框，
    // 而 Playwright 在移动鼠标前做遮挡检查、tooltip 要等鼠标移开才隐藏，双方互
    // 等死锁（单独跑时序快侥幸不触发，全量并发下必挂）。先移鼠标并断言 tooltip
    // 收起（自动重试吸收淡出过渡）再点画廊
    await page.mouse.move(400, 400)
    await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
    const panel = page.locator('.assets-panel')
    await expect(panel).toBeVisible()
    const cards = panel.locator('.asset-card')
    await expect(cards).toHaveCount(2)
    // 初始无标签（ListAssetFiles 序列首帧），验证后续标签确来自批量完成刷新链
    await expect(panel.locator('.card-tag')).toHaveCount(0)

    // 勾选两张文件卡：浮动操作条浮出（第六批 PR3 批量入口挂此条）
    await cards.nth(0).locator('.asset-card-check').click()
    await cards.nth(1).locator('.asset-card-check').click()
    const bar = panel.locator('.asset-selection-bar')
    await expect(bar).toBeVisible()
    await expect(bar.locator('.selection-count')).toHaveText('已选 2 项')

    // 打开批量打标签弹窗
    await bar.getByRole('button', { name: '批量打标签' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '批量打标签' })
    await expect(dialog).toBeVisible()
    await expect(dialog.locator('.batch-tag-section').filter({ hasText: '添加标签' })).toBeVisible()
    await expect(dialog.locator('.batch-tag-section').filter({ hasText: '移除标签' })).toBeVisible()
    // 两区全空提交禁用（PRD D4 前端防线）
    await expect(dialog.getByRole('button', { name: '提交' })).toBeDisabled()

    // 添加区选标签：展开树形下拉（ListAssetTags mock 三标签），勾选第一个节点
    //（重点客户；check-strictly 逐节点独立勾选，绑定不级联）
    const addSection = dialog.locator('.batch-tag-section').filter({ hasText: '添加标签' })
    await addSection.locator('.el-select__wrapper').click()
    const popper = page.locator('.el-tree-select__popper:visible')
    // 顶层 2 个根节点（子节点嵌套渲染在父节点内，.el-tree-node 总数含子级为 3）
    await expect(popper.locator('.el-tree > .el-tree-node')).toHaveCount(2)
    await popper.locator('.el-tree-node__content').first().locator('.el-checkbox').click()
    // 勾选回显：下拉收起前 select 选择区已显示标签名
    await expect(addSection.locator('.el-select__wrapper')).toContainText('重点客户')
    await addSection.locator('.el-select__wrapper').click() // 收起下拉防遮挡页脚

    // 提交：mock 全部成功 → toast + 弹窗关闭（EP 弹窗关闭后 DOM 保留仅隐藏）
    await dialog.getByRole('button', { name: '提交' }).click()
    await expect(page.locator('.el-message').filter({ hasText: '已为 2 项更新标签' })).toBeVisible()
    await expect(dialog).not.toBeVisible()

    // completed 刷新链重拉底册（序列第二帧带标签）：条目卡片渲染「重点客户」标签
    await expect(panel.locator('.card-tag').filter({ hasText: '重点客户' })).toHaveCount(2)
    // 选中保留（PRD D5：数据变更点位非语境变更），浮动条照常可见
    await expect(bar).toBeVisible()
    await expect(bar.locator('.selection-count')).toHaveText('已选 2 项')

    // 调用参数断言：文件模式分派 BatchUpdateAssetFileTags（fileIDs 数组 + op 标签名数组）
    const calls = await getWailsCalls(page, 'BatchUpdateAssetFileTags')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e', [11, 12], { addTags: ['重点客户'], removeTags: [] }])
  })
})
