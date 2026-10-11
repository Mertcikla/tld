import { Box, Button, HStack, IconButton, Popover, PopoverBody, PopoverContent, PopoverTrigger, Portal, Text, VStack } from '@chakra-ui/react'
import type { Tag } from '../types'
import { EyeIcon, EyeOffIcon, TagsIcon } from './Icons'
import { EDGE_CHANGE_META, type ZUIEdgeChange } from './ZUI/edgeChange'

export type RepositoryChangeViewMode = 'standard' | 'plain'

const EDGE_CHANGE_ORDER: ZUIEdgeChange[] = ['added', 'removed', 'modified']

export default function RepositoryChangeMenu({
  busy = false,
  viewMode,
  onViewModeChange,
  allTags = [],
  tagCounts = {},
  tagColors = {},
  hiddenTags = [],
  onHoverTag,
  onToggleTagVisibility,
}: {
  busy?: boolean
  viewMode: RepositoryChangeViewMode
  onViewModeChange: (mode: RepositoryChangeViewMode) => void
  allTags?: string[]
  tagCounts?: Record<string, number>
  tagColors?: Record<string, Tag>
  hiddenTags?: string[]
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
        <HStack spacing={1} align="center" data-testid="repositories-view-mode" px={1}>
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
        </HStack>
        {allTags.length > 0 && onHoverTag && onToggleTagVisibility && (
          <>
            <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />
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
                    {allTags.map((tag) => {
                      const isHidden = hiddenTags.includes(tag)
                      return (
                        <HStack
                          key={`tag-${tag}`}
                          px={2}
                          py={1}
                          spacing={2}
                          borderRadius="md"
                          onMouseEnter={() => onHoverTag(tag)}
                          opacity={isHidden ? 0.4 : 1}
                          transition="opacity 0.15s"
                        >
                          <Box w="8px" h="8px" rounded="full" bg={tagColors[tag]?.color || '#A0AEC0'} flexShrink={0} />
                          <Text fontSize="xs" fontWeight="600" color="gray.300" flex={1} isTruncated>
                            {tag}
                          </Text>
                          <Text fontSize="10px" color="gray.600" flexShrink={0}>
                            {tagCounts[tag] ?? 0}
                          </Text>
                          <IconButton
                            aria-label={isHidden ? 'Show tag' : 'Hide tag'}
                            icon={isHidden ? <EyeOffIcon size={12} /> : <EyeIcon size={12} />}
                            size="xs"
                            variant="ghost"
                            color={isHidden ? 'whiteAlpha.300' : 'whiteAlpha.600'}
                            _hover={{ color: 'white', bg: 'whiteAlpha.200' }}
                            onClick={(e) => { e.stopPropagation(); onToggleTagVisibility(tag) }}
                            flexShrink={0}
                          />
                        </HStack>
                      )
                    })}
                  </PopoverBody>
                </PopoverContent>
              </Portal>
            </Popover>
          </>
        )}
      </HStack>
      <HStack
        pointerEvents="auto"
        spacing={3}
        bg="var(--bg-panel)"
        border="1px solid"
        borderColor="whiteAlpha.100"
        rounded="xl"
        boxShadow="0 5px 10px rgba(0,0,0,0.5)"
        backdropFilter="blur(20px)"
        px={3}
        py={1.5}
        data-testid="repositories-change-legend"
      >
        {EDGE_CHANGE_ORDER.map((kind) => (
          <HStack key={kind} spacing={1.5} data-testid={`repositories-edge-${kind}`}>
            <Box w="14px" h="0" borderTop="2px solid" borderColor="var(--accent)" borderTopStyle={kind === 'removed' ? 'dashed' : 'solid'} flexShrink={0} />
            <Text fontSize="10px" color="gray.400" title={EDGE_CHANGE_META[kind].label}>
              {kind} edge
            </Text>
          </HStack>
        ))}
      </HStack>
    </VStack>
  )
}
