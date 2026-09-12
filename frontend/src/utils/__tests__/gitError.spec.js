import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// mock element-plus 的 ElMessage，避免依赖真实组件挂载
vi.mock('element-plus', () => ({
  ElMessage: {
    warning: vi.fn(),
    error: vi.fn(),
    success: vi.fn(),
    info: vi.fn()
  }
}))

import { ElMessage } from 'element-plus'
import { isGitOperationInProgress, handleGitError } from '../gitError'

describe('gitError', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  describe('isGitOperationInProgress', () => {
    it('null/undefined 返回 false', () => {
      expect(isGitOperationInProgress(null)).toBe(false)
      expect(isGitOperationInProgress(undefined)).toBe(false)
    })

    it('含 ErrOperationInProgress 文案返回 true', () => {
      const err = new Error('该仓库有 Git 操作进行中，请稍后重试: D:/repo')
      expect(isGitOperationInProgress(err)).toBe(true)
    })

    it('普通失败返回 false', () => {
      expect(isGitOperationInProgress(new Error('提交失败: 网络异常'))).toBe(false)
    })

    it('非 Error 对象按字符串判定', () => {
      expect(isGitOperationInProgress('该仓库有 Git 操作进行中，请稍后重试')).toBe(true)
      expect(isGitOperationInProgress('其他错误')).toBe(false)
    })
  })

  describe('handleGitError', () => {
    it('操作进行中错误弹 warning 且不拼前缀', () => {
      const err = new Error('该仓库有 Git 操作进行中，请稍后重试')
      handleGitError('提交失败: ', err)
      expect(ElMessage.warning).toHaveBeenCalledTimes(1)
      expect(ElMessage.warning).toHaveBeenCalledWith('该仓库有 Git 操作进行中，请稍后重试')
      // 不应走 error 分支
      expect(ElMessage.error).not.toHaveBeenCalled()
    })

    it('普通失败弹 error 并拼前缀', () => {
      const err = new Error('网络超时')
      handleGitError('推送失败: ', err)
      expect(ElMessage.error).toHaveBeenCalledTimes(1)
      expect(ElMessage.error).toHaveBeenCalledWith('推送失败: 网络超时')
      expect(ElMessage.warning).not.toHaveBeenCalled()
    })

    it('无 message 的错误回退 String()', () => {
      handleGitError('拉取失败: ', '字符串错误')
      expect(ElMessage.error).toHaveBeenCalledWith('拉取失败: 字符串错误')
    })
  })
})
