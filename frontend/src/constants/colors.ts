// ── Accent color ────────────────────────────────────────────────────────────
// The interactive brand color. Drives edges, selection highlights, drawing
// stroke default, and focus indicators. Users can customize via Appearance
// settings; the chosen value is stored in localStorage and exposed as the
// CSS variable --accent on :root.

export const ACCENT_DEFAULT = '#63b3ed'

export const ACCENT_OPTIONS: { name: string; value: string }[] = [
  { name: 'Blue', value: '#63b3ed' },
  { name: 'Teal', value: '#4fd1c5' },
  { name: 'Purple', value: '#b794f4' },
  { name: 'Green', value: '#68d391' },
  { name: 'Orange', value: '#f6ad55' },
  { name: 'Pink', value: '#f687b3' },
  { name: 'White', value: '#ffffff' },
  { name: 'Black', value: '#000000' },
  { name: 'Red', value: '#f56565' },
  { name: 'Yellow', value: '#ecc94b' },
]

// ── Background color ─────────────────────────────────────────────────────────
export const BACKGROUND_DEFAULT = '#0d121e'

export const BACKGROUND_OPTIONS: { name: string; value: string }[] = [
  { name: 'Midnight', value: '#0d121e' }, // Original dark blue/purplish
  { name: 'Deep Sea', value: '#0a192f' },
  { name: 'Obsidian', value: '#0f172a' },
  { name: 'Coal', value: '#111111' },
  { name: 'Space', value: '#0b0e14' },
  { name: 'Asphalt', value: '#1a202c' },
  { name: 'Dark', value: '#1e1e1e' },
  { name: 'Ebony', value: '#121212' },
  { name: 'Pitch', value: '#000000' },
  { name: 'Charcoal', value: '#242424' },
]

// ── Element color ────────────────────────────────────────────────────────────
export const ELEMENT_DEFAULT = '#2d3748'

export const ELEMENT_OPTIONS: { name: string; value: string }[] = [
  { name: 'Slate', value: '#2d3748' },
  { name: 'Navy', value: '#1a365d' },
  { name: 'Deep Purple', value: '#322659' },
  { name: 'Dark Grey', value: '#171923' },
  { name: 'Steel', value: '#4a5568' },
  { name: 'Cobalt', value: '#2c5282' },
  { name: 'Forest', value: '#22543d' },
  { name: 'Rust', value: '#742a2a' },
  { name: 'Bronze', value: '#744210' },
  { name: 'Amethyst', value: '#553c9a' },
]

// ── View hierarchy ───────────────────────────────────────────────────────────
// Blue = parent / zoom-out direction (equivalent to Chakra blue.400)
// Teal = child  / zoom-in  direction (equivalent to Chakra teal.400)

export const PARENT_VIEW_COLOR = '#63b3ed'
export const CHILD_VIEW_COLOR = '#4fd1c5'

export const PARENT_VIEW_BG = 'rgba(99,179,237,0.12)'
export const PARENT_VIEW_BORDER = 'rgba(99,179,237,0.25)'
export const CHILD_VIEW_BG = 'rgba(79,209,197,0.12)'
export const CHILD_VIEW_BORDER = 'rgba(79,209,197,0.25)'

// ── Drawing toolbar palette ───────────────────────────────────────────────────
export const DRAWING_COLORS = ['#63b3ed', '#f56565', '#48bb78', '#ecc94b', '#ed64a6', '#ffffff']

// ── Utility ───────────────────────────────────────────────────────────────────
/**
 * Convert a 6-char hex color to `rgba(r, g, b, alpha)`.
 * Used to compute dynamic shadows from the accent value.
 */
export function hexToRgba(hex: string, alpha: number): string {
  const h = hex.replace('#', '')
  const r = parseInt(h.substring(0, 2), 16)
  const g = parseInt(h.substring(2, 4), 16)
  const b = parseInt(h.substring(4, 6), 16)
  return `rgba(${r},${g},${b},${alpha})`
}
