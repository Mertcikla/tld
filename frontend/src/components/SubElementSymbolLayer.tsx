import { Fragment, type CSSProperties, type MouseEvent } from 'react'
import { Box, Text } from '@chakra-ui/react'
import { getSmoothStepPath, Handle, Position } from 'reactflow'
import { symbolStems, type SymbolArea, type SymbolEndpoint, type SymbolOverflow, type SymbolPresentation } from '../utils/subElementSymbols'
import { groupHighlightColors } from '../utils/elementGroups'
import { SUB_ELLIPSIS, SUB_HEADER } from '../utils/subElements'
import type { NodeData } from './ElementNode'
import ElementReconnectZone from './ElementReconnectZone'
import { compareReconnectCandidates } from './elementReconnect'

export interface SymbolInteractions {
  isActive: (area: SymbolArea) => boolean
  showHighlight: (area: SymbolArea) => boolean
  showStem: (endpoint: SymbolEndpoint) => boolean
  onHover: (key: string | null) => void
  onActivate: (area: SymbolArea, event: MouseEvent<HTMLElement>) => void
}
const symbolStyle = (color: string) => ({ '--symbol-color': color } as CSSProperties)
const CARD_BORDER = 1
const PORT_SIZE = 9

export function SubElementSymbolRegions({ presentation, scrollTop, interactions }: {
  presentation: SymbolPresentation; scrollTop: number; interactions: SymbolInteractions
}) {
  return <>{presentation.areas.filter(area => SUB_HEADER + area.contentTop + area.height - scrollTop >= presentation.viewportTop && SUB_HEADER + area.contentTop - scrollTop <= presentation.viewportBottom).map(area => {
    const highlighted = interactions.showHighlight(area)
    const active = highlighted && interactions.isActive(area)
    const colors = groupHighlightColors(area.color, active)
    return <Box key={area.key} data-testid="sub-symbol-region" data-symbol-key={area.key} data-symbol-handle={area.leftId} data-highlighted={highlighted ? 'true' : undefined} data-active={active ? 'true' : undefined}
      className="sub-symbol-region nodrag" position="absolute" left={`${area.x}px`} top={`${area.contentTop}px`} w={`${area.width}px`} h={`${area.height}px`}
      border="1px solid" borderColor={highlighted ? colors.borderColor : 'transparent'} bg={highlighted ? colors.bg : 'transparent'} borderRadius={`${Math.min(8, area.height / 2, presentation.rowHeight / 2 - presentation.highlightInset)}px`} style={symbolStyle(area.color)}
      onMouseEnter={() => interactions.onHover(area.key)} onMouseLeave={() => interactions.onHover(null)} onClick={event => interactions.onActivate(area, event)} />
  })}</>
}

export function SubElementSymbolPorts({ data, presentation, width, height, scrollTop, scrollLeft, canEdit, interactions, onReveal }: {
  data: NodeData; presentation: SymbolPresentation; width: number; height: number; scrollTop: number; scrollLeft: number; canEdit: boolean; interactions: SymbolInteractions
  onReveal: (index: number) => void
}) {
  return <>
    <svg className="sub-symbol-stems" width={width} height={height} style={{ position: 'absolute', inset: 0, pointerEvents: 'none', zIndex: 4 }} aria-hidden="true">
      <defs><clipPath id={`symbol-stems-${data.element_id}`}><rect x={0} y={SUB_HEADER} width={width} height={height - SUB_HEADER} /></clipPath></defs>
      <g clipPath={`url(#symbol-stems-${data.element_id})`}>
        {symbolStems(presentation, width, height, scrollTop, scrollLeft).filter(({ endpoint }) => interactions.showStem(endpoint)).map(({ endpoint, area, sourceX, sourceY, targetX: edgeX, targetY }) => {
          const right = endpoint.side === 'right'
          // SVG coordinates start inside the border; meet the port at the border's center.
          const targetX = edgeX - CARD_BORDER * (right ? 1.5 : 0.5)
          const path = sourceY === targetY ? `M ${sourceX} ${sourceY} H ${targetX}` : getSmoothStepPath({ sourceX, sourceY, targetX, targetY,
            sourcePosition: right ? Position.Right : Position.Left, targetPosition: right ? Position.Left : Position.Right, borderRadius: 8, offset: 10 })[0]
          return <path key={endpoint.id} data-testid="sub-symbol-stem" data-symbol-handle={endpoint.id} className="sub-symbol-stem" data-active={interactions.isActive(area) ? 'true' : undefined} d={path} fill="none" stroke={area.color} strokeWidth={1.4} style={symbolStyle(area.color)} />
        })}
      </g>
    </svg>
    {(['above', 'below'] as const).map(direction => {
      const overflow: SymbolOverflow = presentation[direction]
      const count = overflow.areas.length
      const label = [count ? `${count} ${count === 1 ? 'symbol' : 'symbols'}` : '', overflow.connectionCount ? `${overflow.connectionCount} connections` : ''].filter(Boolean).join(' · ')
      const names = overflow.areas.map(area => area.symbol.name).join(', ')
      const representative = overflow.representative?.area
      if (!label) return null
      return <Box key={direction} data-testid={`sub-symbol-ellipsis-${direction}`} data-symbol-handle={overflow.representative?.id}
        position="absolute" top={direction === 'above' ? `${SUB_HEADER}px` : undefined} bottom={direction === 'below' ? 0 : undefined} left={0} right={0} h={`${SUB_ELLIPSIS}px`}
        bg="#20252d" borderTop={label ? '1px solid' : undefined} borderBottom={label && direction === 'above' ? '1px solid' : undefined} borderColor="whiteAlpha.100" zIndex={6}
        borderBottomRadius={direction === 'below' ? 'calc(var(--chakra-radii-lg) - 1px)' : undefined} overflow="hidden"
        cursor={count ? 'pointer' : 'default'} title={names || label || undefined}
        onClick={event => {
          event.stopPropagation()
          if (data.interactionSourceId && representative && canEdit) interactions.onActivate(representative, event)
          else if (count) onReveal((direction === 'above' ? overflow.areas[count - 1] : overflow.areas[0]).start)
        }}>
        {label && <Text fontFamily="mono" fontSize="9px" lineHeight={`${SUB_ELLIPSIS - 2}px`} color={overflow.color} textAlign="center" isTruncated px={12}>
          ⋯ {label} ⋯
        </Text>}
      </Box>
    })}
    {[...presentation.endpoints.values()].map(endpoint => {
      const active = endpoint.area && interactions.isActive(endpoint.area)
      const shown = endpoint.visible && interactions.showStem(endpoint)
      // Aliases and offscreen symbols share one visible port. Pick up the selected
      // connection there while retaining its original semantic handle ID.
      const candidate = endpoint.visible && canEdit && data.onStartHandleReconnect
        ? [...(data.reconnectCandidates ?? [])].filter(candidate => {
          const anchor = presentation.endpoints.get(candidate.handleId)
          return anchor?.side === endpoint.side && anchor.y === endpoint.y && !data.tagHiddenHandleIds?.includes(candidate.handleId)
        }).sort(compareReconnectCandidates)[0] : undefined
      const position = endpoint.side === 'left' ? Position.Left : Position.Right
      return <Fragment key={endpoint.id}><Handle id={endpoint.id} type="source" position={position}
        isConnectable={canEdit} data-testid={shown ? 'sub-symbol-port' : undefined} data-symbol-port={shown ? 'true' : undefined}
        data-symbol-overflow={endpoint.overflow} data-active={active ? 'true' : undefined} className="sub-symbol-port" title={endpoint.area ? `${endpoint.area.symbol.name} · line ${endpoint.area.symbol.startLine}` : 'Source line'}
        aria-label={endpoint.area ? `Connect ${endpoint.area.symbol.name}` : undefined}
        style={{ ...symbolStyle(endpoint.color), top: endpoint.y, width: PORT_SIZE, height: PORT_SIZE,
          left: endpoint.side === 'left' ? -(PORT_SIZE + CARD_BORDER) / 2 : undefined,
          right: endpoint.side === 'right' ? -(PORT_SIZE + CARD_BORDER) / 2 : undefined,
          opacity: shown ? 1 : 0, pointerEvents: endpoint.visible && canEdit ? 'all' : 'none', background: endpoint.color, border: `2px solid ${endpoint.color}`, zIndex: 9 }}
        onMouseEnter={() => endpoint.area && interactions.onHover(endpoint.area.key)} onMouseLeave={() => interactions.onHover(null)}
        onClick={event => event.stopPropagation()}
        onMouseDown={event => { if (canEdit && event.button === 0) data.onInteractionStart(data.element_id, { sourceHandle: endpoint.id, clientX: event.clientX, clientY: event.clientY }) }}
        onTouchStart={event => {
          const touch = event.touches[0]
          if (canEdit && touch) data.onInteractionStart(data.element_id, { sourceHandle: endpoint.id, clientX: touch.clientX, clientY: touch.clientY })
        }} />
        <ElementReconnectZone candidate={candidate} position={position} anchorStyle={{ top: endpoint.y }} zIndex={10} />
      </Fragment>
    })}
  </>
}
