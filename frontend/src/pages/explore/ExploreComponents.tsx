import {
  Badge,
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
import type { ExploreDiffDetail, ExploreDiffLens, ExploreDiffTarget } from '../../utils/exploreDiffLens'

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

export function ExploreDiffPanel({
  diffLens,
  diffLoading,
  activeDiffTarget,
  activeDiffTargetIndex,
  showContent,
  onExit,
  onNavigate,
}: {
  diffLens: ExploreDiffLens | null
  diffLoading: boolean
  activeDiffTarget: ExploreDiffTarget | null
  activeDiffTargetIndex: number
  showContent: boolean
  onExit: () => void
  onNavigate: (offset: number) => void
}) {
  return (
    <Box
      position="absolute"
      top={4}
      right={4}
      zIndex={14}
      className="glass"
      borderRadius="lg"
      px={3}
      py={2.5}
      w={{ base: 'calc(100vw - 32px)', md: '340px' }}
      maxW="calc(100vw - 32px)"
      pointerEvents="auto"
      opacity={showContent ? 1 : 0}
      transition="opacity 0.3s"
    >
      <VStack align="stretch" spacing={2}>
        <HStack justify="space-between" spacing={3}>
          <HStack spacing={2} minW={0}>
            <Badge colorScheme="blue" variant="subtle">Diff map</Badge>
            <Text fontSize="xs" color="gray.400" fontFamily="mono" flexShrink={0}>
              +{diffLens?.totalAddedLines ?? 0} -{diffLens?.totalRemovedLines ?? 0}
            </Text>
          </HStack>
          <Button size="xs" variant="ghost" color="gray.300" onClick={onExit}>
            Exit
          </Button>
        </HStack>
        <Text fontSize="xs" color="gray.200" noOfLines={1} minH="18px">
          {diffLoading
            ? 'Loading changed resources...'
            : activeDiffTarget
              ? `${activeDiffTargetIndex + 1} of ${diffLens?.orderedTargets.length ?? 0}: ${activeDiffTarget.label}`
              : 'No placed changed resources'}
        </Text>
        <HStack spacing={2}>
          <Button
            size="xs"
            variant="solid"
            bg="whiteAlpha.200"
            _hover={{ bg: 'whiteAlpha.300' }}
            flex={1}
            isDisabled={!diffLens?.orderedTargets.length}
            onClick={() => onNavigate(-1)}
          >
            Previous
          </Button>
          <Button
            size="xs"
            variant="solid"
            bg="whiteAlpha.200"
            _hover={{ bg: 'whiteAlpha.300' }}
            flex={1}
            isDisabled={!diffLens?.orderedTargets.length}
            onClick={() => onNavigate(1)}
          >
            Next
          </Button>
        </HStack>
      </VStack>
    </Box>
  )
}

export function ExploreUnplacedDiffPanel({
  diffLens,
  onOpenDiffSource,
}: {
  diffLens: ExploreDiffLens
  onOpenDiffSource: (detail: ExploreDiffDetail) => void
}) {
  if (diffLens.unplacedTargets.length === 0) return null
  return (
    <Box
      position="absolute"
      top={{ base: '150px', md: '132px' }}
      right={4}
      zIndex={13}
      className="glass"
      borderRadius="lg"
      px={3}
      py={3}
      w={{ base: 'calc(100vw - 32px)', md: '340px' }}
      maxH="260px"
      overflowY="auto"
      pointerEvents="auto"
      data-zui-native-wheel="true"
      sx={{ overscrollBehavior: 'contain', WebkitOverflowScrolling: 'touch', touchAction: 'pan-y' }}
    >
      <VStack align="stretch" spacing={2}>
        <Text fontSize="11px" color="gray.400" fontWeight="700" textTransform="uppercase">
          Deleted or unplaced
        </Text>
        {diffLens.unplacedTargets.slice(0, 8).map((target) => (
          <Box key={target.key} borderTop="1px solid" borderColor="whiteAlpha.100" pt={2}>
            <HStack spacing={2} align="start">
              <Badge colorScheme={target.changeType === 'deleted' ? 'red' : 'yellow'} variant="subtle" fontSize="9px">
                {target.changeType}
              </Badge>
              <Box minW={0} flex={1}>
                <Text fontSize="xs" color="gray.100" noOfLines={1}>{target.label}</Text>
                {target.sourcePath && (
                  <Text fontSize="10px" color="gray.500" fontFamily="mono" noOfLines={1}>{target.sourcePath}</Text>
                )}
              </Box>
              {target.sourcePath && (
                <Button size="xs" variant="ghost" color="var(--accent)" onClick={() => onOpenDiffSource(target)}>
                  Open
                </Button>
              )}
            </HStack>
          </Box>
        ))}
        {diffLens.unplacedTargets.length > 8 && (
          <Text fontSize="xs" color="gray.500">
            +{diffLens.unplacedTargets.length - 8} more
          </Text>
        )}
      </VStack>
    </Box>
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
                            {layer.name}
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
