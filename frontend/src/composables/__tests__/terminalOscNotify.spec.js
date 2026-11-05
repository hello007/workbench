/**
 * terminalOscNotify 纯函数与节流测试
 * 覆盖：OSC 9/777/99 载荷解析（含 9;4 进度条放行与 kitty 元数据段）、
 * 超长载荷截断、总开关实时判定、同内容节流窗口、过期键清理防 Map 无界增长
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { parseOscNotify, createOscNotifier, OSC_NOTIFY_THROTTLE_MS, OSC_NOTIFY_TEXT_MAX } from '../terminalOscNotify'

describe('parseOscNotify - 载荷解析', () => {
  it('OSC 9 通知形态：整段为 message，title 为空', () => {
    expect(parseOscNotify(9, 'build done')).toEqual({ title: '', body: 'build done' })
  })

  it('OSC 9;4 进度条子类型返回 null（放行给 xterm 内部处理）', () => {
    expect(parseOscNotify(9, '4;1;50')).toBeNull()
    expect(parseOscNotify(9, '4')).toBeNull()
  })

  it('OSC 777 通知形态：notify;title;body 三段解析', () => {
    expect(parseOscNotify(777, 'notify;构建任务;编译完成')).toEqual({ title: '构建任务', body: '编译完成' })
  })

  it('OSC 777 非 notify 子类型返回 null（放行）', () => {
    expect(parseOscNotify(777, 'other;foo')).toBeNull()
  })

  it('OSC 777 body 含分号时回拼（只切 title 边界）', () => {
    expect(parseOscNotify(777, 'notify;t;a;b')).toEqual({ title: 't', body: 'a;b' })
  })

  it('OSC 99 单段形态：无 title', () => {
    expect(parseOscNotify(99, '部署完成')).toEqual({ title: '', body: '部署完成' })
  })

  it('OSC 99 双段形态：title;body', () => {
    expect(parseOscNotify(99, '部署完成;artifact.zip')).toEqual({ title: '部署完成', body: 'artifact.zip' })
  })

  it('OSC 99 kitty 元数据段（首段含 =）不作为 title', () => {
    expect(parseOscNotify(99, 'i=1:p=1;部署完成;artifact.zip')).toEqual({ title: '部署完成', body: 'artifact.zip' })
  })

  it('空载荷与非通知序号返回 null', () => {
    expect(parseOscNotify(9, '')).toBeNull()
    expect(parseOscNotify(0, 'x')).toBeNull()
    expect(parseOscNotify(1337, 'x')).toBeNull()
  })
})

describe('createOscNotifier - 开关与节流', () => {
  let notify

  beforeEach(() => {
    notify = vi.fn()
  })

  it('开关关闭时放行（返回 false）且不提示', () => {
    const n = createOscNotifier({ isEnabled: () => false, notify })
    expect(n.handle(9, 'hello')).toBe(false)
    expect(notify).not.toHaveBeenCalled()
  })

  it('开关开启后消费（返回 true）并转提示，title 回退「终端通知」', () => {
    const n = createOscNotifier({ isEnabled: () => true, notify })
    expect(n.handle(9, 'hello')).toBe(true)
    expect(notify).toHaveBeenCalledWith({ title: '终端通知', message: 'hello', type: 'info', duration: 5000 })
  })

  it('getTitle 提供的标题优先于载荷 title', () => {
    const n = createOscNotifier({
      isEnabled: () => true,
      getTitle: () => 'PowerShell',
      notify
    })
    n.handle(777, 'notify;脚本;完成')
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ title: 'PowerShell' }))
  })

  it('空 body 不消费（放行，不弹空通知）', () => {
    const n = createOscNotifier({ isEnabled: () => true, notify })
    expect(n.handle(9, '   ')).toBe(false)
    expect(notify).not.toHaveBeenCalled()
  })

  it('超长载荷截断：body 超上限时保留前 N 字符 + 省略号（防撑爆通知 UI）', () => {
    const n = createOscNotifier({ isEnabled: () => true, notify })
    const huge = 'x'.repeat(5000)
    expect(n.handle(9, huge)).toBe(true)
    expect(notify).toHaveBeenCalledTimes(1)
    const opts = notify.mock.calls[0][0]
    expect(opts.message).toBe('x'.repeat(OSC_NOTIFY_TEXT_MAX) + '…')
    expect(opts.message.length).toBe(OSC_NOTIFY_TEXT_MAX + 1)
  })

  it('超长载荷截断作用于节流键：同超长内容窗口内仍只提示一次', () => {
    vi.useFakeTimers()
    const n = createOscNotifier({ isEnabled: () => true, notify })
    const huge = 'y'.repeat(3000)
    n.handle(9, huge)
    n.handle(9, huge)
    expect(notify).toHaveBeenCalledTimes(1)
    vi.useRealTimers()
  })

  it('长度恰在上限内不截断（边界无损）', () => {
    const n = createOscNotifier({ isEnabled: () => true, notify })
    const exact = 'z'.repeat(OSC_NOTIFY_TEXT_MAX)
    n.handle(9, exact)
    expect(notify.mock.calls[0][0].message).toBe(exact)
  })

  it('同实例同内容节流窗口内只提示一次（默认 5s）', () => {
    vi.useFakeTimers()
    const n = createOscNotifier({ isEnabled: () => true, notify })
    n.handle(9, 'same')
    vi.advanceTimersByTime(OSC_NOTIFY_THROTTLE_MS - 1000)
    n.handle(9, 'same')
    expect(notify).toHaveBeenCalledTimes(1)
    // 窗口过期后可再次提示
    vi.advanceTimersByTime(1100)
    n.handle(9, 'same')
    expect(notify).toHaveBeenCalledTimes(2)
    vi.useRealTimers()
  })

  it('不同内容不互相节流', () => {
    const n = createOscNotifier({ isEnabled: () => true, notify })
    n.handle(9, 'a')
    n.handle(9, 'b')
    expect(notify).toHaveBeenCalledTimes(2)
  })

  it('节流状态实例隔离（每终端实例独立）', () => {
    const n1 = createOscNotifier({ isEnabled: () => true, notify })
    const n2 = createOscNotifier({ isEnabled: () => true, notify })
    n1.handle(9, 'x')
    n2.handle(9, 'x')
    expect(notify).toHaveBeenCalledTimes(2)
  })

  it('节流表超上限时清理过期键值（防 Map 无界增长）', () => {
    vi.useFakeTimers()
    const n = createOscNotifier({ isEnabled: () => true, notify })
    // 填满 200 个不重复键（上限内全部提示）
    for (let i = 0; i < 200; i++) {
      n.handle(9, `msg-${i}`)
    }
    expect(notify).toHaveBeenCalledTimes(200)
    // 时间推进后第 201 个触发过期清理，旧键被移除后新内容可提示
    vi.advanceTimersByTime(OSC_NOTIFY_THROTTLE_MS + 1)
    n.handle(9, 'msg-late')
    expect(notify).toHaveBeenCalledTimes(201)
    vi.useRealTimers()
  })

  it('dispose 清空节流表：释放后同内容可重新提示', () => {
    const n = createOscNotifier({ isEnabled: () => true, notify })
    n.handle(9, 'x')
    n.handle(9, 'x')
    expect(notify).toHaveBeenCalledTimes(1)
    n.dispose()
    n.handle(9, 'x')
    expect(notify).toHaveBeenCalledTimes(2)
  })
})
