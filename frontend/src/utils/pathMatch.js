// 路径前缀匹配工具。
// Windows 文件系统不区分大小写，后端 filepath.Abs 规范化路径主键但不改大小写，
// 故前端比较须先统一分隔符（\ -> /）再 toLowerCase，规避 startsWith 大小写敏感导致静默失败。
// 详见 research/cross-workdir-locate.md「路径匹配分析」。

/**
 * 规范化路径：分隔符统一为 / + 转小写。用于工作目录归属判定等大小写不敏感比较。
 * @param {string} p 原始路径
 * @returns {string} 规范化后的路径（/ 分隔、小写）
 */
export function normalizePath(p) {
  return (p || '').replace(/\\/g, '/').toLowerCase()
}

/**
 * 判断 repoPath 是否属于某工作目录（路径前缀匹配，大小写不敏感）。
 * 匹配规则：规范化后 repoPath 以 dir.path 为前缀，且紧跟分隔符或完全相等，
 * 避免 `D:\projects` 误匹配 `D:\projects-other` 这类同前缀串。
 * @param {string} repoPath 仓库绝对路径
 * @param {string} dirPath 工作目录绝对路径
 * @returns {boolean}
 */
export function belongsToDir(repoPath, dirPath) {
  const normRepo = normalizePath(repoPath)
  const normDir = normalizePath(dirPath)
  if (!normDir || !normRepo.startsWith(normDir)) return false
  if (normRepo === normDir) return true
  // normDir 以分隔符结尾（如盘根 d:/ 或带尾分隔符路径）时 startsWith 已保证边界；
  // 否则前缀后须紧跟分隔符，排除 projects vs projects-other 同前缀串
  if (normDir.endsWith('/')) return true
  return normRepo[normDir.length] === '/'
}

/**
 * 在工作目录列表中找到 repoPath 所属的最具体工作目录（最长前缀匹配）。
 * 嵌套工作目录场景（如 D:\projects 与 D:\projects\sub 均已添加，仓库在 D:\projects\sub\repo），
 * 取路径最长者 = 最贴近仓库的工作目录，避免选中外层导致 locateNode 在错误树里定位。
 * @param {string} repoPath 仓库绝对路径
 * @param {Array<{path: string}>} directories 工作目录列表（每项含 path 字段）
 * @returns {object|null} 匹配的工作目录对象，无匹配返回 null
 */
export function findOwningDirectory(repoPath, directories) {
  if (!repoPath || !directories || directories.length === 0) return null
  const matched = directories.filter(d => belongsToDir(repoPath, d.path))
  if (matched.length === 0) return null
  // 最长前缀匹配：path 最长者最具体
  return matched.reduce((best, cur) =>
    cur.path.length > best.path.length ? cur : best
  )
}

/**
 * 仓库根绝对路径 + 仓库相对路径 拼接为文件绝对路径（保留原大小写，不转小写）。
 * 供提交历史/文件 diff 的追溯入口换算 FileHistoryDialog / blame 所需的 fileAbsPath：
 * repoPath 来自工作目录节点（Windows 下可能 \ 分隔、可能带尾分隔符），relPath 来自
 * git 输出（恒 / 分隔）。任一入参为空返回另一参原值（防御空拼接出非法路径）。
 * @param {string} repoPath 仓库根绝对路径
 * @param {string} relPath 仓库相对路径（/ 分隔）
 * @returns {string} 文件绝对路径
 */
export function joinRepoPath(repoPath, relPath) {
  if (!repoPath) return relPath || ''
  if (!relPath) return repoPath
  const base = repoPath.replace(/[\\/]+$/, '')
  return `${base}/${relPath.replace(/^[\\/]+/, '')}`
}
