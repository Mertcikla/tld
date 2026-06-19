const DEFAULT_WEB_BASE = "/"

function trimTrailingSlash(value: string): string {
  let end = value.length
  while (end > 0 && value[end - 1] === '/') {
    end--
  }
  return value.slice(0, end)
}

function trim(value: string | undefined): string | undefined {
  if (!value) return undefined
  const cleaned = value.trim()
  return cleaned.length > 0 ? cleaned : undefined
}

const configuredApiBase = trim(import.meta.env.VITE_API_BASE_URL)

export const appBase = trim(import.meta.env.VITE_APP_BASE) ?? DEFAULT_WEB_BASE
export const routerBasename = (() => {
  const configuredBase = trim(import.meta.env.VITE_ROUTER_BASENAME)
  if (configuredBase && configuredBase !== "/") return configuredBase
  return undefined
})()

declare global {
  interface Window {
    __TLD_SERVER_URL__?: string
    __TLD_DIAGRAM_ID__?: number
    __TLD_APP__?: boolean
    __TLD_APP_STORE__?: boolean
    __TLD_PLATFORM__?: string
    __TLD_VERSION__?: string
  }
}

export const isNativeApp = false
const runtimeWindow = typeof window !== 'undefined'
  ? window as Window & { runtime?: unknown; wails?: unknown }
  : undefined
const browserPlatform = typeof navigator !== 'undefined' ? navigator.platform : ''
export const isWailsApp = !!runtimeWindow && (!!runtimeWindow.__TLD_APP__ || !!runtimeWindow.runtime || !!runtimeWindow.wails)
export const isWailsAppStore = !!runtimeWindow?.__TLD_APP_STORE__
export const wailsPlatform = runtimeWindow?.__TLD_PLATFORM__
export const tldVersion = runtimeWindow?.__TLD_VERSION__ ?? trim(import.meta.env.VITE_APP_VERSION) ?? 'dev'
export const isWailsMac = isWailsApp && (wailsPlatform === 'darwin' || (!wailsPlatform && browserPlatform.toLowerCase().includes('mac')))
export const isWailsWindows = isWailsApp && (wailsPlatform === 'windows' || (!wailsPlatform && browserPlatform.toLowerCase().includes('win')))

const defaultApiBase = typeof window !== 'undefined' && window.__TLD_SERVER_URL__ ? `${window.__TLD_SERVER_URL__.replace(/\/$/, "")}/api` : "/api"

export const apiBase = trimTrailingSlash(
  configuredApiBase ?? defaultApiBase,
)

export function apiUrl(path: string): string {
  return `${apiBase}${path.startsWith("/") ? path : `/${path}`}`
}

export function fetchApiAsset(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  return fetch(input, { ...init, credentials: init?.credentials ?? 'include' })
}
