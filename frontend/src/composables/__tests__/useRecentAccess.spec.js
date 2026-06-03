import { describe, it, expect, beforeEach } from 'vitest'
import { useRecentAccess } from '../useRecentAccess'

describe('useRecentAccess', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('records access and retrieves recent items', () => {
    const { record, getRecent } = useRecentAccess()

    record({ path: '/a/file.js', type: 'file', workDir: '/a' })
    record({ path: '/a/src', type: 'dir', workDir: '/a' })

    const items = getRecent(10)
    expect(items.length).toBe(2)
    expect(items[0].path).toBe('/a/src')
  })

  it('deduplicates by path and updates lastAccess', () => {
    const { record, getRecent } = useRecentAccess()

    record({ path: '/a/file.js', type: 'file', workDir: '/a' })
    record({ path: '/a/other.js', type: 'file', workDir: '/a' })
    record({ path: '/a/file.js', type: 'file', workDir: '/a' })

    const items = getRecent(10)
    expect(items.length).toBe(2)
    expect(items[0].path).toBe('/a/file.js')
  })

  it('limits to 50 records', () => {
    const { record, getRecent } = useRecentAccess()

    for (let i = 0; i < 60; i++) {
      record({ path: `/dir/file${i}.js`, type: 'file', workDir: '/dir' })
    }

    const items = getRecent(100)
    expect(items.length).toBe(50)
  })

  it('clears all records', () => {
    const { record, getRecent, clear } = useRecentAccess()

    record({ path: '/a/file.js', type: 'file', workDir: '/a' })
    clear()

    const items = getRecent(10)
    expect(items.length).toBe(0)
  })
})
