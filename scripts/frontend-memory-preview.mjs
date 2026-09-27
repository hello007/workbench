// WorkBench 前端大文件预览传输-内存测量脚本（perf-baseline.md §15 收口，2026-09-27）
//
// 测什么：ReadFileBytes（50MB 上限，base64 全量过 Wails 桥）在大文件下的
// 传输体积膨胀系数与前端 JSHeap 峰值，验证 TooLarge 阈值是否合理。
//
// 口径修正（相对任务 PRD 假设）：真实触发 ReadFileBytes 的 kind 是 image/office
// （ContentPanel.vue `needsBytes` 判定）；文本/markdown 走 PreviewFile 1MB 通道，
// 不进本链路。故 fixture 用 .png（image kind）。PDF 走 iframe + Range 流式（无上限），
// 亦不在本链路。
//
// 测量方式：serve 进程 cwd 指向隔离临时目录（data/ 零污染），Playwright 打开页面后
// page.evaluate 直调 window.go.main.App.ReadFileBytes（浏览器通道 RPC，与桌面同一
// Go 链路），CDP Performance.getMetrics 双驻留点采样：
//   驻留点 1 = RPC 返回后（base64 字符串驻留页面堆，window 引用保持防 GC）
//   驻留点 2 = decode 后（atob -> Uint8Array，与 FilePreviewRenderer
//        base64ToUint8Array 同实现；image 真实路径后续 dataURL + <img> 位图由
//        合成器持有不在 JSHeap，文档定性说明）
// performance.memory 同执行流内读数不刷新（实测恒 0 增量）弃用；CDP getMetrics
// 调用间隙采样可靠；基线与驻留点采样前经 HeapProfiler.collectGarbage 强制 GC，
// 消除跨档残留对驻留读数的污染。
//
// 档位：1 / 4 / 16 / 48 / 64 MB。48MB 为 50MB 上限内边界档（base64 ~64MB 传输成功），
// 64MB 超限验证 TooLarge 分支（后端拒绝读，无 base64 返回）。
//
// 运行：node scripts/frontend-memory-preview.mjs [--runs 3] [--port 36125]
// 依赖：build/bin/workbench.exe（--serve 模式）；frontend/node_modules（playwright）。
// 只测量不改生产代码。

import { spawn } from 'child_process'
import net from 'net'
import { readFileSync, existsSync, mkdtempSync, writeFileSync, mkdirSync, rmSync } from 'fs'
import os from 'os'
import path from 'path'
import { fileURLToPath } from 'url'
import { createRequire } from 'module'
import { randomBytes } from 'crypto'

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const require_ = createRequire(import.meta.url)
const { chromium } = require_(path.join(repoRoot, 'frontend/node_modules/playwright'))

const args = process.argv.slice(2)
const argOf = (name, def) => {
  const i = args.indexOf(name)
  return i >= 0 && args[i + 1] ? args[i + 1] : def
}
const RUNS = parseInt(argOf('--runs', '3'), 10)
const PORT = parseInt(argOf('--port', '36125'), 10)
for (const [k, v] of [['--runs', RUNS], ['--port', PORT]]) {
  if (!Number.isFinite(v) || v <= 0) {
    console.error(`参数 ${k} 非法: 须为正整数`)
    process.exit(1)
  }
}

// 档位（MB）：48MB 为 50MB 上限内边界档，64MB 超限验证 TooLarge 分支
const SIZES_MB = [1, 4, 16, 48, 64]

const EXE = path.join(repoRoot, 'build/bin/workbench.exe')
if (!existsSync(EXE)) {
  console.error(`未找到 ${EXE}，请先执行 wails build`)
  process.exit(1)
}

const workspace = mkdtempSync(path.join(os.tmpdir(), 'wb-preview-mem-'))
mkdirSync(path.join(workspace, 'data'))
console.log(`=== WorkBench 前端大文件预览传输-内存测量（浏览器通道，${RUNS} 轮/档）===`)
console.log(`隔离工作区: ${workspace}`)

// fixture：每档一个 .png（image kind），内容伪随机（避免 V8 对重复模式字符串的
// 压缩/去重优化扭曲 heap 读数）
const fixtures = []
for (const mb of SIZES_MB) {
  const p = path.join(workspace, `sample-${mb}mb.png`)
  const buf = randomBytes(mb * 1024 * 1024)
  writeFileSync(p, buf)
  fixtures.push({ mb, path: p, bytes: buf.length })
  console.log(`fixture ${mb} MB 构造完成: sample-${mb}mb.png`)
}
console.log()

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
  cwd: workspace,
  stdio: 'ignore',
})
// 统一早退路径：kill 后等 exit 确认（Windows 句柄释放异步，立即退留孤儿占端口），
// 再清理隔离工作区（fixture 5 个伪随机文件共 133MB 残留 tmpdir）
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
  try { rmSync(workspace, { recursive: true, force: true, maxRetries: 3, retryDelay: 200 }) } catch (e) {
    console.error(`清理失败（可手动删除）: ${workspace}: ${e.message}`)
  }
}
serve.on('error', err => {
  console.error(`serve 进程拉起失败: ${err.message}`)
  cleanupWorkspace() // spawn 阶段失败：browser/测量未启动，仅工作区须清理
  process.exit(1)
})

const BASE = `http://127.0.0.1:${PORT}`
const deadline = Date.now() + 30000
let token = ''
let ready = false
while (Date.now() < deadline) {
  const tokenFile = path.join(workspace, 'data', 'web_token')
  if (existsSync(tokenFile)) {
    try {
      token = readFileSync(tokenFile, 'utf8').trim()
      const res = await fetch(`${BASE}/?token=${encodeURIComponent(token)}`, { redirect: 'manual' })
      if (res.status < 500) { ready = true; break }
    } catch { }
  }
  await new Promise(r => setTimeout(r, 50))
}
if (!ready) {
  console.error('serve 进程 30s 未就绪，退出')
  killServeAndExit(serve)
}

// launch 在 try 内：launch 失败（浏览器缺失/版本不匹配）时 finally 兜底杀 serve，
// 否则 Windows 不级联杀子进程，孤儿占端口致下次运行预检退出
let browser = null
try {
  browser = await chromium.launch()
  const median = arr => {
    const s = [...arr].sort((a, b) => a - b)
    return s[Math.floor((s.length - 1) / 2)]
  }

  async function measureOnce(fixture, label) {
    const context = await browser.newContext() // 独立 context = 冷缓存
    try {
      const page = await context.newPage()
      await page.goto(`${BASE}/?token=${encodeURIComponent(token)}`, { waitUntil: 'load' })
      await page.waitForSelector('.directory-tree-panel', { state: 'visible', timeout: 30000 })

      // CDP 会话：JSHeapUsedSize（performance.memory 在同一执行流内读数不刷新，
      // 分步插桩失真；CDP getMetrics 在调用间隙采样可靠，见曲线一同款）
      const cdp = await context.newCDPSession(page)
      await cdp.send('Performance.enable')
      // 采样前强制 GC：消除上一档/上一轮残留对基线与驻留读数的污染
      await cdp.send('HeapProfiler.collectGarbage')
      const heapBefore = (await cdp.send('Performance.getMetrics')).metrics
        .find(m => m.name === 'JSHeapUsedSize')?.value ?? 0

      // 驻留点 1：RPC 返回驻留页面堆（window 引用保持，防 GC）
      const r = await page.evaluate(async (filePath) => {
        let res
        try {
          // App 层 ReadFileBytes 单参（50MB 上限硬编码于 app_preview.go）
          res = await window.go.main.App.ReadFileBytes(filePath)
        } catch (e) {
          return { rpcError: (e && (e.code || e.message)) || String(e) }
        }
        if (res.error) return { rpcError: `后端返回 error: ${res.error}` }
        window.__wbMem = res // 保持 base64 字符串驻留
        return { tooLarge: !!res.tooLarge, base64Len: res.base64 ? res.base64.length : 0 }
      }, fixture.path)

      if (r.rpcError) throw new Error(`[${fixture.mb} MB] RPC 调用失败: ${r.rpcError}`)
      await cdp.send('HeapProfiler.collectGarbage') // 清 RPC 调用中间对象，保留 window 引用的 base64
      const afterRpc = (await cdp.send('Performance.getMetrics')).metrics
        .find(m => m.name === 'JSHeapUsedSize')?.value ?? 0

      // 驻留点 2：decode（atob -> Uint8Array，与 FilePreviewRenderer base64ToUint8Array 同实现）
      let bytesLen = 0
      if (!r.tooLarge) {
        bytesLen = await page.evaluate(() => {
          const binary = atob(window.__wbMem.base64)
          const bytes = new Uint8Array(binary.length)
          for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
          window.__wbDecoded = bytes
          return bytes.length
        })
      }
      const afterDecode = (await cdp.send('Performance.getMetrics')).metrics
        .find(m => m.name === 'JSHeapUsedSize')?.value ?? 0

      // 清理驻留引用
      await page.evaluate(() => { delete window.__wbMem; delete window.__wbDecoded })

      const peak = Math.max(afterRpc, afterDecode)
      const result = {
        轮次: label,
        档位MB: fixture.mb,
        tooLarge: r.tooLarge,
        base64传输MB: r.tooLarge ? 0 : +(r.base64Len / 1048576).toFixed(1),
        膨胀系数: r.tooLarge ? 0 : +(r.base64Len / fixture.bytes).toFixed(3),
        RPC驻留增量MB: +((afterRpc - heapBefore) / 1048576).toFixed(1),
        decode驻留增量MB: r.tooLarge ? 0 : +((afterDecode - heapBefore) / 1048576).toFixed(1),
        峰值增量MB: +((peak - heapBefore) / 1048576).toFixed(1),
      }
      if (!r.tooLarge && bytesLen !== fixture.bytes) {
        throw new Error(`decode 字节数不匹配: ${bytesLen} != ${fixture.bytes}`)
      }
      console.log(`[${fixture.mb} MB] 第 ${label} 轮：${r.tooLarge
        ? `TooLarge=true（超 50MB 上限被拒，无 base64 返回）`
        : `base64 ${result.base64传输MB} MB（膨胀 ${result.膨胀系数}x）/ RPC 驻留 +${result.RPC驻留增量MB} MB / decode 驻留 +${result.decode驻留增量MB} MB`}`)
      return result
    } finally {
      await context.close()
    }
  }

  const allRows = []
  for (const fixture of fixtures) {
    console.log(`--- 档位：${fixture.mb} MB ---`)
    await measureOnce(fixture, 'W') // 热身
    const rows = []
    for (let i = 1; i <= RUNS; i++) rows.push(await measureOnce(fixture, i))
    allRows.push(...rows)
    const peakMed = median(rows.map(r => r.峰值增量MB))
    const b64Med = median(rows.map(r => r.base64传输MB))
    console.log(`[${fixture.mb} MB] 中位数：${rows[0].tooLarge ? 'TooLarge 分支' : `base64 ${b64Med} MB / 峰值驻留 +${peakMed} MB`}\n`)
  }

  console.log('=== 全部档位逐轮明细 ===')
  console.table(allRows)
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
console.log('\n口径说明：RPC 驻留 = base64 字符串驻留页面堆增量；decode 驻留 = 叠加 atob 中间串 +')
console.log('Uint8Array 后增量（CDP JSHeapUsedSize 调用间隙采样）。image 真实路径后续 dataURL 拼接')
console.log('+ <img> 位图（合成器持有）未计入 JSHeap。传输体积为页面堆内 base64 字符串长度。')
