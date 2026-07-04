import xlsxIcon from '../assets/icons/xlsx.png'
import docxIcon from '../assets/icons/docx.png'
import txtIcon from '../assets/icons/txt.png'
import pdfIcon from '../assets/icons/pdf.png'
import pngIcon from '../assets/icons/png.png'
import markdownIcon from '../assets/icons/markdown.png'
import javaIcon from '../assets/icons/java.png'
import pythonIcon from '../assets/icons/python.png'
import goIcon from '../assets/icons/go.png'
import htmlIcon from '../assets/icons/html.png'
import javascriptIcon from '../assets/icons/javascript.png'
import typescriptIcon from '../assets/icons/typescript.png'
import cssIcon from '../assets/icons/css.png'
import vueIcon from '../assets/icons/vue.png'
import jsonIcon from '../assets/icons/json.png'
import yamlIcon from '../assets/icons/yaml.png'
import xmlIcon from '../assets/icons/xml.png'
import shellIcon from '../assets/icons/shell.png'
import dbIcon from '../assets/icons/db.png'
import zipIcon from '../assets/icons/zip.png'
import propertiesIcon from '../assets/icons/properties.png'
import jpgIcon from '../assets/icons/jpg.png'
import pptIcon from '../assets/icons/ppt.png'
import xmindIcon from '../assets/icons/xmind.png'
import exeIcon from '../assets/icons/exe.png'
import licenseIcon from '../assets/icons/license.png'
import gitignoreIcon from '../assets/icons/gitignore.png'
import tmplIcon from '../assets/icons/tmpl.png'

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
  // go.png
  go: goIcon,
  // html.png
  html: htmlIcon, htm: htmlIcon,
  // javascript.png
  js: javascriptIcon, mjs: javascriptIcon, cjs: javascriptIcon, jsx: javascriptIcon,
  // typescript.png
  ts: typescriptIcon, tsx: typescriptIcon,
  // css.png
  css: cssIcon, scss: cssIcon, sass: cssIcon, less: cssIcon,
  // vue.png
  vue: vueIcon,
  // json.png
  json: jsonIcon,
  // yaml.png
  yaml: yamlIcon, yml: yamlIcon,
  // xml.png
  xml: xmlIcon, svg: xmlIcon,
  // shell.png
  sh: shellIcon, bash: shellIcon, zsh: shellIcon, fish: shellIcon,
  // db.png
  db: dbIcon, sqlite: dbIcon, sqlite3: dbIcon,
  // zip.png（压缩包）
  zip: zipIcon, rar: zipIcon, '7z': zipIcon, tar: zipIcon, gz: zipIcon, bz2: zipIcon, xz: zipIcon, tgz: zipIcon,
  // properties.png（配置及 properties 类文件）
  properties: propertiesIcon, ini: propertiesIcon, conf: propertiesIcon, cfg: propertiesIcon, env: propertiesIcon, toml: propertiesIcon,
  // jpg.png
  jpg: jpgIcon, jpeg: jpgIcon,
  // ppt.png
  ppt: pptIcon, pptx: pptIcon,
  // xmind.png
  xmind: xmindIcon,
  // exe.png（Windows 可执行/安装包）
  exe: exeIcon, msi: exeIcon,
  // tmpl.png（模板系列）
  tmpl: tmplIcon, tpl: tmplIcon, template: tmplIcon,
  // gitignore.png（.gitignore 文件，getExtension 取得 "gitignore" 后缀）
  gitignore: gitignoreIcon
}

// 特殊文件名→图标映射（大小写不敏感）：匹配无后缀特殊文件如 LICENSE/COPYING
// getIconForFile 先查文件名，再查后缀
const FILENAME_ICON_MAP = {
  license: licenseIcon,
  licence: licenseIcon,
  copying: licenseIcon,
  notice: licenseIcon
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
  if (!name) return null
  // 1. 完整文件名匹配（大小写不敏感）—— 命中无后缀特殊文件如 LICENSE
  const filenameKey = name.toLowerCase()
  if (FILENAME_ICON_MAP[filenameKey]) return FILENAME_ICON_MAP[filenameKey]
  // 2. 后缀匹配
  const ext = getExtension(name)
  if (!ext) return null
  return DEFAULT_ICON_MAP[ext] || null
}

export { DEFAULT_ICON_MAP, FILENAME_ICON_MAP }
