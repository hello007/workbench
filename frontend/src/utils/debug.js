/**
 * 调试日志工具
 * 在开发环境输出调试信息，生产环境静默
 */

const isDevelopment = import.meta.env?.DEV ?? process.env.NODE_ENV !== 'production'

export const debug = {
  log: (...args) => {
    if (isDevelopment) {
      console.log('[DEBUG]', ...args)
    }
  },
  error: (...args) => {
    // 错误日志始终输出
    console.error('[ERROR]', ...args)
  },
  warn: (...args) => {
    if (isDevelopment) {
      console.warn('[WARN]', ...args)
    }
  }
}
