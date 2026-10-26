import { useCallback, useEffect, useRef } from 'react'
import type { FlowFocus } from './model'

type Viewport = { x: number; y: number; zoom: number }
interface Options {
  viewId: number | null
  active: boolean
  recording?: boolean
  open?: boolean
  canvasSize?: { width: number; height: number }
  focus: FlowFocus | null
  getViewport: () => Viewport
  setViewport: (viewport: Viewport) => unknown
  fitView: (options: { nodes: { id: string }[]; padding: number; maxZoom: number; duration: number }) => unknown
}

/** Fit selected highlights automatically, keeping the camera steady during recording. */
export function useFlowCamera({ viewId, active, recording = false, open = active, canvasSize, focus, getViewport, setViewport, fitView }: Options) {
  const saved = useRef<{ viewId: number; viewport: Viewport; width?: number; height?: number } | null>(null)
  const width = canvasSize?.width
  const height = canvasSize?.height
  const previousSize = useRef<{ viewId: number | null; width: number; height: number; open: boolean } | null>(null)
  useEffect(() => {
    if (!width || !height) return
    const previous = previousSize.current
    if (previous?.viewId === viewId && previous.width === width && previous.height === height) return
    previousSize.current = { viewId, width, height, open }
    if (!previous || previous.viewId !== viewId || (!recording && (active || saved.current)) || !(open || previous.open)) return
    const viewport = getViewport()
    // Keep the same world point centered when docking or expanding the editor.
    void setViewport({ ...viewport, x: viewport.x + (width - previous.width) / 2, y: viewport.y + (height - previous.height) / 2 })
  }, [width, height, viewId, open, active, recording, getViewport, setViewport])
  const fit = useCallback((target: FlowFocus) => {
    if (target.viewId !== viewId || !target.elementIds.length) return
    void fitView({ nodes: target.elementIds.map(id => ({ id: String(id) })), padding: 0.3, maxZoom: 1.2, duration: 0 })
  }, [viewId, fitView])
  const elementKey = focus?.elementIds.join(',') ?? ''
  const focusViewId = focus?.viewId
  useEffect(() => {
    if (saved.current && saved.current.viewId !== viewId) saved.current = null
    if (!active) {
      const previous = saved.current
      if (!previous || previous.viewId !== viewId) return
      const frame = requestAnimationFrame(() => {
        saved.current = null
        void setViewport({ ...previous.viewport,
          x: previous.viewport.x + (width && previous.width ? (width - previous.width) / 2 : 0),
          y: previous.viewport.y + (height && previous.height ? (height - previous.height) / 2 : 0),
        })
      })
      return () => cancelAnimationFrame(frame)
    }
    if (viewId === null || focusViewId !== viewId) return
    if (!saved.current) saved.current = { viewId, viewport: getViewport(), width, height }
    if (!elementKey || recording) return
    // Let the canvas measure its new size after the panel docks or changes layout.
    const frame = requestAnimationFrame(() => fit({ viewId, elementIds: elementKey.split(',').map(Number), connectors: [] }))
    return () => cancelAnimationFrame(frame)
  }, [active, recording, viewId, focusViewId, elementKey, getViewport, setViewport, fit, width, height])
}
