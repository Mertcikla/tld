import { memo, useCallback } from 'react'
import { BaseEdge, EdgeLabelRenderer, useStore, type ConnectionLineComponentProps, type EdgeProps, type Node as RFNode } from 'reactflow'
import { measureEdgeLabel, useEdgeLabelLayout } from './ViewEditorEdgeLabelLayout'
import type { ProxyConnectorDetails } from '../crossBranch/types'
import { buildViewConnectorPath, handleSideForPosition, positionForHandleSide, routeStyleFromValue } from '../utils/connectorRoute'
import {
  DEFAULT_SOURCE_HANDLE_SIDE,
  DEFAULT_TARGET_HANDLE_SIDE,
  getCenterVisualHandleId,
  getHandleFlowPosition,
  getLogicalHandleId,
} from '../utils/edgeDistribution'

function getNodeFlowOrigin(node: RFNode) {
  return ((node as { positionAbsolute?: { x: number; y: number } }).positionAbsolute ?? node.position)
}

export function ViewConnectorConnectionLine({
  connectionLineStyle,
  connectionLineType,
  fromNode,
  fromHandle,
  fromX,
  fromY,
  toX,
  toY,
  fromPosition,
  toPosition,
}: ConnectionLineComponentProps) {
  const sourceFallbackSide = handleSideForPosition(fromPosition, DEFAULT_SOURCE_HANDLE_SIDE)
  const sourceSide = getLogicalHandleId(fromHandle?.id, sourceFallbackSide) ?? sourceFallbackSide
  const sourceHandle = getCenterVisualHandleId(fromHandle?.id ?? sourceSide, sourceSide) ?? sourceSide
  let finalFromX = fromX
  let finalFromY = fromY
  const finalToX = toX
  const finalToY = toY
  let finalSourceSide = sourceSide
  const finalTargetSide = handleSideForPosition(toPosition, DEFAULT_TARGET_HANDLE_SIDE)
  let sourceWidth = 200
  let sourceHeight = 100
  const targetWidth = 200
  const targetHeight = 100

  if (fromNode) {
    sourceWidth = fromNode.width ?? sourceWidth
    sourceHeight = fromNode.height ?? sourceHeight
    const sourceOrigin = getNodeFlowOrigin(fromNode)
    const sourcePoint = getHandleFlowPosition(sourceOrigin.x, sourceOrigin.y, sourceWidth, sourceHeight, sourceHandle, sourceSide)
    finalFromX = sourcePoint.x
    finalFromY = sourcePoint.y
    finalSourceSide = sourcePoint.side
  }

  const { path } = buildViewConnectorPath({
    routeStyle: routeStyleFromValue(connectionLineType),
    sourceX: finalFromX,
    sourceY: finalFromY,
    targetX: finalToX,
    targetY: finalToY,
    sourcePosition: positionForHandleSide(finalSourceSide),
    targetPosition: positionForHandleSide(finalTargetSide),
    sourceWidth,
    sourceHeight,
    targetWidth,
    targetHeight,
  })

  return <path className="vieweditor-temporary-connector-path" fill="none" d={path} style={connectionLineStyle} />
}

function ViewBezierConnector({
  id, source, target,
  sourceX, sourceY, sourcePosition,
  targetX, targetY, targetPosition,
  style, label, labelStyle, labelBgStyle, labelShowBg: _labelShowBg, labelBgPadding, labelBgBorderRadius,
  markerStart, markerEnd,
  selected,
  data,
}: EdgeProps) {
  const sourceNode = useStore((s) => s.nodeInternals.get(source))
  const targetNode = useStore((s) => s.nodeInternals.get(target))
  const edge = useStore((s) => s.edges.find((candidate) => candidate.id === id))

  const finalSourceX = sourceX
  const finalSourceY = sourceY
  const finalTargetX = targetX
  const finalTargetY = targetY

  const srcW = sourceNode?.width ?? 200
  const srcH = sourceNode?.height ?? 100
  const tgtW = targetNode?.width ?? 200
  const tgtH = targetNode?.height ?? 100

  const routeStyle = routeStyleFromValue((data as { style?: unknown } | undefined)?.style)
  const { path, labelX, labelY } = buildViewConnectorPath({
    routeStyle,
    sourceX: finalSourceX,
    sourceY: finalSourceY,
    targetX: finalTargetX,
    targetY: finalTargetY,
    sourcePosition,
    targetPosition,
    sourceWidth: srcW,
    sourceHeight: srcH,
    targetWidth: tgtW,
    targetHeight: tgtH,
  })

  const INTERACTION_PADDING = 24
  const interactionPath = routeStyle === 'bezier'
    ? buildViewConnectorPath({
      routeStyle,
      sourceX: finalSourceX,
      sourceY: finalSourceY,
      targetX: finalTargetX,
      targetY: finalTargetY,
      sourcePosition,
      targetPosition,
      sourceWidth: Math.max(srcW - INTERACTION_PADDING * 2, 0),
      sourceHeight: Math.max(srcH - INTERACTION_PADDING * 2, 0),
      targetWidth: Math.max(tgtW - INTERACTION_PADDING * 2, 0),
      targetHeight: Math.max(tgtH - INTERACTION_PADDING * 2, 0),
    }).path
    : path

  const fontSize = Number(labelStyle?.fontSize ?? 11)
  const fontWeight = 500
  const fullText = typeof label === 'string' ? label : ''
  const displayText = (!selected && fullText.length > 30) ? `${fullText.slice(0, 30)}...` : fullText
  const textWidth = displayText ? measureEdgeLabel(displayText, `${fontWeight} ${fontSize}px Inter, system-ui, sans-serif`) : 0
  const padding = Array.isArray(labelBgPadding) ? labelBgPadding : [2, 4]
  const proxyBadgeCount = typeof (edge?.data as { proxyBadgeCount?: number } | undefined)?.proxyBadgeCount === 'number'
    ? (edge?.data as { proxyBadgeCount: number }).proxyBadgeCount
    : 0
  const proxyBadgeDetails = ((edge?.data as { proxyBadgeDetails?: ProxyConnectorDetails | null } | undefined)?.proxyBadgeDetails) ?? null
  const proxyBadgeText = proxyBadgeCount > 0 ? `+${proxyBadgeCount}` : ''
  const versionChangeType = (edge?.data as { versionChangeType?: string } | undefined)?.versionChangeType
  const versionBadgeText = versionChangeType === 'added'
    ? '+ connector'
    : versionChangeType === 'deleted'
      ? '- connector'
      : versionChangeType
        ? '~ connector'
        : ''
  const badgeFontSize = 11
  const badgeHorizontalPadding = 7
  const badgeSize = 24
  const labelWidth = textWidth + padding[1] * 2
  const versionBadgeWidth = versionBadgeText
    ? measureEdgeLabel(versionBadgeText, `700 ${badgeFontSize}px Inter, system-ui, sans-serif`) + badgeHorizontalPadding * 2
    : 0
  const badgeWidth = proxyBadgeText
    ? Math.max(badgeSize, measureEdgeLabel(proxyBadgeText, `600 ${badgeFontSize}px Inter, system-ui, sans-serif`) + badgeHorizontalPadding * 2)
    : 0
  const labelHeight = fullText ? fontSize + padding[0] * 2 : 0
  const badgeGap = (fullText && (proxyBadgeText || versionBadgeText)) || (proxyBadgeText && versionBadgeText) ? 8 : 0
  const stackWidth = Math.max(labelWidth, badgeWidth, versionBadgeWidth)
  const stackHeight = labelHeight +
    (fullText && (proxyBadgeText || versionBadgeText) ? badgeGap : 0) +
    (versionBadgeText ? badgeSize : 0) +
    (versionBadgeText && proxyBadgeText ? badgeGap : 0) +
    (proxyBadgeText ? badgeSize : 0)

  const labelLayout = useEdgeLabelLayout({
    id,
    preferredX: labelX,
    preferredY: labelY + (stackHeight > 0 ? (stackHeight - labelHeight) / 2 : 0),
    width: stackWidth,
    height: stackHeight || (fontSize + padding[0] * 2),
    dx: finalTargetX - finalSourceX,
    dy: finalTargetY - finalSourceY,
  })

  const labelCenterY = labelLayout.y - ((proxyBadgeText || versionBadgeText) ? (stackHeight - labelHeight) / 2 : 0)
  const labelPath = fullText ? ` M ${labelLayout.x - labelWidth / 2},${labelCenterY} L ${labelLayout.x + labelWidth / 2},${labelCenterY}` : ''
  const combinedInteractionPath = `${interactionPath}${labelPath}`
  const handleBadgeClick = useCallback((event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault()
    event.stopPropagation()
    if (!proxyBadgeDetails) return
    const onOpenProxyBadge = (edge?.data as { onOpenProxyBadge?: (details: ProxyConnectorDetails) => void } | undefined)?.onOpenProxyBadge
    onOpenProxyBadge?.(proxyBadgeDetails)
  }, [edge?.data, proxyBadgeDetails])

  return (
    <>
      <BaseEdge
        path={path}
        markerStart={markerStart}
        markerEnd={markerEnd}
        style={style}
        interactionWidth={0}
      />
      <BaseEdge
        id={id}
        path={combinedInteractionPath}
        interactionWidth={20}
        style={{ stroke: 'transparent' }}
      />
      {(fullText || proxyBadgeText || versionBadgeText) && (
        <EdgeLabelRenderer>
          <div
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelLayout.x}px, ${labelLayout.y}px)`,
              pointerEvents: 'none',
              opacity: Number(labelStyle?.opacity ?? 1),
              zIndex: 2,
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              gap: badgeGap,
            }}
          >
            {fullText && (
              <div
                aria-label={fullText}
                title={fullText}
                style={{
                  width: labelWidth,
                  padding: `${padding[0]}px ${padding[1]}px`,
                  borderRadius: Array.isArray(labelBgBorderRadius) ? labelBgBorderRadius[0] : Number(labelBgBorderRadius ?? 4),
                  background: String(labelBgStyle?.fill ?? 'var(--chakra-colors-gray-900)'),
                  color: String(labelStyle?.fill ?? 'var(--accent)'),
                  fontSize,
                  fontWeight,
                  lineHeight: 1,
                  whiteSpace: 'nowrap',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  boxSizing: 'border-box',
                }}
              >
                {fullText}
              </div>
            )}
            {proxyBadgeText && (
              <button
                type="button"
                onClick={handleBadgeClick}
                style={{
                  minWidth: badgeWidth,
                  height: badgeSize,
                  padding: `0 ${badgeHorizontalPadding}px`,
                  borderRadius: 999,
                  background: 'var(--bg-element)',
                  border: '1px dashed rgba(var(--accent-rgb), 0.8)',
                  color: 'white',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: badgeFontSize,
                  fontWeight: 600,
                  lineHeight: 1,
                  boxShadow: selected ? '0 0 0 1px rgba(255,255,255,0.2)' : 'none',
                  cursor: proxyBadgeDetails ? 'pointer' : 'default',
                  pointerEvents: 'auto',
                  appearance: 'none',
                }}
              >
                {proxyBadgeText}
              </button>
            )}
            {versionBadgeText && (
              <div
                style={{
                  minWidth: versionBadgeWidth,
                  height: badgeSize,
                  padding: `0 ${badgeHorizontalPadding}px`,
                  borderRadius: 999,
                  background: 'rgba(17, 24, 39, 0.9)',
                  border: `1px solid ${versionChangeType === 'added' ? '#68d391' : versionChangeType === 'deleted' ? '#fc8181' : '#f6e05e'}`,
                  color: versionChangeType === 'added' ? '#68d391' : versionChangeType === 'deleted' ? '#fc8181' : '#f6e05e',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: badgeFontSize,
                  fontWeight: 700,
                  lineHeight: 1,
                  boxShadow: '0 6px 18px rgba(0,0,0,0.28)',
                }}
              >
                {versionBadgeText}
              </div>
            )}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
}

export default memo(ViewBezierConnector)
