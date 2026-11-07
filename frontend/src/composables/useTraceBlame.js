// 行归属追溯入口分流（提交历史变更文件 / 文件 diff 文件名共用）。
// blame 为工作树语义（后端 GetFileBlame 不传 rev），仅工作树存在的文件可标注：
// 已被后续提交删除/重命名的文件经 CheckFileExists 预判后降级转文件历史（追溯链
// 不中断）；JSON 系与 markdown/HTML 文件类型无行对齐语义（FilePreviewRenderer
// blameAvailable 同源条件）同样降级。可用文件经 uiStore 追溯信箱请求预览联动
// （Home 消费：切选中节点 → 打开预览 → 自动开启行归属标注）。
import { ElMessage } from 'element-plus'
import { CheckFileExists } from '../../wailsjs/go/main/App'
import { useUiStore } from '../store/ui'

// blame 不可用扩展名：JSON 系预览是 pretty-print 派生视图（显示行号与文件行号无
// 对应）；markdown/HTML 预览默认渲染视图（v-html/iframe，行尾标注无宿主，联动
// 打开必为渲染视图——htmlViewMode 在 previewFile 中恒重置为 'render'）。
const BLAME_INCAPABLE_EXTS = new Set(['json', 'jsonc', 'json5', 'md', 'markdown', 'html', 'htm'])

const getExt = (name = '') => {
  const idx = name.lastIndexOf('.')
  return idx >= 0 ? name.slice(idx + 1).toLowerCase() : ''
}

/**
 * 文件类型是否支持行归属标注（入口预判，与 FilePreviewRenderer.blameAvailable 同源）。
 * @param {string} fileName 文件名（含扩展名）
 * @returns {boolean}
 */
export function blameCapableExt(fileName) {
  return !BLAME_INCAPABLE_EXTS.has(getExt(fileName))
}

/**
 * 「行归属」入口分流：
 *   文件不在工作树 / 类型不支持 → toast 提示 + 打开文件历史（fallback 闭包，追溯不断链）
 *   可用 → 写 uiStore 追溯信箱（Home 消费联动预览）
 * @param {string} absPath 文件绝对路径
 * @param {string} fileName 文件名（缺省时从 absPath 提取）
 * @param {Function} fallbackOpenHistory 降级打开文件历史弹窗的闭包（调用方自带 initialSha）
 */
export async function requestTraceBlame(absPath, fileName, fallbackOpenHistory) {
  if (!absPath) return
  const name = fileName || absPath.split(/[\\/]/).pop()
  // 存在性预判：绑定异常按不存在处理（fail-safe 永远降级到文件历史，入口恒可达）
  let exists = false
  try {
    exists = await CheckFileExists(absPath)
  } catch {
    exists = false
  }
  if (!exists) {
    ElMessage.info('文件已不在工作树，已转入文件历史追溯')
    fallbackOpenHistory()
    return
  }
  if (!blameCapableExt(name)) {
    ElMessage.info('该文件类型不支持行归属，已转入文件历史追溯')
    fallbackOpenHistory()
    return
  }
  useUiStore().pushTraceBlameRequest(absPath)
}
