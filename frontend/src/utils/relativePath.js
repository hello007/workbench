// 相对路径计算工具。
// 文件树右键「复制路径」二级菜单与内容面板路径下拉共用：
// 复制相对路径（相对当前工作目录根）与文件引用路径（@ + 相对路径，AI 工具文件引用格式）。
// 分隔符统一 '/'：Windows 反斜杠先归一，与既有 copyPath 的 replaceAll 规范化口径一致。

/**
 * 将节点绝对路径转换为相对 rootPath 的路径（'/' 分隔）。
 * @param {string} nodePath 节点绝对路径（\ 或 / 分隔均可）
 * @param {string} rootPath 工作目录根绝对路径（\ 或 / 分隔均可）
 * @returns {string} 相对路径；节点即根时返回 '.'；入参为空或节点不在根下时返回归一化绝对路径（防御分支）
 */
export function toRelativePath(nodePath, rootPath) {
  const normalize = (p) => (p || '').replace(/\\/g, '/')
  const node = normalize(nodePath)
  const root = normalize(rootPath)
  if (!node || !root) return node
  if (node === root) return '.'
  const prefix = root.endsWith('/') ? root : root + '/'
  if (!node.startsWith(prefix)) return node
  return node.slice(prefix.length)
}

/**
 * 文件引用路径：相对路径前加 @（AI 工具引用文件的标准前缀）。
 * @param {string} nodePath 节点绝对路径
 * @param {string} rootPath 工作目录根绝对路径
 * @returns {string} '@' + 相对路径
 */
export function toFileRefPath(nodePath, rootPath) {
  return '@' + toRelativePath(nodePath, rootPath)
}
