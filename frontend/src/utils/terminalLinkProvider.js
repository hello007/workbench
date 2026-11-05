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
 * - 匹配字符类排除空白、引号/括号（半角+全角）与全角句读：路径在终端输出里
 *   通常以空白收尾，含空格路径（如 `C:\Program Files (x86)\…`）不可匹配属
 *   已知取舍（形态边界无法与自然语言区分，宁可漏选不误选）；全角标点入集
 *   是中文行文场景——「（见 /tmp/build.log。）」「日志 /var/app.log。然后退出」
 *   中全角符号若不终止匹配会把整句汉字吞进路径
 * - 匹配后修剪尾随标点（半角句读与全角句读收尾常出现在行文中的路径后；
 *   半角句读不入终止字符类是因扩展名 `file.go` 的 `.` 与盘符 `D:` 的 `:`
 *   须可出现在路径主体，只能在尾随位修剪）
 */

// 路径主体字符类（否定集，四个形态分支共享）：路径主体不含以下字符——
// - 空白（\s 已含全角空格 \u3000）
// - 引号：半角 "'、全角弯引号 “”‘’、直角引号「」『』
// - 括号类：半角 ()、全角（）、方头括号【】、书名号《》〈〉
// - 全角句读：。，；：！？、（中文行文句读后常直接跟汉字，不终止会把整句
//   吞进匹配；半角句读不在此列，理由见上）
const PATH_BODY_CLASS = `[^\\s"'“”‘’()（）【】「」『』《》〈〉。，；：！？、]`

// 路径形态全局正则（供纯函数与测试共享；由共享字符类拼接构造，
// `${PATH_BODY_CLASS}+` 量词不可漏——缺 + 时每分支只吃一个正文字符）
export const TERMINAL_FILE_PATH_PATTERN = new RegExp(
  `(?<![A-Za-z0-9:/])(?:[A-Za-z]:[\\\\/]${PATH_BODY_CLASS}+|~/${PATH_BODY_CLASS}+|(?:\\.\\.?/)${PATH_BODY_CLASS}+|/(?!/)${PATH_BODY_CLASS}+)`,
  'g'
)

// 尾随标点修剪：句读/闭括号出现在匹配尾部时剔除（行文中路径后常接句读）。
// 半角：., ; : ! ? 与闭方括号/闭花括号；全角：句读 。，；：！？、 与间隔号 ·
// （全角句读已在终止字符类，此处保留为防御——若匹配串内仍出现仍能收口）；
// 只收闭符号与句读，开括号/开引号（（【「『《）不是路径结束标志，不修剪
const TRAILING_PUNCTUATION = /[.,;:!?\]}。，；：！？、·]+$/

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
