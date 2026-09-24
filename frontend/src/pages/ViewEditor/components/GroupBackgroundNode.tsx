import React from 'react'
import { Box, HStack, Text } from '@chakra-ui/react'
import type { NodeProps } from 'reactflow'

export interface GroupBackgroundNodeData {
  label: string
  color: string
  memberCount: number
  memberNodeIds: string[]
}

const GroupBackgroundNode: React.FC<NodeProps<GroupBackgroundNodeData>> = React.memo(({ data }) => (
  <Box
    data-testid="vieweditor-group-background"
    position="relative"
    w="100%"
    h="100%"
    border="1px solid"
    borderColor={`color-mix(in srgb, ${data.color} 52%, transparent)`}
    borderRadius="14px"
    bg={`color-mix(in srgb, ${data.color} 10%, transparent)`}
    pointerEvents="none"
    overflow="visible"
  >
    <HStack
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
    >
      <Box w="7px" h="7px" rounded="full" bg={data.color} flexShrink={0} />
      <Text fontSize="11px" fontWeight="700" noOfLines={1}>
        {data.label}
      </Text>
      <Text fontSize="10px" color="whiteAlpha.700" flexShrink={0}>
        {data.memberCount}
      </Text>
    </HStack>
  </Box>
))

GroupBackgroundNode.displayName = 'GroupBackgroundNode'

export default GroupBackgroundNode
