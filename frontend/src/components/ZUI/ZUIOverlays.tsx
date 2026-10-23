import {
  Badge,
  Box,
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  Button,
  HStack,
  Icon,
  Image as ChakraImage,
  Popover,
  PopoverBody,
  PopoverContent,
  PopoverHeader,
  PopoverTrigger,
  Portal,
  Text,
  VStack,
} from '@chakra-ui/react'
import { EditorIcon } from '../Icons'
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import {
  canvasPopoverContentStyles,
  canvasPopoverInsetCardStyles,
  canvasPopoverKindLabelStyles,
  canvasPopoverNameButtonStyles,
  canvasPopoverPillStyles,
} from '../../styles/surfaces'
import type { HoveredItem } from './types'
import type { ProxyConnectorDetails } from '../../crossBranch/types'
import type { ZUIHiddenProxyBadge } from '../../crossBranch/resolve'
import type { PathItem } from './camera'
import { EDGE_CHANGE_META } from './edgeChange'

const BREADCRUMB_MAX_LABEL_WIDTH = 200
const BREADCRUMB_HORIZONTAL_GUTTER = 32

function directionGlyph(direction?: string): string {
  switch (direction) {
    case 'backward':
      return '←'
    case 'both':
    case 'bidirectional':
      return '↔'
    case 'none':
      return '—'
    case 'forward':
    default:
      return '→'
  }
}

function isDistinctProxySummaryLabel(label: string | undefined, count: number): boolean {
  const trimmed = label?.trim()
  if (!trimmed) return false
  const lower = trimmed.toLowerCase()
  return (
    lower !== 'cross-view' &&
    lower !== 'off-view connector' &&
    lower !== 'connectors' &&
    lower !== `${count} connector` &&
    lower !== `${count} connectors`
  )
}

function PopoverNameButton({
  label,
  to,
  isDisabled = false,
  size = 'sm',
}: {
  label: string
  to?: string
  isDisabled?: boolean
  size?: 'sm' | 'md'
}) {
  const canNavigate = Boolean(to) && !isDisabled
  return (
    <Button
      as={canNavigate ? RouterLink : undefined}
      isDisabled={!canNavigate}
      to={canNavigate ? to : undefined}
      size="xs"
      variant="outline"
      sx={{
        ...canvasPopoverNameButtonStyles,
        fontSize: size === 'md' ? '12px' : '11px',
        fontWeight: size === 'md' ? '700' : '600',
        minH: size === 'md' ? '24px' : '22px',
        px: size === 'md' ? 2.5 : 2,
        '& .chakra-button__icon': { opacity: size === 'md' ? 1 : 0 },
      }}
      rightIcon={<EditorIcon size={10} />}
      onClick={(e) => e.stopPropagation()}
      title={label}
    >
      <Text as="span" isTruncated>
        {label}
      </Text>
    </Button>
  )
}

function PopoverEndpointRoute({
  source,
  sourceTo,
  sourceDisabled,
  target,
  targetTo,
  targetDisabled,
  direction,
  subtitle,
  compact = false,
}: {
  source: string
  sourceTo?: string
  sourceDisabled?: boolean
  target: string
  targetTo?: string
  targetDisabled?: boolean
  direction?: string
  subtitle?: string | null
  compact?: boolean
}) {
  return (
    <Box
      px={compact ? 2 : 2.5}
      py={compact ? 1.5 : 2}
      sx={canvasPopoverInsetCardStyles}
      borderRadius={compact ? 'md' : 'lg'}
    >
      <HStack spacing={1.5} align="center" justify="space-between" minW={0}>
        <Box flex={1} minW={0} display="flex" justifyContent="flex-start">
          <PopoverNameButton
            label={source}
            to={sourceTo}
            isDisabled={sourceDisabled}
            size="sm"
          />
        </Box>
        <Text
          as="span"
          fontSize="11px"
          fontWeight="700"
          color="var(--accent)"
          flexShrink={0}
          lineHeight={1}
        >
          {directionGlyph(direction)}
        </Text>
        <Box flex={1} minW={0} display="flex" justifyContent="flex-end">
          <PopoverNameButton
            label={target}
            to={targetTo}
            isDisabled={targetDisabled}
            size="sm"
          />
        </Box>
      </HStack>
      {subtitle && (
        <Text
          fontSize="10px"
          color="gray.400"
          mt={1}
          noOfLines={1}
          lineHeight="1.3"
        >
          {subtitle}
        </Text>
      )}
    </Box>
  )
}

function BreadcrumbItemIcon({ item }: { item: PathItem }) {
  return (
    <>
      {item.type === 'group' && (
        <Icon viewBox="0 0 24 24" boxSize={3} fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
          <polyline points="9 22 9 12 15 12 15 22" />
        </Icon>
      )}
      {item.isCircular && (
        <Icon viewBox="0 0 24 24" boxSize={3.5} fill="none" stroke="currentColor" strokeWidth="3.5">
          <path d="M20 4l-4 4 4 4" />
          <path d="M16 8h-4a8 8 0 1 0 8 8" />
        </Icon>
      )}
    </>
  )
}

export function ZUIBreadcrumb({
  initialized,
  isMobileLayout,
  currentPath,
  onZoomToPathItem,
  breadcrumbBoundaryRef,
}: {
  initialized: boolean
  isMobileLayout: boolean
  currentPath: PathItem[]
  onZoomToPathItem: (item: PathItem) => void
  breadcrumbBoundaryRef?: RefObject<HTMLDivElement>
}) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [overflowing, setOverflowing] = useState(false)
  const [availableWidth, setAvailableWidth] = useState<number | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const measureRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    if (typeof window === 'undefined') return
    const container = containerRef.current
    const measure = measureRef.current
    const canvas = container?.parentElement
    const boundary = breadcrumbBoundaryRef?.current
    if (!container || !canvas || !measure) return
    const update = () => {
      let maxWidth = Math.max(0, canvas.clientWidth - BREADCRUMB_HORIZONTAL_GUTTER)
      if (boundary) {
        const breadcrumbRect = container.getBoundingClientRect()
        const boundaryRect = boundary.getBoundingClientRect()
        if (boundaryRect.top < breadcrumbRect.bottom && boundaryRect.bottom > breadcrumbRect.top) {
          maxWidth = Math.max(0, Math.min(maxWidth, boundaryRect.left - breadcrumbRect.left - BREADCRUMB_HORIZONTAL_GUTTER / 2))
        }
      }
      const naturalWidth = measure.getBoundingClientRect().width
      setAvailableWidth(maxWidth)
      setOverflowing(Math.ceil(naturalWidth) > Math.floor(maxWidth))
    }
    update()
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', update)
      return () => window.removeEventListener('resize', update)
    }
    const observer = new ResizeObserver(update)
    observer.observe(canvas)
    observer.observe(measure)
    if (boundary) observer.observe(boundary)
    return () => observer.disconnect()
  }, [breadcrumbBoundaryRef, currentPath, initialized, isMobileLayout])

  if (!initialized || currentPath.length === 0) return null

  const lastIndex = currentPath.length - 1
  const truncated = overflowing && currentPath.length > 2
  const showMenu = truncated && menuOpen
  const collapsedLabelMaxW = availableWidth !== null
    ? Math.max(0, Math.floor(availableWidth / 2))
    : BREADCRUMB_MAX_LABEL_WIDTH
  const separator = <Text color="whiteAlpha.400" fontSize="xs">/</Text>

  const renderLabel = (item: PathItem, maxLabelWidth = BREADCRUMB_MAX_LABEL_WIDTH) => (
    <>
      <BreadcrumbItemIcon item={item} />
      <Text as="span" display="inline-block" isTruncated minW={0} maxW={`${maxLabelWidth}px`} verticalAlign="middle">
        {item.label}
      </Text>
    </>
  )

  const renderCrumbLink = (item: PathItem, index: number, maxLabelWidth?: number) => {
    const isLast = index === lastIndex
    return (
      <BreadcrumbLink
        onClick={() => onZoomToPathItem(item)}
        color={isLast ? 'var(--accent)' : 'gray.400'}
        fontSize="xs"
        fontWeight={isLast ? '600' : 'normal'}
        _hover={{ color: 'var(--accent)', textDecoration: 'none' }}
        display="flex"
        alignItems="center"
        gap={1.5}
        minW={0}
      >
        {renderLabel(item, maxLabelWidth)}
      </BreadcrumbLink>
    )
  }

  return (
    <>
      <Box
        ref={containerRef}
        data-testid="zui-breadcrumb"
        position="absolute"
        top={isMobileLayout ? '66px' : 4}
        left={4}
        zIndex={10}
        maxW={availableWidth !== null ? `${availableWidth}px` : `calc(100% - ${BREADCRUMB_HORIZONTAL_GUTTER}px)`}
        pointerEvents="auto"
        onMouseEnter={() => setMenuOpen(true)}
        onMouseLeave={() => setMenuOpen(false)}
      >
        <Box className="glass" borderRadius="lg" px={3} py={1.5} overflow="hidden" whiteSpace="nowrap">
          <Breadcrumb spacing="8px" separator={separator}>
            {truncated ? (
              <>
                <BreadcrumbItem minW={0}>{renderCrumbLink(currentPath[0], 0, collapsedLabelMaxW)}</BreadcrumbItem>
                <BreadcrumbItem>
                  <BreadcrumbLink
                    aria-label="Show full path"
                    onClick={() => setMenuOpen((open) => !open)}
                    color="gray.500"
                    fontSize="xs"
                    _hover={{ color: 'var(--accent)', textDecoration: 'none' }}
                  >
                    …
                  </BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbItem isCurrentPage minW={0}>{renderCrumbLink(currentPath[lastIndex], lastIndex, collapsedLabelMaxW)}</BreadcrumbItem>
              </>
            ) : (
              currentPath.map((item, idx) => (
                <BreadcrumbItem key={item.id} isCurrentPage={idx === lastIndex}>
                  {renderCrumbLink(item, idx)}
                </BreadcrumbItem>
              ))
            )}
          </Breadcrumb>
          {currentPath[lastIndex]?.isCircular && (
            <Text mt={1.5} color="var(--accent)" fontSize="2xs" fontWeight="500" letterSpacing="wide">
              Recursive reference.
            </Text>
          )}
        </Box>

        {showMenu && (
          <Box position="absolute" top="100%" left={0} pt={2} minW="220px" maxW="340px" zIndex={20}>
            <Box
              data-testid="zui-breadcrumb-menu"
              className="glass"
              border="1px solid"
              borderColor="whiteAlpha.100"
              borderRadius="lg"
              boxShadow="0 12px 32px rgba(0,0,0,0.45)"
              p={2}
              maxH="60vh"
              overflowY="auto"
            >
              <Box borderLeft="2px solid" borderColor="whiteAlpha.100" pl={1.5} ml={1}>
                <VStack align="stretch" spacing={0.5}>
                  {currentPath.map((item, idx) => {
                    const isLast = idx === lastIndex
                    return (
                      <Box
                        key={item.id}
                        as="button"
                        type="button"
                        textAlign="left"
                        display="flex"
                        alignItems="center"
                        gap={1.5}
                        px={2}
                        py={1}
                        borderRadius="md"
                        fontSize="xs"
                        color={isLast ? 'var(--accent)' : 'gray.300'}
                        fontWeight={isLast ? '600' : 'normal'}
                        bg={isLast ? 'rgba(var(--accent-rgb), 0.08)' : 'transparent'}
                        _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                        onClick={() => onZoomToPathItem(item)}
                      >
                        {renderLabel(item)}
                      </Box>
                    )
                  })}
                </VStack>
              </Box>
            </Box>
          </Box>
        )}
      </Box>

      <Box
        ref={measureRef}
        className="glass"
        borderRadius="lg"
        px={3}
        py={1.5}
        width="max-content"
        position="fixed"
        top="-10000px"
        left={0}
        visibility="hidden"
        pointerEvents="none"
        aria-hidden="true"
        whiteSpace="nowrap"
      >
        <Breadcrumb spacing="8px" separator={separator}>
          {currentPath.map((item, idx) => (
            <BreadcrumbItem key={item.id} isCurrentPage={idx === lastIndex}>
              {renderCrumbLink(item, idx)}
            </BreadcrumbItem>
          ))}
        </Breadcrumb>
      </Box>
    </>
  )
}

function UnderlyingConnectors({ details }: { details: ProxyConnectorDetails }) {
  return (
    <VStack align="stretch" spacing={1} width="full">
      <Text color="gray.500" fontSize="9px" fontWeight="700" letterSpacing="0.08em" textTransform="uppercase">
        Underlying Connectors · {details.connectors.length}
      </Text>
      <VStack align="stretch" spacing={1} maxH="240px" overflowY="auto" data-zui-native-wheel="true">
        {details.connectors.map((leaf, index) => {
          const sourceViewId = leaf.source.placementViewId ?? leaf.source.anchorViewId ?? leaf.ownerViewId
          const targetViewId = leaf.target.placementViewId ?? leaf.target.anchorViewId ?? leaf.ownerViewId
          return (
            <PopoverEndpointRoute
              key={`${leaf.ownerViewId}-${leaf.connector.id}-${index}`}
              source={leaf.source.actualElementName}
              sourceTo={`/views/${sourceViewId}?element=${leaf.source.actualElementId}`}
              sourceDisabled={sourceViewId < 0}
              target={leaf.target.actualElementName}
              targetTo={`/views/${targetViewId}?element=${leaf.target.actualElementId}`}
              targetDisabled={targetViewId < 0}
              direction={leaf.connector.direction}
              subtitle={leaf.connector.label?.trim() || leaf.connector.relationship?.trim() || null}
              compact
            />
          )
        })}
      </VStack>
    </VStack>
  )
}

export function ZUIHoverPopover({
  hoveredItem,
  hiddenProxyBadges = [],
  hoveredScreenRect,
  isHoveredItemFullyVisible,
  onHoverLock,
  onPopoverElement,
}: {
  hoveredItem: HoveredItem | null
  hiddenProxyBadges?: ZUIHiddenProxyBadge[]
  hoveredScreenRect: { sx: number; sy: number; sw: number; sh: number } | null
  isHoveredItemFullyVisible: boolean
  onHoverLock: (locked: boolean) => void
  onPopoverElement?: (element: HTMLElement | null) => void
}) {
  const isOpen = isHoveredItemFullyVisible
  const mergedProxyDetails = hoveredItem?.type === 'edge' && !hoveredItem.data.isPortalConn && !hoveredItem.data.isProxy
    ? hiddenProxyBadges.find((badge) => (
      (badge.sourceAnchorElementId === hoveredItem.data.sourceObjId && badge.targetAnchorElementId === hoveredItem.data.targetObjId) ||
      (badge.targetAnchorElementId === hoveredItem.data.sourceObjId && badge.sourceAnchorElementId === hoveredItem.data.targetObjId)
    ))?.details
    : undefined
  const contentElementRef = useRef<HTMLElement | null>(null)
  const isOpenRef = useRef(isOpen)
  isOpenRef.current = isOpen
  // Keep the ref callback stable across opening/closing: the motion component
  // retains its initial forwarded ref. Report visibility separately.
  const registerPopoverElement = useCallback((element: HTMLElement | null) => {
    contentElementRef.current = element
    onPopoverElement?.(isOpenRef.current ? element : null)
  }, [onPopoverElement])
  useLayoutEffect(() => {
    onPopoverElement?.(isOpen ? contentElementRef.current : null)
  }, [isOpen, onPopoverElement])

  useEffect(() => {
    if (!isOpen) onHoverLock(false)
    return () => onHoverLock(false)
  }, [isOpen, onHoverLock])

  return (
    <Popover
      isOpen={isOpen}
      placement={hoveredItem?.type === 'edge' ? 'bottom-start' : 'right-start'}
      autoFocus={false}
      returnFocusOnClose={false}
      modifiers={[{ name: 'preventOverflow', options: { altAxis: true, padding: 8 } }]}
      closeOnBlur={false}
      gutter={12}
      isLazy
    >
      <PopoverTrigger>
        <Box
          data-testid="zui-hover-anchor"
          position="absolute"
          left={hoveredScreenRect?.sx ?? 0}
          top={hoveredScreenRect?.sy ?? 0}
          width={hoveredScreenRect?.sw ?? 0}
          height={hoveredScreenRect?.sh ?? 0}
          pointerEvents="none"
        />
      </PopoverTrigger>
      <Portal>
        <PopoverContent
          ref={registerPopoverElement}
          data-testid="zui-hover-popover"
          sx={canvasPopoverContentStyles}
          width="264px"
          pointerEvents="auto"
          onMouseEnter={() => onHoverLock(true)}
          onMouseLeave={() => onHoverLock(false)}
        >
          {hoveredItem?.type === 'node' && (() => {
            const hasNodeDetails = Boolean(
              hoveredItem.data.changeOverlay ||
              hoveredItem.data.technology ||
              hoveredItem.data.description,
            )
            const rawKind = hoveredItem.data.kind !== undefined ? hoveredItem.data.kind : hoveredItem.data.type
            const kindLabel = rawKind?.trim() || ''
            const primaryHref = hoveredItem.data.isPortal
              ? `/views/${hoveredItem.data.linkedDiagramId}`
              : `/views/${hoveredItem.data.diagramId}?element=${hoveredItem.data.elementId}`

            return (
              <>
                <PopoverHeader
                  borderBottom={hasNodeDetails ? '1px solid' : 'none'}
                  borderColor="whiteAlpha.100"
                  px={3.5}
                  py={2.5}
                >
                  <HStack spacing={2} align="center" justify="space-between">
                    <HStack spacing={2} align="center" flex={1} minW={0}>
                      {hoveredItem.data.logoUrl && (
                        <Box
                          flexShrink={0}
                          w="22px"
                          h="22px"
                          rounded="md"
                          bg="whiteAlpha.50"
                          border="1px solid"
                          borderColor="whiteAlpha.100"
                          display="flex"
                          alignItems="center"
                          justifyContent="center"
                        >
                          <ChakraImage src={hoveredItem.data.logoUrl} boxSize="14px" objectFit="contain" />
                        </Box>
                      )}
                      <PopoverNameButton
                        label={hoveredItem.data.label}
                        to={primaryHref}
                        isDisabled={hoveredItem.data.diagramId < 0}
                        size="md"
                      />
                    </HStack>
                    {kindLabel && (
                      <Text sx={canvasPopoverKindLabelStyles} isTruncated title={kindLabel}>
                        {kindLabel}
                      </Text>
                    )}
                  </HStack>
                </PopoverHeader>
                {hasNodeDetails && (
                  <PopoverBody px={3.5} py={2.5}>
                    <VStack align="stretch" spacing={2}>
                      {hoveredItem.data.changeOverlay && (
                        <Box data-testid="zui-change-details" px={2.5} py={2} sx={canvasPopoverInsetCardStyles}>
                          <HStack justify="space-between" align="center" spacing={2}>
                            <Text fontSize="11px" fontWeight="600" color="gray.100" isTruncated>
                              {hoveredItem.data.changeOverlay.change} · {hoveredItem.data.changeOverlay.path}
                            </Text>
                            {hoveredItem.data.changeOverlay.linesAdded !== undefined && (
                              <HStack spacing={1.5} fontSize="10px" fontWeight="700" flexShrink={0}>
                                <Text color="green.300">+{hoveredItem.data.changeOverlay.linesAdded}</Text>
                                <Text color="red.300">−{hoveredItem.data.changeOverlay.linesRemoved ?? 0}</Text>
                              </HStack>
                            )}
                          </HStack>
                          {hoveredItem.data.changeOverlay.symbols.slice(0, 6).map((symbol, i) => (
                            <Text key={i} fontSize="10px" color="gray.300" isTruncated mt={0.5}>
                              {symbol}
                            </Text>
                          ))}
                          {hoveredItem.data.changeOverlay.symbols.length > 6 && (
                            <Text fontSize="10px" color="gray.400" mt={0.5}>
                              +{hoveredItem.data.changeOverlay.symbols.length - 6} more changed symbols
                            </Text>
                          )}
                        </Box>
                      )}
                      {hoveredItem.data.technology && (
                        <Text fontSize="11px" fontWeight="500" color="gray.300" noOfLines={1}>
                          {hoveredItem.data.technology}
                        </Text>
                      )}
                      {hoveredItem.data.description && (
                        <Text fontSize="11px" color="gray.400" noOfLines={3} lineHeight="1.4">
                          {hoveredItem.data.description}
                        </Text>
                      )}
                    </VStack>
                  </PopoverBody>
                )}
              </>
            )
          })()}
          {hoveredItem?.type === 'edge' && hoveredItem.data.isProxy && hoveredItem.data.details && (() => {
            const { details } = hoveredItem.data
            const summarySubtitle = isDistinctProxySummaryLabel(details.label, details.count)
              ? details.label
              : null
            const relationships = Array.from(new Set(
              details.connectors
                .map((leaf) => leaf.connector.relationship?.trim() || '')
                .filter(Boolean),
            ))
            const proxyKindLabel = relationships.length === 1 ? relationships[0] : ''
            const sourceAnchorViewId = details.connectors[0]?.source.anchorViewId ?? hoveredItem.data.diagramId
            const targetAnchorViewId = details.connectors[0]?.target.anchorViewId ?? hoveredItem.data.diagramId

            return (
              <>
                <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.100" px={3.5} py={2.5}>
                  <HStack spacing={2} align="center" justify="space-between">
                    <Text fontWeight="700" fontSize="13px" isTruncated color="gray.50" lineHeight="1.2">
                      Off-View Connector
                    </Text>
                    {proxyKindLabel && (
                      <Badge colorScheme="blue" variant="subtle" sx={canvasPopoverPillStyles}>
                        {proxyKindLabel}
                      </Badge>
                    )}
                  </HStack>
                </PopoverHeader>
                <PopoverBody px={3.5} py={2.5}>
                  <VStack align="stretch" spacing={2.5}>
                    <PopoverEndpointRoute
                      source={details.sourceAnchorName}
                      sourceTo={`/views/${sourceAnchorViewId}?element=${hoveredItem.data.sourceObjId}`}
                      sourceDisabled={sourceAnchorViewId < 0}
                      target={details.targetAnchorName}
                      targetTo={`/views/${targetAnchorViewId}?element=${hoveredItem.data.targetObjId}`}
                      targetDisabled={targetAnchorViewId < 0}
                      direction={hoveredItem.data.direction}
                      subtitle={summarySubtitle}
                    />
                    <UnderlyingConnectors details={details} />
                  </VStack>
                </PopoverBody>
              </>
            )
          })()}
          {hoveredItem?.type === 'edge' && !hoveredItem.data.isProxy && (() => {
            const connectorTitle = hoveredItem.data.label || (hoveredItem.data.isPortalConn ? 'Portal Connection' : 'Connector')
            const connectorHref = hoveredItem.data.id != null && hoveredItem.data.id > 0 && hoveredItem.data.diagramId > 0
              ? `/views/${hoveredItem.data.diagramId}?connector=${hoveredItem.data.id}`
              : undefined
            const sourceHref = hoveredItem.data.isPortalConn
              ? `/views/${hoveredItem.data.diagramId}`
              : `/views/${hoveredItem.data.diagramId}?element=${hoveredItem.data.sourceObjId}`
            const targetHref = hoveredItem.data.isPortalConn
              ? `/views/${hoveredItem.data.targetDiagId}`
              : `/views/${hoveredItem.data.diagramId}?element=${hoveredItem.data.targetObjId}`

            return (
              <>
                <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.100" px={3.5} py={2.5}>
                  <HStack spacing={2} align="center" justify="space-between">
                    {connectorHref ? (
                      <PopoverNameButton label={connectorTitle} to={connectorHref} size="md" />
                    ) : (
                      <Text fontWeight="700" fontSize="13px" isTruncated flex={1} color="gray.50" lineHeight="1.2">
                        {connectorTitle}
                      </Text>
                    )}
                    {hoveredItem.data.change && (
                      <Badge
                        data-testid="zui-edge-change"
                        bg={EDGE_CHANGE_META[hoveredItem.data.change].color}
                        color="black"
                        variant="solid"
                        sx={canvasPopoverPillStyles}
                      >
                        {EDGE_CHANGE_META[hoveredItem.data.change].label}
                      </Badge>
                    )}
                  </HStack>
                </PopoverHeader>
                <PopoverBody px={3.5} py={2.5}>
                  <VStack align="stretch" spacing={2}>
                    <PopoverEndpointRoute
                      source={hoveredItem.data.sourceId}
                      sourceTo={sourceHref}
                      sourceDisabled={hoveredItem.data.diagramId < 0}
                      target={hoveredItem.data.targetId}
                      targetTo={targetHref}
                      targetDisabled={hoveredItem.data.diagramId < 0}
                      direction={hoveredItem.data.direction}
                    />
                    {hoveredItem.data.description && (
                      <Text fontSize="11px" color="gray.400" noOfLines={3} lineHeight="1.4">
                        {hoveredItem.data.description}
                      </Text>
                    )}
                    {mergedProxyDetails && <UnderlyingConnectors details={mergedProxyDetails} />}
                  </VStack>
                </PopoverBody>
              </>
            )
          })()}
          {hoveredItem?.type === 'group' && (() => {
            const groupKindLabel = hoveredItem.data.levelLabel?.trim() || ''
            return (
              <>
                <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.100" px={3.5} py={2.5}>
                  <HStack spacing={2} align="center" justify="space-between">
                    <PopoverNameButton
                      label={hoveredItem.data.label}
                      to={`/views/${hoveredItem.data.diagramId}`}
                      isDisabled={hoveredItem.data.diagramId < 0}
                      size="md"
                    />
                    {groupKindLabel && (
                      <Badge colorScheme="purple" variant="subtle" sx={canvasPopoverPillStyles}>
                        {groupKindLabel}
                      </Badge>
                    )}
                  </HStack>
                </PopoverHeader>
                <PopoverBody px={3.5} py={2.5}>
                  <VStack align="stretch" spacing={2}>
                    {hoveredItem.data.description && (
                      <Text fontSize="11px" color="gray.400" noOfLines={3} lineHeight="1.4">
                        {hoveredItem.data.description}
                      </Text>
                    )}
                    <Text fontSize="11px" color="gray.300">
                      {hoveredItem.data.nodes.length} elements · {hoveredItem.data.edges.length} connections
                    </Text>
                  </VStack>
                </PopoverBody>
              </>
            )
          })()}
        </PopoverContent>
      </Portal>
    </Popover>
  )
}
