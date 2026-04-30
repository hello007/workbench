class GitCache {
  constructor() {
    this.cache = new Map()
    this.maxAge = 5 * 60 * 1000 // 5 分钟过期
  }

  set(key, value) {
    this.cache.set(key, {
      value,
      timestamp: Date.now()
    })
  }

  get(key) {
    const item = this.cache.get(key)
    if (!item) return null

    if (Date.now() - item.timestamp > this.maxAge) {
      this.cache.delete(key)
      return null
    }

    return item.value
  }

  clear() {
    this.cache.clear()
  }

  delete(key) {
    this.cache.delete(key)
  }
}

export const gitCache = new GitCache()

export const getCacheKey = (type, path) => {
  return `${type}:${path}`
}
