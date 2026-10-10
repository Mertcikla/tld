import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react'
import type { RepositoryChangeScope } from '../utils/repositoryChangeScene'
import { PROVENANCE_META, type ZUINodeProvenance } from './ZUI/provenance'
import { EDGE_CHANGE_META, type ZUIEdgeChange } from './ZUI/edgeChange'

export type RepositoryChangeViewMode = 'standard' | 'plain'

const PROVENANCE_ORDER: ZUINodeProvenance[] = ['authored', 'augmented', 'generated']
const EDGE_CHANGE_ORDER: ZUIEdgeChange[] = ['added', 'removed', 'modified']

export default function RepositoryChangeMenu({
  radius,
  maxRadius,
  onRadiusChange,
  busy = false,
  viewMode,
  onViewModeChange,
  scope,
  onScopeChange,
  authoredCount,
  showProvenanceLegend = false,
}: {
  radius: number
  maxRadius: number
  onRadiusChange: (radius: number) => void
  busy?: boolean
  viewMode: RepositoryChangeViewMode
  onViewModeChange: (mode: RepositoryChangeViewMode) => void
  scope: RepositoryChangeScope
  onScopeChange: (scope: RepositoryChangeScope) => void
  authoredCount: number
  showProvenanceLegend?: boolean
}) {
  const radiusStops = Array.from({ length: Math.max(0, maxRadius) + 1 }, (_, r) => r)

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

        <Box w="1px" h="16px" bg="whiteAlpha.200" flexShrink={0}/>

        <HStack spacing={1} align="center" data-testid="repositories-diagram-scope" px={1}>
          {(['grounded', 'mapped', 'authored'] as const).map((option) => {
            const disabled = busy || (option === 'authored' && authoredCount === 0)
            return (
              <Button
                key={option}
                data-testid={`repositories-diagram-scope-${option}`}
                title={option === 'authored' && authoredCount === 0 ? 'No authored elements matched this change' : undefined}
                size="xs"
                h="28px"
                px={2}
                variant={scope === option ? 'solid' : 'ghost'}
                isDisabled={disabled}
                onClick={() => {
                  if (option !== scope) onScopeChange(option)
                }}
              >
                {option === 'grounded' ? 'Grounded' : option === 'mapped' ? 'Mapped' : 'Authored'}
              </Button>
            )
          })}
        </HStack>

        <Box w="1px" h="16px" bg="whiteAlpha.200" flexShrink={0}/>

        <HStack spacing={1} align="center" data-testid="repositories-radius" px={1}>
          <Text fontSize="10px" fontWeight="bold" color="gray.500"letterSpacing="0.06em">
            Blast Radius
          </Text>
          {radiusStops.map((r) => (
            <Button
              key={r}
              data-testid={`repositories-radius-${r}`}
              size="xs"
              minW="28px"
              h="28px"
              px={0}
              variant={radius === r ? 'solid' : 'ghost'}
              isDisabled={busy}
              onClick={() => {
                if (r !== radius) onRadiusChange(r)
              }}
            >
              {r}
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
