const STORAGE_KEY = 'recentAccess'
const MAX_RECORDS = 50

export function useRecentAccess() {
  function loadRecords() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      if (!raw) return []
      return JSON.parse(raw)
    } catch {
      return []
    }
  }

  function saveRecords(records) {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(records))
    } catch {
      // 静默失败
    }
  }

  function record({ path, type, workDir }) {
    let records = loadRecords()

    records = records.filter(r => r.path !== path)

    records.unshift({
      path,
      type,
      workDir,
      lastAccess: Date.now()
    })

    if (records.length > MAX_RECORDS) {
      records = records.slice(0, MAX_RECORDS)
    }

    saveRecords(records)
  }

  function getRecent(limit = 10) {
    const records = loadRecords()
    return records.slice(0, limit)
  }

  function clear() {
    localStorage.removeItem(STORAGE_KEY)
  }

  return { record, getRecent, clear }
}
