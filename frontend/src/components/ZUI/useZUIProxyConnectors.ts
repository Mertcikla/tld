import { useCallback, useMemo, useRef } from 'react'
import type { CrossBranchContextSettings } from '../../crossBranch/types'
import { DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA } from '../../crossBranch/settings'
import type { DiagramGroupLayout, HoveredItem, ZUIViewState } from './types'
import {
  buildProxyConnectorRenderState,
  buildProxyConnectorSpatialIndex,
  buildVisibleProxyConnectors,
  collectVisibleNodeAnchors,
  findHoveredProxyConnector,
  type ProxyConnectorSpatialIndex,
  type ZUIProxyConnectorRenderState,
  type VisibleNodeAnchor,
} from './proxy'
import type { WorkspaceGraphSnapshot } from '../../crossBranch/types'

export interface ZUIProxyConnectorState {
  byNodeId: Map<string, VisibleNodeAnchor>
  visibleAnchors: Map<number, VisibleNodeAnchor>
  proxyConnectors: ReturnType<typeof buildVisibleProxyConnectors>['connectors']
  proxyConnectorRenderState: ZUIProxyConnectorRenderState
  hiddenProxyBadges: ReturnType<typeof buildVisibleProxyConnectors>['hiddenBadges']
  resolveHoveredProxyItem: (worldX: number, worldY: number, view: ZUIViewState, canvasW: number) => HoveredItem | null
}

function anchorViewForZoom(zoom: number): ZUIViewState {
  return { x: 0, y: 0, zoom: Math.max(0.0001, zoom) }
}

export function useZUIProxyConnectors(
  groups: DiagramGroupLayout[],
  workspaceSnapshot: WorkspaceGraphSnapshot,
  viewState: ZUIViewState,
  canvasW: number,
  viewportBounds: { minX: number; minY: number; maxX: number; maxY: number; centerX: number; centerY: number },
  crossBranchSettings: CrossBranchContextSettings,
  hiddenTags: string[] | undefined,
): ZUIProxyConnectorState {
  // Anchor positions depend on zoom expansion thresholds, but not panning.
  // Keeping camera translation out of this memo prevents tree walks during drag frames.
  const anchors = useMemo(() =>
    collectVisibleNodeAnchors(groups, anchorViewForZoom(viewState.zoom), canvasW || 1, hiddenTags),
    [groups, viewState.zoom, canvasW, hiddenTags],
  )

  const visibleElementSig = useMemo(() =>
    Array.from(anchors.visibleAnchors.entries())
      .sort(([a], [b]) => a - b)
      .map(([id, anchor]) => `${id}:${anchor.nodeId}:${anchor.renderAlpha >= (crossBranchSettings.minConnectorAnchorAlpha ?? DEFAULT_MIN_CONNECTOR_ANCHOR_ALPHA) ? 1 : 0}:${anchor.nativeRendered ? 1 : 0}`)
      .join(','),
    [anchors.visibleAnchors, crossBranchSettings.minConnectorAnchorAlpha],
  )
  const proxySettingsSig = [
    crossBranchSettings.enabled,
    crossBranchSettings.depth,
    crossBranchSettings.connectorBudget,
    crossBranchSettings.connectorPriority,
    crossBranchSettings.minConnectorAnchorAlpha ?? '',
    crossBranchSettings.maxProxyConnectorGroups ?? '',
  ].join(':')

  const viewportReady = canvasW > 0 && viewportBounds.maxY > viewportBounds.minY
  const viewportElementSig = useMemo(() => Array.from(anchors.visibleAnchors.entries())
    .filter(([, anchor]) => {
      const x = anchor.worldX + anchor.worldW / 2
      const y = anchor.worldY + anchor.worldH / 2
      return x >= viewportBounds.minX && x <= viewportBounds.maxX && y >= viewportBounds.minY && y <= viewportBounds.maxY
    })
    .map(([id]) => id)
    .sort((a, b) => a - b)
    .join(','), [anchors.visibleAnchors, viewportBounds])

  // Rebuild when anchors enter or leave the viewport, including camera focus changes.
  // Panning within the same set of anchors reuses topology without another graph walk.
  const proxyConnectors = useMemo(() => {
    return buildVisibleProxyConnectors(workspaceSnapshot, anchors.visibleAnchors, crossBranchSettings, viewportReady ? viewportBounds : undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceSnapshot, visibleElementSig, proxySettingsSig, viewportReady, viewportElementSig])

  const proxyConnectorRenderState = useMemo(() => (
    buildProxyConnectorRenderState(proxyConnectors.connectors)
  ), [proxyConnectors.connectors])

  const proxyHoverIndex = useMemo(() => (
    buildProxyConnectorSpatialIndex(proxyConnectorRenderState, anchors.byNodeId)
  ), [proxyConnectorRenderState, anchors.byNodeId])

  const proxyHoverIndexRef = useRef<ProxyConnectorSpatialIndex | null>(null)
  proxyHoverIndexRef.current = proxyHoverIndex

  const resolveHoveredProxyItem = useCallback((worldX: number, worldY: number, view: ZUIViewState) => {
    const index = proxyHoverIndexRef.current
    if (!index) return null
    return findHoveredProxyConnector(worldX, worldY, index, view)
  }, [])

  return {
    ...anchors,
    proxyConnectors: proxyConnectors.connectors,
    proxyConnectorRenderState,
    hiddenProxyBadges: proxyConnectors.hiddenBadges,
    resolveHoveredProxyItem,
  }
}
