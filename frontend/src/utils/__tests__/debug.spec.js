import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { debug } from '../debug'

describe('debug', () => {
  let logSpy, errorSpy, warnSpy

  beforeEach(() => {
    logSpy = vi.spyOn(console, 'log').mockImplementation(() => {})
    errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('error 始终输出', () => {
    debug.error('err')
    expect(errorSpy).toHaveBeenCalledWith('[ERROR]', 'err')
  })

  it('log 在开发环境输出带前缀', () => {
    debug.log('msg')
    expect(logSpy).toHaveBeenCalledWith('[DEBUG]', 'msg')
  })

  it('warn 在开发环境输出带前缀', () => {
    debug.warn('w')
    expect(warnSpy).toHaveBeenCalledWith('[WARN]', 'w')
  })

  it('log 支持多参数', () => {
    debug.log('a', 'b', 1, { k: 'v' })
    expect(logSpy).toHaveBeenCalledWith('[DEBUG]', 'a', 'b', 1, { k: 'v' })
  })

  it('error 无参数也调用', () => {
    debug.error()
    expect(errorSpy).toHaveBeenCalled()
  })
})
