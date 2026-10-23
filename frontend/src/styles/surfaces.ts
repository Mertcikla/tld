import type { SystemStyleObject } from '@chakra-ui/react'

// Docked and floating UI share a surface; geometry and elevation express placement.
export const panelSurfaceStyles: SystemStyleObject = {
  bg: 'var(--bg-panel)',
  bgImage: 'none',
  borderColor: 'var(--border-main)',
}

export const floatingSurfaceStyles: SystemStyleObject = {
  border: '1px solid',
  ...panelSurfaceStyles,
  borderRadius: 'xl',
  backdropFilter: 'blur(20px)',
  boxShadow: 'panel',
}

export const canvasPopoverContentStyles: SystemStyleObject = {
  ...floatingSurfaceStyles,
  bg: 'var(--bg-menu)',
  overflow: 'hidden',
  _focus: { boxShadow: 'none', outline: 'none' },
}

export const canvasPopoverPillStyles: SystemStyleObject = {
  px: 2,
  py: 0.5,
  borderRadius: 'full',
  fontSize: '9px',
  fontWeight: '700',
  textTransform: 'uppercase',
  letterSpacing: '0.06em',
  lineHeight: '1.2',
  flexShrink: 0,
}

export const canvasPopoverKindLabelStyles: SystemStyleObject = {
  color: 'gray.400',
  fontSize: '10px',
  fontWeight: '500',
  lineHeight: '1.2',
  maxW: '40%',
}

export const canvasPopoverInsetCardStyles: SystemStyleObject = {
  bg: 'whiteAlpha.50',
  border: '1px solid',
  borderColor: 'whiteAlpha.100',
  borderRadius: 'lg',
}

export const canvasPopoverNameButtonStyles: SystemStyleObject = {
  height: 'auto',
  minH: '22px',
  px: 2,
  py: 0.5,
  borderRadius: 'md',
  bg: 'transparent',
  border: '1px solid',
  borderColor: 'transparent',
  boxShadow: 'none',
  textDecoration: 'none',
  color: 'gray.100',
  fontWeight: '600',
  fontSize: '11px',
  lineHeight: '1.2',
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: 1,
  maxW: '100%',
  minW: 0,
  '& .chakra-button__icon': {
    opacity: 0,
    transition: 'opacity 120ms ease',
  },
  _hover: {
    bg: 'rgba(var(--accent-rgb), 0.12)',
    borderColor: 'rgba(var(--accent-rgb), 0.32)',
    color: 'var(--accent)',
    textDecoration: 'none',
    '& .chakra-button__icon': { opacity: 1 },
  },
  _focusVisible: {
    bg: 'rgba(var(--accent-rgb), 0.12)',
    borderColor: 'rgba(var(--accent-rgb), 0.32)',
    color: 'var(--accent)',
    '& .chakra-button__icon': { opacity: 1 },
  },
  _active: {
    bg: 'rgba(var(--accent-rgb), 0.18)',
  },
  _disabled: {
    opacity: 0.55,
    cursor: 'not-allowed',
    _hover: {
      bg: 'transparent',
      borderColor: 'transparent',
      color: 'gray.100',
      '& .chakra-button__icon': { opacity: 0 },
    },
  },
}

export const canvasPopoverPrimaryActionStyles: SystemStyleObject = {
  bg: 'rgba(var(--accent-rgb), 0.08)',
  border: '1px solid',
  borderColor: 'rgba(var(--accent-rgb), 0.24)',
  borderRadius: 'lg',
  color: 'var(--accent)',
  fontWeight: '600',
  fontSize: '11px',
  _hover: {
    bg: 'rgba(var(--accent-rgb), 0.14)',
    borderColor: 'rgba(var(--accent-rgb), 0.36)',
  },
  _active: {
    bg: 'rgba(var(--accent-rgb), 0.18)',
  },
}

export const canvasPopoverSecondaryActionStyles: SystemStyleObject = {
  bg: 'whiteAlpha.50',
  border: '1px solid',
  borderColor: 'whiteAlpha.100',
  borderRadius: 'lg',
  color: 'gray.200',
  fontWeight: '600',
  fontSize: '11px',
  _hover: {
    bg: 'whiteAlpha.100',
    borderColor: 'whiteAlpha.200',
    color: 'white',
  },
  _active: {
    bg: 'whiteAlpha.150',
  },
}

