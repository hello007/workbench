import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import BanTokenDialog from '../BanTokenDialog.vue'

vi.mock('element-plus', () => ({
  ElMessage: { success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }
}))

vi.mock('../../../wailsjs/go/main/App', () => ({
  BanWebToken: vi.fn(),
  KickConnection: vi.fn()
}))

// 联动踢出编排 mock：弹窗 spec 只断言接线（按 connections + tokenId 委托），
// 逐连接踢出与 fail-soft 语义由 tokenBan 自身 spec 覆盖
vi.mock('../../utils/tokenBan', () => ({
  kickTokenConnections: vi.fn().mockResolvedValue()
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
    props: ['modelValue', 'placeholder', 'size', 'maxlength'],
    emits: ['update:modelValue']
  },
  'el-date-picker': {
    template: '<input class="el-date-picker" :value="modelValue || \'\'" :placeholder="placeholder" @change="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'type', 'placeholder', 'disabledDate', 'size'],
    emits: ['update:modelValue']
  },
  'el-checkbox': {
    template: '<label class="el-checkbox"><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" /><slot /></label>',
    props: ['modelValue'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="loading" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'size', 'loading', 'text'],
    emits: ['click']
  }
}

/** 连接明细 fixture（形状对齐 useDeviceConnections 的 connections 条目） */
const conn = (id, tokenId) => ({ id, ip: `10.0.0.${id}`, tokenId, tokenName: '', connectedAt: new Date().toISOString() })

async function createWrapper(over = {}) {
  const wrapper = mount(BanTokenDialog, {
    props: {
      modelValue: true,
      token: { id: 'tok-1', name: '手机' },
      connections: [conn(1, 'tok-1'), conn(2, 'tok-2')],
      ...over
    },
    global: { stubs }
  })
  await flushPromises()
  return wrapper
}

describe('BanTokenDialog.vue（封禁令牌弹窗，连接管控四期）', () => {
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

  it('默认表单态：永久档选中、联动踢出默认勾选、渲染目标令牌名与封禁语义警示', async () => {
    wrapper = await createWrapper()
    expect(wrapper.find('select.ban-expiry-select').element.value).toBe('permanent')
    expect(wrapper.find('.ban-kick-check input[type="checkbox"]').element.checked).toBe(true)
    expect(wrapper.find('.ban-token-target-name').text()).toBe('手机')
    // 警示文案含封禁语义（新连接被拒 / 已建立连接不受影响 / 可勾选同时断开）
    expect(wrapper.text()).toContain('新连接将被拒绝')
    expect(wrapper.text()).toContain('已建立连接不受影响')
    expect(wrapper.text()).toContain('同时断开在途连接')
  })

  it('永久档提交：BanWebToken(id, 0, reason) 且 reason 透传，成功后关弹窗并 emit banned', async () => {
    const { BanWebToken } = await import('../../../wailsjs/go/main/App')
    BanWebToken.mockResolvedValue()
    wrapper = await createWrapper()
    await wrapper.find('input.ban-reason-input').setValue('疑似泄露')
    await wrapper.find('.ban-confirm-btn').trigger('click')
    await flushPromises()
    expect(BanWebToken).toHaveBeenCalledTimes(1)
    expect(BanWebToken).toHaveBeenCalledWith('tok-1', 0, '疑似泄露')
    // 成功后关闭弹窗 + 通知父级刷新
    expect(wrapper.emitted('update:modelValue')).toContainEqual([false])
    expect(wrapper.emitted('banned')).toBeTruthy()
  })

  it('天级档提交：expiresAt 换算为当前时间 + N 天（unix 秒）', async () => {
    const { BanWebToken } = await import('../../../wailsjs/go/main/App')
    BanWebToken.mockResolvedValue()
    wrapper = await createWrapper()
    await wrapper.find('select.ban-expiry-select').setValue('7')
    await wrapper.find('.ban-confirm-btn').trigger('click')
    await flushPromises()
    const [, expiresAt] = BanWebToken.mock.calls[0]
    const now = Math.floor(Date.now() / 1000)
    expect(expiresAt).toBeGreaterThanOrEqual(now + 7 * 86400 - 5)
    expect(expiresAt).toBeLessThanOrEqual(now + 7 * 86400 + 5)
  })

  it('自定义档未选日期时前置拦截：warning 且不调用', async () => {
    const { BanWebToken } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    wrapper = await createWrapper()
    await wrapper.find('select.ban-expiry-select').setValue('custom')
    await wrapper.find('.ban-confirm-btn').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('解封日期'))
    expect(BanWebToken).not.toHaveBeenCalled()
  })

  it('联动踢出勾选（默认）：成功后按 connections + tokenId 委托编排函数', async () => {
    const { kickTokenConnections } = await import('../../utils/tokenBan')
    const { BanWebToken } = await import('../../../wailsjs/go/main/App')
    BanWebToken.mockResolvedValue()
    const connections = [conn(1, 'tok-1'), conn(2, 'tok-2')]
    wrapper = await createWrapper({ connections })
    await wrapper.find('.ban-confirm-btn').trigger('click')
    await flushPromises()
    expect(kickTokenConnections).toHaveBeenCalledTimes(1)
    expect(kickTokenConnections).toHaveBeenCalledWith(connections, 'tok-1')
  })

  it('取消联动踢出勾选：成功后不触发编排', async () => {
    const { kickTokenConnections } = await import('../../utils/tokenBan')
    const { BanWebToken } = await import('../../../wailsjs/go/main/App')
    BanWebToken.mockResolvedValue()
    wrapper = await createWrapper()
    await wrapper.find('.ban-kick-check input[type="checkbox"]').setValue(false)
    await wrapper.find('.ban-confirm-btn').trigger('click')
    await flushPromises()
    expect(BanWebToken).toHaveBeenCalledTimes(1)
    expect(kickTokenConnections).not.toHaveBeenCalled()
  })

  it('封禁失败走 handleError 分流（E_TOKEN_REVOKED 预期拒绝 warning，弹窗不关闭）', async () => {
    const { BanWebToken } = await import('../../../wailsjs/go/main/App')
    const { ElMessage } = await import('element-plus')
    BanWebToken.mockRejectedValue({ code: 'E_TOKEN_REVOKED', message: '令牌已撤销（凭据已失效，无需封禁），请刷新列表' })
    wrapper = await createWrapper()
    await wrapper.find('.ban-confirm-btn').trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('令牌已撤销（凭据已失效，无需封禁），请刷新列表')
    expect(ElMessage.error).not.toHaveBeenCalled()
    // 弹窗保持打开（未 emit update:modelValue false）
    expect(wrapper.emitted('update:modelValue')).toBeFalsy()
    expect(wrapper.emitted('banned')).toBeFalsy()
  })

  it('点击取消仅关弹窗：不调用且不 emit banned', async () => {
    const { BanWebToken } = await import('../../../wailsjs/go/main/App')
    wrapper = await createWrapper()
    await wrapper.findAll('button').find(b => b.text() === '取消').trigger('click')
    await flushPromises()
    expect(BanWebToken).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')).toContainEqual([false])
    expect(wrapper.emitted('banned')).toBeFalsy()
  })

  it('令牌名空串降级展示 ID（明细行已撤销令牌入口形态）', async () => {
    wrapper = await createWrapper({ token: { id: 'tok-9', name: '' } })
    expect(wrapper.find('.ban-token-target-name').text()).toBe('tok-9')
  })
})
