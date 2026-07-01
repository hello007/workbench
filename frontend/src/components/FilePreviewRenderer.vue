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

    <!-- Markdown 渲染：markdown-it（html:false 防 XSS） + highlight.js 代码块 -->
    <div v-else-if="isMarkdown" class="preview-markdown-wrap">
      <div class="markdown-body" ref="markdownBodyRef" v-html="renderedMarkdown"></div>
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
import { CopyDocument, Select } from '@element-plus/icons-vue'
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
  pdfPath: { type: String, default: '' }
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
    setupCodeMirror()
  } else if (props.kind === 'office') {
    await renderOfficeBySubType()
  }
})

// ---------- 右键复制菜单（仅 text/markdown 预览） ----------
const markdownBodyRef = ref(null)
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

/* Markdown */
.preview-markdown-wrap {
  overflow: hidden;
}

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
