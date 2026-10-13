<template>
  <div class="toolbox-panel">
    <div class="toolbox-header">
      <span class="toolbox-title"><el-icon :size="18" style="margin-right:4px;vertical-align:middle;"><ToolboxIcon /></el-icon>工具箱</span>
      <span class="toolbox-close" @click="$emit('close')">&#10005;</span>
    </div>
    <div class="toolbox-content">
      <div
        v-for="tool in tools"
        :key="tool.id"
        class="toolbox-item"
        @click="tool.handler"
      >
        <div class="toolbox-item-header">
          <el-icon :size="16"><component :is="tool.icon" /></el-icon>
          <span class="toolbox-item-name">{{ tool.name }}</span>
        </div>
        <div class="toolbox-item-desc">{{ tool.description }}</div>
      </div>
    </div>

    <!-- 拷贝到对话框 -->
    <el-dialog
      v-model="copyToDialogVisible"
      title="拷贝到"
      width="480px"
      append-to-body
    >
      <el-form label-width="100px">
        <el-form-item label="原地址">
          <el-input
            v-model="copyToSourcePath"
            placeholder="请输入原文件或文件夹路径"
            :disabled="copyToLoading"
          />
        </el-form-item>
        <div class="swap-row">
          <el-button
            text
            size="small"
            :disabled="copyToLoading"
            @click="swapCopyToPaths"
          >
            <el-icon class="swap-icon"><Sort /></el-icon>
            互换
          </el-button>
        </div>
        <el-form-item label="目标地址">
          <el-input
            ref="copyToTargetInputRef"
            v-model="copyToTargetPath"
            placeholder="请输入目标文件夹路径"
            :disabled="copyToLoading"
            @keyup.enter="handleCopyTo"
          />
        </el-form-item>
        <el-form-item label="文件名">
          <el-input
            v-model="copyToTargetName"
            placeholder="默认与源同名（仅文件生效），可修改"
            :disabled="copyToLoading"
            clearable
            @keyup.enter="handleCopyTo"
          />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="copyToWholeDir" :disabled="copyToLoading">
            包含文件夹本身
          </el-checkbox>
        </el-form-item>
      </el-form>
      <div v-if="copyToPreview" class="copy-to-preview">
        <div class="copy-to-preview-label">拷贝效果预览</div>
        <div class="copy-to-preview-row">{{ copyToPreview.from }}</div>
        <div class="copy-to-preview-arrow">↓</div>
        <div class="copy-to-preview-row">{{ copyToPreview.to }}</div>
      </div>
      <template #footer>
        <el-button @click="copyToDialogVisible = false" :disabled="copyToLoading">取消</el-button>
        <el-button type="primary" @click="handleCopyTo" :loading="copyToLoading">确定</el-button>
      </template>
    </el-dialog>
    <!-- 进程管理：检索端口/进程名并结束进程（独立组件，弹窗尺寸按列表内容量走 900 档） -->
    <ProcessManagerDialog v-model="processManagerVisible" />
    <!-- 端口占用：监听端口一览 + 行内终止（复用进程枚举与停止链路底座，零后端新增） -->
    <PortListDialog v-model="portListVisible" />
    <!-- JSON/YAML 工具：校验/格式化/压缩/互转，纯前端实现（js-yaml） -->
    <JsonYamlToolDialog v-model="jsonYamlVisible" />
    <!-- 环境变量：系统/用户/进程三来源只读查看（Windows 三组 / Linux 仅进程组），敏感值打码 -->
    <EnvVarDialog v-model="envVarsVisible" />
    <!-- hosts 文件：本机 hosts 只读解析查看（IP/主机名/注释），支持过滤与复制 -->
    <HostsDialog v-model="hostsVisible" />
  </div>
</template>

<script setup>
import { ref, computed, nextTick, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Connection, CopyDocument, Cpu, Document, Guide, Key, Sort } from '@element-plus/icons-vue'
import { CopyTo } from '../../wailsjs/go/main/App'
import ProcessManagerDialog from './ProcessManagerDialog.vue'
import PortListDialog from './PortListDialog.vue'
import JsonYamlToolDialog from './JsonYamlToolDialog.vue'
import EnvVarDialog from './EnvVarDialog.vue'
import HostsDialog from './HostsDialog.vue'
import ToolboxIcon from './icons/ToolboxIcon.vue'

defineEmits(['close'])

// ---- 工具项定义 ----
const copyToDialogVisible = ref(false)
const processManagerVisible = ref(false)
const portListVisible = ref(false)
const jsonYamlVisible = ref(false)
const envVarsVisible = ref(false)
const hostsVisible = ref(false)

// JSON/YAML 工具弹窗：纯前端无枚举，无需 @open 预拉数据
const openJsonYamlTool = () => {
  jsonYamlVisible.value = true
}

// 进程管理弹窗：打开动作只置可见，首次枚举由组件 @open 触发（destroy-on-close 下每次打开重拉）
const openProcessManager = () => {
  processManagerVisible.value = true
}

// 端口占用弹窗：同上，打开时机交由组件 @open
const openPortList = () => {
  portListVisible.value = true
}

// 环境变量弹窗：同上，打开时机交由组件 @open
const openEnvVars = () => {
  envVarsVisible.value = true
}

// hosts 文件弹窗：同上，打开时机交由组件 @open
const openHosts = () => {
  hostsVisible.value = true
}

const copyToSourcePath = ref('')
const copyToTargetPath = ref('')
const copyToWholeDir = ref(true)
const copyToLoading = ref(false)
const copyToTargetInputRef = ref()

const showCopyToDialog = () => {
  copyToSourcePath.value = ''
  copyToTargetPath.value = ''
  copyToTargetName.value = ''
  copyToWholeDir.value = true
  copyToLoading.value = false
  copyToDialogVisible.value = true
  nextTick(() => {
    const input = copyToTargetInputRef.value?.input
    if (input) input.focus()
  })
}

// 自定义目标文件名：源路径手填无类型信息，输入框始终显示，默认取源路径最后一段；
// 源实际是目录时由后端忽略并在返回串中提示
const copyToTargetName = ref('')

const defaultCopyToName = () => {
  const src = copyToSourcePath.value.trim().replaceAll('\\', '/')
  return src.split('/').pop() || ''
}

// 源路径变化时同步默认名：仅当用户未自行改名（当前值为空或等于旧默认名）时跟随，
// 避免覆盖用户已输入的自定义名
watch(copyToSourcePath, (newVal, oldVal) => {
  const oldSrc = (oldVal || '').trim().replaceAll('\\', '/')
  const oldDefault = oldSrc.split('/').pop() || ''
  if (!copyToTargetName.value || copyToTargetName.value === oldDefault) {
    copyToTargetName.value = defaultCopyToName()
  }
})

const copyToPreview = computed(() => {
  const src = copyToSourcePath.value.trim().replaceAll('\\', '/')
  const dst = copyToTargetPath.value.trim().replaceAll('\\', '/')
  if (!src || !dst) return null
  const srcName = src.split('/').pop() || ''
  const normalizedDst = dst.replace(/\/+$/, '')
  if (copyToWholeDir.value) {
    const finalName = copyToTargetName.value.trim() || srcName
    return { from: src, to: normalizedDst + '/' + finalName }
  }
  return { from: src + '/*', to: normalizedDst + '/*' }
})

// 互换原地址与目标地址；文件名重置为新源路径的默认值（源已变，旧自定义名不再适用）
const swapCopyToPaths = () => {
  const temp = copyToSourcePath.value
  copyToSourcePath.value = copyToTargetPath.value
  copyToTargetPath.value = temp
  copyToTargetName.value = defaultCopyToName()
}

// 文件名非法字符校验（与 Windows 资源管理器一致 + 路径分隔符）
const ILLEGAL_NAME_RE = /[\\/:*?"<>|]/

const handleCopyTo = async () => {
  if (!copyToSourcePath.value.trim()) {
    ElMessage.warning('请输入原地址')
    return
  }
  if (!copyToTargetPath.value.trim()) {
    ElMessage.warning('请输入目标地址')
    return
  }
  const targetName = copyToTargetName.value.trim()
  if (targetName && (ILLEGAL_NAME_RE.test(targetName) || targetName === '.' || targetName === '..')) {
    ElMessage.warning('文件名含非法字符（\\ / : * ? " < > |）')
    return
  }
  copyToLoading.value = true
  try {
    const result = await CopyTo(copyToSourcePath.value, copyToTargetPath.value, targetName, copyToWholeDir.value)
    if (result && result.startsWith('错误')) {
      ElMessage.error(result)
    } else {
      // 源为目录且自定义名被忽略时，后端在结果串附提示，完整展示给用户
      ElMessage.success(result ? `拷贝成功 ${result}` : '拷贝成功')
      copyToDialogVisible.value = false
    }
  } catch (error) {
    ElMessage.error('拷贝失败: ' + (error.message || String(error)))
  } finally {
    copyToLoading.value = false
  }
}

const tools = [
  {
    id: 'copyTo',
    name: '拷贝到',
    icon: CopyDocument,
    description: '将文件/文件夹拷贝到指定目录',
    handler: showCopyToDialog
  },
  {
    id: 'processManager',
    name: '进程管理',
    icon: Cpu,
    description: '按端口或进程名检索并结束进程',
    handler: openProcessManager
  },
  {
    id: 'portList',
    name: '端口占用',
    icon: Connection,
    description: '查看本机监听端口与占用进程，可快速终止',
    handler: openPortList
  },
  {
    id: 'jsonYamlTool',
    name: 'JSON/YAML 工具',
    icon: Document,
    description: 'JSON/YAML 校验、格式化、压缩与互转',
    handler: openJsonYamlTool
  },
  {
    id: 'envVars',
    name: '环境变量',
    icon: Key,
    description: '查看系统/用户/进程环境变量，敏感值打码',
    handler: openEnvVars
  },
  {
    id: 'hosts',
    name: 'hosts 文件',
    icon: Guide,
    description: '查看本机 hosts 解析映射，支持过滤与复制',
    handler: openHosts
  }
]
</script>

<style scoped>
.toolbox-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  background-color: var(--bg-primary);
}

.toolbox-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
}

.toolbox-title {
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 0.5px;
}

.toolbox-close {
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

.toolbox-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-sm);
}

.toolbox-item {
  padding: 10px 12px;
  background: var(--bg-secondary);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-color);
  cursor: pointer;
  margin-bottom: 6px;
  transition: all var(--transition-normal);
}

.toolbox-item:hover {
  border-color: var(--primary-light);
  box-shadow: var(--shadow-sm);
  background: var(--primary-bg);
  transform: translateX(2px);
}

.toolbox-item:active {
  transform: scale(0.98);
}

.toolbox-item-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-primary);
  font-weight: 500;
}

.toolbox-item:hover .toolbox-item-header {
  color: var(--primary-color);
}

.toolbox-item-desc {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-top: 3px;
}

/* 互换按钮行 */
.swap-row {
  display: flex;
  justify-content: center;
  margin: -8px 0 0;
}

.swap-row .el-button {
  color: var(--text-tertiary, #909399);
  font-size: 12px;
}

.swap-row .el-button:hover {
  color: var(--primary-color, #409eff);
}

.swap-row .el-button:hover .swap-icon {
  transform: rotate(180deg);
}

.swap-icon {
  transition: transform 0.3s ease;
}
</style>
