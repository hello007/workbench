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
    <el-tooltip v-if="panelVisible" :content="deviceHint" placement="right" :show-after="300">
      <div class="devices-indicator-anchor">
        <!-- @show 打开面板时重拉自动封禁列表（五期：面板打开/可见时加载） -->
        <el-popover trigger="click" placement="right" :width="320" :persistent="false" @show="reloadIPBans">
          <template #reference>
            <div class="activity-bar-item activity-bar-devices">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none">
                <rect x="2" y="4" width="13" height="10" rx="1.5" stroke="currentColor" stroke-width="1.6"/>
                <rect x="9" y="10" width="13" height="10" rx="1.5" stroke="currentColor" stroke-width="1.6" fill="var(--sidebar-bg)"/>
                <rect x="9" y="10" width="13" height="10" rx="1.5" stroke="currentColor" stroke-width="1.6"/>
              </svg>
              <span class="devices-badge" :class="{ 'devices-badge-warn': !multiDevice }">{{ badgeText }}</span>
            </div>
          </template>
          <div class="devices-panel">
            <div class="devices-panel-title">已连接设备（{{ deviceCount }}）</div>
            <div v-if="connections.length" class="devices-panel-list">
              <div v-for="c in connections" :key="c.id" class="devices-panel-row">
                <span class="devices-panel-ip" :title="c.ip">{{ c.ip }}</span>
                <span class="devices-panel-token">{{ c.tokenName || '已撤销令牌' }}</span>
                <span class="devices-panel-time">{{ formatConnTime(c.connectedAt) }}</span>
                <!-- 踢出（断开该连接，非封禁）与封禁（tokenID 维度临时拒连，
                     连接管控四期）：popover 内容区直挂按钮，不在 tooltip/popover
                     reference 链上，无嵌套 forwardRef 约束问题 -->
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
                <button
                  class="devices-panel-ban"
                  type="button"
                  title="封禁该令牌"
                  @click.stop="openBanDialog(c)"
                >
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                    <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-width="2"/>
                    <path d="M5.8 5.8 18.2 18.2" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                  </svg>
                </button>
              </div>
            </div>
            <div v-else class="devices-panel-empty">
              {{ isDesktopChannel ? '无设备连接' : '明细仅在桌面端显示' }}
            </div>
            <div class="devices-panel-hint">多端共享同一后端状态，后操作覆盖先操作</div>
            <!-- IP 封禁区块（连接管控五期自动封禁 + 七期手动封禁 + 八期网段封禁）：
                 数据源 ListIPBans（仅桌面通道可达——浏览器通道被 RPCHiddenMethods
                 屏蔽，composable 内 isBrowserChannel 跳过加载，此处恒空态）；行 =
                 IP/网段串 + 类型徽标（kind=cidr 显「网段」）+ 覆盖提示徽标
                 （coveredBy 非空，tooltip 展示覆盖方网段——后端算透传，前端禁复刻
                 Contains 判定）+ 来源徽标（手动=「手动」/ 自动递增=「第 N 次」）+
                 窗口失败计数 + 到期时间（永久封禁「永久」/ 过期态「已过期」info /
                 生效中 warning）。档位徽标（六期）：strike>1 才渲染——首触无递增
                 语义，渲染「第 1 次」纯噪音；strike undefined（旧载荷）falsy 不
                 渲染向后兼容。解封按钮过期行同样保留：UnbanIP/UnbanCIDR 幂等
                 no-op 安全，且可顺带清理列表中已过期残留记录。头部「封禁 IP」
                 按钮（七期手动封禁入口）仅桌面通道渲染 -->
            <div class="devices-ban-section">
              <div class="devices-ban-header">
                <div class="devices-panel-subtitle">IP 封禁（{{ ipBans.length }}）</div>
                <button
                  v-if="isDesktopChannel"
                  class="devices-ban-add"
                  type="button"
                  title="手动封禁来源 IP"
                  @click.stop="openBanIPDialog"
                >封禁 IP</button>
              </div>
              <div v-if="ipBans.length" class="devices-ban-list">
                <div v-for="b in ipBans" :key="b.ip" class="devices-ban-row">
                  <div class="devices-ban-cell-ip">
                    <span class="devices-panel-ip" :title="b.ip">{{ b.ip }}</span>
                    <span v-if="b.kind === 'cidr'" class="devices-ban-kind" title="网段封禁：该网段内全部来源 IP 命中">网段</span>
                    <el-tooltip
                      v-if="b.coveredBy"
                      :content="`已被网段 ${b.coveredBy} 覆盖：该条目与更宽网段封禁重叠，网段封禁不因本条解封而失效`"
                      placement="top"
                      :show-after="200"
                    >
                      <span class="devices-ban-covered">被覆盖</span>
                    </el-tooltip>
                  </div>
                  <span v-if="b.source === 'manual'" class="devices-ban-source" title="管理员手动下发">手动</span>
                  <span v-else-if="b.strike > 1" class="devices-ban-strike" :title="`第 ${b.strike} 次触发，封禁时长递增`">第 {{ b.strike }} 次</span>
                  <span v-if="b.source !== 'manual'" class="devices-ban-fails">{{ b.failCount }} 次失败</span>
                  <span class="devices-ban-expiry" :class="{ 'is-expired': b.expired }">
                    {{ b.expired ? '已过期' : b.expiresAt === 0 ? '永久' : `至 ${formatBanExpiry(b.expiresAt)}` }}
                  </span>
                  <button
                    class="devices-ban-unban"
                    type="button"
                    :title="b.kind === 'cidr' ? '解封该网段' : '解封该 IP'"
                    @click.stop="unbanIP(b)"
                  >解封</button>
                </div>
              </div>
              <div v-else class="devices-ban-empty">暂无封禁记录</div>
            </div>
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
    <!-- 封禁令牌弹窗（连接管控四期，明细行封禁入口，与设置页令牌管理共用组件）：
         渲染于组件根级（popover persistent=false 不承载弹窗），确认提交与联动
         踢出在弹窗内完成 -->
    <BanTokenDialog v-model="banDialogVisible" :token="banTarget" :connections="connections" />
    <!-- 手动封禁 IP 弹窗（连接管控七期，「IP 封禁」区块头部入口）：@banned 重拉
         封禁列表（手动封禁不触发 webserve-ip-banned 事件，前端经回调刷新） -->
    <BanIPDialog v-model="banIPDialogVisible" @banned="reloadIPBans" />
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { Folder, MagicStick, Setting, Monitor, TrendCharts, DataBoard, ChatDotRound } from '@element-plus/icons-vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import ToolboxIcon from './icons/ToolboxIcon.vue'
import AssetsIcon from './icons/AssetsIcon.vue'
import { KickConnection, UnbanIP, UnbanCIDR } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import { useUiStore } from '../store'
import { useDeviceConnections } from '../composables/useDeviceConnections'
import { useIPBanList, formatBanExpiry } from '../composables/useIPBans'
import BanTokenDialog from './BanTokenDialog.vue'
import BanIPDialog from './BanIPDialog.vue'

const uiStore = useUiStore()

defineEmits(['toggleTerminal', 'openSettings'])

// 多设备连接感知（PR3 语义明示 + 明细面板 D2）：指示项显示条件沿用 composable
// 分流口径（桌面 len>=1 / 浏览器 count>1），明细列表与通道归属供 popover 消费
const { deviceCount, multiDevice, connections, isDesktopChannel } = useDeviceConnections()

// 自动封禁列表（连接管控五期，桌面专用数据源）：面板「自动封禁」区块渲染与
// 解封后/面板打开时重拉；webserve-ip-banned 事件到达由 composable 内自动刷新
const { ipBans, reload: reloadIPBans } = useIPBanList()

// 面板可见条件扩展（五期）：有自动封禁记录时即使无在途连接也显示入口——被封禁
// IP 的新连接在认证点即被拒（不会产生连接明细），仅凭连接数判定面板将不可达，
// 管理员将无从解封
const panelVisible = computed(() => multiDevice.value || ipBans.value.length > 0)
// 角标内容分流：有设备连接显示连接数；无连接仅封禁记录时显示封禁数（warning
// 色区分，见 devices-badge-warn 样式）
const badgeText = computed(() =>
  multiDevice.value ? String(deviceCount.value) : String(ipBans.value.length)
)
const deviceHint = computed(() =>
  multiDevice.value
    ? `${deviceCount.value} 个设备已连接：点击查看明细`
    : `${ipBans.value.length} 个来源 IP 已被自动封禁：点击查看`
)

/**
 * 解封来源 IP 的封禁条目（连接管控五期自动封禁 + 八期网段封禁分流）：单次确认链
 * （confirm-dialog 单次确认形态）→ kind=cidr 走 UnbanCIDR（精确匹配单网段条目），
 * 否则 UnbanIP → 成功 toast + 重拉列表。过期行解封同样放行——两者均幂等 no-op
 * 安全，且可顺带清理列表中的已过期残留记录。确认文案带目标与影响面：网段档明示
 * 「该网段内全部来源 IP 恢复可达」，单 IP 档保留「再犯会再次被自动封禁」警示。
 */
async function unbanIP(b) {
  const isCidr = b.kind === 'cidr'
  const message = isCidr
    ? `解封后该网段（${b.ip}）内全部来源 IP 的新连接立即可达；网段内单 IP 的自动封禁记录不受影响，再次达到阈值仍会触发自动封禁。是否解封？`
    : `解封后来源 ${b.ip} 的新连接立即可达；若其继续高频认证失败，达到阈值会再次被自动封禁。是否解封？`
  try {
    await ElMessageBox.confirm(
      message,
      isCidr ? '解封网段' : '解封 IP',
      { type: 'warning', confirmButtonText: '解封', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  try {
    if (isCidr) {
      await UnbanCIDR(b.ip)
    } else {
      await UnbanIP(b.ip)
    }
    ElMessage.success(`已解封 ${b.ip}`)
    reloadIPBans()
  } catch (e) {
    handleError('解封失败: ', e)
  }
}

// 封禁弹窗（连接管控四期，明细行封禁入口）：目标取行 tokenId（封禁为 tokenID
// 维度，非单连接）与令牌名（空串降级由弹窗按 tokenId 展示）
const banDialogVisible = ref(false)
const banTarget = ref(null)

/**
 * 打开封禁弹窗（按行 tokenId 封禁该令牌全部连接对应设备；弹窗内含「同时断开
 * 在途连接」勾选，勾选后按 tokenID 过滤明细逐个踢出）。
 */
function openBanDialog(c) {
  banTarget.value = { id: c.tokenId, name: c.tokenName || '' }
  banDialogVisible.value = true
}

// 手动封禁 IP 弹窗（连接管控七期，区块头部入口）：成功后 @banned 重拉封禁列表
//（手动封禁不触发 webserve-ip-banned 事件——该出口为自动封禁专属通知面）
const banIPDialogVisible = ref(false)

function openBanIPDialog() {
  banIPDialogVisible.value = true
}

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
  grid-template-columns: 120px minmax(0, 1fr) auto auto auto;
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

/* 封禁按钮：warning 语义色小图标钮（与踢出 danger 区分——封禁为 tokenID 维度
 * 时限性拒连，踢出为单连接断开），形态与踢出钮一致 */
.devices-panel-ban {
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

.devices-panel-ban:hover {
  background: color-mix(in srgb, var(--warning-color) 14%, transparent);
  color: var(--warning-color);
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

/* 无连接仅封禁记录时的角标（warning 语义色，与连接数 primary 区分） */
.devices-badge-warn {
  color: var(--warning-color);
  border-color: var(--warning-color);
}

/* 自动封禁区块（连接管控五期）：子标题 + 行列表 + 空态；行网格与明细行同构
 * （IP 弹性列 + 计数/到期定宽列 + 解封钮），过期态 info 灰 / 生效中 warning */
.devices-ban-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  padding-top: var(--spacing-sm);
  border-top: 1px solid var(--border-color);
}

.devices-panel-subtitle {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.devices-ban-list {
  max-height: 160px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.devices-ban-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto auto;
  gap: var(--spacing-sm);
  align-items: center;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-sm);
}

.devices-ban-row:hover {
  background: var(--bg-tertiary);
}

.devices-ban-fails {
  font-size: 11px;
  color: var(--text-tertiary);
  white-space: nowrap;
}

/* 递增档位徽标（六期）：warning 色系小徽标，strike>1 才渲染（首触无递增语义） */
.devices-ban-strike {
  font-size: 10px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: var(--radius-sm);
  color: var(--warning-color);
  background: color-mix(in srgb, var(--warning-color) 14%, transparent);
  white-space: nowrap;
}

/* 来源徽标（七期）：手动封禁 primary 色系，与自动递增 warning 徽标形态一致 */
.devices-ban-source {
  font-size: 10px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: var(--radius-sm);
  color: var(--primary-color);
  background: color-mix(in srgb, var(--primary-color) 14%, transparent);
  white-space: nowrap;
}

/* IP 单元格（八期）：IP/网段串与类型/覆盖徽标同行收纳为第一列整体，避免逐个
 * 独立占网格列撑破 4 列布局折行；min-width:0 让 ellipsis 串在 flex 内可收缩 */
.devices-ban-cell-ip {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  min-width: 0;
}

.devices-ban-cell-ip .devices-panel-ip {
  min-width: 0;
}

/* 类型徽标（八期）：kind=cidr 网段条目标识，info 色系与来源徽标（primary）、
 * 递增档位徽标（warning）三色区分 */
.devices-ban-kind {
  font-size: 10px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: var(--radius-sm);
  color: var(--info-color);
  background: color-mix(in srgb, var(--info-color) 14%, transparent);
  white-space: nowrap;
  flex-shrink: 0;
}

/* 覆盖提示徽标（八期）：coveredBy 非空（后端算透传，前端禁复刻 Contains），
 * 中性灰弱提示，详情经 tooltip 展示覆盖方网段 */
.devices-ban-covered {
  font-size: 10px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: var(--radius-sm);
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
  white-space: nowrap;
  flex-shrink: 0;
}

/* 区块头部：标题 + 「封禁 IP」入口钮（七期手动封禁，弱化为 text 级小钮不抢
 * 明细信息，hover warning 提亮与封禁语义一致） */
.devices-ban-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-xs);
}

.devices-ban-add {
  padding: 1px 6px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  font-size: 11px;
  color: var(--text-tertiary);
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.devices-ban-add:hover {
  background: color-mix(in srgb, var(--warning-color) 14%, transparent);
  color: var(--warning-color);
}

.devices-ban-expiry {
  font-size: 11px;
  color: var(--warning-color);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.devices-ban-expiry.is-expired {
  color: var(--text-tertiary);
}

.devices-ban-unban {
  padding: 1px 6px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  font-size: 11px;
  color: var(--text-tertiary);
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.devices-ban-unban:hover {
  background: color-mix(in srgb, var(--warning-color) 14%, transparent);
  color: var(--warning-color);
}

.devices-ban-empty {
  padding: var(--spacing-xs) var(--spacing-sm);
  text-align: center;
  font-size: 11px;
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
}
</style>
