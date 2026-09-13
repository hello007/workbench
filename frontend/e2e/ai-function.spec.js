/**
 * AI 功能触发流程 E2E（mock 后端事件流，测「异步状态机 UI 链路正确性」）。
 *
 * 真实链路中 RunAiFunction 起 claude CLI 子进程并经 Wails 事件推送
 * queued/started/output/done；E2E 以 {__value__, __events__} 描述符 mock 该
 * 异步事件流（wails-init.js 派发至 EventsOn 注册的回调），覆盖：
 * 1. AI 功能面板渲染：功能卡列表来自 GetAiFunctions mock
 * 2. 运行成功流：RunAiFunction(functionId, params) 参数正确 -> 任务 Tab 状态流转
 *    （排队中 -> 运行中 -> 已完成）-> 流式输出渲染
 * 3. 失败流：done 事件带 error -> 「失败」标签 + 错误信息展示
 * 4. 取消链路：运行中点取消 -> CancelAiTask(taskId) 参数正确
 *
 * 事件 payload 形状对齐 service/ai_function.go emit 数据与 model.AiTaskRunResult
 * （taskId/text/output/exitCode/error/canceled 等字段）。
 */
import { test, expect, getWailsCalls } from './fixtures'

/** AI 功能面板公共前置：打开首页并切到 AI 功能面板，等待功能卡渲染 */
async function openAiPanelPage(page) {
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.goto('/')
  // 活动画板第 2 项为 AI 功能入口（panels 顺序：工作目录/AI 功能/仓库统计/工具箱）
  await page.locator('.activity-bar-item').nth(1).click()
  await expect(page.locator('.ai-function-panel')).toBeVisible()
  // 功能卡渲染（来自 wails-mock-defaults.js 的 GetAiFunctions 默认值）
  await expect(page.locator('.ai-card', { hasText: '代码评审' })).toBeVisible()
}

test.describe('AI 功能触发流程', () => {
  test.describe('面板渲染', () => {
    test('功能卡列表渲染：名称与描述来自 GetAiFunctions', async ({ page }) => {
      await openAiPanelPage(page)

      const card = page.locator('.ai-card', { hasText: '代码评审' })
      await expect(card).toBeVisible()
      await expect(card.getByText('对当前变更做一次代码评审并输出建议')).toBeVisible()

      // 面板挂载即加载功能列表
      const calls = await getWailsCalls(page, 'GetAiFunctions')
      expect(calls).toEqual([{ method: 'GetAiFunctions', args: [] }])
    })
  })

  test.describe('运行成功流', () => {
    test('运行后任务状态流转：排队/运行 -> 已完成，流式输出渲染', async ({ page }) => {
      await openAiPanelPage(page)

      // 打开功能 Tab 并运行（无参数功能直跑）
      await page.locator('.ai-card', { hasText: '代码评审' }).click()
      await expect(page.locator('.run-btn')).toBeVisible()
      await page.locator('.run-btn').click()

      // RunAiFunction 参数正确（functionId + 空参数对象）
      const runCalls = await getWailsCalls(page, 'RunAiFunction')
      expect(runCalls).toEqual([{ method: 'RunAiFunction', args: ['ai-fmt-review', {}] }])

      // 事件流末端：状态标签「已完成」（queued/started/output/done 经 mock 按序派发）
      const statusTag = page.locator('.ai-function-panel .el-tag', { hasText: '已完成' })
      await expect(statusTag).toBeVisible()

      // 流式输出渲染（ai-task:output 事件 text 追加）
      await expect(page.locator('.task-output pre')).toContainText('发现 1 个问题')
    })
  })

  test.describe('失败流', () => {
    test.use({
      wailsOverrides: {
        RunAiFunction: {
          __value__: 'task-fail-1',
          __events__: [
            { event: 'ai-task:queued', payload: { taskId: 'task-fail-1' } },
            { event: 'ai-task:started', payload: { taskId: 'task-fail-1' } },
            {
              event: 'ai-task:done',
              payload: {
                taskId: 'task-fail-1',
                sessionId: '',
                exitCode: 1,
                error: 'claude CLI not found',
                output: '',
                outputSize: 0,
                outputFile: '',
                canceled: false
              }
            }
          ]
        }
      }
    })

    test('done 事件带 error：状态标签「失败」且错误信息展示', async ({ page }) => {
      await openAiPanelPage(page)

      await page.locator('.ai-card', { hasText: '代码评审' }).click()
      await page.locator('.run-btn').click()

      await expect(
        page.locator('.ai-function-panel .el-tag', { hasText: '失败' })
      ).toBeVisible()
      await expect(page.locator('.error-text')).toContainText('✗ 失败 · claude CLI not found')

      // 失败后不展示「复制输出」等完成动作按钮（completion=none）
      await expect(page.getByRole('button', { name: '取消' })).toHaveCount(0)
    })
  })

  test.describe('取消链路', () => {
    test.use({
      wailsOverrides: {
        RunAiFunction: {
          __value__: 'task-cancel-1',
          __events__: [
            { event: 'ai-task:queued', payload: { taskId: 'task-cancel-1' } },
            { event: 'ai-task:started', payload: { taskId: 'task-cancel-1' } },
            {
              event: 'ai-task:output',
              payload: { taskId: 'task-cancel-1', text: '评审进行中…' }
            },
            // done 延迟派发模拟长任务：用例在延迟窗口内完成取消断言，页面销毁后 setTimeout 丢弃
            {
              event: 'ai-task:done',
              delayMs: 8000,
              payload: { taskId: 'task-cancel-1', exitCode: 0, error: '', canceled: true }
            }
          ]
        }
      }
    })

    test('运行中取消：CancelAiTask 收到 taskId', async ({ page }) => {
      await openAiPanelPage(page)

      await page.locator('.ai-card', { hasText: '代码评审' }).click()
      await page.locator('.run-btn').click()

      // 运行中态：状态标签「运行中」+ 取消按钮出现（expect 自动等待事件流推进）
      await expect(
        page.locator('.ai-function-panel .el-tag', { hasText: '运行中' })
      ).toBeVisible()
      const cancelBtn = page.getByRole('button', { name: '取消' })
      await expect(cancelBtn).toBeVisible()

      await cancelBtn.click()

      const cancelCalls = await getWailsCalls(page, 'CancelAiTask')
      expect(cancelCalls).toEqual([{ method: 'CancelAiTask', args: ['task-cancel-1'] }])
    })
  })
})
