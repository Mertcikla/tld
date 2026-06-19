/**
 * VS Code webview runtime configuration.
 *
 * Replaces runtime.ts for the vscode webview build:
 * - Reads server URL from window.__TLD_SERVER_URL__ instead of import.meta.env
 * - No Capacitor dependency (isNativeApp = false)
 */

declare global {
  interface Window {
    __TLD_SERVER_URL__?: string
  }
}

function trimTrailingSlash(value: string): string {
  let end = value.length
  while (end > 0 && value[end - 1] === '/') {
    end--
  }
  return value.slice(0, end)
}

const serverUrl = trimTrailingSlash(window.__TLD_SERVER_URL__ ?? 'http://127.0.0.1:8060')

export const appBase = '/app/'
export const routerBasename = undefined
export const isNativeApp = false
export const isWailsApp = false
export const isWailsAppStore = false
export const wailsPlatform = undefined
export const tldVersion = 'dev'
export const isWailsMac = false
export const isWailsWindows = false

export const apiBase = `${serverUrl}/api`

export function apiUrl(path: string): string {
  return `${apiBase}${path.startsWith('/') ? path : `/${path}`}`
}

export function fetchApiAsset(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  return fetch(input, init)
}

export function oauthGoogleStartUrl(): string {
  return apiUrl('/auth/oauth/google')
}

export function oauthGithubStartUrl(): string {
  return apiUrl('/auth/oauth/github')
}

export function oauthAppleStartUrl(): string {
  return apiUrl('/auth/oauth/apple')
}

// Web OAuth client ID - also used as serverClientId for native Google Sign-In
export const googleClientId = '945690634753-lcrtd97c5hnqdo5shkoaetstmtrqbk5t.apps.googleusercontent.com'

export const turnstileSiteKey = '0x4AAAAAACyQUcIpN2Yuy8-a'
