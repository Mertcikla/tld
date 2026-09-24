import { useMemo } from 'react'
import { type Edge as RFEdge, type Node as RFNode } from 'reactflow'
import type { Connector, LibraryElement, PlacedElement } from '../../../types'
import type {
  CrossBranchContextSettings,
  ProxyConnectorDetails,
  ProxyConnectorLeaf,
  ProxyEndpoint,
  WorkspaceGraphSnapshot,
} from '../../../crossBranch/types'
import { resolveViewProxyGraph } from '../../../crossBranch/resolve'
import { placedElementToLibraryElement } from '../../../store/useStore'
import { canonicalNodePairKey } from '../pairKey'
import { Z_CONTEXT_BOUNDARY, Z_CONTEXT_GROUP_ANCHOR, Z_CONTEXT_NODE } from '../../../utils/zOrder'
import {
  buildContextSummaryForest,
  buildVisibleContextSummaryForest,
  contextSummaryLeafId,
  type VisibleContextSummaryForest,
} from './contextSummaryTree'

interface Props {
  snapshot: WorkspaceGraphSnapshot | null
  settings: CrossBranchContextSettings
  viewId: number | null
  viewElements: PlacedElement[]
  rfNodes: RFNode[]
  stableOnNavigateToView: (id: number) => void
  contextNodePositionOverrides: Record<string, ContextNodePositionOverride>
  onSelectContextElement: (element: LibraryElement) => void
  onSelectProxyDetails: (details: ProxyConnectorDetails) => void
  expandedAncestorGroups: Set<string>
  onToggleAncestorGroup: (anchorId: string) => void
}

export type ContextSide = 'top' | 'bottom' | 'left' | 'right'

export interface ContextNodePositionOverride {
  side: ContextSide
  axisPosition: number
}

export interface ContextBoundaryBounds {
  left: number
  right: number
  top: number
  bottom: number
}

export interface ContextRootLayoutSeed {
  nodeId: string
  centerX: number
  centerY: number
}

export interface ExternalContextGroup {
  pairKey: string
  source: string
  target: string
  contextNodeId: string
  currentNodeId: string
  connectors: ProxyConnectorLeaf[]
}

const CONTEXT_NODE_W = 200
const CONTEXT_NODE_H = 100
const CONTEXT_NODE_HALF_W = CONTEXT_NODE_W / 2
const CONTEXT_NODE_HALF_H = CONTEXT_NODE_H / 2
const HORIZONTAL_STACK_SPACING = 128
const VERTICAL_STACK_SPACING = 74
const HORIZONTAL_BOUNDARY_CLEARANCE = 72
const VERTICAL_BOUNDARY_CLEARANCE = 44
// Extra space to clear the chevron button when a cluster is expanded.
// The chevron sits at 75% of the node's layout dimension (scale-0.5 visual edge) + 6px offset.
// For right side: visual right ≈ 150px, chevron ~36px wide → children need ~80px extra.
// For bottom side: visual bottom ≈ 75px, chevron ~14px tall → children need ~30px extra.
const CHEVRON_H_CLEARANCE = 30
const CHEVRON_V_CLEARANCE = 10
const SIDE_CLUSTER_THRESHOLD = 90
const TOP_BOTTOM_CLUSTER_THRESHOLD = 140

function stableAngleFromId(id: string) {
  let hash = 0
  for (let i = 0; i < id.length; i += 1) hash = ((hash << 5) - hash) + id.charCodeAt(i)
  return ((hash % 360) * Math.PI) / 180
}

function averageAngles(angles: number[]): number {
  if (angles.length === 0) return 0
  const sum = angles.reduce((acc, angle) => ({
    x: acc.x + Math.cos(angle),
    y: acc.y + Math.sin(angle),
  }), { x: 0, y: 0 })
  if (sum.x === 0 && sum.y === 0) return angles[0]
  return Math.atan2(sum.y, sum.x)
}

function classifySide(angle: number): ContextSide {
  const dx = Math.cos(angle)
  const dy = Math.sin(angle)
  if (Math.abs(dx) > Math.abs(dy)) return dx < 0 ? 'left' : 'right'
  return dy < 0 ? 'top' : 'bottom'
}

function canonicalElementPairKey(leftId: number, rightId: number) {
  return leftId <= rightId ? `${leftId}::${rightId}` : `${rightId}::${leftId}`
}

export function clampContextNodeAxisPosition(
  side: ContextSide,
  axisPosition: number,
  bounds: ContextBoundaryBounds,
) {
  if (side === 'left' || side === 'right') {
    return Math.max(bounds.top - CONTEXT_NODE_HALF_H, Math.min(axisPosition, bounds.bottom - CONTEXT_NODE_HALF_H))
  }
  return Math.max(bounds.left - CONTEXT_NODE_HALF_W, Math.min(axisPosition, bounds.right - CONTEXT_NODE_HALF_W))
}

function applyContextNodePositionOverride(
  position: { x: number; y: number },
  side: ContextSide,
  override: ContextNodePositionOverride | undefined,
  bounds: ContextBoundaryBounds,
) {
  if (!override || override.side !== side) return position
  if (side === 'left' || side === 'right') {
    return { ...position, y: clampContextNodeAxisPosition(side, override.axisPosition, bounds) }
  }
  return { ...position, x: clampContextNodeAxisPosition(side, override.axisPosition, bounds) }
}

function buildContextNodeExtent(
  side: ContextSide,
  position: { x: number; y: number },
  bounds: ContextBoundaryBounds,
): [[number, number], [number, number]] {
  if (side === 'left' || side === 'right') {
    return [
      [position.x, bounds.top - CONTEXT_NODE_HALF_H],
      [position.x, bounds.bottom - CONTEXT_NODE_HALF_H],
    ]
  }

  return [
    [bounds.left - CONTEXT_NODE_HALF_W, position.y],
    [bounds.right - CONTEXT_NODE_HALF_W, position.y],
  ]
}

function buildDirectConnectorPairSet(connectors: Connector[], visibleElementIds: Set<number>) {
  const pairs = new Set<string>()
  for (const connector of connectors) {
    if (!visibleElementIds.has(connector.source_element_id) || !visibleElementIds.has(connector.target_element_id)) continue
    pairs.add(canonicalElementPairKey(connector.source_element_id, connector.target_element_id))
  }
  return pairs
}

function mergeHiddenProxyDetails(
  existing: ProxyConnectorDetails | undefined,
  next: ProxyConnectorDetails,
): ProxyConnectorDetails {
  if (!existing) {
    return {
      ...next,
      ownerViewIds: [...next.ownerViewIds],
      ownerViewNames: [...next.ownerViewNames],
      connectors: [...next.connectors],
    }
  }

  const ownerViews = new Map<number, string>()
  existing.ownerViewIds.forEach((ownerViewId, index) => {
    ownerViews.set(ownerViewId, existing.ownerViewNames[index] ?? `View ${ownerViewId}`)
  })
  next.ownerViewIds.forEach((ownerViewId, index) => {
    ownerViews.set(ownerViewId, next.ownerViewNames[index] ?? `View ${ownerViewId}`)
  })

  const connectors = [...existing.connectors, ...next.connectors]
  const count = connectors.length

  return {
    key: existing.key,
    label: count === 1 ? connectors[0]?.connector.label?.trim() || connectors[0]?.connector.relationship?.trim() || 'Cross-view' : `${count} connectors`,
    count,
    sourceAnchorId: existing.sourceAnchorId,
    targetAnchorId: existing.targetAnchorId,
    sourceAnchorName: existing.sourceAnchorName,
    targetAnchorName: existing.targetAnchorName,
    ownerViewIds: Array.from(ownerViews.keys()),
    ownerViewNames: Array.from(ownerViews.values()),
    connectors,
  }
}

function summarizeProxyConnectorLabel(connectors: ProxyConnectorDetails['connectors']) {
  if (connectors.length === 0) return 'Connectors'
  if (connectors.length === 1) {
    return connectors[0]?.connector.label?.trim() || connectors[0]?.connector.relationship?.trim() || 'Cross-view'
  }

  const labels = Array.from(new Set(connectors.map((leaf) => (
    leaf.connector.label?.trim() || leaf.connector.relationship?.trim() || ''
  )).filter(Boolean)))

  if (labels.length === 1) return `${connectors.length} x ${labels[0]}`
  return `${connectors.length} connectors`
}

function leafExternalEndpoint(leaf: ProxyConnectorLeaf): ProxyEndpoint | null {
  if (leaf.source.externalToView) return leaf.source
  if (leaf.target.externalToView) return leaf.target
  return null
}

function leafInternalEndpoint(leaf: ProxyConnectorLeaf): ProxyEndpoint | null {
  if (leaf.source.externalToView) return leaf.target
  if (leaf.target.externalToView) return leaf.source
  return null
}

function ownerViewsFromConnectorLeaves(connectors: ProxyConnectorLeaf[]) {
  const ownerViews = new Map<number, string>()
  connectors.forEach((leaf) => ownerViews.set(leaf.ownerViewId, leaf.ownerViewName))
  return {
    ownerViewIds: Array.from(ownerViews.keys()),
    ownerViewNames: Array.from(ownerViews.values()),
  }
}

function summaryConnectorDetails(
  key: string,
  sourceAnchorId: string,
  targetAnchorId: string,
  sourceAnchorName: string,
  targetAnchorName: string,
  connectors: ProxyConnectorLeaf[],
): ProxyConnectorDetails {
  const { ownerViewIds, ownerViewNames } = ownerViewsFromConnectorLeaves(connectors)
  return {
    key,
    label: summarizeProxyConnectorLabel(connectors),
    count: connectors.length,
    sourceAnchorId,
    targetAnchorId,
    sourceAnchorName,
    targetAnchorName,
    ownerViewIds,
    ownerViewNames,
    connectors,
  }
}

function contextElementSignature(element: PlacedElement | null) {
  if (!element) return null
  return [
    element.id,
    element.view_id,
    element.element_id,
    element.name,
    element.description,
    element.kind,
    element.technology,
    element.url,
    element.logo_url,
    element.technology_connectors,
    element.tags,
    element.repo,
    element.branch,
    element.file_path,
    element.language,
    element.bypass_noise_gate,
    element.has_view,
    element.view_label,
  ]
}

function proxyEndpointSignature(endpoint: ProxyEndpoint) {
  return [
    endpoint.actualElementId,
    endpoint.actualElementName,
    endpoint.anchorElementId,
    endpoint.anchorElementName,
    endpoint.anchorViewId,
    endpoint.anchorViewName,
    endpoint.placementViewId,
    endpoint.placementViewName,
    endpoint.depth,
    endpoint.externalToView,
    endpoint.currentBranchElementId,
    endpoint.commonAncestorViewId,
    endpoint.commonAncestorViewName,
    endpoint.mergeAncestorElementId,
    endpoint.contextPathElementIds,
    endpoint.branchPathElementIds,
  ]
}

function connectorSignature(connector: Connector) {
  return [
    connector.id,
    connector.view_id,
    connector.source_element_id,
    connector.target_element_id,
    connector.label,
    connector.description,
    connector.relationship,
    connector.direction,
    connector.style,
    connector.url,
    connector.source_handle,
    connector.target_handle,
    connector.tags,
    connector.created_at,
    connector.updated_at,
  ]
}

function proxyConnectorDetailsSignature(details: ProxyConnectorDetails | undefined) {
  if (!details) return null
  return [
    details.key,
    details.label,
    details.count,
    details.sourceAnchorId,
    details.targetAnchorId,
    details.sourceAnchorName,
    details.targetAnchorName,
    details.ownerViewIds,
    details.ownerViewNames,
    details.connectors.map((leaf) => [
      leaf.ownerViewId,
      leaf.ownerViewName,
      connectorSignature(leaf.connector),
      proxyEndpointSignature(leaf.source),
      proxyEndpointSignature(leaf.target),
    ]),
  ]
}

const CONTEXT_NODE_RUNTIME_KEYS = new Set(['positionAbsolute', 'dragging', 'resizing', 'measured'])
const CONTEXT_NODE_DATA_CALLBACK_KEYS = new Set([
  'onNavigateToDiagram',
  'onSelectElement',
  'onOpenRelationshipDetails',
  'onToggleGroup',
])

function contextValueEqual(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true
  if (typeof left !== typeof right) return false
  if (left == null || right == null) return false

  if (Array.isArray(left) || Array.isArray(right)) {
    if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) return false
    return left.every((item, index) => contextValueEqual(item, right[index]))
  }

  if (typeof left !== 'object' || typeof right !== 'object') return false

  const leftRecord = left as Record<string, unknown>
  const rightRecord = right as Record<string, unknown>
  const keys = new Set([...Object.keys(leftRecord), ...Object.keys(rightRecord)])
  for (const key of keys) {
    if (!contextValueEqual(leftRecord[key], rightRecord[key])) return false
  }
  return true
}

function contextNodeDataEqual(left: unknown, right: unknown) {
  if (Object.is(left, right)) return true
  if (!left || !right || typeof left !== 'object' || typeof right !== 'object') return false

  const leftRecord = left as Record<string, unknown>
  const rightRecord = right as Record<string, unknown>
  const keys = new Set([...Object.keys(leftRecord), ...Object.keys(rightRecord)])
  for (const key of keys) {
    const leftValue = leftRecord[key]
    const rightValue = rightRecord[key]
    if (CONTEXT_NODE_DATA_CALLBACK_KEYS.has(key)) {
      if (typeof leftValue !== typeof rightValue) return false
      continue
    }
    if (!contextValueEqual(leftValue, rightValue)) return false
  }
  return true
}

function contextNodeEqual(left: RFNode, right: RFNode) {
  const keys = new Set([...Object.keys(left), ...Object.keys(right)])
  for (const key of keys) {
    if (CONTEXT_NODE_RUNTIME_KEYS.has(key)) continue

    const leftValue = left[key as keyof RFNode]
    const rightValue = right[key as keyof RFNode]
    if (key === 'data') {
      if (!contextNodeDataEqual(leftValue, rightValue)) return false
      continue
    }
    if (!contextValueEqual(leftValue, rightValue)) return false
  }
  return true
}

function withMeasuredContextNodeDimensions(node: RFNode, existing: RFNode | undefined): RFNode {
  if (existing?.width == null || existing.height == null) return node
  if (node.width === existing.width && node.height === existing.height) return node
  return { ...node, width: existing.width, height: existing.height }
}

export function reconcileMeasuredContextNodes(prev: RFNode[], contextNodes: RFNode[]) {
  const prevById = new Map(prev.map((node) => [node.id, node]))
  let changed = prev.length !== contextNodes.length

  const next = contextNodes.map((node, index) => {
    const existing = prevById.get(node.id)
    const measuredNode = withMeasuredContextNodeDimensions(node, existing)
    const nextNode = existing && contextNodeEqual(existing, measuredNode) ? existing : measuredNode
    if (nextNode !== prev[index]) changed = true
    return nextNode
  })

  return changed ? next : prev
}

function visibleSummarySubtreeSpan(
  nodeId: string,
  forest: VisibleContextSummaryForest,
  cache: Map<string, number>,
): number {
  const cached = cache.get(nodeId)
  if (cached != null) return cached
  const node = forest.nodesById[nodeId]
  if (!node || node.visibleChildIds.length === 0) {
    cache.set(nodeId, 1)
    return 1
  }
  const span = Math.max(1, node.visibleChildIds.reduce((sum, childId) => sum + visibleSummarySubtreeSpan(childId, forest, cache), 0))
  cache.set(nodeId, span)
  return span
}

export function contextNodeFanoutBudget(settings: CrossBranchContextSettings): number {
  const connectorBudget = settings.connectorBudget ?? 50
  if (connectorBudget <= 10) return 1
  if (connectorBudget <= 25) return 2
  if (connectorBudget <= 50) return 3
  if (connectorBudget <= 100) return 5
  return 8
}

function compareExternalContextGroups(
  left: ExternalContextGroup,
  right: ExternalContextGroup,
  forest: VisibleContextSummaryForest,
) {
  if (right.connectors.length !== left.connectors.length) {
    return right.connectors.length - left.connectors.length
  }

  const depthDelta = (forest.nodesById[left.contextNodeId]?.depth ?? 0) - (forest.nodesById[right.contextNodeId]?.depth ?? 0)
  if (depthDelta !== 0) return depthDelta

  const leafDelta =
    (forest.nodesById[right.contextNodeId]?.descendantLeafIds.length ?? 0) -
    (forest.nodesById[left.contextNodeId]?.descendantLeafIds.length ?? 0)
  if (leafDelta !== 0) return leafDelta

  return left.pairKey.localeCompare(right.pairKey)
}

export function budgetExternalContextGroups(
  groups: ExternalContextGroup[],
  forest: VisibleContextSummaryForest,
  settings: CrossBranchContextSettings,
): ExternalContextGroup[] {
  if (groups.length === 0) return []

  const perNodeBudget = contextNodeFanoutBudget(settings)
  const groupsByContextNodeId = new Map<string, ExternalContextGroup[]>()
  groups.forEach((group) => {
    const bucket = groupsByContextNodeId.get(group.contextNodeId)
    if (bucket) bucket.push(group)
    else groupsByContextNodeId.set(group.contextNodeId, [group])
  })

  return Array.from(groupsByContextNodeId.values())
    .flatMap((bucket) => bucket
      .slice()
      .sort((left, right) => compareExternalContextGroups(left, right, forest))
      .slice(0, perNodeBudget))
    .sort((left, right) => compareExternalContextGroups(left, right, forest))
}

export function buildSummaryLayoutById(
  forest: VisibleContextSummaryForest,
  rootLayoutsBySide: Record<ContextSide, ContextRootLayoutSeed[]>,
  bounds: ContextBoundaryBounds,
): Map<string, { position: { x: number; y: number }; side: ContextSide }> {
  const summaryLayoutById = new Map<string, { position: { x: number; y: number }; side: ContextSide }>()
  const spanCache = new Map<string, number>()

  ;(['top', 'bottom', 'left', 'right'] as const).forEach((side) => {
    const sideRoots = [...rootLayoutsBySide[side]].sort((left, right) => {
      const leftCoord = side === 'left' || side === 'right' ? left.centerY : left.centerX
      const rightCoord = side === 'left' || side === 'right' ? right.centerY : right.centerX
      return leftCoord - rightCoord
    })
    if (sideRoots.length === 0) return

    const threshold = side === 'left' || side === 'right' ? SIDE_CLUSTER_THRESHOLD : TOP_BOTTOM_CLUSTER_THRESHOLD
    const clusters: ContextRootLayoutSeed[][] = []
    sideRoots.forEach((layout) => {
      const coord = side === 'left' || side === 'right' ? layout.centerY : layout.centerX
      const cluster = clusters[clusters.length - 1]
      if (!cluster) {
        clusters.push([layout])
        return
      }

      const last = cluster[cluster.length - 1]
      const lastCoord = side === 'left' || side === 'right' ? last.centerY : last.centerX
      if (Math.abs(coord - lastCoord) <= threshold) cluster.push(layout)
      else clusters.push([layout])
    })

    clusters.forEach((cluster) => {
      const axisGap = side === 'left' || side === 'right' ? VERTICAL_STACK_SPACING : HORIZONTAL_STACK_SPACING
      const depthSpacing = side === 'left' || side === 'right'
        ? HORIZONTAL_STACK_SPACING + CHEVRON_H_CLEARANCE
        : VERTICAL_STACK_SPACING + CHEVRON_V_CLEARANCE
      const clusterAnchorCoord = cluster.reduce((sum, item) => sum + (side === 'left' || side === 'right' ? item.centerY : item.centerX), 0) / cluster.length
      const rootStart = clusterAnchorCoord - ((cluster.length - 1) * axisGap) / 2

      const assignNodeLayout = (nodeId: string, axisCenter: number, depth: number) => {
        const node = forest.nodesById[nodeId]
        if (!node) return

        let x: number
        let y: number
        if (side === 'top') {
          x = axisCenter - CONTEXT_NODE_HALF_W
          y = bounds.top - VERTICAL_BOUNDARY_CLEARANCE - depth * depthSpacing - CONTEXT_NODE_HALF_H
        } else if (side === 'bottom') {
          x = axisCenter - CONTEXT_NODE_HALF_W
          y = bounds.bottom + VERTICAL_BOUNDARY_CLEARANCE + depth * depthSpacing - CONTEXT_NODE_HALF_H
        } else if (side === 'left') {
          x = bounds.left - HORIZONTAL_BOUNDARY_CLEARANCE - depth * depthSpacing - CONTEXT_NODE_HALF_W
          y = axisCenter - CONTEXT_NODE_HALF_H
        } else {
          x = bounds.right + HORIZONTAL_BOUNDARY_CLEARANCE + depth * depthSpacing - CONTEXT_NODE_HALF_W
          y = axisCenter - CONTEXT_NODE_HALF_H
        }
        summaryLayoutById.set(nodeId, { position: { x, y }, side })

        if (node.visibleChildIds.length === 0) return

        const totalSpan = Math.max(1, node.visibleChildIds.reduce(
          (sum, childId) => sum + visibleSummarySubtreeSpan(childId, forest, spanCache),
          0,
        ))
        const childStart = axisCenter - ((totalSpan - 1) * axisGap) / 2

        let nextUnit = 0
        node.visibleChildIds.forEach((childId) => {
          const childSpan = visibleSummarySubtreeSpan(childId, forest, spanCache)
          const childAxisCenter = childStart + (nextUnit + (childSpan - 1) / 2) * axisGap
          assignNodeLayout(childId, childAxisCenter, depth + 1)
          nextUnit += childSpan
        })
      }

      cluster.forEach(({ nodeId }, index) => {
        assignNodeLayout(nodeId, rootStart + index * axisGap, 0)
      })
    })
  })

  return summaryLayoutById
}

export function useViewContextNeighbours({
  snapshot,
  settings,
  viewId,
  viewElements,
  rfNodes,
  stableOnNavigateToView,
  contextNodePositionOverrides,
  onSelectContextElement,
  onSelectProxyDetails,
  expandedAncestorGroups,
  onToggleAncestorGroup,
}: Props) {
  return useMemo(() => {
    if (!snapshot || viewId == null || !settings.enabled) {
      return {
        contextNodes: [] as RFNode[],
        contextConnectors: [] as RFEdge[],
        proxyConnectorDetailsByKey: {} as Record<string, ProxyConnectorDetails>,
        hiddenProxyCountsByPair: {} as Record<string, number>,
        hiddenProxyDetailsByPair: {} as Record<string, ProxyConnectorDetails>,
      }
    }

    const { proxyConnectors, proxyConnectorDetailsByKey } = resolveViewProxyGraph(snapshot, viewId, viewElements, settings)
    if (proxyConnectors.length === 0) {
      return {
        contextNodes: [] as RFNode[],
        contextConnectors: [] as RFEdge[],
        proxyConnectorDetailsByKey,
        hiddenProxyCountsByPair: {} as Record<string, number>,
        hiddenProxyDetailsByPair: {} as Record<string, ProxyConnectorDetails>,
      }
    }

    const mainNodes = rfNodes.filter((node) => node.type === 'elementNode')
    if (mainNodes.length === 0) {
      return {
        contextNodes: [] as RFNode[],
        contextConnectors: [] as RFEdge[],
        proxyConnectorDetailsByKey,
        hiddenProxyCountsByPair: {} as Record<string, number>,
        hiddenProxyDetailsByPair: {} as Record<string, ProxyConnectorDetails>,
      }
    }
    const visibleElementIds = new Set(viewElements.map((element) => element.element_id))
    const currentElementsById = new Map(viewElements.map((element) => [element.element_id, element] as const))
    const directConnectorPairs = buildDirectConnectorPairSet(snapshot.connectorsByViewId[viewId] ?? [], visibleElementIds)

    let minX = Infinity
    let minY = Infinity
    let maxX = -Infinity
    let maxY = -Infinity
    for (const node of mainNodes) {
      const width = node.width ?? 200
      const height = node.height ?? 90
      minX = Math.min(minX, node.position.x)
      minY = Math.min(minY, node.position.y)
      maxX = Math.max(maxX, node.position.x + width)
      maxY = Math.max(maxY, node.position.y + height)
    }

    const centerX = (minX + maxX) / 2
    const centerY = (minY + maxY) / 2
    const boundaryW = maxX - minX
    const boundaryH = maxY - minY
    const totalInset = 200
    const padding = 180
    const radiusX = boundaryW / 2 + padding
    const radiusY = boundaryH / 2 + padding
    const boundaryLeft = minX - totalInset
    const boundaryRight = maxX + totalInset
    const boundaryTop = minY - totalInset
    const boundaryBottom = maxY + totalInset
    const boundaryBounds = {
      left: boundaryLeft,
      right: boundaryRight,
      top: boundaryTop,
      bottom: boundaryBottom,
    }

    const livePositions = new Map(rfNodes.map((node) => [node.id, node.position] as const))
    const currentViewPositions = new Map(viewElements.map((element) => {
      const live = livePositions.get(String(element.element_id))
      return [
        element.element_id,
        live ? { ...element, position_x: live.x, position_y: live.y } : element,
      ] as const
    }))

    const allConnectorLeaves = proxyConnectors.flatMap((connector) => connector.details.connectors)
    const externalConnectorLeaves = allConnectorLeaves.filter((leaf) => leaf.source.externalToView || leaf.target.externalToView)
    const internalOnlyProxyConnectors = proxyConnectors.filter((connector) => connector.details.connectors.every((leaf) => !leaf.source.externalToView && !leaf.target.externalToView))

    const externalEndpoints = externalConnectorLeaves
      .map((leaf) => leafExternalEndpoint(leaf))
      .filter((endpoint): endpoint is ProxyEndpoint => endpoint != null)

    const connectorLeavesBySummaryLeafId = new Map<string, ProxyConnectorLeaf[]>()
    externalConnectorLeaves.forEach((leaf) => {
      const external = leafExternalEndpoint(leaf)
      if (!external) return
      const leafId = contextSummaryLeafId(external)
      const group = connectorLeavesBySummaryLeafId.get(leafId)
      if (group) group.push(leaf)
      else connectorLeavesBySummaryLeafId.set(leafId, [leaf])
    })

    const summaryForest = buildContextSummaryForest(externalEndpoints)
    const visibleSummaryForest = buildVisibleContextSummaryForest(summaryForest, expandedAncestorGroups, settings)
    const summaryNodeConnectorLeavesById = new Map<string, ProxyConnectorLeaf[]>()
    Object.values(visibleSummaryForest.nodesById).forEach((node) => {
      summaryNodeConnectorLeavesById.set(
        node.id,
        node.descendantLeafIds.flatMap((leafId) => connectorLeavesBySummaryLeafId.get(leafId) ?? []),
      )
    })

    const currentNodeNameById = new Map<string, string>(viewElements.map((element) => [String(element.element_id), element.name]))
    const summaryNodeNameById = new Map<string, string>()
    const summaryNodeDetailsById = new Map<string, ProxyConnectorDetails>()

    const ContextBoundaryElement: RFNode = {
      id: 'context-boundary',
      type: 'ContextBoundaryElement',
      position: { x: minX - totalInset, y: minY - totalInset },
      width: boundaryW + totalInset * 2,
      height: boundaryH + totalInset * 2,
      selectable: false,
      draggable: false,
      connectable: false,
      zIndex: Z_CONTEXT_BOUNDARY,
      data: {
        width: boundaryW + totalInset * 2,
        height: boundaryH + totalInset * 2,
        parentName: snapshot.viewById[viewId]?.name ?? 'Current view',
        onNavigateToDiagram: () => { /* intentionally read-only */ },
      },
      style: {
        pointerEvents: 'none',
      },
    }

    const rootLayoutsBySide: Record<ContextSide, ContextRootLayoutSeed[]> = {
      top: [],
      bottom: [],
      left: [],
      right: [],
    }

    visibleSummaryForest.rootIds.forEach((rootId) => {
      const rootNode = visibleSummaryForest.nodesById[rootId]
      if (!rootNode) return

      const relatedAngles = (summaryNodeConnectorLeavesById.get(rootId) ?? []).map((leaf) => {
        const external = leafExternalEndpoint(leaf)
        const internal = leafInternalEndpoint(leaf)
        if (!external || !internal) return null

        if (external.anchorViewId === viewId) {
          const externalPos = currentViewPositions.get(external.anchorElementId)
          const internalPos = currentViewPositions.get(internal.anchorElementId)
          if (externalPos && internalPos) {
            return Math.atan2(externalPos.position_y - internalPos.position_y, externalPos.position_x - internalPos.position_x)
          }
        }

        if (external.commonAncestorViewId != null && external.currentBranchElementId != null) {
          const commonAncestorPositions = snapshot.placementsByViewId[external.commonAncestorViewId] ?? []
          const currentBranchPlacement = commonAncestorPositions.find((placement) => placement.element_id === external.currentBranchElementId)
          const externalPlacement = commonAncestorPositions.find((placement) => placement.element_id === external.anchorElementId)
          if (currentBranchPlacement && externalPlacement) {
            return Math.atan2(
              externalPlacement.position_y - currentBranchPlacement.position_y,
              externalPlacement.position_x - currentBranchPlacement.position_x,
            )
          }
        }

        const internalPos = currentViewPositions.get(internal.anchorElementId)
        if (internalPos) {
          return Math.atan2(internalPos.position_y - centerY, internalPos.position_x - centerX) + Math.PI
        }

        return null
      }).filter((angle): angle is number => angle != null)

      let angle = relatedAngles.length > 0 ? averageAngles(relatedAngles) : stableAngleFromId(rootId)
      const isDescendantRoot = rootNode.descendantLeafIds.some((leafId) => {
        const placementViewId = summaryForest.leavesById[leafId]?.endpoint.placementViewId
        return placementViewId != null && (snapshot.descendantsByViewId[viewId]?.includes(placementViewId) ?? false)
      })

      if (isDescendantRoot) angle = -Math.PI / 2
      const side = isDescendantRoot ? 'top' : classifySide(angle)
      rootLayoutsBySide[side].push({
        nodeId: rootId,
        centerX: centerX + Math.cos(angle) * radiusX,
        centerY: centerY + Math.sin(angle) * radiusY,
      })
    })

    const summaryLayoutById = buildSummaryLayoutById(visibleSummaryForest, rootLayoutsBySide, boundaryBounds)

    const contextNodes = Object.values(visibleSummaryForest.nodesById).reduce<RFNode[]>((nodes, summaryNode) => {
        const layout = summaryLayoutById.get(summaryNode.id)
        if (!layout) return nodes
        const adjustedPosition = applyContextNodePositionOverride(
          layout.position,
          layout.side,
          contextNodePositionOverrides[summaryNode.id],
          boundaryBounds,
        )
        const placements = snapshot.placementsByElementId[summaryNode.elementId] ?? []
        const placementViews = new Map<number, string>()
        placements.forEach((placement) => {
          placementViews.set(placement.viewId, placement.viewName)
        })
        const primaryPlacement = placements[0]?.element
        const representativeLeaf = summaryForest.leavesById[summaryNode.directLeafIds[0] ?? summaryNode.descendantLeafIds[0] ?? '']
        const representativeElement = primaryPlacement ?? currentElementsById.get(summaryNode.elementId) ?? null
        const name = representativeElement?.name
          ?? representativeLeaf?.endpoint.anchorElementName
          ?? representativeLeaf?.endpoint.actualElementName
          ?? `Element ${summaryNode.elementId}`
        const connectors = summaryNodeConnectorLeavesById.get(summaryNode.id) ?? []
        summaryNodeNameById.set(summaryNode.id, name)
        if (connectors.length > 0) {
          summaryNodeDetailsById.set(
            summaryNode.id,
            summaryConnectorDetails(
              `node:${summaryNode.id}`,
              summaryNode.id,
              summaryNode.id,
              name,
              'Multiple connections',
              connectors,
            ),
          )
        }
        const relationshipDetails = summaryNodeDetailsById.get(summaryNode.id)
        const displayElement = representativeElement
        const isGroupAnchor = summaryNode.childIds.length > 0 && (!summaryNode.isAutoExpanded || summaryNode.isExpanded)
        nodes.push({
          id: summaryNode.id,
          type: 'contextNeighborNode',
          position: adjustedPosition,
          extent: buildContextNodeExtent(layout.side, adjustedPosition, boundaryBounds),
          width: CONTEXT_NODE_W,
          height: CONTEXT_NODE_H,
          selectable: false,
          draggable: true,
          connectable: false,
          zIndex: isGroupAnchor && summaryNode.visibleChildIds.length > 0 ? Z_CONTEXT_GROUP_ANCHOR : Z_CONTEXT_NODE,
          data: {
            element_id: summaryNode.elementId,
            name,
            kind: displayElement?.kind ?? null,
            description: displayElement?.description ?? null,
            technology: displayElement?.technology ?? null,
            logo_url: displayElement?.logo_url ?? null,
            technology_connectors: displayElement?.technology_connectors ?? [],
            ownerViewIds: Array.from(placementViews.keys()),
            ownerViewNames: Array.from(placementViews.values()),
            currentViewId: viewId,
            commonAncestorViewId: representativeLeaf?.endpoint.commonAncestorViewId ?? null,
            commonAncestorViewName: representativeLeaf?.endpoint.commonAncestorViewName ?? null,
            connectorCount: connectors.length,
            contextElementSignature: contextElementSignature(displayElement),
            relationshipDetailsSignature: proxyConnectorDetailsSignature(relationshipDetails),
            onNavigateToView: stableOnNavigateToView,
            onSelectElement: displayElement
              ? () => onSelectContextElement(placedElementToLibraryElement(displayElement))
              : undefined,
            onOpenRelationshipDetails: relationshipDetails
              ? () => onSelectProxyDetails(relationshipDetails)
              : undefined,
            isGroupAnchor,
            groupChildCount: summaryNode.hiddenLeafCount,
            isGroupExpanded: summaryNode.visibleChildIds.length > 0,
            onToggleGroup: isGroupAnchor ? () => onToggleAncestorGroup(summaryNode.id) : undefined,
            side: layout.side,
          },
        })
        return nodes
      }, [])

    const externalGroups = new Map<string, ExternalContextGroup>()
    externalConnectorLeaves.forEach((leaf) => {
      const external = leafExternalEndpoint(leaf)
      const internal = leafInternalEndpoint(leaf)
      if (!external || !internal) return
      const leafId = contextSummaryLeafId(external)
      const contextNodeId = visibleSummaryForest.leafToVisibleNodeId[leafId]
      if (!contextNodeId) return
      const currentNodeId = String(internal.anchorElementId)
      const [source, target] = contextNodeId <= currentNodeId ? [contextNodeId, currentNodeId] : [currentNodeId, contextNodeId]
      const pairKey = canonicalNodePairKey(source, target)
      const group = externalGroups.get(pairKey)
      if (group) group.connectors.push(leaf)
      else externalGroups.set(pairKey, {
        pairKey,
        source,
        target,
        contextNodeId,
        currentNodeId,
        connectors: [leaf],
      })
    })

    const visibleExternalGroups = budgetExternalContextGroups(Array.from(externalGroups.values()), visibleSummaryForest, settings)

    const externalContextEdges: RFEdge[] = visibleExternalGroups.map((group) => ({
      id: `summary-proxy:${group.pairKey}`,
      source: group.source,
      target: group.target,
      type: 'proxyConnectorEdge',
      animated: false,
      selectable: true,
      updatable: false,
      data: {
        isProxy: true,
        proxyKey: `summary:${group.pairKey}`,
        details: summaryConnectorDetails(
          `summary:${group.pairKey}`,
          group.source,
          group.target,
          summaryNodeNameById.get(group.source) ?? currentNodeNameById.get(group.source) ?? group.source,
          summaryNodeNameById.get(group.target) ?? currentNodeNameById.get(group.target) ?? group.target,
          group.connectors,
        ),
      },
      style: {
        stroke: 'rgba(255, 255, 255, 0.2)',
        strokeWidth: 2,
      },
    }))

    const seenCollapsedPairs = new Set<string>()
    const hiddenProxyCountsByPair: Record<string, number> = {}
    const hiddenProxyDetailsByPair: Record<string, ProxyConnectorDetails> = {}
    const internalProxyEdges: RFEdge[] = internalOnlyProxyConnectors.flatMap((connector) => {
      if (connector.sourceAnchorId === connector.targetAnchorId) return []
      const pairKey = canonicalNodePairKey(connector.sourceAnchorId, connector.targetAnchorId)
      if (directConnectorPairs.has(pairKey)) {
        hiddenProxyCountsByPair[pairKey] = (hiddenProxyCountsByPair[pairKey] ?? 0) + connector.details.count
        hiddenProxyDetailsByPair[pairKey] = mergeHiddenProxyDetails(
          hiddenProxyDetailsByPair[pairKey],
          {
            ...connector.details,
            key: `hidden:${pairKey}`,
            sourceAnchorId: connector.sourceAnchorId,
            targetAnchorId: connector.targetAnchorId,
          },
        )
        return []
      }
      if (seenCollapsedPairs.has(pairKey)) return []
      seenCollapsedPairs.add(pairKey)

      return [{
        id: `proxy:${connector.key}`,
        source: connector.sourceAnchorId,
        target: connector.targetAnchorId,
        type: 'proxyConnectorEdge',
        animated: false,
        selectable: true,
        updatable: false,
        data: {
          isProxy: true,
          proxyKey: connector.key,
          details: connector.details,
        },
        style: {
          stroke: 'rgba(255, 255, 255, 0.2)',
          strokeWidth: 2,
        },
      }]
    })

    const contextConnectors: RFEdge[] = [...externalContextEdges, ...internalProxyEdges]

    return {
      contextNodes: contextNodes.length > 0 ? [ContextBoundaryElement, ...contextNodes] : contextNodes,
      contextConnectors,
      proxyConnectorDetailsByKey,
      hiddenProxyCountsByPair,
      hiddenProxyDetailsByPair,
    }
  }, [snapshot, settings, viewId, viewElements, rfNodes, stableOnNavigateToView, contextNodePositionOverrides, onSelectContextElement, onSelectProxyDetails, expandedAncestorGroups, onToggleAncestorGroup])
}
