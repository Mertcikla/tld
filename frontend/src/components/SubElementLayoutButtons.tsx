import { HStack, IconButton, Tooltip } from '@chakra-ui/react'
import type { SubDetail } from '../utils/subElements'

const layouts: Array<{ value: SubDetail; label: string; path: string }> = [
  { value: 'overview', label: 'Card layout', path: 'M3 3h14v14H3z M8 7h4 M6 11h8 M7 14h6' },
  { value: 'outline', label: 'Collapsed symbols layout', path: 'M4 5h12 M4 10h12 M4 15h12' },
  { value: 'code', label: 'Full layout', path: 'm7 5-5 5 5 5 M13 5l5 5-5 5' },
]

export default function SubElementLayoutButtons({ detail, onChange }: {
  detail: SubDetail; onChange: (detail: SubDetail) => void
}) {
  return <HStack className="nodrag nopan" position="absolute" top="calc(100% + 6px)" right={0} spacing="1px" zIndex={12}
    bg="#20252d" borderRadius="5px" p="1px" onPointerDown={event => event.stopPropagation()} onClick={event => event.stopPropagation()}>
    {layouts.map(layout => <Tooltip key={layout.value} label={layout.label} openDelay={350}>
      <IconButton aria-label={layout.label} aria-pressed={detail === layout.value} variant="ghost" minW="20px" w="20px" h="20px" borderRadius="3px"
        color={detail === layout.value ? 'var(--accent)' : 'gray.500'} bg={detail === layout.value ? 'whiteAlpha.100' : 'transparent'}
        _hover={{ color: 'gray.100', bg: 'whiteAlpha.200' }} onClick={() => onChange(layout.value)}
        icon={<svg width="13" height="13" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round" aria-hidden="true"><path d={layout.path} /></svg>} />
    </Tooltip>)}
  </HStack>
}
