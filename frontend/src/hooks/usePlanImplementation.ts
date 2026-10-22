import { useCallback, useEffect, useRef, useState } from 'react'
import type { ImplementationResponse } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/plan_service_pb'
import { plansClient, planProjection, type PlanResource } from '../api/plans'

export function usePlanImplementation(repositoryId: string, gitRevision: string, enabled: boolean) {
  const [state, setState] = useState<ImplementationResponse>()
  const [context, setContext] = useState<PlanResource[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const generation = useRef(0)
  const refresh = useCallback(async (current: number) => {
    const next = await plansClient.getImplementation({ repositoryId, gitRevision })
    const projection = next.locked ? await planProjection(next.planId, true) : null
    if (generation.current !== current) return
    setState(next)
    setContext(projection?.resources.flatMap(item => item.resource ? [item.resource] : []) ?? [])
  }, [repositoryId, gitRevision])
  useEffect(() => {
    const current = ++generation.current
    setState(undefined)
    setContext([])
    setError('')
    if (!enabled || !repositoryId) return
    let timer: ReturnType<typeof setTimeout>
    const poll = async () => {
      try { await refresh(current) } catch (e) { if (generation.current === current) setError(String(e)) }
      if (generation.current === current) timer = setTimeout(() => void poll(), 2000)
    }
    void poll()
    return () => { generation.current = current + 1; clearTimeout(timer) }
  }, [enabled, repositoryId, refresh])
  const review = async (action: string, operationId = '', observedHash = '') => {
    if (!state || busy) return
    const current = generation.current
    setBusy(true)
    setError('')
    try {
      await plansClient.reviewImplementation({ repositoryId, expectedRevision: state.revision, action, operationId, observedHash })
      await refresh(current)
    } catch (e) { if (generation.current === current) setError(String(e)) } finally { setBusy(false) }
  }
  return { state, context, error, busy, review }
}
