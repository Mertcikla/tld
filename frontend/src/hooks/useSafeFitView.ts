import { useCallback, type RefObject } from 'react'
import { useReactFlow } from 'reactflow'

const MIN_CONTAINER_WIDTH = 50
const MIN_CONTAINER_HEIGHT = 50
const NAN_CHECK_DELAY = 80

/**
 * Wraps React Flow's fitView with protections against the d3-zoom NaN
 * poisoning bug that occurs when the container is narrow at load time.
 *
 * Guards:
 * 1. Container width - skips fitView if container is too narrow for d3-zoom
 *    to compute a valid transform.
 * 2. Post-call NaN check - if the viewport is still NaN after the call,
 *    resets to a known-safe fallback.
 *
 * Returns `boolean`: true if fitView was attempted, false if skipped.
 */
export function useSafeFitView(containerRef?: RefObject<HTMLElement | null>) {
  const { fitView, getViewport, setViewport } = useReactFlow()

  const safeFitView = useCallback(
    (options?: Parameters<typeof fitView>[0]): boolean => {
      // Guard: skip if container is too narrow/short for d3-zoom to compute valid transform
      // Zero height causes d3-zoom to produce zoom=0, which leads to NaN in RF's DotPattern
      if (containerRef?.current) {
        const { clientWidth, clientHeight } = containerRef.current
        if (clientWidth < MIN_CONTAINER_WIDTH || clientHeight < MIN_CONTAINER_HEIGHT) return false
      }

      const duration = options?.duration ?? 0
      fitView({ ...options, duration })

      // For instant transitions, check synchronously so the NaN never reaches a render.
      if (duration === 0) {
        const vp = getViewport()
        if (Number.isNaN(vp.x) || Number.isNaN(vp.y) || Number.isNaN(vp.zoom)) {
          setViewport({ x: 0, y: 0, zoom: 1 }, { duration: 0 })
          return false
        }
      }

      // Post-call validation: check viewport after transition completes
      // and reset to a safe fallback if d3-zoom produced NaN
      const checkDelay = Math.max(duration, NAN_CHECK_DELAY)
      setTimeout(() => {
        const vp = getViewport()
        if (Number.isNaN(vp.x) || Number.isNaN(vp.y) || Number.isNaN(vp.zoom)) {
          setViewport({ x: 0, y: 0, zoom: 1 }, { duration: 0 })
        }
      }, checkDelay)

      return true
    },
    [fitView, getViewport, setViewport, containerRef],
  )

  return { safeFitView }
}
