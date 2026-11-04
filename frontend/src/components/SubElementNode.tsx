import SubElementBanner from './SubElementBanner'
import PreviewInteractionHint from './PreviewInteractionHint'
import SubElementTextControls from './SubElementTextControls'
import { readSourcePreviewAppearance, writeSourcePreviewAppearance, subscribeSourcePreviewAppearance } from '../utils/sourcePreviewAppearance'
import { ElementContainer } from './NodeContainer'
import { usePreviewInteraction } from '../hooks/usePreviewInteraction'
import ElementNavigation from './ElementNavigation'
import { symbolDropHandle } from '../utils/symbolDropHandle'
import { useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState, useSyncExternalStore, type MouseEvent, type ReactNode } from 'react'
import { flushSync } from 'react-dom'
import { Box, Button, Text } from '@chakra-ui/react'
import { Handle, Position, useStore, useStoreApi } from 'reactflow'
import { useViewEditorContext } from '../pages/ViewEditor/context'
import { readSourcePreviewSize, subscribeSourcePreviewSizes } from '../utils/sourcePreviewSize'
import { buildSymbolPresentation, foldBannerBounds, type SymbolArea } from '../utils/subElementSymbols'
import { SubElementSymbolPorts, SubElementSymbolRegions, type SymbolInteractions } from './SubElementSymbolLayer'
import type { NodeData } from './ElementNode'
import { useSubElementLayout } from '../hooks/useSubElementLayout'
import SubElementLayoutButtons from './SubElementLayoutButtons'
import SubElementResizeHandle from './SubElementResizeHandle'
import { expandLineHandles, isSubElement, SUB_HEADER, subEditorSize, subElementKind, type SubDetail } from '../utils/subElements'
import { peekSubSource, requestSubSource, subscribeSubSources } from '../utils/subElementSource'
import { expandCodeTabs, expandCodeTokens, SUB_CODE_FONT_FAMILY } from '../utils/subElementText'
import { elementTechnologyText, resolveElementIconUrl } from '../utils/elementIcon'
import { ElementBody, ElementHeaderIcon } from './NodeBody'
import ElementTags from './ElementTags'
import SourceLinkActions from './SourceLinkActions'
import ElementReconnectZone from './ElementReconnectZone'
import { compareReconnectCandidates } from './elementReconnect'
import { getVisualHandleStyle } from '../utils/edgeDistribution'

export default function SubElementNode({ data, selected, renderCard }: { data: NodeData; selected: boolean; renderCard: (onOpenSource: () => void) => ReactNode }) {
  const kind = subElementKind(data)!
  const [draftSize, setDraftSize] = useState<{ width: number; height: number; detail: SubDetail } | null>(null)
  const { canEdit, viewId, isConnectingConnector, selectedConnector, connectorDragSource } = useViewEditorContext()
  const { detail: savedDetail, setDetail } = useSubElementLayout(viewId ?? data.view_id, data.element_id, isSubElement(data.kind) ? 'code' : 'overview')
  const detail = draftSize?.detail ?? savedDetail
  const appearance = useSyncExternalStore(subscribeSourcePreviewAppearance, () => readSourcePreviewAppearance(viewId ?? data.view_id, data.element_id), () => readSourcePreviewAppearance(viewId ?? data.view_id, data.element_id))
  const lineNumbers = appearance.lineNumbers ?? detail === 'code'
  const previewSize = useSyncExternalStore(subscribeSourcePreviewSizes, () => readSourcePreviewSize(viewId ?? data.view_id, data.element_id), () => null)
  const savedSize = subEditorSize(kind, previewSize, savedDetail)
  const { width, height } = draftSize ?? savedSize
  const [hoveredSymbol, setHoveredSymbol] = useState<string | null>(null)
  const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null)
  const nativeSourceHandle = useStore(state => state.connectionNodeId === String(data.element_id) ? state.connectionHandleId : null)
  const nativeConnecting = useStore(state => state.connectionNodeId != null)
  const [, refresh] = useReducer(n => n + 1, 0)
  const [scrollTop, setScrollTop] = useState(0)
  const [scrollLeft, setScrollLeft] = useState(0)
  const [expandedFolds, setExpandedFolds] = useState<number[]>([])
  const [scrollViewport, setScrollViewport] = useState<{ width: number; height: number } | null>(null)
  const rootRef = useRef<HTMLDivElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const revealedSelection = useRef('')
  const flow = useStoreApi()
  const previewInteraction = usePreviewInteraction(detail !== 'overview', data.isCanvasMoving, scrollRef)
  const foldIdentity = JSON.stringify([data.connectedHandleIds, data.tagHiddenHandleIds])
  const sourceIdentity = JSON.stringify([data.repo, data.repository_id, data.branch, data.file_path, data.language])
  const sourceLink = useMemo(() => ({ repo: data.repo, repository_id: data.repository_id, branch: data.branch, file_path: data.file_path, language: data.language }), [data.repo, data.repository_id, data.branch, data.file_path, data.language])
  const source = kind === 'code' ? peekSubSource(data) : undefined
  useEffect(() => kind === 'code' && detail !== 'overview' ? subscribeSubSources(refresh) : undefined, [kind, detail])
  useEffect(() => { if (kind === 'code' && detail !== 'overview') requestSubSource(sourceLink) }, [kind, sourceLink, detail])
  useEffect(() => { setScrollTop(0); setScrollLeft(0); if (scrollRef.current) { scrollRef.current.scrollTop = 0; scrollRef.current.scrollLeft = 0 } }, [sourceIdentity, detail, appearance.connectedOnly])
  useEffect(() => { setExpandedFolds(previous => previous.length ? [] : previous) }, [sourceIdentity, detail, appearance.connectedOnly, source?.lines, source?.symbols, foldIdentity])
  const iconUrl = resolveElementIconUrl(data.logo_url, data.technology_connectors)
  const technology = elementTechnologyText(data.technology, data.technology_connectors, !!iconUrl)
  const metadata = source?.status !== 'ready' ? source?.error || 'Loading source…' : technology
  // The header and scroll surface fit inside the card's one-pixel border.
  const visibleHeight = height - SUB_HEADER - 2
  const connectionCounts: Record<string, number> = {}
  for (const candidate of data.reconnectCandidates ?? []) connectionCounts[candidate.handleId] = (connectionCounts[candidate.handleId] ?? 0) + 1
  for (const id of data.tagHiddenHandleIds ?? []) connectionCounts[id] = 0
  const presentation = buildSymbolPresentation({ kind, source, path: data.file_path ?? '', detail, width, height, scrollTop, connected: data.connectedHandleIds, connectionCounts, fontSize: appearance.fontSize, lineNumbers, connectedOnly: appearance.connectedOnly, expandedFolds })
  const { rowHeight, rowCount } = presentation
  const bannerHeight = foldBannerBounds(presentation, 0).height
  const endpointsRef = useRef(presentation.endpoints)
  endpointsRef.current = presentation.endpoints
  const areasRef = useRef(presentation.areas)
  areasRef.current = presentation.areas
  useEffect(() => {
    const key = JSON.stringify([detail, sourceIdentity, appearance.connectedOnly, selectedConnector?.id, data.selectedHandleIds])
    if (detail === 'overview' || !data.selectedHandleIds?.length) { revealedSelection.current = ''; return }
    if (revealedSelection.current === key) return
    const scroller = scrollRef.current
    const endpoint = data.selectedHandleIds.flatMap(expandLineHandles).map(id => endpointsRef.current.get(id)).find(endpoint => endpoint?.area)
    const area = endpoint?.area
    if (!scroller || !area) return
    revealedSelection.current = key
    const top = area.start * rowHeight, bottom = (area.end + 1) * rowHeight
    const nextTop = top < scroller.scrollTop + bannerHeight || bottom > scroller.scrollTop + scroller.clientHeight - bannerHeight
      ? Math.max(0, top - bannerHeight) : scroller.scrollTop
    const nextLeft = area.x < scroller.scrollLeft || area.x > scroller.scrollLeft + scroller.clientWidth - 16
      ? Math.max(0, area.x - 12) : scroller.scrollLeft
    scroller.scrollTo({ top: nextTop, left: nextLeft, behavior: 'auto' })
  }, [detail, sourceIdentity, appearance.connectedOnly, selectedConnector?.id, data.selectedHandleIds, presentation.rows, rowHeight, bannerHeight])
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
  const connecting = nativeConnecting || isConnectingConnector || data.isCreateConnectMode || data.isClickConnectMode || data.interactionSourceId != null
  const dragSourceHandle = nativeSourceHandle ?? (connectorDragSource?.nodeId === String(data.element_id) ? connectorDragSource.handleId : null)
  const dragSourceSet = useMemo(() => {
    const set = new Set<string>()
    if (dragSourceHandle) {
      for (const id of [dragSourceHandle, ...expandLineHandles(dragSourceHandle)]) {
        set.add(id)
        const areaKey = endpointsRef.current.get(id)?.area?.key
        if (areaKey) set.add(areaKey)
      }
    }
    return set
  // presentation.rows is the cached geometry identity; endpoint->area mapping only changes when geometry changes.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dragSourceHandle, presentation.rows])
  const activeSet = useMemo(() => {
    const set = new Set<string>()
    for (const rawId of [dragSourceHandle, data.interactionSourceHandleId, ...(data.selectedHandleIds ?? [])]) {
      if (!rawId) continue
      for (const id of [rawId, ...expandLineHandles(rawId)]) {
        set.add(id)
        const areaKey = endpointsRef.current.get(id)?.area?.key
        if (areaKey) set.add(areaKey)
      }
    }
    return set
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dragSourceHandle, data.interactionSourceHandleId, data.selectedHandleIds, presentation.rows])
  const isDragSource = (area: SymbolArea) => dragSourceSet.size > 0 && (dragSourceSet.has(area.key) || dragSourceSet.has(area.leftId) || dragSourceSet.has(area.rightId))
  const interactions: SymbolInteractions = {
    showHighlight: area => detail === 'structure' || area.connected || hoveredSymbol === area.key || isDragSource(area) || !!connecting,
    showStem: endpoint => endpoint.connected || hoveredSymbol === endpoint.area?.key || connecting,
    isActive: area => hoveredSymbol === area.key || selectedSymbol === area.key || (activeSet.size > 0 && (activeSet.has(area.key) || activeSet.has(area.leftId) || activeSet.has(area.rightId))),
    onHover: setHoveredSymbol,
    onActivate: (area: SymbolArea, event: MouseEvent<HTMLElement>) => {
      event.stopPropagation()
      const selection = window.getSelection()
      if (!data.interactionSourceId && selection && !selection.isCollapsed && scrollRef.current?.contains(selection.anchorNode)) return
      setSelectedSymbol(area.key)
      if (canEdit && data.interactionSourceId && data.interactionSourceId !== data.element_id) data.onInteractionStart(data.element_id, { sourceHandle: symbolDropHandle(event.currentTarget, event.clientX, String(data.interactionSourceId)) ?? area.leftId, clientX: event.clientX, clientY: event.clientY })
    },
  }
  const trackedEndpoints = connecting
    ? [...presentation.endpoints.values()]
    : [...presentation.endpoints.values()].filter(endpoint => endpoint.connected)
  const geometry = `${detail}:${(data.connectedHandleIds ?? []).join(',')}:${trackedEndpoints.map(endpoint => `${endpoint.id}:${endpoint.y}:${endpoint.side}`).join('|')}`
  const connectedHandlesRef = useRef(data.connectedHandleIds)
  connectedHandlesRef.current = data.connectedHandleIds
  const hasTrackedEndpoints = trackedEndpoints.length > 0
  useLayoutEffect(() => {
    const nodeElement = rootRef.current?.closest<HTMLDivElement>('.react-flow__node')
    if (!nodeElement) return
    const nodeId = nodeElement.dataset.id ?? String(data.element_id)
    const state = flow.getState()
    const existingNode = state.nodeInternals.get(nodeId)
    if (!connecting && existingNode && existingNode.width === width && existingNode.height === height && detail !== 'overview') {
      const sym = Object.getOwnPropertySymbols(existingNode)[0]
      const internals = sym ? (existingNode as unknown as Record<symbol, { handleBounds?: { source?: Array<{ id?: string | null; y: number; height: number }>; target?: unknown } }>)[sym] : undefined
      const sourceBounds = internals?.handleBounds?.source
      const connectedHandles = connectedHandlesRef.current ?? []
      if (sourceBounds && connectedHandles.every(id => sourceBounds.some(h => h.id === id))) {
        const endpoints = endpointsRef.current
        let changed = false
        const nextSource = sourceBounds.map(h => {
          const ep = h.id ? endpoints.get(h.id) : undefined
          if (!ep) return h
          const nextY = ep.y - (h.height || 0) / 2
          if (h.y === nextY) return h
          changed = true
          return { ...h, y: nextY }
        })
        if (changed || hasTrackedEndpoints) {
          const nextInternals = new Map(state.nodeInternals)
          nextInternals.set(nodeId, {
            ...existingNode,
            [sym]: {
              ...internals,
              handleBounds: { ...internals.handleBounds, source: nextSource },
            },
          })
          flow.setState({ nodeInternals: nextInternals })
        }
        return
      }
    }
    // The public updater defers a frame; scrolling ports and their edges must agree before paint.
    state.updateNodeDimensions([{ id: nodeId, nodeElement, forceUpdate: true }])
  }, [flow, data.element_id, width, height, geometry, connecting, detail, hasTrackedEndpoints])
  const sourceActive = data.interactionSourceId === data.element_id
  const reconnectCandidates = useMemo(() => canEdit && data.onStartHandleReconnect
    ? [...(data.reconnectCandidates ?? [])].filter(candidate => !data.tagHiddenHandleIds?.includes(candidate.handleId)).sort(compareReconnectCandidates)
    : [], [canEdit, data.onStartHandleReconnect, data.reconnectCandidates, data.tagHiddenHandleIds])
  const interactionsRef = useRef(interactions)
  interactionsRef.current = interactions
  const stableInteractions = useMemo<SymbolInteractions>(() => ({
    showHighlight: area => interactionsRef.current.showHighlight(area),
    showStem: endpoint => interactionsRef.current.showStem(endpoint),
    isActive: area => interactionsRef.current.isActive(area),
    onHover: key => interactionsRef.current.onHover(key),
    onActivate: (area, event) => interactionsRef.current.onActivate(area, event),
  }), [])
  const hasVisibleFoldBanner = useMemo(() => {
    for (let i = first; i < last; i += 1) {
      if (presentation.rows[i]?.collapsed) return true
    }
    return false
  }, [first, last, presentation.rows])
  const foldScrollLeft = hasVisibleFoldBanner ? scrollLeft : 0
  const renderedRows = useMemo(() => {
    if (detail === 'structure') return null
    const areas = areasRef.current
    return Array.from({ length: last - first }, (_, offset) => {
      const index = first + offset
      const displayRow = presentation.rows[index]
      const sourceIndex = displayRow.sourceIndex
      const line = (source?.startLine ?? 1) + sourceIndex
      let area: SymbolArea | undefined
      for (let i = areas.length - 1; i >= 0; i -= 1) {
        const candidate = areas[i]
        if (index >= candidate.start && index <= candidate.end) {
          area = candidate
          break
        }
      }
      if (displayRow.collapsed) {
        const foldBounds = foldBannerBounds({ rowHeight, contentInset: presentation.contentInset }, index)
        return <SubElementBanner key={index} data-testid="sub-source-fold-banner" direction="collapsed"
          label={`${displayRow.collapsed} ${reading ? 'lines' : 'symbols'} hidden`} top={`${foldBounds.top}px`} left={`${foldScrollLeft}px`} right="auto" w={`${viewportWidth}px`} h={`${foldBounds.height}px`} fontSize={`${Math.max(1, Math.min(11, foldBounds.height - 2))}px`}
          onClick={() => setExpandedFolds(previous => [...previous, sourceIndex])} />
      }
      return <Box key={index} position="absolute" top={`${index * rowHeight + presentation.contentInset}px`} left={0} minW={`${width - 2}px`} h={`${rowHeight}px`} px="14px" fontFamily={SUB_CODE_FONT_FAMILY} fontWeight="400" style={{ fontVariantLigatures: 'none' }} fontSize={`${appearance.fontSize}px`} lineHeight={`${rowHeight}px`} whiteSpace="pre" pointerEvents="none" zIndex={1}>
        {kind === 'code' && lineNumbers && !displayRow.collapsed && <Text data-testid="source-code-line-number" as="span" display="inline-block" w={`${presentation.textX - 14}px`} color="gray.500">{reading ? line : area?.symbol.startLine}</Text>}
        <span data-symbol-handle={area?.leftId} data-symbol-key={area?.key} className="sub-symbol-text" style={{ pointerEvents: 'auto', display: 'inline-block', minWidth: area && !(kind === 'code' && reading) ? Math.max(0, area.width - 12) : undefined }} onMouseEnter={() => stableInteractions.onHover(area?.key ?? null)} onMouseLeave={() => stableInteractions.onHover(null)} onClick={event => area && stableInteractions.onActivate(area, event)}>
          {reading ? <>{source?.tokens[sourceIndex]?.length ? expandCodeTokens(source.tokens[sourceIndex]).map((token, i) => <span key={i} style={{ color: token.color }}>{token.text}</span>) : expandCodeTabs(source?.lines[sourceIndex] ?? '')}</> : area?.signature?.tokens.map((token, i) => <span key={i} style={{ color: token.color }}>{token.text}</span>)}
        </span>
      </Box>
    })
  }, [detail, first, last, presentation.rows, presentation.contentInset, presentation.textX, rowHeight, width, kind, lineNumbers, reading, source, appearance.fontSize, foldScrollLeft, viewportWidth, stableInteractions])
  const bodySlotHandles = useMemo(() => (['top', 'bottom', 'left', 'right'] as const).flatMap(side => [0, 1, 2, 3, 4].map(slot => {
    const id = `${side}-${slot}`, position = { top: Position.Top, bottom: Position.Bottom, left: Position.Left, right: Position.Right }[side]
    const anchorStyle = getVisualHandleStyle(position, slot)
    return <Box key={id} position="absolute" inset={0} pointerEvents="none">
      <Handle id={id} type="source" isConnectable={false} className={`element-node-handle handle-choice ${slot === 2 ? 'handle-center-slot' : ''} ${data.connectedHandleIds?.includes(id) ? 'handle-active-slot' : ''}`} position={position} style={{ ...anchorStyle, opacity: 0 }} />
      <ElementReconnectZone candidate={reconnectCandidates.find(candidate => candidate.handleId === id)} position={position} anchorStyle={anchorStyle} zIndex={10} />
    </Box>
  })), [data.connectedHandleIds, reconnectCandidates])
  return <Box ref={rootRef} position="relative" w={`${width}px`} h={`${height}px`} data-detail-level={detail} data-testid="vieweditor-node" data-element-id={data.element_id} data-node-name={data.name} aria-selected={selected} data-preview-interaction={previewInteraction.phase}>
    {detail !== 'overview' && data.onResize && <SubElementResizeHandle name={data.name} width={width} height={height} keyboardStep={10}
      onPreview={size => setDraftSize(size ? { ...size, detail: savedDetail } : null)}
      onCommit={async size => {
        await data.onResize!(data.element_id, size.width, size.height)
      }} />}
    {detail === 'overview' ? renderCard(() => setDetail('code')) : <ElementContainer data-testid="sub-element-container" w={`${width}px`} h={`${height}px`} bg="var(--bg-source-preview, #232732)"
      isSelected={selected} isSource={sourceActive} isTarget={!!data.interactionSourceId && !sourceActive}
      isConnectorHighlighted={!!data.isConnectorHighlighted} hasStack={data.links.length > 0 || data.has_view} kind={data.kind}
      borderTopWidth={data.layerHighlightColor ? '2px' : undefined} borderTopColor={data.layerHighlightColor ?? undefined}
      onClick={e => {
    e.stopPropagation()
    const selection = window.getSelection()
    if (selection && !selection.isCollapsed && scrollRef.current?.contains(selection.anchorNode)) return
    if (sourceActive) data.onInteractionStart(data.element_id)
    else if (data.interactionSourceId) data.onConnectTo(data.element_id)
    else data.onSelect(data)
  }}>
    <Box data-testid="sub-element-header" h={`${SUB_HEADER}px`} position="relative" bg="var(--bg-element)" borderTopRadius="calc(var(--chakra-radii-lg) - 1px)">
      <ElementBody name={detail === 'structure' ? '' : data.name} type={detail === 'structure' ? '' : data.kind ?? ''} nameSize="18px" nameNoOfLines={1} typeSize="9px" h={`${SUB_HEADER - 1}px`} px={12} pt={iconUrl ? 9 : 2} pb={2}>
        {detail !== 'structure' && metadata && <Text fontSize="10px" color="gray.500" lineHeight={1.1} noOfLines={1} maxW="100%" title={metadata}>[{metadata}]</Text>}
      </ElementBody>
      <ElementTags tags={data.tags} tagColors={data.tagColors} forceShowTagPopup={data.forceShowTagPopup} isCanvasMoving={data.isCanvasMoving} />
      <ElementNavigation data={data}><ElementHeaderIcon src={iconUrl} /></ElementNavigation>
      {(data.repo || data.repository_id || data.url) && <Box position="absolute" bottom={1} right={1}><SourceLinkActions element={data} source={source} /></Box>}
    </Box>
    <PreviewInteractionHint interaction={previewInteraction} />
    <Box ref={scrollRef} onPointerEnter={previewInteraction.onPointerEnter} onPointerMove={previewInteraction.onPointerMove} onPointerLeave={previewInteraction.onPointerLeave} className={`source-preview-scroll nodrag${previewInteraction.phase === 'ready' ? ' nowheel nopan' : ''}`} sx={canEdit && previewInteraction.phase === 'ready' ? { '& [data-symbol-handle]': { cursor: 'crosshair' } } : undefined} h={`${visibleHeight}px`} overflow={previewInteraction.phase === 'ready' ? 'auto' : 'hidden'} overscrollBehavior="none" onScroll={e => {
      const top = e.currentTarget.scrollTop, left = e.currentTarget.scrollLeft
      if (top === scrollTop && left === scrollLeft) return
      // The pinned scene and connector overlay advance together, rather than native content leading.
      flushSync(() => { setScrollTop(top); setScrollLeft(left) })
    }} borderTop="1px solid #455263" borderBottomRadius="calc(var(--chakra-radii-lg) - 1px)">
      <Box w={`${Math.max(presentation.contentWidth, viewportWidth)}px`} h={`${Math.max(rowCount * rowHeight + presentation.contentInset * 2, viewportHeight)}px`} position="relative">
        {/* Native scrolling moves only the spacer. Keep the visible scene pinned until the
            same commit updates its translation, connector paths, ports, and external edges. */}
        <Box data-testid="sub-element-scroll-viewport" position="sticky" top={0} left={0} w={`${viewportWidth}px`} h={`${viewportHeight}px`} overflow="clip">
          <Box h={`${rowCount * rowHeight + presentation.contentInset * 2}px`} w={`${presentation.contentWidth}px`} position="absolute" left={0} top={0} transform={`translate(${-scrollLeft}px, ${-scrollTop}px)`}>
      {detail !== 'structure' && kind === 'code' && source?.status === 'error' && <Button size="xs" m={3} onClick={() => { requestSubSource(data, true); refresh() }}>Retry source</Button>}
        <SubElementSymbolRegions presentation={presentation} scrollTop={scrollTop} interactions={interactions} />
        {renderedRows}
          </Box>
        </Box>
      </Box>
    </Box>
    {bodySlotHandles}
    <SubElementSymbolPorts textless={detail === 'structure'} data={data} presentation={presentation} width={width} height={height} scrollTop={scrollTop} scrollLeft={scrollLeft} canEdit={canEdit} interactions={interactions} onReveal={index => {
      // Overflow rows overlay the document: leave room for the top band so the
      // revealed symbol doesn't remain hidden behind the same control.
      if (scrollRef.current) scrollRef.current.scrollTop = Math.max(0, index * rowHeight - bannerHeight)
    }} />
    </ElementContainer>}
    <SubElementLayoutButtons detail={detail} onChange={setDetail} connectedOnly={appearance.connectedOnly}
      onConnectedOnlyChange={() => writeSourcePreviewAppearance(viewId ?? data.view_id, data.element_id, { ...appearance, connectedOnly: !appearance.connectedOnly })} />
    {detail !== 'overview' && detail !== 'structure' && <SubElementTextControls fontSize={appearance.fontSize} lineNumbers={lineNumbers}
      onFontSize={fontSize => writeSourcePreviewAppearance(viewId ?? data.view_id, data.element_id, { ...appearance, fontSize })}
      onLineNumbers={() => writeSourcePreviewAppearance(viewId ?? data.view_id, data.element_id, { ...appearance, lineNumbers: !lineNumbers })} />}
    {detail === 'overview' && (data.connectedHandleIds ?? []).filter(id => /-(member|line|symbol)-/.test(id)).map(id => <Handle key={id} id={id} type="source" position={id.startsWith('left') ? Position.Left : Position.Right} style={{ top: height / 2, opacity: 0, pointerEvents: 'none' }} />)}
    {detail === 'overview' && (['left', 'right'] as const).map(side => <ElementReconnectZone key={side}
      candidate={reconnectCandidates.find(candidate => candidate.handleId.startsWith(`${side}-`) && /-(member|line|symbol)-/.test(candidate.handleId))}
      position={side === 'left' ? Position.Left : Position.Right} anchorStyle={{ top: height / 2 }} />)}
  </Box>
}
