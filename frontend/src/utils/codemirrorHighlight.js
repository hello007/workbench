/**
 * CodeMirror 6 语法高亮样式（文件预览只读代码/文本用）
 *
 * 浅色沿用 CodeMirror 内置 defaultHighlightStyle，保持既有观感不变；
 * 深色为本项目自定义，色值与 FilePreviewRenderer.vue 内 `.hljs-*` 暗色主题一致。
 *
 * 自定义原因：defaultHighlightStyle 面向浅色背景设计（深蓝 #00c、暗紫 #708、
 * 暗橙 #940、暗绿 #404740 等），落到深色预览底 --bg-secondary(#1e293b) 上
 * 对比度过低，属性名/变量名等 token 无法辨认。
 *
 * 深色样式未覆盖的 tag 不产生样式类，继承 `.cm-editor` 的正文色（#d4d4d4），
 * 不会回落到浅色色板；浅色仍用内置 defaultHighlightStyle（保留原 fallback 行为）。
 */
import { HighlightStyle, defaultHighlightStyle } from '@codemirror/language'
// tags 由 @lezer/highlight 提供（@codemirror/language 不再转导出）；已登记为顶层依赖
import { tags as t } from '@lezer/highlight'

export const darkCodeHighlightStyle = HighlightStyle.define([
  // 注释：与 .hljs-comment 一致
  { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], color: '#6a9955', fontStyle: 'italic' },
  // 关键字 / 标签名：与 .hljs-keyword、.hljs-tag 一致
  {
    tag: [
      t.keyword,
      t.controlKeyword,
      t.definitionKeyword,
      t.moduleKeyword,
      t.operatorKeyword,
      t.modifier,
      t.tagName,
      t.self
    ],
    color: '#569cd6'
  },
  // 字符串 / 类型 / 属性值 / 字面量：与 .hljs-string、.hljs-type、.hljs-literal 一致
  {
    tag: [
      t.string,
      t.special(t.string),
      t.docString,
      t.character,
      t.regexp,
      t.escape,
      t.attributeValue,
      t.typeName,
      // 内建类型（standard 修饰）与普通类型名同色，避免同一语义两套颜色
      t.standard(t.typeName),
      t.literal,
      t.inserted,
      t.color,
      t.labelName
    ],
    color: '#ce9178'
  },
  // 数字 / 函数名 / 元信息：与 .hljs-number、.hljs-meta 一致
  {
    tag: [
      t.number,
      t.integer,
      t.float,
      t.bool,
      t.null,
      t.atom,
      t.unit,
      t.meta,
      t.documentMeta,
      t.annotation,
      t.processingInstruction,
      t.link,
      t.url,
      t.definition(t.variableName),
      t.function(t.variableName),
      t.function(t.propertyName),
      // 类名（JS/TS class、CSS 类选择器）：与 .hljs-title / .hljs-selector-class 同组
      t.className
    ],
    color: '#dcdcaa'
  },
  // 变量 / 属性名：与 .hljs-variable、.hljs-attr 一致（用户报告不可读的 key 字段在此组）
  {
    tag: [
      t.variableName,
      t.propertyName,
      t.attributeName,
      t.namespace
    ],
    color: '#9cdcfe'
  },
  // 运算符与标点：与正文同色，避免暗底上割裂
  {
    tag: [
      t.operator,
      t.derefOperator,
      t.arithmeticOperator,
      t.logicOperator,
      t.bitwiseOperator,
      t.compareOperator,
      t.updateOperator,
      t.definitionOperator,
      t.typeOperator,
      t.punctuation,
      t.separator,
      t.bracket,
      t.angleBracket,
      t.squareBracket,
      t.paren,
      t.brace
    ],
    color: '#d4d4d4'
  },
  // 删除 / 非法 token：与 .hljs-deletion 一致
  { tag: [t.deleted, t.invalid], color: '#f44747' },
  // Markdown 标题与强调
  { tag: [t.heading, t.heading1, t.heading2, t.heading3], color: '#569cd6', fontWeight: '600' },
  { tag: [t.heading4, t.heading5, t.heading6], color: '#9cdcfe', fontWeight: '600' },
  { tag: t.emphasis, fontStyle: 'italic' },
  { tag: t.strong, fontWeight: '600' },
  { tag: t.strikethrough, textDecoration: 'line-through' },
  { tag: t.monospace, color: '#d4d4d4' }
])

/**
 * 按当前实际生效主题（settingsStore.resolvedTheme）选取高亮样式。
 * @param {'light'|'dark'} theme
 */
export const codeHighlightStyleFor = (theme) =>
  theme === 'dark' ? darkCodeHighlightStyle : defaultHighlightStyle
