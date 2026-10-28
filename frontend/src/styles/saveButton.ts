import type { SystemStyleObject } from '@chakra-ui/react'

/** Shared by the Markdown and flow editors, including their disabled state. */
export const saveButtonStyles: SystemStyleObject = {
  height: '32px',
  minWidth: '32px',
  width: '32px',
  paddingInline: '0',
  border: '1px solid var(--chakra-colors-whiteAlpha-100)',
  borderRadius: 'var(--chakra-radii-md)',
  fontWeight: 600,
  background: 'var(--chakra-colors-whiteAlpha-200)',
  color: 'var(--baseTextContrast, var(--chakra-colors-whiteAlpha-900))',
  boxShadow: 'var(--chakra-shadows-panel-sm)',
  transition: 'all 0.2s var(--chakra-transitions-easing-pop)',
  '& svg': { width: '15px', height: '15px' },
  _hover: {
    background: 'var(--chakra-colors-whiteAlpha-300)',
    color: 'var(--baseTextContrast, var(--chakra-colors-whiteAlpha-900))',
    transform: 'translateY(-1px)',
    boxShadow: 'var(--chakra-shadows-panel-hover)',
  },
  _disabled: {
    opacity: 0.38,
    transform: 'none',
    boxShadow: 'var(--chakra-shadows-panel-sm)',
  },
  _focusVisible: { outline: '2px solid var(--accent)', outlineOffset: '2px' },
}
