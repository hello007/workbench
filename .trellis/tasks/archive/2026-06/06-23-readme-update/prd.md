# README 更新：补充 PDF 内嵌相关项目结构与说明

## Goal

README 主体已较全（功能、文件类型预览含 PDF 内嵌方案 B、Obsidian、终端、发版等均已描述）。但**项目结构部分遗漏了 PDF 内嵌预览新增的 `server/` 与 `frontend/public/pdfjs-viewer/`**，需补充使其完整准确。

## 改动（只改 README.md）

1. **项目结构补充**：
   - `server/` 目录（`preview.go` — PDF 预览的 AssetServer handler，`/preview-pdf?path=`，同源提供本地 PDF 字节、支持 HTTP Range、路径安全校验）
   - `frontend/public/pdfjs-viewer/`（pdfjs 官方 viewer v4.8.69 静态资源：`web/` + `build/` + `locale/` 中英，iframe 加载，`go:embed` 打包进 exe）
2. **确认文件类型预览 PDF 条目准确**（trellis-check 已更新为「内嵌预览 pdfjs viewer」，复核无误即可）
3. **可选补充**：构建产物体积说明（`wails build` 后 exe 约 33MB，含 pdfjs viewer 资源）

## 约束

- 只改 `README.md`，不动任何代码
- 不破坏现有 README 结构与内容

## Acceptance Criteria

* [ ] 项目结构含 `server/` 与 `frontend/public/pdfjs-viewer/`
* [ ] 文件类型预览 PDF 描述准确
