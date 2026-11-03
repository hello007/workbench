/**
 * 素材管理 PPT 拆分 E2E（SRS §2.2.3 库级批量拆分主链路，mock 驱动）。
 *
 * 覆盖：素材面板入口（活动栏）→ 库级「拆分全部 PPT」→ 落点确认弹窗（radio 默认
 * 就地替换、切源目录出现自动排除索引警示）→ 开始拆分 → mock 拆分完成 →
 * 汇总提示 + SplitAssetLibrary 调用参数断言（getWailsCalls）。
 * 默认落点为就地替换（删源须强确认），主链路显式切「素材库缓存目录」以维持
 * cache 落点覆盖。
 * mock 数据源为 wails-mock-defaults.js E2E 合并表 + 本文件 overrides（形状对齐
 * model.AssetLibrary / AssetFileEntry / AssetSplitBatchResult）。
 */
import { test, expect, getWailsCalls, openPanel } from './fixtures'

// 素材面板基础数据：单库 + 两个 ok 状态 pptx 文件（画廊可渲染、拆分入口可用）；
// AssetThumbsAvailable=false 走降级路径（E2E 环境无 PowerPoint，缩略图面不参与）。
// 各 describe 用 test.use 注入（须在 describe 层声明，test 函数体内调用非法）。
const baseAssetsOverrides = {
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
    canceled: false,
    items: [
      { relPath: '方案A/城商行方案.pptx', status: 'split', message: '', target: 'cache', sourceRemoved: false, rolledBack: false, code: '' },
      { relPath: '方案B/获客方案.pptx', status: 'split', message: '', target: 'cache', sourceRemoved: false, rolledBack: false, code: '' }
    ]
  }
}

test.describe('素材管理 PPT 拆分', () => {
  test.use({ wailsOverrides: baseAssetsOverrides })

  test('库级拆分主链路：入口→落点确认弹窗→mock 拆分完成→汇总提示', async ({ page }) => {
    await page.goto('/')

    // data-panel-id="assets" 语义定位素材管理面板
    await openPanel(page, 'assets')
    const panel = page.locator('.assets-panel')
    await expect(panel).toBeVisible()
    await expect(panel.locator('.asset-card')).toHaveCount(2)

    // 库级拆分入口 → 确认弹窗（落点 radio 默认「就地替换（删除源文件，推荐）」，
    // 删源不可逆须勾选强确认后才可提交）
    await panel.getByRole('button', { name: '拆分全部 PPT' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '拆分 PPT' })
    await expect(dialog).toBeVisible()
    const radios = dialog.locator('.el-radio')
    await expect(radios).toHaveCount(3)
    await expect(dialog.locator('.el-radio.is-checked')).toContainText('就地替换（删除源文件，推荐）')
    // 默认落点为就地替换：删源警示可见，缓存说明不渲染
    await expect(dialog.locator('.split-replace-warning')).toBeVisible()
    await expect(dialog.locator('.split-cache-hint')).toHaveCount(0)

    // 切「源文件所在目录」：自动排除索引警示出现，就地替换警示隐藏
    await radios.filter({ hasText: '源文件所在目录' }).click()
    await expect(dialog.locator('.split-source-warning')).toContainText('已自动排除索引')
    await expect(dialog.locator('.split-cache-hint')).toHaveCount(0)
    await expect(dialog.locator('.split-replace-warning')).toHaveCount(0)

    // 显式切「素材库缓存目录」发起拆分（默认落点已非 cache，该分支不再被隐式
    // 走到）：缓存说明可见、就地替换警示隐藏（确认按钮进行中态防重复提交）
    await radios.filter({ hasText: '素材库缓存目录' }).click()
    await expect(dialog.locator('.split-cache-hint')).toBeVisible()
    await expect(dialog.locator('.split-replace-warning')).toHaveCount(0)
    await dialog.getByRole('button', { name: '开始拆分' }).click()

    // mock 拆分完成：Promise 分支汇总提示弹出 + 确认弹窗关闭（EP el-dialog 关闭后
    // DOM 保留仅隐藏，断言不可见而非 count 0）
    await expect(page.locator('.el-message').filter({ hasText: '拆分完成' })).toBeVisible()
    await expect(dialog).not.toBeVisible()

    // 调用参数断言：库级批量 + 缓存目录落点（target='cache'，B 方案 PR1 起落点
    // 由 boolean 改为字符串三态 cache/source/replace）
    const calls = await getWailsCalls(page, 'SplitAssetLibrary')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e', 'cache'])
  })
})

test.describe('素材管理 PPT 拆分·就地替换', () => {
  // replace 专用 mock：items 必须回显 target=replace / sourceRemoved=true，否则
  // 「已删除源」汇总与刷新链分支不会被执行（cache 形状会对任何成功路径都成立）
  const replaceOverrides = {
    ...baseAssetsOverrides,
    SplitAssetLibrary: {
      total: 2,
      split: 2,
      skipped: 0,
      failed: 0,
      canceled: false,
      items: [
        { relPath: '方案A/城商行方案.pptx', status: 'split', message: '', target: 'replace', sourceRemoved: true, rolledBack: false, code: '' },
        { relPath: '方案B/获客方案.pptx', status: 'split', message: '', target: 'replace', sourceRemoved: true, rolledBack: false, code: '' }
      ]
    }
  }
  test.use({ wailsOverrides: replaceOverrides })

  test('就地替换落点：不可逆强确认未勾选时提交禁用，勾选后以 target=replace 调用', async ({ page }) => {
    await page.goto('/')

    await openPanel(page, 'assets')
    const panel = page.locator('.assets-panel')
    await expect(panel).toBeVisible()

    await panel.getByRole('button', { name: '拆分全部 PPT' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '拆分 PPT' })
    await expect(dialog).toBeVisible()

    // 切「就地替换」：删源警示 + 强确认勾选出现，另两落点提示隐藏
    await dialog.locator('.el-radio').filter({ hasText: '就地替换' }).click()
    const warning = dialog.locator('.split-replace-warning')
    await expect(warning).toContainText('原始文件将被移入系统回收站')
    await expect(warning).toContainText('不可撤销')
    // 批量入口目录回显为通用描述 + 禁用命名提示（非 ElMessageBox，无需 first/last）
    await expect(warning).toContainText('各源文件同名的独立目录')
    await expect(warning).toContainText('源名主干')
    await expect(dialog.locator('.split-cache-hint')).toHaveCount(0)

    // 未勾选强确认：提交按钮禁用
    const submit = dialog.getByRole('button', { name: '开始拆分' })
    await expect(submit).toBeDisabled()
    await dialog.locator('.el-checkbox').click()
    await expect(submit).toBeEnabled()

    const listCallsBefore = (await getWailsCalls(page, 'ListAssetFiles')).length
    await submit.click()

    // 汇总按 replace 三类收尾口径（已删除源 2），并触发清单重拉（异步刷新链，
    // 用 expect.poll 轮询而非同步断言，避免调用计数竞态）
    await expect(page.locator('.el-message').filter({ hasText: '已删除源 2' })).toBeVisible()
    await expect(dialog).not.toBeVisible()
    await expect.poll(async () => (await getWailsCalls(page, 'ListAssetFiles')).length)
      .toBeGreaterThan(listCallsBefore)

    const calls = await getWailsCalls(page, 'SplitAssetLibrary')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e', 'replace'])
  })

  test('勾选强确认后取消不发起拆分；重开弹窗回落就地替换默认且未勾选', async ({ page }) => {
    await page.goto('/')

    await openPanel(page, 'assets')
    const panel = page.locator('.assets-panel')
    await expect(panel).toBeVisible()

    await panel.getByRole('button', { name: '拆分全部 PPT' }).click()
    const dialog = page.locator('.el-dialog').filter({ hasText: '拆分 PPT' })
    await expect(dialog).toBeVisible()

    // 首次打开默认就地替换落点：删源警示可见、强确认未勾选时提交禁用；
    // 勾选强确认后提交按钮可用
    await expect(dialog.locator('.el-radio.is-checked')).toContainText('就地替换')
    await expect(dialog.locator('.split-replace-warning')).toBeVisible()
    const submit = dialog.getByRole('button', { name: '开始拆分' })
    await expect(submit).toBeDisabled()
    await dialog.locator('.el-checkbox').click()
    await expect(submit).toBeEnabled()

    // 取消：关闭弹窗且不发起任何拆分调用
    await dialog.getByRole('button', { name: '取消' }).click()
    await expect(dialog).not.toBeVisible()
    expect(await getWailsCalls(page, 'SplitAssetLibrary')).toHaveLength(0)

    // 重开：回落就地替换默认落点（不沿用上次勾选），强确认未勾选、提交禁用
    await panel.getByRole('button', { name: '拆分全部 PPT' }).click()
    await expect(dialog).toBeVisible()
    await expect(dialog.locator('.el-radio.is-checked')).toContainText('就地替换')
    await expect(dialog.locator('.split-replace-warning')).toBeVisible()
    await expect(dialog.locator('.el-checkbox input')).not.toBeChecked()
    await expect(submit).toBeDisabled()
  })
})
