# PRD：深色模式代码预览语法高亮配色适配

## 1. 背景

工作目录右侧「操作面板 - 文件预览」在预览 TypeScript 等源码文件时，走 `FilePreviewRenderer.vue` 的 CodeMirror 6 分支（`kind === 'text'`）。该分支固定使用 `@codemirror/language` 的 `defaultHighlightStyle`，其配色是为**浅色背景**设计的（深蓝 `#00c`、暗紫 `#708`、暗橙 `#940`、暗绿 `#404740` 等）。

深色模式容器背景为 `#1e293b`（`--bg-secondary` 暗色取值）。上述深色 token 颜色与背景亮度接近，关键 token（属性名、变量名、字符串等）几乎不可见，用户只能通过选中文字或整行才能辨认内容。

用户报告的近似颜色：`#0300c7`、`#0003be`（深蓝系 token），背景 `#1e293b`。

## 2. 目标

- 深色模式下代码预览的语法高亮颜色与暗色背景具备可读对比度。
- 浅色模式配色保持不变（现有观感不回退）。
- 深色配色与项目既有 hljs 暗色主题（同文件 1702 行 `.hljs-*` 规则）保持一致，避免同一应用内两套代码配色割裂。

## 3. 方案

1. 在 `frontend/src/components/FilePreviewRenderer.vue` 内用 `@codemirror/language` 的 `HighlightStyle` + `tags` 定义暗色高亮样式，色值与文件内既有 hljs 暗色主题对齐：
   - 注释 `#6a9955`（斜体）
   - 关键字 / 标签 `#569cd6`
   - 字符串 / 类型 / 属性值 `#ce9178`
   - 数字 / 函数名 / 类名 `#dcdcaa`
   - 变量 / 属性名 `#9cdcfe`
   - 删除 / 无效 `#f44747`
   - 默认正文 `#d4d4d4`
2. 高亮样式放入 `Compartment`，按 `settingsStore.resolvedTheme` 选择暗色/默认（浅色）样式。
3. 监听 `resolvedTheme` 变化，经 `StateEffect.reconfigure` 重配该 Compartment，不重建编辑器（保留滚动位置与折叠状态）。
4. 暗色下补充 CodeMirror 内置的选中行/选区/折叠槽可见性微调，避免浅色默认值在暗底上不可见。

## 4. 范围外

- 不改 Markdown 预览（已使用 v-html + hljs 暗色规则）。
- 不改终端、AI 对话的配色。
- 不新增依赖（不引入 `@codemirror/theme-one-dark`）。

## 5. 验收

- 深色模式下打开任一 `.ts` 文件预览，属性名、变量名、字符串、关键字均可直接辨认。
- 切换浅色模式后配色与改动前一致。
- 运行中切换主题，预览颜色即时刷新且滚动位置不丢。
- `cd frontend && npm test` 通过；新增/调整单测覆盖暗色样式选择。
