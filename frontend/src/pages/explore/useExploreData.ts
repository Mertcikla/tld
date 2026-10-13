import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../../api/client'
import type { ExploreData } from '../../types'
import { primeWorkspaceGraphSnapshot } from '../../crossBranch/store'
import { withUnplacedElements } from '../../crossBranch/unplacedElements'

export interface ExploreDataState {
  data: ExploreData | null
  loading: boolean
  error: string | null
  hasPlacements: boolean
  reload: () => Promise<ExploreData | null>
}

export function useExploreData(sharedToken?: string, enabled = true): ExploreDataState {
  const [data, setData] = useState<ExploreData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const pendingLoad = useRef<{ token?: string; promise: Promise<ExploreData | null> } | null>(null)

  const loadExploreData = useCallback(() => {
    if (pendingLoad.current && pendingLoad.current.token === sharedToken) return pendingLoad.current.promise
    setLoading(true)
    setError(null)
    const loader = sharedToken ? api.explore.loadShared(sharedToken) : api.explore.load()
    const promise: Promise<ExploreData | null> = loader.then((loaded) => {
      if (pendingLoad.current?.promise !== promise) return null
      if (loaded.password_required) {
        setData(null)
        return null
      } else {
        primeWorkspaceGraphSnapshot(loaded)
        setData(loaded)
        return loaded
      }
    }).catch((loadError: unknown) => {
      if (pendingLoad.current?.promise === promise) {
        setError(loadError instanceof Error ? loadError.message : 'Explore data could not be loaded.')
        setData(null)
      }
      return null
    }).finally(() => {
      if (pendingLoad.current?.promise === promise) {
        pendingLoad.current = null
        setLoading(false)
      }
    })
    pendingLoad.current = { token: sharedToken, promise }
    return promise
  }, [sharedToken])

  useEffect(() => {
    if (enabled) void loadExploreData()
  }, [enabled, loadExploreData])

  const hasPlacements = useMemo(() => {
    if (!data || !data.views) return false
    return Object.values(withUnplacedElements(data).views).some((view) => (view?.placements ?? []).length > 0)
  }, [data])

  return {
    data,
    loading,
    error,
    hasPlacements,
    reload: loadExploreData,
  }
}
