import React from 'react'
import {
  Box,
  Button,
  HStack,
  IconButton,
  Popover,
  PopoverBody,
  PopoverContent,
  PopoverTrigger,
  Portal,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import TagUpsert from '../../../components/TagUpsert'
import { FitViewIcon, TagsIcon, TrashIcon } from '../../../components/Icons'
import type { Tag } from '../../../types'
import type { SelectionAlign, SelectionDistribute } from '../selection'

function AlignIcon({ kind }: { kind: SelectionAlign | SelectionDistribute }) {
  const isVertical = kind === 'top' || kind === 'middle' || kind === 'bottom' || kind === 'vertical'
  const line = (() => {
    switch (kind) {
      case 'left': return <path d="M5 4v16M9 7h10M9 17h7" />
      case 'center': return <path d="M12 4v16M6 7h12M8 17h8" />
      case 'right': return <path d="M19 4v16M5 7h10M8 17h7" />
      case 'top': return <path d="M4 5h16M7 9v10M17 9v7" />
      case 'middle': return <path d="M4 12h16M7 6v12M17 8v8" />
      case 'bottom': return <path d="M4 19h16M7 5v10M17 8v7" />
      case 'horizontal': return <path d="M6 7v10M18 7v10M9 12h6M12 9l3 3-3 3" />
      case 'vertical': return <path d="M7 6h10M7 18h10M12 9v6M9 12l3 3 3-3" />
    }
  })()

  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={isVertical ? 2.4 : 2.2} strokeLinecap="round" strokeLinejoin="round">
      {line}
    </svg>
  )
}

function ToolbarIconButton({
  label,
  onClick,
  children,
  color,
}: {
  label: string
  onClick: () => void
  children: React.ReactElement
  color?: string
}) {
  return (
    <Tooltip label={label} placement="top" openDelay={200}>
      <IconButton
        aria-label={label}
        icon={children}
        variant="ghost"
        h="28px"
        minW="28px"
        px={0}
        color={color ?? 'gray.300'}
        _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: color ?? 'var(--accent)' }}
        onClick={onClick}
      />
    </Tooltip>
  )
}

export interface SelectionBulkBarProps {
  count: number
  availableTags: string[]
  selectedTagCounts: Record<string, number>
  tagColors: Record<string, Tag>
  onAlign: (align: SelectionAlign) => void
  onDistribute: (direction: SelectionDistribute) => void
  onFitSelection: () => void
  onAddTag: (tag: string) => void
  onRemoveTag: (tag: string) => void
  onRemoveFromView: () => void
}

export default function SelectionBulkBar({
  count,
  availableTags,
  selectedTagCounts,
  tagColors,
  onAlign,
  onDistribute,
  onFitSelection,
  onAddTag,
  onRemoveTag,
  onRemoveFromView,
}: SelectionBulkBarProps) {
  if (count < 2) return null

  const removableTags = Object.keys(selectedTagCounts)
    .filter((tag) => selectedTagCounts[tag] > 0)
    .sort((a, b) => a.localeCompare(b))

  return (
    <HStack
      data-testid="vieweditor-selection-bulk-bar"
      position="absolute"
      left="50%"
      top="calc(5rem + env(safe-area-inset-top, 0px))"
      transform="translateX(-50%)"
      zIndex={30}
      pointerEvents="auto"
      spacing={0}
      bg="var(--bg-panel)"
      border="1px solid"
      borderColor="whiteAlpha.100"
      rounded="xl"
      boxShadow="0 10px 34px rgba(0,0,0,0.48)"
      backdropFilter="blur(20px)"
      px={1.5}
      py={1.5}
    >
      <Text fontSize="11px" color="whiteAlpha.800" fontWeight="700" px={2.5} whiteSpace="nowrap">
        {count} selected
      </Text>

      <Box w="1px" h="16px" bg="whiteAlpha.100" mx={0.5} />

      {(['left', 'center', 'right', 'top', 'middle', 'bottom'] as const).map((align) => (
        <ToolbarIconButton key={align} label={`Align ${align}`} onClick={() => onAlign(align)}>
          <AlignIcon kind={align} />
        </ToolbarIconButton>
      ))}

      <Box w="1px" h="16px" bg="whiteAlpha.100" mx={0.5} />

      <ToolbarIconButton label="Distribute horizontal" onClick={() => onDistribute('horizontal')}>
        <AlignIcon kind="horizontal" />
      </ToolbarIconButton>
      <ToolbarIconButton label="Distribute vertical" onClick={() => onDistribute('vertical')}>
        <AlignIcon kind="vertical" />
      </ToolbarIconButton>

      <Box w="1px" h="16px" bg="whiteAlpha.100" mx={0.5} />

      <Popover placement="top" isLazy closeOnBlur>
        <PopoverTrigger>
          <Button
            variant="ghost"
            h="28px"
            px={2.5}
            color="gray.300"
            _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
            aria-label="Bulk tags"
          >
            <HStack spacing={1.5}>
              <TagsIcon />
              <Text fontSize="11px">Tags</Text>
            </HStack>
          </Button>
        </PopoverTrigger>
        <Portal>
          <PopoverContent
            bg="gray.900"
            borderColor="whiteAlpha.200"
            boxShadow="0 18px 48px rgba(0,0,0,0.48)"
            borderRadius="lg"
            width="280px"
            _focus={{ boxShadow: 'none' }}
          >
            <PopoverBody p={3}>
              <VStack align="stretch" spacing={3}>
                <TagUpsert
                  currentTags={[]}
                  availableTags={availableTags}
                  onAddTag={onAddTag}
                />
                {removableTags.length > 0 && (
                  <VStack align="stretch" spacing={1.5}>
                    {removableTags.map((tag) => (
                      <HStack key={tag} spacing={2} minW={0}>
                        <Box w="7px" h="7px" rounded="full" bg={tagColors[tag]?.color ?? 'var(--accent)'} flexShrink={0} />
                        <Text fontSize="xs" color="gray.300" flex={1} noOfLines={1}>{tag}</Text>
                        <Button size="xs" h="22px" variant="ghost" color="red.300" onClick={() => onRemoveTag(tag)}>
                          Remove
                        </Button>
                      </HStack>
                    ))}
                  </VStack>
                )}
              </VStack>
            </PopoverBody>
          </PopoverContent>
        </Portal>
      </Popover>

      <ToolbarIconButton label="Fit selection" onClick={onFitSelection}>
        <FitViewIcon size={14} />
      </ToolbarIconButton>
      <ToolbarIconButton label="Remove from view" onClick={onRemoveFromView} color="red.300">
        <TrashIcon size={13} />
      </ToolbarIconButton>
    </HStack>
  )
}
