<!-- frontend/src/components/icons/BranchCompareIcon.vue -->
<!-- 分支对比图标：两条分歧分支（各带起点节点，底部相向弯折汇聚）+ 中部对比双栏块，
     stroke/fill 走 currentColor，继承所在容器（ActivityBar 活动栏）的文字色实现
     常态、hover、active 三态变色，无需额外样式。
     元素级图标单文件 SFC 规范见 docs/spec/frontend-visual-conventions.md §图标着色与接入 -->
<template>
  <svg viewBox="0 0 1024 1024" fill="none" aria-hidden="true">
    <!-- 左分支：起点节点沿左缘下行，底部向右弯折（汇聚向对比带） -->
    <path
      d="M256 256 v336 a144 144 0 0 0 144 144 h56"
      stroke="currentColor" stroke-width="64" stroke-linecap="round" />
    <!-- 右分支：起点节点沿右缘下行，底部向左弯折（汇聚向对比带） -->
    <path
      d="M768 256 v336 a144 144 0 0 1 -144 144 h-56"
      stroke="currentColor" stroke-width="64" stroke-linecap="round" />
    <!-- 分支起点节点（左 A / 右 B） -->
    <circle cx="256" cy="176" r="72" fill="currentColor" />
    <circle cx="768" cy="176" r="72" fill="currentColor" />
    <!-- 中部对比带：2×2 色块模拟 diff 双栏对照 -->
    <rect x="400" y="416" width="88" height="64" rx="14" fill="currentColor" />
    <rect x="536" y="416" width="88" height="64" rx="14" fill="currentColor" />
    <rect x="400" y="552" width="88" height="64" rx="14" fill="currentColor" />
    <rect x="536" y="552" width="88" height="64" rx="14" fill="currentColor" />
  </svg>
</template>

<script setup>
defineOptions({ name: 'BranchCompareIcon' })
</script>
