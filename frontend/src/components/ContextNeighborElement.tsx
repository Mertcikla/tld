import { memo, useEffect, useMemo, useRef, useState } from 'react'
import { Handle, Position } from 'reactflow'
import {
  Badge,
  Box,
  Button,
  Divider,
  HStack,
  Menu,
  MenuButton,
  MenuItem,
  MenuList,
  Popover,
  PopoverArrow,
  PopoverBody,
  PopoverContent,
  PopoverHeader,
  PopoverTrigger,
  Portal,
  Tag,
  Text,
  VStack,
} from '@chakra-ui/react'
import { ChevronDownIcon, ChevronLeftIcon, ChevronRightIcon, ChevronUpIcon, EditIcon } from '@chakra-ui/icons'
import type { PlacedElement } from '../types'
import { TYPE_COLORS } from '../types'
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

  const color = TYPE_COLORS[data.kind ?? ''] ?? 'gray'

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
  const hasDetails = Boolean(data.technology || data.description)

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
            bg="gray.900"
            borderColor="whiteAlpha.300"
            boxShadow="2xl"
            width="300px"
            _focus={{ boxShadow: 'none' }}
            pointerEvents="auto"
            onMouseEnter={openPopover}
            onMouseLeave={schedulePopoverClose}
          >
            <PopoverArrow bg="gray.900" />
            <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.200" px={4} py={3}>
              <HStack spacing={3} align="start">
                {logoUrl && (
                  <Box flexShrink={0}>
                    <Box as="img" src={logoUrl} w="24px" h="24px" objectFit="contain" />
                  </Box>
                )}
                <VStack align="start" spacing={0} flex={1} overflow="hidden">
                  <Text fontWeight="600" fontSize="sm" isTruncated width="100%" color="white">
                    {data.name}
                  </Text>
                  <Tag colorScheme={color} variant="subtle" fontSize="2xs">
                    {data.kind || 'branch'}
                  </Tag>
                </VStack>
                <VStack align="stretch" spacing={1.5} flexShrink={0} minW="92px">
                  {data.onSelectElement && (
                    <Button
                      size="xs"
                      leftIcon={<EditIcon />}
                      variant="ghost"
                      color="gray.200"
                      bg="whiteAlpha.50"
                      border="1px solid"
                      borderColor="whiteAlpha.100"
                      _hover={{ bg: 'whiteAlpha.100', borderColor: 'whiteAlpha.200', color: 'white' }}
                      onClick={(event) => {
                        event.stopPropagation()
                        data.onSelectElement?.()
                      }}
                    >
                      Edit
                    </Button>
                  )}
                  <Menu placement="bottom-end" isLazy>
                    <MenuButton
                      as={Button}
                      size="xs"
                      rightIcon={<ChevronDownIcon />}
                      variant="ghost"
                      color={availableViews.length > 0 ? 'gray.200' : 'gray.500'}
                      bg="whiteAlpha.50"
                      border="1px solid"
                      borderColor="whiteAlpha.100"
                      _hover={{ bg: 'whiteAlpha.100', borderColor: 'whiteAlpha.200', color: 'white' }}
                      _active={{ bg: 'whiteAlpha.150' }}
                      isDisabled={availableViews.length === 0}
                    >
                      Views
                    </MenuButton>
                    <MenuList
                      bg="rgba(26, 32, 44, 0.98)"
                      border="1px solid"
                      borderColor="whiteAlpha.200"
                      borderRadius="lg"
                      boxShadow="0 12px 32px rgba(0,0,0,0.45)"
                      minW="180px"
                      py={1}
                    >
                      {availableViews.map(({ viewId, viewName }) => (
                        <MenuItem
                          key={viewId}
                          fontSize="13px"
                          color="gray.200"
                          bg="transparent"
                          _hover={{ bg: 'whiteAlpha.100' }}
                          _focus={{ bg: 'whiteAlpha.100' }}
                          onClick={() => data.onNavigateToView(viewId)}
                        >
                          {viewName}
                        </MenuItem>
                      ))}
                    </MenuList>
                  </Menu>
                </VStack>
              </HStack>
            </PopoverHeader>
            <PopoverBody px={4} py={3}>
              <VStack align="start" spacing={3}>
                {data.technology && (
                  <Box>
                    <Text color="gray.400" fontSize="xs" fontWeight="600" mb={0.5} letterSpacing="wider">TECHNOLOGY</Text>
                    <Text fontSize="xs" color="gray.200">{data.technology}</Text>
                  </Box>
                )}
                {data.description && (
                  <Box>
                    <Text color="gray.400" fontSize="xs" fontWeight="600" mb={0.5} letterSpacing="wider">DESCRIPTION</Text>
                    <Text fontSize="xs" color="gray.200" noOfLines={4}>{data.description}</Text>
                  </Box>
                )}
                {hasDetails && data.onOpenRelationshipDetails && <Divider borderColor="whiteAlpha.200" />}
                {data.onOpenRelationshipDetails && (
                  <Button
                    size="sm"
                    width="full"
                    justifyContent="space-between"
                    variant="ghost"
                    color="gray.200"
                    _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                    onClick={() => data.onOpenRelationshipDetails?.()}
                  >
                    <Text fontSize="xs">Show connectors</Text>
                    <Badge colorScheme="blue" variant="subtle">
                      {data.connectorCount}
                    </Badge>
                  </Button>
                )}
              </VStack>
            </PopoverBody>
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
