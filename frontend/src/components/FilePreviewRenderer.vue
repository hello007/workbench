<template>
  <div class="file-preview-renderer">
    <!-- 图片预览：base64 → dataURL → <img> -->
    <div v-if="kind === 'image'" class="preview-image-wrap">
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

    <!-- PDF 预览：pdf.js 渲染到 canvas -->
    <div v-else-if="kind === 'pdf'" class="preview-pdf-wrap">
      <div class="pdf-toolbar">
        <el-button-group size="small">
          <el-button :disabled="pdfPage <= 1" @click="prevPage">上一页</el-button>
          <el-button :disabled="pdfPage >= pdfPageCount" @click="nextPage">下一页</el-button>
        </el-button-group>
        <span class="pdf-page-label">第 {{ pdfPage }} / {{ pdfPageCount }} 页</span>
        <el-button-group size="small">
          <el-button @click="pdfZoomOut">缩小</el-button>
          <el-button @click="pdfResetZoom">100%</el-button>
          <el-button @click="pdfZoomIn">放大</el-button>
        </el-button-group>
      </div>
      <div class="pdf-canvas-scroll" ref="pdfScrollRef" v-loading="pdfLoading">
        <canvas ref="pdfCanvasRef" class="pdf-canvas"></canvas>
        <div v-if="pdfError" class="pdf-error-tip">{{ pdfError }}</div>
      </div>
    </div>

    <!-- Markdown 渲染：markdown-it（html:false 防 XSS） + highlight.js 代码块 -->
    <div v-else-if="isMarkdown" class="preview-markdown-wrap">
      <div class="markdown-body" v-html="renderedMarkdown"></div>
    </div>

    <!-- 代码 / txt / json / sql 只读：CodeMirror 6 -->
    <div v-else-if="kind === 'text'" class="preview-codemirror-wrap">
      <div ref="cmHostRef" class="cm-host"></div>
    </div>

    <!-- Office：阶段 2 占位 -->
    <div v-else-if="kind === 'office'" class="preview-fallback">
      <p class="fallback-tip">Office 文档预览开发中，暂时请用默认程序打开。</p>
      <el-button type="primary" @click="$emit('openExternal')">用默认程序打开</el-button>
    </div>

    <!-- 不支持 / 超大 / 损坏 -->
    <div v-else class="preview-fallback">
      <p class="fallback-tip">{{ fallbackMessage }}</p>
      <el-button type="primary" @click="$emit('openExternal')">用默认程序打开</el-button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onBeforeUnmount, onMounted, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import MarkdownIt from 'markdown-it'
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
import { defaultKeymap, historyKeymap } from '@codemirror/commands'
import { syntaxHighlighting, defaultHighlightStyle, foldGutter, bracketMatching } from '@codemirror/language'
import { json as cmJson } from '@codemirror/lang-json'
import { sql as cmSql } from '@codemirror/lang-sql'
import { markdown as cmMarkdown } from '@codemirror/lang-markdown'
import { javascript as cmJs } from '@codemirror/lang-javascript'
import { html as cmHtml } from '@codemirror/lang-html'
import { css as cmCss } from '@codemirror/lang-css'

// pdf.js v6：按需懒加载，避免顶层 import 在 jsdom 测试环境触发 DOMMatrix 未定义，
// 同时让 pdfjs 主库拆分为独立 chunk，非 PDF 场景不进入首屏包。
let pdfjsLibPromise = null
const loadPdfjs = async () => {
  if (!pdfjsLibPromise) {
    pdfjsLibPromise = Promise.all([
      import('pdfjs-dist'),
      import('pdfjs-dist/build/pdf.worker.min.mjs?url')
    ]).then(([lib, workerMod]) => {
      lib.GlobalWorkerOptions.workerSrc = workerMod.default
      return lib
    })
  }
  return pdfjsLibPromise
}

const props = defineProps({
  kind: { type: String, default: '' },
  fileName: { type: String, default: '' },
  // 文本内容（kind=text / markdown 用）
  content: { type: String, default: '' },
  // 图片/PDF 的 base64 原始字节（不含 data: 前缀）
  base64: { type: String, default: '' },
  // 错误/超大/二进制等附加状态
  error: { type: String, default: '' },
  tooLarge: { type: Boolean, default: false },
  isBinary: { type: Boolean, default: false }
})

defineEmits(['openExternal'])

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

// ---------- Markdown 渲染 ----------
const md = new MarkdownIt({
  html: false, // 关闭原始 HTML，防 XSS
  linkify: true,
  breaks: false,
  highlight(code, lang) {
    const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
    try {
      return `<pre class="hljs"><code>${hljs.highlight(code, { language, ignoreIllegals: true }).value}</code></pre>`
    } catch {
      return `<pre class="hljs"><code>${md.utils.escapeHtml(code)}</code></pre>`
    }
  }
})

const renderedMarkdown = computed(() => {
  if (!isMarkdown.value) return ''
  try {
    return md.render(props.content || '')
  } catch (e) {
    return `<p>Markdown 渲染失败：${String(e)}</p>`
  }
})

// ---------- 图片预览 ----------
const imageDataUrl = computed(() => {
  if (props.kind !== 'image' || !props.base64) return ''
  const ext = getExt(props.fileName)
  const mime = imageMimeMap[ext] || 'image/octet-stream'
  return `data:${mime};base64,${props.base64}`
})

const imageScale = ref(1)
const imageScrollRef = ref(null)
const zoomIn = () => { imageScale.value = Math.min(5, imageScale.value + 0.2) }
const zoomOut = () => { imageScale.value = Math.max(0.2, imageScale.value - 0.2) }
const resetZoom = () => { imageScale.value = 1 }

// ---------- PDF 预览 ----------
const pdfCanvasRef = ref(null)
const pdfScrollRef = ref(null)
const pdfLoading = ref(false)
const pdfError = ref('')
const pdfDoc = ref(null)
const pdfPage = ref(1)
const pdfPageCount = ref(0)
const pdfScale = ref(1.2)

const base64ToUint8 = (b64) => {
  const bin = atob(b64)
  const len = bin.length
  const bytes = new Uint8Array(len)
  for (let i = 0; i < len; i++) bytes[i] = bin.charCodeAt(i)
  return bytes
}

const renderPdfPage = async () => {
  if (!pdfDoc.value || !pdfCanvasRef.value) return
  pdfLoading.value = true
  pdfError.value = ''
  try {
    const page = await pdfDoc.value.getPage(pdfPage.value)
    const viewport = page.getViewport({ scale: pdfScale.value })
    const canvas = pdfCanvasRef.value
    const context = canvas.getContext('2d')
    // 高分屏适配
    const ratio = window.devicePixelRatio || 1
    canvas.width = Math.floor(viewport.width * ratio)
    canvas.height = Math.floor(viewport.height * ratio)
    canvas.style.width = `${Math.floor(viewport.width)}px`
    canvas.style.height = `${Math.floor(viewport.height)}px`
    context.setTransform(ratio, 0, 0, ratio, 0, 0)
    await page.render({ canvasContext: context, viewport }).promise
  } catch (e) {
    pdfError.value = 'PDF 页面渲染失败：' + (e?.message || String(e))
  } finally {
    pdfLoading.value = false
  }
}

const loadPdf = async () => {
  if (props.kind !== 'pdf' || !props.base64) return
  pdfLoading.value = true
  pdfError.value = ''
  try {
    // 切换到新 PDF 前，先销毁旧文档，避免内存泄漏
    if (pdfDoc.value) {
      try { pdfDoc.value.destroy() } catch {}
      pdfDoc.value = null
      pdfPageCount.value = 0
    }
    const lib = await loadPdfjs()
    const data = base64ToUint8(props.base64)
    // 注：CJK PDF 需要 cMap 资源。Wails 打包为 file:// + dist/assets，
    // 动态 cMap 资源路径较难处理，阶段 1 MVP 暂不配置 cMapUrl（纯英文 PDF 不受影响）。
    // 阶段 2/3 收尾时再按打包资源路径补 cMapUrl + standardFontDataUrl。
    const task = lib.getDocument({ data })
    pdfDoc.value = await task.promise
    pdfPageCount.value = pdfDoc.value.numPages
    pdfPage.value = 1
    await nextTick()
    await renderPdfPage()
  } catch (e) {
    pdfError.value = 'PDF 加载失败：' + (e?.message || String(e))
  } finally {
    pdfLoading.value = false
  }
}

const prevPage = async () => {
  if (pdfPage.value <= 1) return
  pdfPage.value--
  await renderPdfPage()
}
const nextPage = async () => {
  if (pdfPage.value >= pdfPageCount.value) return
  pdfPage.value++
  await renderPdfPage()
}
const pdfZoomIn = async () => { pdfScale.value = Math.min(4, pdfScale.value + 0.3); await renderPdfPage() }
const pdfZoomOut = async () => { pdfScale.value = Math.max(0.4, pdfScale.value - 0.3); await renderPdfPage() }
const pdfResetZoom = async () => { pdfScale.value = 1.2; await renderPdfPage() }

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

// ---------- 降级提示文案 ----------
const fallbackMessage = computed(() => {
  if (props.error) return '文件预览失败：' + props.error
  if (props.tooLarge) return '文件过大，暂不支持内嵌预览。'
  if (props.isBinary) return '二进制文件，无法内嵌预览。'
  if (props.kind === 'unsupported') return '暂不支持预览此文件类型。'
  return '暂不支持预览此文件。'
})

// ---------- 生命周期：随 props 变化重建对应渲染器 ----------
watch(() => [props.kind, props.fileName, props.base64, props.content], async () => {
  // 销毁 CodeMirror（切到非 text 类型时）
  if (props.kind !== 'text' && cmView) {
    cmView.destroy()
    cmView = null
  }
  // 销毁 PDF 文档
  if (props.kind !== 'pdf' && pdfDoc.value) {
    try { pdfDoc.value.destroy() } catch {}
    pdfDoc.value = null
    pdfPageCount.value = 0
  }
  if (props.kind === 'text') {
    await nextTick()
    setupCodeMirror()
  } else if (props.kind === 'pdf') {
    await nextTick()
    await loadPdf()
  } else if (props.kind === 'image') {
    imageScale.value = 1
  }
}, { immediate: false })

// 组件挂载后首次初始化（父组件切到本组件时 props 已就绪）
onMounted(async () => {
  if (props.kind === 'text') {
    await nextTick()
    setupCodeMirror()
  } else if (props.kind === 'pdf') {
    await nextTick()
    await loadPdf()
  }
})

onBeforeUnmount(() => {
  if (cmView) { cmView.destroy(); cmView = null }
  if (pdfDoc.value) {
    try { pdfDoc.value.destroy() } catch {}
    pdfDoc.value = null
  }
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
.preview-pdf-wrap,
.preview-markdown-wrap,
.preview-codemirror-wrap {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.image-toolbar,
.pdf-toolbar {
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

.image-scale-label,
.pdf-page-label {
  font-size: 12px;
  color: var(--text-secondary);
  margin: 0 auto;
}

.image-scroll {
  flex: 1;
  overflow: auto;
  background: var(--bg-tertiary);
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

/* PDF */
.pdf-canvas-scroll {
  flex: 1;
  overflow: auto;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  display: flex;
  justify-content: center;
  padding: var(--spacing-sm, 8px);
  min-height: 120px;
  position: relative;
}

.pdf-canvas {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  background: #fff;
}

.pdf-error-tip {
  color: var(--danger-color, #f56c6c);
  font-size: 13px;
  padding: var(--spacing-md, 12px);
}

/* Markdown */
.preview-markdown-wrap {
  overflow: hidden;
}

.markdown-body {
  flex: 1;
  overflow: auto;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  padding: var(--spacing-md, 16px);
  color: var(--text-primary);
  font-size: 14px;
  line-height: 1.7;
  min-height: 0;
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
  background: var(--bg-tertiary);
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
  background: var(--bg-tertiary);
  border-right: 1px solid var(--border-color);
}

/* 降级 */
.preview-fallback {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-md, 16px);
  background: var(--bg-tertiary);
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
