import type { HoveredItem, ZUIViewState } from './types'
import { worldToScreenX, worldToScreenY } from './layoutEngine'

export function getHoverScreenRect(item: HoveredItem | null, view: ZUIViewState) {
  if (!item) return null
  if (item.type === 'edge') {
    return { sx: worldToScreenX(item.absX, view), sy: worldToScreenY(item.absY, view), sw: 1, sh: 1 }
  }
  if (item.type === 'node') {
    return {
      sx: worldToScreenX(item.absX, view), sy: worldToScreenY(item.absY, view),
      sw: item.absW * view.zoom, sh: item.absH * view.zoom,
    }
  }
  const group = item.data
  return {
    sx: worldToScreenX(group.worldX + group.diagramX + group.diagramW / 2, view) - 100,
    sy: worldToScreenY(group.worldY + group.diagramY, view) - 50,
    sw: 200, sh: 50,
  }
}

/** Keep a connector popover anchored to the first point hovered on its route. */
export function preserveHoverAnchor(previous: HoveredItem | null, next: HoveredItem | null): HoveredItem | null {
  if (!previous || !next || previous.type !== next.type) return next
  if (previous.type !== 'edge' || next.type !== 'edge') return next
  const a = previous.data
  const b = next.data
  if (a.isProxy || b.isProxy) {
    return a.isProxy && b.isProxy && a.details?.key && a.details.key === b.details?.key ? previous : next
  }
  if (a.diagramId !== b.diagramId || a.isPortalConn !== b.isPortalConn) return next
  if (a.id != null || b.id != null) return a.id === b.id ? previous : next
  return a.sourceId === b.sourceId && a.targetId === b.targetId && a.targetDiagId === b.targetDiagId ? previous : next
}

type Point = { x: number; y: number }
type HoverBounds = { left: number; top: number; right: number; bottom: number }

function cross(a: Point, b: Point, c: Point): number {
  return (b.x - a.x) * (c.y - a.y) - (b.y - a.y) * (c.x - a.x)
}

function insideTriangle(point: Point, a: Point, b: Point, c: Point): boolean {
  if (Math.abs(cross(a, b, c)) < 0.000001) return false
  const sides = [cross(a, b, point), cross(b, c, point), cross(c, a, point)]
  return sides.every((side) => side >= 0) || sides.every((side) => side <= 0)
}

/** The corridor follows the actual popover bounds, including flipped placements. */
export function isWithinHoverSafeArea(point: Point, anchor: Point, bounds: HoverBounds, padding = 8): boolean {
  if (bounds.right <= bounds.left || bounds.bottom <= bounds.top) return false
  const left = bounds.left - padding
  const right = bounds.right + padding
  const top = bounds.top - padding
  const bottom = bounds.bottom + padding
  if (point.x >= left && point.x <= right && point.y >= top && point.y <= bottom) return true
  if (Math.hypot(point.x - anchor.x, point.y - anchor.y) <= padding) return true
  const corners = [{ x: left, y: top }, { x: right, y: top }, { x: right, y: bottom }, { x: left, y: bottom }]
  return corners.some((corner, index) => insideTriangle(point, anchor, corner, corners[(index + 1) % corners.length]))
}
