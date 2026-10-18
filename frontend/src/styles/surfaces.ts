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
