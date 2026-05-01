import {
  resolveZUIProxyConnectors,
  type ZUIHiddenProxyBadge,
  type ZUIProxyResolution,
  type ZUIResolvedConnector,
} from '../../crossBranch/resolve'
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

function connectorAlpha(alpha: number): number {
  return clamp(alpha * 1.1, 0.35, 0.95)
}

function transitionT(screenW: number, start: number, end: number): number {
  return clamp((screenW - start) / (end - start), 0, 1)
}

function visualRectForNode(
  absX: number,
  absY: number,
  absW: number,
  absH: number,
  hasChildren: boolean,
  screenW: number,
  thresholds: { start: number; end: number },
) {
  if (!hasChildren && screenW > thresholds.end) {
    const scale = thresholds.end / screenW
    const visualW = absW * scale
    const visualH = absH * scale
    return {
      worldX: absX + (absW - visualW) / 2,
      worldY: absY + (absH - visualH) / 2,
      worldW: visualW,
      worldH: visualH,
    }
  }

  return {
    worldX: absX,
    worldY: absY,
    worldW: absW,
    worldH: absH,
  }
}

function registerVisibleAnchor(
  node: LayoutNode,
  visibleAnchors: Map<number, VisibleNodeAnchor>,
  byNodeId: Map<string, VisibleNodeAnchor>,
  anchor: VisibleNodeAnchor,
) {
  const existing = visibleAnchors.get(node.elementId)
  if (!existing || existing.pathDepth < anchor.pathDepth || existing.renderAlpha < anchor.renderAlpha) {
    visibleAnchors.set(node.elementId, anchor)
  }
  byNodeId.set(node.id, anchor)
}

function collectVisibleAnchorForNode(
  node: LayoutNode,
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
  if (hiddenTags.size > 0 && node.tags.some((tag) => hiddenTags.has(tag))) return { selfDrawn: false }

  const absX = parentAbsX + (node.worldX - parentChildOffsetX) * parentAbsScale
  const absY = parentAbsY + (node.worldY - parentChildOffsetY) * parentAbsScale
  const absScale = parentAbsScale
  const absW = node.worldW * absScale
  const absH = node.worldH * absScale
  const screenW = absW * view.zoom
  if (screenW < 2) return { selfDrawn: false }

  const hasChildren = node.children && node.children.length > 0
  const t = hasChildren ? transitionT(screenW, thresholds.start, thresholds.end) : 0
  const parentAlpha = inheritedAlpha * (1 - t)
  const childAlpha = inheritedAlpha * t
  const selfDrawn = !hasChildren || t <= 0.95
  const visualRect = visualRectForNode(absX, absY, absW, absH, hasChildren, screenW, thresholds)

  if (selfDrawn) {
    registerVisibleAnchor(node, visibleAnchors, byNodeId, {
      nodeId: node.id,
      elementId: node.elementId,
      label: node.label,
      worldX: visualRect.worldX,
      worldY: visualRect.worldY,
      worldW: visualRect.worldW,
      worldH: visualRect.worldH,
      pathDepth: node.pathElementIds.length,
      renderAlpha: hasChildren ? parentAlpha : inheritedAlpha,
    })
  }

  let hasDirectChildDrawn = false
  if (hasChildren && t > 0.05) {
    for (const child of node.children) {
      const childResult = collectVisibleAnchorForNode(
        child,
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
      hasDirectChildDrawn = hasDirectChildDrawn || childResult.selfDrawn
    }
  }

  if (!selfDrawn && hasDirectChildDrawn) {
    registerVisibleAnchor(node, visibleAnchors, byNodeId, {
      nodeId: node.id,
      elementId: node.elementId,
      label: node.label,
      worldX: visualRect.worldX,
      worldY: visualRect.worldY,
      worldW: visualRect.worldW,
      worldH: visualRect.worldH,
      pathDepth: node.pathElementIds.length,
      renderAlpha: Math.max(0.12, inheritedAlpha * 0.28),
    })
  }

  return { selfDrawn }
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
    collectVisibleAnchorForNode(
      node,
      view,
      thresholds,
      hiddenTags,
      visibleAnchors,
      byNodeId,
      inheritedAlpha,
      parentAbsX,
      parentAbsY,
      parentAbsScale,
      parentChildOffsetX,
      parentChildOffsetY,
    )
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

function getAnchorCenter(anchor: VisibleNodeAnchor) {
  return {
    x: anchor.worldX + anchor.worldW / 2,
    y: anchor.worldY + anchor.worldH / 2,
  }
}

function containsPoint(anchor: VisibleNodeAnchor, x: number, y: number) {
  return x >= anchor.worldX &&
    x <= anchor.worldX + anchor.worldW &&
    y >= anchor.worldY &&
    y <= anchor.worldY + anchor.worldH
}

function getRectBoundaryPoint(anchor: VisibleNodeAnchor, dx: number, dy: number) {
  const cx = anchor.worldX + anchor.worldW / 2
  const cy = anchor.worldY + anchor.worldH / 2
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

function getDirectAnchorPoint(anchor: VisibleNodeAnchor, towards: VisibleNodeAnchor) {
  const anchorCenter = getAnchorCenter(anchor)
  const towardsCenter = getAnchorCenter(towards)

  // Nested anchors represent parent/child nodes. Aim the child endpoint away
  // from the parent center so proxy lines attach to the nearer child edge.
  if (containsPoint(towards, anchorCenter.x, anchorCenter.y)) {
    return getRectBoundaryPoint(
      anchor,
      anchorCenter.x - towardsCenter.x,
      anchorCenter.y - towardsCenter.y,
    )
  }

  return getRectBoundaryPoint(
    anchor,
    towardsCenter.x - anchorCenter.x,
    towardsCenter.y - anchorCenter.y,
  )
}

function getDirectAnchorPoints(source: VisibleNodeAnchor, target: VisibleNodeAnchor) {
  const sourcePoint = getDirectAnchorPoint(source, target)
  const targetPoint = getDirectAnchorPoint(target, source)
  return { sourcePoint, targetPoint }
}

export function buildVisibleProxyConnectors(
  snapshot: WorkspaceGraphSnapshot | null,
  visibleAnchors: Map<number, VisibleNodeAnchor>,
  settings: CrossBranchContextSettings,
): ZUIProxyResolution {
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
  const connectorsByActualPair = new Map<string, ZUIResolvedConnector[]>()
  for (const connector of connectors) {
    const pairKey = `${Math.min(connector.sourceElementId, connector.targetElementId)}::${Math.max(connector.sourceElementId, connector.targetElementId)}`
    const family = connectorsByActualPair.get(pairKey)
    if (family) family.push(connector)
    else connectorsByActualPair.set(pairKey, [connector])
  }

  const provenanceKeys = new Set<string>()
  for (const family of connectorsByActualPair.values()) {
    if (family.length < 2) continue
    const sorted = [...family].sort((left, right) => {
      if (left.maxDepth !== right.maxDepth) return left.maxDepth - right.maxDepth
      return (left.sourceDepth + left.targetDepth) - (right.sourceDepth + right.targetDepth)
    })
    for (const connector of sorted.slice(1)) provenanceKeys.add(connector.key)
  }

  for (const connector of connectors) {
    const source = visibleAnchorsByNodeId.get(connector.sourceNodeId)
    const target = visibleAnchorsByNodeId.get(connector.targetNodeId)
    if (!source || !target) continue
    const alpha = Math.min(source.renderAlpha, target.renderAlpha)
    if (alpha < 0.01) continue

    const { sourcePoint, targetPoint } = getDirectAnchorPoints(source, target)
    const midX = (sourcePoint.x + targetPoint.x) / 2
    const midY = (sourcePoint.y + targetPoint.y) / 2
    const label = String(connector.details.count)

    ctx.save()
    const isProvenanceStub = provenanceKeys.has(connector.key)
    if (isProvenanceStub) {
      const stubFromSource = connector.sourceDepth >= connector.targetDepth
      const start = stubFromSource ? sourcePoint : targetPoint
      const end = stubFromSource ? targetPoint : sourcePoint
      const dx = end.x - start.x
      const dy = end.y - start.y
      const length = Math.hypot(dx, dy)
      if (length <= 0) {
        ctx.restore()
        continue
      }

      const stubLength = Math.min(length * 0.34, 120 / zoom)
      const ux = dx / length
      const uy = dy / length
      const stubEndX = start.x + ux * stubLength
      const stubEndY = start.y + uy * stubLength
      const gradient = ctx.createLinearGradient(start.x, start.y, stubEndX, stubEndY)
      gradient.addColorStop(0, `rgba(255, 255, 255, ${0.36 * connectorAlpha(alpha)})`)
      gradient.addColorStop(1, 'rgba(255, 255, 255, 0)')
      ctx.strokeStyle = gradient
      ctx.lineWidth = 2 / zoom
      ctx.beginPath()
      ctx.moveTo(start.x, start.y)
      ctx.lineTo(stubEndX, stubEndY)
      ctx.stroke()
      ctx.restore()
      continue
    }

    ctx.globalAlpha = connectorAlpha(alpha)
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.46)'
    ctx.lineWidth = 2.4 / zoom
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
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.38)'
    ctx.lineWidth = 1 / zoom
    ctx.stroke()
    ctx.fillStyle = 'white'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(label, labelPos.x, labelPos.y)

    ctx.restore()
  }
}

export function drawVisibleDirectProxyBadges(
  ctx: CanvasRenderingContext2D,
  badges: ZUIHiddenProxyBadge[],
  visibleAnchorsByNodeId: Map<string, VisibleNodeAnchor>,
  zoom: number,
  labelBg: string,
  occupiedLabelRects: ScreenRect[],
) {
  for (const badge of badges) {
    const source = visibleAnchorsByNodeId.get(badge.sourceNodeId)
    const target = visibleAnchorsByNodeId.get(badge.targetNodeId)
    if (!source || !target) continue
    const alpha = Math.min(source.renderAlpha, target.renderAlpha)
    if (alpha < 0.01) continue

    const { sourcePoint, targetPoint } = getDirectAnchorPoints(source, target)
    const midX = (sourcePoint.x + targetPoint.x) / 2
    const midY = (sourcePoint.y + targetPoint.y) / 2
    const label = `+${badge.count}`

    ctx.save()
    ctx.globalAlpha = alpha
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
    const px = 7 / zoom
    const badgeW = Math.max(24 / zoom, textW + px * 2)
    const badgeH = 24 / zoom
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
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.5)'
    ctx.lineWidth = 1 / zoom
    ctx.setLineDash([4 / zoom, 3 / zoom])
    ctx.stroke()
    ctx.setLineDash([])
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
    const { sourcePoint, targetPoint } = getDirectAnchorPoints(source, target)
    const x1 = sourcePoint.x
    const y1 = sourcePoint.y
    const x2 = targetPoint.x
    const y2 = targetPoint.y
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
