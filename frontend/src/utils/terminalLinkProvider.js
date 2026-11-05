/**
 * 终端文件路径链接检测（Ctrl+Click 打开，xterm registerLinkProvider 消费）。
 *
 * 覆盖三类路径形态：
 * 1. Windows 绝对路径：`D:\work\demo` / `D:/work/demo`
 * 2. Unix 绝对路径与家目录相对：`/usr/local/bin` / `~/work`
 * 3. 仓库相对路径：`./src/main.js` / `../package.json`
 *
 * 设计口径（PRD Q3）：形态正则只决定 hover 链接态——「形态像路径即可点」，
 * 不做存在性预检；点击后 OpenWithDefaultApp 返回 false 时静默（零新增 RPC）。
 *
 * 误判抑制：
 * - 负向 lookbehind `(?<![A-Za-z0-9:/])` 排除 URL（`https://` 的 `://` 与
 *   协议相对 `//host` 的双斜杠都不会被识别为 Unix 路径）
 * - Unix 形态要求首个 `/` 后不是 `/`（`/` 单独成段无意义）
 * - 匹配字符类排除空白与引号/括号：路径在终端输出里通常以空白收尾，
 *   含空格路径（如 `C:\Program Files (x86)\…`）不可匹配属已知取舍（形态
 *   边界无法与自然语言区分，宁可漏选不误选）
 * - 匹配后修剪尾随标点（句号/逗号/冒号等收尾常出现在行文中的路径后）
 */

// 路径形态全局正则（供纯函数与测试共享）
export const TERMINAL_FILE_PATH_PATTERN = /(?<![A-Za-z0-9:/])(?:[A-Za-z]:[\\/][^\s"'()]+|~\/[^\s"'()]+|(?:\.\.?\/)[^\s"'()]+|\/(?!\/)[^\s"'()]+)/g

// 尾随标点修剪：句读符号出现在路径后时从匹配尾部剔除
const TRAILING_PUNCTUATION = /[.,;:!?\]}]+$/

/**
 * 在一行文本中查找全部文件路径形态匹配。
 * @param {string} text 终端行文本（逐 cell 拼接，宽字符占一格文本位）
 * @returns {Array<{text: string, start: number, end: number}>} 文本偏移区间
 *          （start 含 / end 不含，供 provider 换算 buffer 坐标）
 */
export function findFilePathMatches(text) {
  if (!text) return []
  const results = []
  const pattern = new RegExp(TERMINAL_FILE_PATH_PATTERN.source, 'g')
  let m
  while ((m = pattern.exec(text)) !== null) {
    const trimmed = m[0].replace(TRAILING_PUNCTUATION, '')
    if (!trimmed) continue
    // 纯前缀形态（"/"、"D:\"、"~/"、"../"）无实体文件语义，跳过
    if (!hasPathBody(trimmed)) continue
    const start = m.index
    results.push({ text: trimmed, start, end: start + trimmed.length })
    // 从修剪后的尾部继续扫描，防止标点被下一轮匹配重复消费
    pattern.lastIndex = start + trimmed.length
  }
  return results
}

/**
 * 判断匹配串是否含路径主体（前缀之外至少一个字符）：
 * - Windows 形态要求盘符冒号 + 分隔符之后有内容
 * - 其余形态要求前缀（/ ~/ ./ ../）之后有内容
 */
function hasPathBody(p) {
  const winMatch = /^[A-Za-z]:[\\/](.*)$/.exec(p)
  if (winMatch) return winMatch[1].length > 0
  const unixPrefix = /^(?:~\/|\.\.?\/|\/)(.*)$/.exec(p)
  if (unixPrefix) return unixPrefix[1].length > 0
  return p.length > 0
}

/**
 * 将一行 buffer cells 拼接为文本并记录每个文本字符的 buffer x 坐标。
 * 宽字符（CJK/emoji，cell width=2）在文本中占一位、右半格 width=0 跳过，
 * 保证「文本索引 → buffer x」在宽字符混排行上不错位（unicode11 生效前提）。
 * @param {object} line xterm IBufferLine
 * @returns {{text: string, xs: number[]}} 文本与逐字符 buffer x 表
 */
export function collectLineTextWithColumns(line) {
  let text = ''
  const xs = []
  for (let x = 0; x < line.length; x++) {
    const cell = line.getCell(x)
    if (!cell) break
    const width = cell.getWidth()
    if (width === 0) continue // 宽字符右半格：无文本产出
    const chars = cell.getChars() || ' '
    text += chars
    for (let k = 0; k < chars.length; k++) {
      xs.push(x)
    }
  }
  return { text, xs }
}

/**
 * 构造文件路径 link provider（xterm registerLinkProvider 入参形态）。
 * 激活要求 Ctrl（macOS Cmd）修饰键——与 WebLinksAddon 的 Ctrl+Click 语义统一；
 * 未按修饰键的点击静默不动作（单击不再打开任何链接）。
 *
 * @param {object} term xterm Terminal 实例（用 buffer 逐 cell 换算坐标）
 * @param {{ onOpen: (path: string) => void }} handlers 点击回调
 * @returns {{ provideLinks: (lineNumber: number, callback: Function) => void }}
 */
export function createFilePathLinkProvider(term, { onOpen }) {
  return {
    provideLinks(bufferLineNumber, callback) {
      const line = term.buffer.active.getLine(bufferLineNumber - 1)
      if (!line) {
        callback(undefined)
        return
      }
      const { text, xs } = collectLineTextWithColumns(line)
      const matches = findFilePathMatches(text)
      if (matches.length === 0) {
        callback(undefined)
        return
      }
      callback(matches.map(m => ({
        start: { x: xs[m.start], y: bufferLineNumber },
        // xterm ILink 坐标区间为含端（_linkAtPosition 用 x <= end.x 判定）
        end: { x: xs[m.end - 1], y: bufferLineNumber },
        text: m.text,
        activate(event) {
          if (!(event.ctrlKey || event.metaKey)) return
          onOpen(m.text)
        }
      })))
    }
  }
}
