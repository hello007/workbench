/**
 * 命令面板内容搜索超时重试编排 E2E（2026-09-30 TimedOut 标志组 + 2026-10-08 批量重试
 * 前端编排，mock 驱动）。
 *
 * 覆盖三条链路：
 * 1. 单目录重试：超时标志组 warning 行点击 → ContentSearchDirs 单目录调用（参数断言）
 *    → 零命中仅移除标志组（现状表示清除，正常组保留）
 * 2. 批量重试编排：banner「检测到 N 个」→ 重试全部 → 预占全部目录（warning 行显重试中）
 *    + 信号量 3 限流（第 4 个调用排队）+ banner 进度 done/total → 全部成功 banner 消失
 * 3. searchVersion 版本守卫：重试在途改词重搜 → 旧 ContentSearchDirs 响应写回丢弃 +
 *    worker 提前终止（不再发出排队调用），新搜索结果不被污染
 *
 * mock 形态：`__deferred__` 挂起解析器（fixtures.js releaseDeferred / pendingDeferredCount
 * 手动放行），对齐 logging-and-errors.md 批量重试节的 pending resolver 构造。
 * 本 spec 位于既有 e2e/ 通道目录，vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls, releaseDeferred, pendingDeferredCount } from './fixtures'

const timedOutGroup = (name) => ({
  repoName: name,
  repoPath: `D:/e2e-demo/${name}`,
  items: [],
  timedOut: true
})

/** 打开命令面板并执行一次全局内容搜索（:: 前缀 + Enter） */
async function searchKeyword(page, keyword) {
  await page.goto('/')
  // 全局 keydown 监听在 Home onMounted 注册：goto 返回（load 事件）早于应用挂载，
  // 立即按键会被丢弃——等待活动栏渲染确认挂载后，按键+断言整体 toPass 重试吸收残余竞态
  await expect(page.locator('.activity-bar').locator('.activity-bar-item').first()).toBeVisible()
  const dialog = page.locator('.command-palette-dialog')
  await expect(async () => {
    await page.keyboard.press('Control+p')
    await expect(dialog).toBeVisible()
  }).toPass({ timeout: 10_000 })
  const input = dialog.getByPlaceholder(/搜索文件、目录/)
  await input.fill('::' + keyword)
  await input.press('Enter')
  return dialog
}

test.describe('命令面板：单目录超时重试（warning 行入口）', () => {
  test.use({
    wailsOverrides: {
      // 形状对齐 model.ContentSearchGroup：repo-a 超时标志组（timedOut: true + 空 items），
      // repo-b 正常命中组；重试后零命中返回 []（默认值）= 仅移除标志组
      ContentSearch: [
        timedOutGroup('repo-a'),
        {
          repoName: 'repo-b',
          repoPath: 'D:/e2e-demo/repo-b',
          items: [
            { filePath: 'src/app.js', lineNum: 12, lineText: 'keyword appears here', repoName: 'repo-b', repoPath: 'D:/e2e-demo/repo-b' }
          ]
        }
      ]
    }
  })

  test('标志组 warning 行渲染，点击重试单目录后标志组移除且正常组保留', async ({ page }) => {
    const dialog = await searchKeyword(page, '获客')

    // 搜索调用参数断言（keyword/fileExt/subDir/isGlobal）
    const searchCalls = await getWailsCalls(page, 'ContentSearch')
    expect(searchCalls.length).toBe(1)
    expect(searchCalls[0].args).toEqual(['获客', '', '', true])

    // 超时标志组 warning 行（单目录 < 2 不出批量 banner，走行内重试）
    const warning = dialog.locator('.content-group-timeout-warning')
    await expect(warning).toHaveCount(1)
    await expect(warning).toContainText('此目录搜索超时，结果可能不完整')
    await expect(warning).toContainText('重试此目录')
    await expect(dialog.locator('.content-batch-retry-banner')).toHaveCount(0)
    // 正常命中项照常渲染
    await expect(dialog.locator('.result-item').filter({ hasText: 'src/app.js' })).toBeVisible()

    // 点击 warning 行 → 单目录重试（重试 mock 即时返回空 = 零命中）
    await warning.click()

    // 标志组移除（零命中仅移除），正常组保留
    await expect(dialog.locator('.content-group-timeout-warning')).toHaveCount(0)
    await expect(dialog.locator('.result-item').filter({ hasText: 'src/app.js' })).toBeVisible()

    // 调用参数断言：单目录独立调用（路径 B），目录路径与名称同序成对
    const calls = await getWailsCalls(page, 'ContentSearchDirs')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['获客', '', '', ['D:/e2e-demo/repo-a'], ['repo-a']])
  })
})

test.describe('命令面板：批量重试编排（banner 入口 + 信号量 3 限流）', () => {
  test.use({
    wailsOverrides: {
      ContentSearch: [timedOutGroup('repo-a'), timedOutGroup('repo-b'), timedOutGroup('repo-c'), timedOutGroup('repo-d')],
      // 挂起解析器：调用即挂起，用例断言中间态后手动放行
      ContentSearchDirs: { __deferred__: 'cp-retry' }
    }
  })

  test('重试全部预占 4 目录、在途上限 3、进度 done/total、完成后 banner 消失', async ({ page }) => {
    const dialog = await searchKeyword(page, '关键词')

    // ≥2 超时目录：banner 初始态
    const banner = dialog.locator('.content-batch-retry-banner')
    const bannerBtn = banner.locator('.batch-retry-btn')
    await expect(banner).toContainText('检测到 4 个目录搜索超时')
    await expect(bannerBtn).toHaveText('重试全部')

    await bannerBtn.click()

    // 批量启动：预占全部目标目录（排队中 warning 行也显「重试中...」）+ 进度态按钮禁用
    await expect(banner).toContainText('重试中 (0/4)...')
    await expect(bannerBtn).toBeDisabled()
    await expect(dialog.locator('.content-group-timeout-warning').filter({ hasText: '重试中' })).toHaveCount(4)

    // 信号量 3：在途挂起恰 3 个，第 4 个排队（未发出）
    await expect.poll(async () => pendingDeferredCount(page, 'cp-retry')).toBe(3)

    // 放行 1 个 → worker 取排队目录（累计调用 4、在途回落 3），进度 1/4
    await releaseDeferred(page, 'cp-retry', [])
    await expect(banner).toContainText('重试中 (1/4)...')
    await expect.poll(async () => (await getWailsCalls(page, 'ContentSearchDirs')).length).toBe(4)
    await expect.poll(async () => pendingDeferredCount(page, 'cp-retry')).toBe(3)

    // 放行剩余 3 个 → 全部成功：banner 消失、标志组清零（零命中移除）
    for (let i = 0; i < 3; i++) {
      await releaseDeferred(page, 'cp-retry', [])
    }
    await expect(banner).toHaveCount(0)
    await expect(dialog.locator('.content-group-timeout-warning')).toHaveCount(0)

    // 每目录独立一次调用（路径 B：N 个单目录 ContentSearchDirs，非一次共享 ctx 批量）
    const calls = await getWailsCalls(page, 'ContentSearchDirs')
    expect(calls.length).toBe(4)
    expect(calls[3].args).toEqual(['关键词', '', '', ['D:/e2e-demo/repo-d'], ['repo-d']])
  })
})

test.describe('命令面板：批量重试在途改词重搜（searchVersion 版本守卫）', () => {
  test.use({
    wailsOverrides: {
      // 首次搜索返回 2 个超时标志组；改词重搜（第二次）返回正常命中组
      ContentSearch: {
        __sequence__: [
          [timedOutGroup('repo-a'), timedOutGroup('repo-b')],
          [
            {
              repoName: 'repo-new',
              repoPath: 'D:/e2e-demo/repo-new',
              items: [
                { filePath: 'src/new.js', lineNum: 3, lineText: 'NEW-RESULT-LINE', repoName: 'repo-new', repoPath: 'D:/e2e-demo/repo-new' }
              ]
            }
          ]
        ]
      },
      ContentSearchDirs: { __deferred__: 'cp-version' }
    }
  })

  test('旧响应写回丢弃不污染新结果，worker 提前终止不再发排队调用', async ({ page }) => {
    const dialog = await searchKeyword(page, '旧词')
    await expect(dialog.locator('.content-group-timeout-warning')).toHaveCount(2)

    // 批量重试启动：2 目录并发上限 3 → 两个调用全部在途挂起
    await dialog.locator('.batch-retry-btn').click()
    await expect.poll(async () => pendingDeferredCount(page, 'cp-version')).toBe(2)
    expect(await pendingDeferredCount(page, 'cp-version')).toBe(2)

    // 改词重搜：searchVersion 自增 → 在途批量请求写回应被丢弃
    const input = dialog.getByPlaceholder(/搜索文件、目录/)
    await input.fill('::新词')
    await input.press('Enter')
    await expect(dialog).toContainText('NEW-RESULT-LINE')

    // 放行旧响应（带污染行）→ 版本不匹配：不写回、worker 终止
    await releaseDeferred(page, 'cp-version', [
      {
        repoName: 'repo-a',
        repoPath: 'D:/e2e-demo/repo-a',
        items: [{ filePath: 'old.js', lineNum: 1, lineText: 'STALE-POLLUTION-LINE', repoName: 'repo-a', repoPath: 'D:/e2e-demo/repo-a' }]
      }
    ])
    await releaseDeferred(page, 'cp-version', [])

    // 新结果未被旧响应污染，banner 已随改词重搜复位消失
    await expect(dialog).toContainText('NEW-RESULT-LINE')
    await expect(dialog).not.toContainText('STALE-POLLUTION-LINE')
    await expect(dialog.locator('.content-batch-retry-banner')).toHaveCount(0)

    // worker 版本失效提前 return：仅发出启动时 2 个调用，无排队续发
    const calls = await getWailsCalls(page, 'ContentSearchDirs')
    expect(calls.length).toBe(2)
  })
})
