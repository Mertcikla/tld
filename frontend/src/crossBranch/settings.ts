import { useCallback, useEffect, useMemo, useState } from 'react'
import type { CrossBranchConnectorPriority, CrossBranchContextSettings, CrossBranchSurface } from './types'
import {
  CROSS_BRANCH_CONNECTOR_BUDGET_DEFAULT,
  CROSS_BRANCH_CONNECTOR_BUDGET_MAX,
  CROSS_BRANCH_CONNECTOR_BUDGET_MIN,
  CROSS_BRANCH_DEPTH_ALL,
} from './types'

const STORAGE_PREFIX = 'diag:cross-branch'
export const DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA = 0.35
export const DEFAULT_MAX_PROXY_CONNECTOR_GROUPS = 32
export const DEFAULT_CONNECTOR_PRIORITY: CrossBranchConnectorPriority = 'external'

function storageKey(surface: CrossBranchSurface) {
  return `${STORAGE_PREFIX}:${surface}`
}

function defaultSettings(surface: CrossBranchSurface): CrossBranchContextSettings {
  return {
    enabled: surface !== 'zui-shared',
    depth: CROSS_BRANCH_DEPTH_ALL,
    connectorBudget: CROSS_BRANCH_CONNECTOR_BUDGET_DEFAULT,
    connectorPriority: DEFAULT_CONNECTOR_PRIORITY,
    minConnectorAnchorAlpha: DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA,
    maxProxyConnectorGroups: DEFAULT_MAX_PROXY_CONNECTOR_GROUPS,
  }
}

function normalizeConnectorBudget(value: unknown, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  return Math.max(
    CROSS_BRANCH_CONNECTOR_BUDGET_MIN,
    Math.min(CROSS_BRANCH_CONNECTOR_BUDGET_MAX, Math.round(value)),
  )
}

function normalizeConnectorPriority(value: unknown, fallback: CrossBranchConnectorPriority): CrossBranchConnectorPriority {
  return value === 'internal' || value === 'external' ? value : fallback
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
      connectorBudget: normalizeConnectorBudget(parsed.connectorBudget, defaults.connectorBudget),
      connectorPriority: normalizeConnectorPriority(parsed.connectorPriority, defaults.connectorPriority),
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

  const setConnectorBudget = useCallback((connectorBudget: number) => {
    setSettings((prev) => ({
      ...prev,
      connectorBudget: normalizeConnectorBudget(connectorBudget, prev.connectorBudget),
      maxProxyConnectorGroups: normalizeConnectorBudget(connectorBudget, prev.connectorBudget),
    }))
  }, [])

  const setConnectorPriority = useCallback((connectorPriority: CrossBranchConnectorPriority) => {
    setSettings((prev) => ({ ...prev, connectorPriority }))
  }, [])

  return useMemo(() => ({
    settings,
    setEnabled,
    setDepth,
    setConnectorBudget,
    setConnectorPriority,
  }), [settings, setEnabled, setDepth, setConnectorBudget, setConnectorPriority])
}
