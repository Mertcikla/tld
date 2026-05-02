import {
  resolveZUIProxyConnectors,
  type ZUIHiddenProxyBadge,
  type ZUIViewportBounds,
  type ZUIProxyResolution,
  type ZUIResolvedConnector,
} from '../../crossBranch/resolve'
import type { WorkspaceGraphSnapshot } from '../../crossBranch/types'
import type { LayoutNode, ZUIViewState, HoveredItem } from './types'
import { getExpandThresholds, pickEdgeLabelPosition, type ScreenRect } from './renderer'
import type { CrossBranchContextSettings } from '../../crossBranch/types'
import { DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA } from '../../crossBranch/settings'

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

function getDevicePixelRatio(): number {
  return typeof window !== 'undefined' ? window.devicePixelRatio || 1 : 1
}

function roundRectPath(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  ctx.beginPath()
  ctx.roundRect(x, y, w, h, r)
}

function drawFixedScreenProxyBadge(
  ctx: CanvasRenderingContext2D,
  label: string,
  labelPos: { x: number; y: number },
  badgeCssW: number,
  badgeCssH: number,
  labelBg: string,
  strokeStyle: string,
  lineDashCss: number[],
  fontWeight = 600,
) {
  const matrix = ctx.getTransform()
  const dpr = getDevicePixelRatio()
  const centerX = matrix.a * labelPos.x + matrix.c * labelPos.y + matrix.e
  const centerY = matrix.b * labelPos.x + matrix.d * labelPos.y + matrix.f
  const badgeW = badgeCssW * dpr
  const badgeH = badgeCssH * dpr
  const radius = badgeH / 2

  ctx.save()
  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.fillStyle = labelBg
  roundRectPath(ctx, centerX - badgeW / 2, centerY - badgeH / 2, badgeW, badgeH, radius)
  ctx.fill()
  ctx.strokeStyle = strokeStyle
  ctx.lineWidth = dpr
  ctx.setLineDash(lineDashCss.map((value) => value * dpr))
  ctx.stroke()
  ctx.setLineDash([])
  ctx.fillStyle = 'white'
  ctx.font = `${fontWeight} ${11 * dpr}px Inter, system-ui, sans-serif`
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(label, centerX, centerY)
  ctx.restore()
}

function measureProxyBadge(ctx: CanvasRenderingContext2D, label: string, zoom: number, fontWeight = 600) {
  ctx.save()
  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.font = `${fontWeight} 11px Inter, system-ui, sans-serif`
  const textW = ctx.measureText(label).width
  ctx.restore()

  const badgeCssW = Math.max(24, textW + 14)
  const badgeCssH = 24
  return {
    badgeCssW,
    badgeCssH,
    worldW: badgeCssW / zoom,
    worldH: badgeCssH / zoom,
  }
}

export function buildVisibleProxyConnectors(
  snapshot: WorkspaceGraphSnapshot | null,
  visibleAnchors: Map<number, VisibleNodeAnchor>,
  settings: CrossBranchContextSettings,
  viewport?: ZUIViewportBounds | null,
): ZUIProxyResolution {
  const minAlpha = settings.minConnectorAnchorAlpha ?? DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA
  const eligibleAnchors = Array.from(visibleAnchors.entries())
    .filter(([, anchor]) => anchor.renderAlpha >= minAlpha)
  const connectorAnchors = new Map(eligibleAnchors.map(([elementId, anchor]) => [elementId, anchor.nodeId]))
  const anchorsByElementId = new Map(eligibleAnchors.map(([elementId, anchor]) => [elementId, {
    nodeId: anchor.nodeId,
    worldX: anchor.worldX,
    worldY: anchor.worldY,
    worldW: anchor.worldW,
    worldH: anchor.worldH,
  }]))
  return resolveZUIProxyConnectors(
    snapshot,
    connectorAnchors,
    settings,
    { viewport, anchorsByElementId },
  )
}

export function drawVisibleProxyConnectors(
  ctx: CanvasRenderingContext2D,
  connectors: ZUIResolvedConnector[],
  visibleAnchorsByNodeId: Map<string, VisibleNodeAnchor>,
  zoom: number,
  labelBg: string,
  accent: string,
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
      ctx.restore()
      continue
    }

    ctx.globalAlpha = connectorAlpha(alpha) * 0.8
    ctx.strokeStyle = accent
    ctx.lineWidth = 1 / zoom
    ctx.lineCap = 'round'
    ctx.setLineDash([1 / zoom, 4 / zoom])
    ctx.beginPath()
    ctx.moveTo(sourcePoint.x, sourcePoint.y)
    ctx.lineTo(targetPoint.x, targetPoint.y)
    ctx.stroke()
    ctx.setLineDash([])
    const badge = measureProxyBadge(ctx, label, zoom)
    const labelPos = pickEdgeLabelPosition(
      ctx.getTransform(),
      midX,
      midY,
      badge.worldW,
      badge.worldH,
      targetPoint.x - sourcePoint.x,
      targetPoint.y - sourcePoint.y,
      occupiedLabelRects,
    )
    drawFixedScreenProxyBadge(ctx, label, labelPos, badge.badgeCssW, badge.badgeCssH, labelBg, accent, [1, 4])

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
    const badgeMetrics = measureProxyBadge(ctx, label, zoom)
    const labelPos = pickEdgeLabelPosition(
      ctx.getTransform(),
      midX,
      midY,
      badgeMetrics.worldW,
      badgeMetrics.worldH,
      targetPoint.x - sourcePoint.x,
      targetPoint.y - sourcePoint.y,
      occupiedLabelRects,
    )
    drawFixedScreenProxyBadge(
      ctx,
      label,
      labelPos,
      badgeMetrics.badgeCssW,
      badgeMetrics.badgeCssH,
      labelBg,
      'rgba(255, 255, 255, 0.5)',
      [4, 3],
    )
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
