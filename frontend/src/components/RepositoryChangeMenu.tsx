import { Box, Button, HStack, Text, Tooltip, VStack } from '@chakra-ui/react'
import { MarkdownIcon } from './Icons'

export type RepositoryChangeViewMode = 'standard' | 'plain'

export default function RepositoryChangeMenu({
  radius,
  maxRadius,
  onRadiusChange,
  busy = false,
  viewMode,
  onViewModeChange,
  mermaidOpen,
  onToggleMermaid,
  hasMermaid = false,
}: {
  radius: number
  maxRadius: number
  onRadiusChange: (radius: number) => void
  busy?: boolean
  viewMode: RepositoryChangeViewMode
  onViewModeChange: (mode: RepositoryChangeViewMode) => void
  mermaidOpen: boolean
  onToggleMermaid: () => void
  hasMermaid?: boolean
}) {
  const radiusStops = Array.from({ length: Math.min(3, maxRadius) + 1 }, (_, r) => r)

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
          <Text fontSize="10px" fontWeight="bold" color="gray.500" textTransform="uppercase" letterSpacing="0.06em">
            View
          </Text>
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

        <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />

        <HStack spacing={1} align="center" data-testid="repositories-radius" px={1}>
          <Text fontSize="10px" fontWeight="bold" color="gray.500" textTransform="uppercase" letterSpacing="0.06em">
            Radius
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

        <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />

        <Tooltip label={mermaidOpen ? 'Hide change diagram' : 'Show change diagram'} placement="top" openDelay={200}>
          <Button
            data-testid="repository-change-mermaid-toggle"
            variant="ghost"
            h="28px"
            px={2.5}
            color={mermaidOpen && hasMermaid ? 'var(--accent)' : 'gray.300'}
            bg={mermaidOpen && hasMermaid ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
            isDisabled={!hasMermaid}
            _disabled={{ opacity: 0.35, cursor: 'not-allowed' }}
            _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
            onClick={onToggleMermaid}
            aria-pressed={mermaidOpen}
          >
            <HStack spacing={1.5}>
              <MarkdownIcon />
              <Text fontSize="11px" fontWeight={mermaidOpen ? 'semibold' : 'normal'}>
                Mermaid
              </Text>
            </HStack>
          </Button>
        </Tooltip>
      </HStack>
    </VStack>
  )
}
