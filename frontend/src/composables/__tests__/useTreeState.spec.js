import { describe, it, expect, beforeEach } from 'vitest'
import { useTreeState } from '../useTreeState'

describe('useTreeState', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('saves and restores expanded paths', () => {
    const { saveState, restoreState } = useTreeState()

    saveState('/work/projectA', {
      expandedPaths: ['/work/projectA/src', '/work/projectA/src/components'],
      scrollTop: 120,
      selectedPath: '/work/projectA/src/main.js'
    })

    const state = restoreState('/work/projectA')
    expect(state.expandedPaths).toEqual(['/work/projectA/src', '/work/projectA/src/components'])
    expect(state.scrollTop).toBe(120)
    expect(state.selectedPath).toBe('/work/projectA/src/main.js')
  })

  it('returns empty state for unknown directory', () => {
    const { restoreState } = useTreeState()

    const state = restoreState('/unknown/dir')
    expect(state.expandedPaths).toEqual([])
    expect(state.scrollTop).toBe(0)
    expect(state.selectedPath).toBeNull()
  })

  it('limits expanded paths to 200', () => {
    const { saveState, restoreState } = useTreeState()

    const paths = Array.from({ length: 250 }, (_, i) => `/dir/path${i}`)
    saveState('/work/big', { expandedPaths: paths, scrollTop: 0, selectedPath: null })

    const state = restoreState('/work/big')
    expect(state.expandedPaths.length).toBe(200)
  })

  it('clears state for a directory', () => {
    const { saveState, clearState, restoreState } = useTreeState()

    saveState('/work/projectA', { expandedPaths: ['/a'], scrollTop: 0, selectedPath: null })
    clearState('/work/projectA')

    const state = restoreState('/work/projectA')
    expect(state.expandedPaths).toEqual([])
  })
})
