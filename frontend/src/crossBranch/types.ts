import type { Connector, ExploreData, PlacedElement, ViewTreeNode } from '../types'

export const CROSS_BRANCH_DEPTH_ALL = 5
export const CROSS_BRANCH_DEPTH_MIN = 1
export const CROSS_BRANCH_DEPTH_MAX = CROSS_BRANCH_DEPTH_ALL
export const CROSS_BRANCH_CONNECTOR_BUDGET_MIN = 10
export const CROSS_BRANCH_CONNECTOR_BUDGET_MAX = 200
export const CROSS_BRANCH_CONNECTOR_BUDGET_DEFAULT = 50

export type CrossBranchConnectorPriority = 'external' | 'internal'

export type CrossBranchSurface = 'editor' | 'zui' | 'zui-shared'

export interface CrossBranchContextSettings {
  enabled: boolean
  depth: number
  connectorBudget: number
  connectorPriority: CrossBranchConnectorPriority
  minConnectorAnchorAlpha?: number
  maxProxyConnectorGroups?: number
}

export interface GraphPlacementRef {
  viewId: number
  viewName: string
  element: PlacedElement
}

export interface WorkspaceGraphSnapshot {
  source: ExploreData
  tree: ViewTreeNode[]
  views: Record<number, { view: ViewTreeNode; placements: PlacedElement[]; connectors: Connector[] }>
  viewById: Record<number, ViewTreeNode>
  placementsByViewId: Record<number, PlacedElement[]>
  connectorsByViewId: Record<number, Connector[]>
  placementsByElementId: Record<number, GraphPlacementRef[]>
  childViewIdByOwnerElementId: Record<number, number>
  descendantsByViewId: Record<number, number[]>
  ancestorsByViewId: Record<number, number[]>
}

export interface ProxyEndpoint {
  actualElementId: number
  actualElementName: string
  anchorElementId: number
  anchorElementName: string
  anchorViewId: number | null
  anchorViewName: string | null
  placementViewId: number | null
  placementViewName: string | null
  depth: number
  externalToView: boolean
  currentBranchElementId: number | null
  commonAncestorViewId: number | null
  commonAncestorViewName: string | null
  mergeAncestorElementId?: number | null
  contextPathElementIds?: number[]
}

export interface ProxyConnectorLeaf {
  connector: Connector
  ownerViewId: number
  ownerViewName: string
  source: ProxyEndpoint
  target: ProxyEndpoint
}

export interface ProxyConnectorDetails {
  key: string
  label: string
  count: number
  sourceAnchorId: string
  targetAnchorId: string
  sourceAnchorName: string
  targetAnchorName: string
  ownerViewIds: number[]
  ownerViewNames: string[]
  connectors: ProxyConnectorLeaf[]
}

export interface AggregatedProxyConnector {
  key: string
  sourceAnchorId: string
  targetAnchorId: string
  direction: string
  style: string
  label: string
  count: number
  sourceElementId: number | null
  targetElementId: number | null
  details: ProxyConnectorDetails
}

export interface ProxyContextNode {
  id: string
  anchorElementId: number
  name: string
  sortLevel: number
  placementViewId: number | null
  kind: string | null
  description: string | null
  technology: string | null
  logoUrl: string | null
  technologyConnectors: PlacedElement['technology_connectors']
  ownerViewIds: number[]
  ownerViewNames: string[]
  commonAncestorViewId: number | null
  commonAncestorViewName: string | null
  currentBranchElementId: number | null
  connectorCount: number
}
