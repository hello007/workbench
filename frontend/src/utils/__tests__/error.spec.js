import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('element-plus', () => ({
  ElMessage: {
    warning: vi.fn(),
    error: vi.fn(),
    success: vi.fn(),
    info: vi.fn()
  }
}))

import { ElMessage } from 'element-plus'
import { handleError, handleGitError, isGitOperationInProgress, ErrorCode } from '../error'

describe('error', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  describe('handleError - code 路径（Wails ErrorFormatter 结构化对象）', () => {
    it('GitInProgress code 弹 warning 且不拼前缀', () => {
      const err = { code: ErrorCode.GitInProgress, message: '该仓库有 Git 操作进行中，请稍后重试' }
      handleError('提交失败: ', err)
      expect(ElMessage.warning).toHaveBeenCalledTimes(1)
      expect(ElMessage.warning).toHaveBeenCalledWith('该仓库有 Git 操作进行中，请稍后重试')
      expect(ElMessage.error).not.toHaveBeenCalled()
    })

    it('无 code 的结构化对象走 error 并拼前缀', () => {
      const err = { message: '网络超时' }
      handleError('推送失败: ', err)
      expect(ElMessage.error).toHaveBeenCalledWith('推送失败: 网络超时')
      expect(ElMessage.warning).not.toHaveBeenCalled()
    })
  })

  describe('handleError - 兼容裸 Error 路径', () => {
    it('裸 Error 无 code 走 error', () => {
      handleError('拉取失败: ', new Error('认证失败'))
      expect(ElMessage.error).toHaveBeenCalledWith('拉取失败: 认证失败')
    })

    it('null/undefined 回退空字符串', () => {
      handleError('操作失败: ', null)
      expect(ElMessage.error).toHaveBeenCalledWith('操作失败: ')
    })

    it('字符串错误回退 String()', () => {
      handleError('操作失败: ', '字符串错误')
      expect(ElMessage.error).toHaveBeenCalledWith('操作失败: 字符串错误')
    })
  })

  describe('isGitOperationInProgress', () => {
    it('code 路径识别', () => {
      expect(isGitOperationInProgress({ code: ErrorCode.GitInProgress, message: 'x' })).toBe(true)
    })

    it('文案路径兜底（未迁移裸 Error）', () => {
      expect(isGitOperationInProgress(new Error('该仓库有 Git 操作进行中，请稍后重试: D:/repo'))).toBe(true)
    })

    it('普通失败不识别', () => {
      expect(isGitOperationInProgress({ code: null, message: '普通失败' })).toBe(false)
      expect(isGitOperationInProgress(new Error('提交失败'))).toBe(false)
    })

    it('null/undefined 返回 false', () => {
      expect(isGitOperationInProgress(null)).toBe(false)
      expect(isGitOperationInProgress(undefined)).toBe(false)
    })
  })

  describe('handleGitError 兼容别名', () => {
    it('与 handleError 行为一致', () => {
      const err = { code: ErrorCode.GitInProgress, message: '操作进行中' }
      handleGitError('前缀: ', err)
      expect(ElMessage.warning).toHaveBeenCalledWith('操作进行中')
      expect(ElMessage.error).not.toHaveBeenCalled()
    })
  })
})
