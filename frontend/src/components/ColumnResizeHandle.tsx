import { Box } from '@chakra-ui/react'
import type { PointerEvent as ReactPointerEvent } from 'react'

/**
 * Thin draggable divider used between the resizable columns on the
 * repositories page. `aria-orientation="vertical"` describes the divider
 * itself, so it pairs with a `col-resize` cursor.
 *
 * Inline handles must stay a direct flex child of the split row to stretch
 * to its height. Overlay handles are positioned at the panel's left edge.
 */
export default function ColumnResizeHandle({
  label,
  onPointerDown,
  isActive = false,
  dataTestId,
  display,
  zIndex = 0,
  overlayWidth,
}: {
  label: string
  onPointerDown: (event: ReactPointerEvent) => void
  isActive?: boolean
  dataTestId?: string
  /** Hidden below `lg` because the split is stacked on small screens. */
  display?: 'none' | 'block' | { base?: string; lg?: string }
  /** Raise the handle above an overlaying pane so it stays grabbable. */
  zIndex?: number
  /** Width of a right-aligned overlay panel; anchors the handle to its left. */
  overlayWidth?: number
}) {
  return (
    <Box
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      data-testid={dataTestId}
      data-group=""
      display={display}
      flex="0 0 auto"
      alignSelf="stretch"
      w="6px"
      position={overlayWidth == null ? 'relative' : 'absolute'}
      top={overlayWidth == null ? undefined : 0}
      bottom={overlayWidth == null ? undefined : 0}
      right={overlayWidth == null ? undefined : `${overlayWidth}px`}
      zIndex={zIndex}
      cursor="col-resize"
      userSelect="none"
      style={{ touchAction: 'none' }}
      onPointerDown={onPointerDown}
      bg={isActive ? 'whiteAlpha.100' : 'transparent'}
      transition="background 0.2s"
      _hover={{ bg: 'whiteAlpha.100' }}
      _active={{ bg: 'whiteAlpha.100' }}
      _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '-2px' }}
    >
      <Box
        position="absolute"
        top="50%"
        left="50%"
        transform="translate(-50%, -50%)"
        w="4px"
        h={isActive ? '48px' : '40px'}
        rounded="full"
        bg={isActive ? 'whiteAlpha.500' : 'whiteAlpha.300'}
        pointerEvents="none"
        transition="all 0.2s cubic-bezier(0.4, 0, 0.2, 1)"
        _groupHover={{
          h: '48px',
          bg: 'whiteAlpha.500',
          boxShadow: '0 0 10px rgba(255, 255, 255, 0.1)',
        }}
      />
      <Box
        position="absolute"
        top="50%"
        left="50%"
        transform="translate(-50%, -50%)"
        display="flex"
        flexDirection="column"
        gap="4px"
        pointerEvents="none"
        opacity={isActive ? 0.6 : 0.3}
        _groupHover={{ opacity: 0.6 }}
        transition="opacity 0.2s"
      >
        {[0, 1, 2].map((i) => (
          <Box key={i} w="2px" h="2px" rounded="full" bg="white" />
        ))}
      </Box>
    </Box>
  )
}
