const DEFAULT_WEB_BASE = "/"

function trimTrailingSlash(value: string): string {
  return value.replace(/\/+$/, "")
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

export const isNativeApp = false

export const apiBase = trimTrailingSlash(
  configuredApiBase ?? "/api",
)

export function apiUrl(path: string): string {
  return `${apiBase}${path.startsWith("/") ? path : `/${path}`}`
}
