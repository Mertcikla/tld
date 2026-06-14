import type { CrossBranchContextSettings, ProxyEndpoint } from '../../../crossBranch/types'

export interface ContextSummaryLeaf {
  id: string
  endpoint: ProxyEndpoint
  pathElementIds: number[]
}

export interface ContextSummaryNode {
  id: string
  elementId: number
  pathElementIds: number[]
  parentId: string | null
  childIds: string[]
  directLeafIds: string[]
  descendantLeafIds: string[]
  depth: number
}

export interface ContextSummaryForest {
  nodesById: Record<string, ContextSummaryNode>
  rootIds: string[]
  leavesById: Record<string, ContextSummaryLeaf>
}

export interface VisibleContextSummaryNode extends ContextSummaryNode {
  isExpanded: boolean
  isAutoExpanded: boolean
  visibleChildIds: string[]
  totalLeafCount: number
  hiddenLeafCount: number
}

export interface VisibleContextSummaryForest {
  nodesById: Record<string, VisibleContextSummaryNode>
  rootIds: string[]
  leafToVisibleNodeId: Record<string, string>
}

function contextSummaryScopeKey(endpoint: ProxyEndpoint) {
  return [
    endpoint.commonAncestorViewId ?? 'none',
    endpoint.currentBranchElementId ?? 'none',
  ].join(':')
}

function contextSummaryNodeId(endpoint: ProxyEndpoint, pathElementIds: number[]) {
  return ['ctx', contextSummaryScopeKey(endpoint), ...pathElementIds].join(':')
}

export function contextSummaryLeafId(endpoint: ProxyEndpoint) {
  return [
    'leaf',
    contextSummaryScopeKey(endpoint),
    endpoint.placementViewId ?? endpoint.anchorViewId ?? 'none',
    endpoint.actualElementId,
  ].join(':')
}

export function contextSummaryBranchPath(endpoint: ProxyEndpoint): number[] {
  if ((endpoint.branchPathElementIds?.length ?? 0) > 0) return endpoint.branchPathElementIds ?? []
  if ((endpoint.contextPathElementIds?.length ?? 0) > 0) return endpoint.contextPathElementIds ?? []
  if (endpoint.externalToView) return [endpoint.anchorElementId]
  return []
}

export function buildContextSummaryForest(endpoints: ProxyEndpoint[]): ContextSummaryForest {
  const rootIds: string[] = []
  const rootIdSet = new Set<string>()
  const leavesById: Record<string, ContextSummaryLeaf> = {}
  const childIdSets = new Map<string, Set<string>>()
  const directLeafIdSets = new Map<string, Set<string>>()
  const descendantLeafIdSets = new Map<string, Set<string>>()
  const nodeDrafts = new Map<string, Omit<ContextSummaryNode, 'childIds' | 'directLeafIds' | 'descendantLeafIds'>>()

  for (const endpoint of endpoints) {
    if (!endpoint.externalToView) continue

    const pathElementIds = contextSummaryBranchPath(endpoint)
    if (pathElementIds.length === 0) continue

    const leafId = contextSummaryLeafId(endpoint)
    leavesById[leafId] = { id: leafId, endpoint, pathElementIds }

    let parentId: string | null = null
    for (let index = 0; index < pathElementIds.length; index += 1) {
      const prefix = pathElementIds.slice(0, index + 1)
      const nodeId = contextSummaryNodeId(endpoint, prefix)
      const existingNode = nodeDrafts.get(nodeId)
      if (!existingNode) {
        nodeDrafts.set(nodeId, {
          id: nodeId,
          elementId: prefix[prefix.length - 1],
          pathElementIds: prefix,
          parentId,
          depth: index,
        })
      }

      if (parentId == null) {
        if (!rootIdSet.has(nodeId)) {
          rootIds.push(nodeId)
          rootIdSet.add(nodeId)
        }
      } else {
        const childIds = childIdSets.get(parentId) ?? new Set<string>()
        childIds.add(nodeId)
        childIdSets.set(parentId, childIds)
      }

      const descendantLeafIds = descendantLeafIdSets.get(nodeId) ?? new Set<string>()
      descendantLeafIds.add(leafId)
      descendantLeafIdSets.set(nodeId, descendantLeafIds)

      if (index === pathElementIds.length - 1) {
        const directLeafIds = directLeafIdSets.get(nodeId) ?? new Set<string>()
        directLeafIds.add(leafId)
        directLeafIdSets.set(nodeId, directLeafIds)
      }

      parentId = nodeId
    }
  }

  const nodesById = Object.fromEntries(Array.from(nodeDrafts.entries()).map(([nodeId, draft]) => [nodeId, {
    ...draft,
    childIds: Array.from(childIdSets.get(nodeId) ?? []),
    directLeafIds: Array.from(directLeafIdSets.get(nodeId) ?? []),
    descendantLeafIds: Array.from(descendantLeafIdSets.get(nodeId) ?? []),
  } satisfies ContextSummaryNode]))

  return {
    nodesById,
    rootIds,
    leavesById,
  }
}

export function autoExpandLeafBudget(settings: CrossBranchContextSettings): number {
  const connectorBudget = settings.connectorBudget ?? 50
  if (connectorBudget <= 50) return 1
  if (connectorBudget <= 100) return 2
  if (connectorBudget <= 150) return 3
  return 4
}

export function buildVisibleContextSummaryForest(
  forest: ContextSummaryForest,
  expandedNodeIds: Set<string>,
  settings: CrossBranchContextSettings,
): VisibleContextSummaryForest {
  const nodesById: Record<string, VisibleContextSummaryNode> = {}
  const leafToVisibleNodeId: Record<string, string> = {}
  const rootIds: string[] = []
  const quietLeafBudget = autoExpandLeafBudget(settings)

  const mapLeavesToNode = (leafIds: string[], nodeId: string) => {
    leafIds.forEach((leafId) => {
      leafToVisibleNodeId[leafId] = nodeId
    })
  }

  const visit = (nodeId: string, allowAutoExpand: boolean) => {
    const node = forest.nodesById[nodeId]
    if (!node) return

    const isAutoExpanded = allowAutoExpand &&
      node.childIds.length > 0 &&
      node.descendantLeafIds.length > 1 &&
      node.descendantLeafIds.length <= quietLeafBudget
    const isExpanded = expandedNodeIds.has(nodeId)
    const shouldShowChildren = isAutoExpanded || isExpanded

    const visibleNode: VisibleContextSummaryNode = {
      ...node,
      isExpanded,
      isAutoExpanded,
      visibleChildIds: [],
      totalLeafCount: node.descendantLeafIds.length,
      hiddenLeafCount: Math.max(0, node.descendantLeafIds.length - node.directLeafIds.length),
    }
    nodesById[nodeId] = visibleNode

    if (!shouldShowChildren) {
      mapLeavesToNode(node.descendantLeafIds, nodeId)
      return
    }

    mapLeavesToNode(node.directLeafIds, nodeId)
    node.childIds.forEach((childId) => {
      visibleNode.visibleChildIds.push(childId)
      visit(childId, isAutoExpanded)
    })
  }

  forest.rootIds.forEach((rootId) => {
    rootIds.push(rootId)
    const root = forest.nodesById[rootId]
    const allowAutoExpand = (root?.descendantLeafIds.length ?? 0) <= quietLeafBudget
    visit(rootId, allowAutoExpand)
  })

  return {
    nodesById,
    rootIds,
    leafToVisibleNodeId,
  }
}
