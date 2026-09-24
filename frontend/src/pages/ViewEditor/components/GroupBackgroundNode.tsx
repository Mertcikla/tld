import React, { useRef } from 'react'
import { Box, HStack, IconButton, Text } from '@chakra-ui/react'
import { useStoreApi, type NodeProps } from 'reactflow'
import { EyeIcon, EyeOffIcon } from '../../../components/Icons'

export interface GroupBackgroundNodeData {
  label: string
  color: string
  memberNodeIds: string[]
  hidden?: boolean
  onToggleVisibility?: () => void
  onGroupDragStart?: () => void
  onGroupDragMove?: (dx: number, dy: number) => void
  onGroupDragEnd?: () => void
}

const GroupBackgroundNode: React.FC<NodeProps<GroupBackgroundNodeData>> = React.memo(({ data }) => {
  const store = useStoreApi()
  const dragRef = useRef<{ pointerId: number; startX: number; startY: number } | null>(null)
  const canDrag = !!data.onGroupDragStart && !!data.onGroupDragMove && !!data.onGroupDragEnd

  const handlePointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!canDrag || event.button !== 0) return
    event.preventDefault()
    event.stopPropagation()
    event.currentTarget.setPointerCapture(event.pointerId)
    dragRef.current = { pointerId: event.pointerId, startX: event.clientX, startY: event.clientY }
    data.onGroupDragStart?.()
  }

  const handlePointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    event.preventDefault()
    event.stopPropagation()
    const zoom = store.getState().transform[2] || 1
    data.onGroupDragMove?.((event.clientX - drag.startX) / zoom, (event.clientY - drag.startY) / zoom)
  }

  const handlePointerUp = (event: React.PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    dragRef.current = null
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    data.onGroupDragEnd?.()
  }

  return (
    <Box
      data-testid="vieweditor-group-background"
      position="relative"
      w="100%"
      h="100%"
      border="1px solid"
      borderColor={`color-mix(in srgb, ${data.color} ${data.hidden ? 20 : 52}%, transparent)`}
      borderRadius="14px"
      bg={`color-mix(in srgb, ${data.color} ${data.hidden ? 3 : 10}%, transparent)`}
      pointerEvents="none"
      overflow="visible"
    >
      <HStack
        data-testid="vieweditor-group-badge"
        position="absolute"
        top="8px"
        left="12px"
        maxW="calc(100% - 24px)"
        spacing={1.5}
        px={2}
        py={1}
        borderRadius="md"
        bg={`color-mix(in srgb, ${data.color} 18%, var(--bg-panel))`}
        border="1px solid"
        borderColor={`color-mix(in srgb, ${data.color} 36%, transparent)`}
        color="white"
        boxShadow="0 2px 8px rgba(0,0,0,0.2)"
        pointerEvents="auto"
        cursor={canDrag ? 'grab' : 'default'}
        _active={canDrag ? { cursor: 'grabbing' } : undefined}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerUp}
      >
        <IconButton
          data-testid="vieweditor-group-visibility-toggle"
          aria-label={data.hidden ? 'Show group' : 'Hide group'}
          icon={data.hidden ? <EyeOffIcon size={12} /> : <EyeIcon size={12} />}
          size="xs"
          variant="ghost"
          minW="16px"
          h="16px"
          p={0}
          color={data.hidden ? 'whiteAlpha.500' : 'white'}
          _hover={{ color: 'white', bg: 'whiteAlpha.200' }}
          onPointerDown={(event) => event.stopPropagation()}
          onClick={(event) => {
            event.stopPropagation()
            data.onToggleVisibility?.()
          }}
        />
        <Text fontSize="11px" fontWeight="700" noOfLines={1}>
          {data.label}
        </Text>
      </HStack>
    </Box>
  )
})

GroupBackgroundNode.displayName = 'GroupBackgroundNode'

export default GroupBackgroundNode
