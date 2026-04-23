import { createContext, useContext } from 'react'
import type { PlatformFeatures } from './types'
import { platform as localPlatform } from './local'

export const PlatformContext = createContext<PlatformFeatures<unknown>>(localPlatform)

export function usePlatform() {
  return useContext(PlatformContext)
}
