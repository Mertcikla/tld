import { useCallback, useEffect, useRef, useState } from 'react'
import type { ImplementationResponse } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/plan_service_pb'
import { planChanged, plansClient, planProjection, type Plan, type PlanResource } from '../api/plans'

export function usePlanImplementation(repositoryId: string, gitRevision: string, enabled: boolean, includeContext = true) {
  const [state, setState] = useState<ImplementationResponse>()
  const [plan, setPlan] = useState<Plan>()
  const [context, setContext] = useState<PlanResource[]>([])
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const generation = useRef(0)
  const reviewing = useRef(false)
  const refreshRequest = useRef(0)
  const cached = useRef('')
  const git = useRef(gitRevision)
  git.current = gitRevision
  const refresh = useCallback(async (current: number) => {
    const request = ++refreshRequest.current
    const next = await plansClient.getImplementation({ repositoryId, gitRevision: git.current })
    if (generation.current !== current || request !== refreshRequest.current) return
    setState(previous => previous?.revision === next.revision && previous.locked === next.locked && previous.planId === next.planId ? previous : next)
    setError('')
    if (!next.locked) { cached.current = ''; setPlan(undefined); setContext([]); return }
    if (cached.current !== next.planId) {
      const { plan: specification } = await plansClient.getPlan({ planId: next.planId })
      const projection = includeContext && specification ? await planProjection(next.planId, true, 0, specification.readyRevision || specification.revision) : null
      if (generation.current !== current) return
      setPlan(specification)
      setContext(projection?.resources.flatMap(item => item.resource ? [item.resource] : []) ?? [])
      cached.current = next.planId
    }
  }, [repositoryId, includeContext])
  useEffect(() => {
    const current = ++generation.current
    cached.current = ''
    setState(undefined); setPlan(undefined); setContext([]); setError(''); setMessage('')
    if (!enabled || typeof window === 'undefined') return
    let timer: ReturnType<typeof setTimeout>
    let polling = false
    const poll = async () => {
      if (polling || generation.current !== current) return
      polling = true
      clearTimeout(timer)
      try { if (!reviewing.current) await refresh(current) }
      catch (e) { if (generation.current === current) setError(String(e)) }
      finally { polling = false }
      if (generation.current === current) timer = setTimeout(() => void poll(), 5000)
    }
    const changed = () => { void poll() }
    window.addEventListener('tld:plan-changed', changed)
    void poll()
    return () => { generation.current = current + 1; clearTimeout(timer); window.removeEventListener('tld:plan-changed', changed) }
  }, [enabled, refresh])
  const review = async (action: string, operationId = '', observedHash = '') => {
    if (!state?.locked || reviewing.current) return
    const current = generation.current
    reviewing.current = true
    refreshRequest.current++
    setBusy(true); setError(''); setMessage('')
    try {
      const result = await plansClient.reviewImplementation({ repositoryId: state.repositoryId, expectedRevision: state.revision, action, operationId, observedHash })
      if (generation.current !== current) return
      const applied = result.edits.filter(edit => edit.status === 'committed').length
      const remaining = result.edits.filter(edit => !['committed', 'skipped', 'discarded'].includes(edit.status)).length
      setMessage(result.locked ? `${applied} applied · ${remaining} remaining` : action === 'cancel' ? 'Implementation ended. Pending diagram updates discarded; source files are unchanged.' : 'Implementation finished. Workspace released.')
      setState(result)
      cached.current = ''
      planChanged(result.planId)
      await refresh(current)
    } catch (e) {
      if (generation.current === current) {
        setError(String(e))
        // Refresh the expected revision for an explicit retry after a conflict.
        try { await refresh(current) } catch { /* Keep the original failure visible. */ }
        setError(String(e))
      }
    } finally { reviewing.current = false; setBusy(false) }
  }
  return { state, plan, context, error, message, busy, review }
}
