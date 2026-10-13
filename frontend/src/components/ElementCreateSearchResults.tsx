import { Badge, Box, HStack, Text, VStack } from '@chakra-ui/react'
import type { LibraryElement } from '../types'
import { TYPE_COLORS } from '../types'
import type { ElementCreateSearchResult } from './elementCreateSearch'

interface ElementCreateSearchResultsProps {
  results: ElementCreateSearchResult[]
  activeIndex: number
  busy?: boolean
  query: string
  existingElementIds: Set<number>
  testIdPrefix: string
  getSecondaryLabel?: (obj: LibraryElement) => string | null
  getDisabledReason?: (obj: LibraryElement) => string | null
  onActiveIndexChange: (index: number) => void
  onConfirm: (index: number) => void
}

export default function ElementCreateSearchResults({
  results,
  activeIndex,
  busy = false,
  query,
  existingElementIds,
  testIdPrefix,
  getSecondaryLabel,
  getDisabledReason,
  onActiveIndexChange,
  onConfirm,
}: ElementCreateSearchResultsProps) {
  if (results.length === 0) return null

  return (
    <VStack spacing={0} align="stretch">
      {results.map((item, i) => {
        const disabledReason = item.kind === 'existing' ? getDisabledReason?.(item.obj) : null
        return (
          <Box
            data-testid={item.kind === 'new' ? `${testIdPrefix}-create-option` : `${testIdPrefix}-existing-option`}
            key={item.kind === 'new' ? `new:${item.label}` : `existing:${item.obj.id}`}
            px={3}
            py={1.5}
            bg={i === activeIndex ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
            aria-disabled={!!disabledReason || busy}
            opacity={disabledReason ? 0.5 : 1}
            cursor={busy || disabledReason ? 'not-allowed' : 'pointer'}
            _hover={{ bg: i === activeIndex ? 'rgba(var(--accent-rgb), 0.12)' : 'whiteAlpha.100' }}
            onMouseEnter={() => onActiveIndexChange(i)}
            onClick={() => !busy && !disabledReason && onConfirm(i)}
            transition="background 0.1s"
          >
            {item.kind === 'new' ? (
              <HStack spacing={1.5}>
                <Text fontSize="10px" color="var(--accent)" fontWeight="bold" flexShrink={0}>
                  + Create
                </Text>
                <Text
                  fontSize="sm"
                  color="white"
                  noOfLines={1}
                  fontStyle={!query.trim() ? 'italic' : 'normal'}
                  pr={1}
                >
                  {item.label}
                </Text>
              </HStack>
            ) : (
              <HStack spacing={2}>
                <Box flex={1} minW={0}>
                  <Text fontSize="sm" color="gray.200" noOfLines={1}>
                    {item.obj.name}
                  </Text>
                  {(disabledReason || getSecondaryLabel?.(item.obj) || item.obj.technology) && (
                    <Text fontSize="10px" color="gray.500" noOfLines={1}>
                      {disabledReason || getSecondaryLabel?.(item.obj) || item.obj.technology}
                    </Text>
                  )}
                </Box>
                <Badge
                  colorScheme={TYPE_COLORS[item.obj.kind ?? ''] ?? 'gray'}
                  fontSize="8px"
                  flexShrink={0}
                >
                  {item.obj.kind}
                </Badge>
                {existingElementIds.has(item.obj.id) && (
                  <Text fontSize="9px" color="gray.500" flexShrink={0}>
                    on canvas
                  </Text>
                )}
              </HStack>
            )}
          </Box>
        )
      })}
    </VStack>
  )
}
