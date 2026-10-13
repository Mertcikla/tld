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
