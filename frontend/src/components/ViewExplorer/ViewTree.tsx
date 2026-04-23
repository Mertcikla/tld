import React from 'react'
import { VStack, HStack, Box, Text } from '@chakra-ui/react'
import ScrollIndicatorWrapper from '../ScrollIndicatorWrapper'
import { TreeNode } from './types'

interface Props {
  filtered: TreeNode[]
  viewId: number | null
  focusedIdx: number
  itemRefs: React.MutableRefObject<(HTMLDivElement | null)[]>
  handleNavigate: (id: number) => void
  onHoverZoom?: (elementId: number | null, type: 'in' | 'out' | null) => void
  viewHoverMap: Map<number, { elementId: number | undefined; type: 'in' | 'out' }>
  handleKeyDown: (e: React.KeyboardEvent) => void
  listRef: React.RefObject<HTMLDivElement>
}

export const ViewTree: React.FC<Props> = ({
  filtered,
  viewId,
  focusedIdx,
  itemRefs,
  handleNavigate,
  onHoverZoom,
  viewHoverMap,
  handleKeyDown,
  listRef,
}) => {
  return (
    <ScrollIndicatorWrapper
      ref={listRef}
      flex={1}
      minH={0}
      px={3}
      pb={4}
      tabIndex={-1}
      outline="none"
      onKeyDown={handleKeyDown}
    >
      <VStack spacing={0.5} mr={-1} align="stretch">
        {filtered.map((node, idx) => {
          const isCurrent = node.id === viewId
          const isFocused = idx === focusedIdx
          return (
            <HStack
              key={node.id}
              ref={(el) => {
                itemRefs.current[idx] = el as HTMLDivElement | null
              }}
              px={3}
              py={2}
              pl={3 + (node.depth * 3)}
              rounded="lg"
              cursor="pointer"
              bg={
                isCurrent
                  ? 'rgba(var(--accent-rgb), 0.12)'
                  : isFocused
                    ? 'whiteAlpha.150'
                    : 'transparent'
              }
              color={isCurrent ? 'var(--accent)' : 'gray.400'}
              _hover={{
                bg: isCurrent ? 'rgba(var(--accent-rgb), 0.18)' : 'whiteAlpha.100',
                color: 'gray.100',
              }}
              onClick={() => handleNavigate(node.id)}
              onMouseEnter={() => {
                const item = viewHoverMap.get(node.id)
                if (item) onHoverZoom?.(item.elementId ?? null, item.type)
              }}
              onMouseLeave={() => onHoverZoom?.(null, null)}
              transition="all 0.15s"
              spacing={1}
            >
              {node.depth > 0 && (
                <HStack spacing="4px" mr={1}>
                  {Array.from({ length: node.depth }).map((_, i) => (
                    <Box
                      key={i}
                      w="3px"
                      h="3px"
                      rounded="full"
                      bg={isCurrent ? 'var(--accent)' : 'whiteAlpha.900'}
                    />
                  ))}
                </HStack>
              )}
              <Text fontSize="xs" fontWeight={isCurrent ? 'bold' : 'medium'} noOfLines={1} isTruncated>
                {node.name}
              </Text>
            </HStack>
          )
        })}
      </VStack>
    </ScrollIndicatorWrapper>
  )
}
