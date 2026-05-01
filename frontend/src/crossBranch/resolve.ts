import type { Connector, PlacedElement } from '../types'
import { CROSS_BRANCH_CONNECTOR_BUDGET_DEFAULT, CROSS_BRANCH_DEPTH_ALL } from './types'
import type {
  AggregatedProxyConnector,
  CrossBranchConnectorPriority,
  CrossBranchContextSettings,
  GraphPlacementRef,
  ProxyConnectorDetails,
  ProxyConnectorLeaf,
  ProxyContextNode,
  ProxyEndpoint,
  WorkspaceGraphSnapshot,
} from './types'
import { allConnectors, findLowestCommonAncestorViewId, isDescendantView, relativeOwnerElementPath, viewName } from './graph'

function firstPlacementForElement(snapshot: WorkspaceGraphSnapshot, elementId: number): GraphPlacementRef | null {
  return snapshot.placementsByElementId[elementId]?.[0] ?? null
}

function placementInView(snapshot: WorkspaceGraphSnapshot, viewId: number | null, elementId: number): GraphPlacementRef | null {
  if (viewId == null) return null
  return snapshot.placementsByElementId[elementId]?.find((placement) => placement.viewId === viewId) ?? null
}

function elementDisplayPlacement(snapshot: WorkspaceGraphSnapshot, elementId: number, preferredViewId: number | null = null): GraphPlacementRef | null {
  return placementInView(snapshot, preferredViewId, elementId) ?? firstPlacementForElement(snapshot, elementId)
}

function ancestorDepth(snapshot: WorkspaceGraphSnapshot, viewId: number | null | undefined): number {
  if (viewId == null) return 0
  return snapshot.ancestorsByViewId[viewId]?.length ?? 0
}

function pathDistance(snapshot: WorkspaceGraphSnapshot, leftViewId: number, rightViewId: number): number {
  const lca = findLowestCommonAncestorViewId(snapshot, leftViewId, rightViewId)
  if (lca == null) return Number.MAX_SAFE_INTEGER
  const lcaDepth = ancestorDepth(snapshot, lca)
  return (ancestorDepth(snapshot, leftViewId) - lcaDepth) + (ancestorDepth(snapshot, rightViewId) - lcaDepth)
}

function appendElementToPath(path: number[], elementId: number): number[] {
  if (path[path.length - 1] === elementId) return path
  return [...path, elementId]
}

function explicitOffViewContextPath(
  snapshot: WorkspaceGraphSnapshot,
  currentViewId: number,
  currentVisibleElementIds: Set<number>,
  chosenPlacementViewId: number,
  elementId: number,
): number[] {
  if (isDescendantView(snapshot, chosenPlacementViewId, currentViewId)) {
    const owners = relativeOwnerElementPath(snapshot, currentViewId, chosenPlacementViewId)
    const lastVisibleOwnerIndex = owners.reduce((bestIndex, ownerElementId, index) => (
      currentVisibleElementIds.has(ownerElementId) ? index : bestIndex
    ), -1)
    return appendElementToPath(owners.slice(lastVisibleOwnerIndex + 1), elementId)
  }

  const commonAncestorViewId = findLowestCommonAncestorViewId(snapshot, currentViewId, chosenPlacementViewId)
  const owners = commonAncestorViewId == null
    ? []
    : relativeOwnerElementPath(snapshot, commonAncestorViewId, chosenPlacementViewId)
  return appendElementToPath(owners, elementId)
}

function chooseBestPlacement(snapshot: WorkspaceGraphSnapshot, elementId: number, focusViewId: number, ownerViewId: number): GraphPlacementRef | null {
  const candidates = snapshot.placementsByElementId[elementId] ?? []
  if (candidates.length === 0) return null

  const score = (placement: GraphPlacementRef) => {
    if (placement.viewId === focusViewId) return 0
    if (placement.viewId === ownerViewId) return 10
    if (isDescendantView(snapshot, placement.viewId, focusViewId)) {
      return 50 + Math.max(0, ancestorDepth(snapshot, placement.viewId) - ancestorDepth(snapshot, focusViewId))
    }

    const lca = findLowestCommonAncestorViewId(snapshot, focusViewId, placement.viewId)
    const lcaDepth = ancestorDepth(snapshot, lca)
    const focusDistance = pathDistance(snapshot, focusViewId, placement.viewId)
    const ownerDistance = pathDistance(snapshot, ownerViewId, placement.viewId)
    return 200 + focusDistance * 10 + ownerDistance * 2 - lcaDepth
  }

  return [...candidates].sort((left, right) => {
    const delta = score(left) - score(right)
    if (delta !== 0) return delta
    return left.viewId - right.viewId
  })[0]
}

function buildProxyEndpoint(
  snapshot: WorkspaceGraphSnapshot,
  currentViewId: number,
  currentVisibleElementIds: Set<number>,
  ownerViewId: number,
  elementId: number,
): ProxyEndpoint | null {
  const chosenPlacement = chooseBestPlacement(snapshot, elementId, currentViewId, ownerViewId)
  if (!chosenPlacement) return null

  const actualPlacement = elementDisplayPlacement(snapshot, elementId, chosenPlacement.viewId)
  const actualName = actualPlacement?.element.name ?? `Element ${elementId}`
  const forceExplicitOffViewContext = ownerViewId === currentViewId && chosenPlacement.viewId !== currentViewId

  if (forceExplicitOffViewContext) {
    const contextPathElementIds = explicitOffViewContextPath(
      snapshot,
      currentViewId,
      currentVisibleElementIds,
      chosenPlacement.viewId,
      elementId,
    )
    const mergeAncestorElementId = contextPathElementIds[0] ?? null
    const commonAncestorViewId = findLowestCommonAncestorViewId(snapshot, currentViewId, chosenPlacement.viewId)
    const currentBranchElementId = commonAncestorViewId == null
      ? null
      : commonAncestorViewId === currentViewId
        ? (relativeOwnerElementPath(snapshot, currentViewId, chosenPlacement.viewId)[0] ?? null)
        : (relativeOwnerElementPath(snapshot, commonAncestorViewId, currentViewId)[0] ?? null)

    return {
      actualElementId: elementId,
      actualElementName: actualName,
      anchorElementId: elementId,
      anchorElementName: actualName,
      anchorViewId: chosenPlacement.viewId,
      anchorViewName: chosenPlacement.viewName,
      placementViewId: chosenPlacement.viewId,
      placementViewName: chosenPlacement.viewName,
      depth: Math.max(1, pathDistance(snapshot, currentViewId, chosenPlacement.viewId)),
      externalToView: true,
      currentBranchElementId,
      commonAncestorViewId,
      commonAncestorViewName: viewName(snapshot, commonAncestorViewId),
      mergeAncestorElementId,
      contextPathElementIds,
    }
  }

  if (chosenPlacement.viewId === currentViewId || isDescendantView(snapshot, chosenPlacement.viewId, currentViewId)) {
    const descendantDepth = chosenPlacement.viewId === currentViewId
      ? 0
      : Math.max(1, ancestorDepth(snapshot, chosenPlacement.viewId) - ancestorDepth(snapshot, currentViewId))
    const owners = chosenPlacement.viewId === currentViewId
      ? []
      : relativeOwnerElementPath(snapshot, currentViewId, chosenPlacement.viewId)

    const candidateIds = [elementId, ...owners.slice().reverse()]
    const visibleAnchorId = candidateIds.find((candidate) => currentVisibleElementIds.has(candidate))
    if (visibleAnchorId == null) return null
    const visibleAnchorPlacement = elementDisplayPlacement(snapshot, visibleAnchorId, currentViewId)

    return {
      actualElementId: elementId,
      actualElementName: actualName,
      anchorElementId: visibleAnchorId,
      anchorElementName: visibleAnchorPlacement?.element.name ?? actualName,
      anchorViewId: currentViewId,
      anchorViewName: viewName(snapshot, currentViewId),
      placementViewId: chosenPlacement.viewId,
      placementViewName: chosenPlacement.viewName,
      depth: descendantDepth,
      externalToView: false,
      currentBranchElementId: null,
      commonAncestorViewId: currentViewId,
      commonAncestorViewName: viewName(snapshot, currentViewId),
      mergeAncestorElementId: null,
      contextPathElementIds: [],
    }
  }

  const commonAncestorViewId = findLowestCommonAncestorViewId(snapshot, currentViewId, chosenPlacement.viewId)
  const externalOwners = commonAncestorViewId == null
    ? []
    : relativeOwnerElementPath(snapshot, commonAncestorViewId, chosenPlacement.viewId)
  const anchorElementId = commonAncestorViewId == null
    ? elementId
    : commonAncestorViewId === chosenPlacement.viewId
      ? elementId
      : (externalOwners[0] ?? elementId)
  const anchorPlacement = elementDisplayPlacement(snapshot, anchorElementId, commonAncestorViewId)
  const currentBranchElementId = commonAncestorViewId == null || commonAncestorViewId === currentViewId
    ? null
    : (relativeOwnerElementPath(snapshot, commonAncestorViewId, currentViewId)[0] ?? null)

  return {
    actualElementId: elementId,
    actualElementName: actualName,
    anchorElementId,
    anchorElementName: anchorPlacement?.element.name ?? actualName,
    anchorViewId: commonAncestorViewId == null ? chosenPlacement.viewId : commonAncestorViewId,
    anchorViewName: commonAncestorViewId == null ? chosenPlacement.viewName : viewName(snapshot, commonAncestorViewId),
    placementViewId: chosenPlacement.viewId,
    placementViewName: chosenPlacement.viewName,
    depth: Math.max(1, externalOwners.length || 1),
    externalToView: true,
    currentBranchElementId,
    commonAncestorViewId,
    commonAncestorViewName: viewName(snapshot, commonAncestorViewId),
    mergeAncestorElementId: null,
    contextPathElementIds: [],
  }
}

function proxyDisplayLabel(connectors: ProxyConnectorLeaf[]): string {
  if (connectors.length === 1) {
    const [leaf] = connectors
    return leaf.connector.label?.trim() || leaf.connector.relationship?.trim() || 'Cross-branch'
  }
  const labels = new Set(connectors.map((leaf) => leaf.connector.label?.trim()).filter(Boolean))
  if (labels.size === 1) return `${connectors.length} × ${Array.from(labels)[0]}`
  return `${connectors.length} connectors`
}

function contextNodeKey(endpoint: ProxyEndpoint): string {
  return [
    'ctx',
    endpoint.anchorViewId ?? 'none',
    endpoint.anchorElementId,
    endpoint.commonAncestorViewId ?? 'none',
    endpoint.currentBranchElementId ?? 'none',
  ].join(':')
}

function visibleNodeKey(elementId: number): string {
  return String(elementId)
}

function displayNodeId(endpoint: ProxyEndpoint): string {
  return endpoint.externalToView ? contextNodeKey(endpoint) : visibleNodeKey(endpoint.anchorElementId)
}

function canonicalPairIds(leftId: string, rightId: string): [string, string] {
  return leftId <= rightId ? [leftId, rightId] : [rightId, leftId]
}

function canonicalPairElements(leftId: number, rightId: number): [number, number] {
  return leftId <= rightId ? [leftId, rightId] : [rightId, leftId]
}

function proxyDisplayGroupKey(leftId: string, rightId: string): string {
  const [first, second] = canonicalPairIds(leftId, rightId)
  return [first, second].join('::')
}

function ownerViewsFromLeaves(leaves: ProxyConnectorLeaf[]) {
  const ownerViews = new Map<number, string>()
  for (const leaf of leaves) {
    ownerViews.set(leaf.ownerViewId, leaf.ownerViewName)
  }
  return {
    ownerViewIds: Array.from(ownerViews.keys()),
    ownerViewNames: Array.from(ownerViews.values()),
  }
}

function collapseEndpointToAncestor(
  snapshot: WorkspaceGraphSnapshot,
  currentViewId: number,
  currentVisibleElementIds: Set<number>,
  ownerViewId: number,
  endpoint: ProxyEndpoint,
  collapseCounts: Map<number, number>,
): ProxyEndpoint {
  if (!endpoint.externalToView) {
    return endpoint
  }

  const collapseAncestorElementId = [...(endpoint.contextPathElementIds ?? [])]
    .reverse()
    .find((elementId) => (collapseCounts.get(elementId) ?? 0) >= 2)

  if (collapseAncestorElementId == null) {
    return endpoint
  }

  const visibleAncestorPlacement = placementInView(snapshot, currentViewId, collapseAncestorElementId)
  if (visibleAncestorPlacement && currentVisibleElementIds.has(collapseAncestorElementId)) {
    return {
      ...endpoint,
      anchorElementId: collapseAncestorElementId,
      anchorElementName: visibleAncestorPlacement.element.name,
      anchorViewId: currentViewId,
      anchorViewName: viewName(snapshot, currentViewId),
      placementViewId: currentViewId,
      placementViewName: viewName(snapshot, currentViewId),
      externalToView: false,
      currentBranchElementId: null,
      commonAncestorViewId: currentViewId,
      commonAncestorViewName: viewName(snapshot, currentViewId),
    }
  }

  const ancestorPlacement = chooseBestPlacement(snapshot, collapseAncestorElementId, currentViewId, ownerViewId)
    ?? firstPlacementForElement(snapshot, collapseAncestorElementId)
  const commonAncestorViewId = findLowestCommonAncestorViewId(
    snapshot,
    currentViewId,
    ancestorPlacement?.viewId ?? endpoint.anchorViewId ?? endpoint.placementViewId,
  )
  const currentBranchElementId = commonAncestorViewId == null || commonAncestorViewId === currentViewId
    ? null
    : (relativeOwnerElementPath(snapshot, commonAncestorViewId, currentViewId)[0] ?? null)

  return {
    ...endpoint,
    anchorElementId: collapseAncestorElementId,
    anchorElementName: ancestorPlacement?.element.name ?? endpoint.anchorElementName,
    anchorViewId: ancestorPlacement?.viewId ?? endpoint.anchorViewId,
    anchorViewName: ancestorPlacement?.viewName ?? endpoint.anchorViewName,
    placementViewId: ancestorPlacement?.viewId ?? endpoint.placementViewId,
    placementViewName: ancestorPlacement?.viewName ?? endpoint.placementViewName,
    currentBranchElementId,
    commonAncestorViewId,
    commonAncestorViewName: viewName(snapshot, commonAncestorViewId),
  }
}

function isNativeCurrentViewConnector(connector: Connector, currentViewId: number, currentVisibleElementIds: Set<number>): boolean {
  return connector.view_id === currentViewId &&
    currentVisibleElementIds.has(connector.source_element_id) &&
    currentVisibleElementIds.has(connector.target_element_id)
}

export interface ViewProxyGraphResult {
  proxyNodes: ProxyContextNode[]
  proxyConnectors: AggregatedProxyConnector[]
  proxyConnectorDetailsByKey: Record<string, ProxyConnectorDetails>
}

export function resolveViewProxyGraph(
  snapshot: WorkspaceGraphSnapshot | null,
  currentViewId: number | null,
  currentViewElements: PlacedElement[],
  settings: CrossBranchContextSettings,
): ViewProxyGraphResult {
  if (!snapshot || currentViewId == null || !settings.enabled) {
    return { proxyNodes: [], proxyConnectors: [], proxyConnectorDetailsByKey: {} }
  }

  const currentVisibleElementIds = new Set(currentViewElements.map((element) => element.element_id))
  const currentVisibleElementsById = new Map(currentViewElements.map((element) => [element.element_id, element]))

  const connectorLeaves: ProxyConnectorLeaf[] = []
  for (const connector of allConnectors(snapshot)) {
    if (isNativeCurrentViewConnector(connector, currentViewId, currentVisibleElementIds)) continue

    const source = buildProxyEndpoint(snapshot, currentViewId, currentVisibleElementIds, connector.view_id, connector.source_element_id)
    const target = buildProxyEndpoint(snapshot, currentViewId, currentVisibleElementIds, connector.view_id, connector.target_element_id)
    if (!source || !target) continue
    if (source.externalToView && target.externalToView) continue

    const maxDepth = Math.max(source.depth, target.depth)
    if (settings.depth < CROSS_BRANCH_DEPTH_ALL && maxDepth > settings.depth) continue

    const sourceAnchorId = displayNodeId(source)
    const targetAnchorId = displayNodeId(target)
    if (sourceAnchorId === targetAnchorId) continue
    if (!source.externalToView && !target.externalToView && connector.view_id === currentViewId) continue

    connectorLeaves.push({
      connector,
      ownerViewId: connector.view_id,
      ownerViewName: viewName(snapshot, connector.view_id) ?? `View ${connector.view_id}`,
      source,
      target,
    })
  }

  const collapseAncestorElements = new Map<number, Set<number>>()
  for (const leaf of connectorLeaves) {
    for (const endpoint of [leaf.source, leaf.target]) {
      for (const ancestorElementId of endpoint.contextPathElementIds ?? []) {
        const elements = collapseAncestorElements.get(ancestorElementId) ?? new Set<number>()
        elements.add(endpoint.actualElementId)
        collapseAncestorElements.set(ancestorElementId, elements)
      }
    }
  }
  const collapseCounts = new Map(
    Array.from(collapseAncestorElements.entries()).map(([ancestorElementId, elements]) => [ancestorElementId, elements.size]),
  )

  const collapsedConnectorLeaves = connectorLeaves.flatMap((leaf) => {
    const source = collapseEndpointToAncestor(
      snapshot,
      currentViewId,
      currentVisibleElementIds,
      leaf.ownerViewId,
      leaf.source,
      collapseCounts,
    )
    const target = collapseEndpointToAncestor(
      snapshot,
      currentViewId,
      currentVisibleElementIds,
      leaf.ownerViewId,
      leaf.target,
      collapseCounts,
    )
    if (displayNodeId(source) === displayNodeId(target)) {
      return []
    }
    return [{ ...leaf, source, target }]
  })

  const grouped = new Map<string, ProxyConnectorLeaf[]>()
  for (const leaf of collapsedConnectorLeaves) {
    const key = proxyDisplayGroupKey(displayNodeId(leaf.source), displayNodeId(leaf.target))
    const group = grouped.get(key)
    if (group) group.push(leaf)
    else grouped.set(key, [leaf])
  }

  const proxyNodesMap = new Map<string, ProxyContextNode>()
  const proxyConnectorDetailsByKey: Record<string, ProxyConnectorDetails> = {}
  const proxyConnectors: AggregatedProxyConnector[] = []

  for (const [key, leaves] of grouped) {
    const [first] = leaves
    const { ownerViewIds, ownerViewNames } = ownerViewsFromLeaves(leaves)
    const [canonicalSourceAnchorId, canonicalTargetAnchorId] = canonicalPairIds(
      displayNodeId(first.source),
      displayNodeId(first.target),
    )
    const canonicalFirstIsSource = canonicalSourceAnchorId === displayNodeId(first.source)
    const details: ProxyConnectorDetails = {
      key,
      label: proxyDisplayLabel(leaves),
      count: leaves.length,
      sourceAnchorId: canonicalSourceAnchorId,
      targetAnchorId: canonicalTargetAnchorId,
      sourceAnchorName: canonicalFirstIsSource ? first.source.anchorElementName : first.target.anchorElementName,
      targetAnchorName: canonicalFirstIsSource ? first.target.anchorElementName : first.source.anchorElementName,
      ownerViewIds,
      ownerViewNames,
      connectors: leaves,
    }
    proxyConnectorDetailsByKey[key] = details

    for (const endpoint of [first.source, first.target]) {
      if (!endpoint.externalToView) continue
      const nodeId = displayNodeId(endpoint)
      const endpointSortLevel = ancestorDepth(snapshot, endpoint.placementViewId ?? endpoint.anchorViewId ?? endpoint.commonAncestorViewId)
      const existingNode = proxyNodesMap.get(nodeId)
      if (existingNode) {
        const mergedOwners = new Map<number, string>()
        existingNode.ownerViewIds.forEach((ownerViewId, index) => {
          mergedOwners.set(ownerViewId, existingNode.ownerViewNames[index] ?? `View ${ownerViewId}`)
        })
        details.ownerViewIds.forEach((ownerViewId, index) => {
          mergedOwners.set(ownerViewId, details.ownerViewNames[index] ?? `View ${ownerViewId}`)
        })
        existingNode.ownerViewIds = Array.from(mergedOwners.keys())
        existingNode.ownerViewNames = Array.from(mergedOwners.values())
        existingNode.connectorCount += details.count
        existingNode.sortLevel = Math.min(existingNode.sortLevel, endpointSortLevel)
        continue
      }
      const anchorPlacement = elementDisplayPlacement(snapshot, endpoint.anchorElementId, endpoint.anchorViewId)
      proxyNodesMap.set(nodeId, {
        id: nodeId,
        anchorElementId: endpoint.anchorElementId,
        name: endpoint.anchorElementName,
        sortLevel: endpointSortLevel,
        placementViewId: endpoint.placementViewId ?? endpoint.anchorViewId ?? null,
        kind: anchorPlacement?.element.kind ?? null,
        description: anchorPlacement?.element.description ?? null,
        technology: anchorPlacement?.element.technology ?? null,
        logoUrl: anchorPlacement?.element.logo_url ?? null,
        technologyConnectors: anchorPlacement?.element.technology_connectors ?? [],
        ownerViewIds: [...details.ownerViewIds],
        ownerViewNames: [...details.ownerViewNames],
        commonAncestorViewId: endpoint.commonAncestorViewId,
        commonAncestorViewName: endpoint.commonAncestorViewName,
        currentBranchElementId: endpoint.currentBranchElementId,
        connectorCount: details.count,
      })
    }

    const sourceElement = first.source.externalToView ? null : currentVisibleElementsById.get(first.source.anchorElementId) ?? null
    const targetElement = first.target.externalToView ? null : currentVisibleElementsById.get(first.target.anchorElementId) ?? null

    proxyConnectors.push({
      key,
      sourceAnchorId: canonicalSourceAnchorId,
      targetAnchorId: canonicalTargetAnchorId,
      direction: 'merged',
      style: first.connector.style || 'bezier',
      label: details.label,
      count: details.count,
      sourceElementId: sourceElement?.element_id ?? null,
      targetElementId: targetElement?.element_id ?? null,
      details,
    })
  }

  return {
    proxyNodes: [...proxyNodesMap.values()],
    proxyConnectors,
    proxyConnectorDetailsByKey,
  }
}

export interface ZUIResolvedConnector {
  key: string
  sourceElementId: number
  targetElementId: number
  sourceAnchorElementId: number
  targetAnchorElementId: number
  sourceNodeId: string
  targetNodeId: string
  direction: string
  style: string
  label: string
  sourceDepth: number
  targetDepth: number
  maxDepth: number
  details: ProxyConnectorDetails
}

export interface ZUIHiddenProxyBadge {
  key: string
  sourceAnchorElementId: number
  targetAnchorElementId: number
  sourceNodeId: string
  targetNodeId: string
  count: number
  details: ProxyConnectorDetails
}

export interface ZUIProxyResolution {
  connectors: ZUIResolvedConnector[]
  hiddenBadges: ZUIHiddenProxyBadge[]
  omittedConnectorCount: number
}

export interface ZUIViewportBounds {
  minX: number
  minY: number
  maxX: number
  maxY: number
  centerX: number
  centerY: number
}

export interface ZUIConnectorAnchorInfo {
  nodeId: string
  worldX: number
  worldY: number
  worldW: number
  worldH: number
}

export interface ResolveZUIProxyConnectorOptions {
  viewport?: ZUIViewportBounds | null
  anchorsByElementId?: Map<number, ZUIConnectorAnchorInfo>
  connectorPriority?: CrossBranchConnectorPriority
}

function endpointPathForOwnerView(snapshot: WorkspaceGraphSnapshot, ownerViewId: number, elementId: number): number[] {
  const placement = chooseBestPlacement(snapshot, elementId, ownerViewId, ownerViewId)
  if (!placement) return [elementId]
  const owners = relativeOwnerElementPath(snapshot, snapshot.ancestorsByViewId[placement.viewId]?.[0] ?? placement.viewId, placement.viewId)
  const path = [...owners]
  if (path[path.length - 1] !== elementId) path.push(elementId)
  return path.length > 0 ? path : [elementId]
}

interface ZUIEndpointCandidate {
  actualElementId: number
  actualElementName: string
  anchorElementId: number
  anchorElementName: string
  anchorViewId: number | null
  anchorViewName: string | null
  placementViewId: number | null
  placementViewName: string | null
  depth: number
}

function visibleEndpointCandidates(
  snapshot: WorkspaceGraphSnapshot,
  ownerViewId: number,
  actualElementId: number,
  visibleElements: Set<number>,
): ZUIEndpointCandidate[] {
  const path = endpointPathForOwnerView(snapshot, ownerViewId, actualElementId)
  const visibleIndexes = path
    .map((elementId, index) => visibleElements.has(elementId) ? index : -1)
    .filter((index) => index >= 0)

  if (visibleIndexes.length === 0) return []

  const actualElementName = firstPlacementForElement(snapshot, actualElementId)?.element.name ?? `Element ${actualElementId}`
  const deepestVisibleIndex = visibleIndexes[visibleIndexes.length - 1]
  const candidateIndexes = [deepestVisibleIndex]
  if (visibleIndexes.length >= 2) candidateIndexes.push(visibleIndexes[visibleIndexes.length - 2])

  return candidateIndexes.map((pathIndex) => {
    const anchorElementId = path[pathIndex]
    const anchorPlacement = firstPlacementForElement(snapshot, anchorElementId)
    return {
      actualElementId,
      actualElementName,
      anchorElementId,
      anchorElementName: anchorPlacement?.element.name ?? `Element ${anchorElementId}`,
      anchorViewId: anchorPlacement?.viewId ?? ownerViewId,
      anchorViewName: anchorPlacement?.viewName ?? viewName(snapshot, ownerViewId),
      placementViewId: ownerViewId,
      placementViewName: viewName(snapshot, ownerViewId),
      depth: Math.max(0, path.length - 1 - pathIndex),
    }
  })
}

function isNativelyRenderedInZUI(
  connector: Connector,
  sourceAnchorElementId: number,
  targetAnchorElementId: number,
  visibleNodeIdsByElementId: Map<number, string>,
): boolean {
  return visibleNodeIdsByElementId.get(sourceAnchorElementId) === `d${connector.view_id}-o${sourceAnchorElementId}` &&
    visibleNodeIdsByElementId.get(targetAnchorElementId) === `d${connector.view_id}-o${targetAnchorElementId}`
}

function usesDeepestVisibleAnchor(
  snapshot: WorkspaceGraphSnapshot,
  ownerViewId: number,
  actualElementId: number,
  anchorElementId: number,
  visibleElements: Set<number>,
): boolean {
  return visibleEndpointCandidates(snapshot, ownerViewId, actualElementId, visibleElements)[0]?.anchorElementId === anchorElementId
}

function anchorCenter(anchor: ZUIConnectorAnchorInfo) {
  return {
    x: anchor.worldX + anchor.worldW / 2,
    y: anchor.worldY + anchor.worldH / 2,
  }
}

function anchorIsInViewport(anchor: ZUIConnectorAnchorInfo, viewport: ZUIViewportBounds): boolean {
  const center = anchorCenter(anchor)
  return center.x >= viewport.minX &&
    center.x <= viewport.maxX &&
    center.y >= viewport.minY &&
    center.y <= viewport.maxY
}

function normalizedDistanceToViewportCenter(anchor: ZUIConnectorAnchorInfo, viewport: ZUIViewportBounds): number {
  const center = anchorCenter(anchor)
  const dx = center.x - viewport.centerX
  const dy = center.y - viewport.centerY
  const diagonal = Math.max(1, Math.hypot(viewport.maxX - viewport.minX, viewport.maxY - viewport.minY))
  return Math.hypot(dx, dy) / diagonal
}

function viewportPriorityScore(
  connector: ZUIResolvedConnector,
  options: ResolveZUIProxyConnectorOptions | undefined,
): number {
  const viewport = options?.viewport
  const anchors = options?.anchorsByElementId
  const source = anchors?.get(connector.sourceAnchorElementId)
  const target = anchors?.get(connector.targetAnchorElementId)
  if (!viewport || !source || !target) {
    return connector.maxDepth * 100 + connector.sourceDepth + connector.targetDepth
  }

  const sourceDistance = normalizedDistanceToViewportCenter(source, viewport)
  const targetDistance = normalizedDistanceToViewportCenter(target, viewport)
  const nearDistance = Math.min(sourceDistance, targetDistance)
  const farDistance = Math.max(sourceDistance, targetDistance)
  const sourceInViewport = anchorIsInViewport(source, viewport)
  const targetInViewport = anchorIsInViewport(target, viewport)
  const inViewportCount = Number(sourceInViewport) + Number(targetInViewport)

  if (connector.details.connectors.length === 0) return Number.MAX_SAFE_INTEGER

  if (options?.connectorPriority === 'internal') {
    return (sourceDistance + targetDistance) * 1000 + farDistance * 400 - inViewportCount * 250
  }

  return nearDistance * 1000 - farDistance * 320 - (inViewportCount > 0 ? 300 : 0) + (inViewportCount === 2 ? 160 : 0)
}

function connectorTouchesViewport(
  connector: ZUIResolvedConnector,
  options: ResolveZUIProxyConnectorOptions | undefined,
): boolean {
  const viewport = options?.viewport
  const anchors = options?.anchorsByElementId
  if (!viewport || !anchors) return true
  const source = anchors.get(connector.sourceAnchorElementId)
  const target = anchors.get(connector.targetAnchorElementId)
  if (!source || !target) return false
  return anchorIsInViewport(source, viewport) || anchorIsInViewport(target, viewport)
}

export function resolveZUIProxyConnectors(
  snapshot: WorkspaceGraphSnapshot | null,
  visibleNodeIdsByElementId: Map<number, string>,
  settings: CrossBranchContextSettings,
  options?: ResolveZUIProxyConnectorOptions,
): ZUIProxyResolution {
  if (!snapshot || !settings.enabled || visibleNodeIdsByElementId.size === 0) {
    return { connectors: [], hiddenBadges: [], omittedConnectorCount: 0 }
  }

  const visibleElements = new Set(visibleNodeIdsByElementId.keys())
  const grouped = new Map<string, ProxyConnectorLeaf[]>()
  const nativeVisiblePairs = new Set<string>()

  for (const connector of allConnectors(snapshot)) {
    if (!visibleElements.has(connector.source_element_id) || !visibleElements.has(connector.target_element_id)) continue
    if (!isNativelyRenderedInZUI(connector, connector.source_element_id, connector.target_element_id, visibleNodeIdsByElementId)) continue
    const [leftAnchorElementId, rightAnchorElementId] = canonicalPairElements(connector.source_element_id, connector.target_element_id)
    nativeVisiblePairs.add([leftAnchorElementId, rightAnchorElementId].join('::'))
  }

  for (const connector of allConnectors(snapshot)) {
    const sourceCandidates = visibleEndpointCandidates(snapshot, connector.view_id, connector.source_element_id, visibleElements)
    const targetCandidates = visibleEndpointCandidates(snapshot, connector.view_id, connector.target_element_id, visibleElements)
    const seenPairsForConnector = new Set<string>()

    for (const sourceCandidate of sourceCandidates) {
      for (const targetCandidate of targetCandidates) {
        if (sourceCandidate.anchorElementId === targetCandidate.anchorElementId) continue
        if (
          sourceCandidate.actualElementId === sourceCandidate.anchorElementId &&
          targetCandidate.actualElementId === targetCandidate.anchorElementId &&
          isNativelyRenderedInZUI(
            connector,
            sourceCandidate.anchorElementId,
            targetCandidate.anchorElementId,
            visibleNodeIdsByElementId,
          )
        ) {
          continue
        }

        const sourceEndpoint: ProxyEndpoint = {
          actualElementId: sourceCandidate.actualElementId,
          actualElementName: sourceCandidate.actualElementName,
          anchorElementId: sourceCandidate.anchorElementId,
          anchorElementName: sourceCandidate.anchorElementName,
          anchorViewId: sourceCandidate.anchorViewId,
          anchorViewName: sourceCandidate.anchorViewName,
          placementViewId: sourceCandidate.placementViewId,
          placementViewName: sourceCandidate.placementViewName,
          depth: sourceCandidate.depth,
          externalToView: sourceCandidate.anchorElementId !== sourceCandidate.actualElementId,
          currentBranchElementId: null,
          commonAncestorViewId: null,
          commonAncestorViewName: null,
        }
        const targetEndpoint: ProxyEndpoint = {
          actualElementId: targetCandidate.actualElementId,
          actualElementName: targetCandidate.actualElementName,
          anchorElementId: targetCandidate.anchorElementId,
          anchorElementName: targetCandidate.anchorElementName,
          anchorViewId: targetCandidate.anchorViewId,
          anchorViewName: targetCandidate.anchorViewName,
          placementViewId: targetCandidate.placementViewId,
          placementViewName: targetCandidate.placementViewName,
          depth: targetCandidate.depth,
          externalToView: targetCandidate.anchorElementId !== targetCandidate.actualElementId,
          currentBranchElementId: null,
          commonAncestorViewId: null,
          commonAncestorViewName: null,
        }

        const leaf: ProxyConnectorLeaf = {
          connector,
          ownerViewId: connector.view_id,
          ownerViewName: viewName(snapshot, connector.view_id) ?? `View ${connector.view_id}`,
          source: sourceEndpoint,
          target: targetEndpoint,
        }

        const [leftAnchorElementId, rightAnchorElementId] = canonicalPairElements(
          sourceCandidate.anchorElementId,
          targetCandidate.anchorElementId,
        )
        const key = [leftAnchorElementId, rightAnchorElementId].join('::')
        const pairKey = `${connector.id}:${key}`
        if (seenPairsForConnector.has(pairKey)) continue
        seenPairsForConnector.add(pairKey)
        const existing = grouped.get(key)
        if (existing) existing.push(leaf)
        else grouped.set(key, [leaf])
      }
    }
  }

  const resolved: ZUIResolvedConnector[] = []
  const hiddenBadges: ZUIHiddenProxyBadge[] = []
  for (const [key, leaves] of grouped) {
    const [first] = leaves
    const { ownerViewIds, ownerViewNames } = ownerViewsFromLeaves(leaves)
    const [canonicalSourceAnchorElementId, canonicalTargetAnchorElementId] = canonicalPairElements(
      first.source.anchorElementId,
      first.target.anchorElementId,
    )
    const canonicalFirstIsSource = canonicalSourceAnchorElementId === first.source.anchorElementId
    const canonicalSourceDepth = canonicalFirstIsSource ? first.source.depth : first.target.depth
    const canonicalTargetDepth = canonicalFirstIsSource ? first.target.depth : first.source.depth
    const details: ProxyConnectorDetails = {
      key,
      label: proxyDisplayLabel(leaves),
      count: leaves.length,
      sourceAnchorId: visibleNodeKey(canonicalSourceAnchorElementId),
      targetAnchorId: visibleNodeKey(canonicalTargetAnchorElementId),
      sourceAnchorName: canonicalFirstIsSource ? first.source.anchorElementName : first.target.anchorElementName,
      targetAnchorName: canonicalFirstIsSource ? first.target.anchorElementName : first.source.anchorElementName,
      ownerViewIds,
      ownerViewNames,
      connectors: leaves,
    }

    const isDirectChildBadgeOnly = leaves.every((leaf) => {
      if (Math.max(leaf.source.depth, leaf.target.depth) !== 1) return false
      const sourceOk = leaf.source.actualElementId === leaf.source.anchorElementId ||
        usesDeepestVisibleAnchor(snapshot, leaf.ownerViewId, leaf.source.actualElementId, leaf.source.anchorElementId, visibleElements)
      const targetOk = leaf.target.actualElementId === leaf.target.anchorElementId ||
        usesDeepestVisibleAnchor(snapshot, leaf.ownerViewId, leaf.target.actualElementId, leaf.target.anchorElementId, visibleElements)
      return sourceOk && targetOk
    })
    const pairHasNativeDirect = nativeVisiblePairs.has(key)
    if (pairHasNativeDirect) {
      if (isDirectChildBadgeOnly) {
        hiddenBadges.push({
          key: `badge:${key}`,
          sourceAnchorElementId: canonicalSourceAnchorElementId,
          targetAnchorElementId: canonicalTargetAnchorElementId,
          sourceNodeId: visibleNodeIdsByElementId.get(canonicalSourceAnchorElementId) ?? '',
          targetNodeId: visibleNodeIdsByElementId.get(canonicalTargetAnchorElementId) ?? '',
          count: details.count,
          details,
        })
      }
      continue
    }

    resolved.push({
      key,
      sourceElementId: canonicalFirstIsSource ? first.source.actualElementId : first.target.actualElementId,
      targetElementId: canonicalFirstIsSource ? first.target.actualElementId : first.source.actualElementId,
      sourceAnchorElementId: canonicalSourceAnchorElementId,
      targetAnchorElementId: canonicalTargetAnchorElementId,
      sourceNodeId: visibleNodeIdsByElementId.get(canonicalSourceAnchorElementId) ?? '',
      targetNodeId: visibleNodeIdsByElementId.get(canonicalTargetAnchorElementId) ?? '',
      direction: 'merged',
      style: first.connector.style || 'bezier',
      label: details.label,
      sourceDepth: canonicalSourceDepth,
      targetDepth: canonicalTargetDepth,
      maxDepth: Math.max(canonicalSourceDepth, canonicalTargetDepth),
      details,
    })
  }

  const visibleResolved = resolved
    .filter((connector) => connector.sourceNodeId && connector.targetNodeId)
    .filter((connector) => connectorTouchesViewport(connector, options))
    .sort((left, right) => {
      const scoreDelta = viewportPriorityScore(left, {
        ...options,
        connectorPriority: settings.connectorPriority,
      }) - viewportPriorityScore(right, {
        ...options,
        connectorPriority: settings.connectorPriority,
      })
      if (scoreDelta !== 0) return scoreDelta
      if (right.details.count !== left.details.count) return right.details.count - left.details.count
      if (left.maxDepth !== right.maxDepth) return left.maxDepth - right.maxDepth
      const depthDelta = (left.sourceDepth + left.targetDepth) - (right.sourceDepth + right.targetDepth)
      if (depthDelta !== 0) return depthDelta
      return left.key.localeCompare(right.key)
    })
  const maxGroups = settings.connectorBudget ?? settings.maxProxyConnectorGroups ?? CROSS_BRANCH_CONNECTOR_BUDGET_DEFAULT
  const budgetedResolved = maxGroups > 0 ? visibleResolved.slice(0, maxGroups) : visibleResolved
  const omittedConnectorIds = new Set<number>()
  if (maxGroups > 0) {
    for (const connector of visibleResolved.slice(maxGroups)) {
      for (const leaf of connector.details.connectors) {
        omittedConnectorIds.add(leaf.connector.id)
      }
    }
  }
  const omittedConnectorCount = omittedConnectorIds.size

  return {
    connectors: budgetedResolved,
    hiddenBadges: hiddenBadges.filter((badge) => badge.sourceNodeId && badge.targetNodeId),
    omittedConnectorCount,
  }
}
