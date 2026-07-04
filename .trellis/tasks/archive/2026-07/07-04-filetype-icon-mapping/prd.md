# 文件树按类型显示图标与自定义后缀映射

## Goal

文件树中的文件节点按扩展名显示对应的类型图标（替换当前统一的 EP `Document` 图标），用 `frontend/src/assets/icons/` 下 13 个文件类型图标（xlsx/docx/txt/pdf/png/markdown/java/python/html/javascript/json/yaml/jpg）。后期支持用户自定义"后缀→图标"映射（如 `doc`→`docx.png`、`xls`→`xlsx.png`）。

## What I already know（已探明的事实）

### 文件树当前图标渲染（`FileTreePanel.vue:27-53`）

```html
<template #default="{ node, data }">
  <el-icon v-if="data.type === 'directory'"><FolderOpened/Folder/></el-icon>
  <el-icon v-else><Document /></el-icon>   ← 文件统一 Document，要按后缀替换
  <span>{{ node.label }}</span>
  <img v-if="data.isGitRepo" :src="gitIcon" />
</template>
```

### 后端

- `service/filetree.go`：`FileTreeNode` 只标 `type: directory|file`，**无后缀/类型字段**。
- `model.AppSettings`（settings.go）：现有字段 GpuDisabled/DefaultShell/GitBashPath/WslDistro/SearchExcludeDirs/SearchExcludeFiles/ShortcutCommandPalette/ShortcutToggleTerminal/ObsidianPath——**无文件图标映射字段**。
- `AppSettings` 走 `service/settings.go` 持久化（settings.json）。

### icons 资源（已入库 e27774e）

`docx.png / html.png / java.png / javascript.png / jpg.png / json.png / markdown.png / pdf.png / png.png / python.png / txt.png / xlsx.png / yaml.png`（13 个）。

### 默认后缀→图标映射（提议，待用户确认）

| 图标 | 默认后缀（不含点） |
|---|---|
| `xlsx.png` | xlsx, xls, csv |
| `docx.png` | docx, doc |
| `txt.png` | txt, log |
| `pdf.png` | pdf |
| `png.png` | png |
| `markdown.png` | md, markdown |
| `java.png` | java |
| `python.png` | py, pyw |
| `html.png` | html, htm |
| `javascript.png` | js, mjs, cjs, jsx |
| `json.png` | json |
| `yaml.png` | yaml, yml |
| `jpg.png` | jpg, jpeg |
| （fallback） | 无匹配 → EP `Document`（保持现状） |

## Assumptions（待验证）

- 后缀匹配大小写不敏感（`.MD` 与 `.md` 同样处理）。
- 复合后缀（`.tar.gz`）按最后一个后缀（`.gz`）处理，无匹配则 fallback。
- 自定义映射与默认合并：自定义优先（用户可覆盖默认，也可新增）。
- 图标尺寸沿用文件树现有图标视觉（约 14-16px，对齐 Document/Folder）。

## Open Questions（仅阻塞/偏好类）

1. **[已定]** MVP 范围 = **方案 A：仅前端硬编码默认映射**（无后端配置、无 UI）。自定义映射留待下期。✅
2. **[待最终确认]** 应用范围：仅文件树，还是也用于 ContentPanel 文件标题 / 命令面板文件项？
3. **[待最终确认]** 默认映射表（含 jsx→javascript、csv→xlsx 等归类）是否 OK？

## Decision (ADR-lite)

**Context**：用户希望文件树按类型显示图标；自定义映射"后期"再做。

**Decision**：MVP 采用方案 A——前端硬编码默认"后缀→图标"映射表（const），文件树文件节点按后缀（大小写不敏感、取最后一个 `.` 后缀）选图标，无匹配 fallback EP `Document`。**不改后端、不加 settings 字段、不加 UI**。映射表抽到独立 `utils/fileIconMap.js` 便于下期接 AppSettings 时替换为"默认 + 自定义合并"。

**Consequences**：
- 优点：改动最小（纯前端 1-2 文件）、零后端/配置/UI 工作量、即开即用。
- 代价：本期用户无法自定义映射（doc→docx.png、xls→xlsx.png 等默认已含；特殊需改代码）。
- 扩展点：`utils/fileIconMap.js` 暴露 `getIconForFile(name)` 函数；下期接 AppSettings 时改为读配置合并，调用方不变。

## Requirements（演进中）

- 文件树文件节点按后缀显示对应类型图标（替换统一 Document）。
- 无匹配后缀 fallback 到 EP Document。
- 后缀匹配大小写不敏感。

## Acceptance Criteria（演进中）

- [ ] 文件树中 .xlsx/.docx/.txt/.pdf/.png/.md/.java/.py/.html/.js/.json/.yaml/.jpg 等显示对应类型图标。
- [ ] 无匹配后缀（如 .exe/.bat）显示 EP Document（不报错）。
- [ ] 大小写不敏感（.MD 与 .md 一致）。
- [ ] 自定义映射（若 MVP 含）生效：用户配置 doc→docx.png 后，.doc 文件显示 docx 图标。
- [ ] `npm test` / `npm run build` 通过。

## Definition of Done

- 上述 AC 全过；
- 自定义映射（若做）有单测/手测；
- `npm test` / `npm run build` 全绿；
- 若行为变化（自定义映射 UI），确认更新 `docs/功能说明.md` / `README.md`。

## Out of Scope（明确排除）

- 图标美术修改、新增图标资源。
- 按文件内容（魔数）识别类型。
- 文件夹图标变化（保持 Folder/FolderOpened）。

## Technical Notes

- 关键文件：
  - 前端：`FileTreePanel.vue`（文件节点图标渲染）、`SettingsPanel.vue`（若做 UI）、可能新建 `composables/useFileIcon.js` 或 `utils/fileIconMap.js`。
  - 后端：`model/settings.go`（加映射字段，若 B/C）、`service/settings.go`（持久化）、`app.go`（暴露读写 API）。
- 默认映射表放前端常量（`{ 后缀: 图标import }`）；自定义映射从 AppSettings 读取，与默认合并。
- 复用 obsidian 等图标的 `<img>` 模式（约 14px）。
- 复合后缀取最后一个 `.` 后的部分。
