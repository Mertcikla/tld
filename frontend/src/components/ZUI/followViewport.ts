import { fitWorldRect } from './camera'
import { findElementFocusTarget } from './focus'
import { rawCameraView } from './layoutEngine'
import type { DiagramGroupLayout, ZUIViewState } from './types'

/** Keep visible changes steady. For widely separated changes, follow the
 * nearest cluster instead of zooming out to the entire repository. */
export function followViewport(groups: DiagramGroupLayout[], refs: Array<{ viewId: number; elementId: number }>, view: ZUIViewState, width: number, height: number, maxZoom: number, fallbackGroups: DiagramGroupLayout[] = []): ZUIViewState | null {
  if (width <= 0 || height <= 0 || view.zoom <= 0) return null
  const targets = refs.map((ref) => findElementFocusTarget(groups, ref.viewId, ref.elementId) ?? findElementFocusTarget(fallbackGroups, ref.viewId, ref.elementId)).filter((target) => target !== null)
  if (!targets.length) return null
  const camera = rawCameraView(view)
  const bounds = (items: typeof targets) => {
    const x = Math.min(...items.map((t) => t.absX)), y = Math.min(...items.map((t) => t.absY))
    return { x, y, w: Math.max(...items.map((t) => t.absX + t.absW)) - x, h: Math.max(...items.map((t) => t.absY + t.absH)) - y }
  }
  let rect = bounds(targets)
  const margin = 0.12
  const fits = rect.x * camera.zoom + camera.x >= width * margin && rect.y * camera.zoom + camera.y >= height * margin &&
    (rect.x + rect.w) * camera.zoom + camera.x <= width * (1 - margin) && (rect.y + rect.h) * camera.zoom + camera.y <= height * (1 - margin)
  if (fits) return null
  if (rect.w * camera.zoom > width * 2 || rect.h * camera.zoom > height * 2) {
    const centerX = (width / 2 - camera.x) / camera.zoom, centerY = (height / 2 - camera.y) / camera.zoom
    targets.sort((a, b) => Math.hypot(a.absX + a.absW / 2 - centerX, a.absY + a.absH / 2 - centerY) - Math.hypot(b.absX + b.absW / 2 - centerX, b.absY + b.absH / 2 - centerY))
    const nearest = targets[0]
    rect = bounds(targets.filter((t) => Math.abs(t.absX - nearest.absX) * camera.zoom < width / 2 && Math.abs(t.absY - nearest.absY) * camera.zoom < height / 2))
  }
  // Preserve zoom when a pan is sufficient; zoom out only to fit context.
  return fitWorldRect(rect, width, height, Math.min(maxZoom, camera.zoom), margin)
}
