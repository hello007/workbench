<template>
  <div class="toolbox-panel">
    <!-- 顶部标题栏：渐变背景，对齐 AiFunctionPanel .panel-header 风格 -->
    <div class="panel-header">
      <div class="panel-heading">
        <span class="panel-title">
          <el-icon :size="18" class="panel-title-icon"><ToolboxIcon /></el-icon>
          工具箱
        </span>
        <span class="panel-subtitle">常用开发工具聚合</span>
      </div>
      <span class="toolbox-close" title="关闭工具箱" @click="$emit('close')">&#10005;</span>
    </div>
    <div class="toolbox-layout">
      <!-- 左：功能清单（active 左指示条 left:0，防 overflow:hidden 裁剪） -->
      <div class="toolbox-sidebar">
        <div
          v-for="tool in tools"
          :key="tool.id"
          class="toolbox-nav-item"
          :class="{ 'is-active': activeToolId === tool.id }"
          :title="tool.description"
          @click="activeToolId = tool.id"
        >
          <el-icon :size="16" class="toolbox-nav-icon"><component :is="tool.icon" /></el-icon>
          <span class="toolbox-nav-name">{{ tool.name }}</span>
        </div>
      </div>
      <!-- 右：选中工具内容面板（component :is 切换即卸载重建，onMounted 拉数据
           自然替代原弹窗 destroy-on-close + @open 语义） -->
      <div class="toolbox-main">
        <component :is="activeToolComponent" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { Connection, CopyDocument, Cpu, Document, Files, Guide, Key } from '@element-plus/icons-vue'
import CopyToPanel from './CopyToPanel.vue'
import ProcessManagerPanel from './ProcessManagerPanel.vue'
import PortListPanel from './PortListPanel.vue'
import JsonYamlToolPanel from './JsonYamlToolPanel.vue'
import EnvVarPanel from './EnvVarPanel.vue'
import HostsPanel from './HostsPanel.vue'
import PptSplitPanel from './PptSplitPanel.vue'
import ToolboxIcon from './icons/ToolboxIcon.vue'

defineEmits(['close'])

// ---- 工具项定义：id -> 内容组件映射（点清单项右侧切换，默认选中首个） ----
const tools = [
  { id: 'copyTo', name: '拷贝到', icon: CopyDocument, component: CopyToPanel },
  { id: 'processManager', name: '进程管理', icon: Cpu, component: ProcessManagerPanel },
  { id: 'portList', name: '端口占用', icon: Connection, component: PortListPanel },
  { id: 'jsonYamlTool', name: 'JSON/YAML 工具', icon: Document, component: JsonYamlToolPanel },
  { id: 'envVars', name: '环境变量', icon: Key, component: EnvVarPanel },
  { id: 'hosts', name: 'hosts 文件', icon: Guide, component: HostsPanel },
  { id: 'pptSplit', name: 'PPT 拆分', icon: Files, component: PptSplitPanel }
]

const activeToolId = ref('copyTo')
const activeToolComponent = computed(() => tools.find((t) => t.id === activeToolId.value)?.component)
</script>

<style scoped>
/* 根元素 flex 约束链（Home 一级面板规范：缺失任一环则面板塌为内容高度或挤压终端） */
.toolbox-panel {
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  height: 100%;
  width: 100%;
  background-color: var(--bg-primary);
}

/* 顶部标题栏：风格对齐 AiFunctionPanel .panel-header（渐变背景 + 16px/700 标题） */
.panel-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
}

.panel-heading {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
}

.panel-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 0.5px;
  white-space: nowrap;
}

.panel-subtitle {
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.panel-title-icon {
  color: var(--primary-color);
}

.toolbox-close {
  flex-shrink: 0;
  font-size: 16px;
  color: var(--text-tertiary);
  cursor: pointer;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  transition: all var(--transition-normal);
}

.toolbox-close:hover {
  color: var(--text-primary);
  background: var(--bg-tertiary);
}

/* 主体左右分栏：左功能清单（导航）+ 右内容区（画布） */
.toolbox-layout {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: var(--spacing-md);
  padding: var(--spacing-md) var(--spacing-md);
}

.toolbox-sidebar {
  width: 208px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow-y: auto;
  padding-right: var(--spacing-md);
  border-right: 1px solid var(--border-color);
}

/* 清单项：active 左侧 3px 指示条（left:0 贴 item 左缘，禁负 left 防 overflow 裁剪） */
.toolbox-nav-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: 10px 12px;
  border-radius: var(--radius-md);
  cursor: pointer;
  color: var(--text-primary);
  transition: background var(--transition-fast), color var(--transition-fast);
}

.toolbox-nav-item:hover {
  background: var(--bg-tertiary);
}

.toolbox-nav-item.is-active {
  background: var(--primary-bg);
  color: var(--primary-color);
}

.toolbox-nav-item.is-active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  background: var(--primary-color);
}

.toolbox-nav-icon {
  flex-shrink: 0;
}

.toolbox-nav-name {
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 右侧内容画布：白底圆角卡片浮于面板底色上（对齐 AiFunctionPanel .ai-main 分层），
   内部滚动（内容溢出仅内容区滚动，侧栏固定不动） */
.toolbox-main {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
}
</style>
