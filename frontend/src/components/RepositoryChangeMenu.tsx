import { TagFilterReset, TagFilterRow } from './TagFilterControls'
import { Box, Button, HStack, Popover, PopoverBody, PopoverContent, PopoverTrigger, Portal, Text, VStack } from '@chakra-ui/react'
import type { Tag } from '../types'
import { TagsIcon } from './Icons'

export type RepositoryChangeViewMode = 'standard' | 'plain'

export default function RepositoryChangeMenu({
  showViewMode = true,
  busy = false,
  viewMode,
  onViewModeChange,
  allTags = [],
  tagCounts = {},
  tagColors = {},
  hiddenTags = [],
  selectedTags = [],
  onToggleTagSelection,
  onResetTagFilters,
  onHoverTag,
  onToggleTagVisibility,
}: {
  showViewMode?: boolean
  busy?: boolean
  viewMode: RepositoryChangeViewMode
  onViewModeChange: (mode: RepositoryChangeViewMode) => void
  allTags?: string[]
  tagCounts?: Record<string, number>
  tagColors?: Record<string, Tag>
  hiddenTags?: string[]
  selectedTags?: string[]
  onToggleTagSelection?: (tag: string) => void
  onResetTagFilters?: () => void
  onHoverTag?: (tag: string | null) => void
  onToggleTagVisibility?: (tag: string) => void
}) {
  return (
    <VStack
      position="absolute"
      left="50%"
      bottom={4}
      transform="translateX(-50%)"
      zIndex={20}
      spacing={2.5}
      align="center"
      pointerEvents="none"
    >
      <HStack
        pointerEvents="auto"
        spacing={0}
        bg="var(--bg-panel)"
        border="1px solid"
        borderColor="whiteAlpha.100"
        rounded="xl"
        boxShadow="0 5px 10px rgba(0,0,0,0.5)"
        backdropFilter="blur(20px)"
        px={1.5}
        py={1.5}
      >
        {showViewMode && <HStack spacing={1} align="center" data-testid="repositories-view-mode" px={1}>
          {(['standard', 'plain'] as const).map((option) => (
            <Button
              key={option}
              data-testid={`repositories-view-mode-${option}`}
              size="xs"
              h="28px"
              px={2}
              variant={viewMode === option ? 'solid' : 'ghost'}
              isDisabled={busy}
              onClick={() => {
                if (option !== viewMode) onViewModeChange(option)
              }}
            >
              {option === 'standard' ? 'Standard' : 'Plain'}
            </Button>
          ))}
        </HStack>}
        {allTags.length > 0 && onHoverTag && onToggleTagVisibility && (
          <>
            {showViewMode && <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />}
            <Popover
              onClose={() => onHoverTag(null)}
              placement="top"
              isLazy
              closeOnBlur
            >
              <PopoverTrigger>
                <Button
                  data-testid="repository-change-tags-button"
                  variant="ghost" h="28px" px={2}
                  color="gray.300"
                  _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                >
                  <HStack spacing={1.5}>
                    <TagsIcon />
                    <Text fontSize="11px" fontWeight="normal">Tags</Text>
                  </HStack>
                </Button>
              </PopoverTrigger>
              <Portal>
                <PopoverContent
                  data-testid="repository-change-tags-panel"
                  bg="glass.bg"
                  backdropFilter="blur(16px)"
                  borderColor="glass.border"
                  boxShadow="panel"
                  borderRadius="lg"
                  width="220px"
                  _focus={{ boxShadow: 'none' }}
                  onMouseLeave={() => onHoverTag(null)}
                >
                  <PopoverBody p={2} maxH="360px" overflowY="auto">
                    {allTags.map((tag) => (
                      <TagFilterRow
                        key={`tag-${tag}`}
                        tag={tag}
                        count={tagCounts[tag] ?? 0}
                        color={tagColors[tag]?.color}
                        hidden={hiddenTags.includes(tag)}
                        selected={selectedTags.includes(tag)}
                        onSelect={onToggleTagSelection}
                        onHover={() => onHoverTag(tag)}
                        onToggleVisibility={onToggleTagVisibility}
                      />
                    ))}
                  </PopoverBody>
                </PopoverContent>
              </Portal>
            </Popover>
            <TagFilterReset active={selectedTags.length > 0} onReset={onResetTagFilters ? () => { onResetTagFilters(); onHoverTag(null) } : undefined} />
          </>
        )}
      </HStack>
    </VStack>
  )
}
