# Research: docx-preview 0.3 → 0.4 major 升级 breaking changes 与本仓适配面

- **Query**: docx-preview 0.3→0.4 major 升级的 breaking changes 与本仓适配面调研
- **Scope**: mixed（内部消费面 grep/read + 外部 npm registry / GitHub releases / tarball 逐版 diff）
- **Date**: 2026-10-21

## 结论速览

**公开消费面（renderAsync 签名 + 本仓 6 个 options 字段 + DOM 产物结构 + 默认样式文本）在 0.4.1 全部兼容，升级零代码改动**。0.4.0 标注的 `!!!BREAKING CHANGE!!!` 仅影响使用 undocumented API（内部 createElement 链路 / renderDocument 旧签名）的消费者，本仓不适用。唯一须改：`package.json` 声明 `~0.3.7` → `~0.4.1`（0.x tilde = minor+patch 全锁，跨 minor 必须显式改声明）。

## 现版本与目标版本

| 项 | 值 | 依据 |
|---|---|---|
| 当前声明 | `~0.3.7` | `frontend/package.json:45` |
| lock 实际 | 0.3.7（2025-09-30 发布） | `frontend/package-lock.json:2557` |
| 目标版本 | 0.4.1（latest，2026-09-21 发布） | npm registry dist-tags `latest: 0.4.1` |
| 中间版本 | 0.4.0（2026-07-07，标注 breaking） | npm registry 时间线 |
| 分档登记 | tilde 档（`scripts/check-dep-consistency.mjs:46`） | 声明须 `~` 开头，lock 须同 major.minor 且 ≥ 声明值（0.x 语义 = minor+patch 全锁），改声明为 `~0.4.1` 即过检，检查脚本零改动 |

数据来源：`https://registry.npmjs.org/docx-preview`（拉取时间 2026-10-21）；0.4.x 期间无其他发布。

## 本仓消费 API 清单

全仓 grep 确认：docx-preview 的**运行时消费仅一处**（`frontend/src/components/FilePreviewRenderer.vue`），另有测试 mock 两处。其余 `docx` 字样（ParamsEditor placeholder、fileIconMap 图标、vite.config.js 注释）与该库无关。

### 1. 导入方式（FilePreviewRenderer.vue:254）

```js
import { renderAsync } from 'docx-preview'
```

静态 import（顶部注释记载历史：曾用动态 import，wails dev 下 Vite optimizeDeps 预构建路径代理不完整致 fetch 失败，故改静态；0.3.x 已是 ESM + mjs，0.4.x 包结构不变，dist 目录产物形态一致）。

### 2. renderAsync 调用（FilePreviewRenderer.vue:828-835）

```js
await renderAsync(data, docxContainerRef.value, null, {
  className: 'docx',      // 渲染产物样式前缀（与库默认值相同，显式传）
  inWrapper: true,        // 库默认 true
  ignoreWidth: false,     // 库默认 false
  ignoreHeight: false,    // 库默认 false
  breakPages: true,       // 库默认 true
  experimental: true      // 库默认 false（本仓显式开启）
})
```

- 第 1 参：`Uint8Array`（base64 解码产物，`Blob | any` 形参接受）
- 第 3 参 styleContainer 显式传 `null` → 库内部 `styleContainer ?? bodyContainer`（两版实现一致，0.4.1 只是显式化），库默认 `<style>` 注入渲染容器自身
- 返回值 Promise 未消费（`await` 后即完成）
- 未使用渲染回调/`renderChanges`/`renderComments`/`useBase64URL`/`styles` 等 options

### 3. 样式覆盖（FilePreviewRenderer.vue:1583-1602）

依赖 docx-preview 生成 DOM 的两个锚点：

| 本仓选择器 | 依赖的库产物 |
|---|---|
| `.docx-container`（自有） | 渲染容器本体（非库产物） |
| `.docx-container :deep(.docx-wrapper)` | `${className}-wrapper` → `<div class="docx-wrapper">`（覆盖灰底/30px padding 为白底 0 padding） |
| `.docx-container :deep(.docx-wrapper > section)` | 每页 `<section class="docx">` 为 wrapper 直接子节点（覆盖阴影/margin-bottom） |

库注入默认样式 `renderDefaultStyle()` 生成 `.docx-wrapper { background: gray; padding: 30px; ... }` 与 `.docx-wrapper>section.docx { background: white; box-shadow: ...; margin-bottom: 30px; }`——0.3.7 与 0.4.1 **该样式文本逐字节相同**，本仓覆盖选择器继续命中。

### 4. 测试 mock（两处，依赖导出名 `renderAsync`）

| 文件 | 位置 | mock 形态 |
|---|---|---|
| `frontend/src/components/__tests__/FilePreviewRenderer.spec.js` | :20 | `vi.mock('docx-preview', () => ({ renderAsync: vi.fn(() => Promise.resolve()) }))` |
| `frontend/src/components/__tests__/ContentPanel.spec.js` | :50-52 | 同上形态（jsdom 下 stub 掉真实渲染） |

### 5. E2E

`frontend/e2e/` 全目录 grep `docx` **零命中**——无 docx 预览 E2E 选择器，无影响面。

## 0.4 breaking changes 逐条

来源：GitHub releases（VolodymyrBaydalka/docxjs）0.4.0 release notes + 两版 npm tarball 解包逐文件 diff（`dist/docx-preview.d.ts` 与 `dist/docx-preview.mjs`）。

### 0.4.0（2026-07-07，标注 `!!!BREAKING CHANGE!!!`）

| # | 变更 | 性质 | 对公开 API 的影响 |
|---|---|---|---|
| 1 | 内部 HTML 节点创建从 `createElement(...)` 链路整体重构为 `h({ })` hook（新 `HElement` 描述结构：tagName/className/style/children/ns） | breaking（release 原文：「could lead to issues in some edge cases, especially if you use undocumented API」） | 仅影响直接消费内部渲染类 / undocumented API 的调用方。`h()` 默认实现（mjs :2870-2898）行为等价旧路径：createElement / setAttribute class / Object.assign style / createTextNode / createComment / createDocumentFragment |
| 2 | Options 新增 `h: (elemOrText: HElement \| Node \| string) => Node`（d.ts 注释 experimental, subject to change），可拦截节点创建 | 新增字段（`Partial<Options>` 下可选，不传即用默认实现） | 纯增量，不传无行为差异 |
| 3 | 嵌套 smart tags 支持（PR #186）、README typo（PR #192） | 功能新增/文档 | 无破坏 |

### 0.4.1（2026-09-21，无 GitHub release 条目，npm tarball diff 确认为纯 bug fix）

| # | 变更（mjs diff 实测） | 性质 |
|---|---|---|
| 1 | 解析 `evenAndOddHeaders` 设置项；偶数页眉引用仅在设置开启时生效（0.4.0 对奇偶页判定有误） | bug fix |
| 2 | smart tag 子节点遍历 `c.localName` → `c?.localName`（null 安全） | bug fix |
| 3 | `#fragment` 节点 children 递归挂载（0.4.0 丢 children）；`#comment` children 缺省容错 | bug fix |
| 4 | 段落自身 `pageBreakBefore` 属性生效（0.4.0 只看样式表继承） | bug fix（渲染行为修正） |

### 关键公开 API 逐项 diff（0.3.7 → 0.4.1）

| API | 0.3.7 | 0.4.1 | 兼容性 |
|---|---|---|---|
| `renderAsync(data, bodyContainer, styleContainer?, userOptions?)` | d.ts 签名 | **逐字相同** | 完全兼容 |
| `parseAsync(data, userOptions?)` | d.ts 签名 | 逐字相同 | 完全兼容 |
| `renderDocument(document, bodyContainer, styleContainer?, userOptions?)` | 签名 | **签名变更**：`(document, userOptions?)`，返回 `Promise<Node[]>`（节点数组交调用方自行挂载，styleContainer 分流逻辑移入 renderAsync） | **breaking**，本仓未使用该 API，不受影响 |
| `defaultOptions` | 20 字段 | 20 个共有字段默认值逐字一致 + 新增 `h`（默认实现） | 完全兼容 |
| `Options` 接口 | 20 字段 | 20 字段全保留（无删无改名）+ 新增 `h` | 完全兼容 |
| 导出符号 | 4 个（defaultOptions/parseAsync/renderDocument/renderAsync） | 4 个 + 新增类型 `HElement` | 兼容（增量导出） |

### DOM 产物结构逐项 diff（本仓样式锚点）

| 产物 | 0.3.7 | 0.4.1 | 一致性 |
|---|---|---|---|
| wrapper | `renderWrapper`: `createElement("div", { className: \`${this.className}-wrapper\` }, children)` | `this.h({ tagName: "div", className: \`${this.className}-wrapper\`, children })` | 产物相同：`<div class="docx-wrapper">` |
| 每页 section | `createElement("section", { className })` + 内联 pageMargins padding + width/minHeight | `this.h({ tagName: "section", className, style })`，width/minHeight 逻辑逐字一致 | 产物相同：`<section class="docx">` 为 wrapper 直接子节点 |
| 库默认样式文本 | `.docx-wrapper { background: gray; padding: 30px; ... }` / `.docx-wrapper>section.docx { ... margin-bottom: 30px; }` | 逐字节相同 | 本仓两处 `:deep` 覆盖继续命中 |
| className 派生命名 | `${className}-wrapper` / `${className}-tab-stop` / `${className}-comments` 等 | 机制相同 | `className: 'docx'` 传参与默认值均不变 |

## 本仓影响判定表（含样式/E2E 选择器影响）

| # | 本仓消费点 | 判定 | 说明 |
|---|---|---|---|
| 1 | `import { renderAsync }`（静态导入） | 不受影响 | 0.4.1 仍导出 `renderAsync`，包 type/module 与 dist 产物形态不变 |
| 2 | `renderAsync(data, container, null, {...6 项})` | 不受影响 | 签名逐字一致；6 个 options 字段全保留且语义/默认值不变 |
| 3 | 第 3 参传 `null`（styleContainer） | 不受影响 | 0.4.1 renderAsync 显式 `styleContainer ?? (styleContainer = bodyContainer)`，与 0.3.7 实际行为一致（默认样式注入渲染容器） |
| 4 | `:deep(.docx-wrapper)` 样式覆盖 | 不受影响 | wrapper class 生成机制与库默认样式文本逐字一致 |
| 5 | `:deep(.docx-wrapper > section)` 样式覆盖 | 不受影响 | section 仍为 wrapper 直接子节点，class 仍 `${className}` |
| 6 | vitest 两处 `vi.mock('docx-preview')` | 不受影响 | mock 只依赖导出名 `renderAsync`，未变 |
| 7 | E2E | 不受影响 | e2e 目录无 docx 选择器（grep 零命中）；vitest.config.js 的 e2e exclude 面无涉 |
| 8 | `vite.config.js` optimizeDeps | 不受影响 | include 仅 `mermaid`；docx-preview 静态 import 不经预构建 include（静态导入不触发 optimizeDeps 重发现问题） |
| 9 | `package.json` 声明 | **须适配（唯一改动）** | `~0.3.7` → `~0.4.1`；0.x tilde = minor+patch 全锁，跨 minor 必须显式改声明并同步 lock（npm install）；tilde 档已登记（check-dep-consistency.mjs:46），`~0.4.1` 格式过检，脚本零改动 |
| 10 | 渲染 core 重构边缘风险 | 无法确证（低概率） | 0.4.0 release 自述「edge cases」风险针对 undocumented API 消费者，本仓不适用；但渲染 core 从 createElement 全面重写为 h()，复杂文档（嵌套表格/页眉页脚/批注等）无本仓单测覆盖，升级后须真机打开真实 docx 冒烟目检 |
| 11 | `renderDocument` 旧签名移除 | 不受影响 | 本仓从未调用；grep 确认仓内无引用 |

## 结论建议

**建议升级（改动成本 ≈ 0，一次声明变更 + 真机冒烟）。**

理由：

1. **公开 API 全兼容**：本仓唯一调用形态 `renderAsync(data, container, null, options)` 的签名、6 个 options 字段、返回值语义在 0.4.1 逐字兼容；`renderDocument` 的 breaking 变更本仓无引用。
2. **样式/E2E 零波及**：DOM 产物（`div.docx-wrapper > section.docx`）与库默认样式文本两版逐字节相同，本仓两处 `:deep` 覆盖与测试 mock 均不需动；E2E 无选择器面。
3. **0.4 标注的 breaking 与本仓无关**：breaking 实质是内部节点创建重构 + undocumented API 变化 + 新增实验性 `h` hook（可选），本仓纯公开 API 消费。
4. **有实际收益**：0.4.x 累计 bug fix——嵌套 smart tags 渲染、偶数页眉 evenAndOddHeaders 判定、段落 `pageBreakBefore` 生效、#fragment children 丢失等，均直接改善 docx 预览保真度；另获得 `h` hook 扩展位（当前不需要，不传即可）。
5. **风险与代价**：声明 `~0.3.7` → `~0.4.1` + lock 同步（一次 npm install）；渲染 core 重构的极端边缘 case 无静态论证手段，用真机冒烟兜底（打开含多页/表格/页眉页脚的 docx 各一，目检纸张分页与阴影样式）。
6. **回归验证面**：`npm run check-dep`（一致性检查 `~0.4.1` 过检）+ vitest（FilePreviewRenderer/ContentPanel 的 docx mock 用例）+ 真机 docx 预览冒烟；E2E 无须专跑 docx 面。

## Caveats / Not Found

- 0.4.1 无 GitHub release 条目（npm 发布未同步 release），其变更内容由 npm tarball 与 0.4.0 逐文件 diff 确认（共 35 行 diff，全部为上表 4 项 bug fix，无其他改动）。
- 0.4.0 release notes 中「issues in some edge cases」未给出具体场景清单，无法静态穷举；已通过「本仓不消费 undocumented API」+ 真机冒烟建议兜底。
- npm tarball 的 mjs 为已构建产物，与 GitHub 源码（ts 化重构）存在构建形态差异；本调研以 npm 产物为准（与本仓实际消费的产物同源）。
