import xlsxIcon from '../assets/icons/xlsx.png'
import docxIcon from '../assets/icons/docx.png'
import txtIcon from '../assets/icons/txt.png'
import pdfIcon from '../assets/icons/pdf.png'
import pngIcon from '../assets/icons/png.png'
import markdownIcon from '../assets/icons/markdown.png'
import javaIcon from '../assets/icons/java.png'
import pythonIcon from '../assets/icons/python.png'
import htmlIcon from '../assets/icons/html.png'
import javascriptIcon from '../assets/icons/javascript.png'
import jsonIcon from '../assets/icons/json.png'
import yamlIcon from '../assets/icons/yaml.png'
import jpgIcon from '../assets/icons/jpg.png'

// 默认"后缀→图标"映射：键为不含点的小写后缀，值为 import 的图标 URL
// 下期接 AppSettings 时，将用户自定义映射与默认合并即可
const DEFAULT_ICON_MAP = {
  // xlsx.png
  xlsx: xlsxIcon, xls: xlsxIcon, csv: xlsxIcon,
  // docx.png
  docx: docxIcon, doc: docxIcon,
  // txt.png
  txt: txtIcon, log: txtIcon,
  // pdf.png
  pdf: pdfIcon,
  // png.png
  png: pngIcon,
  // markdown.png
  md: markdownIcon, markdown: markdownIcon,
  // java.png
  java: javaIcon,
  // python.png
  py: pythonIcon, pyw: pythonIcon,
  // html.png
  html: htmlIcon, htm: htmlIcon,
  // javascript.png
  js: javascriptIcon, mjs: javascriptIcon, cjs: javascriptIcon, jsx: javascriptIcon,
  // json.png
  json: jsonIcon,
  // yaml.png
  yaml: yamlIcon, yml: yamlIcon,
  // jpg.png
  jpg: jpgIcon, jpeg: jpgIcon
}

// 取文件名最后一个 `.` 之后的部分作为后缀（如 a.tar.gz → gz），无 `.` 返回空串
function getExtension(name) {
  if (!name) return ''
  const lastDot = name.lastIndexOf('.')
  if (lastDot < 0) return ''
  return name.slice(lastDot + 1).toLowerCase()
}

// 根据文件名返回对应类型图标 URL；无匹配返回 null（调用方 fallback EP Document）
export function getIconForFile(name) {
  const ext = getExtension(name)
  if (!ext) return null
  return DEFAULT_ICON_MAP[ext] || null
}

export { DEFAULT_ICON_MAP }
