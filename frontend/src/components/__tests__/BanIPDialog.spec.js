import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import BanIPDialog from '../BanIPDialog.vue'

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }
}))

vi.mock('../../../wailsjs/go/main/App', () => ({
  BanIP: vi.fn(),
  BanCIDR: vi.fn(),
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
  'el-radio-group': {
    template: '<div class="el-radio-group"><slot /></div>',
    props: ['modelValue', 'size'],
    emits: ['update:modelValue', 'change']
  },
  'el-radio': {
    template: '<label class="el-radio"><slot /></label>',
    props: ['value', 'size']
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

describe('BanIPDialog.vue 网段档（连接管控八期）', () => {
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

  /** 挂载并切到网段档（el-radio-group stub emit update:modelValue 写 targetKind） */
  async function createCidrWrapper(settings = {}) {
    const { GetSettings } = await import('../../../wailsjs/go/main/App')
    GetSettings.mockResolvedValue(settings)
    wrapper = mount(BanIPDialog, {
      props: { modelValue: false },
      global: { stubs }
    })
    await wrapper.setProps({ modelValue: true })
    await flushPromises()
    await wrapper.findComponent(stubs['el-radio-group']).vm.$emit('update:modelValue', 'cidr')
    await flushPromises()
    return wrapper
  }

  it('档位切换：默认单 IP 档（placeholder IP 形态），切网段档后输入框换网段 placeholder', async () => {
    const { GetSettings } = await import('../../../wailsjs/go/main/App')
    GetSettings.mockResolvedValue({})
    wrapper = mount(BanIPDialog, {
      props: { modelValue: false },
      global: { stubs }
    })
    await wrapper.setProps({ modelValue: true })
    await flushPromises()
    expect(wrapper.find('input.ban-ip-input').attributes('placeholder')).toContain('192.0.2.10')
    await wrapper.findComponent(stubs['el-radio-group']).vm.$emit('update:modelValue', 'cidr')
    await flushPromises()
    expect(wrapper.find('input.ban-ip-input').attributes('placeholder')).toContain('203.0.113.0/24')
    expect(wrapper.find('.ban-ip-kind-group').exists()).toBe(true)
  })

  it('网段为空与格式非法：行内错误 + 提交按钮禁用（不发请求）', async () => {
    const { BanCIDR } = await import('../../../wailsjs/go/main/App')
    wrapper = await createCidrWrapper()
    expect(wrapper.find('.ban-ip-field-error').text()).toContain('请填写要封禁的网段')
    expect(wrapper.find('.ban-ip-confirm-btn').attributes('disabled')).toBeDefined()
    await wrapper.find('input.ban-ip-input').setValue('not-a-cidr')
    await flushPromises()
    expect(wrapper.find('.ban-ip-field-error').text()).toContain('网段格式无效')
    expect(wrapper.find('.ban-ip-confirm-btn').attributes('disabled')).toBeDefined()
    // stub button disabled 不派发 click；防御直调也不应触达后端
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    expect(BanCIDR).not.toHaveBeenCalled()
  })

  it('合法网段永久档提交：BanCIDR(cidr, 0, reason) 透传且 BanIP 不调用，成功后关弹窗并 emit banned', async () => {
    const { BanCIDR, BanIP } = await import('../../../wailsjs/go/main/App')
    BanCIDR.mockResolvedValue()
    wrapper = await createCidrWrapper()
    await wrapper.find('input.ban-ip-input').setValue('203.0.113.0/24')
    await wrapper.find('input[placeholder^="如：持续"]').setValue('扫描源网段')
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    expect(BanCIDR).toHaveBeenCalledTimes(1)
    expect(BanCIDR).toHaveBeenCalledWith('203.0.113.0/24', 0, '扫描源网段')
    expect(BanIP).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')).toContainEqual([false])
    expect(wrapper.emitted('banned')).toBeTruthy()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已封禁网段 203.0.113.0/24')
  })

  it('裸 IP 网段档合法（与后端 /32 //128 口径一致）：原串透传 BanCIDR', async () => {
    const { BanCIDR } = await import('../../../wailsjs/go/main/App')
    BanCIDR.mockResolvedValue()
    wrapper = await createCidrWrapper()
    await wrapper.find('input.ban-ip-input').setValue('2001:db8::1')
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    expect(BanCIDR).toHaveBeenCalledWith('2001:db8::1', 0, '')
  })

  it('白名单部分交集：警示行显交集条数（非阻断，按钮可用）', async () => {
    // 10.1.0.0/16 仅是封禁网段 10.0.0.0/8 的子集：相交但非完全覆盖（封禁合法）
    wrapper = await createCidrWrapper({ ipban: { whitelist: ['10.1.0.0/16', '192.168.0.0/16'] } })
    await wrapper.find('input.ban-ip-input').setValue('10.0.0.0/8')
    await flushPromises()
    expect(wrapper.find('.ban-ip-field-warn').exists()).toBe(true)
    expect(wrapper.find('.ban-ip-field-warn').text()).toContain('有交集')
    expect(wrapper.find('.ban-ip-field-warn').text()).toContain('1 条')
    expect(wrapper.find('.ban-ip-confirm-btn').attributes('disabled')).toBeUndefined()
  })

  it('白名单完全覆盖（被单条条目完全包含）：提示封禁将被拒绝 + 提交禁用', async () => {
    const { BanCIDR } = await import('../../../wailsjs/go/main/App')
    wrapper = await createCidrWrapper({ ipban: { whitelist: ['192.168.0.0/16'] } })
    await wrapper.find('input.ban-ip-input').setValue('192.168.1.0/24')
    await flushPromises()
    expect(wrapper.find('.ban-ip-field-warn').text()).toContain('完全包含')
    expect(wrapper.find('.ban-ip-confirm-btn').attributes('disabled')).toBeDefined()
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    expect(BanCIDR).not.toHaveBeenCalled()
  })

  it('全网段封禁（0.0.0.0/0）：确认文案升级「拒绝除白名单与本机回环外的全部来源」', async () => {
    wrapper = await createCidrWrapper()
    await wrapper.find('input.ban-ip-input').setValue('0.0.0.0/0')
    await flushPromises()
    expect(wrapper.find('.ban-ip-warning').text()).toContain('本机回环外的全部来源')
  })

  it('IPv6 全网段（::/0）确认文案同升级', async () => {
    wrapper = await createCidrWrapper()
    await wrapper.find('input.ban-ip-input').setValue('::/0')
    await flushPromises()
    expect(wrapper.find('.ban-ip-warning').text()).toContain('本机回环外的全部来源')
  })

  it('网段档提交失败走 handleError 分流（E_IP_BAN_CIDR_INVALID 预期拒绝码 warning 提示，弹窗不关闭）', async () => {
    const { BanCIDR } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    BanCIDR.mockRejectedValue({ code: 'E_IP_BAN_CIDR_INVALID', message: '网段格式无效' })
    wrapper = await createCidrWrapper()
    await wrapper.find('input.ban-ip-input').setValue('203.0.113.0/24')
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('网段格式无效')
    expect(wrapper.emitted('update:modelValue')).toBeFalsy()
    expect(wrapper.emitted('banned')).toBeFalsy()
  })
})

describe('BanIPDialog.vue 网段预填（连接管控九期聚合建议入口）', () => {
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

  /** 挂载并打开（props 可带 initialCidr/initialIP，与真实 v-model 打开时序一致） */
  async function createOpenedWrapper(over = {}, settings = {}) {
    const { GetSettings } = await import('../../../wailsjs/go/main/App')
    GetSettings.mockResolvedValue(settings)
    wrapper = mount(BanIPDialog, {
      props: { modelValue: false, ...over },
      global: { stubs }
    })
    await wrapper.setProps({ modelValue: true })
    await flushPromises()
    return wrapper
  }

  it('initialCidr 预填：档位自动切网段档（placeholder 换网段形态）+ 网段输入预填', async () => {
    wrapper = await createOpenedWrapper({ initialCidr: '203.0.113.0/24' })
    // 档位自动切网段档：输入框 placeholder 换网段形态（含网段示例串）
    const input = wrapper.find('input.ban-ip-input')
    expect(input.attributes('placeholder')).toContain('203.0.113.0/24')
    // 网段输入预填建议网段串
    expect(input.element.value).toBe('203.0.113.0/24')
    // 合法网段无行内错误（白名单空快照无警示）
    expect(wrapper.find('.ban-ip-field-error').exists()).toBe(false)
  })

  it('initialCidr 预填后直接提交：BanCIDR 以建议网段与永久档/空原因透传且 BanIP 不调用', async () => {
    const { BanCIDR, BanIP } = await import('../../../wailsjs/go/main/App')
    BanCIDR.mockResolvedValue()
    wrapper = await createOpenedWrapper({ initialCidr: '203.0.113.0/24' })
    // 时长档不预填：维持默认永久档
    expect(wrapper.find('select.ban-ip-expiry-select').element.value).toBe('permanent')
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    expect(BanCIDR).toHaveBeenCalledTimes(1)
    expect(BanCIDR).toHaveBeenCalledWith('203.0.113.0/24', 0, '')
    expect(BanIP).not.toHaveBeenCalled()
    expect(wrapper.emitted('banned')).toBeTruthy()
  })

  it('initialCidr 网段与白名单完全覆盖时预检拦截（复用网段档现成逻辑零改动）', async () => {
    const { BanCIDR } = await import('../../../wailsjs/go/main/App')
    wrapper = await createOpenedWrapper(
      { initialCidr: '192.168.1.0/24' },
      { ipban: { whitelist: ['192.168.0.0/16'] } }
    )
    expect(wrapper.find('.ban-ip-field-warn').text()).toContain('完全包含')
    expect(wrapper.find('.ban-ip-confirm-btn').attributes('disabled')).toBeDefined()
    await wrapper.find('.ban-ip-confirm-btn').trigger('click')
    await flushPromises()
    expect(BanCIDR).not.toHaveBeenCalled()
  })

  it('双空默认 ip 档（两 prop 均空串 = 现状默认路径零回归）', async () => {
    wrapper = await createOpenedWrapper({ initialCidr: '', initialIP: '' })
    const input = wrapper.find('input.ban-ip-input')
    // 单 IP 档 placeholder 形态 + 输入为空
    expect(input.attributes('placeholder')).toContain('192.0.2.10')
    expect(input.element.value).toBe('')
    expect(wrapper.find('.ban-ip-field-error').text()).toContain('来源 IP')
  })

  it('initialIP 路径零回归：预填不切网段档（placeholder 保持 IP 形态）', async () => {
    wrapper = await createOpenedWrapper({ initialIP: '192.0.2.99' })
    const input = wrapper.find('input.ban-ip-input')
    expect(input.attributes('placeholder')).toContain('192.0.2.10')
    expect(input.element.value).toBe('192.0.2.99')
  })

  it('双非空防御形态 initialIP 优先（真实调用面互斥不传双值，防御口径钉死）', async () => {
    wrapper = await createOpenedWrapper({ initialIP: '192.0.2.99', initialCidr: '203.0.113.0/24' })
    const input = wrapper.find('input.ban-ip-input')
    // initialIP 优先：单 IP 档 + IP 预填，网段串不落地
    expect(input.attributes('placeholder')).toContain('192.0.2.10')
    expect(input.element.value).toBe('192.0.2.99')
    expect(wrapper.find('select.ban-ip-expiry-select').element.value).toBe('permanent')
  })
})
