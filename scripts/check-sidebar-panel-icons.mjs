// 侧边栏面板图标映射对齐静态守护（防 panelIconMap 漏加/残留静默漂移，任务 11-24-sidebar-panels）。
//
// 背景：侧边栏面板 id 真相源是 frontend/src/store/sidebarPanels.js 的 SIDEBAR_PANELS，
// 图标组件映射是 frontend/src/components/ActivityBar.vue 的 panelIconMap（手写 id → 组件）。
// 新增面板只改真相源而漏加映射时，<component :is="undefined"> 渲染为空图标且无任何报错，
// 与 useSessionState 崩溃恢复白名单漂移同型。治理策略：本脚本双向比对两个集合——
//   - 缺失：SIDEBAR_PANELS 的任一 id 不在 panelIconMap 中；
//   - 残留：panelIconMap 的任一键不在 SIDEBAR_PANELS 中（删面板后未清理的死映射）。
//
// 检查项：
//   1. 真相源可用性：定位 `SIDEBAR_PANELS = [ ... ]` 数组块，逐个面板对象读取 id；id 必须是
//      字符串字面量，非对象项/非字面量 id 一律失败（fail-closed，静态检查器无法推断动态值）。
//   2. 图标映射可用性：定位 `const panelIconMap = { ... }`，只采集对象一级属性键；支持引号键
//      与裸标识符键，跳过行注释/块注释/字符串，嵌套对象与函数调用中的键不计入。
//   3. 双向集合比对：缺失逐项报 SIDEBAR_PANELS 定义行；残留逐项报 ActivityBar.vue 行号。
//
// 边界声明（有意不覆盖，命中需人工判断）：
//   - 只做文本解析，不 import 任何 Vue/TS 源码（frontend/wailsjs/ 不入库，node 侧无法解析）
//   - 正则/模板字面量作为值时不参与解析；若 panelIconMap 值改为正则等含 `{` `}` 的表达式，
//     可能误判——当前值均为组件标识符或简单嵌套对象，命中时请人工确认
//   - panelIconMap 若被重构为其他形态（迁入别处/改为函数），本脚本报「未找到声明」并失败，
//     迫使同步调整而非静默失效
//
// 用法：
//   node scripts/check-sidebar-panel-icons.mjs                              # 默认源
//   node scripts/check-sidebar-panel-icons.mjs --panel-source <f> --icon-source <f>
//   node scripts/check-sidebar-panel-icons.mjs --selftest                  # 注入 fixture 自证
//
// 退出码：0 = 全部通过；1 = 存在违规（或源文件缺失 / selftest 失败）。
// CI 在「E2E 侧边栏面板顺序同源守护」之后立跑（纯读文本，零依赖零网络）。

import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const PANEL_ARRAY_DECL_RE = /(?:export\s+)?const\s+SIDEBAR_PANELS\s*=\s*\[/;
const ICON_MAP_DECL_RE = /const\s+panelIconMap\s*=\s*\{/;
const IDENT_START_RE = /[A-Za-z_$]/;
const IDENT_PART_RE = /[\w$]/;

// ── 行号工具：字符下标 → 1-based 行号 ──

function lineOf(text, index) {
  return text.slice(0, index).split('\n').length;
}

// ── 词法跳读：注释 / 字符串（保持原下标，不做等长改写）──

function skipLineComment(text, i) {
  const nl = text.indexOf('\n', i + 2);
  return nl === -1 ? text.length : nl + 1;
}

function skipBlockComment(text, i) {
  const end = text.indexOf('*/', i + 2);
  return end === -1 ? text.length : end + 2;
}

function skipString(text, i) {
  const quote = text[i];
  let j = i + 1;
  while (j < text.length) {
    if (text[j] === '\\') {
      j += 2;
      continue;
    }
    if (text[j] === quote) {
      return j + 1;
    }
    j++;
  }
  return text.length;
}

function skipWsAndComments(text, i) {
  while (i < text.length) {
    if (/\s/.test(text[i])) {
      i++;
      continue;
    }
    if (text[i] === '/' && text[i + 1] === '/') {
      i = skipLineComment(text, i);
      continue;
    }
    if (text[i] === '/' && text[i + 1] === '*') {
      i = skipBlockComment(text, i);
      continue;
    }
    break;
  }
  return i;
}

/**
 * 等长掩码：注释与字符串内容替换为空格（保留 \r\n 与下标），仅用于定位声明与括号配对。
 * 不能用它读取属性值——字符串内容已被抹掉，值解析仍走原文。
 */
function maskCommentsAndStrings(text) {
  const chars = text.split('');
  let i = 0;
  while (i < text.length) {
    const ch = text[i];
    if (ch === '/' && text[i + 1] === '/') {
      let end = text.indexOf('\n', i + 2);
      if (end === -1) end = text.length;
      for (let k = i; k < end; k++) chars[k] = ' ';
      i = end;
      continue;
    }
    if (ch === '/' && text[i + 1] === '*') {
      let end = text.indexOf('*/', i + 2);
      end = end === -1 ? text.length : end + 2;
      for (let k = i; k < end; k++) {
        if (chars[k] !== '\n' && chars[k] !== '\r') chars[k] = ' ';
      }
      i = end;
      continue;
    }
    if (ch === '"' || ch === "'" || ch === '`') {
      const end = skipString(text, i);
      for (let k = i; k < end; k++) {
        if (chars[k] !== '\n' && chars[k] !== '\r') chars[k] = ' ';
      }
      i = end;
      continue;
    }
    i++;
  }
  return chars.join('');
}

/** 从 openIdx 处的开括号开始配对，跳过注释/字符串；返回闭括号下标或 -1 */
function findMatching(text, openIdx, openCh, closeCh) {
  let depth = 0;
  for (let i = openIdx; i < text.length; ) {
    const ch = text[i];
    if (ch === '/' && text[i + 1] === '/') {
      i = skipLineComment(text, i);
      continue;
    }
    if (ch === '/' && text[i + 1] === '*') {
      i = skipBlockComment(text, i);
      continue;
    }
    if (ch === '"' || ch === "'" || ch === '`') {
      i = skipString(text, i);
      continue;
    }
    if (ch === openCh) {
      depth++;
      i++;
      continue;
    }
    if (ch === closeCh) {
      depth--;
      if (depth === 0) {
        return i;
      }
      i++;
      continue;
    }
    i++;
  }
  return -1;
}

// ── 对象字面量扫描：只采集一级属性（嵌套对象/函数调用中的键不计入）──

/**
 * 扫描 openIdx 处的对象字面量，返回一级属性列表。
 * 属性形态：`key: <string 字面量>` 记 valueKind='string'，其余记 'other'。
 * @returns {{ properties: Array<{key: string, keyStart: number, valueKind: string, value?: string}>, problems: string[], closed: boolean, end: number }}
 */
function scanObjectProperties(text, openIdx) {
  const properties = [];
  const problems = [];
  let depth = 0;
  let parenDepth = 0;
  let bracketDepth = 0;
  let expectingKey = false;
  let i = openIdx;

  while (i < text.length) {
    const ch = text[i];

    // 注释：无论层级一律跳过（保证注释内的 `}` 不影响配对）
    if (ch === '/' && text[i + 1] === '/') {
      i = skipLineComment(text, i);
      continue;
    }
    if (ch === '/' && text[i + 1] === '*') {
      i = skipBlockComment(text, i);
      continue;
    }

    // 花括号层级：嵌套对象只跟踪层级，不采集其键
    if (ch === '{') {
      depth++;
      if (depth === 1) {
        expectingKey = true;
      }
      i++;
      continue;
    }
    if (ch === '}') {
      depth--;
      if (depth === 0) {
        return { properties, problems, closed: true, end: i };
      }
      i++;
      continue;
    }

    // 一级属性分隔符：值内部的括号/方括号内逗号不算属性分隔
    if (depth === 1 && ch === ',' && parenDepth === 0 && bracketDepth === 0) {
      expectingKey = true;
      i++;
      continue;
    }

    // 值内部的圆括号/方括号（函数调用、数组）：不参与属性判定
    if (ch === '(') {
      parenDepth++;
      i++;
      continue;
    }
    if (ch === ')') {
      if (parenDepth > 0) parenDepth--;
      i++;
      continue;
    }
    if (ch === '[') {
      bracketDepth++;
      i++;
      continue;
    }
    if (ch === ']') {
      if (bracketDepth > 0) bracketDepth--;
      i++;
      continue;
    }

    if (depth === 1 && expectingKey) {
      if (/\s/.test(ch)) {
        i++;
        continue;
      }
      // 尾随逗号后直接闭合：交给下一轮 `}` 分支处理
      if (ch === ',') {
        i++;
        continue;
      }
      let key;
      let keyStart = i;
      let next;
      if (ch === '"' || ch === "'") {
        next = skipString(text, i);
        key = text.slice(i + 1, next - 1);
      } else if (IDENT_START_RE.test(ch)) {
        next = i;
        while (next < text.length && IDENT_PART_RE.test(text[next])) {
          next++;
        }
        key = text.slice(i, next);
      } else {
        problems.push(`对象属性解析失败：第 ${lineOf(text, i)} 行遇到意外字符 "${ch}"`);
        return { properties, problems, closed: false, end: i };
      }
      next = skipWsAndComments(text, next);
      if (text[next] !== ':') {
        problems.push(`对象属性 "${key}" 缺少冒号（第 ${lineOf(text, keyStart)} 行）`);
        return { properties, problems, closed: false, end: next };
      }
      next = skipWsAndComments(text, next + 1);
      let valueKind = 'other';
      let value;
      if (text[next] === '"' || text[next] === "'") {
        const endStr = skipString(text, next);
        valueKind = 'string';
        value = text.slice(next + 1, endStr - 1);
        next = endStr;
      }
      properties.push({ key, keyStart, valueKind, value });
      expectingKey = false;
      i = next;
      continue;
    }

    // 值中的字符串：跳过，避免字符串内的 `{` `}` `,` 干扰
    if (ch === '"' || ch === "'" || ch === '`') {
      i = skipString(text, i);
      continue;
    }

    i++;
  }

  return { properties, problems: [...problems, '对象字面量未闭合'], closed: false, end: text.length };
}

// ── 真相源解析：SIDEBAR_PANELS 数组块 → [{ id, line }] ──

function parsePanelEntries(text) {
  // 用注释/字符串掩码定位声明，避免注释里先出现同名声明导致配对到错误块
  const masked = maskCommentsAndStrings(text);
  const decl = PANEL_ARRAY_DECL_RE.exec(masked);
  if (!decl) {
    return { entries: [], problems: ['未找到 SIDEBAR_PANELS 数组声明'] };
  }
  const openIdx = masked.indexOf('[', decl.index);
  const closeIdx = findMatching(masked, openIdx, '[', ']');
  if (closeIdx < 0) {
    return { entries: [], problems: ['SIDEBAR_PANELS 数组未闭合'] };
  }

  const entries = [];
  const problems = [];
  let i = openIdx + 1;
  let itemCount = 0;

  while (i < closeIdx) {
    const ch = text[i];
    if (ch === '/' && text[i + 1] === '/') {
      i = skipLineComment(text, i);
      continue;
    }
    if (ch === '/' && text[i + 1] === '*') {
      i = skipBlockComment(text, i);
      continue;
    }
    if (/\s/.test(ch) || ch === ',') {
      i++;
      continue;
    }
    if (ch === '{') {
      const result = scanObjectProperties(text, i);
      itemCount++;
      const idProp = result.properties.find((p) => p.key === 'id');
      if (!idProp) {
        problems.push(`第 ${lineOf(text, i)} 行面板对象缺少 id 属性`);
      } else if (idProp.valueKind !== 'string') {
        problems.push(`第 ${lineOf(text, i)} 行面板 id 不是字符串字面量（无法静态比对）`);
      } else {
        entries.push({ id: idProp.value, line: lineOf(text, idProp.keyStart) });
      }
      if (!result.closed) {
        problems.push(`第 ${lineOf(text, i)} 行面板对象未闭合`);
        break;
      }
      i = result.end + 1;
      continue;
    }
    // 顶层非对象项（字符串/数组/标识符/展开语法等）：静态检查器无法取 id，fail-closed
    problems.push(`第 ${lineOf(text, i)} 行 SIDEBAR_PANELS 存在非对象项，无法静态取 id`);
    break;
  }

  if (itemCount === 0 && problems.length === 0) {
    problems.push('SIDEBAR_PANELS 中未找到面板对象');
  }
  return { entries, problems };
}

function validatePanelEntries(entries) {
  const problems = [];
  if (entries.length === 0) {
    problems.push('未解析到任何面板 id（SIDEBAR_PANELS 结构是否变更？）');
  }
  const seen = new Set();
  for (const { id } of entries) {
    if (seen.has(id)) {
      problems.push(`面板 id 重复：${id}`);
    }
    seen.add(id);
  }
  return problems;
}

// ── 图标映射解析：panelIconMap 一级属性键 ──

/** 截取首个 `<script>` 块内容（无 script 标签时退回全文）；start 为块内绝对起始下标 */
function scriptRegion(text) {
  const open = /<script\b[^>]*>/.exec(text);
  if (!open) {
    return { start: 0, content: text };
  }
  const contentStart = open.index + open[0].length;
  const close = text.indexOf('</script>', contentStart);
  if (close < 0) {
    return { start: 0, content: text };
  }
  return { start: contentStart, content: text.slice(contentStart, close) };
}

function parseIconKeys(text) {
  // 只在 <script> 块内定位与解析：模板文本中的未配对引号不应吞掉脚本内容
  const region = scriptRegion(text);
  const masked = maskCommentsAndStrings(region.content);
  const decl = ICON_MAP_DECL_RE.exec(masked);
  if (!decl) {
    return { found: false, keys: [], problems: ['未找到 `const panelIconMap = {` 声明（映射是否被重构或迁走？）'] };
  }
  const openIdx = region.start + masked.indexOf('{', decl.index);
  const result = scanObjectProperties(text, openIdx);
  const keys = result.properties.map((p) => ({ key: p.key, line: lineOf(text, p.keyStart) }));

  const problems = [...result.problems];
  if (!result.closed) {
    problems.push('panelIconMap 对象花括号未闭合（文件结构异常）');
  }
  if (keys.length === 0) {
    problems.push('未从 panelIconMap 解析到任何图标键（映射结构是否变更？）');
  }
  const seen = new Set();
  for (const { key } of keys) {
    if (seen.has(key)) {
      problems.push(`图标键重复：${key}`);
    }
    seen.add(key);
  }
  return { found: true, keys, problems };
}

// ── 核心比对：返回违规描述数组（空数组 = 通过）──

function compareSets(panelEntries, iconEntries, panelFile, iconFile) {
  const violations = [];
  const iconKeys = new Set(iconEntries.map((e) => e.key));
  const panelIds = new Set(panelEntries.map((e) => e.id));

  for (const { id, line } of panelEntries) {
    if (!iconKeys.has(id)) {
      violations.push({
        message: `缺少图标映射：面板 "${id}" 不在 panelIconMap 中`,
        file: panelFile,
        line,
        hint: `在 ${path.basename(iconFile)} 的 panelIconMap 中补 \`${id}: <图标组件>\``
      });
    }
  }
  for (const { key, line } of iconEntries) {
    if (!panelIds.has(key)) {
      violations.push({
        message: `残留图标映射：panelIconMap 键 "${key}" 未在 SIDEBAR_PANELS 中登记`,
        file: iconFile,
        line,
        hint: '删除该键，或确认面板 id 是否写错'
      });
    }
  }
  return violations;
}

/**
 * 文件级检查（CLI 与 selftest 共用）。
 * @returns {{ problems: string[], violations: Array<{message,file,line,hint}>, panelCount: number }}
 */
function runCheck(panelFile, iconFile) {
  const problems = [];
  if (!fs.existsSync(panelFile) || !fs.statSync(panelFile).isFile()) {
    problems.push(`面板真相源不存在 —— ${panelFile}`);
  }
  if (!fs.existsSync(iconFile) || !fs.statSync(iconFile).isFile()) {
    problems.push(`图标映射源不存在 —— ${iconFile}`);
  }
  if (problems.length > 0) {
    return { problems, violations: [], panelCount: 0 };
  }

  const panelText = fs.readFileSync(panelFile, 'utf8');
  const iconText = fs.readFileSync(iconFile, 'utf8');

  const panelResult = parsePanelEntries(panelText);
  problems.push(...panelResult.problems);
  problems.push(...validatePanelEntries(panelResult.entries));

  const iconResult = parseIconKeys(iconText);
  problems.push(...iconResult.problems);

  if (problems.length > 0) {
    return { problems, violations: [], panelCount: panelResult.entries.length };
  }

  const violations = compareSets(panelResult.entries, iconResult.keys, panelFile, iconFile);
  return { problems, violations, panelCount: panelResult.entries.length };
}

// ── selftest：临时 fixture 注入各形态断言必拦 / 合规形态断言必过 ──

const FIXTURE_PANELS = [
  'export const SIDEBAR_PANELS = [',
  "  { id: 'directory', label: '工作目录' },",
  "  { id: 'ai', label: 'AI 功能' },",
  "  { id: 'ai-chat', label: 'AI 对话' },",
  "  { id: 'todo', label: '临时待办' }",
  ']',
  ''
].join('\n');

const buildVue = (mapLines) =>
  ['<script setup>', 'const panelIconMap = {', ...mapLines, '}', '</script>', ''].join('\n');

const ALIGNED_MAP = [
  '  directory: Folder,',
  '  ai: MagicStick,',
  "  'ai-chat': ChatDotRound,",
  '  todo: Memo,'
];

function writeFixture(dir, name, content) {
  const full = path.join(dir, name);
  fs.writeFileSync(full, content);
  return full;
}

function runSelftest() {
  const cases = [
    {
      desc: '面板 id 与图标键完全一致时通过',
      vue: buildVue(ALIGNED_MAP),
      expect: 'pass'
    },
    {
      desc: '缺失图标键失败，命中缺失 id 与定义行号',
      vue: buildVue(ALIGNED_MAP.slice(0, 3)),
      expect: 'fail',
      expectHit: '缺少图标映射',
      expectLine: 5
    },
    {
      desc: '残留未登记图标键失败，命中残留键与行号',
      vue: buildVue([...ALIGNED_MAP, '  legacy: Folder']),
      expect: 'fail',
      expectHit: '残留图标映射',
      expectLine: 7
    },
    {
      desc: '注释中的孤立 `}` 不提前闭合对象，其后残留键仍被检出',
      vue: buildVue([
        '  // 说明：单花括号 } 不应影响配对',
        '  directory: Folder,',
        '  ai: MagicStick,',
        "  'ai-chat': ChatDotRound,",
        '  todo: Memo,',
        '  legacy: Folder'
      ]),
      expect: 'fail',
      expectHit: '残留图标映射',
      expectLine: 8
    },
    {
      desc: '块注释中的疑似键不计入',
      vue: buildVue(['  /* legacy: Folder, */', ...ALIGNED_MAP]),
      expect: 'pass'
    },
    {
      desc: '单行对象字面量解析',
      vue: '<script setup>\nconst panelIconMap = { directory: Folder, ai: MagicStick, "ai-chat": ChatDotRound, todo: Memo }\n</script>\n',
      expect: 'pass'
    },
    {
      desc: '双引号键解析',
      vue: buildVue([
        '  directory: Folder,',
        '  ai: MagicStick,',
        '  "ai-chat": ChatDotRound,',
        '  todo: Memo'
      ]),
      expect: 'pass'
    },
    {
      desc: '嵌套对象值中的子键不计入图标键',
      vue: buildVue([
        '  directory: { icon: Folder },',
        '  ai: MagicStick,',
        "  'ai-chat': ChatDotRound,",
        '  todo: Memo'
      ]),
      expect: 'pass'
    },
    {
      desc: '函数调用值中的逗号不误判为属性分隔',
      vue: buildVue([
        '  directory: makeIcon(Folder, MagicStick),',
        '  ai: MagicStick,',
        "  'ai-chat': ChatDotRound,",
        '  todo: Memo'
      ]),
      expect: 'pass'
    },
    {
      desc: '面板 id 非字符串字面量 fail-closed',
      panels: "export const SIDEBAR_PANELS = [\n  { id: makeId(), label: '动态' }\n]\n",
      vue: buildVue([]),
      expect: 'fail',
      expectHit: '不是字符串字面量'
    },
    {
      desc: '面板 id 非首位且带引号仍可解析',
      panels: "export const SIDEBAR_PANELS = [\n  { label: '工作目录', 'id': 'directory' }\n]\n",
      vue: buildVue(['  directory: Folder']),
      expect: 'pass'
    },
    {
      desc: '面板源注释中的旧面板不参与解析',
      panels: [
        'export const SIDEBAR_PANELS = [',
        "  // { id: 'ghost', label: '旧面板' }",
        "  { id: 'directory', label: '工作目录' }",
        ']',
        ''
      ].join('\n'),
      vue: buildVue(['  directory: Folder']),
      expect: 'pass'
    },
    {
      desc: '顶层字符串项 fail-closed（不得静默跳过）',
      panels: "export const SIDEBAR_PANELS = [\n  'directory',\n  { id: 'ai', label: 'AI 功能' }\n]\n",
      vue: buildVue(['  ai: MagicStick']),
      expect: 'fail',
      expectHit: '存在非对象项'
    },
    {
      desc: '顶层嵌套数组项 fail-closed',
      panels: "export const SIDEBAR_PANELS = [\n  ['directory'],\n  { id: 'ai', label: 'AI 功能' }\n]\n",
      vue: buildVue(['  ai: MagicStick']),
      expect: 'fail',
      expectHit: '存在非对象项'
    },
    {
      desc: '注释中先出现的 SIDEBAR_PANELS 声明不干扰定位',
      panels: [
        "// const SIDEBAR_PANELS = [ { id: 'ghost' } ]",
        'export const SIDEBAR_PANELS = [',
        "  { id: 'directory', label: '工作目录' }",
        ']',
        ''
      ].join('\n'),
      vue: buildVue(['  directory: Folder']),
      expect: 'pass'
    },
    {
      desc: '注释中先出现的 panelIconMap 声明不干扰定位',
      vue: [
        '<script setup>',
        '// const panelIconMap = { ghost: Folder }',
        'const panelIconMap = { directory: Folder }',
        '</script>',
        ''
      ].join('\n'),
      panels: "export const SIDEBAR_PANELS = [\n  { id: 'directory', label: '工作目录' }\n]\n",
      expect: 'pass'
    },
    {
      desc: '模板文本中的未配对引号不影响 script 块定位',
      vue: [
        '<template>',
        "  <p>don't panic</p>",
        '</template>',
        '<script setup>',
        'const panelIconMap = { directory: Folder }',
        '</script>',
        ''
      ].join('\n'),
      panels: "export const SIDEBAR_PANELS = [\n  { id: 'directory', label: '工作目录' }\n]\n",
      expect: 'pass'
    },
    {
      desc: 'CRLF 换行下行号准确',
      panels: FIXTURE_PANELS.replace(/\n/g, '\r\n'),
      vue: buildVue(ALIGNED_MAP.slice(0, 3)).replace(/\n/g, '\r\n'),
      expect: 'fail',
      expectHit: '缺少图标映射',
      expectLine: 5
    },
    {
      desc: 'SIDEBAR_PANELS 空数组失败',
      panels: 'export const SIDEBAR_PANELS = []\n',
      vue: buildVue([]),
      expect: 'fail',
      expectHit: '未解析到任何面板 id'
    },
    {
      desc: '面板 id 重复失败',
      panels: [
        'export const SIDEBAR_PANELS = [',
        "  { id: 'ai', label: 'A' },",
        "  { id: 'ai', label: 'B' }",
        ']',
        ''
      ].join('\n'),
      vue: buildVue(['  ai: MagicStick']),
      expect: 'fail',
      expectHit: '面板 id 重复'
    },
    {
      desc: '未找到 SIDEBAR_PANELS 声明 fail-closed',
      panels: 'export const OTHER = []\n',
      vue: buildVue([]),
      expect: 'fail',
      expectHit: '未找到 SIDEBAR_PANELS 数组声明'
    },
    {
      desc: '未找到 panelIconMap 声明 fail-closed',
      vue: '<script setup>\nconst panels = []\n</script>\n',
      expect: 'fail',
      expectHit: '未找到 `const panelIconMap = {` 声明'
    }
  ];

  let failed = 0;
  for (const c of cases) {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'sidebar-panel-icons-selftest-'));
    let result;
    try {
      const panelFile = writeFixture(dir, 'sidebarPanels.js', c.panels ?? FIXTURE_PANELS);
      const iconFile = writeFixture(dir, 'ActivityBar.vue', c.vue);
      result = runCheck(panelFile, iconFile);
    } finally {
      fs.rmSync(dir, { recursive: true, force: true });
    }
    const pass = result.problems.length === 0 && result.violations.length === 0;
    const ok = c.expect === 'pass' ? pass : !pass;
    const messages = [...result.problems, ...result.violations.map((v) => v.message)];
    const detailOk = !c.expectHit || messages.some((m) => m.includes(c.expectHit));
    const lineOk = !c.expectLine || result.violations.some((v) => v.line === c.expectLine);
    if (!ok || !detailOk || !lineOk) {
      failed++;
      console.error(`selftest FAIL: ${c.desc}`);
      if (!ok) {
        console.error(`  期望 ${c.expect}，实际 ${pass ? 'pass' : 'fail'}${messages.length ? '：' + messages.join('；') : ''}`);
      }
      if (!detailOk) {
        console.error(`  期望命中 ${c.expectHit}，实际：${messages.length ? messages.join('；') : '无违规'}`);
      }
      if (!lineOk) {
        console.error(`  期望行号 ${c.expectLine}，实际：${result.violations.map((v) => v.line).join(',') || '无违规'}`);
      }
    } else {
      console.log(`selftest ok: ${c.desc}`);
    }
  }

  const total = cases.length;
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

function argValue(name) {
  const idx = args.indexOf(name);
  if (idx < 0) {
    return undefined;
  }
  const value = args[idx + 1];
  if (!value || value.startsWith('--')) {
    console.error(`参数 ${name} 缺少取值`);
    process.exit(1);
  }
  return value;
}

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultPanelSource = path.resolve(scriptDir, '..', 'frontend', 'src', 'store', 'sidebarPanels.js');
const defaultIconSource = path.resolve(scriptDir, '..', 'frontend', 'src', 'components', 'ActivityBar.vue');
const panelFile = path.resolve(argValue('--panel-source') || defaultPanelSource);
const iconFile = path.resolve(argValue('--icon-source') || defaultIconSource);

const { problems, violations, panelCount } = runCheck(panelFile, iconFile);

if (problems.length > 0) {
  console.error('侧边栏面板图标映射检查失败：检查依据异常');
  for (const p of problems) console.error(`  - ${p}`);
  process.exit(1);
}

if (violations.length > 0) {
  console.error(`侧边栏面板图标映射检查失败（${violations.length} 处）：\n`);
  for (const v of violations) {
    console.error(`  - ${v.file}:${v.line}: ${v.message}（${v.hint}）`);
  }
  console.error('\n修复指引：SIDEBAR_PANELS 与 panelIconMap 必须双向一致——新增面板补图标键，删除面板同步清理残留键');
  console.error('规范见 docs/spec/prompt-library.md §5（面板注册点）');
  process.exit(1);
}

console.log(`侧边栏面板图标映射检查通过：SIDEBAR_PANELS ${panelCount} 项均命中 panelIconMap（无缺失、无残留）`);
