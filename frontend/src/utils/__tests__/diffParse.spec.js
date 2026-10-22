import { describe, it, expect } from 'vitest'
import { parseDiff, parseRangeDiff } from '../diffParse'

describe('utils/diffParse', () => {
  describe('parseDiff', () => {
    it('空文本返回空双栏', () => {
      expect(parseDiff('')).toEqual({ leftLines: [], rightLines: [] })
      expect(parseDiff(null)).toEqual({ leftLines: [], rightLines: [] })
    })

    it('hunk 头进入双栏且为 hunk 行，不占行号', () => {
      const { leftLines, rightLines } = parseDiff('@@ -1,2 +1,2 @@')
      expect(leftLines).toHaveLength(1)
      expect(leftLines[0].kind).toBe('hunk')
      expect(leftLines[0].text).toBe('@@ -1,2 +1,2 @@')
      expect(rightLines).toEqual(leftLines)
    })

    it('context 行左右对齐且行号自 hunk 头起始递增', () => {
      const { leftLines, rightLines } = parseDiff('@@ -10,2 +20,2 @@\n ctx-a\n ctx-b')
      expect(leftLines.map(l => l.no)).toEqual(['', '10', '11'])
      expect(rightLines.map(l => l.no)).toEqual(['', '20', '21'])
      expect(leftLines[1]).toEqual({ kind: 'context', no: '10', text: 'ctx-a' })
      expect(rightLines[1]).toEqual({ kind: 'context', no: '20', text: 'ctx-a' })
    })

    it('del 行进左栏、右栏补空占位；add 行进右栏、左栏补空占位', () => {
      const { leftLines, rightLines } = parseDiff('@@ -1,2 +1,2 @@\n-old\n+new\n ctx')
      expect(leftLines.map(l => l.kind)).toEqual(['hunk', 'del', 'empty', 'context'])
      expect(rightLines.map(l => l.kind)).toEqual(['hunk', 'empty', 'add', 'context'])
      expect(leftLines[1]).toEqual({ kind: 'del', no: '1', text: 'old' })
      expect(rightLines[2]).toEqual({ kind: 'add', no: '1', text: 'new' })
      // 占位行无行号
      expect(rightLines[1]).toEqual({ kind: 'empty', no: '', text: '' })
      expect(leftLines[2]).toEqual({ kind: 'empty', no: '', text: '' })
    })

    it('del/add 交错时行号各自独立递增（对齐填充）', () => {
      const { leftLines, rightLines } = parseDiff('@@ -1,3 +1,3 @@\n-a1\n-a2\n+b1\n+b2\n keep')
      expect(leftLines.map(l => [l.kind, l.no])).toEqual([
        ['hunk', ''],
        ['del', '1'],
        ['del', '2'],
        ['empty', ''],
        ['empty', ''],
        ['context', '3']
      ])
      expect(rightLines.map(l => [l.kind, l.no])).toEqual([
        ['hunk', ''],
        ['empty', ''],
        ['empty', ''],
        ['add', '1'],
        ['add', '2'],
        ['context', '3']
      ])
    })

    it('跳过文件头行（diff --git / index / +++ / ---）不污染双栏', () => {
      const text = [
        'diff --git a/src/a.js b/src/a.js',
        'index 1111111..2222222 100644',
        '--- a/src/a.js',
        '+++ b/src/a.js',
        '@@ -1,1 +1,1 @@',
        ' ctx'
      ].join('\n')
      const { leftLines, rightLines } = parseDiff(text)
      // 仅 hunk + ctx，4 行文件头全部跳过
      expect(leftLines).toHaveLength(2)
      expect(rightLines).toHaveLength(2)
      expect(leftLines[1].text).toBe('ctx')
    })

    it('"\\ No newline at end of file" 提示行被忽略不进双栏', () => {
      const text = ['@@ -1,1 +1,1 @@', ' ctx', '\\ No newline at end of file'].join('\n')
      const { leftLines, rightLines } = parseDiff(text)
      expect(leftLines).toHaveLength(2) // hunk + ctx
      expect(rightLines).toHaveLength(2)
    })

    it('兼容 \\r\\n 行尾', () => {
      const { leftLines } = parseDiff('@@ -1,1 +1,1 @@\r\n ctx\r\n')
      expect(leftLines).toHaveLength(2)
      expect(leftLines[1].text).toBe('ctx')
    })
  })

  describe('parseRangeDiff', () => {
    it('空文本返回空分组', () => {
      expect(parseRangeDiff('')).toEqual([])
      expect(parseRangeDiff(null)).toEqual([])
    })

    it('多文件按 diff --git 头拆分，取 b/ 路径为文件名', () => {
      const text = [
        'diff --git a/src/a.go b/src/a.go',
        'index 111..222 100644',
        '--- a/src/a.go',
        '+++ b/src/a.go',
        '@@ -1,1 +1,2 @@',
        ' ctx',
        '+new',
        'diff --git a/src/b.go b/src/b.go',
        'index 333..444 100644',
        '--- a/src/b.go',
        '+++ b/src/b.go',
        '@@ -1,1 +1,1 @@',
        '-gone'
      ].join('\n')
      const groups = parseRangeDiff(text)
      expect(groups).toHaveLength(2)
      expect(groups[0].file).toBe('src/a.go')
      expect(groups[0].binary).toBe(false)
      expect(groups[0].right.filter(l => l.kind === 'add')).toHaveLength(1)
      expect(groups[1].file).toBe('src/b.go')
      expect(groups[1].left.filter(l => l.kind === 'del')).toHaveLength(1)
    })

    it('二进制段单独标记不进双栏', () => {
      const text = [
        'diff --git a/bin.dat b/bin.dat',
        'Binary files a/bin.dat and b/bin.dat differ'
      ].join('\n')
      const groups = parseRangeDiff(text)
      expect(groups).toHaveLength(1)
      expect(groups[0].binary).toBe(true)
      expect(groups[0].file).toBe('bin.dat')
      expect(groups[0].left).toHaveLength(0)
    })

    it('无行内容段（仅文件头）不产出分组', () => {
      const text = 'diff --git a/x.txt b/x.txt\nindex 111..222 100644\n--- a/x.txt\n+++ b/x.txt\n'
      expect(parseRangeDiff(text)).toEqual([])
    })
  })
})
