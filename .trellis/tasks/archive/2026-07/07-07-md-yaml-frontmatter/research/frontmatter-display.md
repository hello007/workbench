# 研究：Markdown 预览中 YAML Frontmatter 的展示方案

- **查询**：为本项目（Wails + Vue3 桌面工作台）的文件预览组件调研 markdown frontmatter 展示方案
- **范围**：外部（主流工具惯例、插件生态）+ 内部（结合本项目 `FilePreviewRenderer.vue` 现状）
- **日期**：2026-07-07

---

## 1. 项目现状（内部）

### 1.1 依赖现状

| 依赖 | 版本 | 与本主题的关系 |
|---|---|---|
| markdown-it | ^14.2.0 | 渲染引擎，默认不识别 frontmatter |
| highlight.js | ^11.11.1 | 已注册 yaml 语言（`FilePreviewRenderer.vue:198,212`），可直接高亮 YAML |
| mermaid | ^11.16.0 | 不相关 |
| js-yaml | 未安装 | 解析 frontmatter 为对象时需要 |
| markdown-it-front-matter | 未安装 | 通过插件回调提取 frontmatter 时需要 |

### 1.2 渲染管线（`FilePreviewRenderer.vue:300-326`）

- `MarkdownIt` 配置 `html:false`（防 XSS）、`linkify:true`、`breaks:false`
- 自定义 `highlight(code, lang)`：mermaid 代码块特殊处理，其余走 hljs
- `renderedMarkdown` computed 调 `md.render(props.content)`，有 try/catch 降级
- TOC 通过 `md.parse()` token 流提取 heading（`FilePreviewRenderer.vue:357-378`）
- **关键约束**：若 frontmatter 不被剥离/识别，会被当作正文渲染（`---` 变 `<hr>`，`key: value` 变段落文本）

### 1.3 样式基础

- `.markdown-body` 容器已有表格、代码块、blockquote 样式
- 全局 `.hljs` 暗色主题已就绪（`FilePreviewRenderer.vue:1246-1254`），YAML 高亮可直接复用

---

## 2. 主流工具 frontmatter 展示惯例

### 2.1 各工具处理方式

| 工具 | 默认处理 | 说明 |
|---|---|---|
| GitHub | 隐藏 | 渲染 README/.md 时剥离 frontmatter，不输出到正文；部分字段（如 title）用于页面标题 |
| VSCode 内置预览 | 代码块/隐藏 | 内置预览器对 frontmatter 支持有限，通常不作为正文一部分；多数用户借助扩展（如 Markdown Preview Enhanced）增强 |
| Typora | YAML 代码块 | 文档顶部以带语法高亮的 YAML 代码块展示，可点击进入编辑；新版支持属性视图 |
| Obsidian | 结构化属性面板 | 阅读视图隐藏原始 YAML，渲染为 Properties 卡片，支持字段类型（Text/List/Number/Checkbox/Date） |
| VitePress | 不直接展示 | frontmatter 作为页面配置（title/description/layout），驱动 SEO、布局、侧边栏 |
| Docusaurus | 不直接展示 | frontmatter 作为元数据（title/tags/sidebar_position），博客列表用于渲染文章卡片 |
| Hexo/VuePress 主题 | 可折叠区块 | 部分主题在文章顶部以折叠区块展示 title/date/tags/categories |

### 2.2 归纳

主流展示模式可归纳为四类：

1. **隐藏**：frontmatter 仅作元数据，不渲染到正文（GitHub、VitePress、Docusaurus）
2. **结构化属性面板/表格**：解析后按字段渲染为表格或属性卡片（Obsidian Properties、Typora 属性视图）
3. **YAML 代码块**：保留原文，以带语法高亮的代码块展示（Typora 默认、VSCode 扩展）
4. **可折叠区块**：原文或解析后置于可折叠容器，默认收起（Hexo/VuePress 部分主题）

---

## 3. 展示模式分类与适用场景

| 模式 | 优点 | 缺点 | 适用场景 |
|---|---|---|---|
| 隐藏 | 正文干净；与 GitHub 行为一致 | 元数据不可见，需查看源码 | 面向终端用户渲染的文档站点 |
| 结构化表格/属性面板 | 可读性最佳；支持类型化展示（标签徽章、日期格式化） | 需引入 YAML 解析；需处理复杂结构与解析失败 | 笔记类、工作台类应用（本项目） |
| YAML 代码块 | 实现简单；保留原文；复用已有 hljs yaml 高亮 | 可读性一般；无法体现结构 | 编辑器预览、轻量展示 |
| 可折叠区块 | 兼顾可见与整洁；默认不占空间 | 交互成本；需 JS 控制展开 | 长文档、frontmatter 字段较多时 |

---

## 4. markdown-it 解析 frontmatter 的技术方案

### 4.1 方案对比

| 方案 | 依赖 | 提取方式 | 是否影响正文 | 推荐度 |
|---|---|---|---|---|
| markdown-it-front-matter 插件 | +1 包 | 回调接收 frontmatter 原文 | 自动剥离，不进正文 | 高 |
| 手动正则剥离 + md.render | 0 | 正则匹配开头 `---\n...\n---` | 需手动切除后再渲染 | 高 |
| markdown-it-meta | +1 包（依赖 js-yaml） | 挂到 `md.meta` 对象 | 自动剥离 | 低（维护停滞） |
| gray-matter | +1 包（Node 端为主） | 构建时解析为主 | 不适合浏览器运行时 | 不适用 |

### 4.2 markdown-it-front-matter 插件

- **包名**：`markdown-it-front-matter`
- **版本**：0.2.3（最新稳定版）
- **用法**：

```js
import frontMatter from 'markdown-it-front-matter'

let fmRaw = ''
md.use(frontMatter, (raw) => {
  fmRaw = raw  // raw 为去除外层 --- 分隔符的 frontmatter 原文
})
```

- **回调签名**：`(frontMatterContent: string) => void`，在 `md.render()` 执行期间被调用，接收去除外层 `---` 的 YAML 原文。多 frontmatter 块场景下可能有第二参数标识是否首个块（需以实际安装版本 README 为准）。
- **正文影响**：插件会消费 frontmatter token，使其不进入后续渲染，正文从 frontmatter 之后开始。
- **识别规则**：
  - 必须位于文档最开头（第 0 字符，前无空白/空行）
  - 起始分隔符 `---` + 换行
  - 结束分隔符 `---`（或 `...`）+ 换行
  - 容错：对 `--- ` 后随空格、CRLF 等有一定容忍度

### 4.3 手动正则剥离（零依赖）

```js
const FRONTMATTER_RE = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/

const frontmatterRaw = computed(() => {
  const m = (props.content || '').match(FRONTMATTER_RE)
  return m ? m[1] : ''
})

const renderedMarkdown = computed(() => {
  const body = (props.content || '').replace(FRONTMATTER_RE, '')
  return md.render(body)
})
```

- **优点**：零新依赖；frontmatter 提取与正文渲染解耦，computed 无副作用；与 Vue 响应式管线无时序耦合
- **局限**：正则需严格覆盖边界（CRLF、结束符后换行、文档仅 frontmatter 无正文等）；不解析内容

### 4.4 YAML 解析：js-yaml vs 正则

| 方式 | 体积 | 能力 | 适用 |
|---|---|---|---|
| js-yaml | ~33KB（gzip ~14KB） | 完整 YAML：数组、嵌套、多行字符串、类型推断 | 需结构化展示时 |
| 简单正则 | 0 | 仅扁平 `key: value` | 仅展示原文或极简字段 |

- **js-yaml 用法**：

```js
import yaml from 'js-yaml'
try {
  const data = yaml.load(fmRaw)  // 返回对象
} catch (e) {
  // 解析失败降级
}
```

- **正则局限**：无法处理块格式数组（`tags:\n  - a`）、嵌套对象、`|`/`>` 多行字符串、引号转义

### 4.5 与本项目 TOC 的兼容性

本项目 TOC 通过 `md.parse(props.content)` 提取 heading（`FilePreviewRenderer.vue:361`）。

- 若采用**手动正则剥离**：`md.parse` 也应传入剥离后的正文，避免 frontmatter 中的 `---` 被解析为 hr token 干扰（实际 frontmatter 内一般无 heading，但需统一处理）。
- 若采用 **markdown-it-front-matter 插件**：`md.parse` 会自动跳过 frontmatter，TOC 无需改动。

---

## 5. 复杂结构与边界情况

### 5.1 复杂结构展示建议

| 结构 | YAML 示例 | 展示建议 |
|---|---|---|
| 数组（块格式） | `tags:` 换行 `  - vue` 换行 `  - wails` | 标签徽章组件或逗号分隔 |
| 数组（流格式） | `tags: [vue, wails]` | 同上 |
| 嵌套对象 | `author:` 换行 `  name: liu` 换行 `  email: x@x` | 缩进子表格或二级区块 |
| 多行字符串 | `description: \|` 换行 `  多行内容` | 保留换行的 `<pre>` 或段落 |
| 日期 | `date: 2026-07-07` | 原样或格式化为 YYYY-MM-DD |
| 布尔 | `draft: true` | 复选框图标或状态徽章 |
| 数字 | `order: 3` | 原样 |

### 5.2 解析失败降级策略

1. `js-yaml.load` 包裹 try/catch
2. 失败时回退为带语法高亮的 YAML 代码块（复用已注册的 hljs yaml）
3. 顶部显示轻量警告标识（如"frontmatter 解析失败，以下为原文"）
4. 不影响正文渲染

### 5.3 类型识别

- YAML 原生支持日期、布尔、数字、null 的类型推断
- 展示时可加图标或徽章区分类型，但**避免过度类型化**（如把 `2026-07-07` 改写为"2026年7月7日"可能误导）
- 建议：保持原值展示，仅对数组/布尔做轻量视觉区分

### 5.4 识别规则容错

- 文档不以 `---\n` 开头 → 无 frontmatter，正常渲染
- `---` 后无匹配的结束分隔符 → 不视为 frontmatter，按正文处理（避免误吞正文）
- 结束分隔符 `...`（YAML 文档结束符）也应支持
- 文档仅 frontmatter 无正文 → 渲染空正文

---

## 6. 针对本项目的推荐

### 6.1 候选方案对比

| 方案 | 新依赖 | 实现复杂度 | 用户体验 | 体积影响 |
|---|---|---|---|---|
| A. 正则剥离 + YAML 代码块 | 无 | 低 | 朴素但清晰 | 0 |
| B. 正则剥离 + js-yaml + 结构化属性面板 | js-yaml | 中 | 接近 Obsidian Properties | ~33KB |
| C. markdown-it-front-matter + js-yaml + 可折叠属性面板 | markdown-it-front-matter、js-yaml | 中高 | 最佳，可折叠更整洁 | ~35KB |

### 6.2 推荐方案：B（正则剥离 + js-yaml + 结构化属性面板）

**理由**：

1. **定位匹配**：本项目是开发者工作台，预览项目 markdown 文档（README/docs/prd.md），frontmatter 多为 title/date/tags/status 等元数据，结构化展示符合"工作台"专业定位
2. **零插件风险**：正则剥离不依赖 markdown-it 插件回调时序，与现有 `renderedMarkdown` computed 管线解耦，避免插件回调与 Vue 响应式交互的潜在时序问题
3. **依赖可控**：仅引入 js-yaml（~33KB，桌面应用体积不敏感），js-yaml 是 YAML 解析事实标准，稳定可靠
4. **降级健壮**：解析失败回退为 YAML 代码块（复用已注册 hljs yaml），不影响正文
5. **与 TOC 兼容**：正则剥离后正文传入 `md.parse`/`md.render`，TOC 行为一致

**实现要点**：

- 正则提取 frontmatter 原文 + 剥离后正文
- js-yaml 解析为对象，遍历键值渲染为属性表格
- 数组渲染为标签徽章（`el-tag`），布尔渲染为状态徽章，其余原样
- 解析失败 try/catch 回退为 `<pre class="hljs"><code>...</code></pre>`
- 属性面板置于正文上方，可选折叠（`el-collapse`）默认展开

### 6.3 备选：方案 A（若希望零新依赖）

若团队希望严格零新依赖，可采用方案 A：正则剥离 + 直接渲染为 YAML 代码块。实现最简，复用已有 hljs yaml 高亮，但缺乏结构化可读性。

---

## 7. 关键代码片段（方案 B 落地参考）

```js
import yaml from 'js-yaml'

const FRONTMATTER_RE = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/

// 提取 frontmatter 原文
const frontmatterRaw = computed(() => {
  const m = (props.content || '').match(FRONTMATTER_RE)
  return m ? m[1] : ''
})

// 解析为对象（失败返回 null）
const frontmatterData = computed(() => {
  const raw = frontmatterRaw.value
  if (!raw) return null
  try {
    return yaml.load(raw)
  } catch {
    return null  // 解析失败标记
  }
})

// 是否解析失败（用于降级提示）
const frontmatterParseFailed = computed(() => {
  return !!frontmatterRaw.value && frontmatterData.value === null
})

// 渲染正文（剥离 frontmatter，避免 --- 变 <hr>）
const renderedMarkdown = computed(() => {
  const body = (props.content || '').replace(FRONTMATTER_RE, '')
  try {
    return md.render(body)
  } catch (e) {
    return `<p>Markdown 渲染失败：${String(e)}</p>`
  }
})

// 复用已注册的 hljs yaml 高亮（降级用）
const highlightYaml = (code) => {
  try {
    return hljs.highlight(code, { language: 'yaml' }).value
  } catch {
    return md.utils.escapeHtml(code)
  }
}
```

模板部分（属性面板，置于 `.markdown-body` 上方）：

```html
<div v-if="frontmatterRaw" class="frontmatter-panel">
  <el-collapse v-model="fmCollapse">
    <el-collapse-item title="Frontmatter" name="fm">
      <!-- 解析失败降级为代码块 -->
      <pre v-if="frontmatterParseFailed" class="hljs">
        <code v-html="highlightYaml(frontmatterRaw)"></code>
      </pre>
      <!-- 解析成功渲染为表格 -->
      <table v-else class="fm-table">
        <tr v-for="(v, k) in (frontmatterData || {})" :key="k">
          <td class="fm-key">{{ k }}</td>
          <td class="fm-value">
            <el-tag
              v-if="Array.isArray(v)"
              v-for="t in v"
              :key="t"
              size="small"
              style="margin-right: 4px"
            >{{ t }}</el-tag>
            <span v-else>{{ v }}</span>
          </td>
        </tr>
      </table>
    </el-collapse-item>
  </el-collapse>
</div>
```

---

## 8. Caveats / 待核实

- **markdown-it-front-matter 回调签名**：第二参数（多块标识）需以实际安装版本 README 为准；本推荐方案 B 不依赖该插件，规避此不确定性
- **VSCode 内置预览具体行为**：不同版本与扩展下 frontmatter 展示可能有差异，本文描述基于常见观察，落地时不必对齐 VSCode
- **js-yaml 安全性**：`yaml.load` 在 v3 旧版本有 unsafe 类型执行风险，v4+ 已默认 safe schema；建议安装 `js-yaml@^4`
- **正则边界**：`^---\r?\n` 要求文档首字符即为 `-`，前导 BOM/空格会导致不匹配；若项目存在 BOM 文件，需先 strip BOM
- **Obsidian Properties 字段类型**：Obsidian 1.4+ 引入，具体类型集合随版本演进，本项目不必完全对齐
- **嵌套对象展示**：方案 B 代码片段仅处理一层键值，嵌套对象会 `[object Object]`；落地时需递归渲染或对 object 类型走 `JSON.stringify`/子表格
