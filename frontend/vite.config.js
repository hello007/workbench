import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
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
