import { describe, it, expect } from 'vitest'
import {
  parseConflictMarkers,
  mergeConflictResult,
  containsConflictMarkers
} from '../conflictMarkers'

describe('conflictMarkers 纯函数', () => {
  describe('parseConflictMarkers', () => {
    it('简单冲突块：before/ours/theirs/after 正确分区', () => {
      const text = [
        'line-1',
        'line-2',
        '<<<<<<< HEAD',
        'ours-a',
        '=======',
        'theirs-a',
        '>>>>>>> feature',
        'line-3'
      ].join('\n')

      const parsed = parseConflictMarkers(text)

      expect(parsed.hasMarkers).toBe(true)
      expect(parsed.before).toEqual(['line-1', 'line-2'])
      expect(parsed.after).toEqual(['line-3'])
      expect(parsed.blocks).toHaveLength(1)
      expect(parsed.blocks[0].ours).toEqual(['ours-a'])
      expect(parsed.blocks[0].theirs).toEqual(['theirs-a'])
      expect(parsed.blocks[0].resolved).toBeNull()
    })

    it('多个冲突块依次解析，块间内容挂到后块 lead 保持原位', () => {
      const text = [
        '<<<<<<< HEAD',
        'o1',
        '=======',
        't1',
        '>>>>>>> b',
        'mid',
        '<<<<<<< HEAD',
        'o2',
        '=======',
        't2',
        '>>>>>>> b'
      ].join('\n')

      const parsed = parseConflictMarkers(text)

      expect(parsed.blocks).toHaveLength(2)
      expect(parsed.blocks[1].lead).toEqual(['mid'])
      expect(parsed.blocks[1].ours).toEqual(['o2'])
      expect(parsed.after).toEqual([])
    })

    it('diff3 base 段被解析进 base，不进 ours/theirs', () => {
      const text = [
        '<<<<<<< HEAD',
        'ours',
        '||||||| merged common',
        'base',
        '=======',
        'theirs',
        '>>>>>>> b'
      ].join('\n')

      const parsed = parseConflictMarkers(text)

      expect(parsed.blocks[0].base).toEqual(['base'])
      expect(parsed.blocks[0].ours).toEqual(['ours'])
      expect(parsed.blocks[0].theirs).toEqual(['theirs'])
    })

    it('无标记文件：hasMarkers false，全文进 before', () => {
      const parsed = parseConflictMarkers('a\nb\nc')

      expect(parsed.hasMarkers).toBe(false)
      expect(parsed.blocks).toHaveLength(0)
      expect(parsed.before).toEqual(['a', 'b', 'c'])
    })

    it('标记不在列 0 时被忽略（普通行）', () => {
      const text = '  <<<<<<< HEAD\nindented\n  >>>>>>> b'

      const parsed = parseConflictMarkers(text)

      expect(parsed.hasMarkers).toBe(false)
      expect(parsed.before).toEqual(['  <<<<<<< HEAD', 'indented', '  >>>>>>> b'])
    })

    it('游离分隔符（无 <<<<<<< 包裹）按普通行处理', () => {
      const text = 'a\n=======\nb'

      const parsed = parseConflictMarkers(text)

      expect(parsed.hasMarkers).toBe(false)
      expect(parsed.before).toEqual(['a', '=======', 'b'])
    })

    it('EOF 未闭合块：防御性按块内容收尾', () => {
      const text = ['pre', '<<<<<<< HEAD', 'ours-open'].join('\n')

      const parsed = parseConflictMarkers(text)

      expect(parsed.hasMarkers).toBe(true)
      expect(parsed.before).toEqual(['pre'])
      expect(parsed.blocks).toHaveLength(1)
      expect(parsed.blocks[0].ours).toEqual(['ours-open'])
      expect(parsed.blocks[0].theirs).toEqual([])
    })
  })

  describe('mergeConflictResult', () => {
    const sample = [
      'pre',
      '<<<<<<< HEAD',
      'ours',
      '=======',
      'theirs',
      '>>>>>>> b',
      'post'
    ].join('\n')

    it('未决策块：ours+theirs 依次拼接（base 排除）', () => {
      const parsed = parseConflictMarkers(sample)
      const result = mergeConflictResult(parsed)

      expect(result).toBe('pre\nours\ntheirs\npost')
    })

    it('决策 ours：块内容替换为 ours 行', () => {
      const parsed = parseConflictMarkers(sample)
      parsed.blocks[0].resolved = { kind: 'ours', lines: parsed.blocks[0].ours }
      expect(mergeConflictResult(parsed)).toBe('pre\nours\npost')
    })

    it('决策 theirs：块内容替换为 theirs 行', () => {
      const parsed = parseConflictMarkers(sample)
      parsed.blocks[0].resolved = { kind: 'theirs', lines: parsed.blocks[0].theirs }
      expect(mergeConflictResult(parsed)).toBe('pre\ntheirs\npost')
    })

    it('决策 both：两侧都保留', () => {
      const parsed = parseConflictMarkers(sample)
      parsed.blocks[0].resolved = {
        kind: 'both',
        lines: [...parsed.blocks[0].ours, ...parsed.blocks[0].theirs]
      }
      expect(mergeConflictResult(parsed)).toBe('pre\nours\ntheirs\npost')
    })

    it('决策 custom：手动编辑行生效', () => {
      const parsed = parseConflictMarkers(sample)
      parsed.blocks[0].resolved = { kind: 'custom', lines: ['手写行'] }
      expect(mergeConflictResult(parsed)).toBe('pre\n手写行\npost')
    })

    it('diff3 base 段默认不参与合并结果', () => {
      const text = [
        '<<<<<<< HEAD',
        'ours',
        '||||||| base',
        'base-line',
        '=======',
        'theirs',
        '>>>>>>> b'
      ].join('\n')
      const parsed = parseConflictMarkers(text)
      expect(mergeConflictResult(parsed)).toBe('ours\ntheirs')
    })

    it('多块合并保持块间上下文原位', () => {
      const text = [
        'pre',
        '<<<<<<< HEAD',
        'o1',
        '=======',
        't1',
        '>>>>>>> b',
        'mid',
        '<<<<<<< HEAD',
        'o2',
        '=======',
        't2',
        '>>>>>>> b',
        'post'
      ].join('\n')
      const parsed = parseConflictMarkers(text)
      parsed.blocks[0].resolved = { kind: 'ours', lines: ['o1'] }
      expect(mergeConflictResult(parsed)).toBe('pre\no1\nmid\no2\nt2\npost')
    })

    it('混合已决策/未决策块：已决策替换、未决策保留 ours+theirs', () => {
      const text = 'pre\n<<<<<<< A\no1\n=======\nt1\n>>>>>>> a\nmid\n<<<<<<< B\no2\n=======\nt2\n>>>>>>> b\npost'
      const parsed = parseConflictMarkers(text)
      parsed.blocks[0].resolved = { kind: 'theirs', lines: [...parsed.blocks[0].theirs] }
      // blocks[1] 不决策
      expect(mergeConflictResult(parsed)).toBe('pre\nt1\nmid\no2\nt2\npost')
    })
  })

  describe('CRLF 行尾保持（Windows 工作区文件回归）', () => {
    it('CRLF 全文解析→决策→合并逐字节保持行尾（标记行替换、正文 \\r 不丢）', () => {
      const crlf = 'context1\r\n<<<<<<< HEAD\r\nours-A\r\nours-B\r\n=======\r\ntheirs-A\r\n>>>>>>> feature\r\ncontext2\r\n'
      const parsed = parseConflictMarkers(crlf)

      expect(parsed.hasMarkers).toBe(true)
      // 正文行保留原始 \r（join('\n') 还原 CRLF）
      expect(parsed.blocks[0].ours).toEqual(['ours-A\r', 'ours-B\r'])
      expect(parsed.blocks[0].theirs).toEqual(['theirs-A\r'])
      // CRLF 文件的 `=======\r` 分隔行被正确识别（不混入 ours 内容）
      expect(parsed.blocks[0].ours).not.toContain('=======\r')

      parsed.blocks[0].resolved = { kind: 'theirs', lines: [...parsed.blocks[0].theirs] }
      expect(mergeConflictResult(parsed)).toBe('context1\r\ntheirs-A\r\ncontext2\r\n')
    })

    it('CRLF 多块合并：lead 上下文与未决策块均保持 \\r\\n', () => {
      const crlf = 'a\r\n<<<<<<< x\r\no1\r\n=======\r\nt1\r\n>>>>>>> y\r\nmid\r\n<<<<<<< z\r\no2\r\n=======\r\nt2\r\n>>>>>>> w\r\n'
      const parsed = parseConflictMarkers(crlf)
      expect(parsed.blocks[1].lead).toEqual(['mid\r'])

      parsed.blocks[0].resolved = { kind: 'ours', lines: [...parsed.blocks[0].ours] }
      expect(mergeConflictResult(parsed)).toBe('a\r\no1\r\nmid\r\no2\r\nt2\r\n')
    })

    it('containsConflictMarkers 对 CRLF 标记行同样检出', () => {
      expect(containsConflictMarkers('a\r\n<<<<<<< HEAD\r\nb\r\n>>>>>>> x\r\n')).toBe(true)
    })
  })

  describe('containsConflictMarkers', () => {
    it('行首 <<<<<<< / >>>>>>> 检出', () => {
      expect(containsConflictMarkers('a\n<<<<<<< HEAD\nb')).toBe(true)
      expect(containsConflictMarkers('a\n>>>>>>> b')).toBe(true)
    })

    it('缩进的标记与无标记文本不检出', () => {
      expect(containsConflictMarkers('  <<<<<<< HEAD')).toBe(false)
      expect(containsConflictMarkers('plain\ntext')).toBe(false)
      expect(containsConflictMarkers('')).toBe(false)
    })
  })
})
