# 清理本地无用文件并优化 gitignore

## 目标

审查项目本地文件，识别无用文件并清理，优化 `.gitignore` 配置，保持仓库整洁。

## 已识别的清理项

### 1. 根目录空 npm 文件（可删除）

| 文件/目录 | 问题 | 大小 |
|----------|------|------|
| `package-lock.json` | 空文件，无 packages | - |
| `node_modules/` | 根目录无 package.json | 13K |

**操作**：直接删除

### 2. Wails 绑定代码（从 git 移除）

| 文件/目录 | 问题 | 大小 |
|----------|------|------|
| `frontend/wailsjs/` | 已被 git 跟踪，但 gitignore 规则存在 | 3 文件 |

**操作**：`git rm -r --cached frontend/wailsjs/`，后续由 `wails dev` 自动生成

### 3. PDF.js 无用文件（可删除）

| 文件 | 问题 | 大小 |
|------|------|------|
| `frontend/public/pdfjs-viewer/web/compressed.tracemonkey-pldi-09.pdf` | PDF.js 示例 PDF，无用 | 996K |
| `frontend/public/pdfjs-viewer/web/viewer.mjs.map` | Source map，生产不需要 | 1.3M |

**操作**：`git rm` 并加入 gitignore

### 4. 已正确处理（无需操作）

| 文件/目录 | 状态 |
|----------|------|
| `.trellis/.runtime/` | 已被 `.trellis/.gitignore` 忽略 |
| `frontend/package.json.md5` | 已被根 `.gitignore` 忽略 |
| `build/` | 已被忽略（38M） |
| `.codegraph/` | 已被忽略（12M） |
| `frontend/dist/` | 已被忽略（24M） |

## 验收标准

- [ ] 删除根目录空 npm 文件
- [ ] 从 git 移除 `frontend/wailsjs/`
- [ ] 删除 PDF.js 示例 PDF 和 source map
- [ ] 更新 `.gitignore` 排除 source map
- [ ] 仓库状态整洁

## 技术说明

- Wails 项目的前端绑定代码 (`frontend/wailsjs/`) 由 `wails dev` 自动生成，不需要提交
- PDF.js source map 和示例 PDF 对生产环境无意义
- 预计可减少仓库大小约 **2.3M**（PDF.js 文件）