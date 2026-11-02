import { useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState, useSyncExternalStore, type MouseEvent, type ReactNode } from 'react'
import { flushSync } from 'react-dom'
import { Box, Button, HStack, Text } from '@chakra-ui/react'
import { Handle, Position, useStore, useStoreApi } from 'reactflow'
import { useViewEditorContext } from '../pages/ViewEditor/context'
import { readSourcePreviewSize, subscribeSourcePreviewSizes, writeSourcePreviewSize } from '../utils/sourcePreviewSize'
import { isElementGroupTag } from '../utils/elementGroups'
import { buildSymbolPresentation, type SymbolArea } from '../utils/subElementSymbols'
import { SubElementSymbolPorts, SubElementSymbolRegions, type SymbolInteractions } from './SubElementSymbolLayer'
import type { NodeData } from './ElementNode'
import { useSubElementLayout } from '../hooks/useSubElementLayout'
import SubElementLayoutButtons from './SubElementLayoutButtons'
import SubElementResizeHandle from './SubElementResizeHandle'
import { isSubElement, SUB_HEADER, subEditorSize, subElementKind, subResizeLayout, type SubDetail } from '../utils/subElements'
import { peekSubSource, requestSubSource, subscribeSubSources } from '../utils/subElementSource'
import { expandCodeTabs, expandCodeTokens, SUB_CODE_FONT_FAMILY } from '../utils/subElementText'
import { elementTechnologyText, resolveElementIconUrl } from '../utils/elementIcon'
import { ElementBody } from './NodeBody'
import ElementTags from './ElementTags'
import SourceLinkActions from './SourceLinkActions'

export default function SubElementNode({ data, selected, renderCard }: { data: NodeData; selected: boolean; renderCard: (onOpenSource: () => void) => ReactNode }) {
  const kind = subElementKind(data)!
  const [draftSize, setDraftSize] = useState<{ width: number; height: number; detail: SubDetail } | null>(null)
  const { canEdit, viewId } = useViewEditorContext()
  const { detail: savedDetail, setDetail } = useSubElementLayout(viewId ?? data.view_id, data.element_id, isSubElement(data.kind) ? 'code' : 'overview')
  const detail = draftSize?.detail ?? savedDetail
  const previewSize = useSyncExternalStore(subscribeSourcePreviewSizes, () => readSourcePreviewSize(viewId ?? data.view_id, data.element_id), () => null)
  useEffect(() => {
    if (detail !== 'overview' && !isSubElement(data.kind) && !previewSize) {
      const size = subEditorSize(kind, null, 'code')
      writeSourcePreviewSize(viewId ?? data.view_id, data.element_id, size)
    }
  }, [detail, data.kind, data.view_id, data.element_id, viewId, kind, previewSize])
  const savedSize = subEditorSize(kind, previewSize, savedDetail)
  const { width, height } = draftSize ?? savedSize
  const [hoveredSymbol, setHoveredSymbol] = useState<string | null>(null)
  const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null)
  const nativeSourceHandle = useStore(state => state.connectionNodeId === String(data.element_id) ? state.connectionHandleId : null)
  const nativeConnecting = useStore(state => state.connectionNodeId != null)
  const [, refresh] = useReducer(n => n + 1, 0)
  const [scrollTop, setScrollTop] = useState(0)
  const [scrollLeft, setScrollLeft] = useState(0)
  const [scrollViewport, setScrollViewport] = useState<{ width: number; height: number } | null>(null)
  const rootRef = useRef<HTMLDivElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const flow = useStoreApi()
  const sourceIdentity = JSON.stringify([data.repo, data.repository_id, data.branch, data.file_path, data.language])
  const sourceLink = useMemo(() => ({ repo: data.repo, repository_id: data.repository_id, branch: data.branch, file_path: data.file_path, language: data.language }), [data.repo, data.repository_id, data.branch, data.file_path, data.language])
  const source = kind === 'code' ? peekSubSource(data) : undefined
  useEffect(() => kind === 'code' && detail !== 'overview' ? subscribeSubSources(refresh) : undefined, [kind, detail])
  useEffect(() => { if (kind === 'code' && detail !== 'overview') requestSubSource(sourceLink) }, [kind, sourceLink, detail])
  useEffect(() => { setScrollTop(0); setScrollLeft(0); if (scrollRef.current) { scrollRef.current.scrollTop = 0; scrollRef.current.scrollLeft = 0 } }, [sourceIdentity, detail])
  const startLine = source?.startLine ?? 1
  const iconUrl = resolveElementIconUrl(data.logo_url, data.technology_connectors)
  const technology = elementTechnologyText(data.technology, data.technology_connectors, !!iconUrl)
  const metadata = source?.status !== 'ready' ? source?.error || 'Loading source…' : technology
  // The header and scroll surface fit inside the card's one-pixel border.
  const visibleHeight = height - SUB_HEADER - 2
  const groupTag = data.tags.find(isElementGroupTag)
  const connectionCounts: Record<string, number> = {}
  for (const candidate of data.reconnectCandidates ?? []) connectionCounts[candidate.handleId] = (connectionCounts[candidate.handleId] ?? 0) + 1
  for (const id of data.tagHiddenHandleIds ?? []) connectionCounts[id] = 0
  const presentation = buildSymbolPresentation({ kind, source, path: data.file_path ?? '', detail, width, height, scrollTop, connected: data.connectedHandleIds, connectionCounts, groupColor: groupTag ? data.tagColors[groupTag]?.color : undefined })
  const { rowHeight, rowCount } = presentation
  useLayoutEffect(() => {
    const scroller = scrollRef.current
    if (!scroller) return
    const measure = () => {
      const next = { width: scroller.clientWidth, height: scroller.clientHeight }
      setScrollViewport(previous => previous?.width === next.width && previous.height === next.height ? previous : next)
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(scroller)
    return () => observer.disconnect()
  }, [detail, width, height, presentation.contentWidth, rowCount, rowHeight])
  const viewportWidth = scrollViewport?.width ?? width - 2
  const viewportHeight = scrollViewport?.height ?? visibleHeight - 1
  const reading = detail === 'code'
  const first = Math.min(rowCount, Math.max(0, Math.floor(scrollTop / rowHeight) - 1))
  const last = Math.min(rowCount, first + Math.ceil(visibleHeight / rowHeight) + 3)
  const interactions: SymbolInteractions = {
    showStem: endpoint => endpoint.connected || hoveredSymbol === endpoint.area?.key || nativeConnecting || data.interactionSourceId != null,
    isActive: area => hoveredSymbol === area.key || selectedSymbol === area.key || [nativeSourceHandle, data.interactionSourceHandleId, ...(data.selectedHandleIds ?? [])].some(id => id && (id === area.leftId || id === area.rightId || presentation.endpoints.get(id)?.area?.key === area.key)),
    onHover: setHoveredSymbol,
    onActivate: (area: SymbolArea, event: MouseEvent<HTMLElement>) => {
      event.stopPropagation()
      const selection = window.getSelection()
      if (!data.interactionSourceId && selection && !selection.isCollapsed && scrollRef.current?.contains(selection.anchorNode)) return
      setSelectedSymbol(area.key)
      if (canEdit && data.interactionSourceId && data.interactionSourceId !== data.element_id) data.onInteractionStart(data.element_id, { sourceHandle: area.leftId, clientX: event.clientX, clientY: event.clientY })
    },
  }
  const geometry = [...presentation.endpoints.values()].map(endpoint => `${endpoint.id}:${endpoint.y}:${endpoint.side}`).join('|')
  useLayoutEffect(() => {
    const nodeElement = rootRef.current?.closest<HTMLDivElement>('.react-flow__node')
    if (!nodeElement) return
    // The public updater defers a frame; scrolling ports and their edges must agree before paint.
    flow.getState().updateNodeDimensions([{ id: nodeElement.dataset.id ?? String(data.element_id), nodeElement, forceUpdate: true }])
  }, [flow, data.element_id, width, height, geometry])
  const sourceActive = data.interactionSourceId === data.element_id
  return <Box ref={rootRef} position="relative" w={`${width}px`} h={`${height}px`} data-detail-level={detail} data-testid="vieweditor-node" data-element-id={data.element_id} data-node-name={data.name} aria-selected={selected}>
    {detail !== 'overview' && data.onResize && <SubElementResizeHandle name={data.name} width={width} height={height} keyboardStep={detail === 'outline' ? 40 : 10}
      onPreview={size => setDraftSize(size ? subResizeLayout(size, savedSize, savedDetail) : null)}
      onCommit={async size => {
        const next = subResizeLayout(size, savedSize, savedDetail)
        if (next.detail === 'code') await data.onResize!(data.element_id, next.width, next.height)
        setDetail(next.detail)
      }} />}
    {detail === 'overview' ? renderCard(() => setDetail('code')) : <Box w="100%" h="100%" position="relative" bg="#20252d" border="1px solid" borderColor={selected || sourceActive ? 'var(--accent)' : '#455263'} borderRadius="10px" onClick={e => {
    e.stopPropagation()
    const selection = window.getSelection()
    if (selection && !selection.isCollapsed && scrollRef.current?.contains(selection.anchorNode)) return
    if (sourceActive) data.onInteractionStart(data.element_id)
    else if (data.interactionSourceId) data.onConnectTo(data.element_id)
    else data.onSelect(data)
  }}>
    <Box data-testid="sub-element-header" h={`${SUB_HEADER}px`} position="relative" bg="var(--bg-element)" borderTopRadius="9px">
      {iconUrl && <Box as="img" src={iconUrl} alt="technology icon" position="absolute" top="6px" left="calc(50% - 12px)" boxSize="24px" objectFit="contain" pointerEvents="none" />}
      <ElementBody name={data.name} type={data.kind ?? ''} nameSize="18px" nameNoOfLines={1} typeSize="9px" h="100%" px={12} pt={iconUrl ? '32px' : '6px'} pb="4px">
        {metadata && <Text fontSize="10px" color="gray.500" lineHeight={1.1} noOfLines={1} maxW="100%" title={metadata}>[{metadata}]</Text>}
      </ElementBody>
      <ElementTags tags={data.tags} tagColors={data.tagColors} forceShowTagPopup={data.forceShowTagPopup} isCanvasMoving={data.isCanvasMoving} />
      <HStack position="absolute" top="6px" right="8px" spacing={1}>
        {kind === 'code' && <Button aria-label="Reload source" size="xs" onClick={e => { e.stopPropagation(); requestSubSource(data, true); refresh() }}>↻</Button>}
        {data.links.length > 0 && <Button size="xs" onClick={e => { e.stopPropagation(); data.onZoomIn(data.element_id) }}>Enter</Button>}
      </HStack>
      {(data.repo || data.repository_id || data.url) && <Box position="absolute" bottom="6px" right="8px"><SourceLinkActions element={data} source={source} /></Box>}
    </Box>
    <Box ref={scrollRef} className="nodrag nowheel" h={`${visibleHeight}px`} overflow="auto" overscrollBehavior="none" onScroll={e => {
      const top = e.currentTarget.scrollTop, left = e.currentTarget.scrollLeft
      if (top === scrollTop && left === scrollLeft) return
      // The pinned scene and connector overlay advance together, rather than native content leading.
      flushSync(() => { setScrollTop(top); setScrollLeft(left) })
    }} borderTop="1px solid #455263" borderBottomRadius="9px">
      <Box w={`${Math.max(presentation.contentWidth, viewportWidth)}px`} h={`${Math.max(rowCount * rowHeight, viewportHeight)}px`} position="relative">
        {/* Native scrolling moves only the spacer. Keep the visible scene pinned until the
            same commit updates its translation, connector paths, ports, and external edges. */}
        <Box data-testid="sub-element-scroll-viewport" position="sticky" top={0} left={0} w={`${viewportWidth}px`} h={`${viewportHeight}px`} overflow="clip">
          <Box h={`${rowCount * rowHeight}px`} w={`${presentation.contentWidth}px`} position="absolute" left={0} top={0} transform={`translate(${-scrollLeft}px, ${-scrollTop}px)`}>
      {kind === 'code' && source?.status === 'error' && <Button size="xs" m={3} onClick={() => { requestSubSource(data, true); refresh() }}>Retry source</Button>}
        <SubElementSymbolRegions presentation={presentation} scrollTop={scrollTop} interactions={interactions} />
        {Array.from({ length: last - first }, (_, offset) => {
          const index = first + offset
          const line = startLine + index
          const area = [...presentation.areas].reverse().find(group => index >= group.start && index <= group.end)
          return <Box key={index} position="absolute" top={`${index * rowHeight}px`} left={0} minW={`${width - 2}px`} h={`${rowHeight}px`} px="14px" fontFamily={SUB_CODE_FONT_FAMILY} fontWeight="400" style={{ fontVariantLigatures: 'none' }} fontSize="12px" lineHeight={`${rowHeight}px`} whiteSpace="pre" pointerEvents="none" zIndex={1}>
            {kind === 'code' && reading && <Text as="span" display="inline-block" w="40px" color="gray.500">{line}</Text>}
            <span data-symbol-handle={area?.leftId} data-symbol-key={area?.key} className="sub-symbol-text" style={{ pointerEvents: 'auto', display: 'inline-block', minWidth: area && !(kind === 'code' && reading) ? Math.max(0, area.width - 12) : undefined }} onMouseEnter={() => interactions.onHover(area?.key ?? null)} onMouseLeave={() => interactions.onHover(null)} onClick={event => area && interactions.onActivate(area, event)}>
            {reading ? <>{source?.tokens[index]?.length ? expandCodeTokens(source.tokens[index]).map((token, i) => <span key={i} style={{ color: token.color }}>{token.text}</span>) : expandCodeTabs(source?.lines[index] ?? '')}</> : area?.signature?.tokens.map((token, i) => <span key={i} style={{ color: token.color }}>{token.text}</span>)}
            </span>
          </Box>
        })}
          </Box>
        </Box>
      </Box>
    </Box>
    {(['top', 'bottom', 'left', 'right'] as const).flatMap(side => [0, 1, 2, 3, 4].map(slot => <Handle key={`${side}-${slot}`} id={`${side}-${slot}`} type="source" isConnectable={false} className={`element-node-handle handle-choice ${slot === 2 ? 'handle-center-slot' : ''} ${data.connectedHandleIds?.includes(`${side}-${slot}`) ? 'handle-active-slot' : ''}`} position={{ top: Position.Top, bottom: Position.Bottom, left: Position.Left, right: Position.Right }[side]} style={side === 'left' || side === 'right' ? { opacity: 0, top: `calc(50% + ${(slot - 2) * 12}px)` } : { opacity: 0, left: `calc(50% + ${(slot - 2) * 12}px)` }} />))}
    <SubElementSymbolPorts data={data} presentation={presentation} width={width} height={height} scrollTop={scrollTop} scrollLeft={scrollLeft} canEdit={canEdit} interactions={interactions} onReveal={index => {
      if (scrollRef.current) scrollRef.current.scrollTop = Math.max(0, index * rowHeight)
    }} />
    </Box>}
    <SubElementLayoutButtons detail={detail} onChange={setDetail} />
    {detail === 'overview' && (data.connectedHandleIds ?? []).filter(id => /-(member|line|symbol)-/.test(id)).map(id => <Handle key={id} id={id} type="source" position={id.startsWith('left') ? Position.Left : Position.Right} style={{ top: height / 2, opacity: 0, pointerEvents: 'none' }} />)}
  </Box>
}
