# 美化文件预览区背景（浅蓝主题底）

## Goal

文件预览区（`.file-preview`）用浅蓝主题底 + 主题色边框 + 圆角阴影，明显成为「预览卡片」，与上方文件信息/操作按钮区（白底）一眼区分。

## Decision

**浅蓝主题底**：`.file-preview` 容器 `background: var(--primary-bg)`(#ecf5ff) + `border: 1px solid var(--primary-light)` + `border-radius: var(--radius-md)` + `box-shadow: var(--shadow-sm)` + 内边距。header 区浅蓝底 + 底部细分隔；body 内渲染器保持各自背景，形成层次（浅蓝容器 > 内容区）。

## 实施

- `ContentPanel.vue`：
  - `.file-preview`：浅蓝底 + 主题边框 + 圆角 + 阴影 + padding + margin-top
  - `.file-preview-header`：浅蓝底（继承）、底部细边框分隔 body、padding
  - `.file-preview-body`：透明/协调，渲染器撑满
- `FilePreviewRenderer.vue`：渲染器内部背景（`.cm-host`/`.image-scroll`/pdf iframe 容器/docx 容器/`.markdown-body`）与浅蓝容器层次分明（保持 `--bg-tertiary` 或白底均可，确保不与容器混淆）

## 约束

- 只改 ContentPanel.vue / FilePreviewRenderer.vue 的 CSS
- 不破坏功能（预览/编辑/降级）与布局
- UI 看效果迭代

## Acceptance Criteria

* [ ] 预览区有明显浅蓝背景 + 边框，一眼识别为「文件预览」
* [ ] 与上方白底信息/按钮区区分清晰
* [ ] 渲染器内容可读、层次分明
* [ ] `npm run build` 通过
