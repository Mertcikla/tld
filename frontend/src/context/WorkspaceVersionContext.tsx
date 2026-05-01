import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { WatchDiff, WatchRepository, WatchVersion, WorkspaceVersion } from '../api/client'
import { normalizeWatchChangeType } from '../utils/watchDiffSummary'

export type VersionChangeType = 'added' | 'updated' | 'deleted' | 'changed'

export interface VersionLineDelta {
  added: number
  removed: number
}

export interface WorkspaceVersionPreview {
  repository: WatchRepository | null
  version: WatchVersion | null
  workspaceVersions: WorkspaceVersion[]
  diffs: WatchDiff[]
  elementChanges: Map<number, VersionChangeType>
  elementLineDeltas: Map<number, VersionLineDelta>
  connectorChanges: Map<number, VersionChangeType>
  summary: {
    added: number
    updated: number
    deleted: number
    changed: number
    elements: number
    connectors: number
  }
}

export interface WorkspaceVersionFollowTarget {
  token: number
  resourceType: string
  resourceId?: number
  viewId?: number
  changeType?: VersionChangeType
}

interface WorkspaceVersionContextValue {
  preview: WorkspaceVersionPreview | null
  followToken: number
  followTarget: WorkspaceVersionFollowTarget | null
  setPreview: (preview: WorkspaceVersionPreview | null) => void
  clearPreview: () => void
  requestFollow: (target?: Omit<WorkspaceVersionFollowTarget, 'token'>) => void
}

const WorkspaceVersionContext = createContext<WorkspaceVersionContextValue | null>(null)

export function buildWorkspaceVersionPreview(args: {
  repository: WatchRepository | null
  version: WatchVersion | null
  workspaceVersions: WorkspaceVersion[]
  diffs: WatchDiff[] | null | undefined
}): WorkspaceVersionPreview {
  const diffs = Array.isArray(args.diffs) ? args.diffs : []
  const elementChanges = new Map<number, VersionChangeType>()
  const elementLineDeltas = new Map<number, VersionLineDelta>()
  const connectorChanges = new Map<number, VersionChangeType>()
  const summary = { added: 0, updated: 0, deleted: 0, changed: 0, elements: 0, connectors: 0 }

  diffs.forEach((diff) => {
    const change = normalizeWatchChangeType(diff.change_type)
    summary[change] += 1
    if (diff.resource_type === 'element' && diff.resource_id) {
      elementChanges.set(diff.resource_id, change)
      const added = Math.max(0, diff.added_lines ?? 0)
      const removed = Math.max(0, diff.removed_lines ?? 0)
      if (added > 0 || removed > 0) {
        elementLineDeltas.set(diff.resource_id, { added, removed })
      }
      summary.elements += 1
    }
    if (diff.resource_type === 'connector' && diff.resource_id) {
      connectorChanges.set(diff.resource_id, change)
      summary.connectors += 1
    }
  })

  return {
    repository: args.repository,
    version: args.version,
    workspaceVersions: args.workspaceVersions,
    diffs,
    elementChanges,
    elementLineDeltas,
    connectorChanges,
    summary,
  }
}

export function WorkspaceVersionProvider({ children }: { children: React.ReactNode }) {
  const [preview, setPreview] = useState<WorkspaceVersionPreview | null>(null)
  const [followToken, setFollowToken] = useState(0)
  const [followTarget, setFollowTarget] = useState<WorkspaceVersionFollowTarget | null>(null)
  const followClearTimerRef = useRef<number | null>(null)
  const clearPreview = useCallback(() => setPreview(null), [])
  const requestFollow = useCallback((target?: Omit<WorkspaceVersionFollowTarget, 'token'>) => {
    setFollowToken((value) => value + 1)
    if (followClearTimerRef.current !== null) {
      window.clearTimeout(followClearTimerRef.current)
      followClearTimerRef.current = null
    }
    if (!target) {
      setFollowTarget(null)
      return
    }
    setFollowTarget({ ...target, token: Date.now() })
    followClearTimerRef.current = window.setTimeout(() => {
      setFollowTarget(null)
      followClearTimerRef.current = null
    }, 1600)
  }, [])
  useEffect(() => {
    return () => {
      if (followClearTimerRef.current !== null) window.clearTimeout(followClearTimerRef.current)
    }
  }, [])
  const value = useMemo(() => ({ preview, followToken, followTarget, setPreview, clearPreview, requestFollow }), [preview, followToken, followTarget, clearPreview, requestFollow])
  return <WorkspaceVersionContext.Provider value={value}>{children}</WorkspaceVersionContext.Provider>
}

export function useWorkspaceVersionPreview() {
  const value = useContext(WorkspaceVersionContext)
  if (!value) throw new Error('useWorkspaceVersionPreview must be used within WorkspaceVersionProvider')
  return value
}
