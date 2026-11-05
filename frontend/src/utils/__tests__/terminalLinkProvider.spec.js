/**
 * 终端文件路径链接检测测试
 * 覆盖：三类路径形态（Windows 绝对 / Unix 绝对与 ~ / 仓库相对）、URL 误判抑制、
 * 尾随标点修剪、纯前缀过滤、宽字符行坐标映射、provider 坐标换算与 Ctrl+Click 语义
 */
import { describe, it, expect, vi } from 'vitest'
import {
  findFilePathMatches,
  collectLineTextWithColumns,
  createFilePathLinkProvider
} from '../terminalLinkProvider'

describe('findFilePathMatches - 路径形态识别', () => {
  it('Windows 绝对路径（正反斜杠均识别）', () => {
    expect(findFilePathMatches('see D:\\work\\demo end')).toEqual([
      { text: 'D:\\work\\demo', start: 4, end: 16 }
    ])
    expect(findFilePathMatches('dir D:/work/demo ok')).toEqual([
      { text: 'D:/work/demo', start: 4, end: 16 }
    ])
  })

  it('Unix 绝对路径', () => {
    expect(findFilePathMatches('log at /var/log/syslog.')).toEqual([
      { text: '/var/log/syslog', start: 7, end: 22 }
    ])
  })

  it('家目录相对（~/）与仓库相对（./ ../）', () => {
    expect(findFilePathMatches('cd ~/work then')).toEqual([
      { text: '~/work', start: 3, end: 9 }
    ])
    expect(findFilePathMatches('see ./src/main.js here')).toEqual([
      { text: './src/main.js', start: 4, end: 17 }
    ])
    expect(findFilePathMatches('up ../package.json end')).toEqual([
      { text: '../package.json', start: 3, end: 18 }
    ])
  })

  it('URL 不误判（https:// 与协议相对 //host 均不识别为路径）', () => {
    expect(findFilePathMatches('open https://example.com/path now')).toEqual([])
    expect(findFilePathMatches('see http://localhost:3000/x end')).toEqual([])
    expect(findFilePathMatches('url //example.com/foo end')).toEqual([])
  })

  it('尾随标点修剪（句号/逗号/冒号收尾不入路径）', () => {
    expect(findFilePathMatches('built /usr/local/bin.')).toEqual([
      { text: '/usr/local/bin', start: 6, end: 20 }
    ])
    expect(findFilePathMatches('at D:\\work\\a.txt, done')).toEqual([
      { text: 'D:\\work\\a.txt', start: 3, end: 16 }
    ])
  })

  it('纯前缀形态跳过（/ 、D:\\ 、~/ 单独出现无实体语义）', () => {
    expect(findFilePathMatches('root is / of all')).toEqual([])
    expect(findFilePathMatches('drive D:\\ empty')).toEqual([])
    expect(findFilePathMatches('home ~/ alone')).toEqual([])
  })

  it('一行多路径全部识别', () => {
    const matches = findFilePathMatches('diff /etc/a.conf /etc/b.conf ok')
    expect(matches).toHaveLength(2)
    expect(matches[0].text).toBe('/etc/a.conf')
    expect(matches[1].text).toBe('/etc/b.conf')
  })

  it('括号包裹的路径（尾巴括号不入匹配）', () => {
    expect(findFilePathMatches('(see /tmp/build.log) end')).toEqual([
      { text: '/tmp/build.log', start: 5, end: 19 }
    ])
  })

  it('空文本与无匹配安全返回空数组', () => {
    expect(findFilePathMatches('')).toEqual([])
    expect(findFilePathMatches('no path here')).toEqual([])
  })

  it('带扩展名文件名不被误剪（file.go / a.txt 的 . 在词中间）', () => {
    const matches = findFilePathMatches('edit /usr/bin/file.go now')
    expect(matches[0].text).toBe('/usr/bin/file.go')
  })
})

describe('collectLineTextWithColumns - 宽字符行坐标映射', () => {
  /** 构造 xterm IBufferLine 形态的最小桩（逐 cell width/chars） */
  function makeLine(cells) {
    return {
      length: cells.length,
      getCell(x) {
        const c = cells[x]
        if (!c) return null
        return {
          getWidth: () => c.w,
          getChars: () => c.ch
        }
      }
    }
  }

  it('ASCII 行：文本索引与 buffer x 一致', () => {
    const { text, xs } = collectLineTextWithColumns(makeLine([
      { w: 1, ch: 'a' }, { w: 1, ch: 'b' }, { w: 1, ch: ' ' }, { w: 1, ch: 'c' }
    ]))
    expect(text).toBe('ab c')
    expect(xs).toEqual([0, 1, 2, 3])
  })

  it('CJK 宽字符行：文本占一位、buffer x 跳跃不错位', () => {
    // '中' 宽字符 width=2 占 x=0/1 两格（右半 width=0 无文本），'a' 在 x=2
    const { text, xs } = collectLineTextWithColumns(makeLine([
      { w: 2, ch: '中' }, { w: 0, ch: '' }, { w: 1, ch: 'a' }, { w: 1, ch: 'b' }
    ]))
    expect(text).toBe('中ab')
    expect(xs).toEqual([0, 2, 3])
  })

  it('空白 cell 产出空格占位（路径以空白为界的前提）', () => {
    const { text } = collectLineTextWithColumns(makeLine([
      { w: 1, ch: 'x' }, { w: 1, ch: '' }, { w: 1, ch: 'y' }
    ]))
    expect(text).toBe('x y')
  })
})

describe('createFilePathLinkProvider - provider 行为', () => {
  function makeTerm(lineText, cells) {
    return {
      buffer: {
        active: {
          getLine(no) {
            if (no !== 0) return null
            return {
              length: cells.length,
              getCell(x) {
                const c = cells[x]
                if (!c) return null
                return { getWidth: () => c.w, getChars: () => c.ch }
              }
            }
          }
        }
      },
      __lineText: lineText
    }
  }

  it('provideLinks 命中路径时回调链接区间与文本（buffer 坐标含端）', () => {
    // 行：'at D:\work\a.txt end'（ASCII，文本索引=buffer x）
    const cells = 'at D:\\work\\a.txt end'.split('').map(ch => ({ w: 1, ch }))
    const term = makeTerm('at D:\\work\\a.txt end', cells)
    const onOpen = vi.fn()
    const provider = createFilePathLinkProvider(term, { onOpen })
    const cb = vi.fn()
    provider.provideLinks(1, cb)
    expect(cb).toHaveBeenCalledTimes(1)
    const links = cb.mock.calls[0][0]
    expect(links).toHaveLength(1)
    expect(links[0].text).toBe('D:\\work\\a.txt')
    expect(links[0].start).toEqual({ x: 3, y: 1 })
    expect(links[0].end).toEqual({ x: 15, y: 1 }) // 含端：start + len - 1
  })

  it('无路径行回调 undefined（不产出链接）', () => {
    const cells = 'plain text only'.split('').map(ch => ({ w: 1, ch }))
    const provider = createFilePathLinkProvider(makeTerm('plain text only', cells), { onOpen: vi.fn() })
    const cb = vi.fn()
    provider.provideLinks(1, cb)
    expect(cb).toHaveBeenCalledWith(undefined)
  })

  it('行号越界（无该行）回调 undefined', () => {
    const provider = createFilePathLinkProvider(makeTerm('', []), { onOpen: vi.fn() })
    const cb = vi.fn()
    provider.provideLinks(99, cb)
    expect(cb).toHaveBeenCalledWith(undefined)
  })

  it('activate 仅在 Ctrl（macOS Cmd）点击时打开，单击静默', () => {
    const cells = 'see ./src/a.js'.split('').map(ch => ({ w: 1, ch }))
    const onOpen = vi.fn()
    const provider = createFilePathLinkProvider(makeTerm('see ./src/a.js', cells), { onOpen })
    const cb = vi.fn()
    provider.provideLinks(1, cb)
    const link = cb.mock.calls[0][0][0]
    link.activate({ ctrlKey: true, metaKey: false })
    expect(onOpen).toHaveBeenCalledWith('./src/a.js')
    link.activate({ ctrlKey: false, metaKey: true }) // macOS Cmd
    expect(onOpen).toHaveBeenCalledTimes(2)
    onOpen.mockClear()
    link.activate({ ctrlKey: false, metaKey: false }) // 单击：静默不开
    expect(onOpen).not.toHaveBeenCalled()
  })

  it('CJK 宽字符前置行：路径坐标换算不不错位', () => {
    // 行：'中ab see /tmp/x' —— '中' 占 x=0/1，'a' x=2，'b' x=3，空格 x=4，
    // 's' x=5 … 路径 '/tmp/x' 从文本索引 7 起，对应 buffer x=9
    const cellsDef = [
      { w: 2, ch: '中' }, { w: 0, ch: '' }, { w: 1, ch: 'a' }, { w: 1, ch: 'b' },
      { w: 1, ch: ' ' }, { w: 1, ch: 's' }, { w: 1, ch: 'e' }, { w: 1, ch: 'e' },
      { w: 1, ch: ' ' }, { w: 1, ch: '/' }, { w: 1, ch: 't' }, { w: 1, ch: 'm' },
      { w: 1, ch: 'p' }, { w: 1, ch: '/' }, { w: 1, ch: 'x' }
    ]
    const onOpen = vi.fn()
    const provider = createFilePathLinkProvider(makeTerm('中ab see /tmp/x', cellsDef), { onOpen })
    const cb = vi.fn()
    provider.provideLinks(1, cb)
    const link = cb.mock.calls[0][0][0]
    expect(link.text).toBe('/tmp/x')
    expect(link.start.x).toBe(9)
    expect(link.end.x).toBe(14)
    link.activate({ ctrlKey: true, metaKey: false })
    expect(onOpen).toHaveBeenCalledWith('/tmp/x')
  })
})
