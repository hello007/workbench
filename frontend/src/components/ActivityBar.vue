<!-- frontend/src/components/ActivityBar.vue -->
<template>
  <div class="activity-bar">
    <el-tooltip
      v-for="item in panels"
      :key="item.id"
      :content="item.label"
      placement="right"
      :show-after="300"
    >
      <div
        class="activity-bar-item"
        :class="{ 'is-active': uiStore.activePanel === item.id }"
        @click="uiStore.activePanel = item.id"
      >
        <el-icon :size="20">
          <component :is="item.icon" />
        </el-icon>
      </div>
    </el-tooltip>
    <!-- 设置图标（不切换面板，直接开弹窗） -->
    <el-tooltip content="设置" placement="right" :show-after="300">
      <div
        class="activity-bar-item"
        @click="$emit('openSettings')"
      >
        <el-icon :size="20">
          <Setting />
        </el-icon>
      </div>
    </el-tooltip>
    <!-- 终端图标（底部） -->
    <div class="activity-bar-spacer"></div>
    <el-tooltip content="终端" placement="right" :show-after="300">
      <div
        class="activity-bar-item"
        :class="{ 'is-active': uiStore.terminalVisible }"
        @click="$emit('toggleTerminal')"
      >
        <el-icon :size="20">
          <Monitor />
        </el-icon>
      </div>
    </el-tooltip>
  </div>
</template>

<script setup>
import { Folder, MagicStick, SetUp, Setting, Monitor, TrendCharts, DataBoard } from '@element-plus/icons-vue'
import { useUiStore } from '../store'

const uiStore = useUiStore()

defineEmits(['toggleTerminal', 'openSettings'])

const panels = [
  { id: 'directory', icon: Folder, label: '工作目录' },
  { id: 'ai', icon: MagicStick, label: 'AI 功能' },
  { id: 'toolbox', icon: SetUp, label: '工具箱' },
  { id: 'dashboard', icon: DataBoard, label: '状态看板' },
  { id: 'stats', icon: TrendCharts, label: '仓库统计' }
]
</script>

<style scoped>
.activity-bar {
  width: 48px;
  flex-shrink: 0;
  background: var(--sidebar-bg);
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 12px 0;
  gap: 6px;
  border-right: 1px solid var(--border-color);
}

.activity-bar-spacer {
  flex: 1;
}

.activity-bar-item {
  position: relative;
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  cursor: pointer;
  color: var(--sidebar-text);
  transition: background var(--transition-fast), color var(--transition-fast);
}

.activity-bar-item:hover {
  background: var(--sidebar-hover);
  color: var(--sidebar-text-hover);
}

/* active：左侧指示条 + 主色填充，去 scale（避免与 hover 重复反馈）
 * 指示条 left:0 贴 item 左缘（item 36px 居中于 48px bar，两侧 6px 间隙），
 * 禁用负 left——.home overflow:hidden!important 会裁剪溢出 item 的部分 */
.activity-bar-item.is-active {
  background: var(--sidebar-active-bg);
  color: var(--sidebar-active-text);
}

.activity-bar-item.is-active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  border-radius: 0 2px 2px 0;
  background: var(--primary-light);
}
</style>
