/**
 * 素材保存检索条件 E2E（assets-management.md §8.6，mock 驱动，矩阵候选 #19）。
 *
 * 覆盖（第七批 SRS §2.5 PR3 主链）：
 * 1. 保存（upsert 新建分支）：检索语境（关键词+标签）命名快照 → SaveAssetSavedSearch
 *    参数断言（全维快照：id=0 / libraryId / keyword / tagIds+tagNames 同序成对 /
 *    facet 五字段 / 两开关 / 排序档位）→ 成功 toast + List 重拉；
 * 2. 覆盖（排除自身的更新分支）：本库条目「覆盖」以当前条件整体替换 → 调用携带
 *    原 id + 原名（后端重名校验的排除自身键）；
 * 3. 异库调取降级（D4）：切库后调取异库条目 → tagIds 属保存时所在库失效，按
 *    tagNames 在当前库标签表重映射（miss 丢弃并计数 warning）→ applySearch 同
 *    路径重搜 → SearchAssets 参数断言（重映射后 tagIds + facet 快照经
 *    restoreFacetStateFromSnapshot 反推再 resolve 的止日末毫秒锚点精确还原 +
 *    排序档位）+ 徽标/输入框/排序触发器语境回填断言；
 * 4. 重名拒绝：新存（id=0）同名 → 后端拒绝经 handleError 透传 toast，弹窗保持
 *    打开可改名重试（与覆盖分支的排除自身形成对照）。
 *
 * mock 说明：SaveAssetSavedSearch 的重试链为无状态 reject 描述符（__sequence__
 * 元素不经描述符处理恒 resolve），重名拒绝拆独立用例以 {__error__} override 驱动；
 * ListAssetSavedSearches override 跨库条目 fixture（tagIds/tagNames 同序成对，
 * id 9 = 当前库不存在的幽灵标签）。本 spec 位于既有 e2e/ 通道目录，
 * vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls } from './fixtures'

const MB_BYTES = 1024 * 1024

// 快照区间按 AssetFilterPopover 自定义档换算同式产出（本地零点 + 止日次日零点前
// 1ms 锚点）——调取回填 restoreFacetStateFromSnapshot 反推毫秒归零后经
// buildFacetOptions resolve，精确还原为原值（round-trip 可逆断言依据）
const startOfLocalDay = (ms) => { const d = new Date(ms); d.setHours(0, 0, 0, 0); return d.getTime() }
const startOfNextLocalDay = (ms) => { const d = new Date(ms); d.setDate(d.getDate() + 1); d.setHours(0, 0, 0, 0); return d.getTime() }
const savedMtimeFrom = startOfLocalDay(new Date(2026, 9, 1).getTime()) * 1e6
const savedMtimeTo = (startOfNextLocalDay(new Date(2026, 9, 20).getTime()) - 1) * 1e6

const libA = { id: 'lib-a', name: '方案库A', rootPath: 'D:/e2e-assets-a', createdAt: '2026-10-20 10:00:00', updatedAt: '2026-10-20 10:00:00' }
const libB = { id: 'lib-b', name: '方案库B', rootPath: 'D:/e2e-assets-b', createdAt: '2026-10-20 11:00:00', updatedAt: '2026-10-20 11:00:00' }

// 跨库已存条目（保存于 lib-a）：tagIds [1, 9] 与 tagNames 同序成对——id 9（幽灵标签）
// 在任何库标签表都不存在，调取时按名重映射 miss 丢弃计数
const savedItem = {
  id: 7,
  name: '获客条件',
  libraryId: 'lib-a',
  keyword: '获客',
  tagIds: [1, 9],
  tagNames: ['重点客户', '幽灵标签'],
  ratings: [4],
  mtimeFrom: savedMtimeFrom,
  mtimeTo: savedMtimeTo,
  sizeFrom: MB_BYTES,
  sizeTo: 10 * MB_BYTES - 1,
  flatten: true,
  slideTagOnlyOwn: false,
  sortBy: 'mtime',
  sortDir: 'desc',
  createdAt: '2026-10-21 09:00:00'
}

const baseOverrides = {
  ListAssetLibraries: [libA, libB],
  GetAssetLibraryStats: { libraryId: 'lib-a', fileCount: 1, slideCount: 2, failedCount: 0, encryptedCount: 0, lostCount: 0, excludedCount: 0 },
  ListAssetSavedSearches: [savedItem]
}

/** 打开素材面板并进入检索语境：关键词「获客」+ 标签筛选勾选「重点客户」 */
async function openSearchContext(page) {
  await page.goto('/')
  await page.locator('.activity-bar-item').nth(4).click()
  await page.mouse.move(400, 400)
  await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
  const panel = page.locator('.assets-panel')
  await expect(panel).toBeVisible()
  await panel.locator('.keyword-input input').fill('获客')
  await panel.locator('.keyword-input input').press('Enter')
  await expect.poll(async () => (await getWailsCalls(page, 'SearchAssets')).length).toBeGreaterThanOrEqual(1)
  // 标签筛选：树形下拉勾选首节点（重点客户，check-strictly 逐节点独立勾选）
  await panel.locator('.tag-filter .el-select__wrapper').click()
  const popper = page.locator('.el-tree-select__popper:visible')
  await popper.locator('.el-tree-node__content').first().locator('.el-checkbox').click()
  await expect(panel.locator('.tag-filter .el-select__wrapper')).toContainText('重点客户')
  await page.keyboard.press('Escape')
  return panel
}

test.describe('素材保存检索条件：保存与覆盖与异库调取', () => {
  test.use({ wailsOverrides: baseOverrides })

  test('保存快照参数 + 覆盖携带原 id 排除自身 + 切库调取按名重映射丢弃计数', async ({ page }) => {
    const panel = await openSearchContext(page)

    // ---- 保存：命名快照 upsert 新建分支（id=0）----
    await panel.getByRole('button', { name: '保存搜索' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '保存检索条件' })
    await expect(dialog).toBeVisible()
    await dialog.getByPlaceholder('例如：近期高分方案').fill('获客条件')
    await dialog.locator('.el-dialog__footer').getByRole('button', { name: '保存' }).click()
    await expect(page.locator('.el-message').filter({ hasText: '检索条件已保存' })).toBeVisible()
    await expect(dialog).not.toBeVisible()

    // 快照参数断言：全维字段（tagIds/tagNames 同序成对、facet 零值、排序默认档）
    const saveCalls = await getWailsCalls(page, 'SaveAssetSavedSearch')
    expect(saveCalls.length).toBe(1)
    expect(saveCalls[0].args).toEqual([{
      id: 0,
      name: '获客条件',
      libraryId: 'lib-a',
      keyword: '获客',
      tagIds: [1],
      tagNames: ['重点客户'],
      ratings: [],
      mtimeFrom: 0,
      mtimeTo: 0,
      sizeFrom: 0,
      sizeTo: 0,
      flatten: true, // D8#2：平铺开关默认打开
      slideTagOnlyOwn: false,
      sortBy: 'relevance',
      sortDir: 'desc'
    }])

    // ---- 覆盖：以当前检索条件整体替换本库条目（调用携带原 id = 重名校验排除自身键）----
    await panel.getByRole('button', { name: '已存搜索' }).click()
    const popper = page.locator('.asset-saved-search-popper:visible')
    await expect(popper.locator('.saved-search-group-title').filter({ hasText: '本库条件' })).toBeVisible()
    const item = popper.locator('.saved-search-item').filter({ hasText: '获客条件' })
    await item.getByRole('button', { name: '覆盖' }).click()
    await expect(page.locator('.el-message').filter({ hasText: '已覆盖更新「获客条件」' })).toBeVisible()
    const overwriteCalls = await getWailsCalls(page, 'SaveAssetSavedSearch')
    expect(overwriteCalls.length).toBe(2)
    expect(overwriteCalls[1].args[0]).toMatchObject({ id: 7, name: '获客条件', libraryId: 'lib-a' })

    // ---- 切库：已存条目归入「其他库条件」组（覆盖禁用防条目静默搬家）----
    await panel.locator('.library-select .el-select__wrapper').click()
    await page.locator('.el-select-dropdown__item:visible').filter({ hasText: '方案库B' }).click()
    await expect.poll(async () => {
      const calls = await getWailsCalls(page, 'ListAssetFiles')
      return calls.length > 0 ? calls[calls.length - 1].args[0] : ''
    }).toBe('lib-b')

    await panel.getByRole('button', { name: '已存搜索' }).click()
    const popperB = page.locator('.asset-saved-search-popper:visible')
    await expect(popperB.locator('.saved-search-group-title').filter({ hasText: '其他库条件' })).toBeVisible()
    const itemB = popperB.locator('.saved-search-item').filter({ hasText: '获客条件' })
    await expect(itemB.locator('.saved-search-lib-mark')).toHaveText('方案库A')

    // ---- 异库调取：tagIds 失效按 tagNames 重映射（重点客户→1 命中、幽灵标签 miss
    // 丢弃计数）→ applySearch 同路径重搜 ----
    await itemB.locator('.saved-search-apply').click()
    await expect(page.locator('.el-message').filter({ hasText: '1 个标签在当前库不存在，已忽略' })).toBeVisible()

    // SearchAssets 参数断言：关键词/重映射后 tagIds/facet 快照精确还原（止日末毫秒
    // 锚点 round-trip）/两开关/排序档位/分页首屏
    const searchCalls = await getWailsCalls(page, 'SearchAssets')
    const applyOpts = searchCalls[searchCalls.length - 1].args[2]
    expect(applyOpts).toEqual({
      flatten: true,
      tagIds: [1],
      slideTagOnlyOwn: false,
      ratings: [4],
      mtimeFrom: savedMtimeFrom,
      mtimeTo: savedMtimeTo,
      sizeFrom: MB_BYTES,
      sizeTo: 10 * MB_BYTES - 1,
      sortBy: 'mtime',
      sortDir: 'desc',
      folderPath: '',
      limit: 50,
      offset: 0
    })

    // 语境回填：关键词输入框 / facet 生效维度徽标（星级+时间+大小三维）/ 排序触发器
    await expect(panel.locator('.keyword-input input')).toHaveValue('获客')
    await expect(panel.locator('.facet-filter-badge .el-badge__content')).toHaveText('3')
    await expect(panel.locator('.sort-trigger')).toContainText('修改时间 ↓')
  })
})

test.describe('素材保存检索条件：重名拒绝', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      // 后端重名校验拒绝（upsert 新建分支 id=0 不排除同名条目）
      SaveAssetSavedSearch: { __error__: '已存在同名检索条件「获客条件」' }
    }
  })

  test('新存同名被拒：错误 toast 透传后端文案，弹窗保持打开可改名重试', async ({ page }) => {
    const panel = await openSearchContext(page)

    await panel.getByRole('button', { name: '保存搜索' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '保存检索条件' })
    await expect(dialog).toBeVisible()
    await dialog.getByPlaceholder('例如：近期高分方案').fill('获客条件')
    await dialog.locator('.el-dialog__footer').getByRole('button', { name: '保存' }).click()

    // 拒绝：handleError 前缀 + 后端 message；弹窗不关闭（可改名重试）
    await expect(page.locator('.el-message').filter({ hasText: '保存检索条件失败: 已存在同名检索条件「获客条件」' })).toBeVisible()
    await expect(dialog).toBeVisible()

    // 新建分支调用形态（id=0）：拒绝发生在后端重名校验，前端照常提交全维快照
    const saveCalls = await getWailsCalls(page, 'SaveAssetSavedSearch')
    expect(saveCalls.length).toBe(1)
    expect(saveCalls[0].args[0]).toMatchObject({ id: 0, name: '获客条件', libraryId: 'lib-a' })
  })
})
