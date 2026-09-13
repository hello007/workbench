/**
 * Git 提交 / 推送流程 E2E（mock 后端，测「UI 链路正确性」）。
 *
 * 覆盖点：
 * 1. 勾选文件 + 填写提交信息 -> 提交，触发 CommitFiles（参数正确）、成功提示、列表刷新
 * 2. 提交并推送 -> CommitFiles 后先 HasUpstream 探测再 PushRepo（调用顺序与参数正确）
 * 3. 仅推送 -> PushRepo 直调
 * 4. 行级暂存 -> StageFiles 后刷新
 * 5. 异常路径：CommitFiles 失败 -> 错误提示出现且列表不刷新
 *
 * mock 返回值来自 wails-mock-defaults.js 的 E2E 合并表（GetLocalChanges /
 * HasUpstream / PushRepo 等），本文件仅按用例覆盖动态行为（序列 / 错误）。
 */
import { test, expect, getWailsCalls } from './fixtures'
import { WAILS_MOCK_E2E_RETURN_VALUES } from '../src/test/wails-mock-defaults'
import { openGitRepoPage, E2E_REPO_PATH, GIT_FLOW_OVERRIDES } from './git-fixtures'

// 与 E2E 合并表默认值同源，避免两处漂移
const DEFAULT_CHANGES = WAILS_MOCK_E2E_RETURN_VALUES.GetLocalChanges

// 提交成功后 GetLocalChanges 的刷新序列：挂载(1) -> 提交成功后刷新(2) 返回空列表
const CHANGES_THEN_EMPTY = { __sequence__: [DEFAULT_CHANGES, []] }
// 提交并推送的刷新序列：挂载(1) -> 提交后(2) -> 推送后(3)
const CHANGES_THEN_EMPTY_TWICE = { __sequence__: [DEFAULT_CHANGES, [], []] }

const COMMIT_MESSAGE = 'feat: e2e 提交链路验证'

test.describe('Git 提交/推送流程', () => {
  test.describe('提交主链路', () => {
    test.use({
      wailsOverrides: { ...GIT_FLOW_OVERRIDES, GetLocalChanges: CHANGES_THEN_EMPTY }
    })

    test('勾选文件并提交：CommitFiles 参数正确，列表刷新为空', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '本地变动' }).click()

      // 列表渲染：变动计数标签 + 两行变动（未暂存/已暂存分组）
      await expect(page.locator('.local-changes-card').getByText('2 个文件')).toBeVisible()
      const appRow = page.getByRole('row', { name: /src\/app\.js/ })
      const utilsRow = page.getByRole('row', { name: /src\/utils\.js/ })
      await expect(appRow).toBeVisible()
      await expect(utilsRow).toBeVisible()
      await expect(appRow.getByText('未暂存')).toBeVisible()
      await expect(utilsRow.getByText('已暂存')).toBeVisible()

      // 未勾选未填写时提交按钮禁用
      const submitBtn = page.getByRole('button', { name: '提交', exact: true })
      await expect(submitBtn).toBeDisabled()

      // 勾选 src/app.js + 填写提交信息 -> 按钮可用
      // （el-checkbox 原生 input 被样式隐藏，点击样式化 label 触发勾选）
      await appRow.locator('.el-checkbox').click()
      await page.getByPlaceholder('请输入提交信息（必填）').fill(COMMIT_MESSAGE)
      await expect(submitBtn).toBeEnabled()
      await submitBtn.click()

      // 成功提示 + 提交区收起（提交后列表为空，提交输入区随 changes-footer 的 v-if 卸载）
      await expect(page.locator('.el-message', { hasText: '提交成功' })).toBeVisible()
      await expect(page.getByPlaceholder('请输入提交信息（必填）')).toHaveCount(0)

      // 列表刷新为空 -> 空状态出现
      await expect(page.getByText('没有本地变动')).toBeVisible()

      // bound method 调用断言：CommitFiles 收到 (repoPath, message, [勾选文件路径])
      const commitCalls = await getWailsCalls(page, 'CommitFiles')
      expect(commitCalls).toEqual([
        { method: 'CommitFiles', args: [E2E_REPO_PATH, COMMIT_MESSAGE, ['src/app.js']] }
      ])
      // 列表刷新：挂载 + 提交成功后各一次
      const loadCalls = await getWailsCalls(page, 'GetLocalChanges')
      expect(loadCalls).toHaveLength(2)
      expect(loadCalls.every((c) => c.args[0] === E2E_REPO_PATH)).toBe(true)
    })
  })

  test.describe('提交并推送', () => {
    test.use({
      wailsOverrides: { ...GIT_FLOW_OVERRIDES, GetLocalChanges: CHANGES_THEN_EMPTY_TWICE }
    })

    test('提交并推送：CommitFiles -> HasUpstream -> PushRepo 顺序与参数正确', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '本地变动' }).click()

      const appRow = page.getByRole('row', { name: /src\/app\.js/ })
      await appRow.locator('.el-checkbox').click()
      await page.getByPlaceholder('请输入提交信息（必填）').fill(COMMIT_MESSAGE)
      await page.getByRole('button', { name: '提交并推送' }).click()

      // 两段提示：提交成功提示 + 推送输出（PushRepo mock 返回值）
      await expect(page.locator('.el-message', { hasText: '提交成功，准备推送...' })).toBeVisible()
      await expect(page.locator('.el-message', { hasText: 'main -> main' })).toBeVisible()

      // 调用顺序：提交 -> 上游探测 -> 推送（有上游，不 set-upstream）
      const calls = await getWailsCalls(page)
      const names = calls.map((c) => c.method)
      const commitIdx = names.indexOf('CommitFiles')
      const upstreamIdx = names.indexOf('HasUpstream')
      const pushIdx = names.indexOf('PushRepo')
      expect(commitIdx).toBeGreaterThanOrEqual(0)
      expect(upstreamIdx).toBeGreaterThan(commitIdx)
      expect(pushIdx).toBeGreaterThan(upstreamIdx)

      const commitCalls = await getWailsCalls(page, 'CommitFiles')
      expect(commitCalls[0].args).toEqual([E2E_REPO_PATH, COMMIT_MESSAGE, ['src/app.js']])
      const upstreamCalls = await getWailsCalls(page, 'HasUpstream')
      expect(upstreamCalls[0].args).toEqual([E2E_REPO_PATH])
      const pushCalls = await getWailsCalls(page, 'PushRepo')
      expect(pushCalls[0].args).toEqual([E2E_REPO_PATH, false])
    })
  })

  test.describe('仅推送与暂存', () => {
    test.use({ wailsOverrides: { ...GIT_FLOW_OVERRIDES } })

    test('仅推送：有变动时推送按钮直调 PushRepo', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '本地变动' }).click()
      await expect(page.getByRole('row', { name: /src\/app\.js/ })).toBeVisible()

      await page.getByRole('button', { name: '推送', exact: true }).click()

      await expect(page.locator('.el-message', { hasText: 'main -> main' })).toBeVisible()

      const pushCalls = await getWailsCalls(page, 'PushRepo')
      expect(pushCalls).toEqual([{ method: 'PushRepo', args: [E2E_REPO_PATH, false] }])
      // 仅推送不触发提交
      expect(await getWailsCalls(page, 'CommitFiles')).toEqual([])
    })

    test('行级暂存：+暂存 调 StageFiles 后刷新列表', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '本地变动' }).click()

      const appRow = page.getByRole('row', { name: /src\/app\.js/ })
      await appRow.getByRole('button', { name: '+暂存' }).click()

      const stageCalls = await getWailsCalls(page, 'StageFiles')
      expect(stageCalls).toEqual([{ method: 'StageFiles', args: [E2E_REPO_PATH, ['src/app.js']] }])
      // 刷新：挂载 + 暂存后各一次
      const loadCalls = await getWailsCalls(page, 'GetLocalChanges')
      expect(loadCalls).toHaveLength(2)
    })
  })

  test.describe('提交失败链路', () => {
    test.use({
      wailsOverrides: {
        ...GIT_FLOW_OVERRIDES,
        CommitFiles: { __error__: 'nothing to commit, working tree clean' }
      }
    })

    test('CommitFiles 失败：错误提示出现且列表不刷新、输入保留', async ({ page }) => {
      await openGitRepoPage(page)
      await page.getByRole('tab', { name: '本地变动' }).click()

      const messageInput = page.getByPlaceholder('请输入提交信息（必填）')
      const appRow = page.getByRole('row', { name: /src\/app\.js/ })
      await appRow.locator('.el-checkbox').click()
      await messageInput.fill(COMMIT_MESSAGE)
      await page.getByRole('button', { name: '提交', exact: true }).click()

      // 错误提示（handleGitError 拼接前缀 + 错误消息）
      await expect(
        page.locator('.el-message--error', { hasText: '提交失败: nothing to commit, working tree clean' })
      ).toBeVisible()

      // 失败后：输入保留、列表未刷新（仍只有挂载时一次调用）
      await expect(messageInput).toHaveValue(COMMIT_MESSAGE)
      await expect(appRow).toBeVisible()
      expect(await getWailsCalls(page, 'GetLocalChanges')).toHaveLength(1)
    })
  })
})
