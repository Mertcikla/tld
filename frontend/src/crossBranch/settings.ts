import { useCallback, useEffect, useMemo, useState } from 'react'
import type { CrossBranchContextSettings, CrossBranchSurface } from './types'
import { CROSS_BRANCH_DEPTH_ALL } from './types'

const STORAGE_PREFIX = 'diag:cross-branch'
export const DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA = 0.35
export const DEFAULT_MAX_PROXY_CONNECTOR_GROUPS = 32

function storageKey(surface: CrossBranchSurface) {
  return `${STORAGE_PREFIX}:${surface}`
}

function defaultSettings(surface: CrossBranchSurface): CrossBranchContextSettings {
  return {
    enabled: surface !== 'zui-shared',
    depth: CROSS_BRANCH_DEPTH_ALL,
    minConnectorAnchorAlpha: DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA,
    maxProxyConnectorGroups: DEFAULT_MAX_PROXY_CONNECTOR_GROUPS,
  }
}

function readSettings(surface: CrossBranchSurface): CrossBranchContextSettings {
  const defaults = defaultSettings(surface)
  if (typeof window === 'undefined') return defaults
  const raw = window.localStorage.getItem(storageKey(surface))
  if (!raw) return defaults
  try {
    const parsed = JSON.parse(raw) as Partial<CrossBranchContextSettings>
    return {
      enabled: parsed.enabled ?? defaults.enabled,
      depth: typeof parsed.depth === 'number' ? parsed.depth : CROSS_BRANCH_DEPTH_ALL,
      minConnectorAnchorAlpha: typeof parsed.minConnectorAnchorAlpha === 'number'
        ? parsed.minConnectorAnchorAlpha
        : defaults.minConnectorAnchorAlpha,
      maxProxyConnectorGroups: typeof parsed.maxProxyConnectorGroups === 'number'
        ? parsed.maxProxyConnectorGroups
        : defaults.maxProxyConnectorGroups,
    }
  } catch {
    return defaults
  }
}

export function useCrossBranchContextSettings(surface: CrossBranchSurface) {
  const [settings, setSettings] = useState<CrossBranchContextSettings>(() => readSettings(surface))

  useEffect(() => {
    setSettings(readSettings(surface))
  }, [surface])

  useEffect(() => {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(storageKey(surface), JSON.stringify(settings))
  }, [surface, settings])

  const setEnabled = useCallback((enabled: boolean) => {
    setSettings((prev) => ({ ...prev, enabled }))
  }, [])

  const setDepth = useCallback((depth: number) => {
    setSettings((prev) => ({ ...prev, depth }))
  }, [])

  return useMemo(() => ({
    settings,
    setEnabled,
    setDepth,
  }), [settings, setEnabled, setDepth])
}
