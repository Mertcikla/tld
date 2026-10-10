import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react'
import type { RepositoryChangeScope } from '../utils/repositoryChangeScene'

export type RepositoryChangeViewMode = 'standard' | 'plain'

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
    </VStack>
  )
}
