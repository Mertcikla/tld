import type { ComponentType, ReactNode } from 'react'

export interface PlatformRouteContext<User = unknown> {
  user: User | null
  onLogin?: (user: User) => void
  onLogout?: () => void
}

export interface PlatformFeatures<User = unknown> {
  hasAuth?: boolean
  hasBilling?: boolean
  initPlatform: (orgId?: string) => Promise<void>
  getUpgradePath?: () => string | null
  getRoutes: (context: PlatformRouteContext<User>) => ReactNode[]
  getAuthenticatedRoutes: (context: PlatformRouteContext<User>) => ReactNode[]
  getSettingsRoutes: (context: PlatformRouteContext<User>) => ReactNode[]
  AuthLayout: ComponentType<unknown>
  connectRealtime?: (...args: unknown[]) => unknown
}
