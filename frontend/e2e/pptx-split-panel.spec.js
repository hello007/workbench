/**
 * 工具箱 PPT 拆分面板·文件夹批量链 E2E（pptx-split.md PR3 批量编排 + 事件语义，mock 驱动）。
 *
 * 覆盖文件夹批量拆分主链（真实 OOXML 扫描/拆分在后端域，E2E 价值在面板编排与
 * pptx-split:* 事件消费面）：
 * 1. 发起批量：目录输入 → SplitPptxFolder（__deferred__ 挂起，resolve 时机由用例控制）
 *    → 在途态（进度区「正在扫描」、取消按钮出现、模式切换/开始按钮锁定）
 * 2. 进度中间态：pptx-split:batch:progress 事件经 emitWailsEvent 驱动（真实后端在
 *    批量期间推送、Promise 末步 resolve，事件先于结果——__events__ 描述符的
 *    setTimeout 派发表达不了该时序）；载荷 dirPath 不匹配不消费（防其他批量串扰），
 *    匹配后文件级进度渲染 + currentFile 接通页级 progress 事件过滤
 * 3. 在途取消：CancelPptxSplit 请求发出（缺省 false，批量仍挂起在途——取消效果由
 *    后端经 Promise reject 收尾，前端按钮态不本地翻转）
 * 4. 放行汇总：PptxBatchResult（含 1 条 error 明细）→ 汇总行成功/失败计数 +
 *    逐文件明细（成功页数 / 失败原因）+ 部分失败 warning toast + 运行态复位
 *    （终态来自 Promise 返回值非事件，PptSplitPanel 仅在 resolve 后写 batchResult）
 *
 * mock 形状对齐 model.PptxBatchResult / PptxBatchItem / PptxSplitProgress（model/pptx.go）。
 */
import { test, expect, getWailsCalls, releaseDeferred, pendingDeferredCount, emitWailsEvent } from './fixtures'

const BATCH_DIR = 'D:/e2e-demo/ppt-batch'
const CURRENT_FILE = 'D:/e2e-demo/ppt-batch/季度汇报.pptx'

/** 形状对齐 model.PptxBatchResult（items 按扫描字典序，含 1 条失败明细） */
const BATCH_RESULT = {
  dirPath: BATCH_DIR,
  total: 3,
  items: [
    { srcPath: 'D:/e2e-demo/ppt-batch/年度总结.pptx', outputDir: 'D:/e2e-demo/ppt-batch/年度总结', pageCount: 12, error: '' },
    { srcPath: CURRENT_FILE, outputDir: 'D:/e2e-demo/ppt-batch/季度汇报', pageCount: 48, error: '' },
    { srcPath: 'D:/e2e-demo/ppt-batch/加密材料.pptx', outputDir: '', pageCount: 0, error: '演示文稿已加密，无法拆分' }
  ]
}

/** 打开工具箱并切入 PPT 拆分面板（活动栏第 4 项 nth(3)，移开鼠标防 tooltip 遮挡） */
async function openPptSplitPanel(page) {
  await page.goto('/')
  await page.locator('.activity-bar-item').nth(3).click()
  await page.mouse.move(400, 400)
  const toolbox = page.locator('.toolbox-panel')
  await expect(toolbox).toBeVisible()
  await toolbox.locator('.toolbox-nav-item').filter({ hasText: 'PPT 拆分' }).click()
  const panel = page.locator('.ps-panel')
  await expect(panel).toBeVisible()
  return panel
}

test.describe('工具箱：PPT 拆分文件夹批量链', () => {
  test.use({
    wailsOverrides: {
      // 挂起解析器：先断言在途/进度事件中间态，再放行批量汇总
      SplitPptxFolder: { __deferred__: 'pptx-batch' }
    }
  })

  test('进度事件中间态 → 在途取消请求 → 放行汇总渲染成功/失败明细', async ({ page }) => {
    const panel = await openPptSplitPanel(page)

    // 切到文件夹批量模式并发起批量
    await panel.locator('.el-radio-button').filter({ hasText: '文件夹批量拆分' }).click()
    await panel.getByPlaceholder('输入或浏览选择包含 .pptx 的文件夹路径').fill(BATCH_DIR)
    await panel.getByRole('button', { name: '开始批量拆分' }).click()

    // 在途中间态：进度区「正在扫描」+ 取消按钮出现 + 模式切换锁定 + 挂起恰 1 次
    await expect(panel.locator('.ps-batch-progress-file')).toHaveText('正在扫描待拆分文件…')
    await expect(panel.getByRole('button', { name: '取消批量拆分' })).toBeVisible()
    await expect(panel.getByRole('radio', { name: '单文件拆分' })).toBeDisabled()
    await expect(panel.getByRole('button', { name: '开始批量拆分' })).toBeDisabled()
    expect(await pendingDeferredCount(page, 'pptx-batch')).toBe(1)
    expect((await getWailsCalls(page, 'SplitPptxFolder'))[0].args).toEqual([BATCH_DIR])

    // 进度事件 dirPath 过滤：不匹配载荷不消费（防其他入口的批量事件串入本面板）
    await emitWailsEvent(page, 'pptx-split:batch:progress', {
      dirPath: 'D:/other-dir', index: 1, total: 3, currentFile: 'D:/other-dir/其他.pptx'
    })
    await expect(panel.locator('.ps-batch-progress-file')).toHaveText('正在扫描待拆分文件…')

    // 匹配载荷：文件级进度渲染（事件先于 Promise resolve 的真实时序）
    await emitWailsEvent(page, 'pptx-split:batch:progress', {
      dirPath: BATCH_DIR, index: 2, total: 3, currentFile: CURRENT_FILE
    })
    await expect(panel.locator('.ps-batch-progress-file')).toHaveText(
      '正在拆分第 2 / 3 个文件：' + CURRENT_FILE
    )

    // 页级进度：批量循环内单文件的 pptx-split:progress 按 currentFile 过滤写入
    await emitWailsEvent(page, 'pptx-split:progress', { srcPath: CURRENT_FILE, done: 3, total: 48 })
    await expect(panel.locator('.ps-batch-progress-page')).toHaveText('当前文件页进度：3 / 48 页')

    // 在途取消：请求发出即返回（缺省 false），批量仍挂起在途、按钮态不本地翻转
    await panel.getByRole('button', { name: '取消批量拆分' }).click()
    await expect.poll(async () => (await getWailsCalls(page, 'CancelPptxSplit')).length).toBe(1)
    expect((await getWailsCalls(page, 'CancelPptxSplit'))[0].args).toEqual([])
    await expect(panel.getByRole('button', { name: '取消批量拆分' })).toBeVisible()

    // 放行汇总（PptxBatchResult 含 1 条 error 明细）：终态渲染来自 Promise 返回值
    await releaseDeferred(page, 'pptx-batch', BATCH_RESULT)

    // 汇总行：处理数/总数 + 成功失败计数（混合态 warning 语义色由 partial 类承担）
    await expect(panel.locator('.ps-result-summary')).toHaveText(
      '共 3 / 3 个文件处理完成：成功 2 个、失败 1 个'
    )

    // 逐文件明细：成功项页数 + 失败项原因
    const batchItems = panel.locator('.ps-batch-item')
    await expect(batchItems).toHaveCount(3)
    const okItem = batchItems.filter({ hasText: '季度汇报.pptx' })
    await expect(okItem.locator('.el-tag')).toHaveText('成功')
    await expect(okItem.locator('.ps-batch-item-pages')).toHaveText('48 页')
    await expect(okItem.locator('.ps-batch-item-path')).toHaveText('输出目录：D:/e2e-demo/ppt-batch/季度汇报')
    const failedItem = batchItems.filter({ hasText: '加密材料.pptx' })
    await expect(failedItem.locator('.el-tag')).toHaveText('失败')
    await expect(failedItem.locator('.ps-batch-item-error')).toHaveText('失败原因：演示文稿已加密，无法拆分')

    // 部分失败 warning toast + 运行态复位（进度区消失、取消按钮消失、开始按钮恢复）
    await expect(
      page.locator('.el-message').filter({ hasText: '批量拆分完成：2 个成功、1 个失败（详见列表）' })
    ).toBeVisible()
    await expect(panel.locator('.ps-progress')).toHaveCount(0)
    await expect(panel.getByRole('button', { name: '取消批量拆分' })).toHaveCount(0)
    await expect(panel.getByRole('button', { name: '开始批量拆分' })).toBeEnabled()
  })
})
