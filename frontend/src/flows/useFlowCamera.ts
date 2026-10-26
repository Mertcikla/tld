import { useEffect, useRef } from 'react'
import type { FlowFocus } from './model'

interface Options {
  viewId: number | null
  active: boolean
  recording?: boolean
  focus: FlowFocus | null
  fitView: (options: { nodes: { id: string }[]; padding: number; maxZoom: number; duration: number }) => unknown
}

/** Fit new highlights; panel visibility and geometry never own the viewport. */
export function useFlowCamera({ viewId, active, recording = false, focus, fitView }: Options) {
  const lastTarget = useRef('')
  const elementKey = focus?.elementIds.join(',') ?? ''
  const focusViewId = focus?.viewId
  useEffect(() => {
    // Resume fitting the current step after picking or recording finishes.
    if (recording) { lastTarget.current = ''; return }
    if (!active || viewId === null || focusViewId !== viewId || !elementKey) return
    const target = `${viewId}:${elementKey}`
    // Reopening the same story must not undo a user's pan or zoom.
    if (lastTarget.current === target) return
    const frame = requestAnimationFrame(() => {
      lastTarget.current = target
      void fitView({ nodes: elementKey.split(',').map(id => ({ id })), padding: 0.3, maxZoom: 1.2, duration: 0 })
    })
    return () => cancelAnimationFrame(frame)
  }, [active, recording, viewId, focusViewId, elementKey, fitView])
}
