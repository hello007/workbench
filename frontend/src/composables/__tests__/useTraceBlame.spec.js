import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// wailsjs 绑定 mock：CheckFileExists 按用例注入返回值
vi.mock('../../../wailsjs/go/main/App', () => ({
  CheckFileExists: vi.fn()
}))

// element-plus 文件级 mock（项目既有范式）：断言 info toast 分支
vi.mock('element-plus', () => ({
  ElMessage: { info: vi.fn(), warning: vi.fn(), error: vi.fn(), success: vi.fn() }
}))

import { CheckFileExists } from '../../../wailsjs/go/main/App'
import { ElMessage } from 'element-plus'
import { requestTraceBlame, blameCapableExt } from '../useTraceBlame'
import { useUiStore } from '../../store/ui'

describe('blameCapableExt', () => {
  it('普通代码文件可用', () => {
    expect(blameCapableExt('main.go')).toBe(true)
    expect(blameCapableExt('App.vue')).toBe(true)
    expect(blameCapableExt('README')).toBe(true)
  })

  it('JSON 系派生视图不可用（大小写不敏感）', () => {
    expect(blameCapableExt('config.json')).toBe(false)
    expect(blameCapableExt('settings.JSONC')).toBe(false)
    expect(blameCapableExt('data.json5')).toBe(false)
  })

  it('markdown/HTML 渲染视图不可用', () => {
    expect(blameCapableExt('README.md')).toBe(false)
    expect(blameCapableExt('doc.markdown')).toBe(false)
    expect(blameCapableExt('index.html')).toBe(false)
    expect(blameCapableExt('page.HTM')).toBe(false)
  })
})

describe('requestTraceBlame', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('文件在工作树存在且类型可用 → 写追溯信箱（absPath + seq 单调递增）', async () => {
    CheckFileExists.mockResolvedValue(true)
    const ui = useUiStore()

    await requestTraceBlame('D:\\repo\\src\\main.go', 'main.go', () => {})
    expect(ui.traceBlameRequest).toEqual({ absPath: 'D:\\repo\\src\\main.go', seq: 1 })
    expect(ElMessage.info).not.toHaveBeenCalled()

    // 同文件两次点击：seq 递增保证信箱 watch 不被依赖比较去重丢失
    await requestTraceBlame('D:\\repo\\src\\main.go', 'main.go', () => {})
    expect(ui.traceBlameRequest.seq).toBe(2)
  })

  it('文件不在工作树（已删除/重命名）→ 降级 toast + 打开文件历史', async () => {
    CheckFileExists.mockResolvedValue(false)
    const ui = useUiStore()
    const fallback = vi.fn()

    await requestTraceBlame('D:\\repo\\old.txt', 'old.txt', fallback)
    expect(ElMessage.info).toHaveBeenCalledWith('文件已不在工作树，已转入文件历史追溯')
    expect(fallback).toHaveBeenCalledTimes(1)
    expect(ui.traceBlameRequest).toBeNull()
  })

  it('CheckFileExists 绑定异常按不存在处理（fail-safe 转文件历史）', async () => {
    CheckFileExists.mockRejectedValue(new Error('rpc failed'))
    const fallback = vi.fn()

    await requestTraceBlame('D:\\repo\\a.txt', 'a.txt', fallback)
    expect(fallback).toHaveBeenCalledTimes(1)
    expect(ElMessage.info).toHaveBeenCalledWith('文件已不在工作树，已转入文件历史追溯')
  })

  it('类型不支持行归属（json/md/html）→ 降级 toast + 打开文件历史', async () => {
    CheckFileExists.mockResolvedValue(true)
    const ui = useUiStore()
    const fallback = vi.fn()

    await requestTraceBlame('D:\\repo\\notes.md', 'notes.md', fallback)
    expect(ElMessage.info).toHaveBeenCalledWith('该文件类型不支持行归属，已转入文件历史追溯')
    expect(fallback).toHaveBeenCalledTimes(1)
    expect(ui.traceBlameRequest).toBeNull()
  })

  it('fileName 缺省时从 absPath 提取扩展名判定', async () => {
    CheckFileExists.mockResolvedValue(true)
    const fallback = vi.fn()

    await requestTraceBlame('D:\\repo\\docs\\README.md', '', fallback)
    expect(ElMessage.info).toHaveBeenCalledWith('该文件类型不支持行归属，已转入文件历史追溯')
    expect(fallback).toHaveBeenCalledTimes(1)
  })

  it('absPath 为空直接返回（不调绑定不降级）', async () => {
    const fallback = vi.fn()

    await requestTraceBlame('', 'a.txt', fallback)
    expect(CheckFileExists).not.toHaveBeenCalled()
    expect(fallback).not.toHaveBeenCalled()
    expect(ElMessage.info).not.toHaveBeenCalled()
  })
})
