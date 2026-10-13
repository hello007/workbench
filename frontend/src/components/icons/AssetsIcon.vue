<!-- frontend/src/components/icons/AssetsIcon.vue -->
<!-- 素材管理图标：内联 SVG，fill 走 currentColor，继承所在容器（ActivityBar 活动栏 / AssetsPanel 标题栏）
     的文字色实现常态、hover、active 三态变色，无需额外样式（同 ToolboxIcon.vue 结构约定）。
     path 数据取自项目依赖 @element-plus/icons-vue 的 PictureFilled 图标（MIT 许可），
     与活动栏其余 Element Plus 图标保持同一视觉语系。 -->
<template>
  <svg viewBox="0 0 1024 1024" fill="currentColor" aria-hidden="true">
    <path d="M96 896a32 32 0 0 1-32-32V160a32 32 0 0 1 32-32h832a32 32 0 0 1 32 32v704a32 32 0 0 1-32 32zm315.52-228.48-68.928-68.928a32 32 0 0 0-45.248 0L128 768.064h778.688l-242.112-290.56a32 32 0 0 0-49.216 0L458.752 665.408a32 32 0 0 1-47.232 2.112M256 384a96 96 0 1 0 192.064-.064A96 96 0 0 0 256 384" />
  </svg>
</template>

<script setup>
defineOptions({ name: 'AssetsIcon' })
</script>
