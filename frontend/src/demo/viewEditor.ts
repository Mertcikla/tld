import { useCallback, useEffect, useMemo, type MutableRefObject, type RefObject } from 'react'
import { getNodesBounds, getViewportForBounds, type Node as RFNode, type ReactFlowInstance } from 'reactflow'

export interface ViewEditorDemoOptions {
  revealProgress?: number
  disableImportExport?: boolean
  hideFlowControls?: boolean
  disableOnboarding?: boolean
  hideFocusView?: boolean
  hideExpandExtras?: boolean
}

export const DEMO_VIEW_EDITOR_OPTIONS: Omit<ViewEditorDemoOptions, 'revealProgress'> = {
  disableImportExport: true,
  hideFlowControls: true,
  disableOnboarding: true,
  hideFocusView: true,
  hideExpandExtras: true,
}



interface UseDemoRevealViewportArgs {
  demoOptions?: ViewEditorDemoOptions
  containerRef: RefObject<HTMLDivElement | null>
  rfNodesRef: MutableRefObject<RFNode[]>
  rfReadyRef: MutableRefObject<boolean>
  needsFitViewRef: MutableRefObject<boolean>
  computedMinZoom: number
  setViewport: ReactFlowInstance['setViewport']
  resetKey: number | null
}

export function useDemoRevealViewport({
  demoOptions,
  containerRef,
  rfNodesRef,
  rfReadyRef,
  needsFitViewRef,
  computedMinZoom,
  setViewport,
}: UseDemoRevealViewportArgs) {
  const clampedRevealProgress = useMemo(() => {
    if (typeof demoOptions?.revealProgress !== 'number') return null
    return Math.max(0, Math.min(1, demoOptions.revealProgress))
  }, [demoOptions?.revealProgress])

  const applyDemoRevealViewport = useCallback(() => {
    if (clampedRevealProgress === null) return false

    const el = containerRef.current
    const nodes = rfNodesRef.current
    if (!el || nodes.length === 0) return false
    if (!nodes.every((node) => typeof node.width === 'number' && node.width > 0 && typeof node.height === 'number' && node.height > 0)) return false

    const width = el.clientWidth
    const height = el.clientHeight
    if (width < 10 || height < 10) return false

    const bounds = getNodesBounds(nodes)
    const fittedViewport = getViewportForBounds(bounds, width, height, computedMinZoom, 2, 0.1)
    if (![fittedViewport.x, fittedViewport.y, fittedViewport.zoom].every(Number.isFinite)) return false

    setViewport(fittedViewport, { duration: 0 })

    return true
  }, [clampedRevealProgress, computedMinZoom, containerRef, rfNodesRef, setViewport])

  useEffect(() => {
    if (clampedRevealProgress === null || !rfReadyRef.current) return
    const ok = applyDemoRevealViewport()
    if (ok && clampedRevealProgress >= 0.999) needsFitViewRef.current = false
  }, [applyDemoRevealViewport, clampedRevealProgress, needsFitViewRef, rfReadyRef])

  return {
    clampedRevealProgress,
    applyDemoRevealViewport,
    disableImportExport: demoOptions?.disableImportExport ?? false,
    hideFlowControls: demoOptions?.hideFlowControls ?? false,
  }
}
