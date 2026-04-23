import React, { type ReactNode } from 'react'
import type { PlatformFeatures } from './types'
import { PlatformContext } from './context'

export const PlatformProvider = ({ 
  platform, 
  children 
}: { 
  platform: PlatformFeatures<unknown>; 
  children: ReactNode 
}) => {
  return (
    <PlatformContext.Provider value={platform}>
      {children}
    </PlatformContext.Provider>
  )
}
