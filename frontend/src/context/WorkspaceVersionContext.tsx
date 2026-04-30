import { createContext, useCallback, useContext, useMemo, useState } from 'react'
import type { WatchDiff, WatchRepository, WatchVersion, WorkspaceVersion } from '../api/client'

export type VersionChangeType = 'added' | 'updated' | 'deleted' | 'changed'

export interface WorkspaceVersionPreview {
  repository: WatchRepository | null
  version: WatchVersion | null
  workspaceVersions: WorkspaceVersion[]
  diffs: WatchDiff[]
  elementChanges: Map<number, VersionChangeType>
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

interface WorkspaceVersionContextValue {
  preview: WorkspaceVersionPreview | null
  followToken: number
  setPreview: (preview: WorkspaceVersionPreview | null) => void
  clearPreview: () => void
  requestFollow: () => void
}

const WorkspaceVersionContext = createContext<WorkspaceVersionContextValue | null>(null)

function normalizeChangeType(value: string): VersionChangeType {
  if (value === 'added' || value === 'updated' || value === 'deleted') return value
  return 'changed'
}

export function buildWorkspaceVersionPreview(args: {
  repository: WatchRepository | null
  version: WatchVersion | null
  workspaceVersions: WorkspaceVersion[]
  diffs: WatchDiff[]
}): WorkspaceVersionPreview {
  const elementChanges = new Map<number, VersionChangeType>()
  const connectorChanges = new Map<number, VersionChangeType>()
  const summary = { added: 0, updated: 0, deleted: 0, changed: 0, elements: 0, connectors: 0 }

  args.diffs.forEach((diff) => {
    const change = normalizeChangeType(diff.change_type)
    summary[change] += 1
    if (diff.resource_type === 'element' && diff.resource_id) {
      elementChanges.set(diff.resource_id, change)
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
    diffs: args.diffs,
    elementChanges,
    connectorChanges,
    summary,
  }
}

export function WorkspaceVersionProvider({ children }: { children: React.ReactNode }) {
  const [preview, setPreview] = useState<WorkspaceVersionPreview | null>(null)
  const [followToken, setFollowToken] = useState(0)
  const clearPreview = useCallback(() => setPreview(null), [])
  const requestFollow = useCallback(() => setFollowToken((value) => value + 1), [])
  const value = useMemo(() => ({ preview, followToken, setPreview, clearPreview, requestFollow }), [preview, followToken, clearPreview, requestFollow])
  return <WorkspaceVersionContext.Provider value={value}>{children}</WorkspaceVersionContext.Provider>
}

export function useWorkspaceVersionPreview() {
  const value = useContext(WorkspaceVersionContext)
  if (!value) throw new Error('useWorkspaceVersionPreview must be used within WorkspaceVersionProvider')
  return value
}
