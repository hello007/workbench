<template>
  <div id="app">
    <router-view />
  </div>
</template>

<script setup>
import { watch, onMounted } from 'vue'
import { useSettingsStore } from './store'

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

onMounted(() => {
  // 启动时从 settings.json 加载主题配置（system 模式实时跟随系统偏好）
  settingsStore.loadTheme()
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
