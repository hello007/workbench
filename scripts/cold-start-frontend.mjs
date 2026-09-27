// WorkBench 前端首屏测量脚本（perf-baseline.md §6 收口，2026-09-27）
//
// 测什么：同一份 embed 前端资产（frontend/dist）经浏览器通道（--serve 无头 + HTTP）
// 在 Chromium 中冷缓存加载渲染的首屏指标。WebView2 亦为 Chromium 内核，此为
// 「前端首屏渲染/可交互」的自动化代理口径（渲染端与 WebView2 的差异仅环境，
// 资产与代码路径一致）；窗口侧耗时（WebView2 初始化）由 cold-start-bench.ps1 测。
//
// 口径（每轮独立 context = 冷缓存，浏览器进程复用 = V8 暖，跑 N 轮取中位数）：
//   goto 发起 → .directory-tree-panel 可见   = 前端首屏渲染（外部时钟）
//   goto 发起 → 工作目录面板可交互代理就绪    = 首屏 + 布局树挂载完成（外部时钟）
//   performance API                          = firstPaint / DOMContentLoaded / load（页内时钟）
//   请求数 + 传输体积                          = 佐证「减少 HTTP 请求」是否瓶颈（路线图子项）
//
// 运行：node scripts/cold-start-frontend.mjs [--runs 5] [--port 36119]
// 依赖：build/bin/workbench.exe（--serve 模式，无 GUI 窗口）；frontend/node_modules（playwright）。
// 注意：serve 模式不写 crash.flag（无 startup 钩子），进程结束直接 kill 无残留。
// 只测量不改生产代码。

import { spawn } from 'child_process'
import net from 'net'
import { readFileSync, existsSync } from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'
import { createRequire } from 'module'

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const require_ = createRequire(import.meta.url)
const { chromium } = require_(path.join(repoRoot, 'frontend/node_modules/playwright'))

// --- 参数解析（与 --version 手动解析风格一致，保持零依赖） ---
const args = process.argv.slice(2)
const argOf = (name, def) => {
  const i = args.indexOf(name)
  return i >= 0 && args[i + 1] ? args[i + 1] : def
}
const RUNS = parseInt(argOf('--runs', '5'), 10)
const PORT = parseInt(argOf('--port', '36119'), 10)

const EXE = path.join(repoRoot, 'build/bin/workbench.exe')
const TOKEN_FILE = path.join(repoRoot, 'data/web_token')
if (!existsSync(EXE)) {
  console.error(`未找到 ${EXE}，请先执行 wails build`)
  process.exit(1)
}
if (!existsSync(TOKEN_FILE)) {
  console.error(`未找到 ${TOKEN_FILE}（应用至少启动过一次生成访问令牌）`)
  process.exit(1)
}
const token = readFileSync(TOKEN_FILE, 'utf8').trim()
const BASE = `http://127.0.0.1:${PORT}`

// freePort 已被 --listen 占用检测覆盖：启动前先探测目标端口须空闲（被占则换 --port）
await new Promise((resolve, reject) => {
  const probe = net.createServer()
  probe.once('error', reject)
  probe.listen(PORT, '127.0.0.1', () => probe.close(() => resolve()))
}).catch(() => {
  console.error(`端口 ${PORT} 已被占用，请用 --port 指定空闲端口`)
  process.exit(1)
})

console.log(`=== WorkBench 前端首屏测量（浏览器通道代理，${RUNS} 轮）===`)
console.log(`serve 进程: ${BASE}（--listen 独立端口，不触碰桌面 36115）`)

// 启动无头 serve 进程（同份 embed 资产；无 GUI 窗口、无 startup 副作用、无 crash.flag）。
// error 监听必须有：spawn 本身失败（ENOENT/EACCES，existsSync 过不掉权限/损坏场景）
// 不抛异常只触发 error 事件，无监听即 uncaught exception。
const serve = spawn(EXE, ['--serve', `--listen=127.0.0.1:${PORT}`], {
  cwd: repoRoot,
  stdio: 'ignore',
})
serve.on('error', err => {
  console.error(`serve 进程拉起失败: ${err.message}`)
  process.exit(1)
})

// 等待 HTTP 就绪（复用 E2E desktop 通道的探测方式：轮询 fetch 健康响应）
const deadline = Date.now() + 30000
let ready = false
while (Date.now() < deadline) {
  try {
    const res = await fetch(`${BASE}/?token=${encodeURIComponent(token)}`, { redirect: 'manual' })
    if (res.status < 500) { ready = true; break }
  } catch { }
  await new Promise(r => setTimeout(r, 50))
}
if (!ready) {
  console.error('serve 进程 30s 未就绪，退出')
  serve.kill()
  process.exit(1)
}

// 热身 1 轮不计入（JIT / 字体 / 内部路由缓存稳定）
const browser = await chromium.launch()

// 整段测量包 try/finally：任一轮抛出（如 selector 超时：token 失效/前端异常）时
// 必须关 browser 并杀 serve——Windows 父进程退出不级联杀子进程，serve.exe 成孤儿
// 会占住端口致下次运行预检退出
try {
  async function measureOnce(label) {
    const context = await browser.newContext()  // 独立 context = 冷缓存（无磁盘/内存缓存复用）
    try {
      const page = await context.newPage()

      let requestCount = 0
      let transferBytes = 0
      page.on('response', async res => {
        requestCount++
        try { transferBytes += (await res.body()).length } catch { }  // 重定向/空 body 忽略
      })

      const t0 = Date.now()
      await page.goto(`${BASE}/?token=${encodeURIComponent(token)}`, { waitUntil: 'commit' })
      await page.waitForSelector('.directory-tree-panel', { state: 'visible' })
      const firstScreenMs = Date.now() - t0

      // 可交互代理：三栏布局全部挂载（对齐 e2e/smoke.spec.js 冒烟断言面）
      await page.waitForSelector('.file-tree-aside', { state: 'visible' })
      await page.waitForSelector('.content-panel', { state: 'visible' })
      const interactiveMs = Date.now() - t0

      // 页内时钟（performance API，不含浏览器启动/网络栈冷启）
      const timing = await page.evaluate(() => {
        const nav = performance.getEntriesByType('navigation')[0]
        const paint = performance.getEntriesByType('paint').find(p => p.name === 'first-paint')
        return {
          firstPaintMs: paint ? Math.round(paint.startTime) : null,
          domContentLoadedMs: nav ? Math.round(nav.domContentLoadedEventEnd) : null,
          loadMs: nav ? Math.round(nav.loadEventEnd) : null,
        }
      })

      const row = { 轮次: label, firstScreenMs, interactiveMs, ...timing, requestCount, transferKB: Math.round(transferBytes / 1024) }
      console.log(`第 ${label} 轮：首屏 ${firstScreenMs} ms / 可交互 ${interactiveMs} ms / 请求 ${requestCount} 个 / 传输 ${row.transferKB} KB`)
      return row
    } finally {
      await context.close()
    }
  }

  console.log('热身轮（不计入统计）...')
  await measureOnce('W')

  const rows = []
  for (let i = 1; i <= RUNS; i++) rows.push(await measureOnce(i))

  const median = arr => {
    const s = [...arr].sort((a, b) => a - b)
    return s[Math.floor((s.length - 1) / 2)]
  }

  console.log('\n=== 逐轮明细（ms / KB）===')
  console.table(rows)

  console.log(`=== 中位数汇总（${RUNS} 轮）===`)
  const summary = {
    前端首屏渲染ms: median(rows.map(r => r.firstScreenMs)),
    首屏可交互代理ms: median(rows.map(r => r.interactiveMs)),
    firstPaint页内ms: median(rows.map(r => r.firstPaintMs)),
    domContentLoaded页内ms: median(rows.map(r => r.domContentLoadedMs)),
    load页内ms: median(rows.map(r => r.loadMs)),
    请求数: median(rows.map(r => r.requestCount)),
    传输KB: median(rows.map(r => r.transferKB)),
  }
  for (const [k, v] of Object.entries(summary)) console.log(`${k.padEnd(24)} ${v}`)
} finally {
  await browser.close()
  serve.kill()
}
console.log('\n口径说明：独立 context = 冷缓存；浏览器进程复用 = V8 暖。')
console.log('传输体积为解压后 body 累计（非网络字节），供量级参考；请求数佐证「减少 HTTP 请求」路线图子项。')
