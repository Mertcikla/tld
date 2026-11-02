import { Box, HStack, Text, VStack } from '@chakra-ui/react'
import type { Tag } from '../types'
import { isElementGroupTag } from '../utils/elementGroups'
import { isGroundingIgnoreTag } from '../utils/groundingTags'

export default function ElementTags({ tags = [], tagColors, forceShowTagPopup, isCanvasMoving }: {
  tags: string[]; tagColors: Record<string, Tag>; forceShowTagPopup?: boolean; isCanvasMoving?: boolean
}) {
  const visibleTags = tags.filter(tag => !isElementGroupTag(tag) && !isGroundingIgnoreTag(tag))
  return visibleTags.length ? (
    <Box
      position="absolute"
      bottom={2}
      left={2.5}
      zIndex={10}
      role="group"
    >
      {/* Tag Dots (up to 5) */}
      <HStack spacing={1} _groupHover={{ opacity: 0 }}>
        {visibleTags.slice(0, 5).map((tag, i) => (
          <Box
            key={i}
            w="6px"
            h="6px"
            rounded="full"
            bg={tagColors[tag]?.color || 'var(--accent)'}
            boxShadow="0 0 3px rgba(0,0,0,0.3)"
            transition="all 0.2s"
          />
        ))}
        {visibleTags.length > 5 && (
          <Text fontSize="8px" fontWeight="bold" color="whiteAlpha.600" lineHeight={1}>
            +{visibleTags.length - 5}
          </Text>
        )}
      </HStack>

      {/* Hover Menu: Expanding tags at -45 degree angle */}
      <VStack
        position="absolute"
        bottom="-4px"
        left="-4px"
        align="start"
        spacing={1}
        opacity={forceShowTagPopup && !isCanvasMoving ? 1 : 0}
        visibility={forceShowTagPopup && !isCanvasMoving ? 'visible' : 'hidden'}
        transform={forceShowTagPopup && !isCanvasMoving ? 'scale(1) translate(0px, 0px)' : 'scale(0.2) translate(10px, 10px)'}
        transformOrigin="bottom left"
        transition="all 0.25s cubic-bezier(0.175, 0.885, 0.32, 1.275)"
        _groupHover={{ opacity: 1, visibility: 'visible', transform: 'scale(1) translate(0px, 0px)' }}
        pointerEvents="none"
      >
        {visibleTags.map((tag) => (
          <Box
            key={tag}
            bg="var(--bg-panel)"
            border="1px solid"
            borderColor="whiteAlpha.300"
            rounded="full"
            px={2}
            py="1px"
            boxShadow="0 4px 12px rgba(0,0,0,0.5)"
            whiteSpace="nowrap"
          >
            <HStack spacing={1.5}>
              <Box w="6px" h="6px" rounded="full" bg={tagColors[tag]?.color || 'var(--accent)'} />
              <Text fontSize="10px" fontWeight="700" color="white">{tag}</Text>
            </HStack>
          </Box>
        ))}
      </VStack>
    </Box>
  ) : null
}
