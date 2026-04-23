import { isNativeApp } from '../config/runtime'

/**
 * Resolves a static asset path (like a logo or icon) taking into account
 * the environment (dev vs production, web vs native Capacitor).
 */
export function resolveIconPath(path: string | null | undefined): string {
  if (!path) return ''

  // Absolute URLs and data URIs are returned as-is
  if (path.startsWith('http://') || path.startsWith('https://') || path.startsWith('data:')) return path

  // If running inside the native mobile app (Capacitor) OR inside an embedded
  // webview that serves content from localhost (e.g. Capacitor production webview),
  // static assets are typically available at the web root. However we must NOT treat
  // the local dev server (like http://localhost:5173) as a native/webview environment.
  // Use a conservative detection: file:// is always native; localhost/127.0.0.1 is
  // treated as native only when served over HTTPS (Capacitor uses https://localhost)
  // or when there is no non-default dev port present.
  const runningOnLocalhost = typeof window !== 'undefined' && (() => {
    // file: is unequivocally a native / packaged environment (Capacitor)
    if (window.location.protocol === 'file:') return true

    // Only consider plain localhost/127.0.0.1 further
    const hostIsLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1'
    if (!hostIsLocal) return false

    // If served over HTTPS on localhost (Capacitor uses https://localhost) assume native/webview.
    if (window.location.protocol === 'https:') return true

    // If there's no explicit port (or it's the default HTTPS port), treat as native/webview.
    // Dev servers usually expose a non-default port (like 5173) - those should NOT be treated as native.
    const port = (window.location.port || '').trim()
    if (!port || port === '443') return true

    return false
  })()

  if (isNativeApp || runningOnLocalhost) {
    // Strip the web app base prefix (/app/) if present - logo_url values saved from
    // the web interface may have this prefix baked in, but on native assets live at root.
    const stripped = path.startsWith('/app/') ? path.slice('/app'.length) : path
    return stripped.startsWith('/') ? stripped : `/${stripped}`
  }

  // Use Vite's BASE_URL (import.meta.env.BASE_URL)
  const base = import.meta.env.BASE_URL || '/'
  const normalizedBase = base.endsWith('/') ? base : `${base}/`

  // If the path already starts with the base, return it
  if (path.startsWith(normalizedBase) || path === normalizedBase.slice(0, -1)) return path

  const normalizedPath = path.startsWith('/') ? path.slice(1) : path
  return `${normalizedBase}${normalizedPath}`
}

  /**
   * Normalizes any git repository reference to an "owner/repo" slug:
   *   - SSH:   git@github.com:owner/repo.git  → owner/repo
   *   - HTTPS: https://github.com/owner/repo.git → owner/repo
   *   - Plain: owner/repo                     → owner/repo
   */
  export function parseRepoSlug(repo: string): string {
    // SSH format: git@host:owner/repo(.git)
    const sshMatch = repo.match(/^git@[^:]+:(.+?)(?:\.git)?$/)
    if (sshMatch) return sshMatch[1]
    // HTTPS format: https://host/owner/repo(.git)
    const httpsMatch = repo.match(/^https?:\/\/[^/]+\/(.+?)(?:\.git)?$/)
    if (httpsMatch) return httpsMatch[1]
    // Already owner/repo, strip optional .git suffix
    return repo.replace(/\.git$/, '')
  }
