import React, { useRef, useEffect, useCallback, forwardRef, useImperativeHandle, useState } from 'react'
import { useReactFlow } from 'reactflow'
import { ACCENT_DEFAULT } from '../constants/colors'
import { getDrawingPathData, pointFromPointerEvent } from './DrawingCanvas.freehand'

export type DrawingPoint = { x: number; y: number; pressure?: number }

export type DrawingPath = {
  id: string
  points: DrawingPoint[]
  color: string
  width: number
  text?: string
  fontSize?: number
}

interface DrawingCanvasProps {
  paths: DrawingPath[]
  isDrawing: boolean
  isVisible: boolean
  strokeColor?: string
  strokeWidth?: number
  mode?: 'pencil' | 'eraser' | 'text' | 'select'
  onPathComplete: (path: DrawingPath) => void
  onPathDelete?: (pathId: string) => void
  onPathUpdate?: (path: DrawingPath) => void
  onTextPositionSelected?: (canvasX: number, canvasY: number, flowX: number, flowY: number) => void
}

export interface DrawingCanvasHandle {
  /** Imperatively update the viewport and redraw avoids React re-renders on every pan frame. */
  notifyViewportChange: (vp: { x: number; y: number; zoom: number }) => void
}

/**
 * Free-drawing overlay with smooth ink-like strokes, hit detection for move/erase,
 * and a broad eraser brush.
 */
function trySetPointerCapture(canvas: HTMLCanvasElement, pointerId: number) {
  try {
    canvas.setPointerCapture(pointerId)
  } catch (e) {
    // Firefox rejects capture for synthetic or otherwise non-active pointer ids.
    // The event data is still usable, so drawing can continue without capture.
    if (e instanceof DOMException && (e.name === 'InvalidPointerId' || e.name === 'NotFoundError')) return
    throw e
  }
}

const DrawingCanvas = forwardRef<DrawingCanvasHandle, DrawingCanvasProps>(function DrawingCanvas({
  paths,
  isDrawing,
  isVisible,
  strokeColor = ACCENT_DEFAULT,
  strokeWidth = 3,
  mode = 'pencil',
  onPathComplete,
  onPathDelete,
  onPathUpdate,
  onTextPositionSelected,
}: DrawingCanvasProps, ref: React.ForwardedRef<DrawingCanvasHandle>) {
  const { getViewport } = useReactFlow()
  const canvasRef = useRef<HTMLCanvasElement>(null)

  // Internal selection state (not lifted to parent to keep drag smooth)
  const [selectedPathId, setSelectedPathId] = useState<string | null>(null)

  // Refs for values needed inside async/event callbacks without stale closure issues
  const viewportRef = useRef(getViewport())
  const pathsRef = useRef(paths)
  const isVisibleRef = useRef(isVisible)
  const strokeColorRef = useRef(strokeColor)
  const strokeWidthRef = useRef(strokeWidth)
  const modeRef = useRef(mode)
  const currentPathRef = useRef<DrawingPoint[]>([])
  const isPointerDownRef = useRef(false)
  const activePointerIdRef = useRef<number | null>(null)
  const dragStartRef = useRef<{ x: number, y: number } | null>(null)
  const pathCloneRef = useRef<DrawingPath | null>(null)
  const devicePixelRatioRef = useRef(1)
  const redrawFrameRef = useRef<number | null>(null)

  useEffect(() => { pathsRef.current = paths }, [paths])
  useEffect(() => { isVisibleRef.current = isVisible }, [isVisible])
  useEffect(() => { strokeColorRef.current = strokeColor }, [strokeColor])
  useEffect(() => { strokeWidthRef.current = strokeWidth }, [strokeWidth])
  useEffect(() => { modeRef.current = mode }, [mode])

  const getPathBounds = (path: DrawingPath) => {
    if (path.points.length === 0) return { minX: 0, minY: 0, maxX: 0, maxY: 0 }
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    for (const p of path.points) {
      if (p.x < minX) minX = p.x
      if (p.y < minY) minY = p.y
      if (p.x > maxX) maxX = p.x
      if (p.y > maxY) maxY = p.y
    }
    const pad = (path.width || 10) / 2 + 2
    return { minX: minX - pad, minY: minY - pad, maxX: maxX + pad, maxY: maxY + pad }
  }

  const drawInkPath = useCallback((ctx: CanvasRenderingContext2D, points: DrawingPoint[], color: string, width: number, last: boolean) => {
    const pathData = getDrawingPathData(points, width, last)
    ctx.fillStyle = color
    if (pathData) {
      ctx.fill(new Path2D(pathData))
      return
    }

    if (points.length === 1) {
      ctx.beginPath()
      ctx.arc(points[0].x, points[0].y, width / 2, 0, Math.PI * 2)
      ctx.fill()
    }
  }, [])

  // ── Redraw all committed paths ────────────────────────────────────────────
  const redrawNow = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const vp = viewportRef.current
    const committed = pathsRef.current
    const visible = isVisibleRef.current
    const dpr = devicePixelRatioRef.current

    ctx.setTransform(1, 0, 0, 1, 0, 0)
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    if (!visible) return

    ctx.save()
    ctx.setTransform(dpr * vp.zoom, 0, 0, dpr * vp.zoom, dpr * vp.x, dpr * vp.y)

    for (const path of committed) {
      if (path.points.length === 0) continue

      if (path.text) {
        ctx.fillStyle = path.color
        ctx.font = `${path.fontSize || 16}px Josefin Sans, sans-serif`
        ctx.textBaseline = 'middle'
        ctx.fillText(path.text, path.points[0].x, path.points[0].y)
      } else {
        drawInkPath(ctx, path.points, path.color, path.width, true)
      }

      // Selection halo
      if (path.id === selectedPathId) {
        const bounds = getPathBounds(path)
        ctx.setLineDash([5, 5])
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.5)'
        ctx.lineWidth = 1 / vp.zoom
        ctx.strokeRect(bounds.minX, bounds.minY, bounds.maxX - bounds.minX, bounds.maxY - bounds.minY)
        ctx.setLineDash([])
      }
    }

    // Active drawing path
    const activePts = currentPathRef.current
    if (activePts.length > 0) {
      drawInkPath(ctx, activePts, strokeColorRef.current, strokeWidthRef.current, false)
    }

    ctx.restore()
  }, [selectedPathId, drawInkPath])

  const scheduleRedraw = useCallback(() => {
    if (redrawFrameRef.current !== null) return
    redrawFrameRef.current = window.requestAnimationFrame(() => {
      redrawFrameRef.current = null
      redrawNow()
    })
  }, [redrawNow])

  useImperativeHandle(ref, () => ({
    notifyViewportChange(vp) {
      viewportRef.current = vp
      scheduleRedraw()
    },
  }), [scheduleRedraw])

  useEffect(() => {
    scheduleRedraw()
  }, [paths, isVisible, scheduleRedraw])

  useEffect(() => {
    const canvas = canvasRef.current
    const parent = canvas?.parentElement
    if (!canvas || !parent) return

    const resizeCanvas = () => {
      const rect = parent.getBoundingClientRect()
      const dpr = Math.max(1, window.devicePixelRatio || 1)
      devicePixelRatioRef.current = dpr
      canvas.width = Math.max(1, Math.round(rect.width * dpr))
      canvas.height = Math.max(1, Math.round(rect.height * dpr))
      scheduleRedraw()
    }

    const ro = new ResizeObserver(resizeCanvas)
    ro.observe(parent)
    resizeCanvas()

    return () => {
      ro.disconnect()
      if (redrawFrameRef.current !== null) {
        window.cancelAnimationFrame(redrawFrameRef.current)
        redrawFrameRef.current = null
      }
    }
  }, [scheduleRedraw])

  const screenToFlow = useCallback((sx: number, sy: number): DrawingPoint => {
    const { x, y, zoom } = viewportRef.current
    return { x: (sx - x) / zoom, y: (sy - y) / zoom }
  }, [])

  const clientToFlowPoint = useCallback((canvas: HTMLCanvasElement, clientX: number, clientY: number): DrawingPoint => {
    const rect = canvas.getBoundingClientRect()
    return screenToFlow(clientX - rect.left, clientY - rect.top)
  }, [screenToFlow])

  const getPointerFlowPoints = useCallback((e: React.PointerEvent<HTMLCanvasElement>, canvas: HTMLCanvasElement): DrawingPoint[] => {
    const nativeEvent = e.nativeEvent
    const coalesced = typeof nativeEvent.getCoalescedEvents === 'function'
      ? nativeEvent.getCoalescedEvents()
      : []
    const events = coalesced.length > 0 ? coalesced : [nativeEvent]
    return events.map((event) => pointFromPointerEvent(event, (clientX, clientY) => clientToFlowPoint(canvas, clientX, clientY)))
  }, [clientToFlowPoint])

  function distToSegment(p: DrawingPoint, v: DrawingPoint, w: DrawingPoint) {
    const l2 = Math.pow(v.x - w.x, 2) + Math.pow(v.y - w.y, 2)
    if (l2 === 0) return Math.hypot(p.x - v.x, p.y - v.y)
    let t = ((p.x - v.x) * (w.x - v.x) + (p.y - v.y) * (w.y - v.y)) / l2
    t = Math.max(0, Math.min(1, t))
    return Math.hypot(p.x - (v.x + t * (w.x - v.x)), p.y - (v.y + t * (w.y - v.y)))
  }

  const findPathAt = useCallback((flowPt: DrawingPoint) => {
    // Reverse to find top-most
    return [...pathsRef.current].reverse().find((path) => {
      if (path.text) {
        const dist = Math.hypot(path.points[0].x - flowPt.x, path.points[0].y - flowPt.y)
        return dist < (path.fontSize || 16)
      }
      for (let i = 0; i < path.points.length - 1; i++) {
        if (distToSegment(flowPt, path.points[i], path.points[i + 1]) < path.width + 10) return true
      }
      return path.points.length === 1 && Math.hypot(path.points[0].x - flowPt.x, path.points[0].y - flowPt.y) < path.width + 10
    })
  }, [])

  const onPointerDown = useCallback((e: React.PointerEvent<HTMLCanvasElement>) => {
    if (!isDrawing) return
    e.preventDefault()
    e.stopPropagation()

    const canvas = canvasRef.current
    if (!canvas) return

    const rect = canvas.getBoundingClientRect()
    const flowPt = screenToFlow(e.clientX - rect.left, e.clientY - rect.top)

    if (modeRef.current === 'eraser') {
      trySetPointerCapture(canvas, e.pointerId)
      activePointerIdRef.current = e.pointerId
      isPointerDownRef.current = true
      const hit = findPathAt(flowPt)
      if (hit && onPathDelete) onPathDelete(hit.id)
      return
    }

    if (modeRef.current === 'select') {
      const hit = findPathAt(flowPt)
      if (hit) {
        trySetPointerCapture(canvas, e.pointerId)
        activePointerIdRef.current = e.pointerId
        setSelectedPathId(hit.id)
        isPointerDownRef.current = true
        dragStartRef.current = flowPt
        pathCloneRef.current = { ...hit, points: hit.points.map(p => ({ ...p })) }
      } else {
        setSelectedPathId(null)
      }
      return
    }

    if (modeRef.current === 'text') {
      onTextPositionSelected?.(e.clientX - rect.left, e.clientY - rect.top, flowPt.x, flowPt.y)
      return
    }

    trySetPointerCapture(canvas, e.pointerId)
    activePointerIdRef.current = e.pointerId
    isPointerDownRef.current = true
    currentPathRef.current = getPointerFlowPoints(e, canvas)
    scheduleRedraw()
  }, [isDrawing, screenToFlow, findPathAt, onPathDelete, onTextPositionSelected, getPointerFlowPoints, scheduleRedraw])

  const onPointerMove = useCallback((e: React.PointerEvent<HTMLCanvasElement>) => {
    if (!isDrawing || !isPointerDownRef.current) return
    e.preventDefault()

    const canvas = canvasRef.current
    if (!canvas) return

    const rect = canvas.getBoundingClientRect()
    const flowPt = screenToFlow(e.clientX - rect.left, e.clientY - rect.top)

    if (modeRef.current === 'eraser') {
      const hit = findPathAt(flowPt)
      if (hit && onPathDelete) onPathDelete(hit.id)
      return
    }

    if (modeRef.current === 'select' && selectedPathId && dragStartRef.current && pathCloneRef.current) {
      const dx = flowPt.x - dragStartRef.current.x
      const dy = flowPt.y - dragStartRef.current.y

      const updatedPath = {
        ...pathCloneRef.current,
        points: pathCloneRef.current.points.map(p => ({ ...p, x: p.x + dx, y: p.y + dy }))
      }

      // Local update for smoothness
      pathsRef.current = pathsRef.current.map(p => p.id === selectedPathId ? updatedPath : p)
      scheduleRedraw()
      return
    }

    if (modeRef.current === 'pencil') {
      currentPathRef.current.push(...getPointerFlowPoints(e, canvas))
      scheduleRedraw()
    }
  }, [isDrawing, screenToFlow, scheduleRedraw, selectedPathId, findPathAt, onPathDelete, getPointerFlowPoints])

  const onPointerUp = useCallback((e: React.PointerEvent<HTMLCanvasElement>) => {
    if (!isPointerDownRef.current) return
    isPointerDownRef.current = false
    const canvas = canvasRef.current
    if (canvas && activePointerIdRef.current !== null && canvas.hasPointerCapture(activePointerIdRef.current)) {
      canvas.releasePointerCapture(activePointerIdRef.current)
    }
    activePointerIdRef.current = null

    if (modeRef.current === 'select' && selectedPathId) {
      const finalPath = pathsRef.current.find(p => p.id === selectedPathId)
      if (finalPath && onPathUpdate) onPathUpdate(finalPath)
      dragStartRef.current = null
      pathCloneRef.current = null
      return
    }

    if (modeRef.current === 'pencil' && canvas) {
      currentPathRef.current.push(...getPointerFlowPoints(e, canvas))
    }

    const pts = [...currentPathRef.current]
    currentPathRef.current = []

    if (pts.length >= 1 && modeRef.current === 'pencil') {
      onPathComplete({
        id: `path-${Date.now()}-${Math.random().toString(36).slice(2)}`,
        points: pts,
        color: strokeColorRef.current,
        width: strokeWidthRef.current,
      })
    }
    scheduleRedraw()
  }, [onPathComplete, onPathUpdate, selectedPathId, getPointerFlowPoints, scheduleRedraw])

  // Delete key handler
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!isDrawing) return
      if ((e.key === 'Delete' || e.key === 'Backspace') && selectedPathId) {
        onPathDelete?.(selectedPathId)
        setSelectedPathId(null)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [selectedPathId, isDrawing, onPathDelete])

  return (
    <canvas
      data-testid="drawing-canvas"
      data-path-count={paths.length}
      data-pressure-point-count={paths.reduce((count, path) => count + path.points.filter((point) => typeof point.pressure === 'number').length, 0)}
      data-drawing-tool={mode}
      data-drawing-visible={isVisible ? 'true' : 'false'}
      ref={canvasRef}
      style={{
        position: 'absolute',
        top: 0,
        left: 0,
        width: '100%',
        height: '100%',
        pointerEvents: isDrawing ? 'auto' : 'none',
        cursor: isDrawing ? (
          mode === 'text' ? 'text' :
          mode === 'eraser' ? 'url("data:image/svg+xml,%3Csvg xmlns=\'http://www.w3.org/2000/svg\' width=\'24\' height=\'24\' viewBox=\'0 0 24 24\' fill=\'none\' stroke=\'white\' stroke-width=\'2\' stroke-linecap=\'round\' stroke-linejoin=\'round\'%3E%3Cpath d=\'M20 20H7L3 16C2 15 2 13 3 12L13 2L22 11L20 13L17 10L10 17L13 20\'/%3E%3C/svg%3E") 6 18, auto' :
          mode === 'select' ? 'move' : 'crosshair'
        ) : 'default',
        opacity: isVisible ? 1 : 0,
        transition: 'opacity 0.15s ease',
        zIndex: 10,
        touchAction: 'none',
        userSelect: 'none',
        overscrollBehavior: 'none',
        WebkitUserSelect: 'none',
      }}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
    />
  )
})

export default DrawingCanvas
