// WorkBench 前端文件树规模-内存测量脚本（perf-baseline.md §15 收口，2026-09-27）
//
// 测什么：单目录 1k / 10k / 100k 文件在 el-tree 懒加载下「选中工作目录 = 展开渲染
// 该层全部节点」的真实链路前端成本。口径对齐：
//   - fixture 单目录 N 文件 = service 包 BenchmarkFileTreeGetChildren_Scale 同构场景
//     （展开一次全量渲染 N 个节点的最坏情况）；
//   - 链路 = 选中 .dir-item → loadTreeNode → App.GetFileTree（GetChildren 单层）→
//     el-tree 渲染 N 个 .el-tree-node，无虚拟滚动，全量 DOM。
//
// 口径（每轮独立 context = 冷缓存，浏览器进程复用 = V8 暖，跑 N 轮取中位数）：
//   选中目录点击 → .el-tree-node 数量达标 = 渲染完成（外部时钟，waitForFunction
//   确定性等待，非固定延时）；CDP Performance.getMetrics = JSHeapUsedSize / Nodes（DOM 节点数）；
//   performance.memory.usedJSHeapSize = 页面内口径交叉验证。
//
// 隔离：serve 进程 cwd 指向临时目录，data/（工作目录配置、settings、web_token）
// 全部落在临时目录，用户真实配置零污染；fixture 亦建在临时目录，脚本结束清理。
//
// 运行：node scripts/frontend-memory-tree.mjs [--runs 3] [--port 36121] [--keep-fixture]
//       [--scales 1000,10000,100000]（默认三档全跑；冒烟可 --scales 1000）
// 依赖：build/bin/workbench.exe（--serve 模式）；frontend/node_modules（playwright）。
// 只测量不改生产代码。100k 档 fixture 构造与 DOM 渲染耗时显著（NTFS 海量小文件、
// 无虚拟滚动全量节点），属预期；渲染超过 --render-timeout 秒记超时（本身即瓶颈证据）。

import { spawn } from 'child_process'
import net from 'net'
import { readFileSync, existsSync, mkdtempSync, writeFileSync, mkdirSync, rmSync } from 'fs'
import os from 'os'
import path from 'path'
import { fileURLToPath } from 'url'
import { createRequire } from 'module'

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const require_ = createRequire(import.meta.url)
const { chromium } = require_(path.join(repoRoot, 'frontend/node_modules/playwright'))

// --- 参数解析（与 cold-start-frontend.mjs 同风格，零依赖） ---
const args = process.argv.slice(2)
const argOf = (name, def) => {
  const i = args.indexOf(name)
  return i >= 0 && args[i + 1] ? args[i + 1] : def
}
const hasFlag = name => args.includes(name)
const RUNS = parseInt(argOf('--runs', '3'), 10)
const PORT = parseInt(argOf('--port', '36121'), 10)
const RENDER_TIMEOUT_MS = parseInt(argOf('--render-timeout', '120'), 10) * 1000
const KEEP_FIXTURE = hasFlag('--keep-fixture')
for (const [k, v] of [['--runs', RUNS], ['--port', PORT], ['--render-timeout', RENDER_TIMEOUT_MS / 1000]]) {
  if (!Number.isFinite(v) || v <= 0) {
    console.error(`参数 ${k} 非法: 须为正整数`)
    process.exit(1)
  }
}

const SCALES = (argOf('--scales', '1000,10000,100000'))
  .split(',')
  .map(s => parseInt(s.trim(), 10))
  .filter(n => Number.isFinite(n) && n > 0)

// 工作目录显示名：mem- 前缀 + text-is 精确匹配（has-text 子串会误匹配 mem-1000/mem-10000）
const dirName = n => `mem-${n}`
const CTRL_NAME = 'mem-ctrl'

const EXE = path.join(repoRoot, 'build/bin/workbench.exe')
if (!existsSync(EXE)) {
  console.error(`未找到 ${EXE}，请先执行 wails build`)
  process.exit(1)
}

// --- 隔离工作区：serve cwd + fixture 同根，脚本结束清理 ---
const workspace = mkdtempSync(path.join(os.tmpdir(), 'wb-tree-mem-'))
const dataDir = path.join(workspace, 'data')
mkdirSync(dataDir)
console.log(`=== WorkBench 前端文件树规模-内存测量（浏览器通道，${RUNS} 轮/档）===`)
console.log(`隔离工作区: ${workspace}（serve cwd 指向此处，data/ 与 fixture 均不触碰真实配置）`)

// fixture 构造：单目录 N 个 1 字节小文件。构造耗时单独计（不混入渲染测量口径）。
// 文件名零填充定宽保证字典序稳定，与 Go 侧 buildFlatFileFixture 同构。
const fixtureDirs = []
let fixtureBuildMs = 0
// 控制目录：空目录。loadDirectories 自动选中首个目录（渲染其树）——每轮先选中
// 控制目录把树归到近零节点态，再点击 scale 目录，增量口径才成立。
const ctrlDir = path.join(workspace, 'ctrl')
mkdirSync(ctrlDir)
fixtureDirs.push({ files: 0, dir: ctrlDir, name: CTRL_NAME })
for (const n of SCALES) {
  const dir = path.join(workspace, `mem-${n}`)
  const t0 = Date.now()
  mkdirSync(dir)
  const nameWidth = String(n).length
  for (let i = 0; i < n; i++) {
    writeFileSync(path.join(dir, `file${String(i).padStart(nameWidth, '0')}.txt`), 'x')
  }
  const ms = Date.now() - t0
  fixtureBuildMs += ms
  fixtureDirs.push({ files: n, dir, name: dirName(n) })
  console.log(`fixture ${n} 文件构造完成: ${(ms / 1000).toFixed(1)} s`)
}
console.log(`fixture 总构造耗时: ${(fixtureBuildMs / 1000).toFixed(1)} s（不计入渲染口径）\n`)

// --- 启动 serve（cwd = 隔离工作区，data/ 全隔离） ---
const PORT_PROBE = await new Promise((resolve, reject) => {
  const probe = net.createServer()
  probe.once('error', reject)
  probe.listen(PORT, '127.0.0.1', () => probe.close(() => resolve(true)))
}).catch(() => false)
if (!PORT_PROBE) {
  console.error(`端口 ${PORT} 已被占用，请用 --port 指定空闲端口`)
  cleanupWorkspace()
  process.exit(1)
}

const serve = spawn(EXE, ['--serve', `--listen=127.0.0.1:${PORT}`], {
  cwd: workspace, // 隔离关键：data/web_token 等落此处
  stdio: 'ignore',
})
serve.on('error', err => {
  console.error(`serve 进程拉起失败: ${err.message}`)
  cleanupWorkspace() // spawn 阶段失败：browser/测量未启动，仅工作区须清理
  process.exit(1)
})

const BASE = `http://127.0.0.1:${PORT}`
// 统一早退路径：kill 后等 exit 确认（Windows 句柄释放异步，立即退留孤儿占端口），
// 再清理隔离工作区（早退时 fixture 已构造，最多 11 万文件残留 tmpdir）
const killServeAndExit = async proc => {
  proc.kill()
  await new Promise(resolve => {
    if (proc.exitCode != null) return resolve()
    const t = setTimeout(resolve, 5000)
    proc.once('exit', () => { clearTimeout(t); resolve() })
  })
  cleanupWorkspace()
  process.exit(1)
}

// 幂等工作区清理（主 finally 与各早退路径共用）
const cleanupWorkspace = () => {
  if (KEEP_FIXTURE) {
    console.log(`--keep-fixture：保留工作区 ${workspace}`)
    return
  }
  try { rmSync(workspace, { recursive: true, force: true, maxRetries: 3, retryDelay: 200 }) } catch (e) {
    console.error(`清理失败（可手动删除）: ${workspace}: ${e.message}`)
  }
}

// 等 serve 就绪后读隔离 token（首次启动生成）
const deadline = Date.now() + 30000
let token = ''
let ready = false
while (Date.now() < deadline) {
  const tokenFile = path.join(dataDir, 'web_token')
  if (existsSync(tokenFile)) {
    try {
      const res = await fetch(`${BASE}/?token=${encodeURIComponent(readFileSync(tokenFile, 'utf8').trim())}`, { redirect: 'manual' })
      if (res.status < 500) {
        token = readFileSync(tokenFile, 'utf8').trim()
        ready = true
        break
      }
    } catch { }
  }
  await new Promise(r => setTimeout(r, 50))
}
if (!ready) {
  console.error('serve 进程 30s 未就绪，退出')
  killServeAndExit(serve)
}

// 整段测量包 try/finally：任一轮抛出必须关 browser 并杀 serve（Windows 父进程退出
// 不级联杀子进程，孤儿占端口致下次运行预检退出），并清理隔离工作区。
// launch 在 try 内：launch 失败（浏览器缺失/版本不匹配）时 finally 兜底杀 serve。
let browser = null
try {
  browser = await chromium.launch()
  const median = arr => {
    const s = [...arr].sort((a, b) => a - b)
    return s[Math.floor((s.length - 1) / 2)]
  }

  // 注册全部工作目录（写隔离 data/directories.json，一次）：首个注册的 ctrl 目录
  // 会被 loadDirectories 自动选中，天然成为每轮的归零起点。
  {
    const context = await browser.newContext()
    try {
      const page = await context.newPage()
      await page.goto(`${BASE}/?token=${encodeURIComponent(token)}`, { waitUntil: 'load' })
      await page.waitForSelector('.directory-tree-panel', { state: 'visible', timeout: 30000 })
      for (const fixture of fixtureDirs) {
        const added = await page.evaluate(async (f) => {
          const dir = await window.go.main.App.AddDirectory(f.name, f.dir, false)
          return dir != null
        }, fixture)
        if (!added) throw new Error(`AddDirectory(${fixture.name}) 失败（隔离 data 写入被拒）`)
      }
      await page.reload({ waitUntil: 'load' })
      await page.waitForSelector('.dir-item', { state: 'visible', timeout: 30000 })
    } finally {
      await context.close()
    }
  }

  const dirItemSel = name => `.dir-item:has(.dir-item-name:text-is("${name}"))`

  async function measureOnce(fixture, label) {
    const context = await browser.newContext() // 独立 context = 冷缓存
    try {
      const page = await context.newPage()
      await page.goto(`${BASE}/?token=${encodeURIComponent(token)}`, { waitUntil: 'load' })
      await page.waitForSelector('.dir-item', { state: 'visible', timeout: 30000 })

      // CDP 会话：JSHeapUsedSize（CDP Nodes 指标 headless 动态插入不刷新，弃用）
      const cdp = await context.newCDPSession(page)
      await cdp.send('Performance.enable')

      // 归零态：选中控制目录（空树），消除上轮/自动选中残留
      await page.click(dirItemSel(CTRL_NAME))
      await page.waitForFunction(
        () => document.querySelectorAll('.el-tree-node').length < 10,
        undefined,
        { timeout: 30000, polling: 100 },
      )

      // 基线（渲染前）：控制目录空树态；采样前强制 GC 消除上一档/上一轮残留污染
      // （控制目录归零只清 DOM 不清堆，无此步驻留读数含跨档残留不可互比）
      await cdp.send('HeapProfiler.collectGarbage')
      const before = (await cdp.send('Performance.getMetrics')).metrics
      const heapBefore = before.find(m => m.name === 'JSHeapUsedSize')?.value ?? 0
      const { nodesBefore } = await page.evaluate(() => ({
        nodesBefore: document.querySelectorAll('*').length,
      }))

      // 选中工作目录 → 根层 loadTreeNode → N 个文件节点全量渲染
      const t0 = Date.now()
      await page.click(dirItemSel(fixture.name))
      await page.waitForFunction(
        n => document.querySelectorAll('.el-tree-node').length >= n,
        fixture.files,
        { timeout: RENDER_TIMEOUT_MS, polling: 100 },
      )
      const renderMs = Date.now() - t0

      await cdp.send('HeapProfiler.collectGarbage') // 清渲染中间对象，保留树节点驻留
      const after = (await cdp.send('Performance.getMetrics')).metrics
      const heapAfter = after.find(m => m.name === 'JSHeapUsedSize')?.value ?? 0
      // DOM 计数用页内选择器（实测 CDP Performance.getMetrics 的 Nodes 指标在
      // headless 动态插入场景不刷新，+0 失真；querySelectorAll('*') 为真实全页计数）
      const { nodesAfter, pageHeap } = await page.evaluate(() => ({
        nodesAfter: document.querySelectorAll('*').length,
        pageHeap: performance.memory?.usedJSHeapSize ?? 0,
      }))

      const row = {
        轮次: label,
        规模: fixture.files,
        渲染ms: renderMs,
        JSHeap增量MB: +((heapAfter - heapBefore) / 1048576).toFixed(1),
        JSHeap驻留MB: +((heapAfter) / 1048576).toFixed(1),
        页内HeapMB: +(pageHeap / 1048576).toFixed(1),
        DOM节点增量: nodesAfter - nodesBefore,
      }
      console.log(`[${fixture.files} 文件] 第 ${label} 轮：渲染 ${renderMs} ms / DOM 节点 +${row.DOM节点增量} / JSHeap +${row.JSHeap增量MB} MB（驻留 ${row.JSHeap驻留MB} MB）`)
      return row
    } finally {
      await context.close()
    }
  }

  const allRows = []
  for (const fixture of fixtureDirs.filter(f => f.files > 0)) {
    console.log(`--- 档位：单目录 ${fixture.files} 文件 ---`)
    console.log('热身轮（不计入统计）...')
    await measureOnce(fixture, 'W')

    const rows = []
    for (let i = 1; i <= RUNS; i++) rows.push(await measureOnce(fixture, i))
    allRows.push(...rows)

    console.log(`[${fixture.files} 文件] 中位数：渲染 ${median(rows.map(r => r.渲染ms))} ms / DOM 节点 +${median(rows.map(r => r.DOM节点增量))} / JSHeap +${median(rows.map(r => r.JSHeap增量MB))} MB / 驻留 ${median(rows.map(r => r.JSHeap驻留MB))} MB\n`)
  }

  console.log('=== 全部档位逐轮明细 ===')
  console.table(allRows)
} finally {
  if (browser) await browser.close()
  // 先 kill 再等 exit：Windows 句柄释放异步，确认退出后才 rm（否则 EPERM）
  serve.kill()
  await new Promise(resolve => {
    if (serve.exitCode != null) return resolve()
    const t = setTimeout(resolve, 5000)
    serve.once('exit', () => { clearTimeout(t); resolve() })
  })
  console.log('清理隔离工作区（100k 文件删除可能耗时数十秒）...')
  cleanupWorkspace()
}
console.log('\n口径说明：独立 context = 冷缓存；浏览器进程复用 = V8 暖。')
console.log('JSHeap 增量 = 渲染后 - 空树基线（CDP JSHeapUsedSize，采样前强制 GC）；DOM 节点为页内 querySelectorAll 全页计数。')
console.log('渲染耗时含 RPC（GetFileTree 单层）+ el-tree 全量节点创建，无虚拟滚动。')
