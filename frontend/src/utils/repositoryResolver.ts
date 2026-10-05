import { api, type IndexedRepository } from '../api/client'

// Element source links use two legacy repo encodings: an absolute local
// worktree root (indexed/materialized elements) or a remote URL/slug (linked
// elements). This module resolves both to an indexed repository and normalizes
// remote identities the same way the backend repolink package does.

export function isLocalRepoPath(value: string | null | undefined): boolean {
  const cleaned = (value ?? '').trim()
  if (!cleaned) return false
  if (cleaned.startsWith('/') || cleaned.startsWith('~') || cleaned.startsWith('\\\\')) return true
  if (cleaned.startsWith('./') || cleaned.startsWith('../') || cleaned === '.' || cleaned === '..') return true
  return /^[A-Za-z]:[\\/]/.test(cleaned)
}

function stripRemoteSuffixes(value: string): string {
  let cleaned = value.trim()
  cleaned = cleaned.replace(/\/+$/, '')
  cleaned = cleaned.replace(/\.git$/, '')
  return cleaned
}

function splitRemote(value: string): { host: string; path: string } | null {
  const cleaned = stripRemoteSuffixes(value)
  if (!cleaned) return null
  let host = ''
  let path = ''
  if (cleaned.includes('://')) {
    let parsed: URL
    try {
      parsed = new URL(cleaned)
    } catch {
      return null
    }
    if (!parsed.hostname) return null
    host = parsed.hostname
    path = parsed.pathname
  } else if (cleaned.includes('@') && cleaned.includes(':')) {
    const at = cleaned.indexOf('@')
    const colon = cleaned.indexOf(':', at)
    host = cleaned.slice(at + 1, colon)
    path = cleaned.slice(colon + 1)
  } else if (cleaned.includes(':')) {
    const colon = cleaned.indexOf(':')
    host = cleaned.slice(0, colon)
    path = cleaned.slice(colon + 1)
  } else {
    const slash = cleaned.indexOf('/')
    if (slash < 0) return null
    host = cleaned.slice(0, slash)
    path = cleaned.slice(slash + 1)
  }
  host = host.trim().toLowerCase()
  path = path.replace(/^\/+/, '').replace(/\/+$/, '')
  if (!host || !path) return null
  if (!host.includes('.')) {
    // GitHub-style owner/repo shorthand.
    if (path.includes('/')) return null
    return { host: 'github.com', path: `${host}/${path}` }
  }
  return { host, path }
}

// normalizeRemoteKey returns a comparable identity for a remote URL, SSH
// remote, or owner/repo shorthand. Local paths and unparseable values yield ''.
export function normalizeRemoteKey(value: string | null | undefined): string {
  const cleaned = (value ?? '').trim()
  if (!cleaned || isLocalRepoPath(cleaned)) return ''
  const split = splitRemote(cleaned)
  if (!split) return ''
  return `${split.host}/${split.path}`.toLowerCase()
}

// resolveElementRepository links an element to an indexed repository, preferring
// the explicit repository id, then the legacy repo value, then an absolute file
// path under a repository root.
export function resolveElementRepository(
  element: { repository_id?: string | null; repo?: string | null; file_path?: string | null },
  repos: IndexedRepository[],
): IndexedRepository | null {
  const id = (element.repository_id ?? '').trim()
  if (id) {
    const match = repos.find((repo) => repo.id === id)
    if (match) return match
  }
  const repoValue = (element.repo ?? '').trim()
  if (repoValue) {
    const byRoot = repos.find((repo) => repo.root === repoValue)
    if (byRoot) return byRoot
    const key = normalizeRemoteKey(repoValue)
    if (key) {
      const byRemote = repos.find((repo) => normalizeRemoteKey(repo.remoteUrl) === key)
      if (byRemote) return byRemote
    }
  }
  const filePath = ((element.file_path ?? '').split('#')[0] ?? '').trim()
  if (filePath.startsWith('/') && !filePath.includes('..')) {
    const byFile = repos.find((repo) => {
      const root = repo.root.replace(/\/+$/, '')
      return root !== '' && (filePath === root || filePath.startsWith(`${root}/`))
    })
    if (byFile) return byFile
  }
  return null
}

// parseRepositoryAddInput separates a local path from a remote repository
// reference. GitHub owner/repo shorthand is treated as remote.
export function parseRepositoryAddInput(value: string): { path?: string; remoteUrl?: string } {
  const cleaned = value.trim()
  if (
    /^(https?:\/\/|ssh:\/\/|git:\/\/|git@)/i.test(cleaned) ||
    /^github\.com\//i.test(cleaned) ||
    /\.git$/i.test(cleaned)
  ) {
    return { remoteUrl: cleaned }
  }
  const parts = cleaned.split('/')
  if (
    parts.length === 2 &&
    parts[0] !== '' &&
    parts[1] !== '' &&
    !parts[0].includes('.') &&
    !parts[0].includes(':') &&
    !parts[0].startsWith('~')
  ) {
    return { remoteUrl: cleaned }
  }
  return { path: cleaned }
}

let cache: { at: number; promise: Promise<IndexedRepository[]> } | null = null
const CACHE_TTL_MS = 5_000

export function listIndexedRepositories(force = false): Promise<IndexedRepository[]> {
  if (force || !cache || Date.now() - cache.at > CACHE_TTL_MS) {
    const promise = api.repositories.list().catch((error) => {
      cache = null
      throw error
    })
    cache = { at: Date.now(), promise }
  }
  return cache.promise
}

export function invalidateIndexedRepositories(): void {
  cache = null
}
