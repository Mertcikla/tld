import { memo } from 'react'
import { Box, BoxProps, forwardRef } from '@chakra-ui/react'
import { useAccentColor } from '../context/ThemeContext'
import { hexToRgba } from '../constants/colors'

export interface ElementContainerProps extends BoxProps {
  isSelected?: boolean
  isSource?: boolean
  isTarget?: boolean
  isConnectorHighlighted?: boolean
}

export const ElementContainer = memo(forwardRef<ElementContainerProps, 'div'>(({
  isSelected,
  isSource,
  isTarget,
  isConnectorHighlighted,
  children,
  ...props
}, ref) => {
  const { accent } = useAccentColor()

  const borderColor = isSource
    ? accent
    : isTarget
      ? 'teal.300'
      : isSelected || isConnectorHighlighted
        ? accent
        : 'gray.600'

  const selectionShadow      = `0 0 0 3px ${hexToRgba(accent, 0.35)}, 0 10px 36px rgba(0,0,0,0.55), 0 3px 10px rgba(0,0,0,0.4)`
  const sourceShadow         = `0 0 0 3px ${hexToRgba(accent, 0.55)}, 0 0 24px ${hexToRgba(accent, 0.25)}`
  const edgeHighlightShadow  = `0 0 0 2px ${hexToRgba(accent, 0.2)}, 0 8px 32px rgba(0,0,0,0.55), 0 2px 8px rgba(0,0,0,0.35)`
  const restingShadow        = '0 8px 32px rgba(0,0,0,0.55), 0 2px 8px rgba(0,0,0,0.35)'
  const hoverShadow          = isSource ? sourceShadow : isSelected ? selectionShadow : '0 12px 40px rgba(0,0,0,0.6), 0 4px 12px rgba(0,0,0,0.4)'

  const boxShadow = isSource ? sourceShadow : isSelected ? selectionShadow : isConnectorHighlighted ? edgeHighlightShadow : restingShadow

  return (
    <Box
      ref={ref}
      bg="var(--bg-element)"
      borderColor={borderColor}
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
  )
}))
