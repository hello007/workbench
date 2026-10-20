import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// backendPreviewRoutesDev404 dev 模式专用插件：把后端 PreviewHandler（server/preview.go）
// 专属路由 /preview-pdf 与 /preview-raw 在 Vite dev server 内提前 404。
//
// why：wails dev 下前端由 Vite dev server 提供，请求先经 Vite 内部中间件链处理，
// 对未匹配到静态资源/模块的 GET 请求 Vite 会执行 SPA fallback（返回 200 +
// index.html，响应头带 @vite/client 佐证），后端 Wails AssetServer.Handler
// （挂载 PreviewHandler 的 fallback 入口）永远轮不到命中——pdfjs viewer 拿到
// HTML 按 PDF 解析得 0 页黑屏（真机 P1：转换成功、viewer 打开但黑屏 0/0 页）。
// Vite 插件的 configureServer 中间件注册在 Vite 内部中间件（含 SPA fallback）
// 之前执行：对这两条路由提前返回 404，Wails dev 资产链路在 dev server 404 后
// 回退 AssetServer.Handler，Go 侧实现（/preview-pdf 的 PDF 字节服务 +
// /preview-raw 的静态资源白名单路由）才能被命中。仅 dev 生效，production
// build 不经 Vite dev server、不受影响。
// 注：404（而非透传）是刻意的——透传会落回 Vite SPA fallback 重新吞掉请求；
// Wails dev 侧对 404 的回退行为才是接入 PreviewHandler 的唯一通道。
const backendPreviewRoutesDev404 = {
  name: 'backend-preview-routes-dev-404',
  configureServer(server) {
    server.middlewares.use((req, res, next) => {
      if (req.url.startsWith('/preview-pdf') || req.url.startsWith('/preview-raw')) {
        res.statusCode = 404
        res.end()
        return
      }
      next()
    })
  }
}

export default defineConfig({
  plugins: [vue(), backendPreviewRoutesDev404],
  server: {
    port: 5173
  },
  optimizeDeps: {
    // mermaid 改为动态 import 懒加载（FilePreviewRenderer.vue ensureMermaid），
    // 显式 include 确保 wails dev 启动即预构建 mermaid，避免动态 import 首次触发时
    // Vite 重新发现依赖并重启 dev server 导致首次渲染失败（与 docx/xlsx 历史同源问题，
    // 见 FilePreviewRenderer.vue 顶部注释）。production build 不受影响。
    include: ['mermaid']
  }
})
