import { Box, Button, HStack, IconButton, Text } from '@chakra-ui/react'
import { CloseIcon, EyeIcon, EyeOffIcon } from './Icons'

export function TagFilterReset({ active, onReset }: { active: boolean; onReset?: () => void }) {
  if (!active || !onReset) return null
  return (
    <IconButton
      size="xs" h="22px" minW="22px" mr={1} variant="ghost" color="gray.400"
      aria-label="Reset tag filters" title="Clear selected tags"
      icon={<CloseIcon size={10} />}
      _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
      onClick={onReset}
    />
  )
}

export function TagFilterRow({ tag, count, color, hidden, selected = false, onSelect, onHover, onToggleVisibility, eyeTestId }: {
  tag: string
  count: number
  color?: string
  hidden: boolean
  selected?: boolean
  onSelect?: (tag: string) => void
  onHover: () => void
  onToggleVisibility: (tag: string) => void
  eyeTestId?: string
}) {
  const contents = <>
    <Box w="8px" h="8px" rounded="full" bg={color || '#A0AEC0'} flexShrink={0} />
    <Text fontSize="xs" fontWeight="600" color={selected ? 'var(--accent)' : 'gray.300'} flex={1} isTruncated textAlign="left">{tag}</Text>
    <Text fontSize="10px" color="gray.600" flexShrink={0}>{count}</Text>
  </>
  return (
    <HStack px={1} py={0.5} spacing={0} borderRadius="md" bg={selected ? 'rgba(var(--accent-rgb), 0.12)' : undefined} onMouseEnter={onHover} opacity={hidden ? 0.4 : 1} transition="opacity 0.15s">
      {onSelect ? (
        <Button variant="ghost" h="28px" px={1} flex={1} minW={0} aria-label={`Filter by tag ${tag}`} aria-pressed={selected} onClick={() => onSelect(tag)} _hover={{ bg: 'whiteAlpha.100' }}>
          <HStack spacing={2} w="full" minW={0}>{contents}</HStack>
        </Button>
      ) : <HStack spacing={2} px={1} flex={1} minW={0}>{contents}</HStack>}
      <IconButton
        aria-label={hidden ? 'Show tag' : 'Hide tag'}
        data-testid={eyeTestId}
        icon={hidden ? <EyeOffIcon size={12} /> : <EyeIcon size={12} />}
        size="xs" variant="ghost" flexShrink={0}
        color={hidden ? 'whiteAlpha.300' : 'whiteAlpha.600'}
        _hover={{ color: 'white', bg: 'whiteAlpha.200' }}
        onClick={(event) => { event.stopPropagation(); onToggleVisibility(tag) }}
      />
    </HStack>
  )
}
