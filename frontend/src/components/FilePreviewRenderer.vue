<template>
  <div class="file-preview-renderer" @contextmenu="onContextMenu">
    <!-- 图片预览：base64 → dataURL → <img>（读取失败/过大时无 base64，走降级分支） -->
    <div v-if="kind === 'image' && !error && !tooLarge" class="preview-image-wrap">
      <div class="image-toolbar">
        <el-button-group size="small">
          <el-button @click="zoomOut">缩小</el-button>
          <el-button @click="resetZoom">原始大小</el-button>
          <el-button @click="zoomIn">放大</el-button>
        </el-button-group>
        <span class="image-scale-label">{{ Math.round(imageScale * 100) }}%</span>
      </div>
      <div class="image-scroll" ref="imageScrollRef">
        <img
          v-if="imageDataUrl"
          :src="imageDataUrl"
          class="preview-image"
          :style="{ transform: `scale(${imageScale})` }"
          alt="图片预览"
        />
      </div>
    </div>

    <!-- Markdown 渲染：markdown-it（html:false 防 XSS） + highlight.js 代码块 + mermaid 图形 -->
    <div v-else-if="isMarkdown" class="preview-markdown-wrap">
      <!-- markdownBodyRef 同时容纳 frontmatter 属性面板与正文，使右键复制选区
           判断（readSelectionInPreview）与全选（selectAllChildren）无需改动即可覆盖面板 -->
      <div class="markdown-body" ref="markdownBodyRef">
        <!-- YAML frontmatter 属性面板（默认展开，置于正文上方；无 frontmatter 不渲染） -->
        <div v-if="frontmatterRaw" class="frontmatter-panel">
          <!-- 解析失败降级：复用已注册 hljs yaml 高亮 + 轻量提示，不影响正文 -->
          <div v-if="frontmatterParseFailed" class="fm-fallback">
            <p class="fm-fallback-tip">frontmatter 解析失败，以下为原文</p>
            <pre class="hljs"><code v-html="highlightedFrontmatterRaw"></code></pre>
          </div>
          <!-- 解析成功：key-value 属性表格；数组值 → el-tag 徽章，标量/对象 → 文本 -->
          <table v-else class="fm-table">
            <tbody>
              <tr v-for="entry in frontmatterEntries" :key="entry.key">
                <td class="fm-key">{{ entry.key }}</td>
                <td class="fm-value">
                  <template v-if="entry.isArray">
                    <el-tag
                      v-for="(t, i) in entry.value"
                      :key="i"
                      size="small"
                      class="fm-tag"
                    >{{ formatFmValue(t) }}</el-tag>
                  </template>
                  <span v-else class="fm-scalar">{{ formatFmValue(entry.value) }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="markdown-content" v-html="renderedMarkdown" @click="onMarkdownClick"></div>
      </div>

      <!-- 标题目录 TOC：默认隐藏，由父组件「目录」按钮控制显隐；右上角 X 关闭 -->
      <div v-if="showToc" class="preview-toc">
        <div class="toc-header">
          <span class="toc-title">目录</span>
          <el-icon class="toc-close-icon" title="关闭目录" @click="emit('closeToc')">
            <Close />
          </el-icon>
        </div>
        <ul v-if="toc.length > 0" class="toc-list">
          <li
            v-for="item in toc"
            :key="item.index"
            class="toc-item"
            :class="'toc-level-' + item.level"
            :title="item.text"
            @click="scrollToHeadingByIndex(item.index)"
          >
            {{ item.text }}
          </li>
        </ul>
        <div v-else class="toc-empty">无标题</div>
      </div>
    </div>

    <!-- 代码 / txt / json / sql 只读：CodeMirror 6 -->
    <div v-else-if="kind === 'text'" class="preview-codemirror-wrap">
      <div ref="cmHostRef" class="cm-host"></div>
    </div>

    <!-- Office：按扩展名子类型分发（docx / xlsx 系内嵌渲染，pptx 与旧格式降级） -->
    <div v-else-if="kind === 'office'" class="preview-office-wrap">
      <!-- Word .docx：docx-preview 内嵌渲染（中保真） -->
      <template v-if="officeSubType === 'docx'">
        <div v-if="officeError" class="preview-fallback">
          <p class="fallback-tip">{{ officeError }}</p>
          <el-button type="primary" @click="$emit('openExternal')">用默认程序打开</el-button>
        </div>
        <div
          v-else
          v-loading="officeLoading"
          ref="docxContainerRef"
          class="docx-container"
          element-loading-text="正在加载 Word 文档..."
        ></div>
      </template>

      <!-- Excel .xlsx/.xls/.csv：SheetJS 读 + el-table 渲染（只读，多 sheet 用 el-tabs） -->
      <template v-else-if="officeSubType === 'xlsx'">
        <div v-if="officeError" class="preview-fallback">
          <p class="fallback-tip">{{ officeError }}</p>
          <el-button type="primary" @click="$emit('openExternal')">用默认程序打开</el-button>
        </div>
        <div v-else v-loading="officeLoading" class="xlsx-wrap" element-loading-text="正在加载 Excel 文档...">
          <el-tabs v-if="xlsxSheets.length > 0" v-model="activeSheetName" class="xlsx-tabs">
            <el-tab-pane
              v-for="sheet in xlsxSheets"
              :key="sheet.name"
              :label="sheet.name"
              :name="sheet.name"
            >
              <!-- 空 sheet 提示 -->
              <div v-if="sheet.rows.length === 0" class="xlsx-empty-tip">该工作表为空。</div>
              <el-table
                v-else
                :data="sheet.dataRows"
                size="small"
                border
                height="100%"
                class="xlsx-table"
              >
                <el-table-column
                  v-for="(col, idx) in sheet.columns"
                  :key="idx"
                  :prop="col.prop"
                  :label="col.label"
                  min-width="100"
                  show-overflow-tooltip
                />
              </el-table>
            </el-tab-pane>
          </el-tabs>
          <div v-else class="xlsx-empty-tip">未解析到任何工作表数据。</div>
        </div>
      </template>

      <!-- PowerPoint .pptx/.ppt：保真较低，降级提示 + 打开 -->
      <div v-else-if="officeSubType === 'pptx'" class="preview-fallback">
        <p class="fallback-tip">PowerPoint 内嵌预览保真较低，建议用默认程序打开查看完整效果。</p>
        <el-button type="primary" @click="$emit('openExternal')">用默认程序打开</el-button>
      </div>

      <!-- 旧版 Office 格式 .doc/.dot/.odt/.odp/.ods/.rtf 等：前端库不支持，降级 -->
      <div v-else class="preview-fallback">
        <p class="fallback-tip">旧版 Office 格式暂不支持内嵌预览，请用默认程序打开。</p>
        <el-button type="primary" @click="$emit('openExternal')">用默认程序打开</el-button>
      </div>
    </div>

    <!-- PDF：iframe 加载 pdfjs 官方完整 viewer（POC-2，方案 B）。
         src=/pdfjs-viewer/web/viewer.html?file=<encoded /preview-pdf?path=...>，
         viewer 内部按 ?file= 拉取 PDF 字节（仍走后端 AssetServer 同源 handler），
         自带完整工具栏（翻页/缩放/搜索/缩略图/打印），保真度最高。
         主页面不 import pdfjs，靠 iframe 独立 browsing context 从架构上规避双实例。 -->
    <div v-else-if="kind === 'pdf'" class="preview-pdf-frame-wrap">
      <iframe
        v-if="pdfViewerSrc"
        :src="pdfViewerSrc"
        class="pdf-frame"
        frameborder="0"
      />
      <div v-else class="preview-fallback">
        <p class="fallback-tip">未提供 PDF 文件路径，无法预览。</p>
        <el-button type="primary" @click="$emit('openExternal')">用默认程序打开</el-button>
      </div>
    </div>

    <!-- 不支持 / 超大 / 损坏 -->
    <div v-else class="preview-fallback">
      <p class="fallback-tip">{{ fallbackMessage }}</p>
      <el-button type="primary" @click="$emit('openExternal')">用默认程序打开</el-button>
    </div>

    <!-- 右键复制菜单（仅 text/markdown 预览） -->
    <ul
      v-if="contextMenu.visible"
      class="context-menu"
      :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px' }"
      @click.stop
      @mousedown.stop
    >
      <li
        class="context-menu-item"
        :class="{ 'is-disabled': !contextMenu.canCopy }"
        @click="onMenuCommand('copy')"
      >
        <el-icon><CopyDocument /></el-icon>复制
      </li>
      <li class="context-menu-item" @click="onMenuCommand('selectAll')">
        <el-icon><Select /></el-icon>全选
      </li>
    </ul>
  </div>
</template>

<script setup>
import { ref, reactive, computed, watch, onBeforeUnmount, onMounted, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, Select, Close } from '@element-plus/icons-vue'
// Office 渲染依赖采用静态 import：
// 历史上曾用 await import('docx-preview') / await import('xlsx') 动态加载，
// 但 wails dev 下 Vite 会把动态 import 重写为 /node_modules/.vite/deps/xlsx.js
// 这类基于 location 解析的 optimizeDeps 预构建路径，而 Wails DevServer 对
// .vite/deps 的代理不完整，导致 fetch 失败（dev 特有，production 无此问题）。
// 改为静态 import 后 dev 与 production 行为一致，Office 才能在 dev 下验证。
// docx-preview / xlsx 不依赖 DOMMatrix/Worker，jsdom 测试环境下安全
// （测试已用 vi.mock stub 掉，见 ContentPanel.spec.js）。
import { renderAsync } from 'docx-preview'
import * as XLSX from 'xlsx'
import MarkdownIt from 'markdown-it'
import mermaid from 'mermaid'
// YAML frontmatter 解析（v4+ 默认 safe schema），用于 markdown 预览属性面板
import jsyaml from 'js-yaml'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import hljs from 'highlight.js/lib/core'
// 按需注册常用语言（控制打包体积）
import javascript from 'highlight.js/lib/languages/javascript'
import typescript from 'highlight.js/lib/languages/typescript'
import xml from 'highlight.js/lib/languages/xml'
import css from 'highlight.js/lib/languages/css'
import jsonLang from 'highlight.js/lib/languages/json'
import sql from 'highlight.js/lib/languages/sql'
import go from 'highlight.js/lib/languages/go'
import yaml from 'highlight.js/lib/languages/yaml'
import markdown from 'highlight.js/lib/languages/markdown'
import bash from 'highlight.js/lib/languages/bash'
import shell from 'highlight.js/lib/languages/shell'
import python from 'highlight.js/lib/languages/python'

hljs.registerLanguage('javascript', javascript)
hljs.registerLanguage('typescript', typescript)
hljs.registerLanguage('xml', xml)
hljs.registerLanguage('html', xml)
hljs.registerLanguage('css', css)
hljs.registerLanguage('json', jsonLang)
hljs.registerLanguage('sql', sql)
hljs.registerLanguage('go', go)
hljs.registerLanguage('yaml', yaml)
hljs.registerLanguage('markdown', markdown)
hljs.registerLanguage('bash', bash)
hljs.registerLanguage('shell', shell)
hljs.registerLanguage('python', python)

// CodeMirror 6 按需引入
import { EditorView, lineNumbers, highlightActiveLine, keymap } from '@codemirror/view'
import { EditorState } from '@codemirror/state'
import { defaultKeymap, historyKeymap, selectAll } from '@codemirror/commands'
import { syntaxHighlighting, defaultHighlightStyle, foldGutter, bracketMatching } from '@codemirror/language'
import { json as cmJson } from '@codemirror/lang-json'
import { sql as cmSql } from '@codemirror/lang-sql'
import { markdown as cmMarkdown } from '@codemirror/lang-markdown'
import { javascript as cmJs } from '@codemirror/lang-javascript'
import { html as cmHtml } from '@codemirror/lang-html'
import { css as cmCss } from '@codemirror/lang-css'

const props = defineProps({
  kind: { type: String, default: '' },
  fileName: { type: String, default: '' },
  // 文本内容（kind=text / markdown 用）
  content: { type: String, default: '' },
  // 图片的 base64 原始字节（不含 data: 前缀）
  base64: { type: String, default: '' },
  // 错误/超大/二进制等附加状态
  error: { type: String, default: '' },
  tooLarge: { type: Boolean, default: false },
  isBinary: { type: Boolean, default: false },
  // PDF 本地绝对路径（kind=pdf 时由父组件传入，用于拼装 /preview-pdf 同源 URL）
  pdfPath: { type: String, default: '' },
  // 当前预览文件的本地绝对路径（kind=text/markdown 时由父组件传入，
  // 用于把 markdown 内相对链接 ./other.md 解析为本地绝对路径）
  filePath: { type: String, default: '' },
  // markdown 目录（TOC）显隐：由父组件「目录」按钮控制，默认隐藏
  showToc: { type: Boolean, default: false }
})

const emit = defineEmits(['openExternal', 'openLink', 'closeToc'])

// ---------- 扩展名工具 ----------
const getExt = (name = '') => {
  const idx = name.lastIndexOf('.')
  return idx >= 0 ? name.slice(idx + 1).toLowerCase() : ''
}

const imageMimeMap = {
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  png: 'image/png',
  bmp: 'image/bmp',
  gif: 'image/gif',
  webp: 'image/webp',
  svg: 'image/svg+xml'
}

// CodeMirror 语言扩展按扩展名映射（未安装语言包的退化为纯文本只读）
const codeMirrorLangByExt = (ext) => {
  switch (ext) {
    case 'json': return cmJson()
    case 'sql': return cmSql()
    case 'md':
    case 'markdown': return cmMarkdown()
    case 'js':
    case 'mjs':
    case 'cjs': return cmJs()
    case 'ts': return cmJs()
    case 'html':
    case 'htm':
    case 'vue':
    case 'xml':
    case 'svg': return cmHtml()
    case 'css': return cmCss()
    default: return []
  }
}

const isMarkdown = computed(() => {
  if (props.kind !== 'text') return false
  const ext = getExt(props.fileName)
  return ext === 'md' || ext === 'markdown'
})

// ---------- Mermaid 初始化 ----------
// startOnLoad:false → 由我们在 DOM 更新后手动 run；securityLevel:'strict' 防注入。
mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', theme: 'default' })

// ---------- Markdown 渲染 ----------
const md = new MarkdownIt({
  html: false, // 关闭原始 HTML，防 XSS
  linkify: true,
  breaks: false,
  highlight(code, lang) {
    // mermaid 代码块不走 highlight.js：输出 <pre class="mermaid"> 保留原文，
    // 供 mermaid.run 在 DOM 更新后解析渲染为 SVG 图形。
    if (lang === 'mermaid') {
      return `<pre class="mermaid">${md.utils.escapeHtml(code)}</pre>`
    }
    const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
    try {
      return `<pre class="hljs"><code>${hljs.highlight(code, { language, ignoreIllegals: true }).value}</code></pre>`
    } catch {
      return `<pre class="hljs"><code>${md.utils.escapeHtml(code)}</code></pre>`
    }
  }
})

// ---------- YAML frontmatter 提取与解析 ----------
// markdown-it 默认不识别 frontmatter，文档开头的 ---\n...\n--- 块会被当作
// 普通正文（--- 变 <hr>，key: value 变段落文本）。这里用正则剥离 frontmatter
// 原文，正文再交 md.render / md.parse，frontmatter 则解析为结构化属性面板。
const FRONTMATTER_RE = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/

// strip 前导 BOM：仅用于 frontmatter 检测与正文剥离，避免 BOM 导致正则不匹配
const stripBom = (s) => (s && s.charCodeAt(0) === 0xFEFF ? s.slice(1) : s)

// frontmatter 原文（捕获组 1，去除外层 --- 分隔符）；无则空串
const frontmatterRaw = computed(() => {
  if (!isMarkdown.value) return ''
  const content = stripBom(props.content || '')
  const m = content.match(FRONTMATTER_RE)
  return m ? m[1] : ''
})

// 剥离 frontmatter 后的正文（供 md.render 与 md.parse 使用，保持 TOC 一致）
const markdownBody = computed(() => {
  if (!isMarkdown.value) return ''
  const content = stripBom(props.content || '')
  return content.replace(FRONTMATTER_RE, '')
})

// frontmatter 解析为对象（失败或非普通对象结构时返回 null）
const frontmatterData = computed(() => {
  const raw = frontmatterRaw.value
  if (!raw) return null
  try {
    const data = jsyaml.load(raw)
    // 仅认可普通对象；数组/标量等非对象结构降级为原文展示
    if (data === null || typeof data !== 'object' || Array.isArray(data)) return null
    return data
  } catch {
    return null
  }
})

// 是否解析失败（有 frontmatter 原文但未解析为普通对象 → 降级为 YAML 代码块）
const frontmatterParseFailed = computed(() => !!frontmatterRaw.value && frontmatterData.value === null)

// 属性面板表格行：预计算 key / value / 是否数组，供模板遍历
const frontmatterEntries = computed(() => {
  const data = frontmatterData.value
  if (!data) return []
  return Object.keys(data).map((key) => {
    const value = data[key]
    return { key, value, isArray: Array.isArray(value) }
  })
})

// 标量 / 嵌套对象 / 多行字符串的展示文本（数组元素也复用）
// - null/undefined → 空串
// - Date（js-yaml 把 YYYY-MM-DD 解析为 Date）→ 还原为 YYYY-MM-DD 原样
// - 嵌套对象 → JSON.stringify
// - 其余 → String(value)，不做类型转换
const formatFmValue = (v) => {
  if (v === null || v === undefined) return ''
  if (v instanceof Date) return v.toISOString().slice(0, 10)
  if (typeof v === 'object' && !Array.isArray(v)) return JSON.stringify(v)
  return String(v)
}

// 降级时复用已注册的 hljs yaml 高亮渲染 frontmatter 原文
const highlightedFrontmatterRaw = computed(() => {
  const raw = frontmatterRaw.value
  if (!raw) return ''
  try {
    return hljs.highlight(raw, { language: 'yaml', ignoreIllegals: true }).value
  } catch {
    return md.utils.escapeHtml(raw)
  }
})

const renderedMarkdown = computed(() => {
  if (!isMarkdown.value) return ''
  try {
    return md.render(markdownBody.value)
  } catch (e) {
    return `<p>Markdown 渲染失败：${String(e)}</p>`
  }
})

// ---------- Mermaid 图形渲染 ----------
// renderedMarkdown 更新（v-html 写入 DOM）后，对 .mermaid 节点调用 mermaid.run
// 渲染为 SVG。单个图表失败时降级为「渲染失败」提示，不影响其余内容与图表。
const renderMermaid = async () => {
  if (!isMarkdown.value || !markdownBodyRef.value) return
  const nodes = markdownBodyRef.value.querySelectorAll('pre.mermaid')
  if (nodes.length === 0) return
  try {
    await mermaid.run({ nodes: Array.from(nodes), suppressErrors: true })
  } catch (e) {
    // 兜底：mermaid.run 整体异常时，逐个标记未渲染成功的图表
    nodes.forEach((n) => {
      if (n.getAttribute('data-processed') !== 'true') {
        n.classList.add('mermaid-error')
        n.setAttribute('title', 'Mermaid 渲染失败：' + (e?.message || String(e)))
      }
    })
  }
}

// 内容变化 → 等 v-html 更新到 DOM 后再渲染 mermaid
watch(renderedMarkdown, async () => {
  await nextTick()
  renderMermaid()
})

// ---------- 标题目录 TOC ----------
// 用 markdown-it token 流提取 heading（层级 + 文本 + 文档内序号），
// 序号用于点击时按 DOM 顺序定位对应标题，规避重复标题文本的歧义。
const toc = computed(() => {
  if (!isMarkdown.value) return []
  let tokens
  try {
    tokens = md.parse(markdownBody.value, {})
  } catch {
    return []
  }
  const items = []
  let index = 0
  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i]
    if (t.type === 'heading_open') {
      const level = Number(t.tag.slice(1)) || 1
      const inline = tokens[i + 1]
      const text = inline && inline.type === 'inline' ? (inline.content || '').trim() : ''
      if (text) items.push({ level, text, index })
      index++
    }
  }
  return items
})


// 点击 TOC 项：按文档序号定位第 index 个标题，滚动到视图。
const scrollToHeadingByIndex = (index) => {
  if (!markdownBodyRef.value) return
  const headings = markdownBodyRef.value.querySelectorAll('h1, h2, h3, h4, h5, h6')
  const target = headings[index]
  if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// ---------- 图片预览 ----------
const imageDataUrl = computed(() => {
  if (props.kind !== 'image' || !props.base64) return ''
  const ext = getExt(props.fileName)
  const mime = imageMimeMap[ext] || 'image/octet-stream'
  return `data:${mime};base64,${props.base64}`
})

// ---------- PDF iframe URL（POC-2：pdfjs 官方完整 viewer） ----------
// 通过 iframe 加载静态资源 /pdfjs-viewer/web/viewer.html，由 viewer 内部用
// ?file= query 指定要打开的 PDF。PDF 本身仍由后端 AssetServer handler
// （/preview-pdf?path=）以同源 URL 提供，dev 与 build 两态行为一致。
//
// 路径拼装（注意双重 encode）：
//   1. pdfUrl = /preview-pdf?path=<encoded 本地路径>  —— 自身含 ?path= query
//   2. 把 pdfUrl 作为 viewer 的 ?file= 参数，整体 encodeURIComponent，
//      避免 pdfUrl 内的 ?/& 被 viewer 误解析为 viewer 自己的 query。
//
// 主页面不 import pdfjs：viewer 是 iframe 独立 browsing context，pdfjs 类
// （含 PagesMapper）只在 iframe 内定义一份，从架构上规避前端 pdfjs 双实例。
const pdfViewerSrc = computed(() => {
  if (props.kind !== 'pdf' || !props.pdfPath) return ''
  const pdfUrl = `/preview-pdf?path=${encodeURIComponent(props.pdfPath)}`
  return `/pdfjs-viewer/web/viewer.html?file=${encodeURIComponent(pdfUrl)}`
})

const imageScale = ref(1)
const imageScrollRef = ref(null)
const zoomIn = () => { imageScale.value = Math.min(5, imageScale.value + 0.2) }
const zoomOut = () => { imageScale.value = Math.max(0.2, imageScale.value - 0.2) }
const resetZoom = () => { imageScale.value = 1 }

// ---------- CodeMirror 只读文本 ----------
const cmHostRef = ref(null)
let cmView = null

const buildDoc = () => {
  const ext = getExt(props.fileName)
  if (ext === 'json') {
    try {
      return JSON.stringify(JSON.parse(props.content || ''), null, 2)
    } catch {
      return props.content || ''
    }
  }
  return props.content || ''
}

const setupCodeMirror = () => {
  if (cmView) {
    cmView.destroy()
    cmView = null
  }
  if (!cmHostRef.value) return
  const ext = getExt(props.fileName)
  const doc = buildDoc()
  const extensions = [
    lineNumbers(),
    foldGutter(),
    bracketMatching(),
    highlightActiveLine(),
    syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
    EditorState.readOnly.of(true),
    keymap.of([...defaultKeymap, ...historyKeymap]),
    codeMirrorLangByExt(ext)
  ]
  cmView = new EditorView({
    state: EditorState.create({ doc, extensions }),
    parent: cmHostRef.value
  })
}

// ---------- Office 子类型分发 ----------
// docx：Word 新格式（docx-preview 可渲染）
// xlsx：Excel 类（含 .xlsx/.xls/.csv，SheetJS 读 + el-table 渲染）
// pptx：PowerPoint（保真低，降级外部打开）
// legacy：旧版/其他 Office 格式（前端库不支持，降级外部打开）
const XLS_EXTS = new Set(['xlsx', 'xls', 'xlsm', 'xlsb', 'csv'])
const PPT_EXTS = new Set(['pptx', 'ppt', 'pptm', 'pps', 'ppsx'])
const LEGACY_OFFICE_EXTS = new Set([
  'doc', 'dot', 'dotx', 'docm',
  'odt', 'odp', 'ods', 'rtf'
])

const officeSubType = computed(() => {
  if (props.kind !== 'office') return ''
  const ext = getExt(props.fileName)
  if (ext === 'docx') return 'docx'
  if (XLS_EXTS.has(ext)) return 'xlsx'
  if (PPT_EXTS.has(ext)) return 'pptx'
  // 剩余扩展名（含 LEGACY_OFFICE_EXTS 与其他未知 office 扩展）统一走旧格式降级
  return 'legacy'
})

// ---------- base64 → ArrayBuffer 工具 ----------
const base64ToUint8Array = (base64) => {
  const binary = atob(base64)
  const len = binary.length
  const bytes = new Uint8Array(len)
  for (let i = 0; i < len; i++) bytes[i] = binary.charCodeAt(i)
  return bytes
}

// ---------- Word .docx 渲染（docx-preview，顶部静态 import） ----------
const docxContainerRef = ref(null)
const officeLoading = ref(false)
const officeError = ref('')

const renderDocx = async () => {
  // 清空旧渲染产物
  if (docxContainerRef.value) docxContainerRef.value.innerHTML = ''
  if (!props.base64) {
    officeError.value = 'Word 文档数据为空，无法预览。'
    return
  }
  officeLoading.value = true
  officeError.value = ''
  try {
    const data = base64ToUint8Array(props.base64)
    await renderAsync(data, docxContainerRef.value, null, {
      className: 'docx', // 渲染产物样式前缀
      inWrapper: true,
      ignoreWidth: false,
      ignoreHeight: false,
      breakPages: true,
      experimental: true
    })
  } catch (e) {
    officeError.value = 'Word 文档渲染失败：' + (e?.message || String(e)) + '，建议用默认程序打开。'
  } finally {
    officeLoading.value = false
  }
}

// ---------- Excel 渲染（SheetJS xlsx，顶部静态 import） ----------
const xlsxSheets = ref([]) // [{ name, rows, columns, dataRows }]
const activeSheetName = ref('')

const renderXlsx = async () => {
  xlsxSheets.value = []
  activeSheetName.value = ''
  if (!props.base64) {
    officeError.value = 'Excel 文档数据为空，无法预览。'
    return
  }
  officeLoading.value = true
  officeError.value = ''
  try {
    const data = base64ToUint8Array(props.base64)
    const workbook = XLSX.read(data, { type: 'array' })
    const sheets = []
    workbook.SheetNames.forEach((sheetName) => {
      const sheet = workbook.Sheets[sheetName]
      // header:1 → 二维数组（按行），仅取单元格值，不保留样式
      const rows = XLSX.utils.sheet_to_json(sheet, { header: 1, blankrows: false, defval: '' })
      if (!rows || rows.length === 0) {
        sheets.push({ name: sheetName, rows: [], columns: [], dataRows: [] })
        return
      }
      // 首行作为表头，其余作为数据行
      const headerRow = rows[0] || []
      const colCount = headerRow.length
      const columns = []
      for (let i = 0; i < colCount; i++) {
        columns.push({
          prop: 'c' + i,
          label: headerRow[i] === '' || headerRow[i] === null || headerRow[i] === undefined
            ? '列' + (i + 1)
            : String(headerRow[i])
        })
      }
      // 将每行数据映射为 { c0: v0, c1: v1, ... }，便于 el-table 按 prop 取值
      const dataRows = rows.slice(1).map((row) => {
        const obj = {}
        for (let i = 0; i < colCount; i++) {
          const v = row[i]
          obj['c' + i] = v === undefined || v === null ? '' : v
        }
        return obj
      })
      sheets.push({ name: sheetName, rows, columns, dataRows })
    })
    xlsxSheets.value = sheets
    if (sheets.length > 0) activeSheetName.value = sheets[0].name
  } catch (e) {
    officeError.value = 'Excel 文档解析失败：' + (e?.message || String(e)) + '，建议用默认程序打开。'
  } finally {
    officeLoading.value = false
  }
}

// ---------- 降级提示文案 ----------
const fallbackMessage = computed(() => {
  if (props.error) return '文件预览失败：' + props.error
  if (props.tooLarge) return '文件过大，暂不支持内嵌预览。'
  if (props.isBinary) return '二进制文件，无法内嵌预览。'
  if (props.kind === 'unsupported') return '暂不支持预览此文件类型。'
  return '暂不支持预览此文件。'
})

// ---------- 生命周期：随 props 变化重建对应渲染器 ----------
const renderOfficeBySubType = async () => {
  // 切换/重渲染前清空 office 内部状态与旧产物
  officeError.value = ''
  xlsxSheets.value = []
  activeSheetName.value = ''
  if (docxContainerRef.value) docxContainerRef.value.innerHTML = ''
  if (officeSubType.value === 'docx') {
    await nextTick()
    await renderDocx()
  } else if (officeSubType.value === 'xlsx') {
    await nextTick()
    await renderXlsx()
  }
  // pptx / legacy 无需渲染，模板走降级分支
}

watch(() => [props.kind, props.fileName, props.base64, props.content], async () => {
  // 销毁 CodeMirror（切到非 text 类型时）
  if (props.kind !== 'text' && cmView) {
    cmView.destroy()
    cmView = null
  }
  // 切到非 office 时，清空 office 渲染产物与状态
  if (props.kind !== 'office') {
    officeError.value = ''
    xlsxSheets.value = []
    activeSheetName.value = ''
    if (docxContainerRef.value) docxContainerRef.value.innerHTML = ''
  }
  if (props.kind === 'text') {
    await nextTick()
    setupCodeMirror()
  } else if (props.kind === 'image') {
    imageScale.value = 1
  } else if (props.kind === 'office') {
    await renderOfficeBySubType()
  }
}, { immediate: false })

// 组件挂载后首次初始化（父组件切到本组件时 props 已就绪）
onMounted(async () => {
  if (props.kind === 'text') {
    await nextTick()
    if (isMarkdown.value) {
      // markdown 走 v-html 分支，无 CodeMirror；首次挂载渲染 mermaid
      renderMermaid()
    } else {
      setupCodeMirror()
    }
  } else if (props.kind === 'office') {
    await renderOfficeBySubType()
  }
})

// ---------- markdown 链接拦截 ----------
// 不拦截 <a> 会触发顶层 window 原生导航：相对 ./other.md 解析到同源 URL 后，
// Wails AssetServer embed.FS 未命中 → fallback 到 PreviewHandler → 返回
// {"error":"缺少 path 参数"}，且 SPA 被该 JSON 文本整体替换、Vue 实例卸载，
// 用户无法返回主界面（只能重启 exe）。这里在点击时拦截，分发到三种去向。
const markdownBodyRef = ref(null)

// 外部协议白名单（http(s)/file/mailto/tel/ftp/data）。Windows 盘符 D: 不匹配。
const isExternalHref = (href) => /^(https?:|file:|mailto:|tel:|ftp:|data:)/i.test(href)

// markdown-it 默认会对链接做 percent-encoding（中文等非 ASCII），还原为原始字符，
// 避免 Windows 按 percent 编码字面名找不到中文文件名。decode 失败则回退原串。
const safeDecodeURIComponent = (s) => {
  try { return decodeURIComponent(s) } catch { return s }
}

// 把相对 href 解析为本地绝对路径，基准 = 当前预览文件所在目录。
// 统一正斜杠后用栈规范化 . 与 ..（.. 越过根时自然弹栈，路径无效由 PreviewFile 报错）。
const resolveAbsolutePath = (href) => {
  const base = (props.filePath || '').replaceAll('\\', '/')
  const slashIdx = base.lastIndexOf('/')
  const dir = slashIdx >= 0 ? base.slice(0, slashIdx) : ''
  const combined = (dir ? dir + '/' : '') + href.replaceAll('\\', '/')
  const stack = []
  for (const seg of combined.split('/')) {
    if (seg === '' || seg === '.') continue
    if (seg === '..') { stack.pop(); continue }
    stack.push(seg)
  }
  return stack.join('/')
}

// 标题文本 → slug（小写、空白转连字符，保留中文与字母数字），用于 #锚点匹配。
const slugifyHeading = (text) =>
  text.trim().toLowerCase().replace(/\s+/g, '-').replace(/[^\w一-龥-]/g, '')

// 在预览区内滚动到锚点对应标题。markdown-it 默认不给标题加 id，
// 这里即时按 slug（或原文）匹配，命中则 scrollIntoView。
const scrollToAnchor = (anchor) => {
  if (!anchor || !markdownBodyRef.value) return false
  const headings = markdownBodyRef.value.querySelectorAll('h1, h2, h3, h4, h5, h6')
  for (const h of headings) {
    const text = (h.textContent || '').trim()
    if (slugifyHeading(text) === anchor || text.toLowerCase() === anchor) {
      h.scrollIntoView({ behavior: 'smooth', block: 'start' })
      return true
    }
  }
  return false
}

const onMarkdownClick = (event) => {
  // 仅 markdown 预览启用（代码/图片等无 v-html 链接）
  if (props.kind !== 'text' || !isMarkdown.value) return
  const a = event.target.closest('a')
  if (!a) return
  const href = (a.getAttribute('href') || '').trim()
  if (!href) return
  // 阻止顶层 window 原生导航（崩溃根因）
  event.preventDefault()
  event.stopPropagation()

  if (isExternalHref(href)) {
    // 外部链接 → 系统默认浏览器打开，不在窗内导航
    BrowserOpenURL(href)
    return
  }
  if (href.startsWith('#')) {
    // 同文档锚点 → 预览区内滚动定位（中文锚点同样会被编码，先还原）
    scrollToAnchor(safeDecodeURIComponent(href.slice(1)))
    return
  }
  // 相对文件引用（./other.md 或 other.md，可能带 #锚点）：
  // 取文件部分解析为绝对路径，通知父组件在应用内切换预览（锚点暂不跨文件滚动）
  const hashIdx = href.indexOf('#')
  const filePart = hashIdx >= 0 ? href.slice(0, hashIdx) : href
  if (!filePart) return
  // 还原 percent-encoding（中文文件名）后再解析路径
  emit('openLink', resolveAbsolutePath(safeDecodeURIComponent(filePart)))
}

// ---------- 右键复制菜单（仅 text/markdown 预览） ----------
const contextMenu = reactive({
  visible: false,
  x: 0,
  y: 0,
  canCopy: false
})

// 最近一次预览区选区文本缓存。
// 背景：浏览器/CodeMirror 在右键 mousedown 时会清除或收缩 DOM 选区，
// 导致第二次右键（如先「全选」再右键「复制」）的 contextmenu 触发时
// DOM 选区已空，canCopy 误判为 false。这里借助 selectionchange 在选区
// 还存在时缓存文本，右键清除 DOM 选区后用缓存回退，保证复制可用。
let lastSelectedText = ''

// 读取预览区内的选区文本。
// 仅当选区 anchorNode 落在 CodeMirror 宿主（cmHostRef）或 markdown
// 容器（markdownBodyRef）内时返回其文本，否则返回空串，避免捕获预览
// 区外的选区。sel / anchorNode 可能为 null，需做空值保护。
const readSelectionInPreview = () => {
  const sel = window.getSelection()
  if (!sel) return ''
  const node = sel.anchorNode
  if (!node) return ''
  const host = cmHostRef.value
  const md = markdownBodyRef.value
  if (host && (host === node || host.contains(node))) return sel.toString()
  if (md && (md === node || md.contains(node))) return sel.toString()
  return ''
}

// selectionchange 回调：仅在选区非空时更新缓存。
// 右键清除选区也会触发 selectionchange（此时读到空），不覆盖缓存，
// 从而保留「全选」时缓存的文本，供后续右键复制回退使用。
const onSelectionChange = () => {
  const t = readSelectionInPreview()
  if (t) lastSelectedText = t
}

const onContextMenu = (event) => {
  // 仅文本/代码（CodeMirror）与 markdown 预览启用右键复制菜单
  if (props.kind !== 'text') return
  event.preventDefault()
  event.stopPropagation()

  const x = event.clientX
  const y = event.clientY
  contextMenu.x = x
  contextMenu.y = y
  // 优先取当前 DOM 选区；右键清除了 DOM 选区时回退到缓存，保证「先全选再右键复制」可用
  contextMenu.canCopy = !!(readSelectionInPreview() || lastSelectedText)
  contextMenu.visible = true

  // 菜单渲染后测量并做视口边界回退
  nextTick(() => {
    const menuEl = document.querySelector('.context-menu')
    if (!menuEl) return
    const rect = menuEl.getBoundingClientRect()
    let ax = x
    let ay = y
    if (ax + rect.width > window.innerWidth) ax = window.innerWidth - rect.width - 5
    if (ay + rect.height > window.innerHeight) ay = window.innerHeight - rect.height - 5
    if (ax < 5) ax = 5
    if (ay < 5) ay = 5
    if (ax !== x || ay !== y) {
      contextMenu.x = ax
      contextMenu.y = ay
    }
  })
}

const closeContextMenu = () => {
  contextMenu.visible = false
}

const copySelectedText = async () => {
  // 优先取当前 DOM 选区；为空时回退到缓存（右键已清除 DOM 选区的场景）
  // 写入剪贴板前去除前后空白，trim 后为空则跳过复制（不提示）
  const text = (readSelectionInPreview() || lastSelectedText).trim()
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}

const selectAllText = () => {
  if (cmView) {
    // CodeMirror：selectAll 命令选中全文
    selectAll(cmView)
    cmView.focus()
  } else if (markdownBodyRef.value) {
    // markdown：Selection API 选中正文容器全部内容
    const sel = window.getSelection()
    sel.removeAllRanges()
    sel.selectAllChildren(markdownBodyRef.value)
  }
}

const onMenuCommand = (command) => {
  closeContextMenu()
  if (command === 'copy') {
    if (contextMenu.canCopy) copySelectedText()
  } else if (command === 'selectAll') {
    selectAllText()
  }
}

const onGlobalClick = () => closeContextMenu()
const onGlobalContextMenu = () => closeContextMenu()

onMounted(() => {
  document.addEventListener('click', onGlobalClick)
  // capture 阶段监听：其他组件右键（如文件树）即便 stopPropagation 也能关闭本菜单，实现互斥
  document.addEventListener('contextmenu', onGlobalContextMenu, true)
  // 监听选区变化，缓存预览区内最近一次非空选区文本，用于右键清除选区后的复制回退
  document.addEventListener('selectionchange', onSelectionChange)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onGlobalClick)
  document.removeEventListener('contextmenu', onGlobalContextMenu, true)
  document.removeEventListener('selectionchange', onSelectionChange)
})

onBeforeUnmount(() => {
  if (cmView) { cmView.destroy(); cmView = null }
  // 卸载时清空 docx 渲染容器，避免 DOM 残留
  if (docxContainerRef.value) docxContainerRef.value.innerHTML = ''
})
</script>

<style scoped>
.file-preview-renderer {
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

/* 图片 */
.preview-image-wrap,
.preview-markdown-wrap,
.preview-codemirror-wrap {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.image-toolbar {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm, 8px);
  padding: 6px 8px;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  margin-bottom: var(--spacing-xs, 4px);
  flex-shrink: 0;
}

.image-scale-label {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 0 auto;
}

.image-scroll {
  flex: 1;
  overflow: auto;
  /* 白底内容区，与浅蓝容器层次分明 */
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-sm, 8px);
  min-height: 0;
}

.preview-image {
  max-width: 100%;
  max-height: 100%;
  transform-origin: center center;
  transition: transform 0.15s ease;
  object-fit: contain;
}

/* Markdown：正文 + 右侧 TOC 横向布局 */
.preview-markdown-wrap {
  overflow: hidden;
  flex-direction: row;
  gap: var(--spacing-sm, 8px);
}

/* TOC 侧边栏 */
.preview-toc {
  flex-shrink: 0;
  width: 180px;
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  overflow: hidden;
  transition: width var(--transition-fast, 0.15s ease);
}
.toc-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--border-color);
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 600;
  flex-shrink: 0;
  user-select: none;
}
.toc-close-icon {
  font-size: 15px;
  cursor: pointer;
  color: var(--text-tertiary);
  border-radius: var(--radius-sm, 4px);
  transition: all var(--transition-fast, 0.15s ease);
}
.toc-close-icon:hover {
  color: var(--danger-color, #f56c6c);
  background: var(--bg-tertiary);
}
.toc-empty {
  padding: 12px 10px;
  font-size: 12px;
  color: var(--text-tertiary);
  text-align: center;
}
.toc-list {
  list-style: none;
  margin: 0;
  padding: 6px 0;
  overflow-y: auto;
  min-height: 0;
}
.toc-item {
  padding: 4px 10px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-secondary);
  cursor: pointer;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  transition: all var(--transition-fast, 0.15s ease);
  border-left: 2px solid transparent;
}
.toc-item:hover {
  color: var(--primary-color);
  background: var(--primary-bg);
  border-left-color: var(--primary-color);
}
.toc-level-1 { padding-left: 10px; font-weight: 600; }
.toc-level-2 { padding-left: 20px; }
.toc-level-3 { padding-left: 30px; }
.toc-level-4 { padding-left: 40px; }
.toc-level-5 { padding-left: 50px; }
.toc-level-6 { padding-left: 60px; }

.markdown-body {
  flex: 1;
  overflow: auto;
  /* 白底内容区，与浅蓝容器层次分明 */
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  padding: var(--spacing-md, 16px);
  color: var(--text-primary);
  font-size: 14px;
  line-height: 1.7;
  min-height: 0;
}

/* YAML frontmatter 属性面板：置于正文上方，复用 .markdown-body 容器与 CSS 变量 */
.frontmatter-panel {
  margin-bottom: var(--spacing-md, 16px);
  padding: var(--spacing-sm, 8px) var(--spacing-md, 16px);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
}

.fm-table {
  width: 100%;
  border-collapse: collapse;
  margin: 0;
}

.fm-table tr {
  border-bottom: 1px solid var(--border-color);
}
.fm-table tr:last-child {
  border-bottom: none;
}

.fm-key,
.fm-value {
  padding: 6px 8px;
  vertical-align: top;
  font-size: 13px;
  line-height: 1.6;
}

.fm-key {
  width: 120px;
  color: var(--text-secondary);
  font-weight: 500;
  white-space: nowrap;
}

.fm-value {
  color: var(--text-primary);
  word-break: break-word;
}

/* 数组值 el-tag 徽章间距 */
.fm-tag {
  margin-right: 6px;
  margin-bottom: 4px;
}

/* 标量值文本：保留换行（多行字符串原样展示） */
.fm-scalar {
  white-space: pre-wrap;
}

/* 解析失败降级：YAML 高亮代码块 + 轻量提示 */
.fm-fallback {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs, 4px);
}

.fm-fallback-tip {
  margin: 0;
  font-size: 12px;
  color: var(--text-secondary);
}

.fm-fallback .hljs {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 10px;
  border-radius: 6px;
  overflow-x: auto;
  margin: 0;
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
}

.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3),
.markdown-body :deep(h4) {
  color: var(--text-primary);
  margin: 16px 0 8px;
  font-weight: 600;
}

.markdown-body :deep(h1) { font-size: 22px; }
.markdown-body :deep(h2) { font-size: 19px; }
.markdown-body :deep(h3) { font-size: 16px; }

.markdown-body :deep(p) { margin: 8px 0; }

.markdown-body :deep(code) {
  background: rgba(175, 184, 193, 0.2);
  padding: 2px 6px;
  border-radius: 3px;
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
}

.markdown-body :deep(pre) {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 6px;
  overflow-x: auto;
  margin: 8px 0;
}

.markdown-body :deep(pre code) {
  background: transparent;
  padding: 0;
  color: inherit;
}

/* Mermaid 图形块：居中白底，与暗色代码块区分 */
.markdown-body :deep(pre.mermaid) {
  background: var(--bg-secondary);
  color: var(--text-primary);
  text-align: center;
  padding: 12px;
  border: 1px solid var(--border-color);
  border-radius: 6px;
  overflow-x: auto;
}
.markdown-body :deep(pre.mermaid svg) {
  max-width: 100%;
  height: auto;
}
/* 渲染失败降级：显示红色边框与原始代码文本 */
.markdown-body :deep(pre.mermaid.mermaid-error) {
  background: #1e1e1e;
  color: #f44747;
  text-align: left;
  border-color: var(--danger-color, #f56c6c);
  font-family: Consolas, 'Courier New', monospace;
  white-space: pre-wrap;
}

.markdown-body :deep(table) {
  border-collapse: collapse;
  margin: 8px 0;
  width: 100%;
}

.markdown-body :deep(th),
.markdown-body :deep(td) {
  border: 1px solid var(--border-color);
  padding: 6px 10px;
}

.markdown-body :deep(blockquote) {
  border-left: 3px solid var(--primary-color, #409eff);
  margin: 8px 0;
  padding: 4px 12px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
}

.markdown-body :deep(a) {
  color: var(--primary-color, #409eff);
  text-decoration: none;
}

.markdown-body :deep(a:hover) {
  text-decoration: underline;
}

/* CodeMirror */
.cm-host {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  /* 白底内容区，与浅蓝容器层次分明 */
  background: var(--bg-secondary);
}

.cm-host :deep(.cm-editor) {
  height: 100%;
  font-size: 13px;
  font-family: Consolas, 'Courier New', monospace;
}

.cm-host :deep(.cm-scroller) {
  overflow: auto;
}

.cm-host :deep(.cm-gutters) {
  background: var(--bg-secondary);
  border-right: 1px solid var(--border-color);
}

/* Office 容器 */
.preview-office-wrap {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

/* docx-preview 渲染产物：白色纸张背景 + 可滚动 */
.docx-container {
  flex: 1;
  overflow: auto;
  background: #fff;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  padding: var(--spacing-md, 16px);
  min-height: 0;
}

.docx-container :deep(.docx-wrapper) {
  background: #fff;
  padding: 0;
}

.docx-container :deep(.docx-wrapper > section) {
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.12);
  margin-bottom: 16px;
}

/* Excel 渲染容器 */
.xlsx-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  /* 白底内容区，与浅蓝容器层次分明 */
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  padding: var(--spacing-xs, 4px);
}

.xlsx-tabs {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.xlsx-tabs :deep(.el-tabs__content) {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.xlsx-tabs :deep(.el-tab-pane) {
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.xlsx-table {
  flex: 1;
  min-height: 0;
}

.xlsx-empty-tip {
  padding: var(--spacing-md, 16px);
  color: var(--text-secondary);
  font-size: 13px;
  text-align: center;
}

/* PDF iframe 容器 */
.preview-pdf-frame-wrap {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.pdf-frame {
  flex: 1;
  min-height: 0;
  width: 100%;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  /* 白底，避免加载前露出浅蓝容器 */
  background: var(--bg-secondary);
}

/* 降级 */
.preview-fallback {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md, 16px);
  /* 白底内容区，与浅蓝容器层次分明 */
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  padding: var(--spacing-lg, 24px);
  min-height: 200px;
}

.fallback-tip {
  color: var(--text-secondary);
  font-size: 14px;
  text-align: center;
  margin: 0;
}
</style>

<style>
/* highlight.js 暗色主题（全局，供 v-html 代码块使用） */
.hljs { color: #d4d4d4; background: transparent; }
.hljs-comment, .hljs-quote { color: #6a9955; font-style: italic; }
.hljs-keyword, .hljs-selector-tag, .hljs-built_in, .hljs-name, .hljs-tag { color: #569cd6; }
.hljs-string, .hljs-title, .hljs-section, .hljs-attribute, .hljs-literal, .hljs-template-tag, .hljs-template-variable, .hljs-type, .hljs-addition { color: #ce9178; }
.hljs-number, .hljs-symbol, .hljs-bullet, .hljs-link, .hljs-meta, .hljs-selector-id, .hljs-selector-class { color: #dcdcaa; }
.hljs-variable, .hljs-attr { color: #9cdcfe; }
.hljs-deletion { color: #f44747; }
</style>
