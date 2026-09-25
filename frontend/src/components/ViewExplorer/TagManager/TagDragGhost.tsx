import React, { useEffect, useRef } from 'react'
import { Box, Flex, HStack, Portal, Text } from '@chakra-ui/react'
import { TagsIcon } from '../../Icons'
import {
  endTagDrag,
  getTagDragPosition,
  useTagDragGhost,
} from './tagDragGhostState'

export const TagDragGhost: React.FC = () => {
  const meta = useTagDragGhost()
  const ref = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!meta) return
    const apply = (x: number, y: number) => {
      const el = ref.current
      if (el) el.style.transform = `translate3d(${x}px, ${y}px, 0) translate(14px, 14px)`
    }
    const initial = getTagDragPosition()
    apply(initial.x, initial.y)

    const onDragOver = (e: DragEvent) => {
      if (e.clientX === 0 && e.clientY === 0) return
      apply(e.clientX, e.clientY)
    }
    const onStop = () => endTagDrag()
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') endTagDrag()
    }

    window.addEventListener('dragover', onDragOver, true)
    window.addEventListener('dragend', onStop, true)
    window.addEventListener('drop', onStop, true)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('dragover', onDragOver, true)
      window.removeEventListener('dragend', onStop, true)
      window.removeEventListener('drop', onStop, true)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [meta])

  if (!meta) return null

  const color = meta.color || '#A0AEC0'

  return (
    <Portal>
      <Box
        ref={ref}
        data-testid="tag-drag-ghost"
        data-kind={meta.kind}
        position="fixed"
        top={0}
        left={0}
        zIndex={9999}
        pointerEvents="none"
        willChange="transform"
        sx={{ transform: 'translate3d(0, 0, 0) translate(14px, 14px)' }}
      >
        <HStack
          spacing={2}
          align="center"
          pl={1.5}
          pr={3}
          py={1.5}
          rounded="full"
          border="1px solid"
          borderColor={`color-mix(in srgb, ${color} 55%, transparent)`}
          bg="color-mix(in srgb, #0b1220 92%, transparent)"
          shadow="0 16px 40px rgba(0, 0, 0, 0.55)"
          backdropFilter="blur(8px)"
        >
          <Flex
            align="center"
            justify="center"
            boxSize="22px"
            rounded="full"
            flexShrink={0}
            color={color}
            bg={`color-mix(in srgb, ${color} 20%, transparent)`}
            border="1px solid"
            borderColor={`color-mix(in srgb, ${color} 40%, transparent)`}
          >
            <TagsIcon size={12} strokeWidth={2.4} />
          </Flex>

          <Text fontSize="12px" fontWeight="700" color={color} isTruncated maxW="200px">
            {meta.name}
          </Text>

          {meta.detail && (
            <Text fontSize="10px" fontWeight="600" color="whiteAlpha.600" whiteSpace="nowrap" flexShrink={0}>
              {meta.detail}
            </Text>
          )}
        </HStack>
      </Box>
    </Portal>
  )
}
