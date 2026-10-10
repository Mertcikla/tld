import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react'
import { PROVENANCE_META, type ZUINodeProvenance } from './ZUI/provenance'
import { EDGE_CHANGE_META, type ZUIEdgeChange } from './ZUI/edgeChange'

export type RepositoryChangeViewMode = 'standard' | 'plain'

const PROVENANCE_ORDER: ZUINodeProvenance[] = ['authored', 'augmented', 'generated']
const EDGE_CHANGE_ORDER: ZUIEdgeChange[] = ['added', 'removed', 'modified']

export default function RepositoryChangeMenu({
  busy = false,
  viewMode,
  onViewModeChange,
  showProvenanceLegend = false,
}: {
  busy?: boolean
  viewMode: RepositoryChangeViewMode
  onViewModeChange: (mode: RepositoryChangeViewMode) => void
  showProvenanceLegend?: boolean
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
      </HStack>
      {showProvenanceLegend && (
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
          data-testid="repositories-provenance-legend"
        >
          {PROVENANCE_ORDER.map((kind) => (
            <HStack key={kind} spacing={1.5} data-testid={`repositories-provenance-${kind}`}>
              <Box boxSize="8px" borderRadius="full" bg={PROVENANCE_META[kind].color} flexShrink={0} />
              <Text fontSize="10px" color="gray.400" title={PROVENANCE_META[kind].blurb}>
                {PROVENANCE_META[kind].glyph} {PROVENANCE_META[kind].label}
              </Text>
            </HStack>
          ))}
          <Box w="1px" h="14px" bg="whiteAlpha.200" flexShrink={0} />
          {EDGE_CHANGE_ORDER.map((kind) => (
            <HStack key={kind} spacing={1.5} data-testid={`repositories-edge-${kind}`}>
              <Box w="14px" h="0" borderTop="2px solid" borderColor={EDGE_CHANGE_META[kind].color} borderTopStyle={kind === 'removed' ? 'dashed' : 'solid'} flexShrink={0} />
              <Text fontSize="10px" color="gray.400" title={EDGE_CHANGE_META[kind].label}>
                {kind} edge
              </Text>
            </HStack>
          ))}
        </HStack>
      )}
    </VStack>
  )
}
