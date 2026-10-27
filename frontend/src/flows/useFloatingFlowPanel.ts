import { useLayoutEffect, useRef, useState, type CSSProperties, type KeyboardEvent, type PointerEvent } from 'react'

interface Point { x: number; y: number }
interface Bounds { left: number; top: number; right: number; bottom: number }
interface Placement extends Point { maxHeight: number }
const GAP = 8
const clamp = (value: number, min: number, max: number) => Math.max(min, Math.min(max, value))

/** Find the closest free position, preferring full panel height over a shorter fit. */
export function placeFloatingFlowPanel(preferred: Point, size: { width: number; height: number }, bounds: Bounds, obstacles: Bounds[]): Placement {
  const width = Math.min(size.width, bounds.right - bounds.left)
  const xs = [preferred.x, bounds.left, bounds.right - width, ...obstacles.flatMap(rect => [rect.left - width - GAP, rect.right + GAP])]
  let best: Placement | undefined
  let bestScore = Infinity
  for (const x of new Set(xs.map(x => clamp(x, bounds.left, bounds.right - width)))) {
    let intervals = [[bounds.top, bounds.bottom]]
    for (const rect of obstacles) {
      if (x >= rect.right + GAP || x + width <= rect.left - GAP) continue
      intervals = intervals.flatMap(([top, bottom]) => {
        if (rect.bottom + GAP <= top || rect.top - GAP >= bottom) return [[top, bottom]]
        return [[top, Math.min(bottom, rect.top - GAP)], [Math.max(top, rect.bottom + GAP), bottom]].filter(([a, b]) => b > a)
      })
    }
    for (const [top, bottom] of intervals) {
      const height = Math.min(size.height, bottom - top)
      const y = clamp(preferred.y, top, bottom - height)
      const score = (x - preferred.x) ** 2 + (y - preferred.y) ** 2 + (size.height - height) ** 2 * 10000
      if (score < bestScore) { bestScore = score; best = { x, y, maxHeight: bottom - top } }
    }
  }
  return best ?? { x: bounds.left, y: bounds.top, maxHeight: Math.max(0, bounds.bottom - bounds.top) }
}

export function useFloatingFlowPanel(enabled: boolean) {
  const ref = useRef<HTMLDivElement>(null)
  const preferred = useRef<Point | null>(null)
  const measureRef = useRef<(() => void) | null>(null)
  const drag = useRef<{ pointerId: number; start: Point; origin: Point } | null>(null)
  const [placement, setPlacement] = useState<(Placement & { maxWidth: number }) | null>(null)
  const [dragging, setDragging] = useState(false)

  useLayoutEffect(() => {
    const panel = ref.current
    const canvas = panel?.parentElement
    if (!enabled || !panel || !canvas) return
    let frame = 0
    const observed = new Set<Element>()
    const resize = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(schedule)
    function schedule() {
      if (!frame) frame = requestAnimationFrame(() => { frame = 0; measure() })
    }
    function measure() {
      const area = canvas!.getBoundingClientRect()
      const panelRect = panel!.getBoundingClientRect()
      if (!area.width || !area.height || !panelRect.width) return
      const bounds = { left: GAP, top: GAP, right: area.width - GAP, bottom: area.height - GAP }
      const obstacles: Bounds[] = []
      const overlays = Array.from(document.querySelectorAll<HTMLElement>('[data-canvas-overlay]'))
      const targets = new Set<Element>([canvas!, panel!, ...overlays])
      for (const target of observed) if (!targets.has(target)) { resize?.unobserve(target); observed.delete(target) }
      for (const target of targets) if (!observed.has(target)) { resize?.observe(target); observed.add(target) }
      for (const overlay of overlays) {
        if (panel!.contains(overlay)) continue
        const style = getComputedStyle(overlay)
        if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') continue
        const rect = overlay.getBoundingClientRect()
        if (!rect.width || !rect.height || rect.right <= area.left || rect.left >= area.right || rect.bottom <= area.top || rect.top >= area.bottom) continue
        const local = { left: rect.left - area.left, top: rect.top - area.top, right: rect.right - area.left, bottom: rect.bottom - area.top }
        if (overlay.dataset.canvasOverlay === 'top') bounds.top = Math.max(bounds.top, local.bottom + GAP)
        else if (overlay.dataset.canvasOverlay === 'bottom') bounds.bottom = Math.min(bounds.bottom, local.top - GAP)
        else obstacles.push(local)
      }
      bounds.right = Math.max(bounds.left, bounds.right)
      bounds.bottom = Math.max(bounds.top, bounds.bottom)
      const height = Math.min(760, Math.max(panelRect.height, (panel!.scrollHeight || 0) + (panel!.offsetHeight || 0) - (panel!.clientHeight || 0)))
      const next = placeFloatingFlowPanel(preferred.current ?? { x: panelRect.left - area.left, y: panelRect.top - area.top }, { width: panelRect.width, height }, bounds, obstacles)
      setPlacement(current => current?.x === next.x && current.y === next.y && current.maxHeight === next.maxHeight && current.maxWidth === bounds.right - bounds.left ? current : { ...next, maxWidth: bounds.right - bounds.left })
    }
    measureRef.current = measure
    measure()
    const mutations = typeof MutationObserver === 'undefined' ? null : new MutationObserver(records => {
      if (records.some(record => record.type === 'childList' || (record.target instanceof Element && !panel.contains(record.target) && record.target.closest('[data-canvas-overlay]')))) schedule()
    })
    mutations?.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['style', 'class', 'hidden'] })
    window.addEventListener('resize', schedule)
    window.addEventListener('scroll', schedule, true)
    return () => {
      measureRef.current = null
      drag.current = null
      if (frame) cancelAnimationFrame(frame)
      resize?.disconnect()
      mutations?.disconnect()
      window.removeEventListener('resize', schedule)
      window.removeEventListener('scroll', schedule, true)
    }
  }, [enabled])

  function move(point: Point) { preferred.current = point; measureRef.current?.() }
  function stop(event: PointerEvent<HTMLButtonElement>) {
    if (drag.current?.pointerId !== event.pointerId) return
    drag.current = null
    setDragging(false)
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
  }
  const handleProps = {
    onPointerDown(event: PointerEvent<HTMLButtonElement>) {
      if (!enabled || event.button !== 0 || !placement) return
      event.preventDefault(); event.stopPropagation()
      event.currentTarget.focus({ preventScroll: true })
      event.currentTarget.setPointerCapture(event.pointerId)
      drag.current = { pointerId: event.pointerId, start: { x: event.clientX, y: event.clientY }, origin: placement }
      setDragging(true)
    },
    onPointerMove(event: PointerEvent<HTMLButtonElement>) {
      const current = drag.current
      if (!current || current.pointerId !== event.pointerId) return
      event.preventDefault(); event.stopPropagation()
      move({ x: current.origin.x + event.clientX - current.start.x, y: current.origin.y + event.clientY - current.start.y })
    },
    onPointerUp: stop,
    onPointerCancel: stop,
    onLostPointerCapture: stop,
    onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
      const direction = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }[event.key]
      if (!enabled || !placement || !direction) return
      event.preventDefault(); event.stopPropagation()
      const distance = event.shiftKey ? 48 : 16
      move({ x: placement.x + direction[0] * distance, y: placement.y + direction[1] * distance })
    },
  }
  const style: CSSProperties | undefined = enabled && placement ? { left: placement.x, top: placement.y, bottom: 'auto', maxHeight: Math.min(760, placement.maxHeight), maxWidth: placement.maxWidth } : undefined
  return { ref, style, dragging, handleProps }
}
