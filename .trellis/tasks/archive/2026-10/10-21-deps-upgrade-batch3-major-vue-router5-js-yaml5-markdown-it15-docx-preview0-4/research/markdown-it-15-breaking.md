# Research: markdown-it 14 → 15 major 升级 breaking changes 与本仓适配面

- **Query**: 调研 markdown-it 14→15 major 升级的 breaking changes 与本仓适配面，产出落盘
- **Scope**: mixed（内部源码消费面 + 外部 v15 包源码 diff）
- **Date**: 2026-10-21

## 现版本与目标版本

| 项 | 值 | 来源 |
|---|---|---|
| 声明版本 | `~14.3.2`（tilde 档，锁 minor 放 patch） | `frontend/package.json` dependencies |
| lock 实际落点 | **14.3.3**（registry.npmjs.org tarball） | `frontend/package-lock.json` |
| 目标版本 | **15.0.3**（15.x 最新 patch，2026-10-16 发布） | npm registry |
| v15.0.0 发布时间 | 2026-07-30 | npm registry `time` 字段 |
| 15.x 版本序列 | 15.0.0 (07-30) / 15.0.1 (08-27) / 15.0.2 (09-11) / 15.0.3 (10-16) | npm registry |
| 相关依赖现状 | highlight.js ~11.12.0 / mermaid ~12.2.0 / js-yaml ~4.3.2 | `frontend/package.json` |

**调研方法说明**：本环境无法访问 github.com（raw/API/jsdelivr 均拦截），breaking changes 结论改用**两版 npm tarball 源码 diff** 取得（14.3.3 vs 15.0.3 全量解包比对，15.0.0 单独补拉对照），证据均为源码级，可信度等同于 CHANGELOG。官方 v15 起已不再随包发布 CHANGELOG.md 文件。

## 本仓消费 API 清单（双链分列）

全仓仅 4 个源文件直接引用 markdown-it，**零 markdown-it 插件依赖**（无 markdown-it-anchor / markdown-it-container / markdown-it-front-matter / markdown-it-emoji 等，`package.json` 逐项核对确认）。anchor、TOC、frontmatter、slugify 均为本仓自实现。

### 链 1：AI 对话气泡渲染链（`frontend/src/utils/chatMarkdown.js`）

| API | 用法位置 | 依赖细节 |
|---|---|---|
| `new MarkdownIt(options)` | chatMarkdown.js:40 | `{html:false, linkify:true, breaks:true, highlight}` 四键 |
| `highlight(code, lang)` 回调 | chatMarkdown.js:44-51 | 只用前两参（第三参 attrs 未用）；返回 `<pre class="hljs">` 开头字符串；catch 分支回退 |
| `md.utils.escapeHtml` | chatMarkdown.js:49,107 | 高亮失败回退 + 选择题卡片属性/文本转义 |
| `md.render(text)` | chatMarkdown.js:62 | assistant 回复唯一渲染入口 |
| `md.renderer.rules.fence` 覆写 | chatMarkdown.js:71-73,143-154 | 先存 defaultFence（fallback 为 `(tokens, idx, options, env, self) => self.renderToken(...)`），覆写后按 info 分流 chat-question 卡片 |
| `token.info` / `token.content` | chatMarkdown.js:144-145,149 | info 取空格分隔首段为语言名；content 取围栏原文做 JSON.parse |
| `self.renderToken` fallback | chatMarkdown.js:73 | defaultFence 兜底签名 `(tokens, idx, options, env, self)` |

### 链 2：文件预览渲染链（`frontend/src/components/FilePreviewRenderer.vue`）

| API | 用法位置 | 依赖细节 |
|---|---|---|
| `new MarkdownIt(options)` | FilePreviewRenderer.vue:437-454 | `{html:false, linkify:true, breaks:false, highlight}` 四键 |
| `highlight(code, lang)` 回调 | FilePreviewRenderer.vue:441-453 | **mermaid fence 拦截点**：`lang==='mermaid'` 时输出 `<pre class="mermaid">` 原文（依赖「返回值以 `<pre` 开头则跳过内部 wrapper」契约）；其余走 hljs，未识别语言回退 plaintext |
| `md.utils.escapeHtml` | FilePreviewRenderer.vue:445,451,526 | mermaid 原文保留 / 高亮失败 / frontmatter 降级三处 |
| `md.render(markdownBody)` | FilePreviewRenderer.vue:533（renderedMarkdown computed） | v-html 输出，try/catch 包裹 |
| `md.parse(markdownBody, {})` | FilePreviewRenderer.vue:601（toc computed） | TOC 提取：遍历 token 流取 `heading_open`（`t.tag.slice(1)` 定层级）+ 后邻 `inline` token 的 `content` |
| normalizeLink percent-encoding 行为 | FilePreviewRenderer.vue:988-992（间接） | 依赖 markdown-it 默认对非 ASCII 链接编码，前端 `decodeURIComponent` 还原中文文件名 |
| `md.validateLink`（间接） | 链接拦截链 | 默认协议黑名单行为影响 `<a href>` 产出 |

### 辅助消费面（测试）

| 文件 | 性质 |
|---|---|
| `frontend/src/components/__tests__/FilePreviewRenderer.spec.js` | **不 mock markdown-it，真实实例跑断言**：锚点定位（:123）、中文链接 percent-encoding 还原（:139-148）、mermaid fence 输出 pre.mermaid（:314）、mermaid.run 调用（:325）、普通代码块不误入 mermaid 分支（:335） |
| `frontend/src/utils/__tests__/chatMarkdown.spec.js` | 真实实例：plaintext 回退、html:false 转义、chat-question 卡片 10 条用例（合法/非法 JSON/未闭合围栏/info 附注不误判） |
| `frontend/e2e/markdown-mermaid.spec.js` | 真实浏览器端到端：三类 mermaid 图同屏真实出 SVG（v12 升级专项哨兵） |
| `frontend/src/components/RepoFilterDialog.vue` | 仅复用 FilePreviewRenderer 组件，不直接触 markdown-it API |

## v15 breaking changes 逐条

以下每条均经两版 tarball 源码 diff 确认（14.3.3 ↔ 15.0.3）。

### 1. 包结构与 exports 重构（破坏面最大，但本仓不在射程）

- v14：`exports` 含 `"./*"` 通配，且 npm 包带完整 `lib/` 源码目录（rules_block/rules_inline/rules_core 等 40+ 文件）。
- v15：`exports` 仅 `"."` 与 `"./browser"` 两个入口；产物为单 bundle（`dist/markdown-it.mjs` ESM / `dist/markdown-it.cjs.js` CJS / `dist/browser/` 预压缩产物）；`lib/` 源码目录**不再发布**。
- **影响**：任何 `import ... from 'markdown-it/lib/xxx.mjs'` 深路径导入直接失效。本仓两链均为 `import MarkdownIt from 'markdown-it'` 主入口导入 → 不受影响。

### 2. 传递依赖全线 major 跳版

| 依赖 | v14 | v15 | 备注 |
|---|---|---|---|
| entities | ^4.5.0 | ^8.1.0 | HTML entity 解码（`md.utils` 内 decodeHTML），API 稳定 |
| linkify-it | ^5.0.2 | ^6.1.0 | linkify:true 生效面；6.1.0 无 engines 限制，导出形态常规（已核 package.json） |
| uc.micro | ^2.1.0 | ^3.0.0 | 正则字符集数据 |
| argparse | ^2.0.1 | ^3.0.3 | **两版源码 0 引用**（纯声明残留，对本仓零影响） |
| mdurl / punycode.js | ^2.x / ^2.3.1 | 不变 | normalizeLink 编码行为基础未动 |

- **影响**：均为 Vite 打包的标准 ESM，现代浏览器目标无兼容问题；lock 会新增 4 个传递依赖节点。

### 3. `md.utils` 导出面收缩

- v14 导出：`lib, assign, isString, has, unescapeMd, unescapeAll, isValidEntityCode, fromCodePoint, escapeHtml, arrayReplaceAt, isSpace, isWhiteSpace, isMdAsciiPunct, isPunctChar, isPunctCharCode, escapeRE, normalizeReference, asciiTrim`。
- v15 导出：保留其中绝大多数，**移除 `assign` / `has` / `isString` 三个**（TS 重写后改用原生实现），新增 `callable`。
- **影响**：本仓只消费 `md.utils.escapeHtml`（两链共 5 处）→ 不受影响。若未来新增插件用到这三个工具函数需注意。

### 4. default 导出改为 callable 兼容包装

- v15 的 `export default` 是 `callable(MarkdownIt)` 包装：无 `new` 调用（`MarkdownIt()`）仍可用，但**官方 JSDoc 明示「legacy wrapper 可能在未来 release 移除」**。
- **影响**：本仓两链均为 `new MarkdownIt({...})` 规范调用 → 不受影响；升级时无动作，长期保持 new 写法即可。

### 5. TypeScript 重写 + 内置类型声明

- v15 新增 `dist/markdown-it.d.mts` / `.d.cts`，exports 带 types 条目。
- **影响**：本仓为纯 JS 前端，无影响（IDE 提示反而受益）。

### 6. 默认选项与 presets：**无变化**

- `default` preset 逐键比对一致：`html:false, xhtmlOut:false, breaks:false, langPrefix:'language-', linkify:false, typographer:false, quotes, highlight:null, maxNesting:100`；`zero` preset 同样保留；字符串 preset 加载（`configure('default')`）机制保留。
- **影响**：本仓显式传四键且与默认值方向一致 → 无默认值漂移风险。

### 7. highlight 回调：**签名与契约均不变**

- 签名仍为 `highlight(str, lang, attrs)` 三参（v15 源码 renderer fence 规则：`options.highlight(token.content, langName, langAttrs)`）。
- 关键契约逐字保留：**返回值以 `<pre` 开头则跳过内部 `<pre><code class="language-xxx">` wrapper**（`highlighted.indexOf("<pre") === 0` 分支）；返回空串/undefined 回退 `escapeHtml(token.content)`；langPrefix class 追加逻辑同构。
- **影响**：本仓两个 highlight 回调（`<pre class="hljs">` 与 `<pre class="mermaid">`）恰好重度依赖该契约 → **完全兼容，零改动**。

### 8. Renderer rules 覆写机制：**不变**

- 规则签名仍为 `(tokens, idx, options, env, self)`；`md.renderer.rules.fence = fn` 覆写与 `self.renderToken(tokens, idx, options)` fallback 均保留（源码文档示例同款）。
- **影响**：chatMarkdown.js 的 fence 分流实现原样工作。

### 9. Token 结构与 API：**不变**

- `Token` 类方法面齐全（attrSet/attrGet/attrJoin/attrDel/attrPush/attrIndex）；`heading_open` / `inline` token 类型与 `content`/`tag`/`info` 字段语义不变（源码 grep 确认）。
- **影响**：TOC token 流提取链、fence info 解析链安全。

### 10. parse / render / renderInline：**签名不变**

- `render(src, env = {})` / `renderInline(src, env = {})`；`parse(src, env)` 非字符串输入抛 `Input data should be a String`（与 v14 相同）。
- **影响**：无。

### 11. 链接三方法：**保留**

- `md.normalizeLink` / `md.validateLink` / `md.normalizeLinkText` 实例方法均保留（源码多处调用点确认）；normalizeLink 底层 mdurl 2.x 未动版，percent-encoding 行为无变化信号。
- **影响**：中文文件名链接 decode 还原链安全（且 vitest 有真实实例断言守护）。

### 12. patch 版本（15.0.0 → 15.0.3）：无新增 breaking

- diff 显示 684 行差异中主体为 JSDoc 注释删减与构建工具链版本（oxc runtime helper 0.142→0.154）；依赖范围微调（argparse ^3.0.0→^3.0.3 等）。行为级变化未见。
- 15.0.1 / 15.0.2 / 15.0.3 均为同版本线 patch 递进。

## 本仓影响判定表

| # | 本仓消费面 | v15 变化点 | 判定 | 所需动作 |
|---|---|---|---|---|
| 1 | `new MarkdownIt({html,linkify,breaks,highlight})` 两链 | 默认选项/preset 无变化；构造双形态保留 | **不受影响** | 无 |
| 2 | highlight 回调（hljs 高亮 + mermaid 拦截） | 三参签名、`<pre` wrapper 契约、空值回退全部不变 | **不受影响** | 无 |
| 3 | `md.renderer.rules.fence` 覆写 + renderToken fallback | 覆写机制与签名不变 | **不受影响** | 无 |
| 4 | `md.utils.escapeHtml`（5 处） | escapeHtml 保留（仅 assign/has/isString 被删） | **不受影响** | 无 |
| 5 | `md.render` / `md.parse`（TOC） | 签名不变、heading_open/inline token 结构不变 | **不受影响** | 无 |
| 6 | normalizeLink percent-encoding + 前端 decode | mdurl 2.x 未动版，行为不变 | **不受影响** | 无 |
| 7 | 主入口 `import MarkdownIt from 'markdown-it'` | exports 深路径通配删除 | **不受影响**（无深路径导入） | 无 |
| 8 | 无 `new` 裸调用 | callable 包装保留（未来可能删） | 不受影响 | 无（保持 new 写法） |
| 9 | package.json 声明 | major 版本号变化 | **需适配** | `~14.3.2` → `~15.0.3`；按 frontend-dep-upgrade 规范核对 `scripts/check-dep-consistency.mjs` 四清单中 markdown-it 条目声明值（tilde 档以 lock 实际落点 minor 重写声明） |
| 10 | lock 传递依赖树 | entities 8 / linkify-it 6 / uc.micro 3 / argparse 3 新节点 | **需适配**（机械性） | 重新 `npm install` 刷 lock；CI 步骤 2.5 依赖一致性检查过一遍 |
| 11 | 解析行为边缘用例（CommonMark 病态输入边界） | TS 重写，源码 diff 无法穷尽逐用例差异 | **无法确证零漂移** | 靠现成测试面兜底（见下方回归面），升级后全量跑 |
| 12 | markdown-it 插件生态 | — | **不适用** | 本仓零插件依赖，无插件兼容断裂面 |

**回归面（现成，升级后必跑）**：

- `cd frontend && npm test`（vitest 全量；FilePreviewRenderer.spec.js 与 chatMarkdown.spec.js 均以真实 markdown-it 实例断言，行为漂移会直接暴露）
- `cd frontend && npm run e2e -- markdown-mermaid.spec.js`（mermaid 真实出图哨兵）
- 手工冒烟：markdown 预览打开含 frontmatter / TOC / mermaid / 中文文件名相对链接的 .md 文件

## mermaid 协同影响评估

**结论：不受 markdown-it 15 升级影响。**

本仓 mermaid 渲染链（FilePreviewRenderer.vue）的职责切分：

```
markdown-it（fence 规则）──只负责──▶ highlight 回调拦截 lang==='mermaid'
                                      输出 <pre class="mermaid">原文（escapeHtml）
                                              │
DOM 更新（v-html 写入）                        ▼
前端 renderMermaid() ──▶ 动态 import('mermaid') ──▶ initialize({startOnLoad:false,
                                      securityLevel:'strict', layout:'dagre', look:'classic'})
                                      ──▶ mermaid.run({nodes, suppressErrors:true})
```

该链路对 markdown-it 的依赖点仅两处，v15 逐项核对均不变：

1. **fence 规则调用 highlight 时传入的 lang 字符串**（`'mermaid'`，取自 info 首段）——v15 fence 规则 info 解析逻辑（`unescapeAll(token.info).trim()` + 空格切分）与 v14 同构。
2. **「highlight 返回值以 `<pre` 开头则原样输出」契约**——本仓 mermaid 分支返回 `<pre class="mermaid">`，依赖该契约绕过代码 wrapper；v15 源码该判定逐字保留（`highlighted.indexOf("<pre") === 0`）。

mermaid 自身（`import('mermaid')` + `run({suppressErrors:true})` + `layout:'dagre'`/`look:'classic'` 视觉保持双键）位于 markdown-it 下游，其初始化与渲染完全不经过 markdown-it API；markdown-it 15 不改变任何 mermaid 消费的 DOM 形态（`pre.mermaid` 节点原样产出）。E2E 哨兵 `markdown-mermaid.spec.js`（三类图同屏真实出 SVG）在升级后照常充当该链路的权威回归面。

同理，AI 对话链的 chat-question fence 分流（chatMarkdown.js）只依赖 fence 覆写机制与 token.info/content，与 mermaid 链共享的「fence 分流」模式同样不受影响。

## 结论建议

**建议升级（目标 `~15.0.3`），预计零代码改动，属低风险升级。**

理由：

1. **消费面全兼容**：本仓两链消费的全部 API（构造 options、highlight 三参回调、`<pre` wrapper 契约、renderer.rules.fence 覆写、md.utils.escapeHtml、md.parse TOC、token 结构、链接三方法）经源码级 diff 逐项确认在 v15 不变；v15 的实质 breaking（exports 深路径删除、utils 三函数删除、callable 包装）全部不在本仓射程内。
2. **零插件断裂面**：本仓无任何 markdown-it 插件依赖，anchor/TOC/frontmatter 自实现，规避了 major 升级最常见的插件生态不兼容问题。
3. **收益明确**：内置 TypeScript 类型声明；传递依赖现代化（entities 4→8 / linkify-it 5→6）；14.x 依赖线（2024 年首发）进入维护尾声，跟进 major 可持续获得修复。
4. **代价极小**：代码零改动；仅需 package.json 声明横移（tilde 档 `~14.3.2` → `~15.0.3`，按 frontend-dep-upgrade 规范同步 check-dep-consistency.mjs 四清单）+ lock 刷新。
5. **风险可控**：唯一不可静态确证的是 CommonMark 边缘用例的解析行为微调（TS 重写），但本仓已有三层守护——vitest 真实实例断言（FilePreviewRenderer.spec.js / chatMarkdown.spec.js）、E2E mermaid 出图哨兵（markdown-mermaid.spec.js）、手工冒烟清单（frontmatter/TOC/mermaid/中文链接）——漂移会被立即暴露。

## Caveats / Not Found

- 官方 CHANGELOG.md 与 GitHub release notes 未能获取（本环境 github.com / raw / api / jsdelivr 全部不可达）；v15 起官方也不再随 npm 包发布 CHANGELOG 文件。本文 breaking changes 结论以**两版 npm tarball 全量源码 diff**为据，覆盖 API 面/包结构/默认值/渲染契约等结构性变化；但 CHANGELOG 中可能存在的纯行为叙述条目（如某边缘语法的 spec 对齐修正）无法逐条枚举，已列入上表 #11 以测试面兜底。
- markdown-it 15 的 `engines` 字段不存在（无 Node 版本硬约束，已核 15.0.3 package.json）；linkify-it 6.1.0 同样无 engines 限制。
- `argparse` 在 markdown-it 两版源码中均为 0 引用（纯依赖声明残留），升级后 lock 多一个无用节点，无害。
