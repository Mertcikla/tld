import { memo, useEffect, useMemo, useState } from 'react'
import { Handle, Position } from 'reactflow'
import {
  Badge,
  Box,
  Button,
  Divider,
  HStack,
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
import { ChevronDownIcon, ChevronLeftIcon, ChevronRightIcon, ChevronUpIcon, LinkIcon } from '@chakra-ui/icons'
import type { PlacedElement } from '../types'
import { TYPE_COLORS } from '../types'
import { resolveIconPath } from '../utils/url'
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
  commonAncestorViewId: number | null
  commonAncestorViewName: string | null
  connectorCount: number
  onNavigateToView: (viewId: number) => void
  onSelectDetails?: () => void
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

  useEffect(() => {
    if (data.isCanvasMoving) setIsHovered(false)
  }, [data.isCanvasMoving])

  const color = TYPE_COLORS[data.kind ?? ''] ?? 'gray'

  const logoUrl = useMemo(() => {
    if (data.logo_url) return resolveIconPath(data.logo_url)
    const selected = data.technology_connectors?.find((link) => link.type === 'catalog' && !!(link.is_primary_icon ?? (link as any).isPrimaryIcon) && !!link.slug)
    if (!selected?.slug) return undefined
    return resolveIconPath(`/icons/${selected.slug}.png`)
  }, [data.logo_url, data.technology_connectors])

  const primaryOwnerViewId = data.ownerViewIds[0] ?? data.commonAncestorViewId ?? null
  const isGroupAnchor = data.isGroupAnchor ?? false
  const groupChildCount = data.groupChildCount ?? 0
  const isGroupExpanded = data.isGroupExpanded ?? false
  const side = data.side ?? 'right'

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
                cursor={data.onSelectDetails || primaryOwnerViewId ? 'pointer' : 'default'}
                pointerEvents="auto"
                onMouseEnter={() => setIsHovered(true)}
                onMouseLeave={() => setIsHovered(false)}
                onClick={() => {
                  if (data.onSelectDetails) {
                    data.onSelectDetails()
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
          >
            <PopoverArrow bg="gray.900" />
            <PopoverHeader borderBottom="1px solid" borderColor="whiteAlpha.200" px={4} py={3}>
              <HStack spacing={3}>
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
                <Badge colorScheme="blue" variant="subtle">
                  {data.connectorCount}
                </Badge>
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
                <Box>
                  <Text color="gray.400" fontSize="xs" fontWeight="600" mb={0.5} letterSpacing="wider">BRANCH CONTEXT</Text>
                  <Text fontSize="xs" color="gray.200">
                    {data.commonAncestorViewName
                      ? `Branch diverges around ${data.commonAncestorViewName}.`
                      : 'Branch lives outside the current visible scope.'}
                  </Text>
                </Box>
                <Divider borderColor="whiteAlpha.200" />
                <VStack align="stretch" spacing={2} width="full">
                  {data.ownerViewIds.slice(0, 4).map((viewId, index) => (
                    <Button
                      key={`${viewId}-${index}`}
                      size="xs"
                      justifyContent="space-between"
                      variant="ghost"
                      color="gray.200"
                      _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                      rightIcon={<LinkIcon />}
                      onClick={() => data.onNavigateToView(viewId)}
                    >
                      <Text fontSize="xs" isTruncated>{data.ownerViewNames[index] ?? `View ${viewId}`}</Text>
                    </Button>
                  ))}
                </VStack>
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
