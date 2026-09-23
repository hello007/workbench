// 真桌面 E2E：会话状态真实持久化恢复链路（mock 通道盲区）。
//
// mock 通道（vite preview + 注入 mock）无法覆盖：真实 session.json 读写、
// Wails 正常 shutdown 钩子、应用重启周期。本链路：切换活动面板 → 正常关闭 →
// 断言 session.json 落盘 → 重启 → 断言面板恢复。
import { test, expect } from '@playwright/test'
import {
  desktopAvailable, freePort, launchApp, openAppPage,
  closeGracefully, killForce, waitProcessGone
} from './desktop-app'
import { mkdtempSync, readFileSync, existsSync } from 'fs'
import { tmpdir } from 'os'
import path from 'path'

test.skip(!desktopAvailable(), '真桌面 E2E 需 Windows + build/bin/workbench.exe（先 wails build）')
test.setTimeout(180000)

// pickActivePanel 读当前活动面板（ActivityBar active 类下标）
async function pickActivePanel(page) {
  return page.evaluate(() => {
    const items = [...document.querySelectorAll('.activity-bar-item')]
    return items.findIndex(i => i.classList.contains('is-active'))
  })
}

test('切换面板后正常关闭重启，活动面板真实恢复', async ({ browser }) => {
  const workDir = mkdtempSync(path.join(tmpdir(), 'wb-desktop-session-'))
  const port = await freePort()

  // 第一次启动：切面板（切到与默认不同的面板），等待 debounce 落盘
  const { proc } = await launchApp(port, workDir)
  const page = await openAppPage(browser, port, workDir)

  const before = await pickActivePanel(page)
  const target = before === 0 ? 1 : 0
  await page.locator('.activity-bar-item').nth(target).click()
  await expect
    .poll(() => pickActivePanel(page), { timeout: 10000 })
    .toBe(target)

  // 等前端 debounce（2s）把快照推给真实后端落盘，再正常关闭
  await page.waitForTimeout(3500)
  // 先断开浏览器页面（WS），再发 WM_CLOSE——页面连接挂着时 shutdown 会等待断开
  await page.close()
  closeGracefully(proc)

  // 正常关闭：Wails shutdown 钩子执行，session.json 落盘
  await expect
    .poll(() => existsSync(path.join(workDir, 'data', 'session.json')), { timeout: 15000 })
    .toBe(true)
  const snapshot = JSON.parse(readFileSync(path.join(workDir, 'data', 'session.json'), 'utf-8'))
  expect(snapshot.activePanel).toBeTruthy()

  // 第二次启动：恢复切换后的活动面板（等前进程完全退出释放 exe 锁）
  await waitProcessGone(proc)
  const port2 = await freePort()
  const { proc: proc2 } = await launchApp(port2, workDir)
  const page2 = await openAppPage(browser, port2, workDir)

  await expect
    .poll(() => pickActivePanel(page2), { timeout: 15000 })
    .toBe(target)

  await page2.close()
  closeGracefully(proc2)
  killForce(proc2) // 兜底清理进程树（幂等）
})
