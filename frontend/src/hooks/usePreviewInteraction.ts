import { useCallback, useEffect, useRef, useState, type RefObject } from 'react'

export const PREVIEW_HOVER_DELAY = 1000

export function usePreviewInteraction(enabled: boolean, moving: boolean | undefined, scroller: RefObject<HTMLDivElement | null>) {
  const [phase, setPhase] = useState<'idle' | 'waiting' | 'ready'>('idle')
  const hint = useRef<HTMLDivElement | null>(null)
  const cursor = useRef({ clientX: 0, clientY: 0 })
  const positionHint = useCallback((point: { clientX: number; clientY: number }) => {
    cursor.current = { clientX: point.clientX, clientY: point.clientY }
    if (hint.current) { hint.current.style.left = `${point.clientX}px`; hint.current.style.top = `${point.clientY}px` }
  }, [])
  const hintRef = useCallback((element: HTMLDivElement | null) => { hint.current = element; positionHint(cursor.current) }, [positionHint])
  const hovered = useRef(false)
  const ready = useRef(false)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const reset = useCallback(() => {
    clearTimeout(timer.current)
    ready.current = false
    setPhase('idle')
  }, [])
  const start = useCallback(() => {
    reset()
    if (!enabled || moving || !hovered.current) return
    setPhase('waiting')
    timer.current = setTimeout(() => { ready.current = true; setPhase('ready') }, PREVIEW_HOVER_DELAY)
  }, [enabled, moving, reset])
  useEffect(() => { start(); return () => clearTimeout(timer.current) }, [start])
  useEffect(() => {
    const element = scroller.current
    if (!enabled || !element) return
    const surface = element
    let drag: { id: number; x: number; y: number; left: number; top: number } | null = null
    const captureWheel = (event: WheelEvent) => {
      if (!ready.current && event.target instanceof Node && surface.contains(event.target)) event.preventDefault()
    }
    const wheel = (event: WheelEvent) => {
      // Suppress native overflow scrolling while leaving the gesture available to
      // the canvas. Once ready, consume it here before React Flow sees it.
      event.preventDefault()
      if (!ready.current) return
      event.stopPropagation()
      if (event.ctrlKey || event.metaKey) return
      const scale = event.deltaMode === 1 ? 24 : event.deltaMode === 2 ? element.clientHeight : 1
      element.scrollBy(event.deltaX * scale, event.deltaY * scale)
    }
    const down = (event: PointerEvent) => {
      if (!ready.current || (event.button !== 1 && event.button !== 2)) return
      event.preventDefault(); event.stopPropagation()
      element.setPointerCapture(event.pointerId)
      drag = { id: event.pointerId, x: event.clientX, y: event.clientY, left: element.scrollLeft, top: element.scrollTop }
    }
    const move = (event: PointerEvent) => {
      if (!drag || drag.id !== event.pointerId) return
      const bounds = element.getBoundingClientRect()
      const zoom = bounds.width / element.offsetWidth || 1
      element.scrollLeft = drag.left - (event.clientX - drag.x) / zoom
      element.scrollTop = drag.top - (event.clientY - drag.y) / zoom
      event.preventDefault(); event.stopPropagation()
    }
    const up = () => { drag = null }
    const context = (event: Event) => { if (ready.current) { event.preventDefault(); event.stopPropagation() } }
    document.addEventListener('wheel', captureWheel, { capture: true, passive: false })
    surface.addEventListener('wheel', wheel, { passive: false })
    surface.addEventListener('pointerdown', down)
    surface.addEventListener('pointermove', move)
    surface.addEventListener('pointerup', up)
    surface.addEventListener('pointercancel', up)
    surface.addEventListener('contextmenu', context)
    return () => {
      document.removeEventListener('wheel', captureWheel, true)
      surface.removeEventListener('wheel', wheel)
      surface.removeEventListener('pointerdown', down)
      surface.removeEventListener('pointermove', move)
      surface.removeEventListener('pointerup', up)
      surface.removeEventListener('pointercancel', up)
      surface.removeEventListener('contextmenu', context)
    }
  }, [enabled, scroller])
  return {
    phase, hintRef, onPointerMove: positionHint,
    onPointerEnter: (event: { clientX: number; clientY: number }) => { positionHint(event); if (hovered.current) return; hovered.current = true; start() },
    onPointerLeave: (event?: { currentTarget: HTMLElement; relatedTarget: EventTarget | null }) => {
      if (event && event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget)) return
      hovered.current = false; reset()
    },
  }
}
