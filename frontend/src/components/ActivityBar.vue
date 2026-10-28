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
        <!-- @show 打开面板时重拉封禁列表、聚合建议与攻击趋势（五期：面板打开/
             可见时加载；九/十期数据源刷新时机对齐） -->
        <el-popover trigger="click" placement="right" :width="320" :persistent="false" @show="reloadBanPanel">
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
                  @click.stop="openBanIPDialog()"
                >封禁 IP</button>
              </div>
              <!-- 聚合建议小节（连接管控九期）：同网段自动封禁条目达阈值时后端产出
                   建议（排序已做——计数降序 + 网段串字典序次级，前端零排序）；行点击
                   预填网段封禁弹窗（闭环止于预填，处置须管理员确认提交）；空建议
                   小节整体不渲染。载荷仅网段串与计数无具体 IP 明细（隐私红线与
                   ListIPBans 同源，浏览器通道 composable 跳过加载恒空态） -->
              <div v-if="suggestions.length" class="devices-suggest-block">
                <div class="devices-suggest-title">聚合建议</div>
                <div class="devices-suggest-list">
                  <button
                    v-for="s in suggestions"
                    :key="s.cidr"
                    type="button"
                    class="devices-suggest-row"
                    :title="`点击预填网段封禁弹窗（${s.cidr}）`"
                    @click.stop="openBanIPDialog(s.cidr)"
                  >
                    <span class="devices-panel-ip">{{ s.cidr }}</span>
                    <span class="devices-suggest-text">该网段 {{ s.count }} 个来源已被封禁，建议网段封禁</span>
                  </button>
                </div>
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
              <!-- 攻击趋势区块（连接管控十期）：审计历史 ip_ban/ip_unban 行天桶聚合
                   的时序视图（跨重启长期面，补九期内存态建议「仅窗口内事实」）。
                   数据源 QueryIPBanTrend（仅桌面通道可达——浏览器通道被
                   RPCHiddenMethods 屏蔽，composable 内 isBrowserChannel 跳过加载，
                   此处恒空态）。每行 = 桶：key 网段串 + autoTotal 窗口自动封禁总数
                   + 天级迷你条形（days 下标 0 = 窗口最旧日、末位 = 今天，高度按桶内
                   max 归一，零值日基线点）+ 处置徽标（manualCount>0——管理员处置
                   分列不混主趋势）+ 再犯徽标（rebounded = 解封后该桶又有 auto 封禁，
                   后端算透传前端零判定）。行点击预填网段封禁弹窗（桶 key 即 CIDR
                   串，复用九期 openBanIPDialog 入口）。排序后端已做（autoTotal 降序
                   + key 字典序次级），前端零排序 -->
              <div class="devices-trend-block">
                <div class="devices-suggest-title">攻击趋势<template v-if="trend.windowDays > 0">（近 {{ trend.windowDays }} 天）</template></div>
                <div v-if="trendBuckets.length" class="devices-trend-list">
                  <button
                    v-for="b in trendBuckets"
                    :key="b.key"
                    type="button"
                    class="devices-trend-row"
                    :title="`点击预填网段封禁弹窗（${b.key}）`"
                    @click.stop="openBanIPDialog(b.key)"
                  >
                    <div class="devices-trend-main">
                      <span class="devices-panel-ip">{{ b.key }}</span>
                      <span class="devices-trend-count">{{ b.autoTotal }} 次自动封禁</span>
                    </div>
                    <div class="devices-trend-meta">
                      <span class="devices-trend-bars" aria-hidden="true">
                        <span
                          v-for="(v, i) in b.days"
                          :key="i"
                          class="devices-trend-bar"
                          :class="{ 'is-empty': !v }"
                          :style="trendBarStyle(v, b.days)"
                        />
                      </span>
                      <span v-if="b.manualCount > 0" class="devices-trend-manual">处置 {{ b.manualCount }}</span>
                      <span v-if="b.rebounded" class="devices-trend-rebounded">再犯</span>
                    </div>
                  </button>
                </div>
                <div v-else class="devices-trend-empty">暂无攻击记录</div>
                <div v-if="trendEarliestDay" class="devices-trend-earliest">数据自 {{ trendEarliestDay }} 起（更早历史已随审计日志轮转）</div>
              </div>
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
    <!-- 手动封禁 IP 弹窗（连接管控七期，「IP 封禁」区块头部入口；九期扩聚合建议
         行预填入口 initialCidr）：@banned 重拉封禁列表、建议与趋势（手动封禁不
         触发 webserve-ip-banned 事件，前端经回调刷新） -->
    <BanIPDialog v-model="banIPDialogVisible" :initial-cidr="banIPCidr" @banned="reloadBanPanel" />
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
import { useIPBanList, useIPBanSuggestions, useIPBanTrend, formatBanExpiry } from '../composables/useIPBans'
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

// 网段聚合封禁建议（连接管控九期，桌面专用数据源）：「聚合建议」小节渲染数据，
// 刷新时机对齐封禁列表（面板打开/封禁动作后 reload；webserve-ip-banned 事件
// 到达由 composable 内自动刷新）
const { suggestions, reload: reloadIPSuggestions } = useIPBanSuggestions()

// 攻击趋势（连接管控十期，桌面专用数据源）：「攻击趋势」区块渲染数据；刷新时机
// 对齐封禁列表与聚合建议（面板打开/封禁动作后 reload；webserve-ip-banned 事件
// 到达由 composable 内自动刷新——新自动封禁改变趋势计数与 rebounded 观测）
const { trend, reload: reloadIPTrend } = useIPBanTrend()

// 三数据源统一刷新出口（面板打开 @show 与封禁动作 @banned 后同调，保证列表、
// 建议与趋势口径同步——新网段封禁会使 CoveredBy 命中条目从建议中剔除，并改变
// 趋势桶的处置计数与再犯观测）
function reloadBanPanel() {
  reloadIPBans()
  reloadIPSuggestions()
  reloadIPTrend()
}

// 后端已排序（autoTotal 降序 + key 字典序次级，对齐九期建议排序纪律），前端零排序
const trendBuckets = computed(() => trend.value.buckets)

// earliestTs 提示行文案：数据自 M-D 起（审计轮转丢历史诚实标注，禁静默降级）。
// 显示条件 = earliestTs 落在窗口起点之后（留存未覆盖整个窗口才提示；earliestTs
// 为 0 = 快照无 ip_ban 行，不提示）。窗口起点计算与后端天桶实现
// （service/ip_ban_trend.go）对齐——今天本地时区 0 点起往回 windowDays-1 天，
// 前端 new Date 本地时区天然同基准，无跨时区换算
const trendEarliestDay = computed(() => {
  const t = trend.value
  if (!t || !t.earliestTs || !t.windowDays || t.windowDays < 1) {
    return ''
  }
  const now = new Date()
  const windowStartSec =
    new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime() / 1000 -
    (t.windowDays - 1) * 86400
  if (t.earliestTs <= windowStartSec) {
    return ''
  }
  const d = new Date(t.earliestTs * 1000)
  return `${d.getMonth() + 1}-${d.getDate()}`
})

/**
 * 天级迷你条形高度（纯 div 数据驱动，免图表库——面板 320 宽容纳不了图表轴）。
 * 非零日高度按桶内最大值归一（下限 12% 保证 1 次与满值档位可辨）；零值日返回
 * null 交由 is-empty 类固定基线高度（动态比例走 :style 数据绑定，静态布局仍走
 * 语义类——视觉规范行内 style 禁令针对静态样式，数据驱动尺寸无类可归并）。
 */
function trendBarStyle(value, days) {
  if (!value) {
    return null
  }
  const max = Math.max(...days, 1)
  const pct = Math.max(Math.round((value / max) * 100), 12)
  return { height: `${pct}%` }
}

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

// 手动封禁 IP 弹窗（连接管控七期，区块头部入口；九期扩聚合建议行入口、十期扩
// 攻击趋势行入口）：成功后 @banned 重拉封禁列表、建议与趋势（手动封禁不触发
// webserve-ip-banned 事件——该出口为自动封禁专属通知面）
const banIPDialogVisible = ref(false)
// 网段预填目标（九期建议行/十期趋势行共用）：行点击带网段串（弹窗档位自动切
// 网段档并预填），头部入口无参为空串 = 默认单 IP 档
const banIPCidr = ref('')

function openBanIPDialog(cidr = '') {
  banIPCidr.value = cidr || ''
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

/* 聚合建议小节（连接管控九期）：小节标题 + 可点击建议行（网段串 + 建议文案）。
 * 标题走 11px/500 弱层文字与父级子标题（devices-panel-subtitle 12px/600）拉开
 * 层级；行形态对齐 devices-ban-list（弱层 hover 提亮 + radius-sm + 四档间距），
 * 整行为按钮承载点击预填，warning 语义色提示待处置（处置面待办的语义色） */
.devices-suggest-block {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.devices-suggest-title {
  font-size: 11px;
  font-weight: 500;
  color: var(--text-tertiary);
}

.devices-suggest-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.devices-suggest-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-sm);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  background: transparent;
  text-align: left;
  cursor: pointer;
  transition: background var(--transition-fast), border-color var(--transition-fast);
}

.devices-suggest-row:hover {
  background: var(--bg-tertiary);
  border-color: var(--warning-color);
}

.devices-suggest-row .devices-panel-ip {
  flex-shrink: 0;
}

/* 建议文案占行内剩余空间可折行（网段串恒完整可见，长文案折行不溢出 320 宽面板） */
.devices-suggest-text {
  flex: 1;
  min-width: 0;
  font-size: 11px;
  line-height: 1.5;
  color: var(--warning-color);
}

/* 攻击趋势区块（连接管控十期）：小节标题复用 devices-suggest-title（11px/500
 * 弱层文字）拉开层级；行形态对齐聚合建议行（可点按钮 + 弱层 hover 提亮 +
 * radius-sm + 四档间距），行内上下两段——上段网段串 + 总计数、下段天级条形 +
 * 徽标；天级条形为纯 div 数据驱动（高度按桶内 max 归一，见 trendBarStyle），
 * warning 语义色（趋势=待观测的攻击面，与建议小节待处置语义同色系） */
.devices-trend-block {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  margin-top: var(--spacing-xs);
}

.devices-trend-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.devices-trend-row {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  background: transparent;
  text-align: left;
  cursor: pointer;
  transition: background var(--transition-fast), border-color var(--transition-fast);
}

.devices-trend-row:hover {
  background: var(--bg-tertiary);
  border-color: var(--warning-color);
}

.devices-trend-main {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  min-width: 0;
}

.devices-trend-main .devices-panel-ip {
  flex-shrink: 0;
}

/* 窗口自动封禁总数：右对齐弱层文字（网段串是主标识，计数次级） */
.devices-trend-count {
  flex: 1;
  min-width: 0;
  font-size: 11px;
  color: var(--text-tertiary);
  text-align: right;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.devices-trend-meta {
  display: flex;
  align-items: flex-end;
  gap: var(--spacing-sm);
}

/* 天级条形组：固定高容器 + 逐日条形（下标 0 = 最旧日在左、末位 = 今天在右），
 * 零值日退化为基线点（is-empty）保持日列对齐 */
.devices-trend-bars {
  flex: 1;
  height: 18px;
  display: flex;
  align-items: flex-end;
  gap: 2px;
  min-width: 0;
}

.devices-trend-bar {
  flex: 1 1 0;
  min-width: 3px;
  background: var(--warning-color);
  border-radius: 1px 1px 0 0;
  opacity: 0.75;
}

.devices-trend-bar.is-empty {
  height: 2px;
  background: var(--border-color);
  opacity: 1;
}

/* 处置徽标（十期）：manualCount>0 渲染「处置 N」，primary 色系对齐封禁列表
 * 「手动」来源徽标（同为管理员处置语义） */
.devices-trend-manual {
  font-size: 10px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: var(--radius-sm);
  color: var(--primary-color);
  background: color-mix(in srgb, var(--primary-color) 14%, transparent);
  white-space: nowrap;
  flex-shrink: 0;
}

/* 再犯徽标（十期）：rebounded 后端算透传（解封后该桶又有 auto 封禁），warning
 * 色系对齐递增档位徽标（攻击持续升级的警示语义） */
.devices-trend-rebounded {
  font-size: 10px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: var(--radius-sm);
  color: var(--warning-color);
  background: color-mix(in srgb, var(--warning-color) 14%, transparent);
  white-space: nowrap;
  flex-shrink: 0;
}

.devices-trend-empty {
  padding: var(--spacing-xs) var(--spacing-sm);
  text-align: center;
  font-size: 11px;
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
}

/* earliestTs 提示行（十期）：留存未覆盖整个窗口时标注「数据自 M-D 起」（轮转
 * 丢历史诚实标注，禁静默降级），弱层文字不抢桶行 */
.devices-trend-earliest {
  font-size: 11px;
  line-height: 1.5;
  color: var(--text-tertiary);
}
</style>
