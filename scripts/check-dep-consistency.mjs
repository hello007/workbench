// 前端依赖分档一致性检查（防 caret 漂移再发，任务 10-20-ep-caret）。
//
// 背景：element-plus 曾以 ^2.13.7 声明，lock 随依赖操作在 caret 范围内漂移至 2.14.5
// （minor 漂移），造成两轮回归排查成本。治理策略：package.json 顶层依赖分三档钳制，
// 本脚本在 CI 校验「档位登记 × 声明格式 × lock 实际版本」三维一致：
//   - exact：纯版本号声明，lock 必须严格相等（UI 组件库核心，根治漂移）
//   - tilde：~x.y 声明，lock 允许 patch 漂移、禁止越 minor（运行时 UI 面）
//   - caret：^x.y.z 声明，lock 允许范围内 minor/patch 漂移（测试/构建工具链）
//   - url：tarball/URL 声明，天然 exact，仅校验格式
//
// 用法：
//   node scripts/check-dep-consistency.mjs            # 检查 frontend/（仓内默认）
//   node scripts/check-dep-consistency.mjs <dir>      # 检查指定目录（selftest 内部用）
//   node scripts/check-dep-consistency.mjs --selftest # 注入漂移 fixture 自证检查器有效
//
// 退出码：0 = 全部通过；1 = 存在违规。CI 在 checkout 后立跑（纯读两个 JSON，零依赖）。

import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

// ── 分档清单（显式登记制：新增依赖未登记即失败，强制人工归类，防静默放行）──

// UI 组件库核心（事故源 element-plus 全家）：升级须显式改版本号 + 走升级 SOP
const EXACT = new Set([
  'element-plus',
  '@element-plus/icons-vue',
]);

// 运行时 UI 面：锁 minor 放 patch，minor 升级须显式改声明（触发 lock 审查）
const TILDE = new Set([
  '@codemirror/commands',
  '@codemirror/lang-css',
  '@codemirror/lang-html',
  '@codemirror/lang-javascript',
  '@codemirror/lang-json',
  '@codemirror/lang-markdown',
  '@codemirror/lang-sql',
  '@codemirror/language',
  '@codemirror/state',
  '@codemirror/view',
  'codemirror',
  'dayjs',
  'docx-preview',
  'echarts',
  'highlight.js',
  'js-yaml',
  'markdown-it',
  'mermaid',
  'pinia',
  'splitpanes',
  'vue',
  'vue-draggable-plus',
  'vue-echarts',
  'vue-router',
  '@vueuse/core',
  '@xterm/addon-fit',
  '@xterm/addon-web-links',
  '@xterm/xterm',
]);

// 测试/构建工具链：保持 caret 灵活（测试环境漂移影响面小，vitest/E2E 全量在 CI 兜底）
const CARET = new Set([
  '@playwright/test',
  '@vitejs/plugin-vue',
  '@vitest/coverage-v8',
  '@vue/test-utils',
  'jsdom',
  'vite',
  'vitest',
  '@pinia/testing',
]);

// tarball/URL 声明（天然 exact）
const URL_DEPS = new Set([
  'xlsx',
]);

const TIERS = [
  ['exact', EXACT],
  ['tilde', TILDE],
  ['caret', CARET],
  ['url', URL_DEPS],
];

// ── semver 判定（自实现最小面：本仓依赖无 prerelease，遇到即报违规转人工处理）──

function parseVersion(v) {
  const m = /^(\d+)\.(\d+)\.(\d+)$/.exec(v);
  if (!m) return null; // prerelease / build metadata / 非法形态不支持
  return [Number(m[1]), Number(m[2]), Number(m[3])];
}

function compareTriple(a, b) {
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
  }
  return 0;
}

// ^x.y.z：x>0 同 major 且 >=x.y.z；0.x（y>0）同 0.y 且 >=0.y.z；0.0.z 恰好相等（npm 语义 ^0.0.z := 0.0.z）
function caretSatisfies(version, spec) {
  const v = parseVersion(version);
  const s = parseVersion(spec.slice(1));
  if (!v || !s) return false;
  if (s[0] > 0) return v[0] === s[0] && compareTriple(v, s) >= 0;
  if (s[1] > 0) return v[0] === 0 && v[1] === s[1] && compareTriple(v, s) >= 0;
  return v[0] === 0 && v[1] === 0 && compareTriple(v, s) === 0;
}

// ~x.y.z：同 major.minor 且 >=x.y.z（0.x 包 tilde = minor+patch 全锁，npm 原生语义）
function tildeSatisfies(version, spec) {
  const v = parseVersion(version);
  const s = parseVersion(spec.slice(1));
  if (!v || !s) return false;
  return v[0] === s[0] && v[1] === s[1] && compareTriple(v, s) >= 0;
}

// ── 核心检查：返回违规描述数组（空数组 = 通过）──

export function runCheck(pkgDir) {
  const violations = [];
  const pkg = JSON.parse(fs.readFileSync(path.join(pkgDir, 'package.json'), 'utf8'));
  const lock = JSON.parse(fs.readFileSync(path.join(pkgDir, 'package-lock.json'), 'utf8'));
  const deps = { ...pkg.dependencies, ...pkg.devDependencies };

  for (const [name, spec] of Object.entries(deps)) {
    const tier = TIERS.find(([, set]) => set.has(name));
    if (!tier) {
      violations.push(`[未登记] ${name}@${spec}：新依赖须先归档（exact/tilde/caret/url 清单）再合入`);
      continue;
    }
    const [tierName] = tier;

    // 档 × 声明格式匹配
    const isUrlSpec = spec.includes('://');
    if (tierName === 'url' && !isUrlSpec) {
      violations.push(`[url 档] ${name}@${spec}：声明须为 tarball/URL`);
      continue;
    }
    if (tierName !== 'url' && isUrlSpec) {
      violations.push(`[${tierName} 档] ${name}@${spec}：URL 声明应归 url 档`);
      continue;
    }
    if (tierName === 'exact' && !/^\d+\.\d+\.\d+$/.test(spec)) {
      violations.push(`[exact 档] ${name}@${spec}：声明须为纯版本号（无 ^~ 前缀）`);
      continue;
    }
    if (tierName === 'tilde' && !spec.startsWith('~')) {
      violations.push(`[tilde 档] ${name}@${spec}：声明须以 ~ 开头`);
      continue;
    }
    if (tierName === 'caret' && !spec.startsWith('^')) {
      violations.push(`[caret 档] ${name}@${spec}：声明须以 ^ 开头`);
      continue;
    }

    // lock 实际版本一致性
    const entry = lock.packages && lock.packages[`node_modules/${name}`];
    if (!entry || !entry.version) {
      violations.push(`[lock 缺失] ${name}：package.json 已声明但 package-lock.json 无对应条目（须重新 npm install 同步）`);
      continue;
    }
    const lockVersion = entry.version;

    if (tierName === 'exact') {
      if (lockVersion !== spec) {
        violations.push(`[exact 漂移] ${name}：声明 ${spec} 但 lock 实际 ${lockVersion}`);
      }
    } else if (tierName === 'tilde') {
      if (!tildeSatisfies(lockVersion, spec)) {
        violations.push(`[tilde 越 minor] ${name}：声明 ${spec} 但 lock 实际 ${lockVersion}（越出 minor 范围）`);
      }
    } else if (tierName === 'caret') {
      if (!caretSatisfies(lockVersion, spec)) {
        violations.push(`[caret 越范围] ${name}：声明 ${spec} 但 lock 实际 ${lockVersion}（越出 caret 范围）`);
      }
    }
  }

  return violations;
}

// ── selftest：内存 fixture 注入各档漂移形态，断言必拦；合形态断言必过 ──

function makeFixture(deps, lockVersions) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'dep-check-selftest-'));
  const lockPackages = { '': { dependencies: {} } };
  for (const [name, spec] of Object.entries(deps)) {
    lockPackages[`node_modules/${name}`] = { version: lockVersions[name] };
  }
  fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({ dependencies: deps }));
  fs.writeFileSync(path.join(dir, 'package-lock.json'), JSON.stringify({ lockfileVersion: 3, packages: lockPackages }));
  return dir;
}

function runSelftest() {
  const cases = [
    {
      desc: 'exact 版本错位被拦',
      deps: { 'element-plus': '2.14.5' },
      lock: { 'element-plus': '2.14.6' },
      expect: 'fail',
    },
    {
      desc: 'exact 声明带 ^ 前缀被拦',
      deps: { 'element-plus': '^2.14.5' },
      lock: { 'element-plus': '2.14.5' },
      expect: 'fail',
    },
    {
      desc: 'tilde 越 minor 被拦',
      deps: { vue: '~3.5.42' },
      lock: { vue: '3.6.0' },
      expect: 'fail',
    },
    {
      desc: 'tilde 声明漏 ~ 被拦',
      deps: { vue: '3.5.42' },
      lock: { vue: '3.5.42' },
      expect: 'fail',
    },
    {
      desc: 'caret 越范围（major 跳变）被拦',
      deps: { vitest: '^4.1.5' },
      lock: { vitest: '5.0.0' },
      expect: 'fail',
    },
    {
      desc: '0.x caret 语义（minor 锁定）越 minor 被拦',
      deps: { '@pinia/testing': '^0.6.1' },
      lock: { '@pinia/testing': '0.7.0' },
      expect: 'fail',
    },
    {
      desc: '0.0.x caret 语义（patch 锁定）patch 漂移被拦',
      deps: { '@pinia/testing': '^0.0.3' },
      lock: { '@pinia/testing': '0.0.4' },
      expect: 'fail',
    },
    {
      desc: '未登记新依赖被拦',
      deps: { 'some-new-pkg': '^1.0.0' },
      lock: { 'some-new-pkg': '1.0.0' },
      expect: 'fail',
    },
    {
      desc: 'lock 缺条目被拦',
      deps: { vue: '~3.5.42' },
      lock: {},
      expect: 'fail',
    },
    {
      desc: '全部合形态通过（exact 相等 / tilde patch 内 / caret minor 内 / url）',
      deps: { 'element-plus': '2.14.5', vue: '~3.5.42', vitest: '^4.1.5', xlsx: 'https://cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz' },
      lock: { 'element-plus': '2.14.5', vue: '3.5.50', vitest: '4.1.11', xlsx: '0.20.3' },
      expect: 'pass',
    },
    {
      desc: 'tilde patch 漂移通过（~3.5.42 允许 3.5.x）',
      deps: { vue: '~3.5.42' },
      lock: { vue: '3.5.49' },
      expect: 'pass',
    },
  ];

  let failed = 0;
  for (const c of cases) {
    const dir = makeFixture(c.deps, c.lock);
    let violations = [];
    try {
      violations = runCheck(dir);
    } finally {
      fs.rmSync(dir, { recursive: true, force: true });
    }
    const pass = violations.length === 0;
    const ok = c.expect === 'pass' ? pass : !pass;
    if (!ok) {
      failed++;
      console.error(`selftest FAIL: ${c.desc}`);
      console.error(`  期望 ${c.expect}，实际 ${pass ? 'pass' : 'fail'}${violations.length ? '：' + violations.join('；') : ''}`);
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

// 默认目标 = 仓库内 frontend/（基于脚本自身位置解析，任意 cwd 调用均正确；CI 与本机一致）
const defaultDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'frontend');
const targetDir = path.resolve(args.find((a) => !a.startsWith('--')) || defaultDir);
const violations = runCheck(targetDir);
if (violations.length > 0) {
  console.error(`依赖分档一致性检查失败（${violations.length} 项）：\n`);
  for (const v of violations) console.error(`  - ${v}`);
  console.error('\n修复指引见 docs/spec/frontend-dep-upgrade.md（分档策略与升级 SOP）');
  process.exit(1);
}
console.log(`依赖分档一致性检查通过：${targetDir}/package.json 与 package-lock.json 三维一致`);
