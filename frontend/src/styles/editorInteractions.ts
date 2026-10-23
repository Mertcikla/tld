import { keyframes } from '@emotion/react'
import type { SystemStyleObject } from '@chakra-ui/react'

const settle = keyframes({
  from: { opacity: 0, transform: 'translateY(3px) scale(0.92)' },
  to: { opacity: 1, transform: 'translateY(0) scale(1)' },
})

const appear = keyframes({ from: { opacity: 0 }, to: { opacity: 1 } })

export const editorRevealStyles: SystemStyleObject = {
  animation: `${appear} 160ms ease both`,
  '@media (prefers-reduced-motion: reduce)': { animation: 'none' },
}

export const editorFeedbackStyles: SystemStyleObject = {
  animation: `${settle} 180ms cubic-bezier(0.2, 0.8, 0.2, 1) both`,
  '@media (prefers-reduced-motion: reduce)': { animation: 'none' },
}

// Apply to controls, never their positioned toolbar: the canvas uses transforms
// to center toolbars and map diagram coordinates.
export const editorControlStyles: SystemStyleObject = {
  transition: 'transform 160ms cubic-bezier(0.2, 0.8, 0.2, 1), background-color 160ms ease, color 160ms ease, box-shadow 160ms ease, opacity 160ms ease',
  '&:active:not(:disabled):not([aria-disabled="true"])': { transform: 'scale(0.94)' },
  _focusVisible: { outline: '2px solid var(--accent)', outlineOffset: '2px' },
  '&[aria-pressed="true"] > svg': editorFeedbackStyles,
  '&[aria-pressed="true"] > .chakra-stack > svg:first-of-type': editorFeedbackStyles,
  '@media (prefers-reduced-motion: reduce)': {
    transition: 'none',
    '&:active:not(:disabled):not([aria-disabled="true"])': { transform: 'none' },
  },
}

export const editorToolbarStyles: SystemStyleObject = {
  '& button': editorControlStyles,
  '& .chakra-switch__track': {
    transition: 'background-color 180ms ease',
    '@media (prefers-reduced-motion: reduce)': { transition: 'none' },
  },
  '& .chakra-switch__thumb': {
    transition: 'transform 180ms cubic-bezier(0.2, 0.8, 0.2, 1)',
    '@media (prefers-reduced-motion: reduce)': { transition: 'none' },
  },
  '& [role="slider"]': {
    transition: 'box-shadow 160ms ease',
    '&:hover:not([aria-disabled="true"])': { boxShadow: '0 0 0 4px rgba(var(--accent-rgb), 0.16)' },
    _focusVisible: { boxShadow: '0 0 0 4px rgba(var(--accent-rgb), 0.24)' },
    '@media (prefers-reduced-motion: reduce)': { transition: 'none' },
  },
}
