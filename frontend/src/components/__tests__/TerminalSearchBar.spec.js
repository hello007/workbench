/**
 * TerminalSearchBar 组件测试
 * 覆盖：Enter=下一处 / Shift+Enter=上一处、Esc 关闭且 stopPropagation
 * （防冒泡触发整窗全屏退出，terminal-multi-session §5 契约）、输入向上通知、
 * 大小写 toggle、匹配计数展示、关闭按钮
 */
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import TerminalSearchBar from '../TerminalSearchBar.vue'

const stubs = {
  'el-icon': { template: '<i><slot /></i>', props: ['size'] }
}

function mountBar(props = {}) {
  return mount(TerminalSearchBar, {
    props,
    global: { stubs }
  })
}

describe('TerminalSearchBar.vue', () => {
  it('挂载后输入框自动聚焦（呼出即可输入）', async () => {
    // jsdom focus 依赖元素在文档内：attachTo 挂载后 activeElement 才随 focus 流转
    const wrapper = mount(TerminalSearchBar, {
      props: {},
      global: { stubs },
      attachTo: document.body
    })
    const input = wrapper.find('[data-search-bar] input')
    expect(input.element).toBe(document.activeElement)
    wrapper.unmount()
  })

  it('输入触发 query 事件携带关键词', async () => {
    const wrapper = mountBar()
    const input = wrapper.find('input')
    await input.setValue('error')
    await input.trigger('input')
    expect(wrapper.emitted('query')).toBeTruthy()
    expect(wrapper.emitted('query')[0]).toEqual(['error'])
    wrapper.unmount()
  })

  it('Enter 触发 next（下一处）', async () => {
    const wrapper = mountBar()
    await wrapper.find('input').setValue('error')
    await wrapper.find('input').trigger('keydown.enter')
    expect(wrapper.emitted('next')).toBeTruthy()
    expect(wrapper.emitted('prev')).toBeFalsy()
    wrapper.unmount()
  })

  it('Shift+Enter 触发 prev（上一处）', async () => {
    const wrapper = mountBar()
    await wrapper.find('input').trigger('keydown.enter', { shiftKey: true })
    expect(wrapper.emitted('prev')).toBeTruthy()
    expect(wrapper.emitted('next')).toBeFalsy()
    wrapper.unmount()
  })

  it('Esc 关闭并阻止冒泡（stopPropagation 防触发整窗全屏退出）', async () => {
    const wrapper = mountBar()
    const evt = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    const stopSpy = vi.spyOn(evt, 'stopPropagation')
    wrapper.find('input').element.dispatchEvent(evt)
    await wrapper.vm.$nextTick()
    expect(stopSpy).toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeTruthy()
    wrapper.unmount()
  })

  it('Aa toggle 切换区分大小写并触发 case 事件', async () => {
    const wrapper = mountBar()
    const toggle = wrapper.find('[data-action="toggle-case"]')
    expect(toggle.classes()).not.toContain('is-active')
    await toggle.trigger('click')
    expect(wrapper.emitted('case')).toBeTruthy()
    expect(wrapper.emitted('case')[0]).toEqual([true])
    expect(toggle.classes()).toContain('is-active')
    await toggle.trigger('click')
    expect(wrapper.emitted('case')[1]).toEqual([false])
    wrapper.unmount()
  })

  it('上一处/下一处/关闭按钮语义锚定位并触发对应事件', async () => {
    const wrapper = mountBar()
    await wrapper.find('[data-action="prev"]').trigger('click')
    await wrapper.find('[data-action="next"]').trigger('click')
    await wrapper.find('[data-action="close"]').trigger('click')
    expect(wrapper.emitted('prev')).toHaveLength(1)
    expect(wrapper.emitted('next')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('匹配计数：有命中展示「当前/总数」（resultIndex 0 基 +1）', async () => {
    const wrapper = mountBar({ resultIndex: 2, resultCount: 5 })
    expect(wrapper.find('[data-search-count]').text()).toBe('3/5')
    wrapper.unmount()
  })

  it('匹配计数：有关键词无命中展示「无结果」', async () => {
    const wrapper = mountBar()
    await wrapper.find('input').setValue('nothing')
    expect(wrapper.find('[data-search-count]').text()).toBe('无结果')
    wrapper.unmount()
  })

  it('空关键词不展示计数占位', () => {
    const wrapper = mountBar({ resultIndex: -1, resultCount: 0 })
    expect(wrapper.find('[data-search-count]').text()).toBe('')
    wrapper.unmount()
  })
})
