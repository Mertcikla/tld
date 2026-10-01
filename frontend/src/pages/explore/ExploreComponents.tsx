import {
  Box,
  Button,
  Center,
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
import type { ReactNode } from 'react'
import type { CrossBranchConnectorPriority, CrossBranchContextSettings } from '../../crossBranch/types'
import type { Tag, ViewLayer } from '../../types'
import CrossBranchControls from '../../components/CrossBranchControls'
import ExplorePageOnboarding from '../../components/ExplorePageOnboarding'
import { EyeIcon, EyeOffIcon, FitViewIcon as FitViewSvg, TagsIcon } from '../../components/Icons'
import { isElementGroupLayer } from '../../utils/elementGroups'

export function ExploreEmptyState({
  noDiagrams,
  sharedToken,
  error,
  onRetry,
  onGoToViews,
}: {
  noDiagrams: boolean
  sharedToken?: string
  error?: string | null
  onRetry: () => void
  onGoToViews: () => void
}) {
  return (
    <Center h="100%" flexDir="column" gap={4} px={6} textAlign="center">
      <VStack spacing={2}>
        <Text color="gray.300" fontWeight="bold" fontSize="lg">
          {error ? 'Could not load explore map' : noDiagrams ? 'No diagrams to explore yet' : 'Your diagrams are empty'}
        </Text>
        <Text color="gray.500" fontSize="sm" maxW="400px">
          {error
            ? error
            : noDiagrams
              ? 'Start by creating your first diagram in the workspace.'
              : 'Add elements to your diagrams in the editor to see them rendered on this infinite canvas.'}
        </Text>
      </VStack>

      {error ? (
        <Button size="sm" colorScheme="blue" onClick={onRetry} borderRadius="full" px={6}>
          Retry
        </Button>
      ) : !sharedToken && (
        <Button size="sm" colorScheme="blue" onClick={onGoToViews} borderRadius="full" px={6}>
          {noDiagrams ? 'Create First Diagram' : 'Go to Editor'}
        </Button>
      )}
      {!error && !noDiagrams && !sharedToken && <ExplorePageOnboarding hasDiagrams={!noDiagrams} />}
    </Center>
  )
}

export function ExploreToolbar({
  showContent,
  shareSlot,
  crossBranchSettings,
  onCrossBranchEnabledChange,
  onCrossBranchBudgetChange,
  onCrossBranchPriorityChange,
  onCrossBranchOpenChange,
  allTags,
  tagColors,
  layers,
  layerElementCounts,
  tagCounts,
  hiddenTags,
  isTagsOpen,
  onTagsClose,
  onTagsToggle,
  setHighlightedTags,
  setHighlightColor,
  toggleLayerVisibility,
  toggleTagVisibility,
  onFitView,
}: {
  showContent: boolean
  shareSlot?: ReactNode
  crossBranchSettings: CrossBranchContextSettings
  onCrossBranchEnabledChange: (enabled: boolean) => void
  onCrossBranchBudgetChange: (budget: number) => void
  onCrossBranchPriorityChange: (priority: CrossBranchConnectorPriority) => void
  onCrossBranchOpenChange: (isOpen: boolean) => void
  allTags: string[]
  tagColors: Record<string, Tag>
  layers: ViewLayer[]
  layerElementCounts: Record<number, number>
  tagCounts: Record<string, number>
  hiddenTags: string[]
  isTagsOpen: boolean
  onTagsClose: () => void
  onTagsToggle: () => void
  setHighlightedTags: (tags: string[]) => void
  setHighlightColor: (color: string) => void
  toggleLayerVisibility: (layer: ViewLayer) => void
  toggleTagVisibility: (tag: string) => void
  onFitView: () => void
}) {
  return (
    <Box
      position="absolute"
      bottom={4}
      left="50%"
      transform="translateX(-50%)"
      zIndex={10}
      className="glass"
      borderRadius="lg"
      boxShadow="0 5px 10px rgba(0,0,0,0.5)"
      px={2}
      py={1}
      opacity={showContent ? 1 : 0}
      transition="opacity 0.3s"
    >
      <HStack spacing={0}>
        <Tooltip label="Fit View" placement="top" openDelay={200}>
          <Button
            data-testid="zui-fit-view"
            variant="ghost" h="28px" px={2.5}
            color="gray.300"
            _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
            onClick={onFitView}
          >
            <HStack spacing={1.5}>
              <FitViewSvg />
              <Text fontSize="11px" fontWeight="normal">Fit View</Text>
            </HStack>
          </Button>
        </Tooltip>

        {shareSlot}

        <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />
        <CrossBranchControls
          settings={crossBranchSettings}
          onEnabledChange={onCrossBranchEnabledChange}
          onBudgetChange={onCrossBranchBudgetChange}
          onPriorityChange={onCrossBranchPriorityChange}
          onOpenChange={onCrossBranchOpenChange}
          label="Filters"
        />

        {(allTags.length > 0 || layers.length > 0) && (
          <>
            <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />
            <Popover
              isOpen={isTagsOpen}
              onClose={() => { onTagsClose(); setHighlightedTags([]); setHighlightColor('') }}
              placement="top"
              isLazy
              closeOnBlur
            >
              <PopoverTrigger>
                <Button
                  data-testid="zui-tags-button"
                  variant="ghost" h="28px" px={2.5}
                  color={isTagsOpen ? 'var(--accent)' : 'gray.300'}
                  _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
                  onClick={onTagsToggle}
                >
                  <HStack spacing={1.5}>
                    <TagsIcon />
                    <Text fontSize="11px" fontWeight="normal">Tags</Text>
                  </HStack>
                </Button>
              </PopoverTrigger>
              <Portal>
                <PopoverContent
                  data-testid="zui-tags-panel"
                  data-zui-native-wheel="true"
                  bg="glass.bg"
                  backdropFilter="blur(16px)"
                  borderColor="glass.border"
                  boxShadow="panel"
                  borderRadius="lg"
                  width="220px"
                  _focus={{ boxShadow: 'none' }}
                  onMouseLeave={() => { setHighlightedTags([]); setHighlightColor('') }}
                >
                  <PopoverBody p={2} maxH="360px" overflowY="auto">
                    {layers.map((layer) => {
                      const isHidden = layer.tags.length > 0 && layer.tags.every((tag) => hiddenTags.includes(tag))
                      return (
                        <HStack
                          key={`layer-${layer.id}`}
                          px={2}
                          py={1}
                          spacing={2}
                          borderRadius="md"
                          _hover={{ bg: 'whiteAlpha.100' }}
                          onMouseEnter={() => { setHighlightedTags(layer.tags); setHighlightColor(layer.color || '') }}
                          opacity={isHidden ? 0.4 : 1}
                          transition="opacity 0.15s"
                        >
                          <Box w="10px" h="10px" rounded="full" bg={layer.color || 'gray.500'} flexShrink={0} />
                          <Text fontSize="xs" fontWeight="600" color="white" flex={1} isTruncated>
                            {isElementGroupLayer(layer) ? `group:${layer.name}` : layer.name}
                          </Text>
                          <Text fontSize="10px" color="gray.600" flexShrink={0}>
                            {layerElementCounts[layer.id] ?? 0}
                          </Text>
                          <IconButton
                            aria-label={isHidden ? 'Show layer' : 'Hide layer'}
                            icon={isHidden ? <EyeOffIcon size={12} /> : <EyeIcon size={12} />}
                            size="xs"
                            variant="ghost"
                            color={isHidden ? 'whiteAlpha.300' : 'whiteAlpha.600'}
                            _hover={{ color: 'white', bg: 'whiteAlpha.200' }}
                            onClick={(e) => { e.stopPropagation(); toggleLayerVisibility(layer) }}
                            flexShrink={0}
                          />
                        </HStack>
                      )
                    })}

                    {allTags.map((tag) => {
                      const isHidden = hiddenTags.includes(tag)
                      return (
                        <HStack
                          key={`tag-${tag}`}
                          px={2}
                          py={1}
                          spacing={2}
                          borderRadius="md"
                          onMouseEnter={() => { setHighlightedTags([tag]); setHighlightColor(tagColors[tag]?.color || '') }}
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
                            onClick={(e) => { e.stopPropagation(); toggleTagVisibility(tag) }}
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
    </Box>
  )
}
