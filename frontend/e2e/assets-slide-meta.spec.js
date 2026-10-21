/**
 * 幻灯片标注保存双代次守卫 E2E（assets-management.md「前端保存链守卫」，mock 驱动，
 * 矩阵候选 #11）。
 *
 * 覆盖：幻灯片视图点选卡片进详情 → 编辑星级/业务备注 → 保存 → 刷新期间
 * ListAssetSlides 经 __sequence__ 注入「保存后首屏无该条目」的新代次响应（模拟
 * 懒加载深页：刷新复位首页后目标页不在返回页）→ miss 分支以提交值就地回填
 * selected 引用、草稿保留提交值不动 → 断言表单不回退旧值（P1 真实踩坑回归面：
 * 无条件 resetSlideDraft 会从保存前旧快照回读，表单回退旧备注、再点保存即写回
 * 旧数据）+ 清单确已刷新（新代次卡片集渲染）。
 *
 * mock 数据源为 wails-mock-defaults.js E2E 合并表 + 本文件 overrides（形状对齐
 * model.AssetSlideEntry / AssetSlidePage：ownTags/note/rating 三标注字段齐全）。
 * 本 spec 位于既有 e2e/ 通道目录，vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls } from './fixtures'

const libFixture = { id: 'lib-e2e', name: 'E2E 素材库', rootPath: 'D:/e2e-assets', createdAt: '2026-10-21 10:00:00', updatedAt: '2026-10-21 10:00:00' }

const makeSlide = (id, fileId, filePath, slideNum, title, note) => ({
  id,
  fileId,
  filePath,
  slideNum,
  title,
  text: `${title} 正文文本`,
  speakerNotes: '',
  tags: [],
  ownTags: [],
  note,
  rating: 0
})

test.describe('幻灯片标注保存双代次守卫', () => {
  // ListAssetSlides 两帧：首帧（进入幻灯片视图拉首页）含目标条目「获客封面」
  //（id 23 / fileId 12 / 第 1 页，旧备注）；第二帧（保存成功刷新链重拉首页）
  // 目标条目不在返回页（模拟其落在懒加载深页），由新方案页顶替
  const slidesOverrides = {
    ListAssetLibraries: [libFixture],
    GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 3, slideCount: 4, failedCount: 0, encryptedCount: 0, lostCount: 0, excludedCount: 0 },
    ListAssetSlides: {
      __sequence__: [
        // 帧形状对齐 model.AssetSlidePage {items, total}
        {
          items: [
            makeSlide(21, 11, '方案A/城商行方案.pptx', 1, '封面页', ''),
            makeSlide(22, 11, '方案A/城商行方案.pptx', 2, '背景页', ''),
            makeSlide(23, 12, '方案B/获客方案.pptx', 1, '获客封面', '旧备注')
          ],
          total: 4
        },
        {
          items: [
            makeSlide(21, 11, '方案A/城商行方案.pptx', 1, '封面页', ''),
            makeSlide(22, 11, '方案A/城商行方案.pptx', 2, '背景页', ''),
            makeSlide(24, 13, '方案C/新方案.pptx', 1, '新封面', '')
          ],
          total: 4
        }
      ]
    },
    AssetThumbsAvailable: false
  }

  test.use({ wailsOverrides: slidesOverrides })

  test('保存标注后刷新 miss：表单不回退旧值、以提交值回填引用、清单刷新至新代次', async ({ page }) => {
    await page.goto('/')
    await page.locator('.activity-bar-item').nth(4).click()
    await page.mouse.move(400, 400)
    await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
    const panel = page.locator('.assets-panel')
    await expect(panel).toBeVisible()

    // 切幻灯片视图（拉首页 = 序列首帧）
    await panel.locator('.el-radio-button').filter({ hasText: '幻灯片视图' }).click()
    const cards = panel.locator('.asset-card')
    await expect(cards).toHaveCount(3)

    // 点选目标卡进详情：草稿从条目旧标注初始化
    await cards.filter({ hasText: '获客封面' }).click()
    const noteArea = panel.locator('.detail-section').filter({ hasText: '业务备注' }).locator('textarea')
    await expect(noteArea).toHaveValue('旧备注')

    // 编辑标注：4 星 + 新备注（提交值快照的断言依据）
    await panel.locator('.detail-section').filter({ hasText: '星级' }).locator('.el-rate__item').nth(3).click()
    await noteArea.fill('新备注')

    // 保存：void 写入 + 刷新链重拉首页（序列第二帧，目标条目 miss）
    await panel.locator('.detail-actions').getByRole('button', { name: '保存' }).click()
    await expect(page.locator('.el-message').filter({ hasText: '已保存' })).toBeVisible()

    // 保存调用参数断言：库 + (fileId, slideNum) 键（禁 slides.id）+ 提交值整体替换
    const saveCalls = await getWailsCalls(page, 'UpdateAssetSlideMeta')
    expect(saveCalls.length).toBe(1)
    expect(saveCalls[0].args).toEqual(['lib-e2e', 12, 1, { rating: 4, note: '新备注', tagNames: [] }])

    // 双代次守卫 miss 分支：selected 引用按提交值就地回填、草稿保留提交值——
    // 表单不回退旧值「旧备注」（P1 回归面：无条件 resetSlideDraft 会从保存前
    // 旧快照回读，此处若回退即命中回归）
    await expect(noteArea).toHaveValue('新备注')

    // 清单确已刷新至新代次：新方案页卡出现、目标卡随 miss 语义不在返回页
    await expect(cards.filter({ hasText: '新封面' })).toHaveCount(1)
    await expect(cards.filter({ hasText: '获客封面' })).toHaveCount(0)
    await expect(cards).toHaveCount(3)

    // 首页复位重拉恰两轮（进入幻灯片视图 + 保存刷新链），无多余分页请求
    const loadCalls = await getWailsCalls(page, 'ListAssetSlides')
    expect(loadCalls.length).toBe(2)
    expect(loadCalls[1].args).toEqual(['lib-e2e', 50, 0])
  })
})
