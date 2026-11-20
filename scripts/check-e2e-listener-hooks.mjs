// E2E window 级监听覆写 hook 静态守护（防 docs/spec/e2e-testing.md §12 事故回流，任务 11-20-e2e-window）。
//
// 背景：终端全屏 ESC 用例曾内联覆写 window.addEventListener 计数并断言绝对基线 toBe(0)，
// 新增合法 window 注册点（FolderTree 菜单监听）即误报挂用例。治理策略：计数统一走
// frontend/e2e/listenerCounter.js 的相对增量 helper，spec 内自写覆写 hook 由本脚本拦截。
//
// 禁令形态（逐行正则，任一命中即失败）：
//   - window.addEventListener =            （覆写）
//   - window.removeEventListener =         （覆写）
//   - addEventListener.bind(window)        （绕过覆写检测的等价形态）
//   - removeEventListener.bind(window)
//   - EventTarget.prototype.addEventListener（更底层覆写）
//   - globalThis / self.addEventListener =（window 的别名形态，覆写同样生效）
//   - Window.prototype.addEventListener    （原型级覆写）
// 唯一放行点：扫描根下的 listenerCounter.js（helper 本体，其内部必须自行包装原生方法；
// 嵌套目录同名文件不放行）。
//
// 边界声明（有意不覆盖，命中需人工判断）：
//   - 逐行正则匹配，不剥离注释与字符串字面量——代码注释或字符串里出现上述禁令字样
//     同样会被报为违规（宁误报不放过，文案中引用禁令写法时改述）
//   - 跨行赋值（`window.addEventListener` 换行后再写 `=`）不检测
//   - 扫描扩展名覆盖 .js/.jsx/.ts/.tsx/.mjs/.cjs/.mts/.cts，无扩展名或其他语言不检测
//
// 用法：
//   node scripts/check-e2e-listener-hooks.mjs            # 扫描 frontend/e2e（仓内默认）
//   node scripts/check-e2e-listener-hooks.mjs <dir>      # 扫描指定目录（selftest 内部用）
//   node scripts/check-e2e-listener-hooks.mjs --selftest # 注入违规/合规 fixture 自证检查器有效
//
// 退出码：0 = 全部通过；1 = 存在违规（或扫描根不存在 / selftest 失败）。CI 在
// 「前端依赖分档一致性检查」步骤之后立跑（纯读脚本文本，零依赖零网络）。

import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

// ── 禁令清单（name 用于报错定位，re 逐行匹配）──

// 赋值形态用 `\s*(?<![=!<>])=(?!=)`：前后同时排除比较符——lookbehind 落在空白之后
// 判定，`window.addEventListener = fn` 仍命中；`==`/`===`/`!=` 比较表达式
// （如 `window.addEventListener == fn`，裸 `=(?!=)` 会在第二个 `=` 处误报）不命中
const BANNED_PATTERNS = [
  { name: 'window.addEventListener 覆写', re: /window\.addEventListener\s*(?<![=!<>])=(?!=)/ },
  { name: 'window.removeEventListener 覆写', re: /window\.removeEventListener\s*(?<![=!<>])=(?!=)/ },
  { name: 'addEventListener.bind(window)', re: /addEventListener\.bind\(\s*window\s*\)/ },
  { name: 'removeEventListener.bind(window)', re: /removeEventListener\.bind\(\s*window\s*\)/ },
  { name: 'EventTarget.prototype.addEventListener 覆写', re: /EventTarget\.prototype\.addEventListener/ },
  {
    name: 'globalThis / self.addEventListener 覆写',
    re: /(?:globalThis|self)\.(?:add|remove)EventListener\s*(?<![=!<>])=(?!=)/,
  },
  { name: 'Window.prototype.addEventListener 覆写', re: /Window\.prototype\.(?:add|remove)EventListener/ },
];

const ALLOWED_BASENAME = 'listenerCounter.js';

// 扫描扩展名：js / jsx / ts / tsx / mjs / cjs / mts / cts（E2E 目录将来引入 TS 时无需改脚本）
const SCRIPT_FILE_RE = /\.[cm]?[jt]sx?$/;

// 放行判定按「扫描根下的 listenerCounter.js」精确路径匹配：嵌套目录里同名文件
// 不是 helper 本体，仍按 spec 校验（防通过新建同名文件绕过守护）
function isAllowedHelper(root, file) {
  return path.resolve(file) === path.resolve(root, ALLOWED_BASENAME);
}

// ── 文件收集：递归脚本文件（见 SCRIPT_FILE_RE），跳过 helper 本体 ──

function collectScriptFiles(root, out = [], dir = root) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      collectScriptFiles(root, out, full);
    } else if (entry.isFile() && SCRIPT_FILE_RE.test(entry.name) && !isAllowedHelper(root, full)) {
      out.push(full);
    }
  }
  return out;
}

// ── 核心检查：返回违规描述数组（空数组 = 通过）──
// 注：本脚本是 CLI（顶层即执行），不导出函数——避免被 import 时产生「导入即扫描/退出」的副作用

function runCheck(rootDir) {
  const violations = [];
  for (const file of collectScriptFiles(rootDir)) {
    const rel = path.relative(rootDir, file).split(path.sep).join('/');
    const lines = fs.readFileSync(file, 'utf8').split(/\r?\n/);
    lines.forEach((line, index) => {
      for (const { name, re } of BANNED_PATTERNS) {
        if (re.test(line)) {
          violations.push(`${rel}:${index + 1}: ${name} — ${line.trim()}`);
          break; // 同一行只报一次
        }
      }
    });
  }
  return violations;
}

// ── selftest：临时 fixture 注入各违规形态断言必拦 / 合规形态断言必过 ──

function writeFixture(caseDir, files) {
  for (const [rel, content] of Object.entries(files)) {
    const full = path.join(caseDir, rel);
    fs.mkdirSync(path.dirname(full), { recursive: true });
    fs.writeFileSync(full, content);
  }
}

function runSelftest() {
  const cases = [
    {
      desc: 'window.addEventListener 覆写被拦',
      files: { 'bad.js': 'window.addEventListener = function (type, fn) { return fn }\n' },
      expect: 'fail',
    },
    {
      desc: 'window.removeEventListener 覆写被拦',
      files: { 'bad.js': 'window.removeEventListener = function (type, fn) { return fn }\n' },
      expect: 'fail',
    },
    {
      desc: 'addEventListener.bind(window) 被拦',
      files: { 'bad.js': 'const add = window.addEventListener.bind(window)\n' },
      expect: 'fail',
    },
    {
      desc: 'removeEventListener.bind(window) 被拦',
      files: { 'bad.js': 'const remove = window.removeEventListener.bind(window)\n' },
      expect: 'fail',
    },
    {
      desc: 'EventTarget.prototype.addEventListener 覆写被拦（嵌套目录）',
      files: { 'nested/bad.js': 'EventTarget.prototype.addEventListener = function () {}\n' },
      expect: 'fail',
    },
    {
      desc: 'globalThis.addEventListener 覆写被拦',
      files: { 'bad.js': 'globalThis.addEventListener = function (type, fn) { return fn }\n' },
      expect: 'fail',
    },
    {
      desc: 'self.removeEventListener 覆写被拦',
      files: { 'bad.js': 'self.removeEventListener = function (type, fn) { return fn }\n' },
      expect: 'fail',
    },
    {
      desc: 'Window.prototype.addEventListener 覆写被拦',
      files: { 'bad.js': 'Window.prototype.addEventListener = function () {}\n' },
      expect: 'fail',
    },
    {
      desc: 'TypeScript 文件同样被扫描',
      files: { 'bad.ts': 'window.addEventListener = function (type, fn) { return fn }\n' },
      expect: 'fail',
    },
    {
      desc: '普通 window.addEventListener 调用放行（不绑定不覆写）',
      files: {
        'ok.js': "window.addEventListener('keydown', () => {})\nwindow.removeEventListener('keydown', () => {})\n",
      },
      expect: 'pass',
    },
    {
      desc: 'helper 本体（listenerCounter.js）为唯一放行点',
      files: { 'listenerCounter.js': 'window.addEventListener = function (type, fn) { return fn }\n' },
      expect: 'pass',
    },
    {
      desc: '嵌套目录同名 listenerCounter.js 不放行（仅扫描根 helper 本体）',
      files: { 'nested/listenerCounter.js': 'window.addEventListener = function (type, fn) { return fn }\n' },
      expect: 'fail',
    },
    {
      desc: '比较表达式 window.addEventListener === fn 不误判为覆写',
      files: { 'ok.js': 'if (window.addEventListener === fn) {}\n' },
      expect: 'pass',
    },
    {
      desc: '比较表达式 window.addEventListener == fn 不误判为覆写',
      files: {
        'ok.js': 'if (window.addEventListener == fn) {}\nif (window.removeEventListener == fn) {}\n',
      },
      expect: 'pass',
    },
    {
      desc: '违规定位精确到 file:line',
      files: { 'ok.js': '// 合规注释\n', 'bad.js': 'const x = 1\nwindow.addEventListener = function () {}\n' },
      expect: 'fail',
      expectHit: 'bad.js:2:',
    },
  ];

  let failed = 0;
  for (const c of cases) {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'e2e-listener-hooks-selftest-'));
    let violations = [];
    try {
      writeFixture(dir, c.files);
      violations = runCheck(dir);
    } finally {
      fs.rmSync(dir, { recursive: true, force: true });
    }
    const pass = violations.length === 0;
    const ok = c.expect === 'pass' ? pass : !pass;
    const detailOk = !c.expectHit || violations.some((v) => v.includes(c.expectHit));
    if (!ok || !detailOk) {
      failed++;
      console.error(`selftest FAIL: ${c.desc}`);
      if (!ok) {
        console.error(`  期望 ${c.expect}，实际 ${pass ? 'pass' : 'fail'}${violations.length ? '：' + violations.join('；') : ''}`);
      }
      if (!detailOk) {
        console.error(`  期望命中 ${c.expectHit}，实际：${violations.length ? violations.join('；') : '无违规'}`);
      }
    } else {
      console.log(`selftest ok: ${c.desc}`);
    }
  }

  if (failed > 0) {
    console.error(`\nselftest 失败 ${failed}/${cases.length}：检查器判定逻辑与预期不符，禁止上线`);
    return false;
  }
  console.log(`\nselftest 全部通过（${cases.length} 用例）`);
  return true;
}

// ── 入口 ──

const args = process.argv.slice(2);
if (args.includes('--selftest')) {
  process.exit(runSelftest() ? 0 : 1);
}

// 默认目标 = 仓库内 frontend/e2e/（基于脚本自身位置解析，任意 cwd 调用均正确；CI 与本机一致）
const defaultDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'frontend', 'e2e');
const targetDir = path.resolve(args.find((a) => !a.startsWith('--')) || defaultDir);

if (!fs.existsSync(targetDir) || !fs.statSync(targetDir).isDirectory()) {
  console.error(`E2E 监听 hook 检查失败：扫描根不存在或不是目录 —— ${targetDir}`);
  process.exit(1);
}

const violations = runCheck(targetDir);
if (violations.length > 0) {
  console.error(`E2E 监听 hook 检查失败（${violations.length} 处）：\n`);
  for (const v of violations) console.error(`  - ${v}`);
  console.error('\n计数请统一走 frontend/e2e/listenerCounter.js（相对增量 API），禁在 spec 内联覆写 window 监听');
  console.error('规范见 docs/spec/e2e-testing.md §12.1');
  process.exit(1);
}
console.log(`E2E 监听 hook 检查通过：${targetDir} 无内联 window 监听覆写（helper 本体放行）`);
