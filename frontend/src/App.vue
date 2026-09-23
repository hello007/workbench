<template>
  <div id="app">
    <router-view />
  </div>
</template>

<script setup>
import { watch, onMounted } from 'vue'
import { ElNotification } from 'element-plus'
import { useSettingsStore } from './store'
import { GetAndClearLastCrashFlag } from '../wailsjs/go/main/App'

const settingsStore = useSettingsStore()

/**
 * 将实际生效主题应用到 <html>：dark 模式加 .dark class，触发 style.css
 * 的 html.dark 变量覆盖与 Element Plus dark/css-vars。light 移除 .dark。
 */
function applyTheme(theme) {
  if (typeof document === 'undefined') return
  document.documentElement.classList.toggle('dark', theme === 'dark')
}

// immediate:true 保证首屏立即按 resolvedTheme 应用，避免亮闪
// loadTheme 完成后 themeMode 由 settings.json 覆盖，resolvedTheme 变化时再次触发
watch(() => settingsStore.resolvedTheme, (theme) => {
  applyTheme(theme)
}, { immediate: true })

// 上次异常退出提示：后端 crash.flag 检测（读后即清），引导从设置导出诊断信息。
// 静默失败（浏览器 mock 环境无此方法/后端异常）不影响启动。
async function checkLastCrash() {
  try {
    if (await GetAndClearLastCrashFlag()) {
      ElNotification({
        title: '上次运行异常退出',
        message: '如遇到问题，可在「设置 - 关于 - 诊断信息」导出诊断文件反馈',
        type: 'warning',
        duration: 8000
      })
    }
  } catch { /* 崩溃检测为增强提示，失败静默 */ }
}

onMounted(() => {
  // 启动时从 settings.json 加载主题配置（system 模式实时跟随系统偏好）
  settingsStore.loadTheme()
  // 启动时解析默认 Shell（用户设置 → 平台默认 → powershell 兜底），终端创建链路消费
  settingsStore.loadDefaultShell()
  // 检测上次会话是否异常退出（崩溃标记读后即清）
  checkLastCrash()
})
</script>

<style>
#app {
  width: 100%;
  height: 100vh;
  margin: 0;
  padding: 0;
  overflow: hidden !important;
  display: flex;
  flex-direction: column;
}
</style>
