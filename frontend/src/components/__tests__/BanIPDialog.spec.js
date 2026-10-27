import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import BanIPDialog from '../BanIPDialog.vue'

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }
}))

vi.mock('../../../wailsjs/go/main/App', () => ({
  BanIP: vi.fn(),
  GetSettings: vi.fn()
}))

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="el-dialog"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width', 'closeOnClickModal', 'class', 'appendToBody'],
    emits: ['update:modelValue']
  },
  'el-select': {
    template: '<select :value="modelValue" @change="onChange"><slot /></select>',
    props: ['modelValue', 'size'],
    emits: ['update:modelValue', 'change'],
    methods: {
      onChange(e) {
        this.$emit('update:modelValue', e.target.value)
        this.$emit('change', e.target.value)
      }
    }
  },
  'el-option': { template: '<option :value="value">{{ label }}</option>', props: ['label', 'value'] },
  'el-input': {
    template: '<input :value="modelValue" :placeholder="placeholder" :maxlength="maxlength" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'size', 'maxlength', 'status'],
    emits: ['update:modelValue']
  },
  'el-date-picker': {
    template: '<input class="el-date-picker" :value="modelValue || \'\'" :placeholder="placeholder" @change="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'type', 'placeholder', 'disabledDate', 'size'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="loading || disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'size', 'loading', 'text', 'disabled'],
    emits: ['click']
  }
}

/**
 * 挂载（modelValue 初始 false，setProps 置 true 触发打开链路——白名单快照拉取
 * 挂在 watch(modelValue) 非 immediate，与真实 v-model 打开时序一致）。
 */
async function createWrapper(over = {}, settings = {}) {
  const { GetSettings } = await import('../../../wailsjs/go/main/App')
  GetSettings.mockResolvedValue(settings)
  const wrapper = mount(BanIPDialog, {
    props: { modelValue: false, ...over },
    global: { stubs }
  })
  await wrapper.setProps({ modelValue: true })
  await flushPromises()
  return wrapper
}

describe('BanIPDialog.vue（手动封禁 IP 弹窗，连接管控七期）', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  it('默认表单态：永久档选中、渲染封禁语义警示与三档时长选项', async () => {
    wrapper = await createWrapper()
    expect(wrapper.find('select.ban-ip-expiry-select').element.value).toBe('permanent')
    expect(wrapper.text()).toContain('新连接将被拒绝')
    expect(wrapper.text()).toContain('已建立连接不受影响')
    // 警示文案含白名单互斥与自动封禁覆盖语义
    expect(wrapper.text()).toContain('可信网段白名单')
    expect(wrapper.text()).toContain('覆盖同 IP 的自动封禁')
  })

  it('打开时拉取白名单快照（GetSettings → ipban.whitelist）', async () => {
    wrapper = await createWrapper({}, { ipban: { whitelist: ['10.0.0.0/8'] } })
    const { GetSettings } = await import('../../../wailsjs/go/main/App')
    expect(GetSettings).toHaveBeenCalled()
  })

  it('合法 IP 永久档提交：BanIP(ip, 0, reason) 且 reason 透传，成功后关弹窗并 emit banned', async () => {
    const { BanIP } = await import('../../../wailsjs/go/main/App')
    BanIP.mockResolvedValue()
    wrapper = await createWrapper()
    await wrapper.find('input.ban-ip-input').setValue('192.0.2.10')
    await wrapper.find('input[placeholder^="如：持续"]').setValue('持续暴力破解')
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    expect(BanIP).toHaveBeenCalledTimes(1)
    expect(BanIP).toHaveBeenCalledWith('192.0.2.10', 0, '持续暴力破解')
    expect(wrapper.emitted('update:modelValue')).toContainEqual([false])
    expect(wrapper.emitted('banned')).toBeTruthy()
  })

  it('天级档提交：expiresAt 换算为当前时间 + N 天（unix 秒）', async () => {
    const { BanIP } = await import('../../../wailsjs/go/main/App')
    BanIP.mockResolvedValue()
    wrapper = await createWrapper()
    await wrapper.find('input.ban-ip-input').setValue('192.0.2.10')
    await wrapper.find('select.ban-ip-expiry-select').setValue('7')
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    const [, expiresAt] = BanIP.mock.calls[0]
    const now = Math.floor(Date.now() / 1000)
    expect(expiresAt).toBeGreaterThanOrEqual(now + 7 * 86400 - 5)
    expect(expiresAt).toBeLessThanOrEqual(now + 7 * 86400 + 5)
  })

  it('IP 为空：行内必填错误 + 提交按钮禁用（禁用态即前置拦截）', async () => {
    const { BanIP } = await import('../../../wailsjs/go/main/App')
    wrapper = await createWrapper()
    expect(wrapper.find('.ban-ip-field-error').text()).toContain('来源 IP')
    expect(wrapper.find('.ban-ip-confirm-btn').attributes('disabled')).toBeDefined()
    // stub button disabled 不派发 click；防御直调也不应触达后端
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    expect(BanIP).not.toHaveBeenCalled()
  })

  it('IP 格式非法：渲染行内错误 + 提交按钮禁用', async () => {
    wrapper = await createWrapper()
    await wrapper.find('input.ban-ip-input').setValue('not-an-ip')
    await flushPromises()
    expect(wrapper.find('.ban-ip-field-error').text()).toContain('格式无效')
    expect(wrapper.find('.ban-ip-confirm-btn').attributes('disabled')).toBeDefined()
  })

  it('白名单命中：即时 warning 提示 + 提交按钮禁用（后端 E_IP_BAN_WHITELISTED 权威兜底前的前端引导）', async () => {
    wrapper = await createWrapper({}, { ipban: { whitelist: ['10.0.0.0/8', '2001:db8::/32'] } })
    await wrapper.find('input.ban-ip-input').setValue('10.1.2.3')
    await flushPromises()
    expect(wrapper.find('.ban-ip-field-warn').text()).toContain('可信网段白名单')
    expect(wrapper.find('.ban-ip-confirm-btn').attributes('disabled')).toBeDefined()
    // 白名单外 IP 不命中
    await wrapper.find('input.ban-ip-input').setValue('192.0.2.10')
    await flushPromises()
    expect(wrapper.find('.ban-ip-field-warn').exists()).toBe(false)
  })

  it('自定义档未选时刻前置拦截：warning 且不调用', async () => {
    const { BanIP } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    wrapper = await createWrapper()
    await wrapper.find('input.ban-ip-input').setValue('192.0.2.10')
    await wrapper.find('select.ban-ip-expiry-select').setValue('custom')
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('解封时刻'))
    expect(BanIP).not.toHaveBeenCalled()
  })

  it('封禁失败走 handleError 分流（E_IP_BAN_WHITELISTED 预期拒绝 warning，弹窗不关闭）', async () => {
    const { BanIP } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    BanIP.mockRejectedValue({ code: 'E_IP_BAN_WHITELISTED', message: '该 IP 在可信网段白名单中（含本机回环地址），禁止封禁' })
    wrapper = await createWrapper()
    await wrapper.find('input.ban-ip-input').setValue('127.0.0.1')
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('该 IP 在可信网段白名单中（含本机回环地址），禁止封禁')
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')).toBeFalsy()
    expect(wrapper.emitted('banned')).toBeFalsy()
  })

  it('initialIP 预填目标 IP（明细行入口形态）；GetSettings 失败降级空白名单不阻塞', async () => {
    const { GetSettings } = await import('../../../wailsjs/go/main/App')
    GetSettings.mockRejectedValue(new Error('boom'))
    wrapper = mount(BanIPDialog, {
      props: { modelValue: false, initialIP: '192.0.2.99' },
      global: { stubs }
    })
    await wrapper.setProps({ modelValue: true })
    await flushPromises()
    expect(wrapper.find('input.ban-ip-input').element.value).toBe('192.0.2.99')
    // 白名单快照拉取失败降级：合法 IP 无 warning，提交链路不受阻
    expect(wrapper.find('.ban-ip-field-warn').exists()).toBe(false)
  })
})
