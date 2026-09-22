<template>
  <div v-show="uiStore.terminalVisible" class="terminal-panel">
    <!-- 工具栏：tab 序列融入单行 36px 工具栏；单 tab 时退化为徽章模式（D1 零视觉噪音） -->
    <div class="terminal-toolbar">
      <div class="terminal-toolbar-left">
        <!-- 多 tab：tab 序列（仅 ≥2 tab 时显示） -->
        <div v-if="tabs.length > 1" ref="tabStrip" class="tab-strip" @wheel.prevent="onTabStripWheel">
          <div
            v-for="tab in tabs"
            :key="tab.id"
            class="terminal-tab"
            :class="{ 'is-active': tab.id === activeId, 'is-exited': tab.isExited }"
            :title="tab.dir"
            @click="onActivateTab(tab.id)"
            @auxclick.middle.prevent="onCloseTab(tab.id)"
          >
            <span v-if="tab.isExited" class="tab-exit-dot" title="会话已退出"></span>
            <span class="tab-title">{{ tab.title }}</span>
            <span class="tab-dir">{{ dirBasename(tab.dir) }}</span>
            <span class="tab-close" title="关闭终端" @click.stop="onCloseTab(tab.id)">
              <svg width="10" height="10" viewBox="0 0 10 10" fill="none">
                <path d="M2 2L8 8M8 2L2 8" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/>
              </svg>
            </span>
          </div>
        </div>
        <!-- 单 tab：shell 徽章 + 路径徽章（延续原工具栏信息结构；Shell 下拉职责移至新建行为 + 设置页默认值） -->
        <template v-else-if="activeTab">
          <div class="shell-badge">
            <span class="shell-dot" :class="{ 'is-exited': activeTab.isExited }"></span>
            <span class="shell-name">{{ activeTab.title }}</span>
          </div>
          <div class="terminal-path">
            <el-icon :size="12" class="path-icon"><Folder /></el-icon>
            <span class="path-text" :title="activeTab.dir">{{ activeTab.dir }}</span>
          </div>
        </template>
        <!-- 新建 tab：目录继承文件树当前目录（D2），Shell 类型取设置页默认值；达上限禁用并提示 -->
        <span
          class="toolbar-btn add-tab-btn"
          :class="{ 'is-disabled': atLimit }"
          :title="atLimit ? `最多支持 ${MAX_TERMINAL_TABS} 个终端` : '新建终端'"
          @click="onAddTab"
        >
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
            <path d="M7 2.5V11.5M2.5 7H11.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
          </svg>
        </span>
      </div>
      <div class="terminal-toolbar-right">
        <transition name="fade">
          <el-button
            v-if="activeTab && activeTab.isExited"
            size="small"
            type="primary"
            text
            class="restart-btn"
            @click="onRestart"
          >
            <el-icon :size="14"><RefreshRight /></el-icon>
            重新启动
          </el-button>
        </transition>
        <div class="toolbar-actions">
          <span class="toolbar-btn minimize-btn" @click="$emit('toggle')" title="收起终端">
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
              <rect x="2" y="7" width="10" height="1.5" rx="0.75" fill="currentColor"/>
            </svg>
          </span>
        </div>
      </div>
    </div>
    <!-- 终端区域：每 tab 独立容器，v-show 仅活动 tab 可见（保 DOM 保会话，xterm 不重挂载） -->
    <div ref="terminalArea" class="terminal-area">
      <div
        v-for="tab in tabs"
        :key="tab.id"
        v-show="tab.id === activeId"
        :ref="el => setTabContainer(tab.id, el)"
        class="terminal-container"
      ></div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { Folder, RefreshRight } from '@element-plus/icons-vue'
import { useTerminal } from '../composables/useTerminal'
import { useTerminalTabs, MAX_TERMINAL_TABS } from '../composables/useTerminalTabs'
import { useUiStore } from '../store'
import { GetShellConfigs, GetSettings } from '../../wailsjs/go/main/App'

const uiStore = useUiStore()

defineEmits(['toggle'])

// tab 列表状态（useTerminalTabs）与 xterm 实例（Map: tabId → useTerminal()）分离持有：
// tab 状态驱动渲染（tab 栏/容器显隐），实例句柄仅生命周期方法消费（init/resize/focus/destroy）
const { tabs, activeId, atLimit, createTab, activateTab, closeTab, markExited, markRunning } = useTerminalTabs()

const terminalArea = ref(null)
const tabStrip = ref(null)
const defaultShell = ref('powershell')
const shellConfigs = ref([])
const settingsReady = ref(false)

// 实例与辅助资源登记表（非响应式 Map/Set：渲染只依赖 tab 状态，实例句柄无渲染依赖）
const instances = new Map()          // tabId → useTerminal() 实例
const tabContainers = new Map()      // tabId → 容器 DOM（v-for 函数 ref 登记）
const initedTabs = new Set()         // 已执行 initTerminal 的 tab（每 tab 仅首次激活 init）
const stopExitedWatchers = new Map() // tabId → isExited 同步 watch 停止函数（关闭/卸载时精准注销）

const activeTab = computed(() => tabs.value.find(t => t.id === activeId.value) || null)

// 加载 Shell 配置和默认 Shell 设置
onMounted(async () => {
  try {
    shellConfigs.value = await GetShellConfigs()
  } catch {
    shellConfigs.value = [
      { type: 'powershell', displayName: 'PowerShell' },
      { type: 'cmd', displayName: 'CMD' },
      { type: 'gitbash', displayName: 'Git Bash' },
      { type: 'wsl', displayName: 'WSL' }
    ]
  }
  // 读取用户设置的默认 Shell 类型（新建 tab 使用）
  try {
    const settings = await GetSettings()
    if (settings.defaultShell) {
      defaultShell.value = settings.defaultShell
    }
  } catch {
    // 读取失败则保持默认 powershell
  }
  settingsReady.value = true
})

function shellDisplayName(type) {
  const cfg = shellConfigs.value.find(c => c.type === type)
  return cfg ? cfg.displayName : type
}

// 目录尾段名（tab 副标签）：'D:\\work\\demo' → 'demo'，兼容正反斜杠；
// 根目录（'C:\\'）尾段为空时回退盘符形式 'C:'，避免副标签空白
function dirBasename(dir) {
  if (!dir) return ''
  const sep = Math.max(dir.lastIndexOf('\\'), dir.lastIndexOf('/'))
  if (sep < 0) return dir
  const base = dir.slice(sep + 1)
  if (base) return base
  return dir.replace(/[\\/]+$/, '') || dir
}

// v-for 函数 ref：登记/注销各 tab 容器 DOM
function setTabContainer(id, el) {
  if (el) {
    tabContainers.set(id, el)
  } else {
    tabContainers.delete(id)
  }
}

// tab 溢出时垂直滚轮转横向滚动（滚动条已隐藏，鼠标滚轮为唯一可达滚动方式；
// 触摸板 deltaX 原生横滑不受影响）
function onTabStripWheel(e) {
  const strip = tabStrip.value
  if (!strip) return
  strip.scrollLeft += (Math.abs(e.deltaX) > Math.abs(e.deltaY) ? e.deltaX : e.deltaY)
}

// 为 tab 创建 xterm 实例，并登记 isExited → tab 状态的同步 watch
// （会话退出/重启恢复双向回流至 tab 标记；停止函数入表，关闭/卸载时精准注销）
function createInstanceForTab(tab) {
  const inst = useTerminal()
  instances.set(tab.id, inst)
  const stop = watch(inst.isExited, (exited) => {
    if (exited) {
      markExited(tab.id)
    } else {
      markRunning(tab.id)
    }
  })
  stopExitedWatchers.set(tab.id, stop)
  return inst
}

// 面板首次可见时创建第一个 tab（沿用原「首次可见才 init」节奏，单终端体验与改造前一致）
watch(
  [() => uiStore.terminalVisible, settingsReady],
  async ([val, ready]) => {
    if (val && ready) {
      if (tabs.value.length === 0) {
        const tab = createTab({
          dir: uiStore.terminalDir || 'C:\\',
          shellType: defaultShell.value,
          title: shellDisplayName(defaultShell.value)
        })
        if (tab) {
          createInstanceForTab(tab)
        }
      }
      await ensureActiveTabReady()
    }
  },
  { flush: 'post' }
)

// 初始化 tab 的 xterm 实例（每 tab 仅首次激活时 init；容器可见后 fit 才有效，
// xterm 在 display:none 下无法正确 fit，故先 nextTick 确保 v-show 放行）
async function initTab(tab) {
  const inst = instances.get(tab.id)
  if (!inst || initedTabs.has(tab.id)) return
  initedTabs.add(tab.id)
  await nextTick()
  const container = tabContainers.get(tab.id)
  if (!container) {
    initedTabs.delete(tab.id)
    return
  }
  await inst.initTerminal(container, tab.dir || 'C:\\', tab.shellType)
  inst.focus()
}

// 活动 tab 就绪：未 init 则首次 init；已 init 则 refit + focus
// （切换/展开时重估尺寸，对齐 xterm 多实例官方实践：隐藏期 write 正常缓冲，激活时 refit）
async function ensureActiveTabReady() {
  const tab = activeTab.value
  if (!tab || !uiStore.terminalVisible) return
  if (!initedTabs.has(tab.id)) {
    await initTab(tab)
    return
  }
  await nextTick()
  const inst = instances.get(tab.id)
  if (inst) {
    inst.resize()
    inst.focus()
  }
}

// 新建 tab：目录继承文件树当前目录（D2 继承语义），Shell 类型取设置页默认值
async function onAddTab() {
  if (atLimit.value) return
  const tab = createTab({
    dir: uiStore.terminalDir || 'C:\\',
    shellType: defaultShell.value,
    title: shellDisplayName(defaultShell.value)
  })
  if (!tab) return
  createInstanceForTab(tab)
  await ensureActiveTabReady()
}

// 切换 tab：激活状态流转 + 目标 tab 首次 init / 已 init refit + focus
function onActivateTab(id) {
  if (id === activeId.value) return
  activateTab(id)
  ensureActiveTabReady()
}

// 关闭 tab（× 点击 / 鼠标中键）：注销同步 watcher + 销毁实例；
// 关闭活动 tab 后邻位自动激活（closeTab 内流转），并就绪新活动 tab
async function onCloseTab(id) {
  const closedWasActive = id === activeId.value
  if (!closeTab(id)) return
  const stop = stopExitedWatchers.get(id)
  if (stop) {
    stop()
    stopExitedWatchers.delete(id)
  }
  const inst = instances.get(id)
  instances.delete(id)
  initedTabs.delete(id)
  if (inst) {
    await inst.destroyTerminal()
  }
  if (closedWasActive) {
    await ensureActiveTabReady()
  }
}

// 重新启动：针对当前活动 tab，按其原目录与 Shell 类型重建会话
async function onRestart() {
  const tab = activeTab.value
  const inst = tab ? instances.get(tab.id) : null
  const container = tab ? tabContainers.get(tab.id) : null
  if (!tab || !inst || !container) return
  await inst.restartTerminal(container, tab.dir, tab.shellType)
}

// 窗口 resize 监听：观察终端区域，尺寸变化时 refit 活动 tab 实例
let resizeObserver = null

onMounted(() => {
  resizeObserver = new ResizeObserver(() => {
    if (uiStore.terminalVisible && activeTab.value && initedTabs.has(activeTab.value.id)) {
      const inst = instances.get(activeTab.value.id)
      if (inst) {
        inst.resize()
      }
    }
  })
  // 观察终端区域容器，检测拖拽调整高度等尺寸变化（隐藏 tab 于激活时 refit）
  if (terminalArea.value) {
    resizeObserver.observe(terminalArea.value)
  }
})

// 卸载：销毁全部 tab 实例（循环 destroyTerminal），停掉全部同步 watcher
onBeforeUnmount(async () => {
  if (resizeObserver) {
    resizeObserver.disconnect()
  }
  stopExitedWatchers.forEach(stop => stop())
  stopExitedWatchers.clear()
  const destroyAll = Array.from(instances.values()).map(inst => inst.destroyTerminal())
  instances.clear()
  initedTabs.clear()
  await Promise.all(destroyAll)
})
</script>

<style scoped>
/* ── 面板容器 ── */
.terminal-panel {
  display: flex;
  flex-direction: column;
  background-color: var(--bg-primary);
  border-top: 1px solid var(--border-color, #e2e8f0);
  height: 100%;
  position: relative;
}

/* 顶部光晕装饰线：主色 20% 透明度派生（禁硬编码 rgba，随主题主色联动） */
.terminal-panel::before {
  content: '';
  position: absolute;
  top: 0;
  left: 10%;
  right: 10%;
  height: 1px;
  background: linear-gradient(90deg, transparent, color-mix(in srgb, var(--primary-color) 20%, transparent), transparent);
}

/* ── 工具栏 ── */
.terminal-toolbar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 36px;
  padding: 0 12px;
  background: linear-gradient(180deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
  border-bottom: 1px solid var(--border-color, #e2e8f0);
}

.terminal-toolbar-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

/* ── tab 序列 ── */
.tab-strip {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
  flex: 1;
  overflow-x: auto;
  overflow-y: hidden;
  /* tab 溢出横向滚动（禁换行压缩），滚动条隐藏保持工具栏干净 */
  scrollbar-width: none;
}

.tab-strip::-webkit-scrollbar {
  display: none;
}

.terminal-tab {
  position: relative;
  display: flex;
  align-items: center;
  gap: 4px;
  height: 26px;
  padding: 0 4px 0 10px;
  border-radius: var(--radius-sm, 4px);
  color: var(--text-secondary, #64748b);
  cursor: pointer;
  flex-shrink: 0;
  max-width: 180px;
  user-select: none;
  transition: background var(--transition-fast, 0.15s ease), color var(--transition-fast, 0.15s ease);
}

.terminal-tab:hover {
  background: var(--bg-tertiary, #f1f5f9);
  color: var(--text-primary, #0f172a);
}

.terminal-tab.is-active {
  background: var(--primary-bg, #eff6ff);
  color: var(--primary-color);
}

/* 活动 tab 顶部主色指示条（贴 tab 上缘内侧，不溢出容器） */
.terminal-tab.is-active::before {
  content: '';
  position: absolute;
  top: 0;
  left: 10px;
  width: 16px;
  height: 2px;
  border-radius: 0 0 var(--radius-sm, 4px) var(--radius-sm, 4px);
  background: var(--primary-color);
}

.tab-title {
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex-shrink: 0;
}

.tab-dir {
  font-size: 11px;
  color: var(--text-tertiary, #64748b);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}

.terminal-tab.is-active .tab-dir {
  color: var(--primary-color);
  opacity: 0.75;
}

/* 会话退出状态标记：黄色圆点（--warning-color 语义） */
.tab-exit-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--warning-color, #d97706);
  flex-shrink: 0;
}

/* tab 关闭按钮：hover 才出现（对齐主流终端关闭交互，避免常驻 X 视觉噪音） */
.tab-close {
  width: 16px;
  height: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm, 4px);
  color: var(--text-tertiary, #64748b);
  opacity: 0;
  flex-shrink: 0;
  transition: opacity var(--transition-fast, 0.15s ease), background var(--transition-fast, 0.15s ease);
}

.terminal-tab:hover .tab-close,
.terminal-tab.is-active .tab-close {
  opacity: 1;
}

.tab-close:hover {
  background: var(--border-color, #e2e8f0);
  color: var(--text-primary, #0f172a);
}

/* ── 单 tab 徽章模式 ── */
.shell-badge {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 4px;
}

.shell-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--success-color, #16a34a);
  box-shadow: 0 0 6px color-mix(in srgb, var(--success-color) 40%, transparent);
  animation: pulse 2.5s ease-in-out infinite;
}

/* 退出态：状态点转警示色并停止脉冲 */
.shell-dot.is-exited {
  background: var(--warning-color, #d97706);
  box-shadow: none;
  animation: none;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.shell-name {
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary, #64748b);
  white-space: nowrap;
}

/* 路径指示 */
.terminal-path {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 3px 10px;
  background: var(--bg-secondary);
  border-radius: var(--radius-sm, 4px);
  border: 1px solid var(--border-color, #e2e8f0);
  min-width: 0;
}

.path-icon {
  color: var(--primary-color);
  flex-shrink: 0;
}

.path-text {
  font-size: 12px;
  font-family: 'Cascadia Code', 'Fira Code', Consolas, monospace;
  color: var(--text-secondary, #64748b);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  letter-spacing: 0.3px;
}

/* ── 右侧操作区 ── */
.terminal-toolbar-right {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.restart-btn {
  color: var(--warning-color, #d97706) !important;
  font-size: 12px;
}

.restart-btn:hover {
  color: var(--warning-color, #d97706) !important;
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.toolbar-btn {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-tertiary, #64748b);
  cursor: pointer;
  border-radius: var(--radius-sm, 4px);
  transition: all 0.2s ease;
}

.toolbar-btn:hover {
  background: var(--bg-tertiary, #f1f5f9);
  color: var(--text-primary, #0f172a);
}

.toolbar-btn:active {
  transform: scale(0.92);
}

/* 新建 tab 按钮达上限态：禁用视觉 + 提示（title），点击 handler 内拦截 */
.add-tab-btn.is-disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.add-tab-btn.is-disabled:active {
  transform: none;
}

/* 淡入淡出 */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* ── 终端内容区 ── */
.terminal-area {
  flex: 1;
  min-height: 0;
  position: relative;
  display: flex;
  flex-direction: column;
}

.terminal-container {
  flex: 1;
  min-height: 0;
  padding: 2px 0 0;
  /* 与 xterm 主题背景一致（亮色 #ffffff / 暗色 #1d1e1f），避免暗色下亮色残留 */
  background: var(--terminal-bg);
}

.terminal-container :deep(.xterm) {
  height: 100%;
  padding: 0 4px;
}

.terminal-container :deep(.xterm-viewport) {
  background-color: var(--terminal-bg) !important;
  overflow-y: auto !important;
}

/* xterm 滚动条 */
.terminal-container :deep(.xterm-viewport)::-webkit-scrollbar {
  width: 6px;
}

.terminal-container :deep(.xterm-viewport)::-webkit-scrollbar-track {
  background: transparent;
}

.terminal-container :deep(.xterm-viewport)::-webkit-scrollbar-thumb {
  background: var(--border-light, #cbd5e1);
  border-radius: 3px;
}

.terminal-container :deep(.xterm-viewport)::-webkit-scrollbar-thumb:hover {
  background: var(--text-tertiary, #64748b);
}
</style>
