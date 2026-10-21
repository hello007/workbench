# mermaid 渲染契约（v12 升级沉淀，2026-10-21）

## 1. Scope / Trigger

markdown 预览的 mermaid 图表渲染链（`FilePreviewRenderer.vue` 单组件消费）。mermaid 11.17.2 → 12.2.0 major 升级时的消费面适配契约与后续 12.x 维护规则。

- 升级动机：katex 传递链漏洞清零（GHSA-238p-pmpm-9mq7，mermaid ≤12.1.0 依赖漏洞区间 katex 0.11.0–0.18.1；12.2.0 起 katex `^0.18.0` 落 0.18.11 出区间）
- 完整 breaking 核对表见任务存档：`.trellis/tasks/archive`（10-21-mermaid-12/research/mermaid12-upgrade.md）

## 2. 消费面签名（唯一合法 API 面）

消费面锁定三个入口，禁止扩面（`parse`/`render`/`registerLayoutLoaders`/内部 layout 导出均未消费）：

```js
// 懒加载（模块级缓存），仅 markdown 含 mermaid 代码块时触发动态 import
mermaidModule = (await import('mermaid')).default

// initialize 五键（可重复调用按主题重配）
mermaidModule.initialize({
  startOnLoad: false,        // DOM 更新后手动 run，禁自动执行
  securityLevel: 'strict',   // 防注入（仓库内任意 md 文件均经此渲染）
  layout: 'dagre',           // v12 视觉保持键（见 §3）
  look: 'classic',           // v12 视觉保持键（见 §3）
  theme: 'dark' | 'default'  // 随 resolvedTheme 切换
})

// 渲染（suppressErrors 必须显式 true，v12 默认改 false 会 throw）
await mermaid.run({ nodes, suppressErrors: true })
```

markdown-it fence 定制：`lang === 'mermaid'` 输出 `<pre class="mermaid">${escapeHtml(code)}</pre>`（不经 highlight.js）；渲染前保存 `data-mermaid-code` 原文，主题切换重渲染前还原（`mermaid.run` 会用 SVG 覆盖节点内容）。

## 3. Design Decision：视觉保持双键

**Context**：mermaid 12 两个默认值变化——① ELK 内置并取代 dagre 成为 flowchart/state/class/ER/requirement/use-case/agentflow 默认布局；② `redux-color`/`neo` 取代 `default`/`classic` 成为十类图默认 theme/look。用户仓库内已提交图表的渲染观感会在升级后整体漂移。

**Decision**：`layout: 'dagre'` + `look: 'classic'` 显式钉回 v11 行为（预览场景要求观感跨升级稳定）。**去掉这两个键即切换到 mermaid 12 新观感**（ELK 布局 + redux 配色），属有意的产品决策而非遗漏。

**代价**：ELK 以 1.46MB 独立 chunk 内置于 mermaid ESM 构建；钉 dagre 后该 chunk 永不触发加载，零首屏代价。dagre 布局由 mermaid 12 直接依赖 `dagre-d3-es 7.0.14` 提供，无需装 `@mermaid-js/layout-dagre`。

## 4. 升级与运行环境契约

| 项 | 契约 |
|---|---|
| 版本声明 | tilde 档 `~12.2.0`（lock 落点同 minor，`check-dep-consistency.mjs` 三维一致） |
| engines | mermaid 12 要求 `node >=22.12`；CI（ci.yml/release.yml 共 4 处）已升 node 22，**后续 CI node 版本不得低于 22** |
| 浏览器目标 | ES2024 / Safari 17.4+；WebView2（Chromium evergreen）满足；Linux 桌面 WebKitGTK 2.44+（Ubuntu 24.04）满足，更低版本发行版列为观察项 |
| audit | npm audit（官方 registry）当前 **0 vulnerabilities**；新基线记录见 [security-scan.md](security-scan.md) §5.2 |
| import 形态 | 必须 `import('mermaid')` 包说明符（核心构建）；禁直指 `dist/mermaid.esm.min.mjs`（v12 该文件含 es-module-lexer 拒绝的语法） |

## 5. 测试面

- **vitest**：`FilePreviewRenderer.spec.js` 以 `vi.mock('mermaid')` stub（jsdom 无布局引擎测不了真实渲染），断言仅到 `run` 被调用 + `pre.mermaid` 形态；initialize 新增键对 stub 无参数断言，天然兼容
- **E2E 冒烟**：`frontend/e2e/markdown-mermaid.spec.js`（真实 Chromium）——三类图（flowchart/sequence/gantt）同屏，断言 `pre.mermaid svg` `toHaveCount(3)` + 逐个 `toBeVisible` + `viewBox` 非空（真实布局坐标特征）；后续动 mermaid 渲染链必须保此用例绿
- **E2E mock 坑**：`PreviewFile` 覆盖值必须含 `kind: 'text'`（`model.FilePreview` 真实字段）——缺 kind 走 CodeMirror 文本分支，markdown/mermaid 分支静默不触发且无报错，mock 面是唯一暴露点

## 6. Wrong vs Correct

### Wrong

```js
// 直指 dist 文件（v12 esm.min.mjs 语法 vite es-module-lexer 拒绝）
import('mermaid/dist/mermaid.esm.min.mjs')

// E2E mock 缺 kind 字段 → 预览走文本分支，mermaid 用例永不触发
PreviewFile: { content: MD, error: '' }
```

### Correct

```js
import('mermaid')  // 包说明符 → 核心构建

PreviewFile: { content: MD, error: '', kind: 'text' }
```

## 7. Tests Required（改渲染链时的回归面）

1. `node scripts/check-dep-consistency.mjs`（tilde 三维一致）
2. vitest 全量（stub 面零回归）
3. `npx playwright test e2e/markdown-mermaid.spec.js`（真实渲染冒烟）
4. mermaid 版本变更时：npm audit 清零核对 + breaking changelog 核对（对照任务存档 research 文档模板）
