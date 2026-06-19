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
import { CopyIcon } from '@chakra-ui/icons'
import { FitViewIcon, MergeIcon, TagsIcon, TrashIcon } from '../../../components/Icons'
import type { Tag } from '../../../types'
import type { SelectionAlign, SelectionDistribute } from '../selection'

function AlignIcon({ kind }: { kind: SelectionAlign | SelectionDistribute }) {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" style={{ display: 'block' }}>
      {(() => {
        switch (kind) {
          case 'left': return (
            <>
              <line x1="4" y1="3" x2="4" y2="21" strokeWidth="2.5" />
              <rect x="7" y="5.5" width="13" height="4.5" rx="1.5" strokeWidth="1.8" />
              <rect x="7" y="14" width="9" height="4.5" rx="1.5" strokeWidth="1.8" />
            </>
          )
          case 'center': return (
            <>
              <line x1="12" y1="3" x2="12" y2="21" strokeWidth="2.5" />
              <rect x="4.5" y="5.5" width="15" height="4.5" rx="1.5" strokeWidth="1.8" />
              <rect x="6.5" y="14" width="11" height="4.5" rx="1.5" strokeWidth="1.8" />
            </>
          )
          case 'right': return (
            <>
              <line x1="20" y1="3" x2="20" y2="21" strokeWidth="2.5" />
              <rect x="4" y="5.5" width="13" height="4.5" rx="1.5" strokeWidth="1.8" />
              <rect x="8" y="14" width="9" height="4.5" rx="1.5" strokeWidth="1.8" />
            </>
          )
          case 'top': return (
            <>
              <line x1="3" y1="4" x2="21" y2="4" strokeWidth="2.5" />
              <rect x="5.5" y="7" width="4.5" height="13" rx="1.5" strokeWidth="1.8" />
              <rect x="14" y="7" width="4.5" height="9" rx="1.5" strokeWidth="1.8" />
            </>
          )
          case 'middle': return (
            <>
              <line x1="3" y1="12" x2="21" y2="12" strokeWidth="2.5" />
              <rect x="5.5" y="4.5" width="4.5" height="15" rx="1.5" strokeWidth="1.8" />
              <rect x="14" y="6.5" width="4.5" height="11" rx="1.5" strokeWidth="1.8" />
            </>
          )
          case 'bottom': return (
            <>
              <line x1="3" y1="20" x2="21" y2="20" strokeWidth="2.5" />
              <rect x="5.5" y="4" width="4.5" height="13" rx="1.5" strokeWidth="1.8" />
              <rect x="14" y="8" width="4.5" height="9" rx="1.5" strokeWidth="1.8" />
            </>
          )
          case 'horizontal': return (
            <>
              <rect x="3" y="6" width="5" height="12" rx="1.5" strokeWidth="1.8" />
              <rect x="16" y="6" width="5" height="12" rx="1.5" strokeWidth="1.8" />
              <line x1="10" y1="12" x2="14" y2="12" strokeWidth="1.8" />
              <path d="M10 9.5l-2 2.5 2 2.5" strokeWidth="1.8" fill="none" />
              <path d="M14 9.5l2 2.5-2 2.5" strokeWidth="1.8" fill="none" />
            </>
          )
          case 'vertical': return (
            <>
              <rect x="6" y="3" width="12" height="5" rx="1.5" strokeWidth="1.8" />
              <rect x="6" y="16" width="12" height="5" rx="1.5" strokeWidth="1.8" />
              <line x1="12" y1="10" x2="12" y2="14" strokeWidth="1.8" />
              <path d="M9.5 10l2.5-2 2.5 2" strokeWidth="1.8" fill="none" />
              <path d="M9.5 14l2.5 2 2.5-2" strokeWidth="1.8" fill="none" />
            </>
          )
        }
      })()}
    </svg>
  )
}

function ToolbarIconButton({
  label,
  onClick,
  children,
  color,
  testId,
}: {
  label: string
  onClick: () => void
  children: React.ReactElement
  color?: string
  testId?: string
}) {
  return (
    <Tooltip label={label} placement="top" openDelay={200}>
      <IconButton
        data-testid={testId}
        aria-label={label}
        icon={children}
        variant="ghost"
        h="28px"
        minW="28px"
        px={0}
        display="inline-flex"
        alignItems="center"
        justifyContent="center"
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
  mergeOptions?: { id: number; name: string; kind?: string | null }[]
  mergeLoadingId?: number | null
  onAlign: (align: SelectionAlign) => void
  onDistribute: (direction: SelectionDistribute) => void
  onFitSelection: () => void
  onAddTag: (tag: string) => void
  onRemoveTag: (tag: string) => void
  onMergeInto?: (survivorId: number) => void
  onRemoveFromView: () => void
  onCopyMermaid?: () => void
}

export default function SelectionBulkBar({
  count,
  availableTags,
  selectedTagCounts,
  tagColors,
  mergeOptions = [],
  mergeLoadingId = null,
  onAlign,
  onDistribute,
  onFitSelection,
  onAddTag,
  onRemoveTag,
  onMergeInto,
  onRemoveFromView,
  onCopyMermaid,
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
        <ToolbarIconButton key={align} testId={`selection-bulk-align-${align}`} label={`Align ${align}`} onClick={() => onAlign(align)}>
          <AlignIcon kind={align} />
        </ToolbarIconButton>
      ))}

      <Box w="1px" h="16px" bg="whiteAlpha.100" mx={0.5} />

      <ToolbarIconButton testId="selection-bulk-distribute-horizontal" label="Distribute horizontal" onClick={() => onDistribute('horizontal')}>
        <AlignIcon kind="horizontal" />
      </ToolbarIconButton>
      <ToolbarIconButton testId="selection-bulk-distribute-vertical" label="Distribute vertical" onClick={() => onDistribute('vertical')}>
        <AlignIcon kind="vertical" />
      </ToolbarIconButton>

      <Box w="1px" h="16px" bg="whiteAlpha.100" mx={0.5} />

      <Popover placement="top" isLazy closeOnBlur>
        <PopoverTrigger>
          <Button
            data-testid="selection-bulk-tags"
            variant="ghost"
            h="28px"
            px={2.5}
            color="gray.300"
            _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
            aria-label="Bulk tags"
          >
            <HStack spacing={1.5}>
              <TagsIcon size={15} />
              <Text fontSize="11px">Tags</Text>
            </HStack>
          </Button>
        </PopoverTrigger>
        <Portal>
          <PopoverContent
            bg="var(--bg-panel)"
            backdropFilter="blur(20px)"
            borderColor="whiteAlpha.100"
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
                          <Text as="span" data-testid={`selection-bulk-remove-tag-${tag}`}>Remove</Text>
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

      {onMergeInto && mergeOptions.length >= 2 && (
        <Popover placement="top" isLazy closeOnBlur>
          <PopoverTrigger>
            <Button
              data-testid="selection-bulk-merge"
              variant="ghost"
              h="28px"
              px={2.5}
              color="gray.300"
              _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
              aria-label="Merge selection"
              isDisabled={mergeLoadingId !== null}
            >
              <HStack spacing={1.5}>
                <MergeIcon size={15} />
                <Text fontSize="11px">Merge</Text>
              </HStack>
            </Button>
          </PopoverTrigger>
          <Portal>
            <PopoverContent
              bg="var(--bg-panel)"
              backdropFilter="blur(20px)"
              borderColor="whiteAlpha.100"
              boxShadow="0 18px 48px rgba(0,0,0,0.48)"
              borderRadius="lg"
              width="260px"
              _focus={{ boxShadow: 'none' }}
            >
              <PopoverBody p={2.5}>
                <VStack align="stretch" spacing={1}>
                  <Text fontSize="xs" color="whiteAlpha.500" fontWeight="semibold" px={1} pb={1}>
                    Choose survivor
                  </Text>
                  {mergeOptions.map((option) => (
                    <Button
                      key={option.id}
                      data-testid={`selection-bulk-merge-survivor-${option.id}`}
                      variant="ghost"
                      size="sm"
                      h="auto"
                      minH="32px"
                      py={2}
                      px={2.5}
                      justifyContent="flex-start"
                      color="gray.300"
                      _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                      isLoading={mergeLoadingId === option.id}
                      isDisabled={mergeLoadingId !== null}
                      onClick={() => onMergeInto(option.id)}
                    >
                      <VStack spacing={0.5} align="start" minW={0}>
                        <Text fontSize="xs" fontWeight="medium" noOfLines={1}>{option.name}</Text>
                        {option.kind && <Text fontSize="10px" color="whiteAlpha.500" noOfLines={1}>{option.kind}</Text>}
                      </VStack>
                    </Button>
                  ))}
                </VStack>
              </PopoverBody>
            </PopoverContent>
          </Portal>
        </Popover>
      )}

      {onCopyMermaid && (
        <ToolbarIconButton testId="selection-bulk-copy-mermaid" label="Copy as Mermaid" onClick={onCopyMermaid}>
          <CopyIcon boxSize="14px" />
        </ToolbarIconButton>
      )}
      <ToolbarIconButton testId="selection-bulk-fit" label="Fit selection" onClick={onFitSelection}>
        <FitViewIcon size={16} />
      </ToolbarIconButton>
      <ToolbarIconButton testId="selection-bulk-remove" label="Remove from view" onClick={onRemoveFromView} color="red.300">
        <TrashIcon size={15} />
      </ToolbarIconButton>
    </HStack>
  )
}
