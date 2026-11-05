/**
 * 终端 OSC 9/777/99 通知解析与应用内提示（terminal-multi-session 任务新增）。
 *
 * 终端程序（构建工具/长任务脚本）经转义序列发送桌面通知；终端内嵌面板无
 * 桌面通知面，转 ElNotification 应用内提示。消费面在 useTerminal.initTerminal
 * 经 term.parser.registerOscHandler 注册，总开关读取 settings store 的
 * terminalOscNotifyEnabled（默认 false，handler 内实时判定：关闭时放行，
 * 开启后即时生效，无需重建终端）。
 *
 * 载荷解析（分号切分，仅消费通知形态）：
 * - OSC 9（iTerm2 形态）：`9;<message>`；`9;4;...` 为 Windows Terminal 进度条
 *   子类型，xterm 内部有处理，handler 必须放行（返回 false），不转通知
 * - OSC 777（urxvt 形态）：`777;notify;<title>;<body>`
 * - OSC 99（kitty 形态）：`99;<title>;<body>` 或含元数据段
 *   `99;i=xx:p=1;<title>;<body>`（含 `=` 的首段判为元数据而非标题）
 *
 * 节流：同一终端实例内「相同标题+正文」5s 窗口内只提示一次（长任务脚本
 * 循环发同内容通知防刷屏），窗口过期自动清理键值防 Map 无界增长。
 */
import { ElNotification } from 'element-plus'

/** 同 tab 同内容通知节流窗口（毫秒） */
export const OSC_NOTIFY_THROTTLE_MS = 5000

/**
 * 通知标题/正文单字段长度上限：超出截断并追加省略号——OSC 载荷无长度约束
 * （极端场景单条可达数 MB，如程序误把整段日志塞进 OSC 9），整段进
 * ElNotification 会撑爆通知 UI；节流键同用截断后文本，保证判定一致
 */
export const OSC_NOTIFY_TEXT_MAX = 500

// 截断省略号
const ELLIPSIS = '…'

/** 超长文本截断：保留前 OSC_NOTIFY_TEXT_MAX 字符 + 省略号 */
function truncateNotifyText(text) {
  if (text.length <= OSC_NOTIFY_TEXT_MAX) return text
  return text.slice(0, OSC_NOTIFY_TEXT_MAX) + ELLIPSIS
}

// 节流表上限：超过即先清理过期键值（极端刷屏场景防内存无界增长）
const THROTTLE_MAP_MAX = 200

/**
 * 解析 OSC 通知载荷为 { title, body }；非通知形态返回 null。
 * @param {number} ident OSC 序号（9 / 777 / 99）
 * @param {string} data OSC 参数串（不含终止符）
 * @returns {{title: string, body: string} | null} null = 非通知形态（调用方放行）
 */
export function parseOscNotify(ident, data) {
  if (!data) return null
  if (ident === 9) {
    // Windows Terminal 进度条子类型（9;4;state;progress）：放行给 xterm 内部处理
    if (data.startsWith('4;') || data === '4') return null
    return { title: '', body: data }
  }
  if (ident === 777) {
    // 777;notify;<title>;<body>：首段须为 notify 才是通知形态，其余放行
    const parts = data.split(';')
    if (parts[0] !== 'notify') return null
    return { title: parts[1] || '', body: parts.slice(2).join(';') }
  }
  if (ident === 99) {
    const parts = data.split(';')
    if (parts.length === 1) return { title: '', body: parts[0] }
    // kitty 元数据段（如 i=xxx、p=1）含 '='，不是标题
    let idx = 0
    if (parts[0].includes('=')) idx = 1
    return { title: parts[idx] || '', body: parts.slice(idx + 1).join(';') }
  }
  return null
}

/**
 * 构造 OSC 通知处理器（每终端实例一份，节流状态实例隔离）。
 *
 * @param {object} options
 * @param {() => boolean} options.isEnabled 总开关实时读取（settings store）
 * @param {() => string} [options.getTitle] 通知标题来源（tab shell 名），
 *        空串回退「终端通知」
 * @param {number} [options.throttleMs] 节流窗口（默认 5000，测试注入缩短）
 * @param {Function} [options.notify] 提示出口（默认 ElNotification，测试注入捕获）
 * @returns {{ handle: (ident: number, data: string) => boolean, dispose: () => void }}
 *          handle 返回 true = 已消费（xterm 停止分发）；false = 放行
 */
export function createOscNotifier({ isEnabled, getTitle, throttleMs = OSC_NOTIFY_THROTTLE_MS, notify } = {}) {
  const emit = notify || ((opts) => ElNotification(opts))
  const lastNotifiedAt = new Map()
  const now = () => Date.now()

  function pruneExpired() {
    const ts = now()
    for (const [key, at] of lastNotifiedAt) {
      if (ts - at >= throttleMs) {
        lastNotifiedAt.delete(key)
      }
    }
  }

  function handle(ident, data) {
    // 总开关关闭：不消费（放行），已开启后即时生效无需重建终端
    if (!isEnabled || !isEnabled()) return false
    const payload = parseOscNotify(ident, data)
    if (!payload) return false
    // 超长载荷截断（500 字符 + 省略号）：防超大 OSC 载荷整段撑爆通知 UI；
    // 节流键同用截断后文本，同内容判定与展示口径一致
    const body = truncateNotifyText(payload.body.trim())
    if (!body) return false
    const title = truncateNotifyText((getTitle ? getTitle() : '') || payload.title || '终端通知')
    // 节流：同实例同「标题|正文」窗口内只提示一次
    const key = `${title}|${body}`
    const last = lastNotifiedAt.get(key)
    const ts = now()
    if (last !== undefined && ts - last < throttleMs) return true
    if (lastNotifiedAt.size >= THROTTLE_MAP_MAX) {
      pruneExpired()
    }
    lastNotifiedAt.set(key, ts)
    emit({ title, message: body, type: 'info', duration: 5000 })
    return true
  }

  function dispose() {
    lastNotifiedAt.clear()
  }

  return { handle, dispose }
}
