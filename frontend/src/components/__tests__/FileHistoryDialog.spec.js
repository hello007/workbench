import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import FileHistoryDialog from '../FileHistoryDialog.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetFileCommitHistory: vi.fn()
}))

import { ElMessage } from 'element-plus'
import { GetFileCommitHistory } from '../../../wailsjs/go/main/App'

const stubs = {
  // modelValue→false 时同步 emit closed（真实 el-dialog 渐隐结束后触发），
  // 使组件的 @closed releaseState 收尾在测试中真实执行
  'el-dialog': {
    template: '<div class="el-dialog" v-if="modelValue"><div class="dialog-title">{{ title }}</div><slot /></div>',
    props: ['modelValue', 'title', 'width'],
    emits: ['update:modelValue', 'closed'],
    watch: {
      modelValue(visible) {
        if (!visible) this.$emit('closed')
      }
    }
  },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['icon', 'loading', 'size', 'type', 'text', 'disabled', 'title'],
    emits: ['click']
  },
  'el-text': { template: '<span v-bind="$attrs"><slot /></span>', props: ['type', 'size'] },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type', 'size'] },
  'el-empty': { template: '<div class="el-empty" />', props: ['description', 'imageSize'] },
  'el-tooltip': { template: '<span class="el-tooltip" :data-content="content"><slot /></span>', props: ['content', 'placement'] },
  'el-icon': { template: '<i class="el-icon"><slot /></i>', props: ['title'] },
  FileDiffDialog: {
    template: '<div class="file-diff-dialog" v-if="modelValue" :data-sha="sha" :data-file="file" />',
    props: ['modelValue', 'mode', 'repoPath', 'sha', 'file'],
    emits: ['update:modelValue']
  }
}
const directives = { loading: () => {} }

// commit 工厂：默认普通提交（无 rename 检出），中文消息往返
const commit = (over = {}) => ({
  sha: 'a'.repeat(40),
  shortSha: 'aaaaaaaa',
  message: '创建设计文档\n',
  author: '张三',
  email: 'z@x.com',
  timestamp: 1794037200,
  dateTime: '2026-11-07 15:40:00',
  pathAtCommit: 'docs/设计文档.md',
  ...over
})

function createWrapper(props = {}) {
  return mount(FileHistoryDialog, {
    props: { modelValue: true, repoPath: '/repo/A', fileAbsPath: '/repo/A/docs/设计文档.md', ...props },
    global: { stubs, directives }
  })
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('FileHistoryDialog', () => {
  it('打开时按 PAGE_SIZE 拉取首页并渲染列表（中文消息/路径原样）', async () => {
    GetFileCommitHistory.mockResolvedValue([commit(), commit({ sha: 'b'.repeat(40), message: '更新内容\n' })])

    const wrapper = createWrapper()
    await flushPromises()

    expect(GetFileCommitHistory).toHaveBeenCalledWith('/repo/A/docs/设计文档.md', 20, 0)
    const rows = wrapper.findAll('[data-action="file-history-row"]')
    expect(rows).toHaveLength(2)
    expect(wrapper.text()).toContain('创建设计文档')
    expect(wrapper.text()).toContain('张三')
    // 标题含文件名（中文）
    expect(wrapper.find('.dialog-title').text()).toContain('设计文档.md')
  })

  it('返回满页时显示「加载更多」并按 offset 续拉', async () => {
    const page = Array.from({ length: 20 }, (_, i) => commit({ sha: String(i).padStart(40, 'c'), shortSha: `c${i}` }))
    GetFileCommitHistory.mockResolvedValueOnce(page)
    GetFileCommitHistory.mockResolvedValueOnce([commit({ sha: 'd'.repeat(40) })])

    const wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.find('[data-action="file-history-load-more"]').exists()).toBe(true)

    await wrapper.find('[data-action="file-history-load-more"]').trigger('click')
    await flushPromises()

    expect(GetFileCommitHistory).toHaveBeenLastCalledWith('/repo/A/docs/设计文档.md', 20, 20)
    expect(wrapper.findAll('[data-action="file-history-row"]')).toHaveLength(21)
    // 末页不满页 → hasMore=false，按钮消失
    expect(wrapper.find('[data-action="file-history-load-more"]').exists()).toBe(false)
  })

  it('rename 提交渲染 R 标记且 tooltip 含 old → new', async () => {
    GetFileCommitHistory.mockResolvedValue([
      commit({ renameFrom: 'docs/设计文档.md', renameTo: 'docs/评审稿.md', pathAtCommit: 'docs/评审稿.md' }),
      commit()
    ])

    const wrapper = createWrapper()
    await flushPromises()

    const tags = wrapper.findAll('.history-rename-tag')
    expect(tags).toHaveLength(1)
    // tooltip content 携带 old → new 文案（重命名路径原文，中文不乱码）
    const tooltip = wrapper.find('.el-tooltip')
    expect(tooltip.exists()).toBe(true)
    expect(tooltip.attributes('data-content')).toBe('重命名：docs/设计文档.md → docs/评审稿.md')
  })

  it('点击提交行打开内嵌 diff，file 取该提交时刻路径 pathAtCommit', async () => {
    GetFileCommitHistory.mockResolvedValue([
      commit({ renameFrom: 'old.md', renameTo: 'new.md', pathAtCommit: 'docs/new.md' })
    ])

    const wrapper = createWrapper()
    await flushPromises()

    await wrapper.find('[data-action="file-history-row"]').trigger('click')

    const diff = wrapper.find('.file-diff-dialog')
    expect(diff.exists()).toBe(true)
    expect(diff.attributes('data-sha')).toBe('a'.repeat(40))
    // rename 后旧段提交也能取到 diff 的关键：用 pathAtCommit 而非当前路径
    expect(diff.attributes('data-file')).toBe('docs/new.md')
  })

  it('空历史渲染空态提示', async () => {
    GetFileCommitHistory.mockResolvedValue([])

    const wrapper = createWrapper()
    await flushPromises()

    expect(wrapper.find('.el-empty').exists()).toBe(true)
    expect(wrapper.find('[data-action="file-history-row"]').exists()).toBe(false)
  })

  it('加载失败经 handleError 提示且不残留 loading', async () => {
    GetFileCommitHistory.mockRejectedValue(new Error('获取文件提交历史失败: not a git repo'))

    const wrapper = createWrapper()
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalled()
    expect(wrapper.find('.el-empty').exists()).toBe(true)
  })

  it('fileAbsPath 为空时不发起请求', async () => {
    createWrapper({ fileAbsPath: '' })
    await flushPromises()

    expect(GetFileCommitHistory).not.toHaveBeenCalled()
  })

  it('关闭后 releaseState 清空列表，重开时重新拉取', async () => {
    GetFileCommitHistory.mockResolvedValue([commit()])

    const wrapper = createWrapper()
    await flushPromises()
    expect(wrapper.findAll('[data-action="file-history-row"]')).toHaveLength(1)

    await wrapper.setProps({ modelValue: false })
    await flushPromises()
    // stub 在 modelValue=false 时 emit closed → 真实 releaseState 执行，列表清空
    expect(wrapper.find('[data-action="file-history-row"]').exists()).toBe(false)

    await wrapper.setProps({ modelValue: true })
    await flushPromises()

    expect(GetFileCommitHistory).toHaveBeenCalledTimes(2)
    expect(GetFileCommitHistory).toHaveBeenLastCalledWith('/repo/A/docs/设计文档.md', 20, 0)
  })

  it('initialSha 在首页命中：定位高亮该行且不触发跨页扫描', async () => {
    const target = 'a'.repeat(40)
    GetFileCommitHistory.mockResolvedValue([commit({ sha: target }), commit({ sha: 'b'.repeat(40) })])

    const wrapper = createWrapper({ initialSha: target })
    await flushPromises()

    // 首页即命中：仅 1 次拉取
    expect(GetFileCommitHistory).toHaveBeenCalledTimes(1)
    const rows = wrapper.findAll('[data-action="file-history-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].classes()).toContain('is-located')
    expect(rows[0].attributes('data-sha')).toBe(target)
    expect(rows[1].classes()).not.toContain('is-located')
  })

  it('initialSha 不在首页：逐页扫描至命中并高亮', async () => {
    const target = 'd'.repeat(40)
    const page = Array.from({ length: 20 }, (_, i) =>
      commit({ sha: String(i).padStart(40, 'c'), shortSha: `c${i}` })
    )
    GetFileCommitHistory.mockResolvedValueOnce(page)
    GetFileCommitHistory.mockResolvedValueOnce([commit({ sha: target })])

    const wrapper = createWrapper({ initialSha: target })
    await flushPromises()

    // 首页未命中（满页 hasMore）→ 自动续拉第 2 页命中；扫描跨页链用 vi.waitFor
    // 轮询消化（coverage 插桩/并行负载下单次 flushPromises 可能未跑完整条链）
    await vi.waitFor(() => {
      expect(GetFileCommitHistory).toHaveBeenCalledTimes(2)
    })
    expect(GetFileCommitHistory).toHaveBeenLastCalledWith('/repo/A/docs/设计文档.md', 20, 20)
    const rows = wrapper.findAll('[data-action="file-history-row"]')
    expect(rows).toHaveLength(21)
    expect(rows[20].classes()).toContain('is-located')
  })

  it('initialSha 超定位上限未命中：info 提示并不再无限扫描', async () => {
    const target = 'f'.repeat(40)
    // 恒返回满页且每页 SHA 唯一（永不命中目标）
    GetFileCommitHistory.mockImplementation((_path, _limit, offset) => {
      const page = Array.from({ length: 20 }, (_, i) =>
        commit({ sha: String(offset * 20 + i + 1).padStart(40, 'e'), shortSha: `e${offset}` })
      )
      return Promise.resolve(page)
    })

    createWrapper({ initialSha: target })
    await flushPromises()

    // 上限 500 条 = 25 页（20 起步每轮 +20 至 500 止）；终点信号（info 提示）
    // 用 vi.waitFor 轮询等待，扫描链完成后精确断言调用次数防无限扫描
    await vi.waitFor(() => {
      expect(ElMessage.info).toHaveBeenCalledWith('该提交不在最近 500 条提交中')
    })
    expect(GetFileCommitHistory).toHaveBeenCalledTimes(25)
    expect(ElMessage.info.mock.calls[0][0]).not.toContain(target)
  })

  it('initialSha 不在有限历史内：首页加载完即提示未命中', async () => {
    GetFileCommitHistory.mockResolvedValue([commit()])

    createWrapper({ initialSha: 'f'.repeat(40) })
    await flushPromises()

    // 单页且不满页（hasMore=false）：不扫描，直接提示「最近 1 条」
    expect(GetFileCommitHistory).toHaveBeenCalledTimes(1)
    expect(ElMessage.info).toHaveBeenCalledWith('该提交不在最近 1 条提交中')
  })
})
