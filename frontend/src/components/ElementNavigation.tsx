import React, { useEffect, useRef, useState } from 'react'
import { Box, Flex, Tooltip, VStack, Text, Divider, Button, HStack } from '@chakra-ui/react'
import { useStore } from 'reactflow'
import type { ViewConnector } from '../types'
import type { NodeData } from './ElementNode'
import { ZoomInIcon, ZoomOutIcon } from './Icons'

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

export default function ElementNavigation({ data, isPending = false, children }: {
  data: NodeData; isPending?: boolean; children?: React.ReactNode
}) {
  const zoom = useStore(state => Math.round(state.transform[2] * 50) / 50)
  const [menuVisible, setMenuVisible] = useState(false)
  const menuRef = useRef<{ type: 'in' | 'out'; links: ViewConnector[] } | null>(null)
  useEffect(() => { if (data.isCanvasMoving) setMenuVisible(false) }, [data.isCanvasMoving])
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

  return <>
      {/* ── Header: zoom-out | zoom-in (absolute overlay, consumes no space) ── */}
      <Flex
        position="absolute"
        top={1}
        left={1}
        right={1}
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

        {children}

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
  </>
}
