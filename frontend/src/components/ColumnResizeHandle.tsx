import { Box } from '@chakra-ui/react'
import type { PointerEvent as ReactPointerEvent } from 'react'

/**
 * Thin draggable divider used between the resizable columns on the
 * repositories page. `aria-orientation="vertical"` describes the divider
 * itself, so it pairs with a `col-resize` cursor.
 *
 * It must stay a direct flex child of the split row: it only gets its height
 * from `align-self: stretch`, which does nothing inside a plain block wrapper.
 */
export default function ColumnResizeHandle({
  label,
  onPointerDown,
  isActive = false,
  dataTestId,
  display,
  zIndex = 0,
}: {
  label: string
  onPointerDown: (event: ReactPointerEvent) => void
  isActive?: boolean
  dataTestId?: string
  /** Hidden below `lg` because the split is stacked on small screens. */
  display?: 'none' | 'block' | { base?: string; lg?: string }
  /** Raise the handle above an overlaying pane so it stays grabbable. */
  zIndex?: number
}) {
  return (
    <Box
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      data-testid={dataTestId}
      display={display}
      flex="0 0 auto"
      alignSelf="stretch"
      w="8px"
      position="relative"
      zIndex={zIndex}
      cursor="col-resize"
      style={{ touchAction: 'none' }}
      onPointerDown={onPointerDown}
      bg={isActive ? 'whiteAlpha.100' : 'transparent'}
      _hover={{ bg: 'whiteAlpha.50' }}
      _active={{ bg: 'whiteAlpha.100' }}
      _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '-2px' }}
    >
      <Box
        position="absolute"
        top={0}
        bottom={0}
        left="50%"
        transform="translateX(-50%)"
        w="1px"
        bg={isActive ? 'var(--accent)' : 'whiteAlpha.200'}
      />
    </Box>
  )
}