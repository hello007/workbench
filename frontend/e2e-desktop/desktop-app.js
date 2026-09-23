// 真桌面 E2E 应用进程管理（真桌面进程 + 浏览器通道，WebView2 CDP 的选型修正见
// 任务 prd.md「选型实证」——Wails v2.16 经 go-webview2 显式传参
// CreateCoreWebView2EnvironmentWithOptions，loader 层 WEBVIEW2_* 环境变量与注册表
// 策略均被忽略，remote-debugging-port 无法外注入，经 4 路实测定性）。
//
// 本通道保真面：真实 Go 进程生命周期（startup/shutdown 钩子、crash.flag）、真实
// session.json 持久化、真实后端方法与事件（浏览器通道 polyfill 经 WS/RPC 触达
// 真实 App 方法，非 mock）。差异面仅为渲染端（系统浏览器 vs WebView2 窗口），
// 而渲染端差异不是本任务验证目标。
//
// 实例隔离（关键——用户桌面常驻真实 workbench.exe，绝不触碰）：
//   1. exe 复制改名 workbench-e2e.exe 后启动——Wails 默认 WebView2 profile 按
//      exe 名隔离（%APPDATA%\<exe 名>\EBWebView），两实例互不冲突；
//   2. --listen=127.0.0.1:<探测端口> 命令行覆盖，HTTP 通道不与用户实例的 36115 冲突；
//   3. cwd=独立临时目录，data/ 全部落隔离目录。
//
// 关闭语义：closeGracefully 用 taskkill（不带 /F）向 GUI 窗口发 WM_CLOSE，Wails 走
// 正常 shutdown 钩子（crash.flag 清除、session.json 落盘）；killForce 用 taskkill /F
// 模拟崩溃/强杀（钩子不执行，crash.flag 残留）。两者是异常退出检测链路的核心差异点。
//
// 端口：结构性须预知（--listen 启动参数），启动前 listen(0) 探测空闲端口后立即
// 使用，存在探测→spawn 间隙被占的理论窗口（TOCTOU）——本地桌面脚本场景接受，
// 与 docs/spec/test-stability.md 固定端口规避同源权衡。
import { spawn, execSync } from 'child_process'
import net from 'net'
import { copyFileSync, existsSync, mkdirSync, readFileSync } from 'fs'
import path from 'path'

// 产物 exe 相对仓库根路径（npm script 以 frontend/ 为 cwd，Playwright 转译
// helper 为 CJS 不可用 import.meta，故以 process.cwd() 上跳一级为仓库根）；
// 不存在时测试 skip（CI Linux / 未构建环境天然跳过）
export const DESKTOP_EXE = path.resolve(process.cwd(), '../build/bin/workbench.exe')
export const desktopAvailable = () => process.platform === 'win32' && existsSync(DESKTOP_EXE)

const HTTP_READY_TIMEOUT = 60000 // 桌面进程冷启动 + web serve 就绪等待上限

// freePort 探测空闲端口（listen(0) 由内核分配后立即释放，端口复用窗口见文件头说明）
export function freePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer()
    srv.listen(0, '127.0.0.1', () => {
      const port = srv.address().port
      srv.close(() => resolve(port))
    })
    srv.on('error', reject)
  })
}

// launchApp 以隔离环境启动真桌面应用：exe 复制改名（WebView2 profile 隔离）、
// cwd=workDir（data/ 隔离）、--listen 独立端口（HTTP 通道隔离）。
// 等待 HTTP 服务就绪后返回 { proc, port, workDir }。
export async function launchApp(port, workDir) {
  mkdirSync(workDir, { recursive: true })
  const exe = path.join(workDir, 'workbench-e2e.exe')
  // 复制带重试：源映像被运行中实例/杀毒扫描瞬时锁定时 EBUSY（Windows 文件锁
  // 释放滞后于进程退出信号），3 次 × 500ms 吸收抖动
  let lastErr
  for (let i = 0; i < 3; i++) {
    try {
      copyFileSync(DESKTOP_EXE, exe)
      lastErr = null
      break
    } catch (e) {
      lastErr = e
      await new Promise(r => setTimeout(r, 500))
    }
  }
  if (lastErr) throw lastErr
  const proc = spawn(exe, [`--listen=127.0.0.1:${port}`], {
    cwd: workDir,
    stdio: 'ignore'
  })
  // 就绪判定 = 带 workDir 自己的 token 访问返回 200。仅靠「端口有响应」会误连
  // 前序用例拖尾实例（同 port 占用 / 内核端口复用），token 匹配保证放行的必是
  // 本 workDir 的实例（拖尾实例 token 不同必 401，继续轮询直至本实例就绪）
  const tokenPath = path.join(workDir, 'data', 'web_token')
  await waitUntil(() => existsSync(tokenPath), 15000, 'web_token 文件')
  const token = readFileSync(tokenPath, 'utf-8').trim()
  await waitUntil(async () => {
    const res = await fetch(`http://127.0.0.1:${port}/?token=${token}`)
    return res.status === 200
  }, HTTP_READY_TIMEOUT, `web serve(${port})`)
  return { proc, port, workDir }
}

// openAppPage 打开浏览器访问应用 UI（query 通道带 token，服务端种 cookie 后续免带）
export async function openAppPage(browser, port, workDir) {
  const token = readFileSync(path.join(workDir, 'data', 'web_token'), 'utf-8').trim()
  const page = await browser.newPage()
  await page.goto(`http://127.0.0.1:${port}/?token=${token}`)
  await page.waitForSelector('.activity-bar-item', { timeout: 30000 })
  return page
}

// waitUntil 轮询直到 fn 返回真值或超时
async function waitUntil(fn, timeoutMs, what) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      if (await fn()) return
    } catch { /* 未就绪，继续轮询 */ }
    await new Promise(r => setTimeout(r, 300))
  }
  throw new Error(`${what} 在 ${timeoutMs}ms 内未就绪`)
}

// waitProcessGone 等待进程完全退出；超时强杀兜底——Wails shutdown 钩子先于进程
// 本体退出完成（webview2 子进程树拖尾），调用方断言（flag/session.json 落盘）
// 均基于落盘产物已完成，强杀仅清理进程树，不回退已验证状态
export async function waitProcessGone(proc, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      process.kill(proc.pid, 0) // 信号 0 仅探测存活；ESRCH = 已退出
      await new Promise(r => setTimeout(r, 200))
    } catch {
      return
    }
  }
  killForce(proc)
  await new Promise(r => setTimeout(r, 1000))
}

// closeGracefully 正常关闭（WM_CLOSE，Wails shutdown 钩子执行）
export function closeGracefully(proc) {
  if (!proc.pid) return
  try {
    execSync(`taskkill /PID ${proc.pid} /T`, { stdio: 'ignore' })
  } catch { /* 进程可能已退出 */ }
}

// killForce 强杀进程树（模拟崩溃，Wails shutdown 钩子不执行）
export function killForce(proc) {
  if (!proc.pid) return
  try {
    execSync(`taskkill /PID ${proc.pid} /T /F`, { stdio: 'ignore' })
  } catch { /* 进程可能已退出 */ }
}
