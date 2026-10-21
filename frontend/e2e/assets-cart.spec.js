/**
 * 素材购物栏收集-溯源-合并导出 E2E（pptx-merge.md PR3 购物栏链路，mock 驱动）。
 *
 * 覆盖：
 * 1. 页级收集主链：幻灯片视图卡片「加入购物栏」→ AddAssetCartItems 条目快照参数
 *    （键 = libraryId|fileId|slideNum）→ 徽标计数 + 卡片已收集态 → 抽屉条目渲染
 *    （来源行「库名/文件名 · 第 N 页」）
 * 2. 溯源跳转：抽屉条目点击 → 抽屉关闭 + filterByFile「仅看该文件页」过滤语境 +
 *    目标页进详情（miss 提示不出现的反断言）
 * 3. 合并导出主链：stale 条目置灰标记 → SaveFileDialog → MergeAssetCartToPptx 在途
 *    进度（assets-cart:export-progress 事件经 emitWailsEvent 驱动中间态）→ 完成
 *    确认框 SkippedItems 跳过报告 →「保留」不清空购物栏（清空为可选动作）
 *
 * mock 数据源：购物栏 7 绑定缺省已在 wails-mock-defaults.js 基础表；合并导出用
 * __deferred__ 挂起解析器 + emitWailsEvent（进度事件先于 promise resolve，对齐
 * 真实后端合并期间推送事件的时序）。
 */
import { test, expect, getWailsCalls, releaseDeferred, emitWailsEvent } from './fixtures'

const libFixture = { id: 'lib-e2e', name: 'E2E 素材库', rootPath: 'D:/e2e-assets', createdAt: '2026-10-19 10:00:00', updatedAt: '2026-10-19 10:00:00' }

const makeSlide = (id, fileId, filePath, slideNum, title) => ({
  id,
  fileId,
  filePath,
  slideNum,
  title,
  text: `${title} 正文文本`,
  speakerNotes: '',
  tags: [],
  ownTags: [],
  note: '',
  rating: 0
})

const slidesFixture = [
  makeSlide(21, 11, '方案A/城商行方案.pptx', 1, '封面页'),
  makeSlide(22, 11, '方案A/城商行方案.pptx', 2, '背景页'),
  makeSlide(23, 12, '方案B/获客方案.pptx', 1, '获客封面')
]

/** 形状对齐 model.AssetCartItem（快照字段 + stale 探测标志非持久化） */
const cartItem = (over = {}) => ({
  libraryId: 'lib-e2e',
  fileId: 11,
  slideNum: 2,
  srcPath: 'D:/e2e-assets/方案A/城商行方案.pptx',
  fileName: '城商行方案.pptx',
  slideTitle: '背景页',
  libraryName: 'E2E 素材库',
  addedAt: '2026-10-21T09:00:00Z',
  ...over
})

const baseOverrides = {
  ListAssetLibraries: [libFixture],
  GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 2, slideCount: 3, failedCount: 0, encryptedCount: 0, lostCount: 0, excludedCount: 0 },
  ListAssetSlides: { items: slidesFixture, total: 3 },
  AssetThumbsAvailable: false
}

/** 打开素材管理面板并切到幻灯片视图（卡片含「加入购物栏」悬浮按钮的唯一视图） */
async function openSlideView(page) {
  await page.goto('/')
  // 活动栏第 5 项（nth(4)）切换到素材管理面板；鼠标移离防 tooltip 遮挡画廊
  //（时序坑先例见 assets-batch-tag.spec）
  await page.locator('.activity-bar-item').nth(4).click()
  await page.mouse.move(400, 400)
  await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
  const panel = page.locator('.assets-panel')
  await expect(panel).toBeVisible()
  await panel.locator('.el-radio-button').filter({ hasText: '幻灯片视图' }).click()
  const cards = panel.locator('.asset-card')
  await expect(cards).toHaveCount(3)
  return { panel, cards }
}

test.describe('素材购物栏：页级收集与抽屉渲染', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      // 挂载拉取空栏 → 加页后刷新回读 1 条（__sequence__ 两帧驱动徽标与抽屉数据）
      ListAssetCartItems: { __sequence__: [[], [cartItem()]] },
      AddAssetCartItems: { added: 1, duplicates: 0 }
    }
  })

  test('卡片加页入栏：条目快照参数 + 徽标计数 + 已收集态 + 抽屉来源行', async ({ page }) => {
    const { panel, cards } = await openSlideView(page)

    // 徽标初始隐藏（空购物栏）
    const badge = panel.locator('.cart-entry-badge .el-badge__content')
    await expect(badge).toBeHidden()

    // 第 2 页卡加入购物栏（悬浮按钮 hover 显现，已收集态常显）
    await cards.nth(1).hover()
    await cards.nth(1).locator('.asset-card-cart').click()
    await expect(page.locator('.el-message').filter({ hasText: '已加入购物栏' })).toBeVisible()

    // 调用参数断言：条目快照字段（srcPath = 库根 + 相对路径，展示字段定格）
    const calls = await getWailsCalls(page, 'AddAssetCartItems')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual([
      [
        {
          libraryId: 'lib-e2e',
          fileId: 11,
          slideNum: 2,
          srcPath: 'D:/e2e-assets/方案A/城商行方案.pptx',
          fileName: '城商行方案.pptx',
          slideTitle: '背景页',
          libraryName: 'E2E 素材库'
        }
      ]
    ])

    // 刷新回读：徽标计数 1 + 卡片切已收集态（✓ 常显）
    await expect(badge).toBeVisible()
    await expect(badge).toHaveText('1')
    await expect(cards.nth(1).locator('.asset-card-cart')).toHaveText('✓')

    // 抽屉打开：条目标题与来源行（库名/文件名 · 页码）
    await panel.getByRole('button', { name: '购物栏', exact: true }).click()
    const drawer = page.locator('.cart-drawer')
    await expect(drawer).toBeVisible()
    await expect(drawer.locator('.cart-item')).toHaveCount(1)
    await expect(drawer.locator('.cart-item-title')).toHaveText('背景页')
    await expect(drawer.locator('.cart-item-source')).toContainText('E2E 素材库')
    await expect(drawer.locator('.cart-item-source')).toContainText('城商行方案.pptx · 第 2 页')
  })
})

test.describe('素材购物栏：溯源跳转', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      ListAssetCartItems: [cartItem()]
    }
  })

  test('抽屉条目点击进入仅看该文件页语境并选中目标页进详情', async ({ page }) => {
    const { panel } = await openSlideView(page)

    await panel.getByRole('button', { name: '购物栏', exact: true }).click()
    const drawer = page.locator('.cart-drawer')
    await expect(drawer.locator('.cart-item')).toHaveCount(1)

    // 点击条目体 → 溯源定位（同库直跳，不触发切库）
    await drawer.locator('.cart-item').click()

    // 抽屉关闭（v-if 卸载），进入「仅看该文件页」过滤语境
    await expect(drawer).toHaveCount(0)
    const banner = panel.locator('.file-filter-banner')
    await expect(banner).toBeVisible()
    await expect(banner).toContainText('城商行方案.pptx')
    await expect(banner).toContainText('（2 页）')

    // 目标页进详情侧栏（标题 = 页标题），画廊收敛为该文件 2 页
    await expect(panel.locator('.assets-detail')).toBeVisible()
    await expect(panel.locator('.detail-title')).toHaveText('背景页')
    await expect(panel.locator('.asset-card')).toHaveCount(2)

    // 反断言：命中路径无「暂不可定位」miss 提示
    await expect(page.locator('.el-message').filter({ hasText: '暂不可定位' })).toHaveCount(0)
  })
})

test.describe('素材购物栏：合并导出主链（SkippedItems 报告 + 保留不清空）', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      // 预收集 2 条：1 正常 + 1 stale（源文件已不存在，List 探测置标志）
      ListAssetCartItems: [
        cartItem(),
        cartItem({
          fileId: 12,
          slideNum: 1,
          srcPath: 'D:/e2e-assets/方案B/已删除.pptx',
          fileName: '已删除.pptx',
          slideTitle: '获客封面',
          stale: true
        })
      ],
      // SaveFileDialog 缺省空串 = 用户取消；导出链 override 为确定路径
      SaveFileDialog: 'D:/e2e-export/合并导出.pptx',
      // 挂起解析器：先驱动进度事件中间态，再放行含 SkippedItems 的结果
      MergeAssetCartToPptx: { __deferred__: 'cart-merge' }
    }
  })

  test('导出进度事件驱动中间态，完成确认框含跳过报告，保留不清空购物栏', async ({ page }) => {
    await page.goto('/')
    await page.locator('.activity-bar-item').nth(4).click()
    await page.mouse.move(400, 400)
    await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
    const panel = page.locator('.assets-panel')
    await expect(panel).toBeVisible()

    // 打开抽屉：2 条目，stale 条目置灰 + 标记（导出将跳过）
    await panel.getByRole('button', { name: '购物栏', exact: true }).click()
    const drawer = page.locator('.cart-drawer')
    await expect(drawer).toBeVisible()
    await expect(drawer.locator('.cart-item')).toHaveCount(2)
    const staleItem = drawer.locator('.cart-item').filter({ hasText: '已删除.pptx' })
    await expect(staleItem).toHaveClass(/is-stale/)
    await expect(staleItem.locator('.cart-item-stale-mark')).toContainText('源文件不存在，导出将跳过')

    // 导出（2 页）：SaveFileDialog 一步选位（mock 直接返回路径）
    await drawer.locator('.cart-footer button').filter({ hasText: '导出（2 页）' }).click()
    const dialogCalls = await getWailsCalls(page, 'SaveFileDialog')
    expect(dialogCalls.length).toBe(1)

    // 在途中间态：进度条出现 + 取消入口并列（合并 pending，尚未 resolve）
    await expect(drawer.locator('.cart-export-progress')).toBeVisible()
    await expect(drawer.locator('.cart-export-progress-text')).toContainText('正在准备合并导出')
    await expect(drawer.getByRole('button', { name: '取消导出' })).toBeVisible()

    // 进度事件（真实后端在合并期间推送，emitWailsEvent 同构广播）
    await emitWailsEvent(page, 'assets-cart:export-progress', { outPath: 'D:/e2e-export/合并导出.pptx', done: 1, total: 2 })
    await expect(drawer.locator('.cart-export-progress-text')).toContainText('合并导出中 1/2')

    // 放行结果：stale 条目以 SkippedItems 报告（跳过口径单点在合并核心）
    await releaseDeferred(page, 'cart-merge', {
      outputPath: 'D:/e2e-export/合并导出.pptx',
      pages: [{ srcPath: 'D:/e2e-assets/方案A/城商行方案.pptx', srcPageNum: 2, outPageNum: 1 }],
      pageCount: 1,
      skippedItems: [{ srcPath: 'D:/e2e-assets/方案B/已删除.pptx', pageNum: 1, reason: '源文件不存在' }],
      sldSzMixed: false
    })

    // 完成确认框（单次确认链）：summary 携带跳过报告
    const msgbox = page.locator('.el-message-box')
    await expect(msgbox).toBeVisible()
    await expect(msgbox).toContainText('已导出 1 页（跳过 1 项失效或越界条目')

    // 调用参数：outPath 透传
    expect((await getWailsCalls(page, 'MergeAssetCartToPptx'))[0].args).toEqual(['D:/e2e-export/合并导出.pptx'])

    // 「保留」= 不清空购物栏（清空为可选动作）：确认框关闭、购物栏原样
    await msgbox.getByRole('button', { name: '保留' }).click()
    await expect(msgbox).toBeHidden()
    await expect(drawer.locator('.cart-item')).toHaveCount(2)
    expect((await getWailsCalls(page, 'ClearAssetCart')).length).toBe(0)
    // 导出运行态复位：进度条消失（done 事件仅复位进度不复活进度条）
    await expect(drawer.locator('.cart-export-progress')).toHaveCount(0)
    await emitWailsEvent(page, 'assets-cart:export-done', {})
    await expect(drawer.locator('.cart-export-progress')).toHaveCount(0)
  })
})
