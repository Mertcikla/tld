import { resolveZUIProxyConnectors, type ZUIResolvedConnector } from '../../crossBranch/resolve'
import type { WorkspaceGraphSnapshot } from '../../crossBranch/types'
import type { LayoutNode, ZUIViewState, HoveredItem } from './types'
import { getExpandThresholds, pickEdgeLabelPosition, type ScreenRect } from './renderer'
import type { CrossBranchContextSettings } from '../../crossBranch/types'

export interface VisibleNodeAnchor {
  nodeId: string
  elementId: number
  label: string
  worldX: number
  worldY: number
  worldW: number
  worldH: number
  pathDepth: number
  renderAlpha: number
}

function clamp(value: number, min: number, max: number) {
  return value < min ? min : value > max ? max : value
}

function transitionT(screenW: number, start: number, end: number): number {
  return clamp((screenW - start) / (end - start), 0, 1)
}

function collectVisibleAnchorsInNodes(
  nodes: LayoutNode[],
  view: ZUIViewState,
  thresholds: { start: number; end: number },
  hiddenTags: Set<string>,
  visibleAnchors: Map<number, VisibleNodeAnchor>,
  byNodeId: Map<string, VisibleNodeAnchor>,
  inheritedAlpha: number,
  parentAbsX: number,
  parentAbsY: number,
  parentAbsScale: number,
  parentChildOffsetX: number,
  parentChildOffsetY: number,
) {
  for (const node of nodes) {
    if (hiddenTags.size > 0 && node.tags.some((tag) => hiddenTags.has(tag))) continue

    const absX = parentAbsX + (node.worldX - parentChildOffsetX) * parentAbsScale
    const absY = parentAbsY + (node.worldY - parentChildOffsetY) * parentAbsScale
    const absScale = parentAbsScale
    const absW = node.worldW * absScale
    const absH = node.worldH * absScale
    const screenW = absW * view.zoom
    if (screenW < 2) continue

    const hasChildren = node.children && node.children.length > 0
    const t = hasChildren ? transitionT(screenW, thresholds.start, thresholds.end) : 0
    const parentAlpha = inheritedAlpha * (1 - t)
    const childAlpha = inheritedAlpha * t

    if (!hasChildren || t <= 0.95) {
      const anchor: VisibleNodeAnchor = {
        nodeId: node.id,
        elementId: node.elementId,
        label: node.label,
        worldX: absX,
        worldY: absY,
        worldW: absW,
        worldH: absH,
        pathDepth: node.pathElementIds.length,
        renderAlpha: hasChildren ? parentAlpha : inheritedAlpha,
      }
      const existing = visibleAnchors.get(node.elementId)
      if (!existing || existing.pathDepth < anchor.pathDepth) visibleAnchors.set(node.elementId, anchor)
      byNodeId.set(node.id, anchor)
    }

    if (hasChildren && t > 0.05) {
      collectVisibleAnchorsInNodes(
        node.children,
        view,
        thresholds,
        hiddenTags,
        visibleAnchors,
        byNodeId,
        childAlpha,
        absX,
        absY,
        absScale * node.childScale,
        node.childOffsetX,
        node.childOffsetY,
      )
    }
  }
}

export function collectVisibleNodeAnchors(
  groups: Array<{ nodes: LayoutNode[] }>,
  view: ZUIViewState,
  canvasW: number,
  hiddenTags: string[] = [],
) {
  const thresholds = getExpandThresholds(canvasW)
  const visibleAnchors = new Map<number, VisibleNodeAnchor>()
  const byNodeId = new Map<string, VisibleNodeAnchor>()
  const hiddenTagSet = new Set(hiddenTags)

  for (const group of groups) {
    collectVisibleAnchorsInNodes(
      group.nodes,
      view,
      thresholds,
      hiddenTagSet,
      visibleAnchors,
      byNodeId,
      1,
      0,
      0,
      1,
      0,
      0,
    )
  }

  return { visibleAnchors, byNodeId }
}

function getDirectAnchorPoint(anchor: VisibleNodeAnchor, towards: VisibleNodeAnchor) {
  const cx = anchor.worldX + anchor.worldW / 2
  const cy = anchor.worldY + anchor.worldH / 2
  const tx = towards.worldX + towards.worldW / 2
  const ty = towards.worldY + towards.worldH / 2
  const dx = tx - cx
  const dy = ty - cy
  const hw = anchor.worldW / 2
  const hh = anchor.worldH / 2

  if (dx === 0 && dy === 0) return { x: cx, y: cy }

  const tanTheta = Math.abs(dy / dx)
  const boxRatio = hh / hw
  if (tanTheta < boxRatio) {
    return {
      x: cx + Math.sign(dx) * hw,
      y: cy + Math.sign(dx) * hw * (dy / dx),
    }
  }

  return {
    y: cy + Math.sign(dy) * hh,
    x: cx + Math.sign(dy) * hh * (dx / dy),
  }
}

export function buildVisibleProxyConnectors(
  snapshot: WorkspaceGraphSnapshot | null,
  visibleAnchors: Map<number, VisibleNodeAnchor>,
  settings: CrossBranchContextSettings,
): ZUIResolvedConnector[] {
  return resolveZUIProxyConnectors(
    snapshot,
    new Map(Array.from(visibleAnchors.entries()).map(([elementId, anchor]) => [elementId, anchor.nodeId])),
    settings,
  )
}

export function drawVisibleProxyConnectors(
  ctx: CanvasRenderingContext2D,
  connectors: ZUIResolvedConnector[],
  visibleAnchorsByNodeId: Map<string, VisibleNodeAnchor>,
  zoom: number,
  labelBg: string,
  occupiedLabelRects: ScreenRect[],
) {
  for (const connector of connectors) {
    const source = visibleAnchorsByNodeId.get(connector.sourceNodeId)
    const target = visibleAnchorsByNodeId.get(connector.targetNodeId)
    if (!source || !target) continue
    const alpha = Math.min(source.renderAlpha, target.renderAlpha)
    if (alpha < 0.01) continue

    const sourcePoint = getDirectAnchorPoint(source, target)
    const targetPoint = getDirectAnchorPoint(target, source)
    const midX = (sourcePoint.x + targetPoint.x) / 2
    const midY = (sourcePoint.y + targetPoint.y) / 2
    const label = String(connector.details.count)

    ctx.save()
    ctx.globalAlpha = alpha
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.2)'
    ctx.lineWidth = 2 / zoom
    ctx.beginPath()
    ctx.moveTo(sourcePoint.x, sourcePoint.y)
    ctx.lineTo(targetPoint.x, targetPoint.y)
    ctx.stroke()
    const fontSize = 11 / zoom
    ctx.font = `${fontSize}px Inter, system-ui, sans-serif`
    const textMetrics = ctx.measureText(label)
    const textW = textMetrics.width
    const textH = fontSize
    const labelPos = pickEdgeLabelPosition(
      ctx.getTransform(),
      midX,
      midY,
      textW,
      textH,
      targetPoint.x - sourcePoint.x,
      targetPoint.y - sourcePoint.y,
      occupiedLabelRects,
    )
    const px = 6 / zoom
    const py = 4 / zoom
    const badgeW = textW + px * 2
    const badgeH = textH + py * 2
    const badgeRadius = badgeH / 2
    ctx.fillStyle = labelBg
    ctx.beginPath()
    ctx.roundRect(
      labelPos.x - badgeW / 2,
      labelPos.y - badgeH / 2,
      badgeW,
      badgeH,
      badgeRadius,
    )
    ctx.fill()
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.2)'
    ctx.lineWidth = 1 / zoom
    ctx.stroke()
    ctx.fillStyle = 'white'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(label, labelPos.x, labelPos.y)

    ctx.restore()
  }
}

export function findHoveredProxyConnector(
  worldX: number,
  worldY: number,
  connectors: ZUIResolvedConnector[],
  visibleAnchorsByNodeId: Map<string, VisibleNodeAnchor>,
  view: ZUIViewState,
): HoveredItem | null {
  const threshold = 18 / view.zoom
  for (const connector of connectors) {
    const source = visibleAnchorsByNodeId.get(connector.sourceNodeId)
    const target = visibleAnchorsByNodeId.get(connector.targetNodeId)
    if (!source || !target) continue
    const x1 = source.worldX + source.worldW / 2
    const y1 = source.worldY + source.worldH / 2
    const x2 = target.worldX + target.worldW / 2
    const y2 = target.worldY + target.worldH / 2
    const dx = x2 - x1
    const dy = y2 - y1
    const l2 = dx * dx + dy * dy
    if (l2 === 0) continue
    let t = ((worldX - x1) * dx + (worldY - y1) * dy) / l2
    t = Math.max(0, Math.min(1, t))
    const nearestX = x1 + t * dx
    const nearestY = y1 + t * dy
    const dist = Math.sqrt((worldX - nearestX) ** 2 + (worldY - nearestY) ** 2)
    if (dist > threshold) continue

    return {
      type: 'edge',
      data: {
        sourceId: connector.details.sourceAnchorName,
        targetId: connector.details.targetAnchorName,
        label: connector.details.label || 'Cross-branch connector',
        diagramId: connector.details.ownerViewIds[0] ?? 0,
        sourceObjId: connector.sourceAnchorElementId,
        targetObjId: connector.targetAnchorElementId,
        isProxy: true,
        details: connector.details,
      },
      absX: (x1 + x2) / 2,
      absY: (y1 + y2) / 2,
    }
  }
  return null
}
