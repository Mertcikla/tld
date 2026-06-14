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
import { useEffect } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import type { ExploreDiffDetail } from '../../utils/exploreDiffLens'
import type { HoveredItem } from './types'
import type { PathItem } from './camera'

const MAX_PROXY_HOVER_VIEW_LINKS = 5

function diffColorScheme(change: string | undefined): 'green' | 'red' | 'yellow' | 'blue' {
  if (change === 'added') return 'green'
  if (change === 'deleted') return 'red'
  if (change === 'initialized') return 'blue'
  return 'yellow'
}

export function DiffDetailBlock({
  detail,
  onOpenSource,
}: {
  detail: ExploreDiffDetail | null
  onOpenSource: (detail: ExploreDiffDetail) => void
}) {
  if (!detail) return null
  const hasLines = detail.addedLines > 0 || detail.removedLines > 0
  return (
    <VStack align="stretch" spacing={2} mb={3}>
      <HStack spacing={2} minW={0}>
        <Badge colorScheme={diffColorScheme(detail.changeType)} variant="subtle" fontSize="2xs">
          {detail.changeType}
        </Badge>
        {hasLines && (
          <HStack spacing={1.5} fontSize="xs" fontFamily="mono">
            {detail.addedLines > 0 && <Text color="green.300">+{detail.addedLines}</Text>}
            {detail.removedLines > 0 && <Text color="red.300">-{detail.removedLines}</Text>}
          </HStack>
        )}
      </HStack>
      {detail.summary && (
        <Text fontSize="xs" color="gray.200" noOfLines={3}>{detail.summary}</Text>
      )}
      {detail.sourcePath && (
        <Text fontSize="11px" color="gray.500" fontFamily="mono" noOfLines={2}>{detail.sourcePath}</Text>
      )}
      {detail.sourcePath && (
        <Button
          size="xs"
          variant="outline"
          colorScheme="blue"
          alignSelf="flex-start"
          onClick={(event) => {
            event.stopPropagation()
            onOpenSource(detail)
          }}
        >
          Open Source
        </Button>
      )}
      <Divider borderColor="whiteAlpha.200" />
    </VStack>
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
  if (!initialized || currentPath.length === 0) return null
  return (
    <Box
      data-testid="zui-breadcrumb"
      position="absolute"
      top={isMobileLayout ? '66px' : 4}
      left={4}
      zIndex={10}
      className="glass"
      borderRadius="lg"
      px={3}
      py={1.5}
      pointerEvents="auto"
    >
      <Breadcrumb
        spacing="8px"
        separator={<Text color="whiteAlpha.400" fontSize="xs">/</Text>}
      >
        {currentPath.map((item, idx) => (
          <BreadcrumbItem key={item.id} isCurrentPage={idx === currentPath.length - 1}>
            <BreadcrumbLink
              onClick={() => onZoomToPathItem(item)}
              color={idx === currentPath.length - 1 ? 'var(--accent)' : 'gray.400'}
              fontSize="xs"
              fontWeight={idx === currentPath.length - 1 ? '600' : 'normal'}
              _hover={{ color: 'var(--accent)', textDecoration: 'none' }}
              display="flex"
              alignItems="center"
              gap={1.5}
            >
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
              {item.label}
            </BreadcrumbLink>
          </BreadcrumbItem>
        ))}
      </Breadcrumb>
      {currentPath[currentPath.length - 1]?.isCircular && (
        <Text mt={1.5} color="var(--accent)" fontSize="2xs" fontWeight="500" letterSpacing="wide">
          Recursive reference.
        </Text>
      )}
    </Box>
  )
}

export function ZUIHoverPopover({
  hoveredItem,
  hoveredScreenRect,
  isHoveredItemFullyVisible,
  hoveredDiffDetail,
  onOpenSource,
  onHoverLock,
}: {
  hoveredItem: HoveredItem | null
  hoveredScreenRect: { sx: number; sy: number; sw: number; sh: number } | null
  isHoveredItemFullyVisible: boolean
  hoveredDiffDetail: ExploreDiffDetail | null
  onOpenSource: (detail: ExploreDiffDetail) => void
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
                  <DiffDetailBlock detail={hoveredDiffDetail} onOpenSource={onOpenSource} />
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
                  <DiffDetailBlock detail={hoveredDiffDetail} onOpenSource={onOpenSource} />
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
