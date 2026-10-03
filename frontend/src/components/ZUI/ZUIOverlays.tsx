import {
  Badge,
  Box,
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  Button,
  Divider,
  HStack,
  Icon,
  Image as ChakraImage,
  Popover,
  PopoverArrow,
  PopoverBody,
  PopoverContent,
  PopoverHeader,
  PopoverTrigger,
  Portal,
  Text,
  VStack,
} from '@chakra-ui/react'
import { ExternalLinkIcon } from '@chakra-ui/icons'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import type { HoveredItem } from './types'
import type { PathItem } from './camera'

const MAX_PROXY_HOVER_VIEW_LINKS = 5
const BREADCRUMB_MAX_LABEL_WIDTH = 200
const BREADCRUMB_DESKTOP_RESERVE = 320

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
}: {
  initialized: boolean
  isMobileLayout: boolean
  currentPath: PathItem[]
  onZoomToPathItem: (item: PathItem) => void
}) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [overflowing, setOverflowing] = useState(false)
  const [availableWidth, setAvailableWidth] = useState<number | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const measureRef = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    if (typeof window === 'undefined' || typeof window.getComputedStyle !== 'function') return
    const container = containerRef.current
    const measure = measureRef.current
    if (!container || !measure) return
    const update = () => {
      const maxWidth = Number.parseFloat(window.getComputedStyle(container).maxWidth)
      const naturalWidth = measure.getBoundingClientRect().width
      setAvailableWidth(Number.isFinite(maxWidth) ? maxWidth : null)
      setOverflowing(Number.isFinite(maxWidth) && naturalWidth > maxWidth + 1)
    }
    update()
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', update)
      return () => window.removeEventListener('resize', update)
    }
    const observer = new ResizeObserver(update)
    observer.observe(container)
    observer.observe(measure)
    return () => observer.disconnect()
  }, [currentPath, initialized, isMobileLayout])

  if (!initialized || currentPath.length === 0) return null

  const lastIndex = currentPath.length - 1
  const truncated = overflowing && currentPath.length > 2
  const showMenu = truncated && menuOpen
  const collapsedLabelMaxW = availableWidth !== null
    ? Math.max(56, Math.floor((availableWidth - 88) / 2))
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
        maxW={isMobileLayout ? 'calc(100vw - 32px)' : `max(160px, calc(50vw - ${BREADCRUMB_DESKTOP_RESERVE}px))`}
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
        position="fixed"
        top="-10000px"
        left={0}
        visibility="hidden"
        pointerEvents="none"
        aria-hidden="true"
        whiteSpace="nowrap"
      >
        <Breadcrumb spacing="8px" separator={separator}>
          {currentPath.map((item) => (
            <BreadcrumbItem key={item.id}>
              <BreadcrumbLink display="flex" alignItems="center" gap={1.5} fontSize="xs">
                {renderLabel(item)}
              </BreadcrumbLink>
            </BreadcrumbItem>
          ))}
        </Breadcrumb>
      </Box>
    </>
  )
}

export function ZUIHoverPopover({
  hoveredItem,
  hoveredScreenRect,
  isHoveredItemFullyVisible,
  onHoverLock,
}: {
  hoveredItem: HoveredItem | null
  hoveredScreenRect: { sx: number; sy: number; sw: number; sh: number } | null
  isHoveredItemFullyVisible: boolean
  onHoverLock: (locked: boolean) => void
}) {
  const isOpen = isHoveredItemFullyVisible

  useEffect(() => {
    if (!isOpen) onHoverLock(false)
    return () => onHoverLock(false)
  }, [isOpen, onHoverLock])

  return (
    <Popover
      isOpen={isOpen}
      placement="right-start"
      closeOnBlur={false}
      gutter={12}
      isLazy
    >
      <PopoverTrigger>
        <Box
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
          data-testid="zui-hover-popover"
          bg="glass.bg"
          border="1px solid"
          borderColor="glass.border"
          boxShadow="panel"
          borderRadius="md"
          width="232px"
          _focus={{ boxShadow: 'none' }}
          pointerEvents="auto"
          onMouseEnter={() => onHoverLock(true)}
          onMouseLeave={() => onHoverLock(false)}
        >
          <PopoverArrow bg="glass.bg" />
          {hoveredItem?.type === 'node' && (
            <>
              <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.100" px={3} py={2}>
                <HStack spacing={2} align="center">
                  {hoveredItem.data.logoUrl && (
                    <Box flexShrink={0}>
                      <ChakraImage src={hoveredItem.data.logoUrl} boxSize="18px" objectFit="contain" />
                    </Box>
                  )}
                  <VStack align="start" spacing={0} flex={1} overflow="hidden">
                    <Text fontWeight="700" fontSize="13px" isTruncated width="100%" color="gray.50" lineHeight="1.15">
                      {hoveredItem.data.label}
                    </Text>
                  </VStack>
                </HStack>
              </PopoverHeader>
              <PopoverBody px={3} py={2.5}>
                <VStack align="stretch" spacing={2}>
                  {hoveredItem.data.technology && (
                    <Box>
                      <Text fontSize="11px" color="gray.300" noOfLines={1}>
                        {hoveredItem.data.technology}
                      </Text>
                    </Box>
                  )}
                  {hoveredItem.data.description && (
                    <Box>
                      <Text fontSize="11px" color="gray.400" noOfLines={2} lineHeight="1.35">
                        {hoveredItem.data.description}
                      </Text>
                    </Box>
                  )}
                  {hoveredItem.data.linkedDiagramId && (
                    <Box>
                      <Text fontSize="11px" color="var(--accent)" fontWeight="600" noOfLines={1}>
                        {'⊞'} {hoveredItem.data.linkedDiagramLabel}
                      </Text>
                    </Box>
                  )}
                  <Divider borderColor="whiteAlpha.100" />
                  <Button
                    as={RouterLink}
                    to={hoveredItem.data.isPortal
                      ? `/views/${hoveredItem.data.linkedDiagramId}`
                      : `/views/${hoveredItem.data.diagramId}?element=${hoveredItem.data.elementId}`}
                    size="xs"
                    variant="outline"
                    h="28px"
                    width="full"
                    bg="rgba(var(--accent-rgb), 0.08)"
                    borderColor="rgba(var(--accent-rgb), 0.24)"
                    color="var(--accent)"
                    fontWeight="700"
                    rightIcon={<ExternalLinkIcon />}
                    _hover={{ bg: 'rgba(var(--accent-rgb), 0.14)', borderColor: 'rgba(var(--accent-rgb), 0.36)' }}
                    _active={{ bg: 'rgba(var(--accent-rgb), 0.18)' }}
                    onClick={(e) => e.stopPropagation()}
                  >
                    {hoveredItem.data.isPortal ? 'Open Diagram' : 'Open in Editor'}
                  </Button>
                </VStack>
              </PopoverBody>
            </>
          )}
          {hoveredItem?.type === 'edge' && hoveredItem.data.isProxy && hoveredItem.data.details && (
            <>
              <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.200" px={4} py={3}>
                <VStack align="start" spacing={0}>
                  <Text fontWeight="600" fontSize="sm" color="white">
                    Off-View Connector
                  </Text>
                  <Badge colorScheme="blue" variant="subtle" fontSize="2xs">
                    {hoveredItem.data.details.count} connector{hoveredItem.data.details.count === 1 ? '' : 's'}
                  </Badge>
                </VStack>
              </PopoverHeader>
              <PopoverBody px={4} py={3}>
                <VStack align="start" spacing={3}>
                  <VStack align="start" spacing={1}>
                    <Text color="gray.400" fontSize="2xs" fontWeight="600" letterSpacing="wider">BETWEEN</Text>
                    <Text fontSize="xs" color="gray.200">
                      {hoveredItem.data.details.sourceAnchorName} &rarr; {hoveredItem.data.details.targetAnchorName}
                    </Text>
                    <Text fontSize="xs" color="gray.400">{hoveredItem.data.details.label}</Text>
                  </VStack>
                  <VStack align="start" spacing={1} width="full">
                    <Text color="gray.400" fontSize="2xs" fontWeight="600" letterSpacing="wider">UNDERLYING PATHS</Text>
                    {hoveredItem.data.details.connectors.slice(0, 4).map((leaf, index) => (
                      <Text key={`${leaf.connector.id}-${index}`} fontSize="xs" color="gray.200">
                        {leaf.source.actualElementName} &rarr; {leaf.target.actualElementName}
                      </Text>
                    ))}
                    {hoveredItem.data.details.connectors.length > 4 && (
                      <Text fontSize="xs" color="gray.500">
                        +{hoveredItem.data.details.connectors.length - 4} more
                      </Text>
                    )}
                  </VStack>
                  <Divider borderColor="whiteAlpha.200" />
                  <VStack align="stretch" spacing={2} width="full">
                    {hoveredItem.data.details.ownerViewIds.slice(0, MAX_PROXY_HOVER_VIEW_LINKS).map((ownerViewId, index) => (
                      <Button
                        key={`${ownerViewId}-${index}`}
                        as={RouterLink}
                        to={`/views/${ownerViewId}`}
                        size="xs"
                        colorScheme="gray"
                        variant="solid"
                        width="full"
                        justifyContent="space-between"
                        rightIcon={<ExternalLinkIcon />}
                        onClick={(e) => e.stopPropagation()}
                      >
                        {hoveredItem.data.details!.ownerViewNames[index] ?? `Open View ${ownerViewId}`}
                      </Button>
                    ))}
                    {hoveredItem.data.details.ownerViewIds.length > MAX_PROXY_HOVER_VIEW_LINKS && (
                      <Text fontSize="xs" color="gray.500" textAlign="center">
                        +{hoveredItem.data.details.ownerViewIds.length - MAX_PROXY_HOVER_VIEW_LINKS} more view{hoveredItem.data.details.ownerViewIds.length - MAX_PROXY_HOVER_VIEW_LINKS === 1 ? '' : 's'}
                      </Text>
                    )}
                  </VStack>
                  <Divider borderColor="whiteAlpha.200" />
                  <HStack width="full" spacing={2}>
                    <Button
                      as={RouterLink}
                      to={`/views/${hoveredItem.data.details!.connectors[0]?.source.anchorViewId ?? hoveredItem.data.diagramId}?element=${hoveredItem.data.sourceObjId}`}
                      size="xs"
                      colorScheme="gray"
                      variant="solid"
                      flex={1}
                      rightIcon={<ExternalLinkIcon />}
                      onClick={(e) => e.stopPropagation()}
                    >
                      Open Source
                    </Button>
                    <Button
                      as={RouterLink}
                      to={`/views/${hoveredItem.data.details!.connectors[0]?.target.anchorViewId ?? hoveredItem.data.diagramId}?element=${hoveredItem.data.targetObjId}`}
                      size="xs"
                      colorScheme="teal"
                      variant="solid"
                      flex={1}
                      rightIcon={<ExternalLinkIcon />}
                      onClick={(e) => e.stopPropagation()}
                    >
                      Open Target
                    </Button>
                  </HStack>
                </VStack>
              </PopoverBody>
            </>
          )}
          {hoveredItem?.type === 'edge' && !hoveredItem.data.isProxy && (
            <>
              <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.200" px={4} py={3}>
                <VStack align="start" spacing={0}>
                  <Text fontWeight="600" fontSize="sm" color="white">
                    {hoveredItem.data.label}
                  </Text>
                  <Badge colorScheme={hoveredItem.data.isPortalConn ? 'purple' : 'orange'} variant="subtle" fontSize="2xs">
                    {hoveredItem.data.isPortalConn ? 'Portal Connection' : 'Connection'}
                  </Badge>
                </VStack>
              </PopoverHeader>
              <PopoverBody px={4} py={3}>
                <VStack align="start" spacing={3}>
                  <VStack align="start" spacing={1}>
                    <Text color="gray.400" fontSize="2xs" fontWeight="600" letterSpacing="wider">BETWEEN</Text>
                    <Text fontSize="xs" color="gray.200">{hoveredItem.data.sourceId} & {hoveredItem.data.targetId}</Text>
                  </VStack>
                  <Divider borderColor="whiteAlpha.200" />

                  {hoveredItem.data.isPortalConn ? (
                    <>
                      <Button
                        as={RouterLink}
                        to={`/views/${hoveredItem.data.diagramId}`}
                        size="xs"
                        colorScheme="gray"
                        variant="solid"
                        width="full"
                        rightIcon={<ExternalLinkIcon />}
                        onClick={(e) => e.stopPropagation()}
                      >
                        Open {hoveredItem.data.sourceId}
                      </Button>
                      <Button
                        as={RouterLink}
                        to={`/views/${hoveredItem.data.targetDiagId}`}
                        size="xs"
                        colorScheme="teal"
                        variant="solid"
                        width="full"
                        rightIcon={<ExternalLinkIcon />}
                        onClick={(e) => e.stopPropagation()}
                      >
                        Open {hoveredItem.data.targetId}
                      </Button>
                    </>
                  ) : (
                    <>
                      <Button
                        as={RouterLink}
                        to={`/views/${hoveredItem.data.diagramId}?element=${hoveredItem.data.sourceObjId}`}
                        size="xs"
                        colorScheme="gray"
                        variant="solid"
                        width="full"
                        rightIcon={<ExternalLinkIcon />}
                        onClick={(e) => e.stopPropagation()}
                      >
                        Go to {hoveredItem.data.sourceId}
                      </Button>
                      <Button
                        as={RouterLink}
                        to={`/views/${hoveredItem.data.diagramId}?element=${hoveredItem.data.targetObjId}`}
                        size="xs"
                        colorScheme="teal"
                        variant="solid"
                        width="full"
                        rightIcon={<ExternalLinkIcon />}
                        onClick={(e) => e.stopPropagation()}
                      >
                        Go to {hoveredItem.data.targetId}
                      </Button>
                    </>
                  )}
                </VStack>
              </PopoverBody>
            </>
          )}
          {hoveredItem?.type === 'group' && (
            <>
              <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.200" px={4} py={3}>
                <VStack align="start" spacing={0}>
                  <Text fontWeight="600" fontSize="sm" color="white">
                    {hoveredItem.data.label}
                  </Text>
                  <Badge colorScheme="purple" variant="subtle" fontSize="2xs">
                    Diagram Group
                  </Badge>
                </VStack>
              </PopoverHeader>
              <PopoverBody px={4} py={3}>
                <VStack align="start" spacing={3}>
                  {hoveredItem.data.description && (
                    <Box>
                      <Text color="gray.400" fontSize="xs" fontWeight="600" mb={0.5} letterSpacing="wider">DESCRIPTION</Text>
                      <Text fontSize="xs" color="gray.200" noOfLines={4}>{hoveredItem.data.description}</Text>
                    </Box>
                  )}
                  <Text fontSize="xs" color="gray.300">
                    Root level diagram containing {hoveredItem.data.nodes.length} elements and {hoveredItem.data.edges.length} connections.
                  </Text>
                  <Divider borderColor="whiteAlpha.200" />
                  <Button
                    as={RouterLink}
                    to={`/views/${hoveredItem.data.diagramId}`}
                    size="xs"
                    colorScheme="teal"
                    variant="solid"
                    width="full"
                    rightIcon={<ExternalLinkIcon />}
                    onClick={(e) => e.stopPropagation()}
                  >
                    Open Diagram
                  </Button>
                </VStack>
              </PopoverBody>
            </>
          )}
        </PopoverContent>
      </Portal>
    </Popover>
  )
}
