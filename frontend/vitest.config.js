import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.js'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'text-summary', 'html', 'lcov'],
      // 排除 Wails 运行时生成绑定（非业务代码，0% 覆盖率会拖低整体）
      exclude: ['wailsjs/**'],
      // 阈值：硬失败，整体四指标 ≥70%（补测后达标）
      thresholds: {
        lines: 70,
        branches: 70,
        functions: 70,
        statements: 70
      }
    }
  }
})

