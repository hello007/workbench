import { ElMessage } from 'element-plus'

// 错误码常量。与后端 model/app_error.go 错误码表保持一致。
// 后端经 Wails ErrorFormatter 将 AppError 序列化为 {code, message} 对象传前端，
// 前端按 code 分流提示级别（warning=预期拒绝，error=真失败）。
// 详见 docs/spec/logging-and-errors.md。
export const ErrorCode = Object.freeze({
  GitInProgress: 'E_GIT_IN_PROGRESS', // 变更类 Git 操作进行中，本次拒绝（warning）
  GitNoStagedChanges: 'E_GIT_NO_STAGED_CHANGES', // 无暂存文件，AI 提交信息生成按钮禁用（warning）
  DiffToolNotConfigured: 'E_DIFF_TOOL_NOT_CONFIGURED', // 外部 diff 工具未配置或配置无效，引导用户去设置（warning）
  DiffToolLaunchFailed: 'E_DIFF_TOOL_LAUNCH_FAILED', // 外部 diff 工具启动失败（error）
  RepoConfigInvalidJson: 'E_REPO_CONFIG_INVALID_JSON', // 仓库列表配置导入：文件不是合法 JSON 或顶层结构缺失（error）
  RepoConfigUnsupportedVersion: 'E_REPO_CONFIG_UNSUPPORTED_VERSION' // 仓库列表配置导入：manifestVersion 缺失或高于当前支持（error）
})

// 预期拒绝类错误码（用户可重试），弹 warning 而非 error。
const WARNING_CODES = new Set([ErrorCode.GitInProgress, ErrorCode.GitNoStagedChanges, ErrorCode.DiffToolNotConfigured])

/**
 * 从错误对象提取 code 与 message。
 *
 * Wails ErrorFormatter 返回结构化对象 {code, message}（非 Error 实例），
 * 故优先读对象属性；兼容旧路径裸 Error（无 code，走 message 字符串）。
 *
 * @param {unknown} error
 * @returns {{code: string|null, message: string}}
 */
function extractError(error) {
  if (!error) return { code: null, message: '' }
  const code = error?.code ?? null
  const message = error?.message ?? String(error)
  return { code, message }
}

/**
 * 判断错误是否为「仓库已有 Git 操作进行中」拒绝。
 * 优先读 code（新路径），兜底文案匹配（未迁移路径）。
 * @param {unknown} error
 * @returns {boolean}
 */
export function isGitOperationInProgress(error) {
  const { code, message } = extractError(error)
  if (code === ErrorCode.GitInProgress) return true
  return message.includes('该仓库有 Git 操作进行中，请稍后重试')
}

/**
 * 判断错误是否为预期拒绝类（用户可重试），应弹 warning。
 * 优先 code 命中 WARNING_CODES（新路径），兜底 GitInProgress 文案匹配（未迁移裸 Error）。
 * @param {string|null} code
 * @param {string} message
 * @returns {boolean}
 */
function isWarningError(code, message) {
  if (code && WARNING_CODES.has(code)) return true
  return message.includes('该仓库有 Git 操作进行中，请稍后重试')
}

/**
 * 统一处理错误反馈。按 code 分流：
 * - 预期拒绝类（如 GitInProgress）→ ElMessage.warning(message)，不拼前缀
 * - 真失败 → ElMessage.error(prefix + message)
 *
 * 用法：在组件 catch 块中用 `handleError('提交失败: ', error)` 替代裸 ElMessage.error。
 *
 * @param {string} prefix 失败前缀（如 '提交失败: '）
 * @param {unknown} error 捕获的错误对象（Wails 结构化 {code, message} 或裸 Error）
 */
export function handleError(prefix, error) {
  const { code, message } = extractError(error)
  if (isWarningError(code, message)) {
    // 预期拒绝类，用 warning 引导用户重试，不拼前缀（后端 message 已自解释）
    ElMessage.warning(message)
    return
  }
  ElMessage.error(prefix + message)
}

/**
 * handleGitError 是 handleError 的 git 域别名，保留兼容现有调用点。
 * 新代码直接用 handleError。
 * @param {string} prefix
 * @param {unknown} error
 */
export function handleGitError(prefix, error) {
  handleError(prefix, error)
}
