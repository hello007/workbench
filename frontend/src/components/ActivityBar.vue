<!-- frontend/src/components/ActivityBar.vue -->
<template>
  <div class="activity-bar">
    <div
      v-for="item in panels"
      :key="item.id"
      class="activity-bar-item"
      :class="{ 'is-active': modelValue === item.id }"
      @click="$emit('update:modelValue', item.id)"
    >
      <el-icon :size="20">
        <component :is="item.icon" />
      </el-icon>
    </div>
    <!-- 设置图标（不切换面板，直接开弹窗） -->
    <div
      class="activity-bar-item"
      @click="$emit('openSettings')"
    >
      <el-icon :size="20">
        <Setting />
      </el-icon>
    </div>
    <!-- 终端图标（底部） -->
    <div class="activity-bar-spacer"></div>
    <div
      class="activity-bar-item"
      :class="{ 'is-active': terminalActive }"
      @click="$emit('toggleTerminal')"
    >
      <el-icon :size="20">
        <Monitor />
      </el-icon>
    </div>
  </div>
</template>

<script setup>
import { Folder, SetUp, Setting, Monitor } from '@element-plus/icons-vue'

defineProps({
  modelValue: { type: String, default: 'directory' },
  terminalActive: { type: Boolean, default: false }
})

defineEmits(['update:modelValue', 'toggleTerminal', 'openSettings'])

const panels = [
  { id: 'directory', icon: Folder, label: '工作目录' },
  { id: 'toolbox', icon: SetUp, label: '工具箱' }
]
</script>

<style scoped>
.activity-bar {
  width: 44px;
  flex-shrink: 0;
  background: var(--sidebar-bg);
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 10px 0;
  gap: 4px;
}

.activity-bar-spacer {
  flex: 1;
}

.activity-bar-item {
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  cursor: pointer;
  color: var(--sidebar-text);
  transition: all var(--transition-normal);
}

.activity-bar-item:hover {
  background: var(--sidebar-hover);
  color: var(--sidebar-text-hover);
  transform: scale(1.08);
}

.activity-bar-item.is-active {
  background: var(--sidebar-active-bg);
  color: var(--sidebar-active-text);
  box-shadow: 0 0 8px 1px rgba(64, 158, 255, 0.35);
  transform: scale(1.08);
}
</style>
