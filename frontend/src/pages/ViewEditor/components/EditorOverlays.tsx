import React from 'react'
import { Box } from '@chakra-ui/react'
import { useViewport, type Node as RFNode } from 'reactflow'
import {
  DEFAULT_SOURCE_HANDLE_SIDE,
  DEFAULT_TARGET_HANDLE_SIDE,
  getHandleFlowPosition,
  getLogicalHandleId,
} from '../../../utils/edgeDistribution'

interface EditorOverlaysProps {
  connectGhostPos: { x: number; y: number } | null
  clickConnectMode: { sourceNodeId: string; sourceHandle: string; targetHandle?: string } | null
  clickConnectCursorPos: { x: number; y: number } | null
  handleReconnectDrag: {
    endpoint: 'source' | 'target'
    fixedNodeId: string
    fixedHandle: string
    movingHandle: string
    cursorPos: { x: number; y: number }
  } | null
  rfNodes: RFNode[]
}

export const EditorOverlays: React.FC<EditorOverlaysProps> = ({
  connectGhostPos,
  clickConnectMode,
  clickConnectCursorPos,
  handleReconnectDrag,
  rfNodes,
}) => {
  const viewportState = useViewport()
  
  return (
    <>
      {/* Ghost node during connector drag */}
      {connectGhostPos && (
        <Box position="fixed" left={`${connectGhostPos.x - 90}px`} top={`${connectGhostPos.y - 42}px`}
          width="180px" minHeight="85px" bg="rgba(var(--bg-element-rgb), 0.4)"
          border="1px solid rgba(113, 128, 150, 0.45)" rounded="lg"
          boxShadow="0 8px 32px rgba(0,0,0,0.4), 0 2px 8px rgba(0,0,0,0.25)"
          pointerEvents="none" zIndex={9999} display="flex" flexDir="column"
          alignItems="center" justifyContent="center" gap="6px" py={3} backdropFilter="blur(4px)">
          <Box w="60%" h="10px" bg="rgba(226,232,240,0.18)" rounded="sm" />
          <Box w="38%" h="7px" bg="rgba(var(--accent-rgb), 0.22)" rounded="sm" />
        </Box>
      )}

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
          clickConnectMode.sourceHandle,
          DEFAULT_SOURCE_HANDLE_SIDE,
        )
        const rfRect = document.querySelector('.react-flow')?.getBoundingClientRect()
        const rfX = rfRect?.left ?? 0; const rfY = rfRect?.top ?? 0
        const sx = fx * viewportState.zoom + viewportState.x + rfX
        const sy = fy * viewportState.zoom + viewportState.y + rfY
        const tx = clickConnectCursorPos.x; const ty = clickConnectCursorPos.y
        const dx = Math.abs(tx - sx); const dy = Math.abs(ty - sy)
        const CURVATURE = 0.5
        const isHSrc = sourceSide === 'left' || sourceSide === 'right'
        const srcMinStem = (isHSrc ? w * 0.5 : h * 0.5) * viewportState.zoom
        let c1x = sx, c1y = sy
        if (sourceSide === 'left') c1x = sx - Math.max(dx * CURVATURE, srcMinStem)
        else if (sourceSide === 'right') c1x = sx + Math.max(dx * CURVATURE, srcMinStem)
        else if (sourceSide === 'top') c1y = sy - Math.max(dy * CURVATURE, srcMinStem)
        else c1y = sy + Math.max(dy * CURVATURE, srcMinStem)
        let c2x = tx, c2y = ty
        if (clickConnectMode.targetHandle) {
          const targetSide = getLogicalHandleId(clickConnectMode.targetHandle, DEFAULT_TARGET_HANDLE_SIDE) ?? DEFAULT_TARGET_HANDLE_SIDE
          const tgtMinStem = (targetSide === 'left' || targetSide === 'right') ? 90 * viewportState.zoom : 40 * viewportState.zoom
          if (targetSide === 'left') c2x = tx - Math.max(dx * CURVATURE, tgtMinStem)
          else if (targetSide === 'right') c2x = tx + Math.max(dx * CURVATURE, tgtMinStem)
          else if (targetSide === 'top') c2y = ty - Math.max(dy * CURVATURE, tgtMinStem)
          else c2y = ty + Math.max(dy * CURVATURE, tgtMinStem)
        }
        return (
          <svg key="click-connect-connector" style={{ position: 'fixed', inset: 0, width: '100%', height: '100%', pointerEvents: 'none', zIndex: 9997 }}>
            <path d={`M${sx},${sy} C${c1x},${c1y} ${c2x},${c2y} ${tx},${ty}`}
              className="react-flow__connector-path" stroke="var(--theme-blue)" strokeWidth="2" fill="none" opacity="0.8" />
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
        const { x: fx, y: fy } = getHandleFlowPosition(
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
        const sourceSide = getLogicalHandleId(
          handleReconnectDrag.endpoint === 'source' ? handleReconnectDrag.movingHandle : handleReconnectDrag.fixedHandle,
          DEFAULT_SOURCE_HANDLE_SIDE,
        ) ?? DEFAULT_SOURCE_HANDLE_SIDE
        const targetSide = getLogicalHandleId(
          handleReconnectDrag.endpoint === 'source' ? handleReconnectDrag.fixedHandle : handleReconnectDrag.movingHandle,
          DEFAULT_TARGET_HANDLE_SIDE,
        ) ?? DEFAULT_TARGET_HANDLE_SIDE

        const dx = Math.abs(targetX - sourceX)
        const dy = Math.abs(targetY - sourceY)
        const CURVATURE = 0.5
        const isHSrc = sourceSide === 'left' || sourceSide === 'right'
        const isHTgt = targetSide === 'left' || targetSide === 'right'
        const srcMinStem = (handleReconnectDrag.endpoint === 'source'
          ? (isHSrc ? 90 : 40)
          : (isHSrc ? w * 0.5 : h * 0.5)) * viewportState.zoom
        const tgtMinStem = (handleReconnectDrag.endpoint === 'target'
          ? (isHTgt ? 90 : 40)
          : (isHTgt ? w * 0.5 : h * 0.5)) * viewportState.zoom

        let c1x = sourceX
        let c1y = sourceY
        if (sourceSide === 'left') c1x = sourceX - Math.max(dx * CURVATURE, srcMinStem)
        else if (sourceSide === 'right') c1x = sourceX + Math.max(dx * CURVATURE, srcMinStem)
        else if (sourceSide === 'top') c1y = sourceY - Math.max(dy * CURVATURE, srcMinStem)
        else c1y = sourceY + Math.max(dy * CURVATURE, srcMinStem)

        let c2x = targetX
        let c2y = targetY
        if (targetSide === 'left') c2x = targetX - Math.max(dx * CURVATURE, tgtMinStem)
        else if (targetSide === 'right') c2x = targetX + Math.max(dx * CURVATURE, tgtMinStem)
        else if (targetSide === 'top') c2y = targetY - Math.max(dy * CURVATURE, tgtMinStem)
        else c2y = targetY + Math.max(dy * CURVATURE, tgtMinStem)

        return (
          <svg key="handle-reconnect-connector" style={{ position: 'fixed', inset: 0, width: '100%', height: '100%', pointerEvents: 'none', zIndex: 9998 }}>
            <path
              d={`M${sourceX},${sourceY} C${c1x},${c1y} ${c2x},${c2y} ${targetX},${targetY}`}
              className="react-flow__connector-path"
              stroke="var(--theme-blue)"
              strokeWidth="2"
              fill="none"
              opacity="0.85"
            />
          </svg>
        )
      })()}

    </>
  )
}
