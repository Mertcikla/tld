import { useRef, useState, type PointerEvent } from 'react'
import { useStoreApi } from 'reactflow'

type Size = { width: number; height: number }
const boundedSize = (width: number, height: number): Size => ({
  width: Math.max(280, Math.min(2400, Math.round(width))),
  height: Math.max(160, Math.min(2400, Math.round(height))),
})

export default function SubElementResizeHandle({ name, width, height, onPreview, onCommit, keyboardStep = 10 }: Size & {
  name: string
  onPreview: (size: Size | null) => void
  onCommit: (size: Size) => Promise<void>
  keyboardStep?: number
}) {
  const flow = useStoreApi()
  const [saving, setSaving] = useState(false)
  const gesture = useRef<{ pointerId: number; x: number; y: number; zoom: number; width: number; height: number } | null>(null)
  const preview = (event: PointerEvent<HTMLButtonElement>) => {
    const start = gesture.current
    if (!start || event.pointerId !== start.pointerId) return null
    const size = boundedSize(start.width + (event.clientX - start.x) / start.zoom, start.height + (event.clientY - start.y) / start.zoom)
    onPreview(size)
    return size
  }
  const commit = async (size: Size) => {
    setSaving(true)
    try { await onCommit(size) } catch { /* The editor reports save failures. */ }
    finally { onPreview(null); setSaving(false) }
  }
  return <button
    type="button"
    className="sub-element-resize nodrag nopan"
    data-testid="sub-element-resize"
    aria-label={`Resize ${name}`}
    title="Drag to resize · arrow keys adjust size"
    aria-busy={saving}
    onClick={event => event.stopPropagation()}
    onPointerDown={event => {
      if (event.button !== 0 || saving) return
      event.preventDefault(); event.stopPropagation()
      event.currentTarget.setPointerCapture(event.pointerId)
      gesture.current = { pointerId: event.pointerId, x: event.clientX, y: event.clientY, zoom: flow.getState().transform[2], width, height }
    }}
    onPointerMove={event => {
      if (!gesture.current) return
      event.preventDefault(); event.stopPropagation()
      preview(event)
    }}
    onPointerUp={event => {
      const start = gesture.current
      const size = preview(event)
      if (!size || !start) return
      event.preventDefault(); event.stopPropagation()
      gesture.current = null
      event.currentTarget.releasePointerCapture(event.pointerId)
      if (size.width === start.width && size.height === start.height) { onPreview(null); return }
      void commit(size)
    }}
    onLostPointerCapture={() => { if (gesture.current) { gesture.current = null; onPreview(null) } }}
    onPointerCancel={() => { gesture.current = null; onPreview(null) }}
    onKeyDown={event => {
      const delta = event.shiftKey ? 50 : keyboardStep
      const changes: Record<string, Size> = { ArrowRight: { width: delta, height: 0 }, ArrowLeft: { width: -delta, height: 0 }, ArrowDown: { width: 0, height: delta }, ArrowUp: { width: 0, height: -delta } }
      const change = changes[event.key]
      if (!change || saving) return
      event.preventDefault(); event.stopPropagation()
      const size = boundedSize(width + change.width, height + change.height)
      onPreview(size)
      void commit(size)
    }}
  >
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M4 13L13 4M9 13L13 9" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  </button>
}
