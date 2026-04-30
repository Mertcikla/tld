import { memo, useEffect, useMemo, useRef, useState } from 'react'
import { Handle, Position, useStore } from 'reactflow'
import { Box, Flex, Text, Tooltip, HStack, Button, Divider, VStack } from '@chakra-ui/react'
import { LinkIcon } from '@chakra-ui/icons'
import { useAccentColor } from '../context/ThemeContext'

import type { PlacedElement, ViewConnector, Tag } from '../types'
import { ElementContainer } from './NodeContainer'
import { ElementBody } from './NodeBody'
import { resolveIconPath } from '../utils/url'
import { ZoomInIcon, ZoomOutIcon, TrashIcon as TrashSvg, EditIcon as EditSvg } from './Icons'
import { vscodeBridge } from '../lib/vscodeBridge'
import type { ExtensionToWebviewMessage } from '../types/vscode-messages'
import {
  getVisualHandleId,
  getVisualHandleStyle,
  HANDLE_SLOT_CENTER_INDEX,
  HANDLE_SLOT_COUNT,
} from '../utils/edgeDistribution'

function VscodeCodePreview({ filePath, isCanvasMoving }: { filePath: string; isCanvasMoving?: boolean }) {
  const [content, setContent] = useState<string | null>(null)
  const [startLineOffset, setStartLineOffset] = useState(0)
  const [isOpen, setIsOpen] = useState(false)
  const hoverTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const [relPath, anchorStr] = filePath.split('#')
  const anchor = useMemo(() => {
    try {
      return anchorStr ? JSON.parse(decodeURIComponent(anchorStr)) : null
    } catch {
      return null
    }
  }, [anchorStr])
  const startLine = typeof anchor?.startLine === 'number' ? anchor.startLine : undefined
  const symbolName = typeof anchor?.name === 'string' ? anchor.name : undefined
  const symbolKind = typeof anchor?.type === 'string' ? anchor.type : undefined

  useEffect(() => {
    if (!isOpen) return
    const requestId = `file-${Math.random().toString(36).slice(2)}`
    
    const unsub = vscodeBridge.onMessage((msg: ExtensionToWebviewMessage) => {
      if (msg.type === 'file-content' && msg.requestId === requestId) {
        setContent(msg.content)
        setStartLineOffset(msg.startLineOffset)
      }
    })

    vscodeBridge.postMessage({
      type: 'request-file-content',
      requestId,
      filePath: relPath,
      startLine: startLine ?? 0
    })

    return unsub
  }, [isOpen, relPath, startLine])

  const handleMouseEnter = () => {
    if (isCanvasMoving) return
    if (hoverTimeoutRef.current) clearTimeout(hoverTimeoutRef.current)
    hoverTimeoutRef.current = setTimeout(() => setIsOpen(true), 300)
  }

  const handleMouseLeave = () => {
    if (hoverTimeoutRef.current) clearTimeout(hoverTimeoutRef.current)
    setIsOpen(false)
  }

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    vscodeBridge.postMessage({
      type: 'open-file',
      filePath: relPath,
      startLine,
      symbolName,
      symbolKind,
    })
  }

  return (
    <Tooltip
      label={
        content ? (
          <Box p={1} maxW="300px" fontFamily="mono" fontSize="xs" whiteSpace="pre" overflowX="auto">
            {content.split('\n').map((line, i) => {
              const actualLine = startLineOffset + i
              const isTargetLine = typeof startLine === 'number' && actualLine === startLine
              return (
                <Text key={i} color={isTargetLine ? 'blue.300' : 'whiteAlpha.800'} bg={isTargetLine ? 'whiteAlpha.200' : 'transparent'} px={1}>
                  {line}
                </Text>
              )
            })}
          </Box>
        ) : (
          <Text>Loading preview...</Text>
        )
      }
      placement="top"
      isOpen={isOpen}
      hasArrow
      bg="gray.800"
      color="white"
      boxShadow="lg"
      borderRadius="md"
      px={0}
      py={0}
    >
      <Box
        as="button"
        display="flex"
        alignItems="center"
        justifyContent="center"
        w="18px"
        h="18px"
        rounded="md"
        color="whiteAlpha.900"
        _hover={{ color: 'teal.300', bg: 'whiteAlpha.200', transform: 'scale(1.1)' }}
        transition="all 0.15s"
        onClick={handleClick}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
        onPointerDown={(e: React.PointerEvent) => e.stopPropagation()}
      >
        <LinkIcon w={2.5} h={2.5} />
      </Box>
    </Tooltip>
  )
}

interface NodeData extends PlacedElement {
  links: ViewConnector[]        // child (zoom-in) connectors
  parentLinks: ViewConnector[]  // parent (zoom-out) connectors
  parentViewId?: number | null
  onZoomIn: (elementId: number) => void
  onZoomOut: (elementId: number) => void
  onNavigateToDiagram: (viewId: number) => void
  onSelect: (obj: PlacedElement) => void
  onInteractionStart: (elementId: number, options?: { sourceHandle?: string; clientX?: number; clientY?: number }) => void
  onConnectTo: (elementId: number) => void
  onStartHandleReconnect?: (args: { edgeId: string; endpoint: 'source' | 'target'; handleId: string; clientX: number; clientY: number }) => void
  onRemove: (elementId: number) => void
  onHoverZoom: (elementId: number, type: 'in' | 'out' | null) => void
  isZoomHovered: 'in' | 'out' | null
  interactionSourceId: number | null
  onOpenCodePreview?: (elementId: number) => void
  isClickConnectMode?: boolean
  tagColors: Record<string, Tag>
  layerHighlightColor?: string
  forceShowTagPopup?: boolean
  isCanvasMoving?: boolean
  connectedHandleIds?: readonly string[]
  selectedHandleIds?: readonly string[]
  reconnectCandidates?: readonly { handleId: string; edgeId: string; endpoint: 'source' | 'target'; selected: boolean }[]
  isConnectorHighlighted?: boolean
}

interface Props {
  data: NodeData
  selected: boolean
}

// Small icon button used for zoom-in / zoom-out
// variant='out' → extruded (raised clay bump)
// variant='in'  → sunken  (pressed clay indentation)
function LayerButton({
  label,
  active,
  variant,
  onClick,
  onMouseEnter,
  onMouseLeave,
  isDisabled,
  count = 0,
  children,
}: {
  label: string
  active: boolean
  variant: 'out' | 'in'
  onClick: (e: React.MouseEvent) => void
  onMouseEnter?: () => void
  onMouseLeave?: () => void
  isDisabled?: boolean
  count?: number
  children: React.ReactNode
}) {
  const isOut = variant === 'out'

  const shadow = active
    ? isOut
      ? 'clay-out'
      : 'clay-in'
    : 'none'



  return (
    <Tooltip label={label} placement="top" openDelay={300} isDisabled={isDisabled}>
      <Box
        as="button"
        w="22px"
        h="22px"
        display="flex"
        alignItems="center"
        justifyContent="center"
        rounded="lg"
        fontSize="xs"
        lineHeight={1}
        color={active ? (isOut ? 'blue.400' : 'teal.400') : 'whiteAlpha.300'}
        border="none"
        opacity={active ? 1 : 0.5}
        boxShadow={shadow}
        position="relative"
        _hover={{
          opacity: 1,
        }}
        onClick={onClick}
        onPointerDown={(e: React.PointerEvent) => e.stopPropagation()}
        onMouseEnter={onMouseEnter}
        onMouseLeave={onMouseLeave}
        flexShrink={0}
        pointerEvents="auto"
      >
        <Box
          transform={!isOut && active ? 'translateY(0.5px)' : 'none'}
          transition="transform 0.15s"
          display="flex"
          alignItems="center"
          justifyContent="center"
        >
          {children}
        </Box>
        {count > 1 && (
          <Box
            position="absolute"
            top="-5px"
            right="-5px"
            color={isOut ? 'blue.400' : 'teal.400'}
            fontSize="8px"
            fontWeight="bold"
            w="11px"
            h="11px"
            rounded="full"
            display="flex"
            alignItems="center"
            justifyContent="center"
            boxShadow="0 1px 3px rgba(0,0,0,0.3)"
          >
            {count}
          </Box>
        )}
      </Box>
    </Tooltip>
  )
}

const zoomSelector = (s: { transform: [number, number, number] }) =>
  Math.round(s.transform[2] * 50) / 50

const HANDLE_SLOTS = Array.from({ length: HANDLE_SLOT_COUNT }, (_, index) => index)
const HANDLE_CONFIGS = [
  { side: 'top', position: Position.Top },
  { side: 'left', position: Position.Left },
  { side: 'right', position: Position.Right },
  { side: 'bottom', position: Position.Bottom },
] as const

function getReconnectZoneStyle(position: Position, slot: number): React.CSSProperties {
  const baseStyle = getVisualHandleStyle(position, slot)

  switch (position) {
    case Position.Top:
      return { ...baseStyle, top: '-18px', width: '28px', height: '18px', transform: 'translateX(-50%)' }
    case Position.Bottom:
      return { ...baseStyle, bottom: '-18px', width: '28px', height: '18px', transform: 'translateX(-50%)' }
    case Position.Left:
      return { ...baseStyle, left: '-18px', width: '18px', height: '28px', transform: 'translateY(-50%)' }
    case Position.Right:
      return { ...baseStyle, right: '-18px', width: '18px', height: '28px', transform: 'translateY(-50%)' }
  }
}

function ElementNode({ data, selected }: Props) {
  const zoom = useStore(zoomSelector)
  useAccentColor()

  const connectedHandleIds = useMemo(
    () => new Set(data.connectedHandleIds ?? []),
    [data.connectedHandleIds],
  )
  const selectedHandleIds = useMemo(
    () => new Set(data.selectedHandleIds ?? []),
    [data.selectedHandleIds],
  )
  const activeSides = useMemo(() => {
    const sides = new Set<string>()
    for (const handleId of connectedHandleIds) {
      const side = handleId.split('-', 1)[0]
      if (side) sides.add(side)
    }
    return sides
  }, [connectedHandleIds])
  const reconnectCandidateByHandle = useMemo(() => {
    const next = new Map<string, { edgeId: string; endpoint: 'source' | 'target' }>()
    const sortedCandidates = [...(data.reconnectCandidates ?? [])].sort((left, right) => {
      if (left.selected !== right.selected) return left.selected ? -1 : 1
      return left.edgeId.localeCompare(right.edgeId)
    })
    for (const candidate of sortedCandidates) {
      if (!next.has(candidate.handleId)) {
        next.set(candidate.handleId, { edgeId: candidate.edgeId, endpoint: candidate.endpoint })
      }
    }
    return next
  }, [data.reconnectCandidates])

  const derivedPrimaryIconPath = (() => {
    const selected = data.technology_connectors?.find((link) => link.type === 'catalog' && !!(link.is_primary_icon ?? link.isPrimaryIcon) && !!link.slug)
    if (!selected?.slug) return undefined
    return resolveIconPath(`/icons/${selected.slug}.png`)
  })()

  const nodeLogoUrl = data.logo_url ? resolveIconPath(data.logo_url) : derivedPrimaryIconPath

  const technologyLinkCount = (data.technology_connectors || []).filter((l) => !!l.label).length
  const technologyParts = (data.technology || '')
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean)
  const technologyCount = technologyLinkCount > 0 ? technologyLinkCount : technologyParts.length

  const showTechnologyText = !nodeLogoUrl || technologyCount > 1
  const technologyText =
    showTechnologyText
      ? (data.technology || (technologyLinkCount > 1 ? data.technology_connectors.map((l) => l.label).join(', ') : undefined))
      : undefined

  const [menuVisible, setMenuVisible] = useState(false)
  const [isDraggedOver, setIsDraggedOver] = useState(false)
  const menuRef = useRef<{ type: 'in' | 'out', links: ViewConnector[] } | null>(null)

  useEffect(() => {
    if (data.isCanvasMoving) {
      setMenuVisible(false)
    }
  }, [data.isCanvasMoving])

  const handleDragOver = (e: React.DragEvent) => {
    if (e.dataTransfer.types.includes('application/diag-tag') || e.dataTransfer.types.includes('application/diag-layer')) {
      e.preventDefault()
      setIsDraggedOver(true)
    }
  }

  const handleDragLeave = () => {
    setIsDraggedOver(false)
  }

  const handleDrop = () => {
    setIsDraggedOver(false)
  }

  const hasParent = data.parentLinks.length > 0 || !!data.parentViewId
  const hasChild = data.links.length > 0 || data.has_view

  const handleZoomOutClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    if (data.parentLinks.length > 1) {
      menuRef.current = { type: 'out', links: data.parentLinks }
      setMenuVisible(true)
    } else {
      data.onZoomOut(data.element_id)
    }
  }

  const handleZoomInClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    if (data.links.length > 1) {
      menuRef.current = { type: 'in', links: data.links }
      setMenuVisible(true)
    } else {
      data.onZoomIn(data.element_id)
    }
  }

  const parentLabel = data.parentLinks.length > 1
    ? `${data.parentLinks.length} views`
    : hasParent
      ? data.parentLinks.length > 0
        ? `Zoom out → ${data.parentLinks[0].to_view_name}`
        : 'Zoom out to parent'
      : ''

  const childLabel = data.links.length > 1
    ? `${data.links.length} sub-views`
    : hasChild
      ? data.links.length > 0
        ? `Zoom in → ${data.links[0].to_view_name}`
        : `Zoom in → ${data.view_label || 'sub-view'}`
      : 'Create child view'

  // ── Long-press-to-connect ──────────────────────────────────────────────────
  const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const longPressActivated = useRef(false)
  const pointerStart = useRef<{ x: number; y: number } | null>(null)

  const onPointerDown = (e: React.PointerEvent) => {
    if ((e.target as Element).closest('.react-flow__handle')) return
    pointerStart.current = { x: e.clientX, y: e.clientY }
    longPressActivated.current = false
    longPressTimer.current = setTimeout(() => {
      longPressActivated.current = true
      data.onInteractionStart(data.element_id)
    }, 500)
  }

  const onPointerMove = (e: React.PointerEvent) => {
    if (!pointerStart.current || !longPressTimer.current) return
    const dx = e.clientX - pointerStart.current.x
    const dy = e.clientY - pointerStart.current.y
    if (Math.hypot(dx, dy) > 8) {
      clearTimeout(longPressTimer.current)
      longPressTimer.current = null
    }
  }

  const onPointerUp = () => {
    if (longPressTimer.current) {
      clearTimeout(longPressTimer.current)
      longPressTimer.current = null
    }
  }

  const handleBodyClick = (e: React.MouseEvent) => {
    if (longPressActivated.current) {
      longPressActivated.current = false
      return
    }
    if (data.interactionSourceId) {
      e.stopPropagation()
      if (data.interactionSourceId === data.element_id) {
        // tap source again → cancel interaction mode
        data.onInteractionStart(data.element_id)
      } else {
        // tap a different element → create connector
        data.onConnectTo(data.element_id)
      }
    } else {
      data.onSelect(data)
    }
  }

  const isSource = data.interactionSourceId === data.element_id
  const isTarget = !!data.interactionSourceId && !isSource

  const bodyCursor = isSource ? 'crosshair' : isTarget ? 'cell' : 'pointer'

  return (
    <ElementContainer
      isSelected={selected}
      isSource={isSource}
      isTarget={isTarget}
      isConnectorHighlighted={!!data.isConnectorHighlighted}
      hasStack={hasChild}
      kind={data.kind}
      minW="180px"
      maxW="230px"
      cursor={bodyCursor}
      outline={isDraggedOver ? '2px solid' : undefined}
      outlineColor={isDraggedOver ? 'var(--accent)' : undefined}
      outlineOffset={isDraggedOver ? '2px' : undefined}
      borderTopWidth={data.layerHighlightColor ? '2px' : undefined}
      borderTopColor={data.layerHighlightColor ?? undefined}
      onClick={handleBodyClick}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onContextMenu={(e) => e.preventDefault()}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      style={{
        userSelect: 'none',
        WebkitUserSelect: 'none',
        transition: 'outline 0.15s, outline-color 0.15s',
      } as React.CSSProperties}
    >
      {HANDLE_CONFIGS.flatMap(({ side, position }) =>
        HANDLE_SLOTS.map((slot) => {
          const handleId = getVisualHandleId(side, slot)
          const isFallbackSlot = slot === HANDLE_SLOT_CENTER_INDEX && !activeSides.has(side)
          const className = [
            'element-node-handle',
            connectedHandleIds.has(handleId) ? 'handle-active-slot' : '',
            isFallbackSlot ? 'handle-fallback-slot' : '',
            connectedHandleIds.has(handleId) ? 'handle-connected' : '',
            selectedHandleIds.has(handleId) ? 'handle-selected-edge' : '',
          ].filter(Boolean).join(' ') || undefined

          return (
            <Box key={handleId} position="absolute" inset={0} pointerEvents="none">
              <Handle
                type="source"
                position={position}
                id={handleId}
                className={className}
                onClick={(e: React.MouseEvent) => {
                  e.preventDefault()
                  e.stopPropagation()
                  data.onInteractionStart(data.element_id, {
                    sourceHandle: handleId,
                    clientX: e.clientX,
                    clientY: e.clientY,
                  })
                }}
                style={{
                  ...getVisualHandleStyle(position, slot),
                  background: 'var(--accent)',
                }}
              />
              {data.onStartHandleReconnect && reconnectCandidateByHandle.has(handleId) && (
                <Box
                  position="absolute"
                  className="element-node-reconnect-zone"
                  style={getReconnectZoneStyle(position, slot)}
                  pointerEvents="auto"
                  cursor="grab"
                  zIndex={4}
                  onPointerDown={(e: React.PointerEvent) => {
                    if (e.button !== 0) return
                    e.preventDefault()
                    e.stopPropagation()
                    const candidate = reconnectCandidateByHandle.get(handleId)
                    if (!candidate) return
                    data.onStartHandleReconnect?.({
                      edgeId: candidate.edgeId,
                      endpoint: candidate.endpoint,
                      handleId,
                      clientX: e.clientX,
                      clientY: e.clientY,
                    })
                  }}
                />
              )}
            </Box>
          )
        }),
      )}

      {/* ── Header: zoom-out | zoom-in (absolute overlay, consumes no space) ── */}
      <Flex
        position="absolute"
        top={1.5}
        left={2}
        right={2}
        align="center"
        justify="space-between"
        gap={1}
        zIndex={1}
        pointerEvents="none"
      >
        <LayerButton
          label={parentLabel}
          active={hasParent}
          variant="out"
          count={data.parentLinks.length}
          onClick={handleZoomOutClick}
          onMouseEnter={() => data.onHoverZoom(data.element_id, 'out')}
          onMouseLeave={() => data.onHoverZoom(data.element_id, null)}
          isDisabled={data.isCanvasMoving}
        >
          <ZoomOutIcon size={12} strokeWidth={2.5} />
        </LayerButton>

        <Flex flex={1} justify="center" align="center" minW={0} pointerEvents="none">
          {nodeLogoUrl && (
            <Box
              as="img"
              src={nodeLogoUrl}
              alt="technology icon"
              maxW="28px"
              maxH="28px"
              objectFit="contain"
              opacity={0.95}
            />
          )}
        </Flex>

        <LayerButton
          label={childLabel}
          active={hasChild}
          variant="in"
          count={data.links.length}
          onClick={handleZoomInClick}
          onMouseEnter={() => data.onHoverZoom(data.element_id, 'in')}
          onMouseLeave={() => data.onHoverZoom(data.element_id, null)}
          isDisabled={data.isCanvasMoving}
        >
          <ZoomInIcon size={12} strokeWidth={2.5} />
        </LayerButton>
      </Flex>

      {/* ── Zoom hover effect ── */}
      {data.isZoomHovered && (
        <Box
          position="absolute"
          inset="-3px"
          rounded="xl"
          border="1.5px solid"
          borderColor={data.isZoomHovered === 'in' ? 'teal.500' : 'blue.500'}
          boxShadow={`0 0 12px ${data.isZoomHovered === 'in' ? 'rgba(20, 184, 166, 0.2)' : 'rgba(59, 130, 246, 0.2)'}`}
          pointerEvents="none"
          zIndex={-1}
          animation="zoom-quiet-breath 3s ease-in-out infinite"
          sx={{
            '@keyframes zoom-quiet-breath': {
              '0%': { opacity: 0.3, transform: 'scale(1)' },
              '50%': { opacity: 0.6, transform: 'scale(1.01)' },
              '100%': { opacity: 0.3, transform: 'scale(1)' },
            }
          }}
        />
      )}

      {/* ── Body: name vertically centred, long-press to connect, click to edit ── */}
      <ElementBody
        name={data.name}
        type={data.kind ?? ''}
        technology={technologyText}
        logoUrl={undefined}
        nameSize="xl"
        minH="85px"
        pt={nodeLogoUrl ? 9 : 2}
        pb={2}
      />

      {/* Tags Dots & Hover Overlay */}
      {data.tags && data.tags.length > 0 && (
        <Box
          position="absolute"
          bottom="8px"
          left="8px"
          zIndex={10}
          role="group"
        >
          {/* Tag Dots (up to 5) */}
          <HStack spacing={1} _groupHover={{ opacity: 0 }}>
            {data.tags.slice(0, 5).map((tag, i) => (
              <Box
                key={i}
                w="6px"
                h="6px"
                rounded="full"
                bg={data.tagColors[tag]?.color || 'var(--accent)'}
                boxShadow="0 0 3px rgba(0,0,0,0.3)"
                transition="all 0.2s"
              />
            ))}
            {data.tags.length > 5 && (
              <Text fontSize="8px" fontWeight="bold" color="whiteAlpha.600" lineHeight={1}>
                +{data.tags.length - 5}
              </Text>
            )}
          </HStack>

          {/* Hover Menu: Expanding tags at -45 degree angle */}
          <VStack
            position="absolute"
            bottom="-4px"
            left="-4px"
            align="start"
            spacing={1}
            opacity={data.forceShowTagPopup && !data.isCanvasMoving ? 1 : 0}
            visibility={data.forceShowTagPopup && !data.isCanvasMoving ? 'visible' : 'hidden'}
            transform={data.forceShowTagPopup && !data.isCanvasMoving ? 'scale(1) translate(0px, 0px)' : 'scale(0.2) translate(10px, 10px)'}
            transformOrigin="bottom left"
            transition="all 0.25s cubic-bezier(0.175, 0.885, 0.32, 1.275)"
            _groupHover={{ opacity: 1, visibility: 'visible', transform: 'scale(1) translate(0px, 0px)' }}
            pointerEvents="none"
          >
            {data.tags.map((tag) => (
              <Box
                key={tag}
                bg="var(--bg-panel)"
                border="1px solid"
                borderColor="whiteAlpha.300"
                rounded="full"
                px={2}
                py="1px"
                boxShadow="0 4px 12px rgba(0,0,0,0.5)"
                whiteSpace="nowrap"
              >
                <HStack spacing={1.5}>
                  <Box w="6px" h="6px" rounded="full" bg={data.tagColors[tag]?.color || 'var(--accent)'} />
                  <Text fontSize="10px" fontWeight="700" color="white">{tag}</Text>
                </HStack>
              </Box>
            ))}
          </VStack>
        </Box>
      )}

      {/* Code Preview Icon/Link in Bottom Right Corner */}
      {((data.repo || data.url) && !window.__TLD_VSCODE__) && (
        <Box
          position="absolute"
          bottom="8px"
          right="8px"
          zIndex={10}
        >
          <Tooltip
            label={
              data.repo
                ? `View source: ${data.file_path?.includes('#') ? (() => { try { return JSON.parse(data.file_path.split('#')[1]).name } catch { return 'Link' } })() : 'Link'}${data.url ? ' / URL' : ''}`
                : 'Open Link'
            }
            placement="top"
            isDisabled={data.isCanvasMoving}
          >
            <Box
              as="button"
              display="flex"
              alignItems="center"
              justifyContent="center"
              w="18px"
              h="18px"
              rounded="md"
              color="whiteAlpha.900"
              _hover={{ color: 'blue.300', bg: 'whiteAlpha.200', transform: 'scale(1.1)' }}
              transition="all 0.15s"
              onClick={(e: React.MouseEvent) => {
                e.stopPropagation()
                if (data.repo) {
                  data.onOpenCodePreview?.(data.element_id)
                } else if (data.url) {
                  window.open(data.url, '_blank', 'noopener,noreferrer')
                }
              }}
              onPointerDown={(e: React.PointerEvent) => e.stopPropagation()}
            >
              <LinkIcon w={2.5} h={2.5} />
            </Box>
          </Tooltip>
        </Box>
      )}

      {/* VSCode specific file link with hover preview */}
      {window.__TLD_VSCODE__ && data.file_path && (
        <Box
          position="absolute"
          bottom="8px"
          right="8px"
          zIndex={10}
        >
          <VscodeCodePreview
            filePath={data.file_path}
            isCanvasMoving={data.isCanvasMoving}
          />
        </Box>
      )}

      {selected && !isSource && (
        <HStack
          position="absolute"
          top="-20px"
          left="0"
          right="0"
          spacing={0}
          justify="space-evenly"
          pointerEvents="none"
          zIndex={10}
          opacity={data.isCanvasMoving ? 0 : 0.6}
          transition="opacity 0.2s"
        >
          <HStack spacing={1.5}>
            <Text color="whiteAlpha.600" fontSize="8px" fontWeight="bold">E</Text>
            <Text color="whiteAlpha.400" fontSize="8px">Connect</Text>
          </HStack>
          <HStack spacing={1.5}>
            <Text color="whiteAlpha.600" fontSize="8px" fontWeight="bold">R</Text>
            <Text color="whiteAlpha.400" fontSize="8px">Remove</Text>
          </HStack>
          <HStack spacing={1.5}>
            <Text color="whiteAlpha.600" fontSize="8px" fontWeight="bold">⇧R</Text>
            <Text color="whiteAlpha.400" fontSize="8px">Delete</Text>
          </HStack>
          <HStack spacing={1.5}>
            <Text color="whiteAlpha.600" fontSize="8px" fontWeight="bold">T</Text>
            <Text color="whiteAlpha.400" fontSize="8px">Tech</Text>
          </HStack>
        </HStack>
      )}

      {/* Interaction-mode menu + badge on the source element */}
      {isSource && !data.isCanvasMoving && (
        <>
          {!data.isClickConnectMode && (
            <Box
              position="absolute"
              bottom="calc(100% + 12px)"
              left="50%"
              transform={`translateX(-50%) scale(${1 / zoom})`}
              transformOrigin="bottom center"
              bg="clay.bg"
              border="1px solid"
              borderColor="rgba(255,255,255,0.08)"
              rounded="xl"
              boxShadow="clay-out"
              p={1}
              zIndex={100}
              onClick={(e) => e.stopPropagation()}
              onPointerDown={(e) => e.stopPropagation()}
            >
              <VStack spacing={0} align="stretch">
                <Button
                  size="sm"
                  variant="ghost"
                  h="30px"
                  px={2.5}
                  justifyContent="flex-start"
                  color="clay.text"
                  _hover={{ bg: 'whiteAlpha.100', color: 'gray.100' }}
                  _active={{ bg: 'whiteAlpha.200' }}
                  onClick={(e) => {
                    e.stopPropagation()
                    data.onSelect(data)
                  }}
                >
                  <HStack spacing={2} w="full">
                    <EditSvg />
                    <Text fontSize="xs" fontWeight="normal" flex={1}>Edit</Text>
                  </HStack>
                </Button>
                <Divider borderColor="whiteAlpha.100" my={1} />
                <Button
                  size="sm"
                  variant="ghost"
                  h="30px"
                  px={2.5}
                  justifyContent="flex-start"
                  color="red.400"
                  _hover={{ bg: 'rgba(254,178,178,0.08)', color: 'red.300' }}
                  _active={{ bg: 'rgba(254,178,178,0.12)' }}
                  onClick={(e) => {
                    e.stopPropagation()
                    data.onRemove(data.element_id)
                  }}
                >
                  <HStack spacing={2} w="full">
                    <TrashSvg />
                    <Text fontSize="xs" fontWeight="normal" flex={1}>Delete</Text>
                  </HStack>
                </Button>
              </VStack>
            </Box>
          )}

          {/* interaction mode */}
          <Box
            position="absolute"
            bottom="-22px"
            left="50%"
            transform={`translateX(-50%) scale(${1 / zoom})`}
            transformOrigin="top center"
            bg="clay.in"
            color="clay.dim"
            border="1px solid"
            borderColor="rgba(255,255,255,0.06)"
            fontSize="9px"
            px={2}
            py="2px"
            rounded="full"
            whiteSpace="nowrap"
            pointerEvents="none"
            zIndex={10}
          >
            tap element to connect · tap canvas to add
          </Box>
        </>
      )}

      {/* Drill-down menu for multiple connectors */}
      {menuVisible && menuRef.current && !data.isCanvasMoving && (
        <Box
          position="absolute"
          top={menuRef.current.type === 'in' ? '30px' : '-2px'}
          left={menuRef.current.type === 'in' ? 'auto' : '2px'}
          right={menuRef.current.type === 'in' ? '2px' : 'auto'}
          transform={`scale(${1 / zoom})`}
          transformOrigin={menuRef.current.type === 'in' ? 'top right' : 'top left'}
          bg="clay.bg"
          border="1px solid"
          borderColor="rgba(255,255,255,0.08)"
          rounded="xl"
          boxShadow="clay-out"
          p={1}
          zIndex={1100}
          onClick={(e) => e.stopPropagation()}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <VStack spacing={0} align="stretch" minW="140px">
            <Box px={2.5} py={1.5}>
              <Text fontSize="10px" fontWeight="bold" color="gray.500" textTransform="uppercase">
                {menuRef.current.type === 'in' ? 'Sub-views' : 'Parent Views'}
              </Text>
            </Box>
            <Divider borderColor="whiteAlpha.100" mb={1} />
            {menuRef.current.links.map((link) => (
              <Button
                key={link.id}
                size="sm"
                variant="ghost"
                h="32px"
                px={2.5}
                justifyContent="flex-start"
                color="clay.text"
                _hover={{ bg: 'whiteAlpha.100', color: 'gray.100' }}
                _active={{ bg: 'whiteAlpha.200' }}
                onClick={(e) => {
                  e.stopPropagation()
                  setMenuVisible(false)
                  if (menuRef.current?.type === 'in') {
                    data.onNavigateToDiagram(link.to_view_id)
                  } else {
                    data.onNavigateToDiagram(link.from_view_id)
                  }
                }}
              >
                <HStack spacing={2} w="full">
                  <Box color={menuRef.current?.type === 'in' ? 'teal.400' : 'blue.400'}>
                    {menuRef.current?.type === 'in' ? <ZoomInIcon size={11} strokeWidth={3} /> : <ZoomOutIcon size={11} strokeWidth={3} />}
                  </Box>
                  <Text fontSize="xs" fontWeight="normal" flex={1} isTruncated>
                    {link.to_view_name}
                  </Text>
                </HStack>
              </Button>
            ))}
            <Divider borderColor="whiteAlpha.100" my={1} />
            <Button
              size="xs"
              variant="ghost"
              onClick={(e) => {
                e.stopPropagation()
                setMenuVisible(false)
              }}
            >
              Cancel
            </Button>
          </VStack>
        </Box>
      )}
    </ElementContainer>
  )
}

function arePropsEqual(prev: Props, next: Props) {
  if (prev.selected !== next.selected) return false
  const p = prev.data
  const n = next.data
  return (
    p.element_id === n.element_id &&
    p.name === n.name &&
    p.description === n.description &&
    p.kind === n.kind &&
    p.technology === n.technology &&
    p.logo_url === n.logo_url &&
    p.links === n.links &&
    p.parentLinks === n.parentLinks &&
    p.isZoomHovered === n.isZoomHovered &&
    p.interactionSourceId === n.interactionSourceId &&
    p.onZoomIn === n.onZoomIn &&
    p.onZoomOut === n.onZoomOut &&
    p.onSelect === n.onSelect &&
    p.onHoverZoom === n.onHoverZoom &&
    p.isCanvasMoving === n.isCanvasMoving &&
    p.onRemove === n.onRemove &&
    p.onInteractionStart === n.onInteractionStart &&
    p.onConnectTo === n.onConnectTo &&
    p.onStartHandleReconnect === n.onStartHandleReconnect &&
    p.onNavigateToDiagram === n.onNavigateToDiagram &&
    p.technology_connectors === n.technology_connectors &&
    p.repo === n.repo &&
    p.branch === n.branch &&
    p.file_path === n.file_path &&
    p.language === n.language &&
    p.isClickConnectMode === n.isClickConnectMode &&
    p.tagColors === n.tagColors &&
    p.layerHighlightColor === n.layerHighlightColor &&
    p.forceShowTagPopup === n.forceShowTagPopup
  )
}

export default memo(ElementNode, arePropsEqual)
