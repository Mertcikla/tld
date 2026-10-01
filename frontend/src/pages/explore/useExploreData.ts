import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../../api/client'
import type { ExploreData } from '../../types'
import { primeWorkspaceGraphSnapshot } from '../../crossBranch/store'

export interface ExploreDataState {
  data: ExploreData | null
  loading: boolean
  error: string | null
  hasPlacements: boolean
  reload: () => void
}

export function useExploreData(sharedToken?: string): ExploreDataState {
  const [data, setData] = useState<ExploreData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const loadExploreData = useCallback(() => {
    setLoading(true)
    setError(null)
    const loader = sharedToken ? api.explore.loadShared(sharedToken) : api.explore.load()
    loader.then((loaded) => {
      if (loaded.password_required) {
        setData(null)
      } else {
        primeWorkspaceGraphSnapshot(loaded)
        setData(loaded)
      }
    }).catch((loadError: unknown) => {
      setError(loadError instanceof Error ? loadError.message : 'Explore data could not be loaded.')
      setData(null)
    }).finally(() => setLoading(false))
  }, [sharedToken])

  useEffect(() => {
    loadExploreData()
  }, [loadExploreData])

  const hasPlacements = useMemo(() => {
    if (!data || !data.views) return false
    return Object.values(data.views).some((view) => (view?.placements ?? []).length > 0)
  }, [data])

  return {
    data,
    loading,
    error,
    hasPlacements,
    reload: loadExploreData,
  }
}
