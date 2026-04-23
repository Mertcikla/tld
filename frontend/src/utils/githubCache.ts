interface CacheEntry<T> {
  data: T
  timestamp: number
}

const CACHE_TTL = 10 * 60 * 1000 // 10 minutes
const STORAGE_KEY = 'diag_github_cache'

interface CacheData {
  branches: Record<string, CacheEntry<string[]>>
  trees: Record<string, CacheEntry<string[]>>
  contents: Record<string, CacheEntry<string>>
  repoVisibility: Record<string, CacheEntry<boolean>>
}

class GithubCache {
  private cache: CacheData

  constructor() {
    this.cache = this.load()
    this.cleanup()
  }

  private load(): CacheData {
    try {
      const stored = localStorage.getItem(STORAGE_KEY)
      if (stored) return JSON.parse(stored)
    } catch { /* empty */ }
    return { branches: {}, trees: {}, contents: {}, repoVisibility: {} }
  }

  private save() {
    try {
      // Basic size limit check - clear if too big (simple for now)
      const str = JSON.stringify(this.cache)
      if (str.length > 4 * 1024 * 1024) { // 4MB
        this.cache = { branches: {}, trees: {}, contents: {}, repoVisibility: {} }
      }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(this.cache))
    } catch { /* empty */ }
  }

  private cleanup() {
    const now = Date.now()
    let changed = false
    const clean = <T>(map: Record<string, CacheEntry<T>>) => {
      for (const [k, v] of Object.entries(map)) {
        if (now - v.timestamp > CACHE_TTL) { delete map[k]; changed = true }
      }
    }
    clean(this.cache.branches)
    clean(this.cache.trees)
    clean(this.cache.contents)
    if (!this.cache.repoVisibility) this.cache.repoVisibility = {}
    clean(this.cache.repoVisibility)
    if (changed) this.save()
  }

  getBranches(repo: string): string[] | null {
    const entry = this.cache.branches[repo]
    if (entry && Date.now() - entry.timestamp < CACHE_TTL) return entry.data
    return null
  }

  setBranches(repo: string, data: string[]) {
    this.cache.branches[repo] = { data, timestamp: Date.now() }
    this.save()
  }

  getTree(repo: string, branch: string): string[] | null {
    const key = `${repo}/${branch}`
    const entry = this.cache.trees[key]
    if (entry && Date.now() - entry.timestamp < CACHE_TTL) return entry.data
    return null
  }

  setTree(repo: string, branch: string, data: string[]) {
    const key = `${repo}/${branch}`
    this.cache.trees[key] = { data, timestamp: Date.now() }
    this.save()
  }

  getContent(rawUrl: string): string | null {
    const entry = this.cache.contents[rawUrl]
    if (entry && Date.now() - entry.timestamp < CACHE_TTL) return entry.data
    return null
  }

  setContent(rawUrl: string, data: string) {
    this.cache.contents[rawUrl] = { data, timestamp: Date.now() }
    this.save()
  }

  getRepoPublic(slug: string): boolean | null {
    if (!this.cache.repoVisibility) return null
    const entry = this.cache.repoVisibility[slug]
    if (entry && Date.now() - entry.timestamp < CACHE_TTL) return entry.data
    return null
  }

  setRepoPublic(slug: string, isPublic: boolean) {
    if (!this.cache.repoVisibility) this.cache.repoVisibility = {}
    this.cache.repoVisibility[slug] = { data: isPublic, timestamp: Date.now() }
    this.save()
  }
}

export const githubCache = new GithubCache()
