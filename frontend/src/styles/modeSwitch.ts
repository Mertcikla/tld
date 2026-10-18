import type { SystemStyleObject } from '@chakra-ui/react'

// Share presentation without changing each page's button/tab semantics or routing.
export const modeSwitchGroupStyles: SystemStyleObject = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: '2px',
  p: '3px',
  bg: 'blackAlpha.200',
  border: '1px solid',
  borderColor: 'whiteAlpha.100',
  borderRadius: 'lg',
  flexShrink: 0,
}

export const modeSwitchOptionStyles: SystemStyleObject = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  h: '28px',
  minW: 0,
  px: 3,
  border: '1px solid transparent',
  borderRadius: 'md',
  bg: 'transparent',
  color: 'gray.400',
  fontSize: 'xs',
  fontWeight: 'semibold',
  lineHeight: 'short',
  whiteSpace: 'nowrap',
  transition: 'background-color 0.15s ease, color 0.15s ease, border-color 0.15s ease',
  _hover: {
    bg: 'whiteAlpha.50',
    color: 'gray.100',
    _disabled: { bg: 'transparent', color: 'gray.400' },
  },
  '&[aria-selected="true"], &[aria-pressed="true"]': {
    bg: 'rgba(var(--accent-rgb), 0.14)',
    borderColor: 'rgba(var(--accent-rgb), 0.3)',
    color: 'gray.100',
  },
  _focusVisible: {
    outline: '2px solid var(--accent)',
    outlineOffset: '2px',
  },
}
