# pptx 文件树预览（COM 转 PDF + pdfjs viewer）契约

> 来源：任务 10-20-pptx-com-pdf-pdfjs-viewer。详细 COM 通用坑见 [assets-thumbs-com.md](assets-thumbs-com.md)，本文只记 pptx 预览域特有契约。

## 1. Scope / Trigger

文件树点击 `.pptx/.ppsx` 触发「COM 转 PDF → pdfjs viewer」内嵌预览。触发本契约的场景：改动转换编排 service、COM 转换会话、缓存层、前端 pptx 分支、新增 Office 转 PDF 类功能（docx/xlsx 未来扩展同构）。

## 2. Signatures

```go
// App 层（Wails 绑定，app_pptx_preview.go）——签名不带 ctx（本仓无 Wails ctx 注入先例，避免绑定生成漂移）
func (a *App) ConvertPptxForPreview(filePath string) *model.PptxPreviewResult

// 编排 service（service/pptx_preview.go）
func (s *PptxPreviewService) ConvertForPreview(ctx context.Context, srcPath string) (string, error) // 返回 pdfPath
// 60s 超时由 service 层 context.WithTimeout 兜底（App 层不传 ctx 的补偿）

// COM 执行器接口（fake 注入点）
type PptxPdfConverter interface {
    ConvertToPdf(ctx context.Context, srcPath, dstPath string) error
    Available() bool
}

// Windows 实现（service/pptx_preview_windows.go）——挂在现有 thumbs 导出器上
func (e *goOleThumbExporter) ConvertToPdf(ctx context.Context, src, dst string) error // 走 e.submit() 同一 STA worker
```

## 3. Contracts

- **model.PptxPreviewResult**：`{pdfPath string, error string, errorCode string}`——容错结构（错误走字段不走 reject），与 FilePreview 模式一致
- **错误码**：`E_PPTX_PREVIEW_UNAVAILABLE`（非 Windows / 无 Office，降级提示）/ `E_PPTX_PREVIEW_FAILED`（转换失败，降级 + 可用默认程序打开）。三处同步：`model/app_error.go` + `frontend/src/utils/error.js` ErrorCode（均 error 级，不入 WARNING_CODES）
- **缓存布局**：`data/pptx_preview_pdf/<路径哈希>-<size>-<mtime纳秒>.pdf` + `index.json`（util.SaveJSON 原子写；键字段 = srcPath+size+mtime，任一变化即 miss）；LRU 上限 512MB 常量（硬编码，未进设置页），按 lastAccess 淘汰文件+索引项
- **前端链路**：`FilePreviewRenderer.vue` pptx 分支 → 调 ConvertPptxForPreview → 成功复用 `buildPdfViewerSrc` 单点走 pdfjs iframe；失败/不可用保留降级提示

## 4. Validation & Error Matrix

| 条件 | 行为 |
|---|---|
| 非 Windows / 无 Office（Available false） | E_PPTX_PREVIEW_UNAVAILABLE，前端降级提示 |
| 源文件不存在 | E_PPTX_PREVIEW_FAILED |
| COM 转换失败（含瞬态） | 重试 1 次（ctx 取消/超时**不重试**）后仍失败 → E_PPTX_PREVIEW_FAILED |
| 转换超时（60s） | 杀自启 COM 进程 + **必须 Discard() 置脏**（见 §7-W2） |
| 缓存命中（键一致且产物存在非空） | 直接返回 pdfPath，零 COM 调用 |
| 同文件并发请求 | inFlight 共享同一次转换，第二调用等待第一完成 |
| 索引有但产物文件缺失 | 同步剔索引项后重转（防死索引） |
| index.json 损坏 | 降级空索引重建，不阻塞 |
| `.ppt/.pptm/.pps` | 前端直接走旧降级分支，不触发转换（有测试锁定） |

## 5. Good/Base/Bad Cases

- **Good**：缓存命中秒回；转换成功 pdfjs 全功能预览；用户 WPS 开着同名文档仍安全（复用实例走 ExportAsFixedFormat）
- **Base**：首次转换 1-3s+（COM 冷启动），loading 态覆盖；源修改后 mtime/size 变化自动重转
- **Bad**：双 COM 队列并发（进程互杀）；转换失败不置脏（后续任务撞死代理全失败）；复用用户实例调 SaveAs（重定向用户会话 FullName）

## 6. Tests Required

- `service/pptx_preview_test.go` 13 用例：缓存命中 / 源变更（os.Chtimes 显式注入，禁 sleep / mtime 时序依赖）/ 在途防抖（**chan 交接 + worker 存活性断言 + 超时守护**——fake 掩蔽回写遗漏 P0 教训）/ 重试 1 次 / 失败分流 / 不可用 / 源缺失 / 索引跨实例持久化 / 损坏降级自愈 / LRU 两态 / 键字段 / 兜底桩
- 前端 `FilePreviewRenderer.spec.js`：成功渲染 viewer URL / 失败降级 / 不可用 / 旧格式不触发转换 / loading / ppsx 支持
- COM 实现文件 0% 覆盖属预期（系统调用核心先例），**真机冒烟必做**：WPS/PowerPoint 至少一方，fixture 须 COM 自产（手写 OOXML zip 被 WPS 拒开）

## 7. Wrong vs Correct

### W1：第二个 COM STA 队列（Wrong） vs 排进现有 thumbs worker（Correct）

```go
// Wrong：新建独立 goOle 转换器 + 自有 STA goroutine——两队列各有进程快照 diff/杀逻辑，
// 并发启动时 A 队列把 B 队列刚启动的 powerpnt/wpp 当「新进程」杀掉
// Correct：ConvertToPdf 走 e.submit() 排进 assets-thumbs 既有单队列串行执行
```

### W2：转换失败/超时不置脏（Wrong） vs Discard()（Correct）

```go
// Wrong：ConvertToPdf 返回 err 后直接透传——超时杀进程后 w.app 代理指向已死实例，
// 后续所有任务（缩略图批次/跳转/转换）持续撞死代理逐个失败，且永不自愈
// Correct：主侧 err != nil 时调 e.Discard() 置脏，下一任务重建 COM 会话
// （复用用户实例场景 ownedPIDs 空，重建链 releaseApp 不 Quit 用户文档，误杀保护不变）
```

### W3：复用用户已开实例时 SaveAs（Wrong） vs ExportAsFixedFormat（Correct）

```go
// Wrong：对复用的 Presentation 调 SaveAs(pdf, 32)——SaveAs 会把用户会话的
// FullName 重定向到 PDF、原文件状态被改写语义污染
// Correct：findOrOpen 命中复用实例 → 直接 ExportAsFixedFormat(dst, 2)（发布语义，
// 不改 Presentation 状态）；仅自启实例走 SaveAs(32)（WPS 对 32 无权威佐证，
// 失败回退 ExportAsFixedFormat(dst, 2)，真机待验证项）
```

### W4：SaveAs 后 Close 处理

SaveAs 后 Presentation 的 FullName 重定向到 PDF 且 Saved 置 true，Close 无提示、不写回源文件——无需额外 dirty 处理；源文件零改动由 ReadOnly 打开本身保证。

## 8. 前端守卫契约

- `pptxConvertSeq` 代次守卫：切换文件 / 关闭预览 / 组件卸载（onBeforeUnmount 须显式 `resetPptxState()` 使代次自增）均自增，在途响应写回被守卫丢弃——缺卸载复位 = 卸载后响应写回已销毁 ref（check 实测 P2）
- 转换成功切 pdf 链路必须复用 `buildPdfViewerSrc` 单点，禁止第二套 URL 拼装

## 9. 预期行为（非缺陷，注释已登记）

- 隐藏幻灯页默认不出现在 PDF（SaveAs 无参可控；需导出隐藏页只能走 ExportAsFixedFormat 全参形态）
- 嵌入视频渲染为海报帧、动画取终态
- 大文件转换（最长 60s）与缩略图批次共用单队列串行——素材库大批量导图时 pptx 预览首开延迟相应拉长（单队列契约设计代价）

## 10. dev 模式 /preview-pdf 路由限制（wails dev 专用修复，2026-10-20 真机 P1）

### 根因

wails dev 下前端由 Vite dev server 提供：请求先经 Vite 内部中间件链，对未匹配到静态资源/模块的 GET 请求 Vite 执行 SPA fallback（返回 200 + index.html，响应头带 `@vite/client` 实锤），后端 Wails `AssetServer.Handler`（挂载 `server.PreviewHandler` 的 fallback 入口）**永远轮不到命中**——pdfjs viewer 拿到 HTML 按 PDF 解析得 0 页黑屏（真机形态：转换成功、viewer 打开、黑屏 0/0 页，PDF 产物本身有效）。生产构建不经 Vite dev server，不受影响；此前 kind=pdf 只在生产验证过，属预存的 dev 模式限制，pptx 转换链路接入时暴露。

### 修复契约

`frontend/vite.config.js` 的 `backendPreviewRoutesDev404` 插件（plugins 数组内，`vue()` 之后）：

- `configureServer` 注册的中间件**先于 Vite 内部中间件（含 SPA fallback）执行**：对 `/preview-pdf` 与 `/preview-raw` 两个 PreviewHandler 专属路由前缀提前返回 404；
- why 404 而非透传：透传会落回 Vite SPA fallback 重新吞掉请求；Wails dev 侧对 dev server 404 的资产回退行为（回退 AssetServer.Handler）是接入 Go 侧实现的唯一通道；
- **仅 dev 生效**：production build 不经 Vite dev server，插件 configureServer 不执行；`previewRawExts`/`/preview-pdf` 白名单语义零改动（server/preview.go 不动）。

### 新增「不经 Vite 静态资产链路的同源路由」时的检查项

前端引用的后端专属同源路由（当前仅 `/preview-pdf`、`/preview-raw`）若再新增，必须同步加入 `backendPreviewRoutesDev404` 的前缀判定，否则 dev 模式被 SPA fallback 吞掉、生产正常（形态隐蔽：仅 dev 黑屏/404 fallback JSON 直出，`npm run build`/`npm test` 全绿测不出）。

### 验证状态

代码层验证：`npm run build` / `npm test` 全绿（插件仅 configureServer 钩子，不影响 build 产物）。**dev 态须重启 wails dev 后真机确认**（dev server 中间件链在启动时装配，改 vite.config.js 后热更不生效——pdfjs viewer 打开真实 PDF 页数 > 0 即通过）。
