# pptx 文件树预览（COM 转 PDF 复用 pdfjs viewer）

## Goal

文件树点击 pptx 时提供高保真内嵌预览，替代当前「PowerPoint 内嵌预览保真较低」降级提示。路线 1：后端经 PowerPoint/WPS COM 将 pptx 整体转换为 PDF（落 LRU 缓存目录），前端复用现成 pdfjs 完整 viewer（kind=pdf iframe 链路）。

## Requirements

- 文件树点击 `.pptx` / `.ppsx` → 后端 COM 转 PDF → 前端 pdfjs viewer 完整预览（翻页/缩放/搜索/缩略图）
- 缓存命中（源未变）直接返回缓存 PDF 路径，无 COM 延迟
- 缓存 LRU 上限 512MB，超限淘汰最旧
- 同文件在途转换时重复点击返回进行中态，不重复排队
- 瞬态 COM 失败重试 1 次
- 转换失败 / 无 Office / Linux / 非 Windows：保留现降级提示 + 用默认程序打开
- 浏览器 serve 通道同样可用（/preview-pdf 同源链路天然复用）

## Acceptance Criteria

- [ ] Windows + WPS/PowerPoint 环境点击 .pptx 显示 pdfjs 预览
- [ ] 二次点击同一文件命中缓存，无 COM 延迟
- [ ] 源文件修改后重新预览，内容为新版（mtime+size 失效）
- [ ] 在途转换中重复点击不产生第二个转换任务
- [ ] 无 Office 环境 fallback 提示正常，转换失败降级 + 可用默认程序打开
- [ ] 现有 PDF/docx/xlsx 预览与素材缩略图链路零回归
- [ ] 真机验证 COM 链路（CI 不可测，参照 assets-thumbs 冒烟先例）

## Definition of Done

- 单测覆盖转换编排服务（缓存判定/LRU/在途防抖，COM 层 fake 注入 + worker 存活性断言，参照 assets-thumbs P0 教训）
- 覆盖率门禁达标（service ≥76%）
- 真机冒烟（WPS + PowerPoint 至少一方）
- README/API参考 文档更新

## Technical Approach

**链路**：`ContentPanel previewFile` → `PreviewFile`（kind=office, ext=pptx/ppsx）→ 调新 Wails 方法 `ConvertPptxToPdf(path)` → 返回 `{pdfPath}` 或 `{pending/降级}` → 前端把 pdfPath 当 kind=pdf 走现成 iframe 链路。

**后端**：
- COM 转换任务排进现有 assets-thumbs STA worker（`goOleThumbExporter` 新增 `ConvertToPdf(ctx, src, dst)`），单队列串行避免双 COM 队列进程互杀；复用 ensureApp / ownedPIDs / 超时杀进程 / 瞬态重试基建
- 转换调用 `Presentation.SaveAs(dst, ppSaveAsPDF)`（WPS 兼容性以 research/com-saveas-pdf.md 调研为准）
- 转换编排抽独立 service（缓存索引 + LRU + 在途 map），COM 执行经接口注入便于 fake 测试
- 缓存：`data/pptx_preview_pdf/<路径哈希>-<size>-<mtime>.pdf`，sidecar 索引 JSON（util.SaveJSON 原子写）
- 超时 60s，超时杀自启 COM 进程返降级错误
- `/preview-pdf` handler 零改动（产物即 .pdf 绝对路径）

**前端**：`FilePreviewRenderer.vue` pptx 分支改造——收到转换成功后切 pdf 分支渲染；loading 态（首次转换 1-3s+）；失败降级现提示。`wailsjs/` 绑定三处手动同步。

## Decision (ADR-lite)

**Context**：前端 JS 库渲染 pptx 保真低（当初降级原因）；纯 Go 无法高保真渲染幻灯片；项目已有生产验证的 Office COM 管线（assets 缩略图）与完整 pdfjs viewer。
**Decision**：COM 整体转 PDF + 复用 pdfjs viewer；转换任务并入现有 COM STA worker 单队列。
**Consequences**：高保真、前端改动小、viewer 功能全（搜索/缩略图白得）；代价是依赖本机 Office 安装（无则降级）、首次转换有延迟（缓存缓解）、素材 COM 域代码被跨域复用（装配耦合需在 AppServices 层处理好）。

## Out of Scope

- `.ppt/.pptm/.pps` 旧二进制/宏格式（保留降级）
- 缓存上限/开关进设置页（硬编码 512MB，后续再暴露）
- docx/xlsx 转 PDF（现有内嵌渲染够用）
- 转换进度事件（loading 态即可，页级进度无必要）

## Research References

- [`research/com-saveas-pdf.md`](research/com-saveas-pdf.md) — COM SaveAs PDF 调用方式与 WPS 兼容性（调研中）

## Technical Notes

- 已查文件：frontend/src/components/FilePreviewRenderer.vue、server/preview.go、app_preview.go、service/assets_thumbs_windows.go（结构）
- `/preview-pdf?path=` 接受任意绝对路径 + `.pdf` 白名单，转换产物直接可服务
- COM 全部已知坑见 docs/spec/assets-thumbs-com.md（STA 单 goroutine、GetProperty 属性、杀进程快照 diff、WPS Visible、Export 绝对路径、0x8001010A 重试等）
- `data/` JSON 持久化须走 util.SaveJSON 原子写；手改 data 文件前关 workbench.exe
- spec 参考：docs/spec/assets-thumbs-com.md、linux-platform.md、browser-channel.md、app-services-assembly.md（新 service 装配 2 处）
