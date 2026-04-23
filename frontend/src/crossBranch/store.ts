import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { ExploreData } from '../types'
import { buildWorkspaceGraphSnapshot, removeConnectorFromSnapshot, removePlacementFromSnapshot, upsertConnectorInSnapshot, upsertPlacementInSnapshot } from './graph'
import type { WorkspaceGraphSnapshot } from './types'

let cachedSnapshot: WorkspaceGraphSnapshot | null = null
let inflightLoad: Promise<WorkspaceGraphSnapshot> | null = null
const listeners = new Set<(snapshot: WorkspaceGraphSnapshot | null) => void>()

function publish(snapshot: WorkspaceGraphSnapshot | null) {
  cachedSnapshot = snapshot
  for (const listener of listeners) listener(snapshot)
}

export function primeWorkspaceGraphSnapshot(data: ExploreData) {
  publish(buildWorkspaceGraphSnapshot(data))
}

export async function loadWorkspaceGraphSnapshot(force = false): Promise<WorkspaceGraphSnapshot> {
  if (!force && cachedSnapshot) return cachedSnapshot
  if (!force && inflightLoad) return inflightLoad
  inflightLoad = api.explore.load().then((data) => {
    const snapshot = buildWorkspaceGraphSnapshot(data)
    publish(snapshot)
    inflightLoad = null
    return snapshot
  }).catch((error) => {
    inflightLoad = null
    throw error
  })
  return inflightLoad
}

export function subscribeWorkspaceGraphSnapshot(listener: (snapshot: WorkspaceGraphSnapshot | null) => void) {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

export function useWorkspaceGraphSnapshot(enabled = true) {
  const [snapshot, setSnapshot] = useState<WorkspaceGraphSnapshot | null>(cachedSnapshot)

  useEffect(() => subscribeWorkspaceGraphSnapshot(setSnapshot), [])

  useEffect(() => {
    if (!enabled) return
    if (cachedSnapshot) {
      setSnapshot(cachedSnapshot)
      return
    }
    void loadWorkspaceGraphSnapshot().catch(() => { /* intentionally empty */ })
  }, [enabled])

  return snapshot
}

export function upsertConnectorGraphSnapshot(connector: Parameters<typeof upsertConnectorInSnapshot>[1]) {
  publish(upsertConnectorInSnapshot(cachedSnapshot, connector))
}

export function removeConnectorGraphSnapshot(viewId: number, connectorId: number) {
  publish(removeConnectorFromSnapshot(cachedSnapshot, viewId, connectorId))
}

export function upsertPlacementGraphSnapshot(viewId: number, placement: Parameters<typeof upsertPlacementInSnapshot>[2]) {
  publish(upsertPlacementInSnapshot(cachedSnapshot, viewId, placement))
}

export function removePlacementGraphSnapshot(viewId: number, elementId: number) {
  publish(removePlacementFromSnapshot(cachedSnapshot, viewId, elementId))
}
