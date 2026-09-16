import { describe, it, expect } from 'vitest'
import { normalizePath, belongsToDir, findOwningDirectory } from '../pathMatch'

describe('normalizePath', () => {
  it('反斜杠转正斜杠 + 转小写', () => {
    expect(normalizePath('D:\\Projects\\Sub')).toBe('d:/projects/sub')
  })

  it('空值兜底空串', () => {
    expect(normalizePath(undefined)).toBe('')
    expect(normalizePath('')).toBe('')
  })

  it('已是正斜杠仅转小写', () => {
    expect(normalizePath('D:/Projects/Sub')).toBe('d:/projects/sub')
  })
})

describe('belongsToDir', () => {
  it('仓库路径属于工作目录（子路径）', () => {
    expect(belongsToDir('D:\\Projects\\repo', 'D:\\Projects')).toBe(true)
  })

  it('大小写不敏感', () => {
    expect(belongsToDir('d:\\projects\\repo', 'D:\\Projects')).toBe(true)
  })

  it('分隔符混合也能匹配', () => {
    expect(belongsToDir('D:/projects/repo', 'D:\\Projects')).toBe(true)
  })

  it('完全相等路径归属自身', () => {
    expect(belongsToDir('D:\\Projects', 'D:\\Projects')).toBe(true)
  })

  it('同前缀串不误匹配（projects vs projects-other）', () => {
    expect(belongsToDir('D:\\projects-other\\repo', 'D:\\projects')).toBe(false)
  })

  it('不属于返回 false', () => {
    expect(belongsToDir('E:\\Other\\repo', 'D:\\Projects')).toBe(false)
  })

  it('空 dirPath 返回 false', () => {
    expect(belongsToDir('D:\\Projects\\repo', '')).toBe(false)
  })

  it('盘根工作目录匹配其下子路径', () => {
    expect(belongsToDir('D:\\Projects\\repo', 'D:\\')).toBe(true)
  })

  it('带尾分隔符的工作目录仍匹配子路径', () => {
    expect(belongsToDir('D:\\Projects\\repo', 'D:\\Projects\\')).toBe(true)
  })
})

describe('findOwningDirectory', () => {
  const dirs = [
    { id: 'a', path: 'D:\\Projects' },
    { id: 'b', path: 'D:\\Projects\\Sub' }
  ]

  it('嵌套工作目录取最长前缀（最具体）', () => {
    const owned = findOwningDirectory('D:\\Projects\\Sub\\repo', dirs)
    expect(owned.id).toBe('b')
  })

  it('仅属外层时取外层', () => {
    const owned = findOwningDirectory('D:\\Projects\\other-repo', dirs)
    expect(owned.id).toBe('a')
  })

  it('无匹配返回 null', () => {
    expect(findOwningDirectory('E:\\Other\\repo', dirs)).toBeNull()
  })

  it('空列表返回 null', () => {
    expect(findOwningDirectory('D:\\Projects\\repo', [])).toBeNull()
  })

  it('空路径返回 null', () => {
    expect(findOwningDirectory('', dirs)).toBeNull()
  })

  it('大小写不敏感匹配', () => {
    const owned = findOwningDirectory('d:\\projects\\sub\\repo', dirs)
    expect(owned.id).toBe('b')
  })

  it('同前缀串不误选外层', () => {
    const owned = findOwningDirectory('D:\\Projects-other\\repo', dirs)
    expect(owned).toBeNull()
  })
})
