import type { CSSProperties } from 'react'
import { Box } from '@chakra-ui/react'
import { Position } from 'reactflow'
import type { HandleReconnectCandidate } from './elementReconnect'

function getReconnectZoneStyle(position: Position, anchorStyle: CSSProperties, selected: boolean): CSSProperties {
  if (selected) return {
    ...anchorStyle, [position]: '-22px', width: '44px', height: '44px',
    transform: position === Position.Top || position === Position.Bottom ? 'translateX(-50%)' : 'translateY(-50%)',
  }
  const horizontal = position === Position.Top || position === Position.Bottom
  return {
    ...anchorStyle, [position]: '-18px', width: horizontal ? '28px' : '18px', height: horizontal ? '18px' : '28px',
    transform: horizontal ? 'translateX(-50%)' : 'translateY(-50%)',
  }
}

// The canvas's existing pointer capture and coordinate hit test own the drag.
export default function ElementReconnectZone({ candidate, position, anchorStyle, zIndex = 4 }: {
  candidate?: HandleReconnectCandidate; position: Position; anchorStyle: CSSProperties; zIndex?: number
}) {
  if (!candidate) return null
  return <Box position="absolute" data-testid="vieweditor-node-reconnect-zone"
    data-edge-id={candidate.edgeId} data-endpoint={candidate.endpoint} data-handle-id={candidate.handleId}
    className="element-node-reconnect-zone" data-selected={candidate.selected ? 'true' : undefined}
    style={getReconnectZoneStyle(position, anchorStyle, candidate.selected)} pointerEvents="auto" cursor="grab" zIndex={zIndex} />
}
