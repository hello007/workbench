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
    <!-- 多设备连接指示 + 连接明细面板（D2 决策：el-popover 点击展开）：仅存在
         其他设备接入时显示，hover 走 tooltip 提示层，点击展开逐连接明细（IP /
         token 名 / 接入时间）。明细仅桌面通道有数据（隐私红线：他人 IP/token
         明细不进浏览器通道，浏览器端面板显示引导文案）。深色容器图标用内联
         SVG fill=currentColor（视觉规范），计数角标走 primary 语义色。
         嵌套次序实证约束（2026-10-23，真实 EP 组件 jsdom 双链路验证）：tooltip
         与 popover 各自的 forwardRef 指令都必须落在原生元素上——popover 外层 +
         tooltip 进 #reference 时 popover 触发器绑定失效点击永不展开（Vue 警告
         "Runtime directive used on component with non-element root node"）；
         故终态为 tooltip 外层，tooltip 与 popover 之间加 anchor 原生 div 隔离，
         popover #reference 直挂原生 div，点击展开与 hover 提示双链路实测通过 -->
    <el-tooltip v-if="multiDevice" :content="deviceHint" placement="right" :show-after="300">
      <div class="devices-indicator-anchor">
        <el-popover trigger="click" placement="right" :width="320" :persistent="false">
          <template #reference>
            <div class="activity-bar-item activity-bar-devices">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none">
                <rect x="2" y="4" width="13" height="10" rx="1.5" stroke="currentColor" stroke-width="1.6"/>
                <rect x="9" y="10" width="13" height="10" rx="1.5" stroke="currentColor" stroke-width="1.6" fill="var(--sidebar-bg)"/>
                <rect x="9" y="10" width="13" height="10" rx="1.5" stroke="currentColor" stroke-width="1.6"/>
              </svg>
              <span class="devices-badge">{{ deviceCount }}</span>
            </div>
          </template>
          <div class="devices-panel">
            <div class="devices-panel-title">已连接设备（{{ deviceCount }}）</div>
            <div v-if="connections.length" class="devices-panel-list">
              <div v-for="c in connections" :key="c.id" class="devices-panel-row">
                <span class="devices-panel-ip" :title="c.ip">{{ c.ip }}</span>
                <span class="devices-panel-token">{{ c.tokenName || '已撤销令牌' }}</span>
                <span class="devices-panel-time">{{ formatConnTime(c.connectedAt) }}</span>
                <!-- 踢出（断开该连接，非封禁）：popover 内容区直挂按钮，不在
                     tooltip/popover reference 链上，无嵌套 forwardRef 约束问题 -->
                <button
                  class="devices-panel-kick"
                  type="button"
                  title="踢出该连接"
                  @click.stop="kickConnection(c)"
                >
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                    <path d="M18 6 6 18M6 6l12 12" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"/>
                  </svg>
                </button>
              </div>
            </div>
            <div v-else class="devices-panel-empty">
              {{ isDesktopChannel ? '无设备连接' : '明细仅在桌面端显示' }}
            </div>
            <div class="devices-panel-hint">多端共享同一后端状态，后操作覆盖先操作</div>
          </div>
        </el-popover>
      </div>
    </el-tooltip>
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
import { computed } from 'vue'
import { Folder, MagicStick, Setting, Monitor, TrendCharts, DataBoard, ChatDotRound } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'
import ToolboxIcon from './icons/ToolboxIcon.vue'
import AssetsIcon from './icons/AssetsIcon.vue'
import { KickConnection } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import { useUiStore } from '../store'
import { useDeviceConnections } from '../composables/useDeviceConnections'

const uiStore = useUiStore()

defineEmits(['toggleTerminal', 'openSettings'])

// 多设备连接感知（PR3 语义明示 + 明细面板 D2）：指示项显示条件沿用 composable
// 分流口径（桌面 len>=1 / 浏览器 count>1），明细列表与通道归属供 popover 消费
const { deviceCount, multiDevice, connections, isDesktopChannel } = useDeviceConnections()
const deviceHint = computed(() =>
  `${deviceCount.value} 个设备已连接：点击查看明细`
)

/**
 * 踢出单个连接（断开该 WS 连接，非封禁）：单次确认（confirm-dialog 单次确认
 * 形态）→ 调 KickConnection → 错误走 handleError 分流。警示文案明示「对端可用
 * 同一令牌自动重连」与「防重连请配合撤销令牌（撤销不影响已建立连接，两手段
 * 互补）」。行消失靠明细事件自动更新（connections 全量快照落地），不手动删。
 */
async function kickConnection(c) {
  try {
    await ElMessageBox.confirm(
      `踢出后该连接（${c.ip}）将立即断开；对端可用同一令牌自动重连（踢出不是封禁）。如需阻止其再连，请到设置中撤销对应令牌——撤销不影响已建立连接，两手段互补。是否踢出？`,
      '踢出连接',
      { type: 'warning', confirmButtonText: '踢出', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  try {
    await KickConnection(c.id)
  } catch (e) {
    handleError('踢出失败: ', e)
  }
}

// 明细接入时间格式化：非法时间返回 '--'；同日只显 HH:mm:ss，跨日加 MM-dd 前缀
const formatConnTime = (iso) => {
  const t = new Date(iso)
  if (Number.isNaN(t.getTime())) {
    return '--'
  }
  const time = t.toLocaleTimeString('zh-CN', { hour12: false })
  if (t.toDateString() === new Date().toDateString()) {
    return time
  }
  const date = t.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' })
  return `${date} ${time}`
}

const panels = [
  { id: 'directory', icon: Folder, label: '工作目录' },
  { id: 'ai', icon: MagicStick, label: 'AI 功能' },
  { id: 'ai-chat', icon: ChatDotRound, label: 'AI 对话' },
  { id: 'toolbox', icon: ToolboxIcon, label: '工具箱' },
  { id: 'assets', icon: AssetsIcon, label: '素材管理' },
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

/* 多设备连接指示：计数角标缀于图标右下（primary 语义色，克制不抢眼） */
.activity-bar-devices .devices-badge {
  position: absolute;
  right: 3px;
  bottom: 3px;
  min-width: 14px;
  height: 14px;
  padding: 0 3px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  line-height: 1;
  font-weight: 600;
  color: var(--primary-color);
  background: var(--bg-secondary);
  border: 1px solid var(--primary-color);
  border-radius: 999px;
}

/* 多设备连接明细面板（D2 决策，popover 内容）：标题 + 三列明细 + 空态 + 底部
 * 共享语义提示，全部走语义变量与四档间距/圆角；明细行 hover 用弱层背景轻提亮 */
.devices-panel {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.devices-panel-title {
  font-size: 13px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.devices-panel-list {
  max-height: 240px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.devices-panel-row {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr) auto auto;
  gap: var(--spacing-sm);
  align-items: center;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-sm);
}

.devices-panel-row:hover {
  background: var(--bg-tertiary);
}

/* 踢出按钮：danger 语义色小图标钮（hover 提亮背景），行内紧凑不抢明细信息 */
.devices-panel-kick {
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-tertiary);
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.devices-panel-kick:hover {
  background: color-mix(in srgb, var(--danger-color) 14%, transparent);
  color: var(--danger-color);
}

.devices-panel-ip {
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  font-size: 12px;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  user-select: text;
}

.devices-panel-token {
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.devices-panel-time {
  font-size: 11px;
  color: var(--text-tertiary);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.devices-panel-empty {
  padding: var(--spacing-md) var(--spacing-sm);
  text-align: center;
  font-size: 12px;
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
}

.devices-panel-hint {
  padding-top: var(--spacing-sm);
  border-top: 1px solid var(--border-color);
  font-size: 11px;
  line-height: 1.5;
  color: var(--text-tertiary);
}
</style>
