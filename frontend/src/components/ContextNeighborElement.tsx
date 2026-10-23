import { memo, useEffect, useMemo, useRef, useState } from 'react'
import { Handle, Position } from 'reactflow'
import {
  Badge,
  Box,
  Button,
  Divider,
  HStack,
  Popover,
  PopoverBody,
  PopoverContent,
  PopoverHeader,
  PopoverTrigger,
  Portal,
  Text,
  VStack,
} from '@chakra-ui/react'
import { ChevronDownIcon, ChevronLeftIcon, ChevronRightIcon, ChevronUpIcon } from '@chakra-ui/icons'
import { EditorIcon } from './Icons'
import type { PlacedElement } from '../types'
import {
  canvasPopoverContentStyles,
  canvasPopoverNameButtonStyles,
  canvasPopoverPillStyles,
  canvasPopoverKindLabelStyles,
  canvasPopoverPrimaryActionStyles,
} from '../styles/surfaces'
import { resolveElementIconUrl } from '../utils/elementIcon'
import { ElementBody } from './NodeBody'
import { ElementContainer } from './NodeContainer'

interface ContextNeighborData {
  element_id: number
  name: string
  kind: string | null
  description: string | null
  technology: string | null
  logo_url: string | null
  technology_connectors: PlacedElement['technology_connectors']
  ownerViewIds: number[]
  ownerViewNames: string[]
  currentViewId: number | null
  commonAncestorViewId: number | null
  commonAncestorViewName: string | null
  connectorCount: number
  onNavigateToView: (viewId: number) => void
  onSelectElement?: () => void
  onOpenRelationshipDetails?: () => void
  isCanvasMoving?: boolean
  isGroupAnchor?: boolean
  groupChildCount?: number
  isGroupExpanded?: boolean
  onToggleGroup?: () => void
  side?: 'top' | 'bottom' | 'left' | 'right'
}

interface Props {
  data: ContextNeighborData
}

function ContextNeighborNode({ data }: Props) {
  const [isHovered, setIsHovered] = useState(false)
  const hoverCloseTimeoutRef = useRef<number | null>(null)

  useEffect(() => {
    if (data.isCanvasMoving) setIsHovered(false)
  }, [data.isCanvasMoving])

  useEffect(() => () => {
    if (hoverCloseTimeoutRef.current !== null) {
      window.clearTimeout(hoverCloseTimeoutRef.current)
    }
  }, [])

  const clearHoverCloseTimeout = () => {
    if (hoverCloseTimeoutRef.current !== null) {
      window.clearTimeout(hoverCloseTimeoutRef.current)
      hoverCloseTimeoutRef.current = null
    }
  }

  const openPopover = () => {
    clearHoverCloseTimeout()
    setIsHovered(true)
  }

  const schedulePopoverClose = () => {
    clearHoverCloseTimeout()
    hoverCloseTimeoutRef.current = window.setTimeout(() => {
      setIsHovered(false)
      hoverCloseTimeoutRef.current = null
    }, 180)
  }


  const logoUrl = useMemo(() => {
    return resolveElementIconUrl(data.logo_url, data.technology_connectors) ?? undefined
  }, [data.logo_url, data.technology_connectors])

  const availableViews = useMemo(() => {
    const uniqueViews = new Map<number, string>()
    data.ownerViewIds.forEach((viewId, index) => {
      if (viewId === data.currentViewId) return
      uniqueViews.set(viewId, data.ownerViewNames[index] ?? `View ${viewId}`)
    })
    return Array.from(uniqueViews.entries()).map(([viewId, viewName]) => ({ viewId, viewName }))
  }, [data.currentViewId, data.ownerViewIds, data.ownerViewNames])

  const primaryOwnerViewId = data.ownerViewIds[0] ?? data.commonAncestorViewId ?? null
  const isGroupAnchor = data.isGroupAnchor ?? false
  const groupChildCount = data.groupChildCount ?? 0
  const isGroupExpanded = data.isGroupExpanded ?? false
  const side = data.side ?? 'right'
  const kindLabel = data.kind?.trim() || ''
  const hasPrimaryAction = Boolean(data.onSelectElement || primaryOwnerViewId)
  const hasDetails = Boolean(data.technology || data.description)
  const hasActions = Boolean(data.onOpenRelationshipDetails || availableViews.length > 0)
  const hasBodyContent = hasDetails || hasActions

  // Position chevron just past the visual node edge (node is scale(0.5) with center origin,
  // so the visual edge is at 75% of the layout box dimension on the outward side).
  const chevronStyle = (() => {
    switch (side) {
      case 'right': return { left: 'calc(75% + 6px)', top: '50%', transform: 'translateY(-50%)' }
      case 'left':  return { right: 'calc(75% + 6px)', top: '50%', transform: 'translateY(-50%)' }
      case 'bottom': return { top: 'calc(75% + 6px)', left: '50%', transform: 'translateX(-50%)' }
      case 'top':   return { bottom: 'calc(75% + 6px)', left: '50%', transform: 'translateX(-50%)' }
    }
  })()

  const ChevronExpandIcon = (() => {
    switch (side) {
      case 'right':  return isGroupExpanded ? ChevronLeftIcon  : ChevronRightIcon
      case 'left':   return isGroupExpanded ? ChevronRightIcon : ChevronLeftIcon
      case 'bottom': return isGroupExpanded ? ChevronUpIcon    : ChevronDownIcon
      case 'top':    return isGroupExpanded ? ChevronDownIcon  : ChevronUpIcon
    }
  })()

  return (
    <Box pointerEvents="none" position="relative">
      <Handle type="source" position={Position.Top} id="top" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />
      <Handle type="source" position={Position.Bottom} id="bottom" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />
      <Handle type="source" position={Position.Left} id="left" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />
      <Handle type="source" position={Position.Right} id="right" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />
      <Handle type="target" position={Position.Top} id="top" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />
      <Handle type="target" position={Position.Bottom} id="bottom" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />
      <Handle type="target" position={Position.Left} id="left" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />
      <Handle type="target" position={Position.Right} id="right" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />

      <Popover isOpen={isHovered && !data.isCanvasMoving} placement="right-start" closeOnBlur={false} gutter={12} isLazy>
        <PopoverTrigger>
          <Box transform="scale(0.5)" transformOrigin="center center">
            <Box position="relative">
              {/* Transparent ghost cards indicating a collapsed stack */}
              {isGroupAnchor && !isGroupExpanded && groupChildCount > 0 && (
                <>
                  <Box
                    position="absolute"
                    top="10px"
                    left="10px"
                    right="-10px"
                    bottom="-10px"
                    borderRadius="md"
                    border="1px solid"
                    borderColor="whiteAlpha.200"
                    zIndex={-2}
                  />
                  <Box
                    position="absolute"
                    top="5px"
                    left="5px"
                    right="-5px"
                    bottom="-5px"
                    borderRadius="md"
                    border="1px solid"
                    borderColor="whiteAlpha.300"
                    zIndex={-1}
                  />
                </>
              )}
              <ElementContainer
                minW="180px"
                maxW="230px"
                cursor={data.onSelectElement || primaryOwnerViewId ? 'pointer' : 'default'}
                pointerEvents="auto"
                onMouseEnter={openPopover}
                onMouseLeave={schedulePopoverClose}
                onClick={() => {
                  if (data.onSelectElement) {
                    data.onSelectElement()
                    return
                  }
                  if (primaryOwnerViewId) data.onNavigateToView(primaryOwnerViewId)
                }}
                opacity={0.82}
                _hover={{ opacity: 1, transform: 'scale(1.05)' }}
                transition="all 0.2s"
                bg="transparent"
              >
                <ElementBody
                  name={data.name}
                  type={data.kind ?? ''}
                  logoUrl={logoUrl}
                  nameSize="xl"
                  minH="85px"
                  pt={2}
                  pb={2}
                />
              </ElementContainer>
            </Box>
          </Box>
        </PopoverTrigger>

        <Portal>
          <PopoverContent
            sx={canvasPopoverContentStyles}
            width="264px"
            pointerEvents="auto"
            onMouseEnter={openPopover}
            onMouseLeave={schedulePopoverClose}
          >
            <PopoverHeader
              borderBottom={hasBodyContent ? '1px solid' : 'none'}
              borderColor="whiteAlpha.100"
              px={3.5}
              py={2.5}
            >
              <HStack spacing={2} align="center" justify="space-between">
                <HStack spacing={2} align="center" flex={1} minW={0}>
                  {logoUrl && (
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
                      <Box as="img" src={logoUrl} w="14px" h="14px" objectFit="contain" />
                    </Box>
                  )}
                  <Button
                    size="xs"
                    variant="outline"
                    isDisabled={!hasPrimaryAction}
                    sx={{
                      ...canvasPopoverNameButtonStyles,
                      fontSize: '12px',
                      fontWeight: '700',
                      minH: '24px',
                      px: 2.5,
                      '& .chakra-button__icon': { opacity: 1 },
                    }}
                    rightIcon={<EditorIcon size={10} />}
                    onClick={(event) => {
                      event.stopPropagation()
                      if (data.onSelectElement) {
                        data.onSelectElement()
                        return
                      }
                      if (primaryOwnerViewId) data.onNavigateToView(primaryOwnerViewId)
                    }}
                    title={data.name}
                  >
                    <Text as="span" isTruncated>
                      {data.name}
                    </Text>
                  </Button>
                </HStack>
                {kindLabel && (
                  <Text sx={canvasPopoverKindLabelStyles} isTruncated title={kindLabel}>
                    {kindLabel}
                  </Text>
                )}
              </HStack>
            </PopoverHeader>
            {hasBodyContent && (
              <PopoverBody px={3.5} py={2.5}>
                <VStack align="stretch" spacing={2}>
                  {data.technology && (
                    <Text fontSize="11px" fontWeight="500" color="gray.300" noOfLines={1}>
                      {data.technology}
                    </Text>
                  )}
                  {data.description && (
                    <Text fontSize="11px" color="gray.400" noOfLines={3} lineHeight="1.4">
                      {data.description}
                    </Text>
                  )}
                  {hasDetails && hasActions && <Divider borderColor="whiteAlpha.100" />}
                  {hasActions && (
                    <VStack align="stretch" spacing={1.5}>
                      {availableViews.length > 0 && (
                        <HStack spacing={1} flexWrap="wrap">
                          {availableViews.map(({ viewId, viewName }) => (
                            <Button
                              key={viewId}
                              size="xs"
                              variant="outline"
                              sx={canvasPopoverNameButtonStyles}
                              rightIcon={<EditorIcon size={10} />}
                              onClick={(event) => {
                                event.stopPropagation()
                                data.onNavigateToView(viewId)
                              }}
                              title={viewName}
                            >
                              <Text as="span" isTruncated>
                                {viewName}
                              </Text>
                            </Button>
                          ))}
                        </HStack>
                      )}
                      {data.onOpenRelationshipDetails && (
                        <Button
                          size="xs"
                          h="26px"
                          width="full"
                          justifyContent="space-between"
                          variant="outline"
                          sx={canvasPopoverPrimaryActionStyles}
                          onClick={() => data.onOpenRelationshipDetails?.()}
                        >
                          <Text as="span" fontSize="11px">Show connectors</Text>
                          <Badge
                            colorScheme="blue"
                            variant="subtle"
                            sx={{
                              ...canvasPopoverPillStyles,
                              bg: 'rgba(var(--accent-rgb), 0.14)',
                              color: 'var(--accent)',
                            }}
                          >
                            {data.connectorCount}
                          </Badge>
                        </Button>
                      )}
                    </VStack>
                  )}
                </VStack>
              </PopoverBody>
            )}
          </PopoverContent>
        </Portal>
      </Popover>

      {/* Expand/collapse chevron positioned on the outward-facing side of the stack */}
      {isGroupAnchor && groupChildCount > 0 && (
        <Box
          position="absolute"
          {...chevronStyle}
          pointerEvents="auto"
          zIndex={10}
        >
          <Button
            aria-label={`${isGroupExpanded ? 'Collapse' : 'Expand'} ${data.name}`}
            variant="unstyled"
            display="inline-flex"
            alignItems="center"
            gap={0.5}
            color="whiteAlpha.400"
            _hover={{ color: 'whiteAlpha.700' }}
            height="auto"
            minW="auto"
            p={0}
            lineHeight={1}
            transition="color 0.15s"
            onClick={(e) => {
              e.stopPropagation()
              data.onToggleGroup?.()
            }}
          >
            <ChevronExpandIcon boxSize={2} />
            {!isGroupExpanded && (
              <Text as="span" fontSize="8px" letterSpacing="0.02em">{groupChildCount}</Text>
            )}
          </Button>
        </Box>
      )}
    </Box>
  )
}

export default memo(ContextNeighborNode)
