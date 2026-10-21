/**
 * 素材检索 facet 多维筛选与排序档位 E2E（第七批 SRS §2.5 + 第八批 SRS §2.5.3，
 * mock 驱动，矩阵候选 #12/#14）。
 *
 * 覆盖（参数断言面，getWailsCalls 消费 SearchAssets opts）：
 * 1. facet 换算：勾评分档（4 星+5 星）+ 相对时间档（近 7 天）+ 大小区间（1-10MB）
 *    → SearchAssets 收 ratings/mtimeFrom/mtimeTo/sizeFrom/sizeTo 精确值；期望值按
 *    AssetFilterPopover 模块导出换算逻辑预计算——相对档上界随检索时刻前进（容差
 *    断言防时钟漂移），自定义区间断言「止日末毫秒」可逆锚点精确值
 *    （上界 = 止日次日零点前 1ms，与组件 resolveMtimeRangeNs 同式复算）；
 * 2. 排序档位透传：工具行下拉切「修改时间」→ applySearch 同路径 → SearchAssets
 *    收 {sortBy:'mtime', sortDir:'desc'}，触发按钮标注降序方向。
 *
 * 弹层韧性交互：EP popper 在多 worker 负载下偶发「弹层可解析但内容不可见」的
 * 中间态（重试点击 30s 不恢复），统一以「Escape 收拢全部弹层 → 重开筛选弹层 →
 * 短超时操作」为原子段经 toPass 整段重试——facet 档位快照宿主持有，重开不丢
 * 状态；星级勾选带 is-selected 幂等守卫（重试不反选）。断言统一轮询「末次
 * SearchAssets 调用状态」（幂等交互产生的重复调用不破坏状态收敛判定），替代
 * 按调用序号的绝对下标断言。
 * mock 数据源为 wails-mock-defaults.js E2E 合并表（SearchAssets 缺省空结果页，
 * facet/排序字段经调用参数断言消费不校验值）+ 本文件零 override。
 * 本 spec 位于既有 e2e/ 通道目录，vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls } from './fixtures'

// 大小档换算常量（对齐 AssetFilterPopover resolveSizeRangeBytes 的 MB_BYTES）
const MB_BYTES = 1024 * 1024

// 自定义时间区间期望值：与 AssetFilterPopover resolveMtimeRangeNs 的 custom 分支
// 同式复算（本地零点口径 setDate+setHours，禁 +24h 手算）；上界 = 止日次日零点前
// 1ms ——「止日末毫秒」可逆锚点（纳秒量级超 2^53 后 1ns 尾差经双精度舍入丢失，
// 毫秒尾差可精确还原，spec 见 docs/spec/assets-management.md §8.6）
const customRangeExpect = (() => {
  const startOfLocalDay = (ms) => { const d = new Date(ms); d.setHours(0, 0, 0, 0); return d.getTime() }
  const startOfNextLocalDay = (ms) => { const d = new Date(ms); d.setDate(d.getDate() + 1); d.setHours(0, 0, 0, 0); return d.getTime() }
  return {
    mtimeFrom: startOfLocalDay(new Date(2026, 9, 1).getTime()) * 1e6,
    mtimeTo: (startOfNextLocalDay(new Date(2026, 9, 20).getTime()) - 1) * 1e6
  }
})()

const libFixture = { id: 'lib-e2e', name: 'E2E 素材库', rootPath: 'D:/e2e-assets', createdAt: '2026-10-20 10:00:00', updatedAt: '2026-10-20 10:00:00' }

const baseOverrides = {
  ListAssetLibraries: [libFixture],
  GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 2, slideCount: 3, failedCount: 0, encryptedCount: 0, lostCount: 0, excludedCount: 0 }
}

/** 打开素材面板并进入检索模式（关键词触发 runSearch，后续 facet/排序在检索语境变更） */
async function openSearchMode(page) {
  await page.goto('/')
  await page.locator('.activity-bar-item').nth(4).click()
  await page.mouse.move(400, 400)
  await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
  const panel = page.locator('.assets-panel')
  await expect(panel).toBeVisible()
  await panel.locator('.keyword-input input').fill('城商行')
  await panel.locator('.keyword-input input').press('Enter')
  await expect.poll(async () => (await getWailsCalls(page, 'SearchAssets')).length).toBeGreaterThanOrEqual(1)
  return panel
}

/**
 * 轮询「末次 SearchAssets 调用」直至 opts 满足 predicate（幂等重试产生的重复
 * 调用不破坏收敛判定——状态最终恒达期望档）。
 */
async function waitForSearchState(page, predicate, description) {
  await expect
    .poll(async () => {
      const calls = await getWailsCalls(page, 'SearchAssets')
      return predicate(calls[calls.length - 1].args[2])
    }, { message: `SearchAssets 末次调用未达期望状态: ${description}` })
    .toBe(true)
}

test.describe('素材检索 facet 筛选参数换算', () => {
  test.use({ wailsOverrides: baseOverrides })

  test('评分档+相对时间档+大小区间换算入 SearchAssets，自定义区间取止日末毫秒锚点', async ({ page }) => {
    test.setTimeout(120_000)
    const panel = await openSearchMode(page)

    // 首次检索（无 facet）：五字段零值/空数组 = 不筛选（首调用无重试面，按下标断言）
    expect((await getWailsCalls(page, 'SearchAssets'))[0].args[2])
      .toMatchObject({ ratings: [], mtimeFrom: 0, mtimeTo: 0, sizeFrom: 0, sizeTo: 0 })

    // 筛选弹层韧性交互原子段（EP popper 在多 worker 负载下的三种病态：打开缓慢、
    // 关闭动画期可解析、内容不可见幽灵态）：
    // 1. Escape 收拢全部弹层，并确认真闭合（闭合等待超时容忍幽灵态残留继续）；
    // 2. 仅在确认不可见后才点「筛选」——弹层尚可见时点击会把它关掉（开关抖动
    //    livelock 是此前 20s 耗尽的根因）；
    // 3. 以 8s 窗口等弹层出现（负载下打开可显著慢于常态）；
    // 4. 整段 toPass 重试，操作函数自身需幂等（星级 is-selected 守卫/fill 覆盖写）。
    const withFilterPopper = async (fn) => {
      await expect(async () => {
        await page.keyboard.press('Escape')
        const popper = page.locator('.asset-filter-popper')
        try {
          await expect(popper).toBeHidden({ timeout: 2_000 })
        } catch {
          // 幽灵态容忍：闭合确认超时继续，由下方可见性判定与操作重试兜底
        }
        if (!(await popper.isVisible())) {
          await panel.getByRole('button', { name: '筛选', exact: true }).click()
        }
        await expect(popper).toBeVisible({ timeout: 8_000 })
        await fn(popper)
      }).toPass({ timeout: 30_000 })
    }

    // 维度一：星级多选 4 星 + 5 星（is-selected 幂等守卫：toPass 重试不反选；
    // multiple 下拉保持展开，收口交由下一段的 Escape）
    await withFilterPopper(async (popper) => {
      await popper.locator('.facet-rating-select .el-select__wrapper').click()
      for (const label of ['4 星', '5 星']) {
        const option = page.locator('.el-select-dropdown__item:visible').filter({ hasText: label })
        const selected = await option.evaluate((el) => el.classList.contains('is-selected'))
        if (!selected) await option.click()
      }
    })
    await waitForSearchState(
      page,
      (opts) => JSON.stringify(opts.ratings) === '[4,5]' && opts.sizeFrom === 0 && opts.mtimeFrom === 0,
      '评分档 [4,5]'
    )

    // 维度三：文件大小 1-10MB 档（闭区间 [1MB, 10MB-1]，相邻档无缝衔接口径）
    await withFilterPopper(async (popper) => {
      await popper.locator('.facet-field').filter({ hasText: '文件大小' }).locator('.el-radio').filter({ hasText: '1-10MB' }).click()
    })
    await waitForSearchState(
      page,
      (opts) => JSON.stringify(opts.ratings) === '[4,5]' && opts.sizeFrom === MB_BYTES && opts.sizeTo === 10 * MB_BYTES - 1,
      '大小档 1-10MB'
    )

    // 维度二：相对档「近 7 天」→ [now-7d, now] 纳秒窗口。上界随检索时刻前进，
    // 断言取页面当下时钟 10s 容差（禁精确相等——page 与 runner 时钟存在调度延迟）
    await withFilterPopper(async (popper) => {
      await popper.locator('.facet-field').filter({ hasText: '修改时间' }).locator('.el-radio').filter({ hasText: '近 7 天' }).click()
    })
    await waitForSearchState(page, (opts) => opts.mtimeFrom > 0 && opts.mtimeTo > opts.mtimeFrom, '相对档近 7 天')
    const after7d = (await getWailsCalls(page, 'SearchAssets')).slice(-1)[0].args[2]
    const nowMs = await page.evaluate(() => Date.now())
    const dayMs = 24 * 60 * 60 * 1000
    expect(Math.abs(Math.round(after7d.mtimeTo / 1e6) - nowMs)).toBeLessThan(10_000)
    expect(Math.abs(Math.round(after7d.mtimeFrom / 1e6) - (nowMs - 7 * dayMs))).toBeLessThan(10_000)

    // 维度二切自定义档并键入区间 → 止日末毫秒锚点精确断言（换算同式预计算）。
    // 直接 fill 起止输入框（focus 即开选择面板；面板开启态输入框被遮挡判定
    // 不可见，禁先 click 编辑框再 fill）；整段含 Enter 确认幂等可重试
    await withFilterPopper(async (popper) => {
      await popper.locator('.facet-field').filter({ hasText: '修改时间' }).locator('.el-radio').filter({ hasText: '自定义' }).click()
      const rangeEditor = popper.locator('.facet-date-range')
      await expect(rangeEditor).toBeVisible()
      await rangeEditor.locator('.el-range-input').nth(0).fill('2026-10-01')
      await rangeEditor.locator('.el-range-input').nth(1).fill('2026-10-20')
      await page.keyboard.press('Enter')
    })
    await waitForSearchState(
      page,
      (opts) => opts.mtimeFrom === customRangeExpect.mtimeFrom && opts.mtimeTo === customRangeExpect.mtimeTo,
      '自定义区间止日末毫秒锚点'
    )
    const finalOpts = (await getWailsCalls(page, 'SearchAssets')).slice(-1)[0].args[2]
    expect(finalOpts).toMatchObject({ ratings: [4, 5], sizeFrom: MB_BYTES, sizeTo: 10 * MB_BYTES - 1 })

    // facet 徽标常显生效维度数（星级/时间/大小三维，countActiveFacets 口径）
    await expect(panel.locator('.facet-filter-badge .el-badge__content')).toHaveText('3')
  })
})

test.describe('素材检索排序档位透传', () => {
  test.use({ wailsOverrides: baseOverrides })

  test('下拉切修改时间降序 → SearchAssets 收 sortBy=mtime sortDir=desc', async ({ page }) => {
    const panel = await openSearchMode(page)

    // 默认相关性档（无方向）
    expect((await getWailsCalls(page, 'SearchAssets'))[0].args[2])
      .toMatchObject({ sortBy: 'relevance', sortDir: 'desc' })

    // 排序下拉切「修改时间」：切档回默认降序，检索条件变更点位走 applySearch 同路径
    await panel.locator('.sort-trigger').click()
    await page.locator('.el-dropdown-menu__item:visible').filter({ hasText: '修改时间' }).click()
    await waitForSearchState(
      page,
      (opts) => opts.sortBy === 'mtime' && opts.sortDir === 'desc',
      '排序档 mtime 降序'
    )

    // 触发按钮标注当前档位与方向（sortTriggerLabel 口径）
    await expect(panel.locator('.sort-trigger')).toContainText('修改时间 ↓')
  })
})
