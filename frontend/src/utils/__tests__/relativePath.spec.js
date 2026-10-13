// relativePath.js 相对路径计算测试
// 覆盖：/ 归一、混合分隔符、根尾分隔符、节点即根、根外防御分支、@ 引用前缀
import { describe, it, expect } from 'vitest'
import { toRelativePath, toFileRefPath } from '../relativePath'

describe('toRelativePath', () => {
  it('Windows 反斜杠路径归一为 / 相对路径', () => {
    expect(toRelativePath('D:\\work\\repo\\src\\a.go', 'D:\\work\\repo')).toBe('src/a.go')
  })

  it('Unix 斜杠路径直接求相对', () => {
    expect(toRelativePath('/home/u/repo/src/a.go', '/home/u/repo')).toBe('src/a.go')
  })

  it('混合分隔符（后端返回 \\ 与根 / 混用）正常归一', () => {
    expect(toRelativePath('D:\\work\\repo\\README.md', 'D:/work/repo')).toBe('README.md')
  })

  it('根路径带尾分隔符时不产生空段', () => {
    expect(toRelativePath('D:/work/repo/a.txt', 'D:/work/repo/')).toBe('a.txt')
  })

  it('根下子目录节点保留层级', () => {
    expect(toRelativePath('D:\\work\\repo\\a\\b\\c.txt', 'D:\\work\\repo')).toBe('a/b/c.txt')
  })

  it('节点即根时返回 .', () => {
    expect(toRelativePath('D:/work/repo', 'D:/work/repo')).toBe('.')
  })

  it('节点不在根下时返回归一化绝对路径（防御分支）', () => {
    expect(toRelativePath('E:/other/x.txt', 'D:/work/repo')).toBe('E:/other/x.txt')
  })

  it('nodePath 为空时返回空串', () => {
    expect(toRelativePath('', 'D:/work/repo')).toBe('')
  })

  it('rootPath 为空时返回归一化 nodePath（防御分支）', () => {
    expect(toRelativePath('D:\\work\\repo\\a.txt', '')).toBe('D:/work/repo/a.txt')
  })
})

describe('toFileRefPath', () => {
  it('相对路径前加 @ 前缀', () => {
    expect(toFileRefPath('D:\\work\\repo\\src\\a.go', 'D:\\work\\repo')).toBe('@src/a.go')
  })

  it('节点即根时为 @.', () => {
    expect(toFileRefPath('D:/work/repo', 'D:/work/repo')).toBe('@.')
  })
})
