import React from 'react'
import { useViewport, type Node as RFNode } from 'reactflow'
import {
  DEFAULT_SOURCE_HANDLE_SIDE,
  DEFAULT_TARGET_HANDLE_SIDE,
  getCenterVisualHandleId,
  getHandleFlowPosition,
  getLogicalHandleId,
  getOppositeHandleSide,
} from '../../../utils/edgeDistribution'
import { buildViewConnectorPath, positionForHandleSide, type ConnectorRouteStyle } from '../../../utils/connectorRoute'
import { findClosestHandles } from '../utils'
import { TEMPORARY_CONNECTOR_PATH_STYLE } from '../temporaryConnectorStyle'

interface EditorOverlaysProps {
  clickConnectMode: { sourceNodeId: string; sourceHandle: string; targetHandle?: string } | null
  clickConnectCursorPos: { x: number; y: number } | null
  connectorRouteStyle: ConnectorRouteStyle
  handleReconnectDrag: {
    endpoint: 'source' | 'target'
    fixedNodeId: string
    fixedHandle: string
    movingHandle: string
    cursorPos: { x: number; y: number }
    routeStyle?: ConnectorRouteStyle
    hoveredNodeId?: string
    hoveredHandleId?: string
  } | null
  altConnectorDrag?: {
    sourceNodeId: string
    sourceHandle: string
    cursorPos: { x: number; y: number }
    hoveredNodeId?: string
    hoveredHandleId?: string | null
  } | null
  rfNodes: RFNode[]
}

export const EditorOverlays: React.FC<EditorOverlaysProps> = React.memo(({
  clickConnectMode,
  clickConnectCursorPos,
  connectorRouteStyle,
  handleReconnectDrag,
  altConnectorDrag,
  rfNodes,
}) => {
  const viewportState = useViewport()
  
  return (
    <>
      {/* Click-connect ghost connector */}
      {clickConnectMode && clickConnectCursorPos && (() => {
        const sourceNode = rfNodes.find((n) => n.id === clickConnectMode.sourceNodeId)
        if (!sourceNode) return null
        const w = sourceNode.width ?? 180; const h = sourceNode.height ?? 80
        const sp = sourceNode.positionAbsolute ?? sourceNode.position
        const { x: fx, y: fy, side: sourceSide } = getHandleFlowPosition(
          sp.x,
          sp.y,
          w,
          h,
          getCenterVisualHandleId(clickConnectMode.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE),
          DEFAULT_SOURCE_HANDLE_SIDE,
        )
        const rfRect = document.querySelector('.react-flow')?.getBoundingClientRect()
        const rfX = rfRect?.left ?? 0; const rfY = rfRect?.top ?? 0
        const sx = fx * viewportState.zoom + viewportState.x + rfX
        const sy = fy * viewportState.zoom + viewportState.y + rfY
        const tx = clickConnectCursorPos.x; const ty = clickConnectCursorPos.y
        const targetSide = getLogicalHandleId(clickConnectMode.targetHandle, null) ?? getOppositeHandleSide(sourceSide)
        const { path } = buildViewConnectorPath({
          routeStyle: connectorRouteStyle,
          sourceX: sx,
          sourceY: sy,
          targetX: tx,
          targetY: ty,
          sourcePosition: positionForHandleSide(sourceSide),
          targetPosition: positionForHandleSide(targetSide),
          sourceWidth: w * viewportState.zoom,
          sourceHeight: h * viewportState.zoom,
          targetWidth: 180 * viewportState.zoom,
          targetHeight: 80 * viewportState.zoom,
        })
        return (
          <svg
            key="click-connect-connector"
            data-testid="click-connect-connector"
            style={{ position: 'fixed', inset: 0, width: '100%', height: '100%', pointerEvents: 'none', zIndex: 9997 }}
          >
            <path
              d={path}
              className="react-flow__connector-path vieweditor-temporary-connector-path"
              fill="none"
              style={TEMPORARY_CONNECTOR_PATH_STYLE}
            />
          </svg>
        )
      })()}

      {/* Alt-drag ghost connector */}
      {altConnectorDrag && (() => {
        const sourceNode = rfNodes.find((n) => n.id === altConnectorDrag.sourceNodeId)
        if (!sourceNode) return null
        const w = sourceNode.width ?? 180; const h = sourceNode.height ?? 80
        const sp = sourceNode.positionAbsolute ?? sourceNode.position
        const { x: fx, y: fy, side: sourceSide } = getHandleFlowPosition(
          sp.x,
          sp.y,
          w,
          h,
          getCenterVisualHandleId(altConnectorDrag.sourceHandle, DEFAULT_SOURCE_HANDLE_SIDE),
          DEFAULT_SOURCE_HANDLE_SIDE,
        )
        const rfRect = document.querySelector('.react-flow')?.getBoundingClientRect()
        const rfX = rfRect?.left ?? 0; const rfY = rfRect?.top ?? 0
        const sx = fx * viewportState.zoom + viewportState.x + rfX
        const sy = fy * viewportState.zoom + viewportState.y + rfY

        const hoveredNode = altConnectorDrag.hoveredNodeId
          ? rfNodes.find((n) => n.id === altConnectorDrag.hoveredNodeId)
          : undefined

        let tx = altConnectorDrag.cursorPos.x
        let ty = altConnectorDrag.cursorPos.y
        let targetSide = getOppositeHandleSide(sourceSide)
        let targetWidth = 180 * viewportState.zoom
        let targetHeight = 80 * viewportState.zoom

        if (hoveredNode) {
          const hw = hoveredNode.width ?? 180; const hh = hoveredNode.height ?? 80
          const hp = hoveredNode.positionAbsolute ?? hoveredNode.position
          const closest = findClosestHandles(sourceNode, hoveredNode)
          const targetHandle = getCenterVisualHandleId(closest.targetHandle, DEFAULT_TARGET_HANDLE_SIDE) ?? closest.targetHandle
          const target = getHandleFlowPosition(hp.x, hp.y, hw, hh, targetHandle, DEFAULT_TARGET_HANDLE_SIDE)
          tx = target.x * viewportState.zoom + viewportState.x + rfX
          ty = target.y * viewportState.zoom + viewportState.y + rfY
          targetSide = target.side
          targetWidth = hw * viewportState.zoom
          targetHeight = hh * viewportState.zoom
        }

        const { path } = buildViewConnectorPath({
          routeStyle: connectorRouteStyle,
          sourceX: sx,
          sourceY: sy,
          targetX: tx,
          targetY: ty,
          sourcePosition: positionForHandleSide(sourceSide),
          targetPosition: positionForHandleSide(targetSide),
          sourceWidth: w * viewportState.zoom,
          sourceHeight: h * viewportState.zoom,
          targetWidth,
          targetHeight,
        })
        return (
          <svg
            key="alt-connector-connector"
            data-testid="alt-connector-connector"
            style={{ position: 'fixed', inset: 0, width: '100%', height: '100%', pointerEvents: 'none', zIndex: 9997 }}
          >
            <path
              d={path}
              className="react-flow__connector-path vieweditor-temporary-connector-path"
              fill="none"
              style={TEMPORARY_CONNECTOR_PATH_STYLE}
            />
          </svg>
        )
      })()}

      {/* Handle-reconnect ghost connector */}
      {handleReconnectDrag && (() => {
        const fixedNode = rfNodes.find((n) => n.id === handleReconnectDrag.fixedNodeId)
        if (!fixedNode) return null
        const w = fixedNode.width ?? 180
        const h = fixedNode.height ?? 80
        const sp = fixedNode.positionAbsolute ?? fixedNode.position
        const { x: fx, y: fy, side: fixedSide } = getHandleFlowPosition(
          sp.x,
          sp.y,
          w,
          h,
          handleReconnectDrag.fixedHandle,
          handleReconnectDrag.endpoint === 'source' ? DEFAULT_TARGET_HANDLE_SIDE : DEFAULT_SOURCE_HANDLE_SIDE,
        )
        const rfRect = document.querySelector('.react-flow')?.getBoundingClientRect()
        const rfX = rfRect?.left ?? 0
        const rfY = rfRect?.top ?? 0
        const fixedScreenX = fx * viewportState.zoom + viewportState.x + rfX
        const fixedScreenY = fy * viewportState.zoom + viewportState.y + rfY
        const movingScreenX = handleReconnectDrag.cursorPos.x
        const movingScreenY = handleReconnectDrag.cursorPos.y

        const sourceX = handleReconnectDrag.endpoint === 'source' ? movingScreenX : fixedScreenX
        const sourceY = handleReconnectDrag.endpoint === 'source' ? movingScreenY : fixedScreenY
        const targetX = handleReconnectDrag.endpoint === 'source' ? fixedScreenX : movingScreenX
        const targetY = handleReconnectDrag.endpoint === 'source' ? fixedScreenY : movingScreenY
        const movingSide = getLogicalHandleId(
          handleReconnectDrag.movingHandle,
          handleReconnectDrag.endpoint === 'source' ? DEFAULT_SOURCE_HANDLE_SIDE : DEFAULT_TARGET_HANDLE_SIDE,
        ) ?? (handleReconnectDrag.endpoint === 'source' ? DEFAULT_SOURCE_HANDLE_SIDE : DEFAULT_TARGET_HANDLE_SIDE)
        const sourceSide = handleReconnectDrag.endpoint === 'source' ? movingSide : fixedSide
        const targetSide = handleReconnectDrag.endpoint === 'source' ? fixedSide : movingSide
        const movingNode = handleReconnectDrag.hoveredNodeId
          ? rfNodes.find((n) => n.id === handleReconnectDrag.hoveredNodeId)
          : null
        const movingWidth = (movingNode?.width ?? 180) * viewportState.zoom
        const movingHeight = (movingNode?.height ?? 80) * viewportState.zoom
        const fixedWidth = w * viewportState.zoom
        const fixedHeight = h * viewportState.zoom
        const { path } = buildViewConnectorPath({
          routeStyle: handleReconnectDrag.routeStyle ?? connectorRouteStyle,
          sourceX,
          sourceY,
          targetX,
          targetY,
          sourcePosition: positionForHandleSide(sourceSide),
          targetPosition: positionForHandleSide(targetSide),
          sourceWidth: handleReconnectDrag.endpoint === 'source' ? movingWidth : fixedWidth,
          sourceHeight: handleReconnectDrag.endpoint === 'source' ? movingHeight : fixedHeight,
          targetWidth: handleReconnectDrag.endpoint === 'source' ? fixedWidth : movingWidth,
          targetHeight: handleReconnectDrag.endpoint === 'source' ? fixedHeight : movingHeight,
        })

        return (
          <svg key="handle-reconnect-connector" style={{ position: 'fixed', inset: 0, width: '100%', height: '100%', pointerEvents: 'none', zIndex: 9998 }}>
            <path
              d={path}
              className="react-flow__connector-path vieweditor-temporary-connector-path"
              fill="none"
              style={TEMPORARY_CONNECTOR_PATH_STYLE}
            />
          </svg>
        )
      })()}

    </>
  )
})
EditorOverlays.displayName = 'EditorOverlays'
