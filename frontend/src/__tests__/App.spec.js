/**
 * App.vue 主题应用测试
 * 覆盖：resolvedTheme → html.dark class 切换、挂载时 loadTheme
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import App from '../App.vue'
import { useSettingsStore } from '../store'

// App.vue onMounted 调用 settingsStore.loadTheme() → GetSettings，需 mock
vi.mock('../../wailsjs/go/main/App', () => ({
  GetSettings: vi.fn(() => Promise.resolve({})),
  SaveSettings: vi.fn(() => Promise.resolve(true))
}))

// router-view 不路由到真实页面，stub 为空 div（主题逻辑不依赖路由内容）
const stubs = {
  'router-view': { template: '<div class="rv" />' }
}

async function mountApp() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const wrapper = mount(App, { global: { plugins: [pinia], stubs } })
  await flushPromises()
  return wrapper
}

describe('App.vue - 主题应用', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    document.documentElement.classList.remove('dark')
  })

  afterEach(() => {
    if (wrapper) { wrapper.unmount(); wrapper = null }
    document.documentElement.classList.remove('dark')
  })

  it('resolvedTheme=light 时 html 不含 dark class', async () => {
    wrapper = await mountApp()
    const store = useSettingsStore()
    store.themeMode = 'light'
    await flushPromises()
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('resolvedTheme=dark 时 html 加 dark class', async () => {
    wrapper = await mountApp()
    const store = useSettingsStore()
    store.themeMode = 'dark'
    await flushPromises()
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('resolvedTheme 切换时 dark class 实时跟随（无刷新）', async () => {
    wrapper = await mountApp()
    const store = useSettingsStore()
    store.themeMode = 'dark'
    await flushPromises()
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    store.themeMode = 'light'
    await flushPromises()
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    // 再切回 dark
    store.themeMode = 'dark'
    await flushPromises()
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('挂载时调用 loadTheme（GetSettings 被调用）', async () => {
    const { GetSettings } = await import('../../wailsjs/go/main/App')
    wrapper = await mountApp()
    await flushPromises()
    expect(GetSettings).toHaveBeenCalled()
  })

  it('system 模式 + 系统浅色（matchMedia matches:false）→ 不加 dark class', async () => {
    // test/setup.js 全局 matchMedia stub 返回 matches:false
    wrapper = await mountApp()
    const store = useSettingsStore()
    store.themeMode = 'system'
    await flushPromises()
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })
})
