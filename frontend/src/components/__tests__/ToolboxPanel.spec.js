import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ToolboxPanel from '../ToolboxPanel.vue'

describe('ToolboxPanel', () => {
  const createWrapper = () => {
    return mount(ToolboxPanel, {
      global: {
        stubs: {
          'el-icon': { template: '<span><slot /></span>' },
          'el-dialog': { template: '<div v-if="modelValue" class="el-dialog"><slot /></div>', props: ['modelValue'] },
          'el-form': { template: '<div><slot /></div>' },
          'el-form-item': { template: '<div><slot /></div>' },
          'el-input': true,
          'el-checkbox': true,
          'el-button': { template: '<button v-bind="$attrs"><slot /></button>' }
        }
      }
    })
  }

  it('应该渲染标题栏和关闭按钮', () => {
    const wrapper = createWrapper()
    expect(wrapper.find('.toolbox-header').text()).toContain('工具箱')
    expect(wrapper.find('.toolbox-close').exists()).toBe(true)
  })

  it('应该渲染工具项列表', () => {
    const wrapper = createWrapper()
    const items = wrapper.findAll('.toolbox-item')
    expect(items.length).toBeGreaterThanOrEqual(1)
  })

  it('应该包含拷贝到工具项', () => {
    const wrapper = createWrapper()
    const items = wrapper.findAll('.toolbox-item')
    const copyToItem = items.find(item => item.text().includes('拷贝到'))
    expect(copyToItem).toBeDefined()
  })

  it('点击关闭按钮应触发 close 事件', async () => {
    const wrapper = createWrapper()
    await wrapper.find('.toolbox-close').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('点击拷贝到工具项应显示对话框', async () => {
    const wrapper = createWrapper()
    const items = wrapper.findAll('.toolbox-item')
    const copyToItem = items.find(item => item.text().includes('拷贝到'))
    await copyToItem.trigger('click')
    expect(wrapper.vm.copyToDialogVisible).toBe(true)
  })
})
