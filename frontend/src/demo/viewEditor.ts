import { useCallback, useEffect, useMemo, useRef, type MutableRefObject, type RefObject } from 'react'
import { getNodesBounds, getViewportForBounds, type Node as RFNode, type ReactFlowInstance } from 'reactflow'

export interface ViewEditorDemoOptions {
  revealProgress?: number
  disableImportExport?: boolean
  hideFlowControls?: boolean
  disableOnboarding?: boolean
}

export const DEMO_VIEW_EDITOR_OPTIONS: Omit<ViewEditorDemoOptions, 'revealProgress'> = {
  disableImportExport: true,
  hideFlowControls: true,
  disableOnboarding: true,
}

function getCenteredViewport(bounds: { x: number; y: number; width: number; height: number }, width: number, height: number, zoom: number) {
  const centerX = bounds.x + bounds.width / 2
  const centerY = bounds.y + bounds.height / 2

  return {
    x: width / 2 - centerX * zoom,
    y: height / 2 - centerY * zoom,
    zoom,
  }
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
  resetKey,
}: UseDemoRevealViewportArgs) {
  const clampedRevealProgress = useMemo(() => {
    if (typeof demoOptions?.revealProgress !== 'number') return null
    return Math.max(0, Math.min(1, demoOptions.revealProgress))
  }, [demoOptions?.revealProgress])

  const revealZoomRef = useRef<number | null>(null)

  useEffect(() => {
    revealZoomRef.current = null
  }, [resetKey, clampedRevealProgress])

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

    if (clampedRevealProgress >= 0.999) {
      revealZoomRef.current = fittedViewport.zoom
      setViewport(fittedViewport, { duration: 0 })
      return true
    }

    const fixedZoom = revealZoomRef.current ?? fittedViewport.zoom
    revealZoomRef.current = fixedZoom

    const centeredViewport = getCenteredViewport(bounds, width, height, fixedZoom)
    const reveal = 1 - Math.pow(1 - clampedRevealProgress, 3)
    const hiddenOffsetX = Math.max(width * 1.15, bounds.width * fixedZoom * 0.75)

    setViewport({
      x: centeredViewport.x + hiddenOffsetX * (1 - reveal),
      y: centeredViewport.y,
      zoom: fixedZoom,
    }, { duration: 0 })

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