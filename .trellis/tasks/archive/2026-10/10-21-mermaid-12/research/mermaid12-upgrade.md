# mermaid 12 升前评估（API 消费清单 + breaking 核对）

> 调研日期：2026-10-21。来源：npm registry（mermaid@12.2.0 deps/engines）、官方 CHANGELOG.md v12.0.0 节（GitHub contents API 原文，存 `_v12.0.0-section.md`）、v12.2.0 dist/mermaid.d.ts（存 `_mermaid12.d.ts`）、本仓 grep。

## 1. 现状基线

| 项 | 值 |
|---|---|
| 声明 | `"mermaid": "~11.17.2"`（frontend/package.json:51，tilde 档） |
| lock 落点 | mermaid 11.17.2 / katex 0.16.47 / dompurify 3.4.17 |
| audit 残留 | 2 low：katex 0.11.0–0.18.1（GHSA-238p-pmpm-9mq7 prototype pollution）+ mermaid 10.9.0-rc.1–12.1.0「Depends on vulnerable versions of katex」 |
| 修复线 | npm audit 官方结论：`npm audit fix --force` 会装 mermaid@12.2.0（12.2.0 起 katex 依赖 `^0.18.0`，lock 落 0.18.11 > 0.18.1 漏洞区间） |

## 2. 本仓消费面（grep 全量，单组件）

唯一消费者：`frontend/src/components/FilePreviewRenderer.vue`（RepoFilterDialog 经它间接复用）。

| API | 位置 | 用法 |
|---|---|---|
| `import('mermaid')` default 导出 | FilePreviewRenderer.vue:419 | 懒加载 + 模块级缓存 |
| `mermaid.initialize({ startOnLoad:false, securityLevel:'strict', theme })` | :422-426 | 每次渲染前按主题重配，重复调用 |
| `mermaid.run({ nodes, suppressErrors: true })` | :551 | DOM 更新后对 `pre.mermaid` 节点渲染，整体 try/catch 兜底标 `.mermaid-error` |
| markdown-it fence 定制 | :438-439 | `lang==='mermaid'` → `<pre class="mermaid">escapeHtml(code)</pre>`（不经 highlight.js） |
| 主题切换重渲染 | :564-573 | 还原 `data-mermaid-code` 原文后重 run |
| 测试 | `__tests__/FilePreviewRenderer.spec.js` | `vi.mock('mermaid')` stub，断言 run 被调用 |

未消费：`mermaid.parse` / `mermaid.render` / `registerLayoutLoaders` / `defaultRenderer` 配置 / 内部 layout 导出。

## 3. v12.0.0 breaking 逐条与本仓命中判定

| # | breaking（CHANGELOG 原文要点） | 本仓命中 | 处置 |
|---|---|---|---|
| 1 | require ES2024, Safari 17.4+, Node 22.12+（engines 声明 node>=22.12） | 命中 CI：ci.yml/release.yml node-version '20'；本地 node v24.14.1 满足 | CI node 20→22（4 处）；运行时 WebView2/WebKitGTK 2.44+ 支持 ES2024，Linux 风险见 §6 |
| 2 | ELK 内置且为默认布局（flowchart/state/class/ER/requirement/use-case/agentflow 重排）；`mermaid.initialize({ layout: 'dagre' })` 可回退；ELK 在 ESM 构建为独立 chunk 仅按需拉取 | 命中（视觉行为变化） | initialize 加 `layout: 'dagre'` 保持 v11 渲染布局（预览场景视觉稳定性优先）；ELK chunk 永不触发 |
| 3 | `redux-color` 成默认主题、`neo` 成十类图默认 look；未显式设 theme+look 者外观变化；保持旧观感须 `mermaid.initialize({ theme:'default', look:'classic' })` 两键都设 | 命中：我们设了 theme（'default'/'dark'）但没设 look | initialize 加 `look: 'classic'`，配合既有 theme 保持 v11 外观 |
| 4 | `flowchart`/`class`/`state` 配置节的 `defaultRenderer` 选项删除（静默忽略不再生效）；顶层 `layout` 取代 | 未命中（从未设置） | 无需改 |
| 5 | 内部 layout 导出移除（`clearLayoutRenderState`/`createCommonLayoutRenderer`/`defaultMeasureLayout`/`paintLayoutData`） | 未命中 | 无需改 |
| 6 | `dist/mermaid.esm.min.mjs` 含 es-module-lexer 拒绝的语法；**按包说明符 `import 'mermaid'` 引用核心构建者不受影响** | 未命中（我们正是 `import('mermaid')` 包说明符） | 无需改 |
| 7 | `run({ suppressErrors })` 签名仍在（v12.2.0 d.ts 确认，默认 false，我们显式传 true） | 兼容 | 无需改 |
| 8 | `mermaid.init` 废弃（v10 起） | 未命中（用 run） | 无需改 |

Minor/Patch 抽查（12.0.0–12.2.0）：chevrotain 13 升级顺带消除 `lodash-es@4.17.23` 漏洞传递；12.1.0 `elk.orientFeedbackEdges` 新选项（我们钉 dagre 不受影响）；错误图现在显示真实错误信息（对 `.mermaid-error` 兜底链路是改善）。

## 4. 依赖树对照（mermaid@12.2.0 dependencies 摘录）

- `katex: ^0.18.0` → lock 预期落 0.18.11（漏洞区间 0.11.0–0.18.1 之外）→ **audit 清零可达**
- `dompurify: ^3.4.12` → 已在 3.4.17，兼容
- `elkjs: ^0.9.3` 新增（ELK 内置代价，ESM 独立 chunk）
- `marked: ^16.3.0` 替换旧 markdown 解析依赖（传递面，不与本仓 markdown-it 冲突）
- engines: `node >=22.12.0`

## 5. 升级操作决定

1. `package.json`：`~11.17.2` → `~12.2.0`（lock 预期落 12.2.0，tilde 重写规则 `~<lock minor>.0` 吻合）
2. `FilePreviewRenderer.vue` initialize 三键扩五键：`startOnLoad:false, securityLevel:'strict', layout:'dagre', look:'classic', theme`（视觉零回归策略；去 dagre/classic 即吃 v12 新观感，沉淀时写明）
3. CI：ci.yml 2 处 + release.yml 2 处 node-version '20'→'22'
4. 依赖分档登记：`scripts/check-dep-consistency.mjs` mermaid 从 tilde 档 `~11.17.2` 改 `~12.2.0`（须先核对脚本清单实际登记形态）

## 6. 风险与验证面

- **CI node 20 下 npm ci**：engines 不匹配默认仅 EBADENGINE 警告非错误；但 vite build 在 node 20 跑属「依赖声明不支持环境」，升 22 消除歧义
- **Linux webkit2gtk**：ES2024 需 WebKitGTK 2.44+（≈Safari 17.4 同代），Ubuntu 24.04 满足；本项目 Windows 优先，Linux 桌面端列为观察项
- **jsdom 测试**：vitest 已 stub mermaid，不触发真实加载，升级行为零影响
- **E2E 冒烟缺口**：现有 E2E 无 mermaid 渲染用例，按 PRD 补 1 条（markdown 预览含 mermaid 代码块 → `pre.mermaid svg` 出现）；jsdom 无布局引擎，真实渲染断言只能落 E2E
- **回归三类图**：流程图（layout dagre 保持）/ 时序图（sequence 在十类重主题清单内，theme+look 双显式保持）/ 甘特图（不在十类清单，天然不变）
