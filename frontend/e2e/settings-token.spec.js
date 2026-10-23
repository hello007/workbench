/**
 * 设置面板令牌管理与多设备语义 E2E（mock 驱动）。
 *
 * 覆盖：
 * 1. 令牌管理链路（多 token，PR1）：进网络访问页拉取列表（名称/状态 tag 渲染）
 *    → 新建令牌（永久档）→ 明文一次性回显（warning alert + 复制按钮）
 *    → 撤销单次确认链（ElMessageBox 单次确认形态，`.el-message-box` 裸选择器安全）
 *    → RevokeWebToken 调用断言。
 * 2. 多设备语义明示（PR3）：ws-connections:changed 事件驱动活动栏「N 个设备
 *    已连接」指示与终端面板多端写入提示行显隐（emitWailsEvent 派发，桌面 mock
 *    通道与真实 EventsEmit 语义一致）。
 */
import { test, expect, emitWailsEvent } from './fixtures'

/** 令牌记录形状（对齐 model.WebTokenRecord，unix 秒；0 = 永不过期/未使用） */
const tokenRecord = (over = {}) => ({
  id: 'tok-1',
  name: '手机',
  hash: '1'.repeat(64),
  createdAt: 1735689600,
  lastUsedAt: 0,
  expiresAt: 0,
  revoked: false,
  ...over
})

/** ListWebTokens 条目形状（对齐 model.WebTokenStatus：record + onlineConns，连接管控任务） */
const tokenItem = (over = {}, onlineConns = 0) => ({ record: tokenRecord(over), onlineConns })

/** 打开设置弹窗并切到网络访问页（第 5 个导航项） */
async function openNetworkTab(page) {
  await page.locator('.activity-bar-item').nth(7).click()
  const dialog = page.locator('.settings-dialog')
  await expect(dialog).toBeVisible()
  await dialog.locator('.settings-nav-item').nth(4).click()
  return dialog
}

test.describe('设置：令牌管理（多 token）', () => {
  test.use({
    wailsOverrides: {
      ListWebTokens: [
        tokenItem({ id: 'tok-1', name: '手机' }, 1),
        tokenItem({ id: 'tok-2', name: '遗失设备', revoked: true }, 0)
      ],
      CreateWebToken: {
        plainText: 'b1b2c3d4e5f6a7b8a1b2c3d4e5f6a7b8b1b2c3d4e5f6a7b8b1b2c3d4e5f6a7b8',
        record: tokenRecord({ id: 'tok-new', name: 'Mac' })
      }
    }
  })

  test('列表渲染状态 tag → 新建令牌明文一次性回显 → 撤销单次确认链', async ({ page }) => {
    await page.goto('/')
    const dialog = await openNetworkTab(page)

    // 列表渲染：名称 + 有效/已撤销状态 tag + 在线连接数徽标（onlineConns>0）
    await expect(dialog.locator('.token-list-row')).toHaveCount(2)
    await expect(dialog.locator('.token-list-row').first()).toContainText('手机')
    await expect(dialog.locator('.token-list-row').first()).toContainText('有效')
    await expect(dialog.locator('.token-list-row').first().locator('.token-online-badge')).toHaveText('在线 1')
    await expect(dialog.locator('.token-list-row').nth(1)).toContainText('已撤销')
    await expect(dialog.locator('.token-list-row').nth(1).locator('.token-online-badge')).toHaveCount(0)

    // 新建令牌（永久档）：明文一次性回显在 warning alert 内
    await dialog.locator('.token-name-input input').fill('Mac')
    await dialog.locator('button', { hasText: '新建令牌' }).click()
    const plainAlert = dialog.locator('.token-plain-alert')
    await expect(plainAlert).toBeVisible()
    await expect(plainAlert).toContainText('仅显示一次')
    await expect(plainAlert).toContainText('b1b2c3d4e5f6a7b8')

    // 撤销：单次确认 → RevokeWebToken 调用（单次确认链，裸 .el-message-box 安全）
    await dialog.locator('.token-list-row').first().locator('button', { hasText: '撤销' }).click()
    const messageBox = page.locator('.el-message-box')
    await expect(messageBox).toBeVisible()
    await expect(messageBox).toContainText('撤销后令牌「手机」立即失效')
    await messageBox.locator('button', { hasText: '撤销' }).click()
    await expect(page.evaluate(() => window.__wailsCalls
      .filter((c) => c.method === 'RevokeWebToken'))).resolves.toEqual([
      { method: 'RevokeWebToken', args: ['tok-1'] }
    ])
    // 确认框关闭
    await expect(messageBox).toBeHidden()
  })
})

test.describe('多设备语义明示（PR3）', () => {
  test('连接数事件驱动活动栏设备指示与终端多端写入提示', async ({ page }) => {
    await page.goto('/')

    // 初始连接数 1（桌面 mock 无事件）：指示与终端提示均不显示
    await expect(page.locator('.activity-bar-devices')).toHaveCount(0)

    // 终端面板先展开（多端写入提示的消费端在线）
    await page.locator('.activity-bar-item').nth(8).click()
    await expect(page.locator('.terminal-panel')).toBeVisible()
    await expect(page.locator('.terminal-multi-device-hint')).toHaveCount(0)

    // 第二设备接入（连接数 2）：活动栏指示 + 终端写入提示同时出现
    await emitWailsEvent(page, 'ws-connections:changed', 2)
    await expect(page.locator('.activity-bar-devices')).toBeVisible()
    await expect(page.locator('.activity-bar-devices')).toContainText('2')
    await expect(page.locator('.terminal-multi-device-hint')).toBeVisible()
    await expect(page.locator('.terminal-multi-device-hint')).toContainText('后输入覆盖先输入')

    // 回落 1（其他设备断开）：指示与提示同时隐藏
    await emitWailsEvent(page, 'ws-connections:changed', 1)
    await expect(page.locator('.activity-bar-devices')).toHaveCount(0)
    await expect(page.locator('.terminal-multi-device-hint')).toHaveCount(0)
  })
})
