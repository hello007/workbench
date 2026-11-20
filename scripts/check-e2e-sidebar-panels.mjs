// E2E 侧边栏面板顺序同源静态守护（防 f0cb6e5 双份名单漂移回流，任务 11-20-e2e-sidebar-panels）。
//
// 背景：侧边栏默认面板顺序真相源是 frontend/src/store/sidebarPanels.js 的 SIDEBAR_PANELS，
// 但 frontend/e2e/sidebar-shortcuts.spec.js 曾各写一份 DEFAULT_ORDER（9 项硬编码），
// 新增第 10 个面板（提示词）后 spec 在 master 上直接变红，直到下一个任务顺手修复。
// 治理策略：spec 从零依赖模块导入 SIDEBAR_PANELS 派生顺序/数量；本脚本拦截「再硬编码」。
//
// 检查项：
//   1. 真相源可用性：frontend/src/store/sidebarPanels.js 的 `{ id: '<x>' }` 列表必须
//      非空且无重复（解析失败 = 检查器失去依据，直接失败）。
//   2. 硬编码名单：扫描目标目录下脚本文件，**同一行出现 ≥3 个已知面板 id 字符串字面量**
//      即判定为「又写了一份名单」，报 file:line。阈值 3 使单 id 语义定位
//      （activityPanel(page, 'directory')、筛选条件 ['stats', ...] 等）照常放行。
//   3. 收敛点：目标目录下若存在 sidebar-shortcuts.spec.js，必须从
//      `.../store/sidebarPanels` 导入 SIDEBAR_PANELS（改回自造常量即刻失败）。
//
// 边界声明（有意不覆盖，命中需人工判断）：
//   - 逐行正则匹配，不剥离注释与字符串字面量——一行里凑够 3 个 id 的注释同样报违规
//     （宁误报不放过）；跨行数组字面量（一行一个 id）不检测
//   - 扫描扩展名覆盖 .js/.jsx/.ts/.tsx/.mjs/.cjs/.mts/.cts，其他语言不检测
//   - 只认 source 模块中解析出的 id；新增面板尚未写入 SIDEBAR_PANELS 时无从比对
//
// 用法：
//   node scripts/check-e2e-sidebar-panels.mjs                     # 默认源 + 扫描 frontend/e2e
//   node scripts/check-e2e-sidebar-panels.mjs <dir>               # 扫描指定目录
//   node scripts/check-e2e-sidebar-panels.mjs <dir> --source <f>  # 指定真相源（selftest 用）
//   node scripts/check-e2e-sidebar-panels.mjs --selftest          # 注入违规/合规 fixture 自证
//
// 退出码：0 = 全部通过；1 = 存在违规（或扫描根/真相源不存在 / selftest 失败）。
// CI 在「E2E 监听计数 hook 静态守护」之后立跑（纯读文本，零依赖零网络）。

import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const SCRIPT_FILE_RE = /\.[cm]?[jt]sx?$/;
const PANEL_ID_RE = /\{\s*id:\s*['"]([^'"]+)['"]/g;
// 单行「已知 id 字面量」计数阈值：≥3 视为硬编码名单（1-2 个多为语义定位/筛选单值）
const HARD_CODE_THRESHOLD = 3;

// ── 真相源解析：返回 id 数组（重复/空由 validateIds 判定）──

function parsePanelIds(sourceFile) {
  const text = fs.readFileSync(sourceFile, 'utf8');
  return [...text.matchAll(PANEL_ID_RE)].map((m) => m[1]);
}

function validateIds(ids) {
  const problems = [];
  if (ids.length === 0) problems.push('未解析到任何面板 id（SIDEBAR_PANELS 结构是否变更？）');
  const seen = new Set();
  for (const id of ids) {
    if (seen.has(id)) problems.push(`面板 id 重复：${id}`);
    seen.add(id);
  }
  return problems;
}

// ── 文件收集（递归脚本文件）──

function collectScriptFiles(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) collectScriptFiles(full, out);
    else if (entry.isFile() && SCRIPT_FILE_RE.test(entry.name)) out.push(full);
  }
  return out;
}

function countIdLiterals(line, ids) {
  let count = 0;
  for (const id of ids) {
    const escaped = id.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const re = new RegExp(`['"]${escaped}['"]`, 'g');
    count += (line.match(re) || []).length;
  }
  return count;
}

// ── 核心检查：返回违规描述数组（空数组 = 通过）──

function runCheck(targetDir, ids) {
  const violations = [];
  for (const file of collectScriptFiles(targetDir)) {
    const rel = path.relative(targetDir, file).split(path.sep).join('/');
    const lines = fs.readFileSync(file, 'utf8').split(/\r?\n/);
    lines.forEach((line, index) => {
      if (countIdLiterals(line, ids) >= HARD_CODE_THRESHOLD) {
        violations.push(`${rel}:${index + 1}: 硬编码面板 id 名单 — ${line.trim()}`);
      }
    });
  }

  // 收敛点：对应 spec 必须从零依赖模块导入真相源
  const specPath = path.join(targetDir, 'sidebar-shortcuts.spec.js');
  if (fs.existsSync(specPath)) {
    const specText = fs.readFileSync(specPath, 'utf8');
    if (!/from\s+['"][^'"]*store\/sidebarPanels(?:\.js)?['"]/.test(specText)) {
      violations.push('sidebar-shortcuts.spec.js: 未从 store/sidebarPanels 导入 SIDEBAR_PANELS（改回自造常量的漂移入口）');
    }
  }
  return violations;
}

// ── selftest：临时 fixture 注入各形态断言必拦 / 合规形态断言必过 ──

function writeFixture(caseDir, files) {
  for (const [rel, content] of Object.entries(files)) {
    const full = path.join(caseDir, rel);
    fs.mkdirSync(path.dirname(full), { recursive: true });
    fs.writeFileSync(full, content);
  }
}

const FIXTURE_SOURCE = [
  "export const SIDEBAR_PANELS = [",
  "  { id: 'directory', label: '工作目录' },",
  "  { id: 'ai', label: 'AI 功能' },",
  "  { id: 'ai-chat', label: 'AI 对话' },",
  "  { id: 'todo', label: '临时待办' }",
  "]",
  ""
].join('\n');

function runSelftest() {
  const cases = [
    {
      desc: '3 个已知 id 的数组字面量被拦',
      files: { 'bad.js': "const order = ['directory', 'ai', 'ai-chat']\n" },
      expect: 'fail',
    },
    {
      desc: '嵌套目录 / TypeScript 同样被扫描',
      files: { 'nested/bad.ts': "const order = ['ai', 'ai-chat', 'todo']\n" },
      expect: 'fail',
    },
    {
      desc: '违规定位精确到 file:line',
      files: { 'ok.js': '// 合规注释\n', 'bad.js': "const x = 1\nconst order = ['directory', 'ai', 'todo']\n" },
      expect: 'fail',
      expectHit: 'bad.js:2:',
    },
    {
      desc: '单 id 语义选择器放行',
      files: { 'ok.js': "activityPanel(page, 'directory').click()\n" },
      expect: 'pass',
    },
    {
      // 必须用真相源 fixture 内真实存在的 2 个 id：若误用未登记 id，实际只计 1 个，
      // 阈值被误改成 2 时该用例仍会「通过」，边界守护失效。
      desc: '阈值边界：一行 2 个已知 id 放行',
      files: { 'ok.js': "const custom = ['ai', 'todo']\n" },
      expect: 'pass',
    },
    {
      desc: '导入 SIDEBAR_PANELS 派生放行',
      files: {
        'ok.js': "import { SIDEBAR_PANELS } from '../src/store/sidebarPanels'\nconst order = SIDEBAR_PANELS.map(p => p.id)\n",
      },
      expect: 'pass',
    },
    {
      desc: '收敛点：spec 未导入真相源被拦',
      files: { 'sidebar-shortcuts.spec.js': "const order = ['directory']\n" },
      expect: 'fail',
      expectHit: 'sidebar-shortcuts.spec.js:',
    },
    {
      desc: '收敛点：spec 已导入真相源放行',
      files: {
        'sidebar-shortcuts.spec.js': "import { SIDEBAR_PANELS } from '../src/store/sidebarPanels'\n",
      },
      expect: 'pass',
    },
  ];

  let failed = 0;
  for (const c of cases) {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'e2e-sidebar-panels-selftest-'));
    const e2eDir = path.join(dir, 'e2e');
    const sourceFile = path.join(dir, 'sidebarPanels.js');
    let violations = [];
    try {
      fs.mkdirSync(e2eDir, { recursive: true });
      fs.writeFileSync(sourceFile, FIXTURE_SOURCE);
      writeFixture(e2eDir, c.files);
      violations = runCheck(e2eDir, parsePanelIds(sourceFile));
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

  // 真相源自身校验：空列表 / 重复 id 必须失败（validateIds 直测）
  const idCases = [
    { desc: '真相源空列表判定失败', ids: [], expect: 'fail' },
    { desc: '真相源重复 id 判定失败', ids: ['directory', 'directory'], expect: 'fail' },
    { desc: '真相源正常 id 列表判定通过', ids: ['directory', 'ai'], expect: 'pass' },
  ];
  for (const c of idCases) {
    const pass = validateIds(c.ids).length === 0;
    const ok = c.expect === 'pass' ? pass : !pass;
    if (!ok) {
      failed++;
      console.error(`selftest FAIL: ${c.desc}`);
    } else {
      console.log(`selftest ok: ${c.desc}`);
    }
  }

  const total = cases.length + idCases.length;
  if (failed > 0) {
    console.error(`\nselftest 失败 ${failed}/${total}：检查器判定逻辑与预期不符，禁止上线`);
    return false;
  }
  console.log(`\nselftest 全部通过（${total} 用例）`);
  return true;
}

// ── 入口 ──

const args = process.argv.slice(2);
if (args.includes('--selftest')) {
  process.exit(runSelftest() ? 0 : 1);
}

const sourceIdx = args.indexOf('--source');
const sourceArg = sourceIdx >= 0 ? args[sourceIdx + 1] : undefined;
// 逐 token 解析：跳过的只能是 --source 及其取值，其余非 -- 前缀参数按序作为 positional。
// 不能用「i !== sourceIdx + 1」过滤——sourceIdx 为 -1 时会连带丢掉 args[0]（首个目标目录）。
const positional = [];
for (let i = 0; i < args.length; i++) {
  const arg = args[i];
  if (arg === '--source') { i++; continue; }
  if (arg.startsWith('--')) continue;
  positional.push(arg);
}

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultSource = path.resolve(scriptDir, '..', 'frontend', 'src', 'store', 'sidebarPanels.js');
const sourceFile = path.resolve(sourceArg || defaultSource);
const targetDir = path.resolve(positional[0] || path.resolve(scriptDir, '..', 'frontend', 'e2e'));

if (!fs.existsSync(sourceFile) || !fs.statSync(sourceFile).isFile()) {
  console.error(`侧边栏面板顺序检查失败：真相源不存在 —— ${sourceFile}`);
  process.exit(1);
}
if (!fs.existsSync(targetDir) || !fs.statSync(targetDir).isDirectory()) {
  console.error(`侧边栏面板顺序检查失败：扫描根不存在或不是目录 —— ${targetDir}`);
  process.exit(1);
}

const ids = parsePanelIds(sourceFile);
const idProblems = validateIds(ids);
if (idProblems.length > 0) {
  console.error('侧边栏面板顺序检查失败：真相源异常');
  for (const p of idProblems) console.error(`  - ${p}`);
  process.exit(1);
}

const violations = runCheck(targetDir, ids);
if (violations.length > 0) {
  console.error(`侧边栏面板顺序检查失败（${violations.length} 处）：\n`);
  for (const v of violations) console.error(`  - ${v}`);
  console.error('\n顺序与数量断言请从 frontend/src/store/sidebarPanels.js 导入 SIDEBAR_PANELS 派生，禁在 E2E 内硬编码面板 id 名单');
  console.error('规范见 docs/spec/e2e-testing.md §12.4');
  process.exit(1);
}
console.log(`侧边栏面板顺序检查通过：${targetDir} 无硬编码面板名单（真相源 ${ids.length} 项）`);
