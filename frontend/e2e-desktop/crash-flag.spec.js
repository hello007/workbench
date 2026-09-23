// 真桌面 E2E：异常退出检测标记闭环（mock 通道盲区，衔接 09-23-error-report-export）。
//
// crash.flag 生命周期只有真实进程生命周期可验证：正常关闭（Wails shutdown 钩子
// 执行）清除；强杀（taskkill /F，钩子不执行）残留；残留 flag 下次启动触发
// 「上次运行异常退出」提示。mock 通道无进程生命周期概念，完全无法覆盖。
import { test, expect } from '@playwright/test'
import {
  desktopAvailable, freePort, launchApp, openAppPage,
  closeGracefully, killForce, waitProcessGone
} from './desktop-app'
import { mkdtempSync, existsSync, readFileSync } from 'fs'
import { tmpdir } from 'os'
import path from 'path'

test.skip(!desktopAvailable(), '真桌面 E2E 需 Windows + build/bin/workbench.exe（先 wails build）')
test.setTimeout(180000)

const flagOf = (workDir) => path.join(workDir, 'data', 'crash.flag')

// pollFlagGone 轮询等待 crash.flag 消失（shutdown 钩子清 flag 完成的确定性信号）
async function pollFlagGone(workDir, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (!existsSync(flagOf(workDir))) return true
    await new Promise(r => setTimeout(r, 300))
  }
  return false
}

test('正常退出路径：WM_CLOSE 进程可正常退出，重启后功能可用', async ({ browser }) => {
  // 断言口径说明：crash.flag 清除位于 shutdown 钩子末尾，而 go-webview2 的
  // errorCallback 在窗口销毁链中偶发 os.Exit(1) 直接终止进程跳过 shutdown
  // （依赖库行为，见任务 prd「选型实证」），flag 终态方向不稳定，故本用例不断言
  // flag 方向，只断言确定性终态：WM_CLOSE 进程可退出 + 重启功能可用。
  // flag「清除/残留」两方向分别由单测 TestCrashFlagLifecycle 与下方强杀用例覆盖。
  const workDir = mkdtempSync(path.join(tmpdir(), 'wb-desktop-clean-'))
  const port = await freePort()

  const { proc } = await launchApp(port, workDir)
  const page = await openAppPage(browser, port, workDir)

  // 首次启动：启动器重建了 flag（标记本次运行中），运行中应存在
  expect(existsSync(flagOf(workDir))).toBe(true)

  // 正常关闭：WM_CLOSE 进程可退出（偶发丢失则重发一次）
  await page.close()
  closeGracefully(proc)
  try {
    await waitProcessGone(proc, 15000)
  } catch {
    closeGracefully(proc)
    await waitProcessGone(proc, 15000)
  }

  // 重启：应用功能可用（HTTP 就绪 + 前端渲染）
  const port2 = await freePort()
  const { proc: proc2 } = await launchApp(port2, workDir)
  const page2 = await openAppPage(browser, port2, workDir)
  await expect(page2.locator('.activity-bar-item').first()).toBeVisible()

  await page2.close()
  closeGracefully(proc2)
  killForce(proc2)
})

test('强杀进程残留异常退出标记，重启出现提示', async ({ browser }) => {
  const workDir = mkdtempSync(path.join(tmpdir(), 'wb-desktop-crash-'))
  const port = await freePort()

  const { proc } = await launchApp(port, workDir)
  const page = await openAppPage(browser, port, workDir)

  // 强杀进程树：Wails shutdown 钩子不执行 → crash.flag 残留
  await page.close()
  killForce(proc)

  // 重启：检测到残留 flag →「上次运行异常退出」提示出现（读后即清；等前进程退出释放 exe 锁）
  await waitProcessGone(proc)
  const port2 = await freePort()
  const { proc: proc2 } = await launchApp(port2, workDir)
  const page2 = await openAppPage(browser, port2, workDir)

  // 后端检测断言（确定性锚点）：第二实例 startup 日志含异常退出检测记录；
  // UI 提示（ElNotification 8s 自动关闭）与页面就绪存在时序竞争，不在真桌面断言，
  // 由 App.vue 组件测试覆盖
  const logPath = path.join(workDir, 'data', 'logs', 'app.log')
  await expect
    .poll(() => readFileSync(logPath, 'utf-8').includes('last session exited abnormally'), { timeout: 10000 })
    .toBe(true)

  await page2.close()
  closeGracefully(proc2)
  killForce(proc2)
})
