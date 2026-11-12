import type { DiagramGroupLayout, LayoutNode, ZUIViewState } from './types'
import { getExpandThresholds, screenToWorldX, screenToWorldY } from './layoutEngine'

export const MIN_ZUI_ZOOM = 0.1

export interface PathItem {
  id: string
  label: string
  type: 'group' | 'node'
  isCircular?: boolean
  absX: number
  absY: number
  absW: number
  absH: number
}

export function getPathAt(
  view: ZUIViewState,
  groups: DiagramGroupLayout[],
  canvasW: number,
  canvasH: number,
): PathItem[] {
  if (canvasW === 0 || canvasH === 0) return []

  const worldCenterX = screenToWorldX(canvasW / 2, view)
  const worldCenterY = screenToWorldY(canvasH / 2, view)
  const thresholds = getExpandThresholds(canvasW)

  for (const group of groups) {
    if (
      worldCenterX >= group.worldX &&
      worldCenterX <= group.worldX + group.worldW &&
      worldCenterY >= group.worldY &&
      worldCenterY <= group.worldY + group.worldH
    ) {
      const path: PathItem[] = [
        {
          id: `g-${group.diagramId}`,
          label: group.label,
          type: 'group',
          absX: group.worldX,
          absY: group.worldY,
          absW: group.worldW,
          absH: group.worldH,
        },
      ]

      let currentNodes = group.nodes
      let currentX = worldCenterX
      let currentY = worldCenterY
      let parentAbsX = 0
      let parentAbsY = 0
      let parentAbsScale = 1
      let parentChildOffsetX = 0
      let parentChildOffsetY = 0

      while (true) {
        let found = false
        for (const node of currentNodes) {
          if (
            currentX >= node.worldX &&
            currentX <= node.worldX + node.worldW &&
            currentY >= node.worldY &&
            currentY <= node.worldY + node.worldH
          ) {
            const absX = parentAbsX + (node.worldX - parentChildOffsetX) * parentAbsScale
            const absY = parentAbsY + (node.worldY - parentChildOffsetY) * parentAbsScale
            const absW = node.worldW * parentAbsScale
            const absH = node.worldH * parentAbsScale
            const screenW = absW * view.zoom

            path.push({
              id: node.id,
              label: node.label,
              type: 'node',
              isCircular: node.isCircular,
              absX,
              absY,
              absW,
              absH,
            })

            // Breadcrumb descent follows the same visual expansion threshold as rendering,
            // so the trail describes what the user is actually zoomed into.
            const isExpanded = screenW > thresholds.start * 1.1

            if (isExpanded && node.children && node.children.length > 0) {
              parentAbsX = absX
              parentAbsY = absY
              parentAbsScale = parentAbsScale * node.childScale
              parentChildOffsetX = node.childOffsetX
              parentChildOffsetY = node.childOffsetY

              currentX = (currentX - node.worldX) / node.childScale + node.childOffsetX
              currentY = (currentY - node.worldY) / node.childScale + node.childOffsetY
              currentNodes = node.children
              found = true
              break
            } else {
              found = false
              break
            }
          }
        }
        if (!found) break
      }
      return path
    }
  }
  return []
}

export function easeOutQuart(t: number): number {
  return 1 - Math.pow(1 - t, 4)
}

export function clamp01(value: number): number {
  return Math.max(0, Math.min(1, value))
}

export function fitWorldRect(
  rect: { x: number; y: number; w: number; h: number },
  canvasW: number,
  canvasH: number,
  maxZoom: number,
  padding: number,
): ZUIViewState | null {
  const bboxW = Math.max(1, rect.w)
  const bboxH = Math.max(1, rect.h)
  const fitZoom = Math.min(
    (canvasW * (1 - padding * 2)) / bboxW,
    (canvasH * (1 - padding * 2)) / bboxH,
    maxZoom,
  )
  if (!Number.isFinite(fitZoom) || fitZoom <= 0) return null
  const zoom = Math.max(MIN_ZUI_ZOOM, fitZoom)

  return {
    x: (canvasW - bboxW * zoom) / 2 - rect.x * zoom,
    y: (canvasH - bboxH * zoom) / 2 - rect.y * zoom,
    zoom,
  }
}

export function findFirstExpandableNode(groups: DiagramGroupLayout[]): PathItem | null {
  for (const group of groups) {
    const found = findFirstExpandableNodeInTree(group.nodes, 0, 0, 1, 0, 0)
    if (found) return found
  }
  return null
}

function findFirstExpandableNodeInTree(
  nodes: LayoutNode[],
  parentAbsX: number,
  parentAbsY: number,
  parentAbsScale: number,
  parentChildOffsetX: number,
  parentChildOffsetY: number,
): PathItem | null {
  for (const node of nodes) {
    const absX = parentAbsX + (node.worldX - parentChildOffsetX) * parentAbsScale
    const absY = parentAbsY + (node.worldY - parentChildOffsetY) * parentAbsScale
    const absW = node.worldW * parentAbsScale
    const absH = node.worldH * parentAbsScale

    if (node.children.length > 0) {
      return {
        id: node.id,
        label: node.linkedDiagramLabel || node.label,
        type: 'node',
        isCircular: node.isCircular,
        absX,
        absY,
        absW,
        absH,
      }
    }

    const found = findFirstExpandableNodeInTree(
      node.children,
      absX,
      absY,
      parentAbsScale * node.childScale,
      node.childOffsetX,
      node.childOffsetY,
    )
    if (found) return found
  }
  return null
}
