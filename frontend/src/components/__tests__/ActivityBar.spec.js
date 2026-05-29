import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ActivityBar from '../ActivityBar.vue'

describe('ActivityBar', () => {
  const createWrapper = (props = {}) => {
    return mount(ActivityBar, {
      props: {
        modelValue: 'directory',
        ...props
      },
      global: {
        stubs: {
          'el-icon': { template: '<span><slot /></span>' },
          'el-tooltip': { template: '<div><slot /></div>', props: ['content', 'placement'] }
        }
      }
    })
  }

  it('应该渲染三个活动栏图标按钮', () => {
    const wrapper = createWrapper()
    const items = wrapper.findAll('.activity-bar-item')
    expect(items.length).toBe(3)
  })

  it('默认选中工作目录', () => {
    const wrapper = createWrapper({ modelValue: 'directory' })
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[0].classes()).toContain('is-active')
    expect(items[1].classes()).not.toContain('is-active')
  })

  it('选中工具箱时高亮对应图标', () => {
    const wrapper = createWrapper({ modelValue: 'toolbox' })
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[0].classes()).not.toContain('is-active')
    expect(items[1].classes()).toContain('is-active')
  })

  it('点击工作目录图标应触发 update:modelValue 事件', async () => {
    const wrapper = createWrapper({ modelValue: 'toolbox' })
    const items = wrapper.findAll('.activity-bar-item')
    await items[0].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeTruthy()
    expect(wrapper.emitted('update:modelValue')[0]).toEqual(['directory'])
  })

  it('点击工具箱图标应触发 update:modelValue 事件', async () => {
    const wrapper = createWrapper({ modelValue: 'directory' })
    const items = wrapper.findAll('.activity-bar-item')
    await items[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeTruthy()
    expect(wrapper.emitted('update:modelValue')[0]).toEqual(['toolbox'])
  })
})
