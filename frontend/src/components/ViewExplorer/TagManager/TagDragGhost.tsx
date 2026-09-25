import React, { useEffect, useRef } from 'react'
import { Box, Flex, HStack, Portal, Text } from '@chakra-ui/react'
import { TagsIcon } from '../../Icons'
import {
  endTagDrag,
  getTagDragPosition,
  useTagDragGhost,
  useTagDragHoverTarget,
} from './tagDragGhostState'

function IconBadge({ color }: { color: string }) {
  return (
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
  )
}

export const TagDragGhost: React.FC = () => {
  const meta = useTagDragGhost()
  const hoverTarget = useTagDragHoverTarget()
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
  const mergeColor = hoverTarget?.color || color

  return (
    <Portal>
      <Box
        ref={ref}
        data-testid="tag-drag-ghost"
        data-kind={meta.kind}
        data-merge={hoverTarget ? 'true' : undefined}
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
          pr={hoverTarget ? 2.5 : 3}
          py={1.5}
          rounded="full"
          border={hoverTarget ? '1px dashed' : '1px solid'}
          borderColor={`color-mix(in srgb, ${mergeColor} ${hoverTarget ? 70 : 55}%, transparent)`}
          bg="color-mix(in srgb, #0b1220 92%, transparent)"
          shadow={
            hoverTarget
              ? `0 0 0 3px color-mix(in srgb, ${hoverTarget.color} 20%, transparent), 0 16px 40px rgba(0, 0, 0, 0.55)`
              : '0 16px 40px rgba(0, 0, 0, 0.55)'
          }
          backdropFilter="blur(8px)"
        >
          <IconBadge color={color} />

          <Text fontSize="12px" fontWeight="700" color={color} isTruncated maxW="200px">
            {meta.name}
          </Text>

          {hoverTarget ? (
            <>
              <Text fontSize="14px" fontWeight="800" color="whiteAlpha.500" flexShrink={0} lineHeight={1}>
                +
              </Text>
              <IconBadge color={hoverTarget.color} />
              <Text fontSize="12px" fontWeight="700" color={hoverTarget.color} isTruncated maxW="160px">
                {hoverTarget.name}
              </Text>
              <Text
                fontSize="10px"
                fontWeight="600"
                color="whiteAlpha.600"
                whiteSpace="nowrap"
                flexShrink={0}
                pl={2}
                borderLeft="1px solid"
                borderColor="whiteAlpha.200"
              >
                Create tag group
              </Text>
            </>
          ) : (
            meta.detail && (
              <Text fontSize="10px" fontWeight="600" color="whiteAlpha.600" whiteSpace="nowrap" flexShrink={0}>
                {meta.detail}
              </Text>
            )
          )}
        </HStack>
      </Box>
    </Portal>
  )
}
