import { Box, HStack, Text, type BoxProps } from '@chakra-ui/react'
import type { MouseEvent } from 'react'

/** Shared source banner for offscreen symbols and folded connected-only ranges. */
export default function SubElementBanner({ label, connectionLabel = '', direction, textless = false, enabled = true, onClick, ...props }: Omit<BoxProps, 'onClick'> & {
  label: string; connectionLabel?: string; direction: 'above' | 'below' | 'collapsed'; textless?: boolean; enabled?: boolean
  onClick?: (event: MouseEvent<HTMLElement>) => void
}) {
  const description = [label, connectionLabel].filter(Boolean).join(' · ')
  return <Box as="button" type="button" disabled={!enabled} className="nodrag nopan" aria-label={enabled ? `Show ${description}` : description}
    position="absolute" left={0} right={0} h="16px" w="100%" p={0} bg="var(--bg-source-preview, #232732)" borderTop="1px solid" borderBottom={direction === 'above' || direction === 'collapsed' ? '1px solid' : undefined} borderColor="var(--border-main)" zIndex={6} fontSize="11px"
    transition="background-color 120ms, border-color 120ms"
    _hover={enabled ? { bg: 'color-mix(in srgb, var(--bg-source-preview, #232732) 94%, var(--accent))', borderColor: 'color-mix(in srgb, var(--accent) 35%, transparent)' } : undefined}
    _focusVisible={{ outline: '1px solid var(--accent)', outlineOffset: '-2px' }} overflow="hidden" cursor={enabled ? 'pointer' : 'default'}
    onClick={event => { event.stopPropagation(); onClick?.(event) }} {...props}>
    <HStack h="100%" justify="center" spacing="6px" px={8} fontSize="inherit" lineHeight={1} color="gray.400" pointerEvents="none" style={{ fontVariantNumeric: 'tabular-nums' }}>
      {enabled && <Box as="svg" boxSize="0.9em" viewBox="0 0 8 8" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true" flexShrink={0}><path d={direction === 'above' ? 'M1 5l3-3 3 3' : 'M1 2l3 3 3-3'} /></Box>}
      {!textless && label && <Text isTruncated>{label}</Text>}
      {!textless && label && connectionLabel && <Text color="whiteAlpha.300">·</Text>}
      {!textless && connectionLabel && <Text color="var(--accent)" fontWeight="600" flexShrink={0}>{connectionLabel}</Text>}
    </HStack>
  </Box>
}
