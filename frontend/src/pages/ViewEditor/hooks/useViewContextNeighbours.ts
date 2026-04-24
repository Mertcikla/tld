import { useMemo } from 'react'
import { type Edge as RFEdge, type Node as RFNode } from 'reactflow'
import type { PlacedElement } from '../../../types'
import type { CrossBranchContextSettings, ProxyConnectorDetails, WorkspaceGraphSnapshot } from '../../../crossBranch/types'
import { resolveViewProxyGraph } from '../../../crossBranch/resolve'

interface Props {
  snapshot: WorkspaceGraphSnapshot | null
  settings: CrossBranchContextSettings
  viewId: number | null
  viewElements: PlacedElement[]
  rfNodes: RFNode[]
  stableOnNavigateToView: (id: number) => void
  onSelectProxyDetails: (details: ProxyConnectorDetails) => void
  expandedAncestorGroups: Set<string>
  onToggleAncestorGroup: (anchorId: string) => void
}

type ContextSide = 'top' | 'bottom' | 'left' | 'right'

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

function isAncestorContextNode(
  snapshot: WorkspaceGraphSnapshot,
  ancestor: { anchorElementId: number },
  descendant: { placementViewId: number | null },
): boolean {
  const ownedViewId = snapshot.childViewIdByOwnerElementId[ancestor.anchorElementId]
  if (ownedViewId == null || descendant.placementViewId == null) return false
  return descendant.placementViewId === ownedViewId ||
    (snapshot.descendantsByViewId[ownedViewId]?.includes(descendant.placementViewId) ?? false)
}

export function useViewContextNeighbours({
  snapshot,
  settings,
  viewId,
  viewElements,
  rfNodes,
  stableOnNavigateToView,
  onSelectProxyDetails,
  expandedAncestorGroups,
  onToggleAncestorGroup,
}: Props) {
  return useMemo(() => {
    if (!snapshot || viewId == null || !settings.enabled) {
      return { contextNodes: [] as RFNode[], contextConnectors: [] as RFEdge[], proxyConnectorDetailsByKey: {} as Record<string, ProxyConnectorDetails> }
    }

    const { proxyNodes, proxyConnectors, proxyConnectorDetailsByKey } = resolveViewProxyGraph(snapshot, viewId, viewElements, settings)
    if (proxyNodes.length === 0 && proxyConnectors.length === 0) {
      return { contextNodes: [] as RFNode[], contextConnectors: [] as RFEdge[], proxyConnectorDetailsByKey }
    }

    const mainNodes = rfNodes.filter((node) => node.type === 'elementNode')
    if (mainNodes.length === 0) {
      return { contextNodes: [] as RFNode[], contextConnectors: [] as RFEdge[], proxyConnectorDetailsByKey }
    }

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

    const livePositions = new Map(rfNodes.map((node) => [node.id, node.position] as const))
    const currentViewPositions = new Map(viewElements.map((element) => {
      const live = livePositions.get(String(element.element_id))
      return [
        element.element_id,
        live ? { ...element, position_x: live.x, position_y: live.y } : element,
      ] as const
    }))
    const proxyNodeDetailsById = new Map(
      proxyNodes.map((proxyNode) => {
        const connectors = proxyConnectors
          .filter((connector) => connector.sourceAnchorId === proxyNode.id || connector.targetAnchorId === proxyNode.id)
          .flatMap((connector) => connector.details.connectors)

        const ownerViews = new Map<number, string>()
        connectors.forEach((leaf) => ownerViews.set(leaf.ownerViewId, leaf.ownerViewName))

        const details: ProxyConnectorDetails = {
          key: `node:${proxyNode.id}`,
          label: 'Merged branch context',
          count: connectors.length,
          sourceAnchorId: proxyNode.id,
          targetAnchorId: proxyNode.id,
          sourceAnchorName: proxyNode.name,
          targetAnchorName: 'Multiple connections',
          ownerViewIds: Array.from(ownerViews.keys()),
          ownerViewNames: Array.from(ownerViews.values()),
          connectors,
        }

        return [proxyNode.id, details] as const
      }),
    )

    const ContextBoundaryElement: RFNode = {
      id: 'context-boundary',
      type: 'ContextBoundaryElement',
      position: { x: minX - totalInset, y: minY - totalInset },
      selectable: false,
      draggable: false,
      connectable: false,
      zIndex: 1,
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

    const provisionalContextLayouts = proxyNodes.map((contextNode) => {
      const relatedAngles = proxyConnectors
        .filter((connector) => connector.sourceAnchorId === contextNode.id || connector.targetAnchorId === contextNode.id)
        .flatMap((connector) => {
          const leafAngles = connector.details.connectors.map((leaf) => {
            const external = leaf.source.externalToView ? leaf.source : leaf.target
            if (external.anchorElementId !== contextNode.anchorElementId) return null
            const internal = leaf.source.externalToView ? leaf.target : leaf.source

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
          })

          return leafAngles.filter((angle): angle is number => angle != null)
        })

      let angle = relatedAngles.length > 0 ? averageAngles(relatedAngles) : stableAngleFromId(contextNode.id)
      const isDescendant = contextNode.placementViewId != null && (snapshot.descendantsByViewId[viewId]?.includes(contextNode.placementViewId) ?? false)

      if (isDescendant) {
        angle = -Math.PI / 2
      }

      const side = isDescendant ? 'top' : classifySide(angle)

      const centerPosX = centerX + Math.cos(angle) * radiusX
      const centerPosY = centerY + Math.sin(angle) * radiusY

      return {
        contextNode,
        angle,
        side,
        centerX: centerPosX,
        centerY: centerPosY,
        position: {
          x: centerPosX - CONTEXT_NODE_HALF_W,
          y: centerPosY - CONTEXT_NODE_HALF_H,
        },
      }
    })

    const layoutsBySide: Record<ContextSide, typeof provisionalContextLayouts> = {
      top: [],
      bottom: [],
      left: [],
      right: [],
    }

    provisionalContextLayouts.forEach((layout) => {
      layoutsBySide[layout.side].push(layout)
    })

    let clusterCounter = 0
    const stackedLayouts = (['top', 'bottom', 'left', 'right'] as const).flatMap((side) => {
      const sideLayouts = [...layoutsBySide[side]].sort((left, right) => {
        const leftCoord = side === 'left' || side === 'right' ? left.centerY : left.centerX
        const rightCoord = side === 'left' || side === 'right' ? right.centerY : right.centerX
        return leftCoord - rightCoord
      })
      if (sideLayouts.length === 0) return []

      const threshold = side === 'left' || side === 'right' ? SIDE_CLUSTER_THRESHOLD : TOP_BOTTOM_CLUSTER_THRESHOLD
      const clusters: typeof sideLayouts[] = []

      for (const layout of sideLayouts) {
        const coord = side === 'left' || side === 'right' ? layout.centerY : layout.centerX
        const cluster = clusters[clusters.length - 1]
        if (!cluster) {
          clusters.push([layout])
          continue
        }
        const last = cluster[cluster.length - 1]
        const lastCoord = side === 'left' || side === 'right' ? last.centerY : last.centerX
        if (Math.abs(coord - lastCoord) <= threshold) cluster.push(layout)
        else clusters.push([layout])
      }

      return clusters.flatMap((cluster) => {
        const clusterId = `${side}-${clusterCounter++}`
        const ordered = [...cluster].sort((left, right) => {
          if (left.contextNode.sortLevel !== right.contextNode.sortLevel) {
            return left.contextNode.sortLevel - right.contextNode.sortLevel
          }
          return left.contextNode.name.localeCompare(right.contextNode.name)
        })

        const anchorX = cluster.reduce((sum, layout) => sum + layout.centerX, 0) / cluster.length
        const anchorY = cluster.reduce((sum, layout) => sum + layout.centerY, 0) / cluster.length
        const clusterExpanded = ordered.length > 1 && expandedAncestorGroups.has(ordered[0].contextNode.id)
        // A cluster is collapsible only when the lowest-sortLevel node genuinely owns a view
        // that contains at least one other node's placement view. Siblings that merely happen
        // to be close in angle but are NOT in an ancestor-descendant view relationship
        // are laid out along the boundary edge instead of stacking away from it.
        const hasAncestorDescendant = ordered.length > 1 &&
          ordered.slice(1).some((layout) => isAncestorContextNode(snapshot, ordered[0].contextNode, layout.contextNode))

        if (side === 'top') {
          if (!hasAncestorDescendant) {
            const startX = anchorX - ((ordered.length - 1) * HORIZONTAL_STACK_SPACING) / 2
            return ordered.map((layout, index) => ({
              ...layout,
              clusterId,
              position: {
                x: startX + index * HORIZONTAL_STACK_SPACING - CONTEXT_NODE_HALF_W,
                y: boundaryTop - VERTICAL_BOUNDARY_CLEARANCE - CONTEXT_NODE_HALF_H,
              },
            }))
          }
          const startY = boundaryTop - VERTICAL_BOUNDARY_CLEARANCE - (ordered.length - 1) * VERTICAL_STACK_SPACING
          return ordered.map((layout, index) => ({
            ...layout,
            clusterId,
            position: {
              x: anchorX - CONTEXT_NODE_HALF_W,
              y: startY + index * VERTICAL_STACK_SPACING - CONTEXT_NODE_HALF_H,
            },
          }))
        }

        if (side === 'bottom') {
          if (!hasAncestorDescendant) {
            const startX = anchorX - ((ordered.length - 1) * HORIZONTAL_STACK_SPACING) / 2
            return ordered.map((layout, index) => ({
              ...layout,
              clusterId,
              position: {
                x: startX + index * HORIZONTAL_STACK_SPACING - CONTEXT_NODE_HALF_W,
                y: boundaryBottom + VERTICAL_BOUNDARY_CLEARANCE - CONTEXT_NODE_HALF_H,
              },
            }))
          }
          const startY = boundaryBottom + VERTICAL_BOUNDARY_CLEARANCE
          return ordered.map((layout, index) => ({
            ...layout,
            clusterId,
            position: {
              x: anchorX - CONTEXT_NODE_HALF_W,
              y: startY + index * VERTICAL_STACK_SPACING + (index > 0 && clusterExpanded ? CHEVRON_V_CLEARANCE : 0) - CONTEXT_NODE_HALF_H,
            },
          }))
        }

        if (side === 'left') {
          if (!hasAncestorDescendant) {
            const startY = anchorY - ((ordered.length - 1) * VERTICAL_STACK_SPACING) / 2
            return ordered.map((layout, index) => ({
              ...layout,
              clusterId,
              position: {
                x: boundaryLeft - HORIZONTAL_BOUNDARY_CLEARANCE - CONTEXT_NODE_HALF_W,
                y: startY + index * VERTICAL_STACK_SPACING - CONTEXT_NODE_HALF_H,
              },
            }))
          }
          const startX = boundaryLeft - HORIZONTAL_BOUNDARY_CLEARANCE - (ordered.length - 1) * HORIZONTAL_STACK_SPACING
          return ordered.map((layout, index) => ({
            ...layout,
            clusterId,
            position: {
              x: startX + index * HORIZONTAL_STACK_SPACING - CONTEXT_NODE_HALF_W,
              y: anchorY - CONTEXT_NODE_HALF_H,
            },
          }))
        }

        // right side
        if (!hasAncestorDescendant) {
          const startY = anchorY - ((ordered.length - 1) * VERTICAL_STACK_SPACING) / 2
          return ordered.map((layout, index) => ({
            ...layout,
            clusterId,
            position: {
              x: boundaryRight + HORIZONTAL_BOUNDARY_CLEARANCE - CONTEXT_NODE_HALF_W,
              y: startY + index * VERTICAL_STACK_SPACING - CONTEXT_NODE_HALF_H,
            },
          }))
        }
        const startX = boundaryRight + HORIZONTAL_BOUNDARY_CLEARANCE
        return ordered.map((layout, index) => ({
          ...layout,
          clusterId,
          position: {
            x: startX + index * HORIZONTAL_STACK_SPACING + (index > 0 && clusterExpanded ? CHEVRON_H_CLEARANCE : 0) - CONTEXT_NODE_HALF_W,
            y: anchorY - CONTEXT_NODE_HALF_H,
          },
        }))
      })
    })

    // Group nodes that the layout already places in the same spatial cluster.
    // Within each cluster, the node with the lowest sortLevel (closest ancestor) is the anchor;
    // all others are children that collapse behind it.
    const clusterGroups = new Map<string, typeof stackedLayouts>()
    for (const layout of stackedLayouts) {
      const group = clusterGroups.get(layout.clusterId) ?? []
      group.push(layout)
      clusterGroups.set(layout.clusterId, group)
    }

    const childToAnchorId = new Map<string, string>()
    const anchorGroupChildCount = new Map<string, number>()

    for (const [, clusterLayouts] of clusterGroups) {
      if (clusterLayouts.length < 2) continue
      const [anchor, ...rest] = clusterLayouts
      // Only collapse nodes whose placement view is actually inside the view tree rooted
      // at the anchor element's child view. Siblings that merely cluster spatially are not collapsed.
      const children = rest.filter((layout) =>
        isAncestorContextNode(snapshot, anchor.contextNode, layout.contextNode),
      )
      if (children.length === 0) continue
      anchorGroupChildCount.set(anchor.contextNode.id, children.length)
      for (const child of children) {
        childToAnchorId.set(child.contextNode.id, anchor.contextNode.id)
      }
    }

    const contextNodes: RFNode[] = stackedLayouts
      .filter(({ contextNode }) => {
        const anchorId = childToAnchorId.get(contextNode.id)
        if (anchorId == null) return true
        return expandedAncestorGroups.has(anchorId)
      })
      .map(({ contextNode, position, side }) => {
        const isGroupAnchor = anchorGroupChildCount.has(contextNode.id)
        const groupChildCount = anchorGroupChildCount.get(contextNode.id) ?? 0
        const isGroupExpanded = expandedAncestorGroups.has(contextNode.id)
        return {
          id: contextNode.id,
          type: 'contextNeighborNode',
          position,
          selectable: false,
          draggable: false,
          connectable: false,
          zIndex: isGroupAnchor && isGroupExpanded ? 8 : 6,
          data: {
            element_id: contextNode.anchorElementId,
            name: contextNode.name,
            kind: contextNode.kind,
            description: contextNode.description,
            technology: contextNode.technology,
            logo_url: contextNode.logoUrl,
            technology_connectors: contextNode.technologyConnectors,
            ownerViewIds: contextNode.ownerViewIds,
            ownerViewNames: contextNode.ownerViewNames,
            commonAncestorViewId: contextNode.commonAncestorViewId,
            commonAncestorViewName: contextNode.commonAncestorViewName,
            connectorCount: contextNode.connectorCount,
            onNavigateToView: stableOnNavigateToView,
            onSelectDetails: proxyNodeDetailsById.get(contextNode.id)
              ? () => onSelectProxyDetails(proxyNodeDetailsById.get(contextNode.id) as ProxyConnectorDetails)
              : undefined,
            isGroupAnchor,
            groupChildCount,
            isGroupExpanded,
            onToggleGroup: isGroupAnchor ? () => onToggleAncestorGroup(contextNode.id) : undefined,
            side,
          },
        }
      })

    const seenCollapsedPairs = new Set<string>()
    const contextConnectors: RFEdge[] = proxyConnectors.flatMap((connector) => {
      let sourceId = connector.sourceAnchorId
      let targetId = connector.targetAnchorId

      const sourceAnchor = childToAnchorId.get(sourceId)
      if (sourceAnchor != null && !expandedAncestorGroups.has(sourceAnchor)) sourceId = sourceAnchor

      const targetAnchor = childToAnchorId.get(targetId)
      if (targetAnchor != null && !expandedAncestorGroups.has(targetAnchor)) targetId = targetAnchor

      if (sourceId === targetId) return []

      const pairKey = `${sourceId}::${targetId}`
      if (seenCollapsedPairs.has(pairKey)) return []
      seenCollapsedPairs.add(pairKey)

      return [{
        id: `proxy:${connector.key}`,
        source: sourceId,
        target: targetId,
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

    return { contextNodes: [ContextBoundaryElement, ...contextNodes], contextConnectors, proxyConnectorDetailsByKey }
  }, [snapshot, settings, viewId, viewElements, rfNodes, stableOnNavigateToView, onSelectProxyDetails, expandedAncestorGroups, onToggleAncestorGroup])
}
