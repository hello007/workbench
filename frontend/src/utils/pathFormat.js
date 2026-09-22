// 路径显示格式化工具。
// 从 DirectoryTree 的同名内联函数沉淀为共享 util：AI 对话侧栏目录项等
// 多处需要以短路径展示长绝对路径（中段折叠，保留末两级）。

/**
 * 长路径折叠为短路径：超过 40 字符且层级多于 3 段时，
 * 折叠为 `.../<倒数第二级>/<末级>`（如 D:/a/b/c/project -> .../c/project）。
 * @param {string} path 原始路径（\ 或 / 分隔均可）
 * @returns {string} 短路径（无需折叠时原样返回）
 */
export function shortenPath(path) {
  if (!path || path.length <= 40) return path
  const parts = path.replace(/\\/g, '/').split('/')
  if (parts.length <= 3) return path
  return `.../${parts[parts.length - 2]}/${parts[parts.length - 1]}`
}
