export const subElementControlSurface = {
  bg: 'var(--bg-source-preview, var(--bg-panel, #20252d))',
  border: '1px solid',
  borderColor: 'var(--border-main)',
  borderRadius: 'lg',
}

export const subElementControlButton = {
  variant: 'ghost', minW: '20px', w: '20px', h: '20px', borderRadius: '3px', color: 'gray.400',
  _hover: { color: 'var(--accent)', bg: 'color-mix(in srgb, var(--accent) 14%, transparent)' },
  sx: { '&[aria-pressed="true"]': { color: 'var(--accent)', bg: 'color-mix(in srgb, var(--accent) 20%, transparent)' } },
}
