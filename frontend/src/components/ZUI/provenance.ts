import type { ZUINodeProvenance } from './types'

export type { ZUINodeProvenance }

// Provenance no longer draws its own ring: each kind is a plain tag whose dot
// and pill reuse the colors below.
export const PROVENANCE_META: Record<ZUINodeProvenance, { label: string; blurb: string; color: string; glyph: string; dash: number[] }> = {
  authored: { label: 'Authored', blurb: 'drawn in the workspace', color: '#94a3b8', glyph: '●', dash: [] },
  augmented: { label: 'Augmented', blurb: 'from graph facts · not in the workspace', color: '#a78bfa', glyph: '◇', dash: [7, 4] },
  generated: { label: 'Generated', blurb: 'materialized by the code map', color: '#2dd4bf', glyph: '▦', dash: [2, 4] },
}
