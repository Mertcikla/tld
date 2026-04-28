import { memo } from 'react'
import { Box, BoxProps, forwardRef } from '@chakra-ui/react'
import { useAccentColor } from '../context/ThemeContext'
import { hexToRgba } from '../constants/colors'

export interface ElementContainerProps extends BoxProps {
  isSelected?: boolean
  isSource?: boolean
  isTarget?: boolean
  isConnectorHighlighted?: boolean
  hasStack?: boolean
  kind?: string | null
}

export const ElementContainer = memo(forwardRef<ElementContainerProps, 'div'>(({
  isSelected,
  isSource,
  isTarget,
  isConnectorHighlighted,
  hasStack,
  kind,
  children,
  ...props
}, ref) => {
  const { accent } = useAccentColor()

  const brandedBorder = hexToRgba('#a0aec0', 0.5)

  const borderColor = isSource
    ? accent
    : isTarget
      ? 'teal.300'
      : isSelected || isConnectorHighlighted
        ? accent
        : brandedBorder

  // Shadows matching ZUICanvas / high-fidelity look
  const selectionShadow      = `0 0 0 3px ${hexToRgba(accent, 0.35)}, 0 10px 36px rgba(0,0,0,0.55), 0 3px 10px rgba(0,0,0,0.4)`
  const sourceShadow         = `0 0 0 3px ${hexToRgba(accent, 0.55)}, 0 0 24px ${hexToRgba(accent, 0.25)}`
  const edgeHighlightShadow  = `0 0 0 2px ${hexToRgba(accent, 0.2)}, 0 8px 32px rgba(0,0,0,0.55), 0 2px 8px rgba(0,0,0,0.35)`
  const restingShadow        = '0 8px 32px rgba(0,0,0,0.55), 0 2px 8px rgba(0,0,0,0.35)'
  const hoverShadow          = isSource ? sourceShadow : isSelected ? selectionShadow : '0 12px 40px rgba(0,0,0,0.6), 0 4px 12px rgba(0,0,0,0.4)'

  const boxShadow = isSource ? sourceShadow : isSelected ? selectionShadow : isConnectorHighlighted ? edgeHighlightShadow : restingShadow

  const finalBorderColor = borderColor

  return (
    <Box position="relative" zIndex={1}>
      {hasStack && (
        <>
          {/* Stack effect matching ZUI renderer.ts (offset 4px and 8px) */}
          <Box
            position="absolute"
            inset={0}
            transform="translate(8px, 8px)"
            bg="var(--bg-element)"
            borderColor={finalBorderColor}
            borderWidth="1px"
            rounded="lg"
            opacity={0.4}
            zIndex={-2}
          />
          <Box
            position="absolute"
            inset={0}
            transform="translate(4px, 4px)"
            bg="var(--bg-element)"
            borderColor={finalBorderColor}
            borderWidth="1px"
            rounded="lg"
            opacity={0.7}
            zIndex={-1}
          />
        </>
      )}
      <Box
        ref={ref}
        bg="var(--bg-element)"
        borderColor={finalBorderColor}
        borderWidth="1px"
        rounded="lg"
        boxShadow={boxShadow}
        transition="all var(--chakra-transitions-duration-fast) var(--chakra-transitions-easing-pop)"
        position="relative"
        _hover={{
          borderColor: isSource ? accent : isTarget ? 'teal.200' : accent,
          boxShadow: hoverShadow,
        }}
        {...props}
      >
        {children}
      </Box>
    </Box>
  )
}))
