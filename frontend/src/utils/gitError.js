import { ElMessage } from 'element-plus'

// 后端 service.ErrOperationInProgress 的错误文案（见 service/git.go ErrOperationInProgress）。
// 变更类 Git 操作并发被 TryLock 拒绝时，error.message 含该文案。
// 前端据此区分「操作进行中」（warning，用户重试）与「普通失败」（error）。
const OP_IN_PROGRESS_TEXT = '该仓库有 Git 操作进行中，请稍后重试'

/**
 * 判断错误是否为「仓库已有 Git 操作进行中」拒绝。
 * @param {unknown} error
 * @returns {boolean}
 */
export function isGitOperationInProgress(error) {
  if (!error) return false
  const msg = error?.message || String(error)
  return msg.includes(OP_IN_PROGRESS_TEXT)
}

/**
 * 统一处理变更类 Git 操作的错误反馈。
 * - 操作进行中拒绝 → ElMessage.warning(后端原文案)，引导用户稍后重试
 * - 其他失败 → ElMessage.error(prefix + 错误信息)
 *
 * 用法：在组件 catch 块中用 `handleGitError('提交失败: ', error)` 替代裸 `ElMessage.error(...)`。
 *
 * @param {string} prefix 失败前缀（如 '提交失败: '）
 * @param {unknown} error 捕获的错误对象
 */
export function handleGitError(prefix, error) {
  if (isGitOperationInProgress(error)) {
    // 操作进行中属预期拒绝，用 warning 而非 error，且不拼前缀（后端文案已自解释）
    ElMessage.warning(error?.message || OP_IN_PROGRESS_TEXT)
    return
  }
  const msg = error?.message || String(error)
  ElMessage.error(prefix + msg)
}
