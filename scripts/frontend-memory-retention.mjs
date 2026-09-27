// WorkBench 前端内存驻留面测量脚本（perf-baseline.md §16，09-27）
//
// 测什么：组件「不再使用后」的 JSHeap 驻留残留——与 §15 渲染面（峰值增量）互补，
// 回答路线图「及时释放不再使用的对象」子项是否需要实施。测两面板：
//   面一 CommitHistory 切仓库释放：选仓库 A → 加载提交历史 → 切回仓库 B →
//        残留 = 切走后驻留 - 历史加载后驻留（验证既有 watch 重置生效，预期 ≈0）
//   面二 FileDiffDialog 关闭释放：打开单文件大 diff → 关闭对话框 →
//        残留 = 关闭后驻留 - 打开前驻留（关闭后 left/right 行数组若无清理则残留）
// 其余候选面（AiFunctionPanel tasks / aiChat 消息 / ContentPanel 预览）依赖
// claude CLI 真实任务驱动或属「使用中数据」，走代码审计定性（见 §16 分流表），
// 不在本脚本范围。
//
// 口径（对齐 §15.1）：每轮独立 context = 冷缓存，浏览器进程复用 = V8 暖，3 轮中位；
// 采样前 CDP HeapProfiler.collectGarbage 强制 GC；CDP Performance.getMetrics JSHeapUsedSize。
//
// 隔离：serve 进程 cwd 指向临时目录，data/ 与 git fixture 全落临时目录，
// 用户真实配置零污染。git fixture 全命令显式 -c core.autocrlf=false / user.name /
// user.email，不依赖环境全局 git 配置。
//
// 运行：node scripts/frontend-memory-retention.mjs [--runs 3] [--port 36123]
//       [--keep-fixture] [--commits 100] [--diff-lines 4000]
// 依赖：build/bin/workbench.exe（--serve 模式）；frontend/node_modules（playwright）；git 在 PATH。

import { spawn, execFileSync } from 'child_process'
import net from 'net'
import { readFileSync, existsSync, mkdtempSync, writeFileSync, mkdirSync, rmSync } from 'fs'
import os from 'os'
import path from 'path'
import { fileURLToPath } from 'url'
import { createRequire } from 'module'

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const require_ = createRequire(import.meta.url)
const { chromium } = require_(path.join(repoRoot, 'frontend/node_modules/playwright'))

// --- 参数解析（与 frontend-memory-tree.mjs 同风格） ---
const args = process.argv.slice(2)
const argOf = (name, def) => {
  const i = args.indexOf(name)
  return i >= 0 && args[i + 1] ? args[i + 1] : def
}
const hasFlag = name => args.includes(name)
const RUNS = parseInt(argOf('--runs', '3'), 10)
const PORT = parseInt(argOf('--port', '36123'), 10)
const COMMIT_COUNT = parseInt(argOf('--commits', '100'), 10)
const DIFF_LINES = parseInt(argOf('--diff-lines', '4000'), 10)
const KEEP_FIXTURE = hasFlag('--keep-fixture')
for (const [k, v] of [['--runs', RUNS], ['--port', PORT], ['--commits', COMMIT_COUNT], ['--diff-lines', DIFF_LINES]]) {
  if (!Number.isFinite(v) || v <= 0) {
    console.error(`参数 ${k} 非法: 须为正整数`)
    process.exit(1)
  }
}

const EXE = path.join(repoRoot, 'build/bin/workbench.exe')
if (!existsSync(EXE)) {
  console.error(`未找到 ${EXE}，请先执行 wails build`)
  process.exit(1)
}

// --- git fixture：repoA（N 空提交 + 大 diff 工作区） / repoB（切换归零目标） ---
const workspace = mkdtempSync(path.join(os.tmpdir(), 'wb-retention-'))
const dataDir = path.join(workspace, 'data')
mkdirSync(dataDir)

const GIT = (cwd, ...argv) => execFileSync('git', ['-C', cwd, '-c', 'core.autocrlf=false', '-c', 'user.name=measure', '-c', 'user.email=measure@local', ...argv], { stdio: 'ignore' })

function buildRepoA() {
  const dir = path.join(workspace, 'repo-a')
  mkdirSync(dir)
  GIT(dir, 'init', '-b', 'master')
  // 大文本文件：每行 ~200 字符伪随机文本（防 V8 字符串去重），后改前 DIFF_LINES 行
  const lineText = i => `line${String(i).padStart(5, '0')}-${'x'.repeat(160)}-${(i * 2654435761) % 99991}`
  const origLines = Array.from({ length: 8000 }, (_, i) => lineText(i))
  writeFileSync(path.join(dir, 'big.txt'), origLines.join('\n') + '\n')
  GIT(dir, 'add', '.')
  GIT(dir, 'commit', '-m', 'initial')
  for (let i = 0; i < COMMIT_COUNT; i++) {
    GIT(dir, 'commit', '--allow-empty', '-m', `commit ${i}`)
  }
  // 工作区改动：前 DIFF_LINES 行替换为另一组伪随机文本 → GetFileDiff 双栏各 ~DIFF_LINES 行
  const modLines = origLines.slice()
  for (let i = 0; i < DIFF_LINES; i++) {
    modLines[i] = `CHANGED${String(i).padStart(5, '0')}-${'y'.repeat(160)}-${(i * 40503) % 99991}`
  }
  writeFileSync(path.join(dir, 'big.txt'), modLines.join('\n') + '\n')
  return dir
}

function buildRepoB() {
  const dir = path.join(workspace, 'repo-b')
  mkdirSync(dir)
  GIT(dir, 'init', '-b', 'master')
  writeFileSync(path.join(dir, 'readme.md'), 'switch target\n')
  GIT(dir, 'add', '.')
  GIT(dir, 'commit', '-m', 'initial')
  return dir
}

console.log(`=== WorkBench 前端内存驻留面测量（浏览器通道，${RUNS} 轮/面）===`)
console.log(`隔离工作区: ${workspace}`)
const t0 = Date.now()
const REPO_B_COMMITS = 1
let repoA, repoB
try {
  repoA = buildRepoA()
  repoB = buildRepoB()
} catch (e) {
  console.error(`git fixture 构造失败（git 须在 PATH）: ${e.message}`)
  cleanupWorkspace()
  process.exit(1)
}
console.log(`git fixture 构造完成: ${((Date.now() - t0) / 1000).toFixed(1)} s（repoA ${COMMIT_COUNT} 提交 + ${DIFF_LINES} 行工作区 diff；不计入测量口径）\n`)

// --- 启动 serve（cwd = 隔离工作区） ---
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

const serve = spawn(EXE, ['--serve', `--listen=127.0.0.1:${PORT}`], { cwd: workspace, stdio: 'ignore' })
serve.on('error', err => {
  console.error(`serve 进程拉起失败: ${err.message}`)
  cleanupWorkspace()
  process.exit(1)
})

const BASE = `http://127.0.0.1:${PORT}`
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

function cleanupWorkspace() {
  if (KEEP_FIXTURE) {
    console.log(`--keep-fixture：保留工作区 ${workspace}`)
    return
  }
  try { rmSync(workspace, { recursive: true, force: true, maxRetries: 3, retryDelay: 200 }) } catch (e) {
    console.error(`清理失败（可手动删除）: ${workspace}: ${e.message}`)
  }
}

// 等 serve 就绪读隔离 token
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
  await killServeAndExit(serve)
}

let browser = null
try {
  browser = await chromium.launch()
  const median = arr => {
    const s = [...arr].sort((a, b) => a - b)
    return s[Math.floor((s.length - 1) / 2)]
  }

  // 注册两个工作目录（一次）：首个注册 repoB 会被自动选中，成为每轮归零起点
  {
    const context = await browser.newContext()
    try {
      const page = await context.newPage()
      await page.goto(`${BASE}/?token=${encodeURIComponent(token)}`, { waitUntil: 'load' })
      await page.waitForSelector('.directory-tree-panel', { state: 'visible', timeout: 30000 })
      for (const f of [
        { name: 'retention-b', dir: repoB },
        { name: 'retention-a', dir: repoA },
      ]) {
        const added = await page.evaluate(async ({ name, dir }) => {
          const d = await window.go.main.App.AddDirectory(name, dir, false)
          return d != null
        }, f)
        if (!added) throw new Error(`AddDirectory(${f.name}) 失败`)
      }
      await page.reload({ waitUntil: 'load' })
      await page.waitForSelector('.dir-item', { state: 'visible', timeout: 30000 })
    } finally {
      await context.close()
    }
  }

  const dirItemSel = name => `.dir-item:has(.dir-item-name:text-is("${name}"))`

  // 选目录并等右侧 git-tabs 出现（选中 git 仓库节点直显仓库详情）
  async function selectRepo(page, name) {
    await page.click(dirItemSel(name))
    await page.waitForSelector('.git-tabs', { state: 'visible', timeout: 30000 })
  }

  // GC + 读 JSHeapUsedSize
  async function heapNow(cdp, page) {
    await cdp.send('HeapProfiler.collectGarbage')
    const { metrics } = await cdp.send('Performance.getMetrics')
    return metrics.find(m => m.name === 'JSHeapUsedSize')?.value ?? 0
  }

  async function measureOnce(label) {
    const context = await browser.newContext()
    try {
      const page = await context.newPage()
      await page.goto(`${BASE}/?token=${encodeURIComponent(token)}`, { waitUntil: 'load' })
      await page.waitForSelector('.dir-item', { state: 'visible', timeout: 30000 })
      const cdp = await context.newCDPSession(page)
      await cdp.send('Performance.enable')

      // 归零：选 repoB，等提交历史 tab 面板就绪
      await selectRepo(page, 'retention-b')
      const heap0 = await heapNow(cdp, page)

      // ---- 面一：CommitHistory 加载 → 切走残留 ----
      await selectRepo(page, 'retention-a')
      await page.locator('.el-tabs__item', { hasText: '提交历史' }).click()
      // 初始分页 PAGE_SIZE=20，点「加载更多」补足到 COMMIT_COUNT 条（测批量历史驻留）
      await page.waitForSelector('.commit-card', { timeout: 60000 })
      while (await page.locator('.commit-card').count() < COMMIT_COUNT) {
        const btn = page.locator('button.load-more, .load-more button').first()
        if (!(await btn.isVisible().catch(() => false))) break
        const before = await page.locator('.commit-card').count()
        await btn.click()
        await page.waitForFunction(
          n => document.querySelectorAll('.commit-card').length > n,
          before,
          { timeout: 60000, polling: 200 },
        )
      }
      const heap1 = await heapNow(cdp, page) // 历史加载后（使用中峰值）

      await selectRepo(page, 'retention-b')
      // 等 A 历史被 repoPath watch 重置并载入 B 的历史（提交数回落至
      // REPO_B_COMMITS；切目录后 activeGitTab 保留在提交历史页）
      await page.waitForFunction(
        n => document.querySelectorAll('.commit-card').length < n + 2,
        REPO_B_COMMITS,
        { timeout: 30000, polling: 200 },
      )
      const heap2 = await heapNow(cdp, page) // 切走后

      // ---- 面二：FileDiffDialog 打开 → 关闭残留 ----
      // 注意基线时点：重选 A 后 CommitHistory（lazy tab 保持挂载）重载 20 条 +
      // 本地变动表格加载的驻留均已发生，须在 dblclick 前采样，否则混入关闭残留
      await selectRepo(page, 'retention-a')
      await page.locator('.el-tabs__item', { hasText: '本地变动' }).click()
      await page.waitForSelector('.el-table__row', { state: 'visible', timeout: 30000 })
      const heapPreOpen = await heapNow(cdp, page) // diff 打开前基线（含切回 A 的重载驻留）

      await page.locator('.el-table__row').first().dblclick()
      await page.waitForFunction(
        n => document.querySelectorAll('.diff-line').length >= n,
        DIFF_LINES,
        { timeout: 60000, polling: 200 },
      )
      const heap3 = await heapNow(cdp, page) // diff 打开态（使用中峰值）

      await page.locator('.el-dialog__footer button', { hasText: '关闭' }).click()
      await page.waitForSelector('.diff-dialog-title', { state: 'hidden', timeout: 30000 })
      const heap4 = await heapNow(cdp, page) // 关闭后

      const row = {
        轮次: label,
        历史加载MB: +((heap1 - heap0) / 1048576).toFixed(1),
        切走残留MB: +((heap2 - heap1) / 1048576).toFixed(1),
        Diff打开MB: +((heap3 - heapPreOpen) / 1048576).toFixed(1),
        关闭残留MB: +((heap4 - heapPreOpen) / 1048576).toFixed(1),
      }
      console.log(`[${label} 轮] 历史 +${row.历史加载MB}MB → 切走残留 ${row.切走残留MB}MB ｜ Diff +${row.Diff打开MB}MB → 关闭残留 ${row.关闭残留MB}MB`)
      return row
    } finally {
      await context.close()
    }
  }

  console.log('热身轮（不计入统计）...')
  await measureOnce('W')
  const rows = []
  for (let i = 1; i <= RUNS; i++) rows.push(await measureOnce(i))

  console.log('\n=== 中位数（GC 口径）===')
  console.table([{
    轮次: 'median',
    历史加载MB: median(rows.map(r => r.历史加载MB)),
    切走残留MB: median(rows.map(r => r.切走残留MB)),
    Diff打开MB: median(rows.map(r => r.Diff打开MB)),
    关闭残留MB: median(rows.map(r => r.关闭残留MB)),
  }])
  console.log('切走残留 ≈0 = CommitHistory 切仓库重置生效；关闭残留 >0 = FileDiffDialog 关闭后行数组驻留（实施目标面）。')
} finally {
  if (browser) await browser.close()
  serve.kill()
  await new Promise(resolve => {
    if (serve.exitCode != null) return resolve()
    const t = setTimeout(resolve, 5000)
    serve.once('exit', () => { clearTimeout(t); resolve() })
  })
  console.log('清理隔离工作区...')
  cleanupWorkspace()
}
