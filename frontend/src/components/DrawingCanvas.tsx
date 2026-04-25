import React, { useRef, useEffect, useCallback, forwardRef, useImperativeHandle, useState } from 'react'
import { useReactFlow } from 'reactflow'
import { ACCENT_DEFAULT } from '../constants/colors'

export type DrawingPoint = { x: number; y: number }

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
 * A premium free-drawing overlay with smooth ink-like strokes, hit detection for move/erase,
 * and a broad eraser brush.
 */
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
  const dragStartRef = useRef<{ x: number, y: number } | null>(null)
  const pathCloneRef = useRef<DrawingPath | null>(null)

  useEffect(() => { pathsRef.current = paths }, [paths])
  useEffect(() => { isVisibleRef.current = isVisible }, [isVisible])
  useEffect(() => { strokeColorRef.current = strokeColor }, [strokeColor])
  useEffect(() => { strokeWidthRef.current = strokeWidth }, [strokeWidth])
  useEffect(() => { modeRef.current = mode }, [mode])

  // Helper: Draw a smooth Catmull-Rom spline on a canvas context
  const drawSpline = useCallback((ctx: CanvasRenderingContext2D, points: DrawingPoint[], tension = 0.5) => {
    if (points.length < 2) return

    ctx.beginPath()
    ctx.moveTo(points[0].x, points[0].y)

    if (points.length === 2) {
      ctx.lineTo(points[1].x, points[1].y)
      ctx.stroke()
      return
    }

    for (let i = 0; i < points.length - 1; i++) {
      const p0 = i > 0 ? points[i - 1] : points[i]
      const p1 = points[i]
      const p2 = points[i + 1]
      const p3 = i < points.length - 2 ? points[i + 2] : p2

      const cp1x = p1.x + (p2.x - p0.x) / 6 * tension
      const cp1y = p1.y + (p2.y - p0.y) / 6 * tension
      const cp2x = p2.x - (p3.x - p1.x) / 6 * tension
      const cp2y = p2.y - (p3.y - p1.y) / 6 * tension

      ctx.bezierCurveTo(cp1x, cp1y, cp2x, cp2y, p2.x, p2.y)
    }
    ctx.stroke()
  }, [])

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

  // ── Redraw all committed paths ────────────────────────────────────────────
  const redraw = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const vp = viewportRef.current
    const committed = pathsRef.current
    const visible = isVisibleRef.current

    ctx.clearRect(0, 0, canvas.width, canvas.height)
    if (!visible) return

    ctx.save()
    ctx.setTransform(vp.zoom, 0, 0, vp.zoom, vp.x, vp.y)

    for (const path of committed) {
      if (path.points.length === 0) continue

      if (path.text) {
        ctx.fillStyle = path.color
        ctx.font = `${path.fontSize || 16}px Josefin Sans, sans-serif`
        ctx.textBaseline = 'middle'
        ctx.fillText(path.text, path.points[0].x, path.points[0].y)
      } else {
        ctx.strokeStyle = path.color
        ctx.lineWidth = path.width
        ctx.lineCap = 'round'
        ctx.lineJoin = 'round'

        if (path.points.length === 1) {
          ctx.beginPath()
          ctx.arc(path.points[0].x, path.points[0].y, path.width / 2, 0, Math.PI * 2)
          ctx.fillStyle = path.color
          ctx.fill()
        } else {
          drawSpline(ctx, path.points)
        }
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
      ctx.strokeStyle = strokeColorRef.current
      ctx.lineWidth = strokeWidthRef.current
      ctx.lineCap = 'round'
      ctx.lineJoin = 'round'
      if (activePts.length === 1) {
        ctx.beginPath()
        ctx.arc(activePts[0].x, activePts[0].y, strokeWidthRef.current / 2, 0, Math.PI * 2)
        ctx.fillStyle = strokeColorRef.current
        ctx.fill()
      } else {
        drawSpline(ctx, activePts)
      }
    }

    ctx.restore()
  }, [selectedPathId, drawSpline])

  useImperativeHandle(ref, () => ({
    notifyViewportChange(vp) {
      viewportRef.current = vp
      redraw()
    },
  }), [redraw])

  useEffect(() => {
    redraw()
  }, [paths, isVisible, redraw])

  useEffect(() => {
    const canvas = canvasRef.current
    const parent = canvas?.parentElement
    if (!canvas || !parent) return

    const ro = new ResizeObserver(() => {
      const rect = parent.getBoundingClientRect()
      canvas.width = Math.round(rect.width)
      canvas.height = Math.round(rect.height)
      redraw()
    })
    ro.observe(parent)

    const rect = parent.getBoundingClientRect()
    canvas.width = Math.round(rect.width)
    canvas.height = Math.round(rect.height)

    return () => ro.disconnect()
  }, [redraw])

  const screenToFlow = useCallback((sx: number, sy: number): DrawingPoint => {
    const { x, y, zoom } = viewportRef.current
    return { x: (sx - x) / zoom, y: (sy - y) / zoom }
  }, [])

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
    canvas.setPointerCapture(e.pointerId)

    const rect = canvas.getBoundingClientRect()
    const flowPt = screenToFlow(e.clientX - rect.left, e.clientY - rect.top)

    if (modeRef.current === 'eraser') {
      isPointerDownRef.current = true
      const hit = findPathAt(flowPt)
      if (hit && onPathDelete) onPathDelete(hit.id)
      return
    }

    if (modeRef.current === 'select') {
      const hit = findPathAt(flowPt)
      if (hit) {
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

    isPointerDownRef.current = true
    currentPathRef.current = [flowPt]
  }, [isDrawing, screenToFlow, findPathAt, onPathDelete, onTextPositionSelected])

  const onPointerMove = useCallback((e: React.PointerEvent<HTMLCanvasElement>) => {
    if (!isDrawing || !isPointerDownRef.current) return
    e.preventDefault()

    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

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
        points: pathCloneRef.current.points.map(p => ({ x: p.x + dx, y: p.y + dy }))
      }
      
      // Local update for smoothness
      pathsRef.current = pathsRef.current.map(p => p.id === selectedPathId ? updatedPath : p)
      redraw()
      return
    }

    if (modeRef.current === 'pencil') {
      currentPathRef.current.push(flowPt)
      redraw()
    }
  }, [isDrawing, screenToFlow, redraw, selectedPathId, findPathAt, onPathDelete])

  const onPointerUp = useCallback(() => {
    if (!isPointerDownRef.current) return
    isPointerDownRef.current = false

    if (modeRef.current === 'select' && selectedPathId) {
      const finalPath = pathsRef.current.find(p => p.id === selectedPathId)
      if (finalPath && onPathUpdate) onPathUpdate(finalPath)
      dragStartRef.current = null
      pathCloneRef.current = null
      return
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
  }, [onPathComplete, onPathUpdate, selectedPathId])

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
      }}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
    />
  )
})

export default DrawingCanvas
