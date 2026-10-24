<template>
  <!-- 封禁令牌弹窗（连接管控四期，SettingsPanel 令牌管理条目与 ActivityBar 连接
       明细行共用）：弹窗本身即确认面（不再叠加 ElMessageBox 二次确认）。尺寸走
       中小弹窗档 min(480px, 92vw)；警示文案明示封禁语义（新连接被拒 / 已建立
       连接不受影响 / 可勾选同时断开），对齐踢出确认「两手段互补」文案先例 -->
  <el-dialog
    :model-value="modelValue"
    :title="dialogTitle"
    width="min(480px, 92vw)"
    :close-on-click-modal="false"
    append-to-body
    class="ban-token-dialog"
    @update:model-value="onVisibleChange"
    @closed="resetForm"
  >
    <div class="ban-token-form">
      <div class="ban-token-target">
        目标令牌：<span class="ban-token-target-name">{{ tokenLabel }}</span>
      </div>
      <div class="ban-token-field">
        <div class="ban-token-field-label">封禁时长</div>
        <div class="ban-token-duration-row">
          <el-select v-model="banExpiry" class="ban-expiry-select" size="small">
            <el-option label="永久（须手动解封）" value="permanent" />
            <el-option label="1 天" value="1" />
            <el-option label="7 天" value="7" />
            <el-option label="30 天" value="30" />
            <el-option label="自定义日期" value="custom" />
          </el-select>
          <el-date-picker
            v-if="banExpiry === 'custom'"
            v-model="customBanDate"
            type="date"
            size="small"
            class="ban-expiry-date"
            placeholder="解封日期"
            :disabled-date="disablePastDate"
          />
        </div>
      </div>
      <div class="ban-token-field">
        <div class="ban-token-field-label">封禁原因（可选，仅本机留痕）</div>
        <el-input
          v-model="reason"
          size="small"
          class="ban-reason-input"
          placeholder="如：疑似泄露，待核实"
          maxlength="100"
        />
      </div>
      <el-checkbox v-model="kickNow" class="ban-kick-check">同时断开该令牌的在途连接</el-checkbox>
      <div class="ban-token-warning">
        封禁后该令牌的新连接将被拒绝（网页访问与实时通道），已建立连接不受影响；勾选上方选项可在封禁同时断开在途连接。解封后新连接立即可达。
      </div>
    </div>
    <template #footer>
      <el-button size="small" @click="close">取消</el-button>
      <el-button size="small" type="danger" :loading="submitting" class="ban-confirm-btn" @click="submit">封禁</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { BanWebToken } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import { kickTokenConnections } from '../utils/tokenBan'

const props = defineProps({
  // 弹窗显隐（v-model）
  modelValue: { type: Boolean, default: false },
  // 封禁目标 { id, name }（name 用于文案；空串降级显示令牌 ID）
  token: { type: Object, default: null },
  // 连接明细快照（联动踢出按 tokenID 过滤；桌面通道专用，浏览器通道恒空）
  connections: { type: Array, default: () => [] }
})

const emit = defineEmits(['update:modelValue', 'banned'])

// 表单态：封禁时长档位（默认永久——封禁核心场景为管理员不在线时持续拒连）/
// 自定义解封日期 / 原因备注 / 联动踢出（默认勾选，D1 决策）
const banExpiry = ref('permanent')
const customBanDate = ref(null)
const reason = ref('')
const kickNow = ref(true)
const submitting = ref(false)

const dialogTitle = computed(() => (props.token?.name ? `封禁令牌「${props.token.name}」` : '封禁令牌'))
const tokenLabel = computed(() => props.token?.name || props.token?.id || '—')

/**
 * 自定义解封日期禁选过去（与后端 E_TOKEN_EXPIRY_INVALID 校验对齐的前端引导）。
 */
function disablePastDate(date) {
  return date.getTime() < Date.now() - 24 * 3600 * 1000
}

/**
 * 由时长档位换算后端 expiresAt（unix 秒；0 = 永久封禁）：天级档自当前时刻起算；
 * 自定义档取所选日期的当日末尾（23:59:59），保证「选择当天」语义为当天全天封禁。
 * 自定义档未选日期返回 null（提交前置拦截）。
 */
function computeBanExpiryUnix() {
  if (banExpiry.value === 'permanent') return 0
  if (banExpiry.value === '1' || banExpiry.value === '7' || banExpiry.value === '30') {
    return Math.floor(Date.now() / 1000) + Number(banExpiry.value) * 86400
  }
  if (banExpiry.value === 'custom' && customBanDate.value) {
    const d = new Date(customBanDate.value)
    d.setHours(23, 59, 59, 0)
    return Math.floor(d.getTime() / 1000)
  }
  return null
}

/**
 * 提交封禁：BanWebToken(id, expiresAt, reason) 成功后关闭弹窗并通知父级刷新；
 * 勾选联动踢出时按 tokenID 过滤在途连接逐个断开（fail-soft，失败统一 warning，
 * 不影响封禁成功态与弹窗关闭）。
 */
async function submit() {
  if (!props.token || submitting.value) return
  const expiresAt = computeBanExpiryUnix()
  if (expiresAt === null) {
    ElMessage.warning('请选择解封日期')
    return
  }
  submitting.value = true
  try {
    await BanWebToken(props.token.id, expiresAt, reason.value.trim())
    ElMessage.success(props.token.name ? `令牌「${props.token.name}」已封禁` : '令牌已封禁')
    const needKick = kickNow.value
    close()
    emit('banned')
    if (needKick) {
      await kickTokenConnections(props.connections, props.token.id)
    }
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

/**
 * 关闭后重置表单（el-dialog @closed：渐隐动画结束后执行，避免关闭过程中
 * 表单值闪变）。
 */
function resetForm() {
  banExpiry.value = 'permanent'
  customBanDate.value = null
  reason.value = ''
  kickNow.value = true
}
</script>

<style scoped>
.ban-token-form {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.ban-token-target {
  font-size: 13px;
  color: var(--text-secondary);
}

.ban-token-target-name {
  font-weight: 600;
  color: var(--text-primary);
}

.ban-token-field {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.ban-token-field-label {
  font-size: 12px;
  color: var(--text-secondary);
}

.ban-token-duration-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.ban-expiry-select {
  width: 180px;
}

.ban-expiry-date {
  width: 140px;
}

.ban-kick-check {
  align-self: flex-start;
}

/* 封禁语义警示：warning 语义色 + color-mix 派生（禁硬编码 rgba，随主题切换） */
.ban-token-warning {
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: 12px;
  line-height: 1.6;
  color: var(--warning-color);
  background: color-mix(in srgb, var(--warning-color) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--warning-color) 35%, transparent);
  border-radius: var(--radius-md);
}
</style>
