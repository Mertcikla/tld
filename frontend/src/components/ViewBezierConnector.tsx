import { memo, useCallback } from 'react'
import { BaseEdge, EdgeLabelRenderer, Position, useStore, type EdgeProps } from 'reactflow'
import { measureEdgeLabel, useEdgeLabelLayout } from './ViewEditorEdgeLabelLayout'
import type { ProxyConnectorDetails } from '../crossBranch/types'

const CURVATURE = 0.5

/**
 * Returns the bezier control point for one end of an connector.
 * The stem extends in the handle's exit direction by at least `minStem`
 * world-units, preventing the curve from turning sharply when dx/dy is small.
 */
function controlPoint(
  x: number, y: number,
  tx: number, ty: number,
  pos: Position,
  minStem: number,
): [number, number] {
  const dx = Math.abs(tx - x)
  const dy = Math.abs(ty - y)
  switch (pos) {
    case Position.Left:   return [x - Math.max(dx * CURVATURE, minStem), y]
    case Position.Right:  return [x + Math.max(dx * CURVATURE, minStem), y]
    case Position.Top:    return [x, y - Math.max(dy * CURVATURE, minStem)]
    case Position.Bottom: return [x, y + Math.max(dy * CURVATURE, minStem)]
  }
}

function ViewBezierConnector({
  id, source, target,
  sourceX, sourceY, sourcePosition,
  targetX, targetY, targetPosition,
  style, label, labelStyle, labelBgStyle, labelShowBg: _labelShowBg, labelBgPadding, labelBgBorderRadius,
  markerStart, markerEnd,
  selected,
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

  const srcMinStem = (sourcePosition === Position.Left || sourcePosition === Position.Right)
    ? srcW * 0.5 : srcH * 0.5
  const tgtMinStem = (targetPosition === Position.Left || targetPosition === Position.Right)
    ? tgtW * 0.5 : tgtH * 0.5

  const [cp1x, cp1y] = controlPoint(finalSourceX, finalSourceY, finalTargetX, finalTargetY, sourcePosition, srcMinStem)
  // Target control point: extend from target back toward source
  const [cp2x, cp2y] = controlPoint(finalTargetX, finalTargetY, finalSourceX, finalSourceY, targetPosition, tgtMinStem)

  const path = `M ${finalSourceX},${finalSourceY} C ${cp1x},${cp1y} ${cp2x},${cp2y} ${finalTargetX},${finalTargetY}`

  const INTERACTION_PADDING = 24
  const lenSource = Math.hypot(cp1x - finalSourceX, cp1y - finalSourceY)
  const lenTarget = Math.hypot(cp2x - finalTargetX, cp2y - finalTargetY)

  const ix1 = lenSource > INTERACTION_PADDING ? finalSourceX + (cp1x - finalSourceX) * (INTERACTION_PADDING / lenSource) : finalSourceX
  const iy1 = lenSource > INTERACTION_PADDING ? finalSourceY + (cp1y - finalSourceY) * (INTERACTION_PADDING / lenSource) : finalSourceY
  const ix2 = lenTarget > INTERACTION_PADDING ? finalTargetX + (cp2x - finalTargetX) * (INTERACTION_PADDING / lenTarget) : finalTargetX
  const iy2 = lenTarget > INTERACTION_PADDING ? finalTargetY + (cp2y - finalTargetY) * (INTERACTION_PADDING / lenTarget) : finalTargetY

  const interactionPath = `M ${ix1},${iy1} C ${cp1x},${cp1y} ${cp2x},${cp2y} ${ix2},${iy2}`

  const fontSize = Number(labelStyle?.fontSize ?? 11)
  const fontWeight = 500
  const fullText = typeof label === 'string' ? label : ''
  const text = (!selected && fullText.length > 30) ? `${fullText.slice(0, 30)}...` : fullText
  const textWidth = text ? measureEdgeLabel(text, `${fontWeight} ${fontSize}px Inter, system-ui, sans-serif`) : 0
  const padding = Array.isArray(labelBgPadding) ? labelBgPadding : [2, 4]
  const proxyBadgeCount = typeof (edge?.data as { proxyBadgeCount?: number } | undefined)?.proxyBadgeCount === 'number'
    ? (edge?.data as { proxyBadgeCount: number }).proxyBadgeCount
    : 0
  const proxyBadgeDetails = ((edge?.data as { proxyBadgeDetails?: ProxyConnectorDetails | null } | undefined)?.proxyBadgeDetails) ?? null
  const proxyBadgeText = proxyBadgeCount > 0 ? `+${proxyBadgeCount}` : ''
  const badgeFontSize = 11
  const badgeHorizontalPadding = 7
  const badgeSize = 24
  const labelWidth = textWidth + padding[1] * 2
  const badgeWidth = proxyBadgeText
    ? Math.max(badgeSize, measureEdgeLabel(proxyBadgeText, `600 ${badgeFontSize}px Inter, system-ui, sans-serif`) + badgeHorizontalPadding * 2)
    : 0
  const labelHeight = text ? fontSize + padding[0] * 2 : 0
  const badgeGap = text && proxyBadgeText ? 8 : 0
  const stackWidth = Math.max(labelWidth, badgeWidth)
  const stackHeight = labelHeight + badgeGap + (proxyBadgeText ? badgeSize : 0)

  // Cubic bezier midpoint at t=0.5
  const labelX = 0.125 * finalSourceX + 0.375 * cp1x + 0.375 * cp2x + 0.125 * finalTargetX
  const labelY = 0.125 * finalSourceY + 0.375 * cp1y + 0.375 * cp2y + 0.125 * finalTargetY

  const labelLayout = useEdgeLabelLayout({
    id,
    preferredX: labelX,
    preferredY: labelY + (stackHeight > 0 ? (stackHeight - labelHeight) / 2 : 0),
    width: stackWidth,
    height: stackHeight || (fontSize + padding[0] * 2),
    dx: finalTargetX - finalSourceX,
    dy: finalTargetY - finalSourceY,
  })

  const labelCenterY = labelLayout.y - (proxyBadgeText ? (badgeGap + badgeSize) / 2 : 0)
  const labelPath = text ? ` M ${labelLayout.x - labelWidth / 2},${labelCenterY} L ${labelLayout.x + labelWidth / 2},${labelCenterY}` : ''
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
      {(text || proxyBadgeText) && (
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
            {text && (
              <div
                style={{
                  padding: `${padding[0]}px ${padding[1]}px`,
                  borderRadius: Array.isArray(labelBgBorderRadius) ? labelBgBorderRadius[0] : Number(labelBgBorderRadius ?? 4),
                  background: String(labelBgStyle?.fill ?? 'var(--chakra-colors-gray-900)'),
                  color: String(labelStyle?.fill ?? 'var(--accent)'),
                  fontSize,
                  fontWeight,
                  lineHeight: 1,
                  whiteSpace: 'nowrap',
                }}
              >
                {text}
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
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
}

export default memo(ViewBezierConnector)
