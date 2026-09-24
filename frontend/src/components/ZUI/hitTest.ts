import type { DiagramGroupLayout, LayoutNode, ZUIViewState } from './types'
import { transitionT } from './layoutEngine'
import { isElementGroupTag } from '../../utils/elementGroups'

export interface ZUIHitTestNodeResult {
  node: LayoutNode
  absX: number
  absY: number
  absW: number
  absH: number
  cumulativeScale: number
}

interface NodeSpatialIndex {
  cellSize: number
  cells: Map<string, LayoutNode[]>
}

const NODE_INDEX_CELL_SIZE = 320
const SELF_HOVER_ALPHA_MIN = 0.08
const CHILD_VISIBILITY_MIN = 0.05
const nodeSpatialIndexCache = new WeakMap<LayoutNode[], NodeSpatialIndex>()

function cellKey(cx: number, cy: number): string {
  return `${cx},${cy}`
}

function isHiddenByTags(node: LayoutNode, hiddenTags: ReadonlySet<string>): boolean {
  return hiddenTags.size > 0 && node.tags.some((tag) => !isElementGroupTag(tag) && hiddenTags.has(tag))
}

function getNodeSpatialIndex(nodes: LayoutNode[]): NodeSpatialIndex {
  const cached = nodeSpatialIndexCache.get(nodes)
  if (cached) return cached

  const index: NodeSpatialIndex = { cellSize: NODE_INDEX_CELL_SIZE, cells: new Map() }
  for (const node of nodes) {
    const startX = Math.floor(node.worldX / index.cellSize)
    const endX = Math.floor((node.worldX + node.worldW) / index.cellSize)
    const startY = Math.floor(node.worldY / index.cellSize)
    const endY = Math.floor((node.worldY + node.worldH) / index.cellSize)

    for (let cx = startX; cx <= endX; cx += 1) {
      for (let cy = startY; cy <= endY; cy += 1) {
        const key = cellKey(cx, cy)
        let bucket = index.cells.get(key)
        if (!bucket) {
          bucket = []
          index.cells.set(key, bucket)
        }
        bucket.push(node)
      }
    }
  }

  nodeSpatialIndexCache.set(nodes, index)
  return index
}

function getNodesAtPoint(nodes: LayoutNode[], worldX: number, worldY: number): Set<LayoutNode> {
  const index = getNodeSpatialIndex(nodes)
  return new Set(index.cells.get(cellKey(Math.floor(worldX / index.cellSize), Math.floor(worldY / index.cellSize))) ?? [])
}

export function warmZUIHitTestIndexes(nodes: LayoutNode[]): void {
  getNodeSpatialIndex(nodes)
  for (const node of nodes) {
    if (node.children.length > 0) warmZUIHitTestIndexes(node.children)
  }
}

export function hitTestZUIRenderedNode(
  worldX: number,
  worldY: number,
  groups: DiagramGroupLayout[],
  view: ZUIViewState,
  thresholds: { start: number; end: number },
  hiddenTags: ReadonlySet<string> = new Set(),
): ZUIHitTestNodeResult | null {
  for (let groupIndex = groups.length - 1; groupIndex >= 0; groupIndex -= 1) {
    const group = groups[groupIndex]
    if (
      worldX >= group.worldX &&
      worldX <= group.worldX + group.worldW &&
      worldY >= group.worldY &&
      worldY <= group.worldY + group.worldH
    ) {
      const found = hitTestNodes(
        worldX,
        worldY,
        group.nodes,
        0,
        0,
        1,
        0,
        0,
        1,
        view,
        thresholds,
        hiddenTags,
      )
      if (found) return found
    }
  }

  return null
}

function hitTestNodes(
  worldX: number,
  worldY: number,
  nodes: LayoutNode[],
  parentAbsX: number,
  parentAbsY: number,
  parentAbsScale: number,
  parentChildOffsetX: number,
  parentChildOffsetY: number,
  inheritedAlpha: number,
  view: ZUIViewState,
  thresholds: { start: number; end: number },
  hiddenTags: ReadonlySet<string>,
): ZUIHitTestNodeResult | null {
  const candidates = getNodesAtPoint(nodes, worldX, worldY)

  for (let nodeIndex = nodes.length - 1; nodeIndex >= 0; nodeIndex -= 1) {
    const node = nodes[nodeIndex]
    if (!candidates.has(node)) continue
    if (isHiddenByTags(node, hiddenTags)) continue
    if (
      worldX < node.worldX ||
      worldX > node.worldX + node.worldW ||
      worldY < node.worldY ||
      worldY > node.worldY + node.worldH
    ) {
      continue
    }

    const absX = parentAbsX + (node.worldX - parentChildOffsetX) * parentAbsScale
    const absY = parentAbsY + (node.worldY - parentChildOffsetY) * parentAbsScale
    const absW = node.worldW * parentAbsScale
    const absH = node.worldH * parentAbsScale
    const screenW = absW * view.zoom
    if (screenW < 2) continue

    const hasChildren = node.children.length > 0
    const t = hasChildren ? transitionT(screenW, thresholds.start, thresholds.end) : 0
    const selfAlpha = inheritedAlpha * (1 - t)

    if (hasChildren && t > CHILD_VISIBILITY_MIN && node.childScale > 0) {
      const childX = (worldX - node.worldX) / node.childScale + node.childOffsetX
      const childY = (worldY - node.worldY) / node.childScale + node.childOffsetY
      const childHit = hitTestNodes(
        childX,
        childY,
        node.children,
        absX,
        absY,
        parentAbsScale * node.childScale,
        node.childOffsetX,
        node.childOffsetY,
        inheritedAlpha * t,
        view,
        thresholds,
        hiddenTags,
      )
      if (childHit) return childHit
    }

    if (selfAlpha < SELF_HOVER_ALPHA_MIN) continue

    return { node, absX, absY, absW, absH, cumulativeScale: parentAbsScale }
  }

  return null
}
