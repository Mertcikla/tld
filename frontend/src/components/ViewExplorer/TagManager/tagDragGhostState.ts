import { useSyncExternalStore } from 'react'
import type { DragEvent as ReactDragEvent } from 'react'
import type { ViewLayer } from '../../../types'
import { isElementGroupTag } from '../../../utils/elementGroups'

export type TagDragGhostKind = 'tag' | 'group' | 'layer'

export interface TagDragGhostMeta {
  kind: TagDragGhostKind
  name: string
  color: string
  detail?: string
}

export interface TagDragHoverTarget {
  name: string
  color: string
}

export interface TagDragSource {
  tag?: string
  layerId?: number
}

export const DEFAULT_TAG_COLOR = '#A0AEC0'

const TRANSPARENT_PIXEL =
  'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7'

let meta: TagDragGhostMeta | null = null
let hoverTarget: TagDragHoverTarget | null = null
let source: TagDragSource = {}
let position = { x: 0, y: 0 }
const listeners = new Set<() => void>()

function emit() {
  listeners.forEach((listener) => listener())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function tagDragMeta(tag: string, color: string, displayLabel?: string): TagDragGhostMeta {
  const label = displayLabel ?? tag
  if (isElementGroupTag(tag)) {
    return { kind: 'group', name: label.replace(/^group:/, ''), color }
  }
  return { kind: 'tag', name: label, color }
}

export function layerDragMeta(layer: Pick<ViewLayer, 'name' | 'color' | 'tags'>): TagDragGhostMeta {
  const count = layer.tags.length
  return {
    kind: 'layer',
    name: layer.name,
    color: layer.color || DEFAULT_TAG_COLOR,
    detail: `${count} tag${count === 1 ? '' : 's'}`,
  }
}

export function beginTagDrag(next: TagDragGhostMeta, at?: { x: number; y: number }, dragSource?: TagDragSource) {
  meta = next
  hoverTarget = null
  source = dragSource ?? {}
  if (at) position = { x: at.x, y: at.y }
  emit()
}

export function endTagDrag() {
  if (!meta && !hoverTarget) return
  meta = null
  hoverTarget = null
  source = {}
  emit()
}

export function getTagDragSource() {
  return source
}

export function setTagDragHoverTarget(target: TagDragHoverTarget | null) {
  if (hoverTarget?.name === target?.name && hoverTarget?.color === target?.color) return
  hoverTarget = target
  emit()
}

export function getTagDragPosition() {
  return position
}

export function useTagDragGhost(): TagDragGhostMeta | null {
  return useSyncExternalStore(
    subscribe,
    () => meta,
    () => null,
  )
}

export function useTagDragHoverTarget(): TagDragHoverTarget | null {
  return useSyncExternalStore(
    subscribe,
    () => hoverTarget,
    () => null,
  )
}

export function suppressNativeDragImage(e: ReactDragEvent) {
  if (typeof document === 'undefined' || !e.dataTransfer?.setDragImage) return
  const ghost = document.createElement('img')
  ghost.src = TRANSPARENT_PIXEL
  ghost.style.position = 'fixed'
  ghost.style.top = '-1000px'
  ghost.style.left = '-1000px'
  ghost.style.width = '1px'
  ghost.style.height = '1px'
  document.body.appendChild(ghost)
  e.dataTransfer.setDragImage(ghost, 0, 0)
  window.setTimeout(() => ghost.remove(), 0)
}
