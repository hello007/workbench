<template>
  <!-- 手动封禁弹窗（连接管控七期单 IP，八期扩网段档，ActivityBar「IP 封禁」区块
       头部入口）：弹窗本身即确认面（对齐 BanTokenDialog 四期先例，不叠加
       ElMessageBox 二次确认）。封禁对象档位（单 IP / 网段 CIDR）+ 三档时长
       （永久 0 / 天级 N 天 / 自定义日期时间到时分）+ 可选原因 + 白名单命中/
       交集即时提示。尺寸走中小弹窗档 min(480px, 92vw) -->
  <el-dialog
    :model-value="modelValue"
    title="封禁来源"
    width="min(480px, 92vw)"
    :close-on-click-modal="false"
    append-to-body
    class="ban-ip-dialog"
    @update:model-value="onVisibleChange"
    @closed="resetForm"
  >
    <div class="ban-ip-form">
      <div class="ban-ip-field">
        <div class="ban-ip-field-label">封禁对象</div>
        <el-radio-group v-model="targetKind" size="small" class="ban-ip-kind-group">
          <el-radio value="ip">单 IP</el-radio>
          <el-radio value="cidr">网段（CIDR）</el-radio>
        </el-radio-group>
      </div>
      <div class="ban-ip-field">
        <div class="ban-ip-field-label">{{ targetKind === 'cidr' ? '目标网段' : '来源 IP' }}</div>
        <el-input
          v-model="targetValue"
          size="small"
          class="ban-ip-input"
          :placeholder="targetKind === 'cidr' ? '如 203.0.113.0/24 或单 IP（自动按 /32 处理）' : '如 192.0.2.10 或 2001:db8::1'"
          :maxlength="targetKind === 'cidr' ? 50 : 45"
          :status="inputError ? 'error' : ''"
        />
        <div v-if="inputError" class="ban-ip-field-error">{{ inputError }}</div>
        <div v-else-if="warnHint" class="ban-ip-field-warn">{{ warnHint }}</div>
      </div>
      <div class="ban-ip-field">
        <div class="ban-ip-field-label">封禁时长</div>
        <div class="ban-ip-duration-row">
          <el-select v-model="banExpiry" class="ban-ip-expiry-select" size="small">
            <el-option label="永久（须手动解封）" value="permanent" />
            <el-option label="1 天" value="1" />
            <el-option label="7 天" value="7" />
            <el-option label="30 天" value="30" />
            <el-option label="自定义时间" value="custom" />
          </el-select>
          <el-date-picker
            v-if="banExpiry === 'custom'"
            v-model="customBanDate"
            type="datetime"
            size="small"
            class="ban-ip-expiry-date"
            placeholder="解封时刻"
            :disabled-date="disablePastDate"
          />
        </div>
      </div>
      <div class="ban-ip-field">
        <div class="ban-ip-field-label">封禁原因（可选，仅本机留痕）</div>
        <el-input
          v-model="reason"
          size="small"
          placeholder="如：持续暴力破解，已核实"
          maxlength="100"
        />
      </div>
      <div class="ban-ip-warning">{{ warningText }}</div>
    </div>
    <template #footer>
      <el-button size="small" @click="close">取消</el-button>
      <el-button
        size="small"
        type="danger"
        :loading="submitting"
        :disabled="submitDisabled"
        class="ban-ip-confirm-btn"
        @click="submit"
      >封禁</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { BanIP, BanCIDR, GetSettings } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import { ipToBytes, isIPWhitelisted, parseCidrInput, cidrOverlapWhitelist } from '../utils/ipWhitelist'

const props = defineProps({
  // 弹窗显隐（v-model）
  modelValue: { type: Boolean, default: false },
  // 预填目标 IP（明细行入口可带；头部入口为空串）
  initialIP: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue', 'banned'])

// 表单态：封禁对象档位（'ip' 单 IP / 'cidr' 网段，连接管控八期扩档）+ 两档
// 输入值 / 时长档位（默认永久——手动封禁核心场景为管理员主动处置攻击源）/
// 自定义解封时刻 / 原因备注 / 当前白名单快照（打开时拉取，即时校验提示面）
const targetKind = ref('ip')
const ip = ref('')
const cidr = ref('')
const banExpiry = ref('permanent')
const customBanDate = ref(null)
const reason = ref('')
const whitelist = ref([])
const submitting = ref(false)

// 单 IP 档输入校验（格式必填；空白名单命中为引导性 warning——后端 Ban 权威兜底）
const ipError = computed(() => {
  if (!ip.value.trim()) return '请填写要封禁的来源 IP'
  if (!ipToBytes(ip.value)) return 'IP 格式无效'
  return ''
})

const whitelistHit = computed(() => {
  if (!ip.value.trim() || ipError.value) return ''
  if (isIPWhitelisted(ip.value, whitelist.value)) {
    return '该 IP 在可信网段白名单内（含本机回环地址），封禁将被拒绝；如确需封禁请先在设置中移除对应白名单条目'
  }
  return ''
})

// 网段档输入校验（连接管控八期）：CIDR 串或裸 IP 均合法（裸 IP 视为 /32 //128
// 等价形态，与后端 parseIPBanWhitelistEntry 同口径）；解析复用 ipWhitelist 单点
const cidrError = computed(() => {
  if (!cidr.value.trim()) return '请填写要封禁的网段'
  if (!parseCidrInput(cidr.value)) return '网段格式无效：须为 CIDR（如 203.0.113.0/24、2001:db8::/32）或单 IP'
  return ''
})

// 网段档白名单预检（后端 BanCIDR 权威兜底，前端仅提示）：
//  - 完全覆盖（被单条白名单条目完全包含）→ 后端必拒绝（E_IP_BAN_CIDR_WHITELISTED），
//    阻断提交并引导先移除白名单条目（对齐单 IP 档 whitelistHit 语义）；
//  - 部分交集（非阻断警示行）→ 交集内来源不受此封禁影响，封禁本身合法。
const cidrFullyCovered = computed(() => {
  if (targetKind.value !== 'cidr' || cidrError.value) return false
  return cidrOverlapWhitelist(cidr.value, whitelist.value).fullyCovered
})

const cidrWhitelistWarn = computed(() => {
  if (targetKind.value !== 'cidr' || cidrError.value || cidrFullyCovered.value) return ''
  const { overlapCount } = cidrOverlapWhitelist(cidr.value, whitelist.value)
  if (overlapCount > 0) {
    return `该网段与可信网段白名单有交集（${overlapCount} 条），交集内来源不受此封禁影响`
  }
  return ''
})

// 输入框按档位读写两套输入值（v-model 须为成员表达式，三元不可用，走可写
// computed 代理分流；切档保留对方输入，关闭弹窗统一 resetForm 清空）
const targetValue = computed({
  get: () => (targetKind.value === 'cidr' ? cidr.value : ip.value),
  set: (v) => {
    if (targetKind.value === 'cidr') {
      cidr.value = v
    } else {
      ip.value = v
    }
  }
})

// 档位分流聚合（模板行内错误/警示/提交禁用单一出口）。网段档完全覆盖态警示
// 由 warnHint 单一出口渲染（阻断语义同单 IP 档 whitelistHit：提示 + 禁提交）
const inputError = computed(() => (targetKind.value === 'ip' ? ipError.value : cidrError.value))
const warnHint = computed(() => {
  if (targetKind.value === 'ip') return whitelistHit.value
  if (cidrFullyCovered.value) {
    return '该网段已被可信网段白名单完全包含，封禁将被拒绝；如确需封禁请先在设置中移除对应白名单条目'
  }
  return cidrWhitelistWarn.value
})
const submitDisabled = computed(() => {
  if (targetKind.value === 'ip') return !!ipError.value || !!whitelistHit.value
  return !!cidrError.value || cidrFullyCovered.value
})

// 封禁语义警示文案（弹窗即确认面，文案承担确认语义）：
//  - 单 IP 档：七期文案不变；
//  - 网段档：影响面从单 IP 扩至全网段来源；
//  - 全网段（前缀 0：0.0.0.0/0 或 ::/0）：确认文案升级明示「除白名单与回环外
//    全部来源被拒」（后端允许该应急锁——回环恒豁免 + 白名单恒优先，本机不死）。
const wholeInternetCidr = computed(() => {
  if (targetKind.value !== 'cidr' || cidrError.value) return false
  return parseCidrInput(cidr.value)?.prefixLen === 0
})

const warningText = computed(() => {
  if (targetKind.value === 'ip') {
    return '封禁后该来源 IP 的新连接将被拒绝（网页访问与实时通道），已建立连接不受影响；若其在可信网段白名单内，封禁将被拒绝。解封后新连接立即可达；手动封禁会覆盖同 IP 的自动封禁（递增档位保留）。'
  }
  if (wholeInternetCidr.value) {
    return '该网段覆盖全部来源：封禁后将拒绝除可信网段白名单与本机回环外的全部来源（含正常用户），解封前外部访问不可达，请确认为应急所需。已建立连接不受影响；白名单内来源不受此封禁影响。'
  }
  return '封禁后该网段内全部来源 IP 的新连接将被拒绝（网页访问与实时通道），已建立连接不受影响；白名单内来源不受此封禁影响，本机回环恒豁免。解封后该网段新连接立即可达。'
})

// 弹窗打开时拉白名单快照 + 预填目标 IP（即时校验的数据源；拉取失败降级空列表
// ——提示面缺失仅损失前端预警，后端 E_IP_BAN_WHITELISTED 权威兜底）
watch(
  () => props.modelValue,
  (visible) => {
    if (!visible) return
    ip.value = props.initialIP || ''
    GetSettings()
      .then((settings) => {
        const list = settings?.ipban?.whitelist
        whitelist.value = Array.isArray(list) ? list : []
      })
      .catch(() => {
        whitelist.value = []
      })
  }
)

/** 自定义解封时刻禁选过去（与后端 E_TOKEN_EXPIRY_INVALID 校验对齐的前端引导）。 */
function disablePastDate(date) {
  return date.getTime() < Date.now() - 24 * 3600 * 1000
}

/**
 * 由时长档位换算后端 expiresAt（unix 秒；0 = 永久封禁）：天级档自当前时刻起算；
 * 自定义档取所选时刻（datetime 精确到分钟，语义即「封至此刻」）；未选时刻返回
 * null（提交前置拦截）。
 */
function computeBanExpiryUnix() {
  if (banExpiry.value === 'permanent') return 0
  if (banExpiry.value === '1' || banExpiry.value === '7' || banExpiry.value === '30') {
    return Math.floor(Date.now() / 1000) + Number(banExpiry.value) * 86400
  }
  if (banExpiry.value === 'custom' && customBanDate.value) {
    return Math.floor(new Date(customBanDate.value).getTime() / 1000)
  }
  return null
}

/**
 * 提交封禁：按档位分流 BanIP(ip, ...) / BanCIDR(cidr, ...)（连接管控八期），
 * 成功后关闭弹窗并通知父级刷新列表（@banned → ActivityBar reloadIPBans）。
 */
async function submit() {
  if (submitting.value) return
  if (inputError.value) {
    ElMessage.warning(inputError.value)
    return
  }
  const expiresAt = computeBanExpiryUnix()
  if (expiresAt === null) {
    ElMessage.warning('请选择解封时刻')
    return
  }
  submitting.value = true
  try {
    if (targetKind.value === 'cidr') {
      await BanCIDR(cidr.value.trim(), expiresAt, reason.value.trim())
      ElMessage.success(`已封禁网段 ${cidr.value.trim()}`)
    } else {
      await BanIP(ip.value.trim(), expiresAt, reason.value.trim())
      ElMessage.success(`已封禁 ${ip.value.trim()}`)
    }
    close()
    emit('banned')
  } catch (e) {
    handleError('封禁失败: ', e)
  } finally {
    submitting.value = false
  }
}

function onVisibleChange(v) {
  emit('update:modelValue', v)
}

function close() {
  emit('update:modelValue', false)
}

/** 关闭后重置表单（el-dialog @closed：渐隐动画结束后执行，避免关闭过程中表单值闪变）。 */
function resetForm() {
  targetKind.value = 'ip'
  ip.value = ''
  cidr.value = ''
  banExpiry.value = 'permanent'
  customBanDate.value = null
  reason.value = ''
}
</script>

<style scoped>
.ban-ip-form {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.ban-ip-field {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.ban-ip-field-label {
  font-size: 12px;
  color: var(--text-secondary);
}

.ban-ip-duration-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.ban-ip-expiry-select {
  width: 180px;
}

.ban-ip-expiry-date {
  width: 200px;
}

.ban-ip-field-error {
  font-size: 11px;
  color: var(--danger-color);
}

.ban-ip-field-warn {
  font-size: 11px;
  line-height: 1.5;
  color: var(--warning-color);
}

/* 封禁语义警示：warning 语义色 + color-mix 派生（禁硬编码 rgba，随主题切换） */
.ban-ip-warning {
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: 12px;
  line-height: 1.6;
  color: var(--warning-color);
  background: color-mix(in srgb, var(--warning-color) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--warning-color) 35%, transparent);
  border-radius: var(--radius-md);
}
</style>
