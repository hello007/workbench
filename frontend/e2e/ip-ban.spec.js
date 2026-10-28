/**
 * IP 自动封禁 E2E（连接管控五期，mock 驱动）。
 *
 * 桌面专用链路（webserve-ip-banned 事件 + ListIPBans 列表 + UnbanIP 解封）在
 * E2E 浏览器 mock 通道无法真实触发后端封禁（阈值计数发生在真实 HTTP/WS 认证点，
 * 且桌面事件浏览器端永不到达），本 spec 以 mock 数据覆盖前端消费面：
 * 1. 活动栏「自动封禁」区块：无连接但有封禁记录时指示可见（可见条件扩展，角标
 *    warning 分流显封禁数）→ 打开明细面板 → 行渲染（IP/递增档位徽标/失败计数/
 *    到期时间/过期态）→ 解封单次确认链 → UnbanIP 调用断言。
 * 2. 桌面 toast：emitWailsEvent 派发 webserve-ip-banned → 全局 warning 提示
 *    （App 级挂载，面板未开也提示）。
 */
import { test, expect, emitWailsEvent } from './fixtures'

/** 封禁条目形状（对齐 model.IPBanStatus：ip/bannedAt/expiresAt unix 秒/failCount/strike/expired/source） */
const banEntry = (over = {}) => ({
  ip: '203.0.113.7',
  bannedAt: Math.floor(Date.now() / 1000) - 60,
  expiresAt: Math.floor(Date.now() / 1000) + 300,
  failCount: 10,
  strike: 1,
  expired: false,
  source: 'auto',
  ...over
})

test.describe('IP 自动封禁（连接管控五期）', () => {
  test.use({
    wailsOverrides: {
      ListIPBans: [banEntry({ strike: 3 }), banEntry({ ip: '198.51.100.9', expired: true })]
    }
  })

  test('活动栏自动封禁区块渲染 → 解封单次确认链 → UnbanIP 调用断言', async ({ page }) => {
    await page.goto('/')

    // 无在途连接但有封禁记录：指示可见（面板可见条件扩展），角标显封禁数
    const indicator = page.locator('.activity-bar-devices')
    await expect(indicator).toBeVisible()
    await expect(indicator.locator('.devices-badge')).toHaveText('2')

    await indicator.click()
    const panel = page.locator('.devices-panel')
    await expect(panel).toBeVisible()
    await expect(panel).toContainText('IP 封禁')

    const rows = panel.locator('.devices-ban-row')
    await expect(rows).toHaveCount(2)
    // 生效中行：IP + 递增档位徽标（六期 strike>1 渲染）+ 窗口失败计数 + 「至 M-D HH:mm」到期形态（非过期态）
    await expect(rows.nth(0)).toContainText('203.0.113.7')
    await expect(rows.nth(0).locator('.devices-ban-strike')).toHaveText('第 3 次')
    await expect(rows.nth(0)).toContainText('10 次失败')
    await expect(rows.nth(0).locator('.devices-ban-expiry')).not.toHaveText('已过期')
    // 首触行（strike=1）无递增语义不渲染徽标
    await expect(rows.nth(1).locator('.devices-ban-strike')).toHaveCount(0)
    // 过期行：「已过期」标注（info 态），解封按钮保留（UnbanIP 幂等 no-op）
    await expect(rows.nth(1)).toContainText('198.51.100.9')
    await expect(rows.nth(1).locator('.devices-ban-expiry')).toHaveText('已过期')

    // 解封：单次确认链（`.el-message-box` 裸选择器安全——本用例唯一 message box）
    await rows.nth(0).locator('button', { hasText: '解封' }).click()
    const messageBox = page.locator('.el-message-box')
    await expect(messageBox).toBeVisible()
    await expect(messageBox).toContainText('203.0.113.7')
    await messageBox.locator('button', { hasText: '解封' }).click()
    await expect(page.evaluate(() => window.__wailsCalls
      .filter((c) => c.method === 'UnbanIP'))).resolves.toEqual([
      { method: 'UnbanIP', args: ['203.0.113.7'] }
    ])
    await expect(messageBox).toBeHidden()
  })

  test('自动封禁触发 toast 提示（App 级挂载，面板未开也提示）', async ({ page }) => {
    await page.goto('/')
    await emitWailsEvent(page, 'webserve-ip-banned', banEntry({ ip: '192.0.2.66' }))
    const toast = page.locator('.el-message')
    await expect(toast).toBeVisible()
    await expect(toast).toContainText('192.0.2.66')
    await expect(toast).toContainText('临时封禁至')
  })

  // 连接管控七期：手动封禁链路（桌面通道入口 → BanIPDialog 填表 → BanIP 调用）。
  // isDesktopChannel 由桌面连接事件锁定（浏览器通道入口隐藏），先派发
  // webserve-connections:changed 再点头部按钮
  test('手动封禁 IP：头部按钮 → 弹窗永久档提交 → BanIP 调用断言', async ({ page }) => {
    await page.goto('/')
    // 先等指示可见（ActivityBar 已挂载、ListIPBans 已加载），再派发桌面连接事件
    // 锁 isDesktopChannel——事件一次性派发，早于 EventsOn 注册会永久丢失
    const indicator = page.locator('.activity-bar-devices')
    await expect(indicator).toBeVisible()
    await emitWailsEvent(page, 'webserve-connections:changed', [
      { id: 1, ip: '192.168.1.5', tokenId: 't1', tokenName: '手机', connectedAt: new Date().toISOString() }
    ])
    await indicator.click()
    const panel = page.locator('.devices-panel')
    await expect(panel).toBeVisible()

    await panel.locator('.devices-ban-add').click()
    const dialog = page.locator('.ban-ip-dialog')
    await expect(dialog).toBeVisible()
    // 默认永久档：仅填 IP 即提交（reason 可选留空）；el-input 自定义 class 挂组件
    // 根 div，原生 input 为其子元素（settings-token.spec 先例同款选择器形态）
    await dialog.locator('.ban-ip-input input').fill('192.0.2.10')
    await dialog.locator('button', { hasText: '封禁' }).click()
    await expect(
      page.evaluate(() => window.__wailsCalls.filter((c) => c.method === 'BanIP'))
    ).resolves.toEqual([{ method: 'BanIP', args: ['192.0.2.10', 0, ''] }])
    await expect(dialog).toBeHidden()
  })
})

// 网段聚合建议（连接管控九期）：同网段自动封禁达阈值产出建议行（封禁列表上方
// 小节），点击行预填 BanIPDialog 网段档（档位自动切换）完成处置闭环。数据源
// ListIPBanSuggestions 桌面专用（浏览器通道 RPCHiddenMethods 屏蔽，E2E mock
// 通道直接 override 消费面）；建议行无通道门（数据空即不渲染），无需派发桌面
// 连接事件锁通道——面板可见由 ListIPBans 覆盖驱动
test.describe('网段聚合建议（连接管控九期）', () => {
  test.use({
    wailsOverrides: {
      ListIPBans: [banEntry()],
      ListIPBanSuggestions: [{ cidr: '203.0.113.0/24', count: 3 }]
    }
  })

  test('聚合建议行渲染 → 点击预填网段封禁弹窗 → BanCIDR 调用断言', async ({ page }) => {
    await page.goto('/')
    const indicator = page.locator('.activity-bar-devices')
    await expect(indicator).toBeVisible()
    await indicator.click()
    const panel = page.locator('.devices-panel')
    await expect(panel).toBeVisible()

    // 建议小节（封禁列表上方）：标题 + 建议行（网段串 + 计数 + 建议文案）
    const block = panel.locator('.devices-suggest-block')
    await expect(block).toContainText('聚合建议')
    const row = block.locator('.devices-suggest-row')
    await expect(row).toContainText('203.0.113.0/24')
    await expect(row).toContainText('该网段 3 个来源已被封禁，建议网段封禁')

    // 点击预填：弹窗档位自动切网段档（placeholder 换网段形态）+ 网段输入预填，
    // 时长档维持默认永久档；直接提交走 BanCIDR（闭环止于预填，提交即管理员确认）
    await row.click()
    const dialog = page.locator('.ban-ip-dialog')
    await expect(dialog).toBeVisible()
    const input = dialog.locator('.ban-ip-input input')
    await expect(input).toHaveValue('203.0.113.0/24')
    await expect(input).toHaveAttribute('placeholder', /203\.0\.113\.0\/24/)
    await dialog.locator('button', { hasText: '封禁' }).click()
    await expect(
      page.evaluate(() => window.__wailsCalls.filter((c) => c.method === 'BanCIDR'))
    ).resolves.toEqual([{ method: 'BanCIDR', args: ['203.0.113.0/24', 0, ''] }])
    await expect(dialog).toBeHidden()
  })
})

// 攻击趋势（连接管控十期）：审计历史 ip_ban/ip_unban 天桶聚合的时序视图（封禁
// 列表下方第四区块），行点击复用九期网段封禁弹窗预填链。数据源 QueryIPBanTrend
// 桌面专用（浏览器通道 RPCHiddenMethods 屏蔽，E2E mock 通道直接 override 消费
// 面）；earliestTs 取「两天前」注入——恒在 7 天窗口内，确定性触发「数据自 M-D 起」
// 轮转提示行（禁依赖本地时区计算的具体日期文案）
test.describe('攻击趋势（连接管控十期）', () => {
  test.use({
    wailsOverrides: {
      ListIPBans: [banEntry()],
      QueryIPBanTrend: {
        windowDays: 7,
        earliestTs: Math.floor(Date.now() / 1000) - 2 * 86400,
        buckets: [
          { key: '203.0.113.0/24', days: [0, 1, 0, 3, 0, 2, 5], autoTotal: 11, manualCount: 2, rebounded: true, lastSeen: 1735689900 },
          { key: '198.51.100.0/24', days: [0, 0, 0, 0, 0, 1, 0], autoTotal: 1, manualCount: 0, rebounded: false, lastSeen: 1735689000 }
        ]
      }
    }
  })

  test('趋势桶行渲染（计数/天级条形/处置/再犯徽标）→ earliest 提示行 → 行点击预填 BanCIDR', async ({ page }) => {
    await page.goto('/')
    const indicator = page.locator('.activity-bar-devices')
    await expect(indicator).toBeVisible()
    await indicator.click()
    const panel = page.locator('.devices-panel')
    await expect(panel).toBeVisible()

    // 趋势区块：标题带窗口天数 + 桶行（网段串 + 总计数 + 天级条形 + 双徽标），
    // 后端已排序（autoTotal 降序），前端零排序原样渲染
    const block = panel.locator('.devices-trend-block')
    await expect(block).toContainText('攻击趋势（近 7 天）')
    const rows = block.locator('.devices-trend-row')
    await expect(rows).toHaveCount(2)
    await expect(rows.nth(0)).toContainText('203.0.113.0/24')
    await expect(rows.nth(0)).toContainText('11 次自动封禁')
    await expect(rows.nth(0).locator('.devices-trend-bar')).toHaveCount(7)
    await expect(rows.nth(0).locator('.devices-trend-manual')).toHaveText('处置 2')
    await expect(rows.nth(0).locator('.devices-trend-rebounded')).toHaveText('再犯')
    // manualCount=0 与 rebounded=false 不渲染徽标
    await expect(rows.nth(1).locator('.devices-trend-manual')).toHaveCount(0)
    await expect(rows.nth(1).locator('.devices-trend-rebounded')).toHaveCount(0)

    // earliest 提示行（留存未覆盖整个窗口的诚实标注）
    await expect(block.locator('.devices-trend-earliest')).toContainText('数据自')
    await expect(block.locator('.devices-trend-earliest')).toContainText('更早历史已随审计日志轮转')

    // 行点击复用九期预填链：弹窗网段档 + 桶 key 预填，提交走 BanCIDR
    await rows.nth(0).click()
    const dialog = page.locator('.ban-ip-dialog')
    await expect(dialog).toBeVisible()
    const input = dialog.locator('.ban-ip-input input')
    await expect(input).toHaveValue('203.0.113.0/24')
    await dialog.locator('button', { hasText: '封禁' }).click()
    await expect(
      page.evaluate(() => window.__wailsCalls.filter((c) => c.method === 'BanCIDR'))
    ).resolves.toEqual([{ method: 'BanCIDR', args: ['203.0.113.0/24', 0, ''] }])
    await expect(dialog).toBeHidden()
  })
})
