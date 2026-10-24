/**
 * 审计历史（连接管控三期）E2E：设置页入口 → 审计日志弹窗全链路（mock 驱动）。
 *
 * 覆盖：
 * 1. 桌面通道入口可见（隐私红线：浏览器通道入口隐藏——E2E 运行于桌面 mock 形态，
 *    浏览器通道行为由 vitest 组件用例 + 后端 HiddenMethods HTTP 断言覆盖）；
 * 2. 打开即调 QueryAuditLog（分页缺省参数）并渲染 mock 行（时间/事件/来源/操作者），
 *    wailsjs mock 形状漂移兜底暴露面（service.AuditQueryResult 大写驼峰 + 行内
 *    json 小写 tag 的双层键名契约）；
 * 3. 事件过滤（EP 多选下拉）→ 重查携带 Events 参数并回第 1 页；
 * 4. 翻页 → Offset/Limit 透传。
 */
import { test, expect, getWailsCalls } from './fixtures'

/** 打开设置弹窗（活动栏设置图标，nth(7)）并切到网络访问 tab */
async function openNetworkTab(page) {
  await page.locator('.activity-bar-item').nth(7).click()
  const dialog = page.locator('.settings-dialog')
  await expect(dialog).toBeVisible()
  await dialog.locator('.settings-nav-item').nth(4).click()
  return dialog
}

test.describe('审计历史（连接管控三期）', () => {
  test.use({
    wailsOverrides: {
      QueryAuditLog: {
        Total: 45, // > pageSize(20)：保证翻页按钮可用（行渲染只断言首行数据）
        Records: [
          { ts: 1735689700, event: 'conn_kick', tokenId: 'tok-default', name: '默认', remoteAddr: '192.168.1.5', connId: 1, actorSource: 'desktop' },
          { ts: 1735689600, event: 'auth_failure', tokenId: 'tok-default', remoteAddr: '192.168.1.6', userAgent: 'Mozilla/5.0' }
        ]
      }
    }
  })

  test('设置页入口打开弹窗，渲染审计行（形状漂移兜底）', async ({ page }) => {
    await page.goto('/')
    const dialog = await openNetworkTab(page)

    const entryBtn = dialog.locator('button', { hasText: '查看审计日志' })
    await expect(entryBtn).toBeVisible()
    await entryBtn.click()

    const auditDialog = page.locator('.audit-history-dialog')
    await expect(auditDialog).toBeVisible()

    // 打开即调 QueryAuditLog（缺省分页参数：第 1 页 20 条，无过滤）
    await expect.poll(async () => (await getWailsCalls(page, 'QueryAuditLog')).length).toBe(1)
    const firstCall = (await getWailsCalls(page, 'QueryAuditLog'))[0].args[0]
    expect(firstCall).toMatchObject({ Offset: 0, Limit: 20 })
    expect(firstCall.Events).toEqual([])

    // 行渲染：外层 Total/Records 大写驼峰 + 行内 json 小写 tag 双层键名契约
    await expect(auditDialog.locator('.el-table')).toBeVisible()
    await expect(auditDialog.getByText('踢出连接')).toBeVisible()
    await expect(auditDialog.getByText('认证失败')).toBeVisible()
    // 操作者归因（desktop → 桌面用户）与来源 IP 留痕
    await expect(auditDialog.getByText('桌面用户')).toBeVisible()
    await expect(auditDialog.getByText('192.168.1.5')).toBeVisible()

    // 翻页：第 2 页 Offset=20
    await auditDialog.locator('.el-pagination .btn-next').click()
    await expect.poll(async () => (await getWailsCalls(page, 'QueryAuditLog')).length).toBe(2)
    const secondCall = (await getWailsCalls(page, 'QueryAuditLog'))[1].args[0]
    expect(secondCall).toMatchObject({ Offset: 20, Limit: 20 })
  })

  test('事件过滤重查携带 Events 参数', async ({ page }) => {
    await page.goto('/')
    const dialog = await openNetworkTab(page)
    await dialog.locator('button', { hasText: '查看审计日志' }).click()
    const auditDialog = page.locator('.audit-history-dialog')
    await expect(auditDialog).toBeVisible()
    await expect.poll(async () => (await getWailsCalls(page, 'QueryAuditLog')).length).toBe(1)

    // 事件多选下拉：点开选择「踢出连接」
    await auditDialog.locator('.audit-filter-events').click()
    await page.locator('.el-select-dropdown__item', { hasText: '踢出连接' }).first().click()
    await page.keyboard.press('Escape')

    await expect.poll(async () => (await getWailsCalls(page, 'QueryAuditLog')).length).toBe(2)
    const secondCall = (await getWailsCalls(page, 'QueryAuditLog'))[1].args[0]
    expect(secondCall.Events).toEqual(['conn_kick'])
    expect(secondCall.Offset).toBe(0)
  })
})
