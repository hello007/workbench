import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ActivityBar from '../ActivityBar.vue'
import { useUiStore } from '../../store'

describe('ActivityBar', () => {
  // activePanel / terminalActive 已迁 ui store：mount 前在 uiStore 上设初值，
  // 组件内直读 uiStore.activePanel / uiStore.terminalVisible；
  // 点击面板项直写 store（不再 emit update:modelValue），设置/终端仍 emit 由 Home 处理。
  const createWrapper = (activePanel = 'directory') => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const uiStore = useUiStore()
    uiStore.activePanel = activePanel
    return mount(ActivityBar, {
      global: {
        plugins: [pinia],
        stubs: {
          'el-icon': { template: '<span><slot /></span>' },
          'el-tooltip': { template: '<div><slot /></div>', props: ['content', 'placement'] }
        }
      }
    })
  }

  it('应该渲染六个活动栏图标按钮（工作目录、AI 功能、仓库统计、工具箱、设置、终端）', () => {
    const wrapper = createWrapper()
    const items = wrapper.findAll('.activity-bar-item')
    expect(items.length).toBe(6)
  })

  it('默认选中工作目录', () => {
    const wrapper = createWrapper('directory')
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[0].classes()).toContain('is-active')
    expect(items[1].classes()).not.toContain('is-active')
    expect(items[2].classes()).not.toContain('is-active')
  })

  it('选中 AI 功能时高亮对应图标', () => {
    const wrapper = createWrapper('ai')
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[0].classes()).not.toContain('is-active')
    expect(items[1].classes()).toContain('is-active')
    expect(items[2].classes()).not.toContain('is-active')
  })

  it('选中仓库统计时高亮对应图标', () => {
    const wrapper = createWrapper('stats')
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[1].classes()).not.toContain('is-active')
    // stats 是第3个图标（index=2）
    expect(items[2].classes()).toContain('is-active')
    expect(items[3].classes()).not.toContain('is-active')
  })

  it('选中工具箱时高亮对应图标', () => {
    const wrapper = createWrapper('toolbox')
    const items = wrapper.findAll('.activity-bar-item')
    expect(items[0].classes()).not.toContain('is-active')
    expect(items[1].classes()).not.toContain('is-active')
    expect(items[2].classes()).not.toContain('is-active')
    // toolbox 是第4个图标（index=3）
    expect(items[3].classes()).toContain('is-active')
  })

  it('点击工作目录图标应将 activePanel 置为 directory', async () => {
    const wrapper = createWrapper('toolbox')
    const items = wrapper.findAll('.activity-bar-item')
    await items[0].trigger('click')
    expect(useUiStore().activePanel).toBe('directory')
  })

  it('点击 AI 功能图标应将 activePanel 置为 ai', async () => {
    const wrapper = createWrapper('directory')
    const items = wrapper.findAll('.activity-bar-item')
    // ai 是第2个图标（index=1），位于工作目录与仓库统计之间
    await items[1].trigger('click')
    expect(useUiStore().activePanel).toBe('ai')
  })

  it('点击仓库统计图标应将 activePanel 置为 stats', async () => {
    const wrapper = createWrapper('directory')
    const items = wrapper.findAll('.activity-bar-item')
    // stats 是第3个图标（index=2）
    await items[2].trigger('click')
    expect(useUiStore().activePanel).toBe('stats')
  })

  it('点击工具箱图标应将 activePanel 置为 toolbox', async () => {
    const wrapper = createWrapper('directory')
    const items = wrapper.findAll('.activity-bar-item')
    // toolbox 是第4个图标（index=3）
    await items[3].trigger('click')
    expect(useUiStore().activePanel).toBe('toolbox')
  })

  it('点击设置图标应触发 openSettings 事件', async () => {
    const wrapper = createWrapper()
    const items = wrapper.findAll('.activity-bar-item')
    // settings 是第5个图标（index=4）
    await items[4].trigger('click')
    expect(wrapper.emitted('openSettings')).toBeTruthy()
  })
})
