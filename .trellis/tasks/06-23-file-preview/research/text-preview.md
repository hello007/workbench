# 研究：Vue3 + Element Plus 高质量只读文本预览方案

- **查询**：对 txt/json/sql/md 及常见代码（go/js/ts/yaml/xml/html/css 等）做高质量只读预览渲染；并处理"可编辑 textarea + SaveFile"现状的兼容取舍
- **范围**：外部（库选型）+ 内部（现状代码约束）
- **日期**：2026-06-23

## 一、现状与约束（来自代码）

### 关键实现位置

| 文件 / 位置 | 说明 |
|---|---|
| `frontend/src/components/ContentPanel.vue:120-133` | 当前预览渲染：`<el-input type="textarea" v-model="filePreview.content" :rows="15">` |
| `ContentPanel.vue:361-365` | `filePreview = { content, error }`（后端实际还返回 `tooLarge / isBinary`，见 `:550-554`） |
| `ContentPanel.vue:542-558` | `previewFile()`：调 `PreviewFile(path)`，把内容塞进 `filePreview`，并缓存 `originalContent` |
| `ContentPanel.vue:560-578` | `handleSave()`：调 `SaveFile(path, content)`；`handleCancelEdit()` 回滚 |
| `ContentPanel.vue:368-370` | `isContentModified = content !== originalContent`（脏检测靠原始字符串比对） |
| `ContentPanel.vue:580-597` | `watch(selectedNode)`：切换文件前若已修改，弹窗"放弃/继续编辑" |

### 现有数据契约
- 后端 `FilePreview.Content` 是 string（`frontend/wailsjs/go/main/App.d.ts`）。
- `PreviewFile` 已返回 `error / tooLarge / isBinary` 标志位，前端分支处理。
- 无前端文件大小阈值判定，依赖后端 `tooLarge`。

### 当前依赖（`frontend/package.json`）
`element-plus 2.13.7` / `vue 3.5.33` / `vue-router 4.6.4` / `@xterm/*`（已用 xterm 做终端） / `splitpanes` / `vue-draggable-plus`。**当前无任何 Markdown/高亮/代码编辑器依赖**，属净增。

---

## 二、Markdown 渲染：markdown-it vs marked

| 维度 | markdown-it 14.2.0 | marked 18.0.5 |
|---|---|---|
| 体积（unpacked） | ~777 KB（含插件/源码，打包后 gzip 约 30-40 KB） | ~450 KB（更轻，gzip 约 12-20 KB） |
| API | 同步 `md.render(src)`，插件生态成熟 | 同步 `marked.parse(src)`，更简洁 |
| 代码块高亮联动 | 官方推荐 `highlight` 选项接入 highlight.js / Shiki | 同样提供 `highlight` 钩子 |
| 安全（XSS） | 默认转义 HTML，可用 `html:false` | 默认转义 HTML |
| 扩展（GFM 表格/任务列表/锚点） | 插件化，按需引入 | 内置 GFM，配置项少 |

**建议：markdown-it**。理由：
1. 插件化架构契合"按需"（仅引入 markdown-it + 仅引入所需 highlight.js 语言），打包体积可控。
2. 与 highlight.js / Shiki 的 `highlight(code, lang)` 联动有官方文档示例，最成熟。
3. 同步 API，无需在 Vue 组件里处理异步渲染（Shiki 的异步高亮需额外包装）。

### XSS 安全结论（重点）
- **md 来源是本地文件，但本地文件 ≠ 可信**：用户仓库里的 md 可能含他人提交的恶意内容（如 `<img src=x onerror=...>`、`<script>`、`javascript:` 链接）。Wails 用系统 webview 渲染，DOM 注入即生效。
- markdown-it 的 `html:true` 选项会原样输出 HTML，**必须配合 DOMPurify**。
- **推荐做法**：`markdown-it` 设 `html: false`（默认）即可挡住绝大多数原始 HTML 注入；若需允许部分内联 HTML（如 `<sub>`/`<kbd>`），再用 **DOMPurify 3.4.11** 做 `DOMPurify.sanitize(html, {USE_PROFILES:{html:true}})` 二次过滤。
- 仅 `html:false` 且不开放 raw HTML 时，可不引入 DOMPurify，降低体积。若产品上不展示外部 md，建议走这条路径。

---

## 三、代码高亮：highlight.js vs Shiki vs Prism

| 维度 | highlight.js 11.11.1 | Shiki 4.2.0 | Prism 1.30.0 |
|---|---|---|---|
| 体积（unpacked，全量语言） | ~5.4 MB | 核心很小，按语言加载 grammar+theme | ~2.1 MB（全量） |
| **实际打包体积** | 选 10-15 个常用语言，gzip 约 15-30 KB | 运行时按需 fetch grammar，bundle 极小但要处理异步/JSON 资源 | 选语言后 gzip 约 10-20 KB |
| 主题 | 内置 `github-dark / github-light / atom-one-dark` 等 CSS | TextMate 主题（VSCode 同款），最美 | 需单独引主题 CSS |
| 与 Vite | 直接 `import hljs from 'highlight.js/lib/core'` + 注册语言，**最顺** | 需 `shiki/bundle/web` 或 fine-grained imports，配置稍多 | ESM 导入 OK |
| sql/json/yaml 支持 | 良好 | 优秀（VSCode 级） | 良好 |
| Markdown 代码块联动 | markdown-it 官方示例首选 | 可联动但需异步 | markdown-it 首选 highlight.js/Prism |
| 渲染时机 | 同步 | 异步（创建 highlighter 是 async） | 同步 |

**建议：highlight.js**（与 markdown-it 联动最省事，同步渲染，Vite 打包最顺，桌面应用无需 Shiki 的 TextMate 精度）。
- 引入方式：`highlight.js/lib/core` + 手动 `registerLanguage` 注册 `go/javascript typescript json sql yaml xml html css markdown bash`，按需控制体积。
- 若追求"VSCode 级配色"且能接受异步/资源加载复杂度，可换 Shiki，但对本场景 ROI 不高。

---

## 四、代码只读查看器（行号/折叠/虚拟滚动）

| 维度 | CodeMirror 6 | Monaco 0.55 | Ace (vue3-ace-editor 2.2.4) |
|---|---|---|---|
| 体积 | 按需组装，核心 `@codemirror/view`(1.2MB unpacked)+`state`+`language`+单语言包，gzip 约 70-120 KB | 极大（unpacked 72 MB，打包后约 3-5 MB），需 worker | ace-builds unpacked 55 MB（含全量），按需也可压到约 200-400 KB |
| 只读 | `EditorState.readOnly.of(true)` | `readOnly:true` | `readOnly:true` |
| 行号/折叠/搜索 | 内置 gutter + `foldGutter()` + `search` | 全部内置，体验最好 | 全部内置 |
| **虚拟滚动/大文件** | **原生支持**（视口只渲染可见行），万行流畅 | worker 解析，极慢文件也行但启动重 | 原生支持，大文件流畅 |
| Vite 兼容 | ESM 原生，最佳 | 需 `vite-plugin-monaco-editor` + worker 配置，Wails webview 下 worker 路径易踩坑 | ESM OK，但 ace-builds 模块结构偏旧 |
| 语言包 | `@codemirror/lang-{json,sql,markdown,html,yaml,xml,css,python,cpp}` 全部存在 | 内置 | 内置 |

**建议：CodeMirror 6**。理由：
1. **只读大文件预览**首重虚拟滚动 + 启动开销，CodeMirror 6 体积/性能最佳平衡。
2. ESM + Vite 原生，无 worker，在 Wails 系统 webview 下最稳（Monaco 的 worker 在 file:///打包资源路径下常出问题）。
3. 同一套可覆盖 txt/代码/json/sql（json 有 `@codemirror/lang-json` 带折叠）。
4. Monaco 体量对本场景（仅预览）严重过剩，且 worker 配置在桌面打包环境易踩坑，**不推荐**。
5. Ace 作为备选，功能够用但生态/现代化程度不如 CodeMirror 6。

---

## 五、JSON 渲染：格式化 + 折叠树

| 方案 | 体积/复杂度 | 折叠树 | 推荐度 |
|---|---|---|---|
| `vue-json-viewer 2.2.22`（Vue2 起家，Vue3 兼容包） | 轻量 | 有 | 中（Vue3 适配需注意） |
| `vue3-json-viewer 2.4.1` | 轻量，Vue3 原生 | 有 | 中高 |
| CodeMirror 6 `@codemirror/lang-json` + `foldGutter` | 复用代码查看器，零额外依赖 | 语法折叠 | **高** |
| 自实现 el-tree | 自由但工作量大 | 自定义 | 低（不推荐重造） |

**建议**：JSON 直接复用 CodeMirror 6（`lang-json` + `foldGutter` + `indentUnit:2` + 预先 `JSON.stringify(obj,null,2)` 格式化）。
- 优点：与代码预览同一套组件、同一套主题，**零新增依赖**；折叠、搜索、行号天然具备。
- 若产品想要"对象树形浏览（点击 key 折叠子树）"的富交互，再叠加 `vue3-json-viewer`；纯"漂亮地看 JSON"用 CodeMirror 足够。

---

## 六、推荐组合（最终方案）

| 用途 | 选型 | 关键依赖 | 预估体积（gzip，仅按需语言） |
|---|---|---|---|
| Markdown → 富文本 | **markdown-it** | `markdown-it@14` | ~30 KB |
| Markdown 内代码块高亮 | **highlight.js (core + 按需语言)** | `highlight.js@11` | ~20-30 KB |
| 代码/txt/sql/yaml/xml/html/css 只读查看器（行号/折叠/虚拟滚动） | **CodeMirror 6** | `codemirror@6` + `@codemirror/{view,state,language,commands}` + `@codemirror/lang-{json,sql,markdown,html,yaml,xml,css,...}` | ~80-120 KB |
| JSON 格式化 + 折叠 | **复用 CodeMirror 6（lang-json + foldGutter）** | 同上 | 0（已含） |
| Monaco | **不引入** | — | — |
| DOMPurify | **可选**（md 关闭 raw HTML 时不引入；开放内联 HTML 时引入 `dompurify@3`） | — | ~20 KB |

**总新增 gzip 约 130-180 KB**，全部 ESM、同步渲染、无 worker，与 Wails/Vite 高度契合。

### 按扩展名分流（落点在 ContentPanel 预览区）
- `.md/.markdown` → markdown-it 渲染富文本 + highlight.js 高亮代码块 → `v-html`（须 sanitize）。
- `.json` → `JSON.parse` + `stringify(,,2)` → CodeMirror 6 只读（lang-json + foldGutter）。
- `.go/.js/.ts/.sql/.yaml/.yml/.xml/.html/.css/.sh/...` → CodeMirror 6 只读（按扩展名映射 language 包）。
- `.txt` 或未知文本 → CodeMirror 6 只读纯文本（不加 language 包，仍享有行号/折叠/虚拟滚动）。
- 二进制/超大文件 → 维持现状（`isBinary / tooLarge` 提示）。

---

## 七、"只读预览" vs "保留可编辑" 取舍建议（核心产品决策）

现状痛点：textarea 既可编辑又会触发脏检测与 SaveFile，与"预览"语义冲突；若直接换成只读高亮，会**丢失现有编辑保存能力**。

### 推荐方案：双模式切换（预览默认，编辑可选）
1. **新增一个 mode 状态**（如 `previewMode = 'view' | 'edit'`），默认 `'view'`（只读高亮/渲染）。
2. **只读视图**：按扩展名渲染（CodeMirror readOnly / markdown-it），不触发脏检测，不出现"已修改"指示。
3. **编辑模式**：用户点击「编辑」按钮（仅对文本类文件显示）→ 切回 `el-input textarea`（保留现有 `originalContent` 脏检测 + `handleSave` + `handleCancelEdit` 逻辑，零改动）。
4. **入口按钮调整**：现有 ContentPanel 文件操作区把"预览"拆为「预览（只读高亮）」「编辑」两个按钮，或在预览区顶部加 `<el-switch>` / `<el-radio-group>` 切换"预览/编辑"。

### 为什么不一刀切废弃编辑
- 后端 `SaveFile` 与脏检测逻辑成熟（含切换前未保存提示 `:580-597`），废弃是能力回退。
- 双模式实现成本低：只读组件与现有 textarea 并存，按 mode `v-if` 切换即可，互不污染 `originalContent`。

### 大文件虚拟滚动方案
- **CodeMirror 6 原生虚拟滚动**：只渲染可视区行，万行级文本流畅，无需额外库。
- **Markdown 富文本**：本身是文档流，浏览器原生滚动即可；超大 md（如 >1MB）建议先做"只渲染前 N 行 + 折叠"或直接提示过大（沿用后端 `tooLarge`）。
- **后端阈值**：建议后端 `PreviewFile` 对超阈值文本返回 `tooLarge`（前端已处理），避免把数 MB 字符串灌进 DOM/CodeMirror。

### 落地影响清单（仅预览区，`ContentPanel.vue:120-133`）
- 替换 textarea 块为 `<component :is="...">` 或多分支模板（markdown 渲染器 / CodeMirror 只读 / 编辑 textarea）。
- `filePreview.content` 仍作为统一数据源；只读组件只读它，编辑组件双向绑定它（沿用 `isContentModified`）。
- 新增依赖后需跑 `cd frontend && npm install`，并确认 `vite build`（wails build）产物体积可接受。

---

## 八、参考资料

- [markdown-it 官方](https://github.com/markdown-it/markdown-it) — `highlight` 选项接入 highlight.js
- [highlight.js 按需引入](https://highlightjs.org/usage/) — `highlight.js/lib/core` + `registerLanguage`
- [CodeMirror 6 官网](https://codemirror.net/) — 只读示例：`EditorState.readOnly.of(true)` + `foldGutter()`
- [DOMPurify](https://github.com/cure53/DOMPurify) — `USE_PROFILES` 白名单
- [Wails webview 注意事项](https://wails.io/) — Monaco worker 在打包资源路径下需特殊处理（故弃用 Monaco）

## 九、待确认 / 未决

- 是否需要"Markdown 实时预览分屏"（左源码右渲染）？默认方案为单一预览。
- 是否允许内联 HTML（决定是否引入 DOMPurify）。
- 超大文件阈值（建议产品定，例如 2MB），由后端 `tooLarge` 控制。
- 是否需要"复制代码"按钮、行号显隐、主题随系统明暗切换（CodeMirror 主题用 `thememirror 2.0.1` 或 `@codemirror/theme-one-dark`）——可作为后续增强。
