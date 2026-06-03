const MAX_EXPANDED_PATHS = 200
const STORAGE_PREFIX = 'treeState:'

export function useTreeState() {
  function saveState(dirPath, state) {
    const key = STORAGE_PREFIX + dirPath
    const data = {
      expandedPaths: (state.expandedPaths || []).slice(-MAX_EXPANDED_PATHS),
      scrollTop: state.scrollTop || 0,
      selectedPath: state.selectedPath || null
    }
    try {
      localStorage.setItem(key, JSON.stringify(data))
    } catch (e) {
      // localStorage 满时静默失败
    }
  }

  function restoreState(dirPath) {
    const key = STORAGE_PREFIX + dirPath
    try {
      const raw = localStorage.getItem(key)
      if (!raw) return emptyState()
      return JSON.parse(raw)
    } catch {
      return emptyState()
    }
  }

  function clearState(dirPath) {
    const key = STORAGE_PREFIX + dirPath
    localStorage.removeItem(key)
  }

  function emptyState() {
    return { expandedPaths: [], scrollTop: 0, selectedPath: null }
  }

  return { saveState, restoreState, clearState }
}
