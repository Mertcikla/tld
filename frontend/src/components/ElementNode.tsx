import { memo, useCallback, useEffect, useMemo, useReducer, useRef, useState } from 'react'
import { Handle, Position, useStore, type ReactFlowState } from 'reactflow'
import { Box, Flex, Text, Tooltip, HStack, Button, Divider, Input, VStack, Portal, Popover, PopoverTrigger, PopoverContent, PopoverBody } from '@chakra-ui/react'
import { LinkIcon } from '@chakra-ui/icons'

import type { LibraryElement, PlacedElement, ViewConnector, Tag } from '../types'
import { ElementContainer } from './NodeContainer'
import { ElementBody } from './NodeBody'
import SubElementNode from './SubElementNode'
import SourceLinkActions from './SourceLinkActions'
import ElementTags from './ElementTags'
import { subElementKind } from '../utils/subElements'
import { elementTechnologyText, resolveElementIconUrl } from '../utils/elementIcon'
import { ZoomInIcon, ZoomOutIcon, TrashIcon as TrashSvg, DrawIcon as EditSvg } from './Icons'
import { KbdHint, ShortcutHint } from './PanelUI'
import { vscodeBridge } from '../lib/vscodeBridge'
import { openExternalUrl } from '../lib/desktop'
import { parseSourceLink, sourceAnchorLabel, sourceLinkKind } from '../utils/sourceLinks'
import type { ExtensionToWebviewMessage } from '../types/vscode-messages'
import { useElementSearch } from '../hooks/useElementSearch'
import ElementCreateSearchResults from './ElementCreateSearchResults'
import { buildElementCreateSearchResults } from './elementCreateSearch'
import {
  getVisualHandleId,
  getVisualHandleStyle,
  HANDLE_SLOT_CENTER_INDEX,
  HANDLE_SLOT_COUNT,
} from '../utils/edgeDistribution'
import {
  ELEMENT_NAME_FONT_STEPS,
  ELEMENT_NAME_INSET_RATIO,
  ELEMENT_NAME_LINE_HEIGHT,
  fitElementName,
  measureTextWithCanvas,
  subscribeElementNameFontsChanged,
} from '../utils/elementName'

const ELEMENT_NODE_WIDTH = 180
const ELEMENT_NODE_HEIGHT = 85
const ELEMENT_NODE_BORDER = 1
const ELEMENT_NAME_BODY_TOP = 36
const ELEMENT_NAME_BODY_TOP_COMPACT = 8
const ELEMENT_NAME_BODY_BOTTOM = 8
const ELEMENT_NAME_TYPE_LINE = 11 * 1.1
const ELEMENT_NAME_TECH_LINE = 12 * 1.1
const ELEMENT_BODY_GAP = 4
const ELEMENT_NAME_MIN_FONT = 13
const ELEMENT_NAME_FONT_SIZES = ELEMENT_NAME_FONT_STEPS.filter((size) => size <= 18)

function computeElementNameFit(
  name: string,
  kind: string | null | undefined,
  hasLogo: boolean,
  hasTechnology: boolean,
) {
  const maxWidth = (ELEMENT_NODE_WIDTH - ELEMENT_NODE_BORDER * 2) * (1 - ELEMENT_NAME_INSET_RATIO)
  const bodyTop = hasLogo ? ELEMENT_NAME_BODY_TOP : ELEMENT_NAME_BODY_TOP_COMPACT
  let band = ELEMENT_NODE_HEIGHT - ELEMENT_NODE_BORDER * 2 - bodyTop - ELEMENT_NAME_BODY_BOTTOM
  if (kind) band -= ELEMENT_NAME_TYPE_LINE + ELEMENT_BODY_GAP
  if (hasTechnology) band -= ELEMENT_NAME_TECH_LINE + ELEMENT_BODY_GAP
  const hasRoomForBand = band >= ELEMENT_NAME_MIN_FONT * ELEMENT_NAME_LINE_HEIGHT
  return fitElementName({
    name,
    maxWidth,
    maxLines: 2,
    fontSizes: ELEMENT_NAME_FONT_SIZES,
    lineHeight: ELEMENT_NAME_LINE_HEIGHT,
    bandHeight: hasRoomForBand ? band : undefined,
    measureKey: 'editor',
    measure: measureTextWithCanvas,
  })
}

function VscodeCodePreview({
  filePath,
  fallbackSymbolName,
  fallbackSymbolKind,
  isCanvasMoving,
}: {
  filePath: string
  fallbackSymbolName?: string
  fallbackSymbolKind?: string | null
  isCanvasMoving?: boolean
}) {
  const [content, setContent] = useState<string | null>(null)
  const [startLineOffset, setStartLineOffset] = useState(0)
  const [isOpen, setIsOpen] = useState(false)
  const hoverTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const { basePath: relPath, anchor } = useMemo(() => parseSourceLink(filePath), [filePath])
  const isFileLink = sourceLinkKind(filePath) === 'file'
  const startLine = anchor.kind === 'line' ? anchor.startLine : undefined
  const symbolName = isFileLink ? anchor.kind === 'symbol' ? anchor.symbolName : fallbackSymbolName : undefined
  const symbolKind = isFileLink ? anchor.kind === 'symbol' ? anchor.nodeType : fallbackSymbolKind ?? undefined : undefined

  useEffect(() => {
    if (!isOpen || !isFileLink) return
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
  }, [isOpen, relPath, startLine, isFileLink])

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
        !isFileLink ? (
          <Text>Folder: {relPath}</Text>
        ) : content ? (
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

export interface NodeData extends PlacedElement {
  links: ViewConnector[]        // child (zoom-in) connectors
  parentLinks: ViewConnector[]  // parent (zoom-out) connectors
  parentViewId?: number | null
  onZoomIn: (elementId: number) => void
  onZoomOut: (elementId: number) => void
  onNavigateToDiagram: (viewId: number) => void
  onResize?: (elementId: number, width: number, height: number) => Promise<void>
  onSelect: (obj: PlacedElement) => void
  onInteractionStart: (elementId: number, options?: { sourceHandle?: string; clientX?: number; clientY?: number }) => void
  onConnectTo: (elementId: number) => void
  onStartHandleReconnect?: (args: { edgeId: string; endpoint: 'source' | 'target'; handleId: string; clientX: number; clientY: number; pointerId: number }) => void
  onRemove: (elementId: number) => void
  onHoverZoom: (elementId: number, type: 'in' | 'out' | null) => void
  isZoomHovered: 'in' | 'out' | null
  interactionSourceHandleId?: string | null
  interactionSourceId: number | null
  isClickConnectMode?: boolean
  isCreateConnectMode?: boolean
  tagColors: Record<string, Tag>
  layerHighlightColor?: string
  forceShowTagPopup?: boolean
  isCanvasMoving?: boolean
  isPendingElement?: boolean
  connectedHandleIds?: readonly string[]
  tagHiddenHandleIds?: readonly string[]
  selectedHandleIds?: readonly string[]
  reconnectCandidates?: readonly { handleId: string; edgeId: string; endpoint: 'source' | 'target'; selected: boolean }[]
  isConnectorHighlighted?: boolean
  isMultiSelected?: boolean
  pendingCreate?: PendingElementCreateData
}

export interface PendingElementCreateData {
  allElements: LibraryElement[]
  existingElementIds: Set<number>
  allowCreate?: boolean
  connectionMode?: 'add' | 'connect'
  onConnectionModeChange?: (mode: 'add' | 'connect') => void
  getSecondaryLabel?: (obj: LibraryElement) => string | null
  getDisabledReason?: (obj: LibraryElement) => string | null
  onConfirmNew: (name: string) => Promise<void>
  onConfirmExisting: (obj: LibraryElement) => Promise<void>
  onCancel: () => void
}

interface Props {
  data: NodeData
  selected: boolean
  onOpenSource?: () => void
  embeddedCard?: boolean
}

// Flat icon button used for zoom-in / zoom-out.
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

  return (
    <Tooltip label={label} placement="top" openDelay={300} isDisabled={isDisabled}>
      <Box
        data-testid={variant === 'in' ? 'vieweditor-node-zoom-in' : 'vieweditor-node-zoom-out'}
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
        boxShadow="none"
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

type PendingInputRect = {
  left: number
  top: number
  width: number
  height: number
  viewportWidth: number
  viewportHeight: number
}

function pendingInputRectsEqual(a: PendingInputRect | null, b: PendingInputRect) {
  return !!a &&
    a.left === b.left &&
    a.top === b.top &&
    a.width === b.width &&
    a.height === b.height &&
    a.viewportWidth === b.viewportWidth &&
    a.viewportHeight === b.viewportHeight
}

function getReconnectZoneStyle(position: Position, slot: number, selected: boolean): React.CSSProperties {
  const baseStyle = getVisualHandleStyle(position, slot)
  if (selected) {
    const offset = { [position]: '-22px' }
    return {
      ...baseStyle, ...offset, width: '44px', height: '44px',
      transform: position === Position.Top || position === Position.Bottom ? 'translateX(-50%)' : 'translateY(-50%)',
    }
  }

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

function PendingElementLabelEditor({
  config,
}: {
  config: PendingElementCreateData
}) {
  const { query, setQuery, remoteElements, fetching, searchedQuery } = useElementSearch()
  const [activeIndex, setActiveIndex] = useState(0)
  const [busy, setBusy] = useState(false)
  const [inputRect, setInputRect] = useState<PendingInputRect | null>(null)
  const anchorRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const didAutoSwitchMode = useRef(false)
  const results = buildElementCreateSearchResults({
    query,
    allElements: config.allElements,
    remoteElements,
    allowCreate: config.connectionMode !== 'connect' && (config.allowCreate ?? true),
  })
  const inputWidth = `clamp(156px, ${Math.max(query.length + 1, 2)}ch, 206px)`

  const updateInputRect = useCallback(() => {
    const anchor = anchorRef.current
    const node = anchor?.closest('[data-testid="vieweditor-node"]')
    if (typeof HTMLElement === 'undefined' || !(node instanceof HTMLElement)) return

    const rect = node.getBoundingClientRect()
    const next = {
      left: Math.round(rect.left),
      top: Math.round(rect.top),
      width: Math.round(rect.width),
      height: Math.round(rect.height),
      viewportWidth: window.innerWidth,
      viewportHeight: window.innerHeight,
    }
    setInputRect((current) => pendingInputRectsEqual(current, next) ? current : next)
  }, [])

  const focusInput = useCallback(() => {
    updateInputRect()
    inputRef.current?.focus({ preventScroll: true })
  }, [updateInputRect])

  useEffect(() => {
    updateInputRect()
    if (typeof window === 'undefined') return

    window.addEventListener('resize', updateInputRect)
    window.addEventListener('scroll', updateInputRect, true)
    return () => {
      window.removeEventListener('resize', updateInputRect)
      window.removeEventListener('scroll', updateInputRect, true)
    }
  }, [updateInputRect])

  useEffect(() => {
    if (!inputRect) return
    focusInput()
    const raf = typeof requestAnimationFrame === 'function' ? requestAnimationFrame(focusInput) : null
    const timers = [0, 50, 100, 200, 350].map((delay) => setTimeout(focusInput, delay))
    return () => {
      if (raf !== null) cancelAnimationFrame(raf)
      timers.forEach(clearTimeout)
    }
  }, [focusInput, inputRect])

  useEffect(() => { setActiveIndex(0) }, [query, config.connectionMode])

  useEffect(() => {
    if (
      didAutoSwitchMode.current ||
      config.connectionMode !== 'connect' ||
      !config.onConnectionModeChange ||
      config.allowCreate === false ||
      busy || fetching ||
      !query.trim() || searchedQuery !== query.trim() ||
      results.length > 0
    ) return

    didAutoSwitchMode.current = true
    config.onConnectionModeChange('add')
  }, [busy, fetching, query, searchedQuery, results.length, config])

  const confirm = useCallback(async (idx: number) => {
    const item = results[idx]
    if (!item || busy) return
    if (item.kind === 'existing' && config.getDisabledReason?.(item.obj)) return
    setBusy(true)
    try {
      if (item.kind === 'new') {
        await config.onConfirmNew(item.label)
      } else {
        await config.onConfirmExisting(item.obj)
      }
    } finally {
      setBusy(false)
    }
  }, [busy, config, results])

  const onKeyDown = useCallback((e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      config.onCancel()
      return
    }
    if (e.key === 'Enter') {
      e.preventDefault()
      e.stopPropagation()
      void confirm(activeIndex)
      return
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      e.stopPropagation()
      setActiveIndex((i) => Math.min(i + 1, results.length - 1))
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      e.stopPropagation()
      setActiveIndex((i) => Math.max(i - 1, 0))
    }
  }, [activeIndex, config, confirm, results.length])

  const resultsWidth = inputRect ? Math.min(340, inputRect.viewportWidth - 16) : undefined
  const resultsOnRight = !inputRect || !resultsWidth || inputRect.left + inputRect.width + 14 + resultsWidth <= inputRect.viewportWidth - 8
  const resultsAbove = inputRect && !resultsOnRight && inputRect.top > inputRect.viewportHeight / 2
  const resultsMaxHeight = inputRect && !resultsOnRight
    ? Math.min(300, resultsAbove ? inputRect.top - 16 : inputRect.viewportHeight - inputRect.top - inputRect.height - 16)
    : 300

  return (
    <Box
      ref={anchorRef}
      position="relative"
      display="inline-flex"
      justifyContent="center"
      maxW="100%"
      minW={0}
      cursor="text"
      onPointerDown={(e) => e.stopPropagation()}
      onClick={(e) => {
        e.stopPropagation()
        focusInput()
      }}
    >
      <Text
        textAlign="center"
        fontSize="xl"
        fontWeight="semibold"
        lineHeight={1.15}
        color="gray.100"
        opacity={query ? 1 : 0}
        minW={0}
        w={inputWidth}
        maxW="100%"
        px={1}
        py={0}
        h="32px"
        display="flex"
        alignItems="center"
        justifyContent="center"
        noOfLines={1}
        pointerEvents="none"
      >
        {query || ' '}
      </Text>
      {inputRect && (
        <Portal>
          <Input
            data-testid="pending-element-label-input"
            aria-label="Element name"
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
            variant="unstyled"
            textAlign="center"
            fontSize="xl"
            fontWeight="semibold"
            lineHeight={1.15}
            color="transparent"
            position="fixed"
            left={`${inputRect.left}px`}
            top={`${inputRect.top}px`}
            zIndex={2400}
            minW={0}
            w={`${inputRect.width}px`}
            maxW="none"
            border={0}
            boxShadow="none"
            bg="transparent"
            px={1}
            py={0}
            h={`${inputRect.height}px`}
            isDisabled={busy}
            autoComplete="off"
            pointerEvents="auto"
            onPointerDown={(e) => {
              e.stopPropagation()
              updateInputRect()
            }}
            onClick={(e) => e.stopPropagation()}
            sx={{
              caretColor: 'var(--accent)',
              touchAction: 'auto',
              userSelect: 'text',
              WebkitUserSelect: 'text',
              WebkitAppearance: 'none',
            }}
          />
        </Portal>
      )}
      {inputRect && (results.length > 0 || config.connectionMode) && (
        <Portal>
          <Box
            data-testid="pending-element-results"
            position="fixed"
            left={`${resultsOnRight ? inputRect.left + inputRect.width + 14 : Math.max(8, Math.min(inputRect.left, inputRect.viewportWidth - (resultsWidth ?? 340) - 8))}px`}
            top={`${resultsOnRight ? inputRect.top + inputRect.height / 2 : resultsAbove ? inputRect.top - 8 : inputRect.top + inputRect.height + 8}px`}
            transform={resultsOnRight ? 'translateY(-50%)' : resultsAbove ? 'translateY(-100%)' : undefined}
            zIndex={2401}
            bg="var(--bg-panel)"
            border="1px solid"
            borderColor="whiteAlpha.100"
            rounded="xl"
            shadow="0 8px 32px rgba(0,0,0,0.5)"
            minW="220px"
            maxW={resultsWidth ? `${resultsWidth}px` : '340px'}
            w="max-content"
            maxH={`${Math.max(80, resultsMaxHeight)}px`}
            overflowY="auto"
            pointerEvents="auto"
            onPointerDown={(e) => e.stopPropagation()}
            onClick={(e) => e.stopPropagation()}
          >
            {config.connectionMode && (
              <HStack px={1.5} py={1.5} spacing={1} borderBottom="1px solid" borderColor="whiteAlpha.100" role="group" aria-label="Target placement">
                {(['add', 'connect'] as const).map((mode) => (
                  <Tooltip
                    key={mode}
                    label={mode === 'connect' ? 'Connect without adding the target to this view.' : 'Add the target to this view and connect it.'}
                    placement="top"
                    openDelay={300}
                  >
                    <Button
                      data-testid={`pending-element-mode-${mode}`}
                      size="xs"
                      h="24px"
                      flex={1}
                      variant="ghost"
                      aria-pressed={config.connectionMode === mode}
                      isDisabled={busy}
                      bg={config.connectionMode === mode ? 'rgba(var(--accent-rgb), 0.16)' : 'transparent'}
                      color={config.connectionMode === mode ? 'var(--accent)' : 'gray.300'}
                      _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)' }}
                      onClick={() => {
                        config.onConnectionModeChange?.(mode)
                        focusInput()
                      }}
                    >
                      {mode === 'connect' ? 'Connect only' : 'Add & connect'}
                    </Button>
                  </Tooltip>
                ))}
              </HStack>
            )}
            <ElementCreateSearchResults
              results={results}
              activeIndex={activeIndex}
              busy={busy}
              query={query}
              existingElementIds={config.existingElementIds}
              testIdPrefix="pending-element"
              getSecondaryLabel={config.getSecondaryLabel}
              getDisabledReason={config.getDisabledReason}
              onActiveIndexChange={setActiveIndex}
              onConfirm={(idx) => { void confirm(idx) }}
            />
            {results.length === 0 && config.connectionMode === 'connect' && (
              <Text px={3} py={2} fontSize="xs" color="gray.400">
                {fetching ? 'Searching…' : query.trim() ? 'No matching elements.' : 'Search for an existing element.'}
              </Text>
            )}
          </Box>
        </Portal>
      )}
    </Box>
  )
}

function ElementNode({ data, selected, embeddedCard, onOpenSource }: Props) {
  const zoom = useStore(zoomSelector)
  const hoveredHandleKey = useStore(useCallback((state: ReactFlowState) => {
    const nodeId = String(data.element_id)
    const handles = new Set<string>()
    for (const edge of state.edges) {
      if (!edge.data?.isHovered || edge.hidden || edge.style?.pointerEvents === 'none') continue
      if (edge.source === nodeId && edge.sourceHandle) handles.add(edge.sourceHandle)
      if (edge.target === nodeId && edge.targetHandle) handles.add(edge.targetHandle)
    }
    return [...handles].sort().join('|')
  }, [data.element_id]))
  const hoveredHandleIds = useMemo(() => new Set(hoveredHandleKey.split('|')), [hoveredHandleKey])
  const isPending = !!data.isPendingElement || !!data.pendingCreate

  const connectedHandleIds = useMemo(
    () => new Set(data.connectedHandleIds ?? []),
    [data.connectedHandleIds],
  )
  const tagHiddenHandleIds = useMemo(
    () => new Set(data.tagHiddenHandleIds ?? []),
    [data.tagHiddenHandleIds],
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
    const next = new Map<string, { edgeId: string; endpoint: 'source' | 'target'; selected: boolean }>()
    const sortedCandidates = [...(data.reconnectCandidates ?? [])].sort((left, right) => {
      if (left.selected !== right.selected) return left.selected ? -1 : 1
      return left.edgeId.localeCompare(right.edgeId)
    })
    for (const candidate of sortedCandidates) {
      if (!next.has(candidate.handleId)) {
        next.set(candidate.handleId, { edgeId: candidate.edgeId, endpoint: candidate.endpoint, selected: candidate.selected })
      }
    }
    return next
  }, [data.reconnectCandidates])
  const pickupOnlySides = useMemo(() => new Set(
    (data.reconnectCandidates ?? [])
      .filter(candidate => candidate.selected)
      .map(candidate => candidate.handleId.split('-')[0]),
  ), [data.reconnectCandidates])
  const isMultiSelected = !!data.isMultiSelected

  const nodeLogoUrl = resolveElementIconUrl(data.logo_url, data.technology_connectors) ?? undefined

  const technologyText = elementTechnologyText(data.technology, data.technology_connectors, !!nodeLogoUrl)

  const [, bumpNameFontGeneration] = useReducer((value: number) => value + 1, 0)
  useEffect(
    () => subscribeElementNameFontsChanged(bumpNameFontGeneration),
    [bumpNameFontGeneration],
  )

  const nameFit = data.pendingCreate || !data.name
    ? null
    : computeElementNameFit(data.name, data.kind, !!nodeLogoUrl, !!technologyText)

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
    if (isPending) return
    if (data.parentLinks.length > 1) {
      menuRef.current = { type: 'out', links: data.parentLinks }
      setMenuVisible(true)
    } else {
      data.onZoomOut(data.element_id)
    }
  }

  const handleZoomInClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    if (isPending) return
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
  const suppressHandleClickUntil = useRef(0)

  const onPointerDown = (e: React.PointerEvent) => {
    if (isPending) return
    if (e.pointerType !== 'touch' && e.button !== 0) return
    if ((e.target as Element).closest('.react-flow__handle')) return
    pointerStart.current = { x: e.clientX, y: e.clientY }
    longPressActivated.current = false
    longPressTimer.current = setTimeout(() => {
      longPressActivated.current = true
      data.onInteractionStart(data.element_id)
    }, 500)
  }

  const cancelActiveSourceHandleInteraction = (e: React.PointerEvent | React.MouseEvent) => {
    if (isPending) return
    if (data.interactionSourceId !== data.element_id) return
    if (!(e.target as Element).closest('.react-flow__handle')) return
    suppressHandleClickUntil.current = Date.now() + 500
    e.preventDefault()
    e.stopPropagation()
    data.onInteractionStart(data.element_id)
  }

  const onPointerMove = (e: React.PointerEvent) => {
    if (isPending) return
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
    if (isPending) {
      e.stopPropagation()
      return
    }
    if (Date.now() < suppressHandleClickUntil.current) {
      suppressHandleClickUntil.current = 0
      e.stopPropagation()
      return
    }
    if ((e.target as Element).closest('.react-flow__handle')) {
      e.stopPropagation()
      return
    }
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

  const bodyCursor = isPending ? 'grab' : isSource ? 'crosshair' : isTarget ? 'cell' : 'pointer'

  return (
    <ElementContainer
      data-testid={embeddedCard ? undefined : 'vieweditor-node'}
      data-element-id={data.element_id}
      data-node-name={data.name}
      data-pending={isPending ? 'true' : undefined}
      tabIndex={isPending ? undefined : -1}
      aria-selected={selected}
      isSelected={selected}
      isSource={isSource}
      isTarget={isTarget}
      isConnectorHighlighted={!!data.isConnectorHighlighted}
      hasStack={hasChild}
      kind={data.kind}
      w={`${ELEMENT_NODE_WIDTH}px`}
      h={`${ELEMENT_NODE_HEIGHT}px`}
      cursor={bodyCursor}
      outline={isDraggedOver ? '2px solid' : undefined}
      outlineColor={isDraggedOver ? 'var(--accent)' : undefined}
      outlineOffset={isDraggedOver ? '2px' : undefined}
      borderTopWidth={data.layerHighlightColor ? '2px' : undefined}
      borderTopColor={data.layerHighlightColor ?? undefined}
      onClick={handleBodyClick}
      onPointerDownCapture={cancelActiveSourceHandleInteraction}
      onMouseDownCapture={cancelActiveSourceHandleInteraction}
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
        transition: 'outline 0.15s, outline-color 0.15s, opacity 0.15s',
      } as React.CSSProperties}
    >
      {HANDLE_CONFIGS.flatMap(({ side, position }) =>
        HANDLE_SLOTS.map((slot) => {
          const handleId = getVisualHandleId(side, slot)
          const isHandleChoice = slot % 2 === 0
          // Retain anchors for saved endpoints without offering removed slots for new drops.
          if (!isHandleChoice && !connectedHandleIds.has(handleId)) return null
          const isTagHidden = tagHiddenHandleIds.has(handleId)
          const pickupOnly = pickupOnlySides.has(side)
          const reconnectCandidate = reconnectCandidateByHandle.get(handleId)
          const isFallbackSlot = slot === HANDLE_SLOT_CENTER_INDEX && !activeSides.has(side)
          const className = [
            'element-node-handle',
            isHandleChoice ? 'handle-choice' : 'handle-legacy-slot',
            activeSides.has(side) ? 'handle-active-side' : '',
            slot === HANDLE_SLOT_CENTER_INDEX ? 'handle-center-slot' : 'handle-visual-slot',
            connectedHandleIds.has(handleId) ? 'handle-active-slot' : '',
            isFallbackSlot ? 'handle-fallback-slot' : '',
            connectedHandleIds.has(handleId) ? 'handle-connected' : '',
            selectedHandleIds.has(handleId) ? 'handle-selected-edge' : '',
            hoveredHandleIds.has(handleId) ? 'handle-hovered-edge' : '',
          ].filter(Boolean).join(' ') || undefined

          return (
            <Box key={handleId} position="absolute" inset={0} pointerEvents="none">
              <Handle
                type="source"
                position={position}
                id={handleId}
                className={className}
                data-handle-choice={isHandleChoice ? 'true' : undefined}
                isConnectable={!embeddedCard && !isPending && !isTagHidden && isHandleChoice}
                isConnectableStart={!pickupOnly && isHandleChoice}
                onPointerDown={(e: React.PointerEvent) => {
                  if (pickupOnly || !isHandleChoice) {
                    e.preventDefault()
                    e.stopPropagation()
                    return
                  }
                  if (isPending) return
                  if (data.interactionSourceId === data.element_id) {
                    suppressHandleClickUntil.current = Date.now() + 500
                    e.preventDefault()
                    e.stopPropagation()
                    data.onInteractionStart(data.element_id)
                  }
                }}
                onMouseDown={(e: React.MouseEvent) => {
                  if (isPending || pickupOnly || !isHandleChoice || e.button !== 0 || data.interactionSourceId === data.element_id) return
                  suppressHandleClickUntil.current = Date.now() + 500
                  data.onInteractionStart(data.element_id, {
                    sourceHandle: handleId,
                    clientX: e.clientX,
                    clientY: e.clientY,
                  })
                }}
                onTouchStart={(e: React.TouchEvent) => {
                  if (isPending || pickupOnly || !isHandleChoice || data.interactionSourceId === data.element_id) return
                  const touch = e.touches[0]
                  if (!touch) return
                  suppressHandleClickUntil.current = Date.now() + 500
                  data.onInteractionStart(data.element_id, {
                    sourceHandle: handleId,
                    clientX: touch?.clientX,
                    clientY: touch?.clientY,
                  })
                }}
                style={{
                  ...getVisualHandleStyle(position, slot),
                  ...(embeddedCard ? { opacity: 0, pointerEvents: 'none' as const } : {}),
                  background: isPending ? 'rgba(var(--accent-rgb), 0.45)' : 'var(--accent)',
                  visibility: isTagHidden || (isPending && !connectedHandleIds.has(handleId)) ? 'hidden' : undefined,
                  pointerEvents: embeddedCard || isPending || isTagHidden ? 'none' : undefined,
                }}
              />
              {!embeddedCard && !isTagHidden && data.onStartHandleReconnect && reconnectCandidateByHandle.has(handleId) && (
                <Box
                  position="absolute"
                  data-testid="vieweditor-node-reconnect-zone"
                  data-edge-id={reconnectCandidateByHandle.get(handleId)?.edgeId}
                  data-endpoint={reconnectCandidateByHandle.get(handleId)?.endpoint}
                  data-handle-id={handleId}
                  className="element-node-reconnect-zone"
                  data-selected={reconnectCandidate?.selected ? 'true' : undefined}
                  style={getReconnectZoneStyle(position, slot, !!reconnectCandidate?.selected)}
                  pointerEvents="auto"
                  cursor="grab"
                  zIndex={4}
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
        {!isPending && (
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
        )}

        <Flex boxSize="28px" justify="center" align="center" minW={0} pointerEvents="none">
          {!isPending && nodeLogoUrl && (
            <Box
              as="img"
              src={nodeLogoUrl}
              alt="technology icon"
              maxW="100%"
              maxH="130%"
              objectFit="contain"
              opacity={0.95}
            />
          )}
        </Flex>

        {!isPending && (
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
        )}
      </Flex>

      {/* ── Zoom hover effect ── */}
      {!isPending && data.isZoomHovered && (
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
        nameContent={data.pendingCreate ? <PendingElementLabelEditor config={data.pendingCreate} /> : undefined}
        type={data.kind ?? ''}
        technology={technologyText}
        logoUrl={undefined}
        nameSize="18px"
        nameLines={nameFit?.lines}
        nameFontSize={nameFit?.fontSize}
        h="100%"
        overflow="hidden"
        px={`${ELEMENT_NAME_INSET_RATIO * 50}%`}
        pt={nodeLogoUrl ? 9 : 2}
        pb={2}
      />

      {!isPending && <ElementTags tags={data.tags} tagColors={data.tagColors} forceShowTagPopup={data.forceShowTagPopup} isCanvasMoving={data.isCanvasMoving} />}

      {/* Source link actions */}
      {(!embeddedCard || onOpenSource) && !isPending && (!window.__TLD_VSCODE__ || onOpenSource) && (data.repo || data.repository_id || data.url) && (
        <HStack
          position="absolute"
          bottom="8px"
          right="8px"
          zIndex={10}
          spacing={1}
          align="center"
        >
          {!onOpenSource && (data.repo || data.repository_id) ? (
            <Popover isLazy placement="top-end"><PopoverTrigger><Box as="button" aria-label="Source link actions" onClick={(event: React.MouseEvent) => event.stopPropagation()} onPointerDown={(event: React.PointerEvent) => event.stopPropagation()}><LinkIcon w={2.5} h={2.5} /></Box></PopoverTrigger><PopoverContent w="auto" bg="var(--bg-panel)"><PopoverBody><SourceLinkActions element={data} /></PopoverBody></PopoverContent></Popover>
          ) : (
            <Tooltip
              label={
                data.repo
                  ? `View source: ${sourceAnchorLabel(parseSourceLink(data.file_path).anchor) || sourceLinkKind(data.file_path)}${data.url ? ' / URL' : ''}`
                  : 'Open Link'
              }
              placement="top"
              isDisabled={data.isCanvasMoving}
            >
              <Box
                as="button"
                data-testid="vieweditor-node-source-link"
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
                  if (onOpenSource) {
                    onOpenSource()
                  } else if (data.url) {
                    openExternalUrl(data.url)
                  }
                }}
                onPointerDown={(e: React.PointerEvent) => e.stopPropagation()}
              >
                <LinkIcon w={2.5} h={2.5} />
              </Box>
            </Tooltip>
          )}
        </HStack>
      )}

      {/* VSCode specific file link with hover preview */}
      {!embeddedCard && !onOpenSource && !isPending && window.__TLD_VSCODE__ && data.file_path && (
        <HStack
          position="absolute"
          bottom="8px"
          right="8px"
          zIndex={10}
          spacing={1}
          align="center"
        >
          <VscodeCodePreview
            filePath={data.file_path}
            fallbackSymbolName={data.name}
            fallbackSymbolKind={data.kind}
            isCanvasMoving={data.isCanvasMoving}
          />
        </HStack>
      )}

      {!isPending && selected && !isSource && (
        <HStack
          position="absolute"
          top="-28px"
          left="-14px"
          right="0"
          spacing={1.5}
          justify="space-evenly"
          pointerEvents="none"
          zIndex={10}
          opacity={data.isCanvasMoving ? 0 : 0.6}
          transition="opacity 0.2s"
        >
          <HStack spacing={1}>
            <KbdHint ml={0}>E</KbdHint>
            <Text color="whiteAlpha.400" fontSize="8px">Connect</Text>
          </HStack>
          <HStack spacing={1}>
            <KbdHint ml={0}>R</KbdHint>
            <Text color="whiteAlpha.400" fontSize="8px">Remove</Text>
          </HStack>
          <HStack spacing={1}>
            <ShortcutHint keys={['shift', 'R']} />
            <Text color="whiteAlpha.400" fontSize="8px">Delete</Text>
          </HStack>
          {!isMultiSelected && (
            <HStack spacing={1}>
              <KbdHint ml={0}>T</KbdHint>
              <Text color="whiteAlpha.400" fontSize="8px">Tech</Text>
            </HStack>
          )}
        </HStack>
      )}

      {/* Interaction-mode menu + badge on the source element */}
      {!isPending && isSource && !data.isCanvasMoving && (
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
                  data-testid="vieweditor-node-menu-delete"
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
        </>
      )}

      {/* Keep the drag instructions next to the ghost, using the source-mode badge. */}
      {((isPending && !data.pendingCreate) || (!isPending && isSource && !data.isCanvasMoving)) && (
        <Box
          data-testid="vieweditor-connect-hint"
          position="absolute"
          bottom="-22px"
          left="50%"
          transform={`translateX(-50%) scale(${1 / zoom})`}
          transformOrigin="top center"
          bg="clay.in"
          color="white"
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
          {isPending
            ? 'release on element to connect · release on canvas to add'
            : 'tap element to connect · tap canvas to add'}
        </Box>
      )}

      {/* Drill-down menu for multiple connectors */}
      {!isPending && menuVisible && menuRef.current && !data.isCanvasMoving && (
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
  if (p.isPendingElement || n.isPendingElement || p.pendingCreate || n.pendingCreate) {
    return (
      p.isPendingElement === n.isPendingElement &&
      p.pendingCreate === n.pendingCreate &&
      p.connectedHandleIds === n.connectedHandleIds &&
      p.name === n.name &&
      p.kind === n.kind &&
      p.isCanvasMoving === n.isCanvasMoving
    )
  }
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
    p.interactionSourceHandleId === n.interactionSourceHandleId &&
    p.onZoomIn === n.onZoomIn &&
    p.onZoomOut === n.onZoomOut &&
    p.onResize === n.onResize &&
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
    p.repository_id === n.repository_id &&
    p.branch === n.branch &&
    p.file_path === n.file_path &&
    p.language === n.language &&
    p.isClickConnectMode === n.isClickConnectMode &&
    p.isCreateConnectMode === n.isCreateConnectMode &&
    p.tagColors === n.tagColors &&
    p.layerHighlightColor === n.layerHighlightColor &&
    p.forceShowTagPopup === n.forceShowTagPopup &&
    p.isMultiSelected === n.isMultiSelected &&
    p.connectedHandleIds === n.connectedHandleIds &&
    p.tagHiddenHandleIds === n.tagHiddenHandleIds &&
    p.selectedHandleIds === n.selectedHandleIds &&
    p.reconnectCandidates === n.reconnectCandidates &&
    p.isConnectorHighlighted === n.isConnectorHighlighted &&
    p.parentViewId === n.parentViewId
  )
}

function ElementNodeSwitch(props: Props) {
  return subElementKind(props.data) && !props.data.isPendingElement
    ? <SubElementNode {...props} renderCard={onOpenSource => <ElementNode {...props} embeddedCard onOpenSource={(props.data.repo || props.data.repository_id) && sourceLinkKind(props.data.file_path) === 'file' ? onOpenSource : undefined} />} />
    : <ElementNode {...props} />
}
export default memo(ElementNodeSwitch, arePropsEqual)
