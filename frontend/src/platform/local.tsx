import { Navigate, Route } from 'react-router-dom'
import type { PlatformFeatures } from './types'

export const platform: PlatformFeatures = {
  hasAuth: false,
  hasBilling: false,
  initPlatform: async () => {},
  getUpgradePath: () => null,
  getRoutes: () => [],
  getAuthenticatedRoutes: () => [],
  getSettingsRoutes: () => [
    <Route key="api-keys" path="api-keys" element={<Navigate to="/settings/appearance" replace />} />,
  ],
  AuthLayout: () => null,
}
