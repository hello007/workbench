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
  </div>
</template>

<script setup>
import { Folder, SetUp } from '@element-plus/icons-vue'

defineProps({
  modelValue: { type: String, default: 'directory' }
})

defineEmits(['update:modelValue'])

const panels = [
  { id: 'directory', icon: Folder, label: '工作目录' },
  { id: 'toolbox', icon: SetUp, label: '工具箱' }
]
</script>

<style scoped>
.activity-bar {
  width: 44px;
  flex-shrink: 0;
  background: #2c3e50;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 10px 0;
  gap: 4px;
}

.activity-bar-item {
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  cursor: pointer;
  color: #8eaabe;
  transition: all var(--transition-normal);
}

.activity-bar-item:hover {
  background: #3a4f63;
  color: #b0c4d8;
}

.activity-bar-item.is-active {
  background: var(--primary-color);
  color: #ffffff;
  box-shadow: 0 0 0 2px var(--primary-light);
}
</style>
