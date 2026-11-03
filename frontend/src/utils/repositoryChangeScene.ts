import type { RepositoryImpactScene } from '../api/client'
import type { ExploreData } from '../types'
import type { ZUIChangeOverlay } from '../components/ZUI/types'
import { CHANGE_RING_COLORS } from '../components/ZUI/changeOverlay'
import type { SubSource } from './subElementSource'
import { subElementKind } from './subElements'
import { PROVENANCE_META } from '../components/ZUI/provenance'

export const CHANGE_STATE_TAGS = ['added', 'removed', 'unchanged'] as const
export const PROVENANCE_TAGS = ['authored', 'augmented', 'generated'] as const

/** Seed colors for overlay tags; workspace tagColors take precedence. */
export const REPOSITORY_TAG_COLORS: Record<string, string> = {
  ...CHANGE_RING_COLORS,
  authored: PROVENANCE_META.authored.color,
  augmented: PROVENANCE_META.augmented.color,
  generated: PROVENANCE_META.generated.color,
}

// The backend selects membership and display mode before freezing the scene.
// This adapter supplies code-card presentation, spacing, and annotations.
export function repositoryChangeScene(scene: RepositoryImpactScene) {
  const codeSources: Record<number, SubSource> = {}
  const overlays: Record<number, ZUIChangeOverlay> = {}
  const data: ExploreData = { tree: scene.tree, views: {}, navigations: scene.navigations }
  for (const [viewId, content] of Object.entries(scene.views)) {
    data.views[viewId] = { ...content, placements: content.placements.map((element) => {
      const key = `placement|${viewId}|${element.element_id}`
      const origin = scene.placements?.[key]?.origin
      const provenanceTag = origin === 'raw' ? 'augmented' : origin === 'mapped' ? 'generated' : 'authored'
      const tags = [...(element.tags ?? []).filter((tag) => tag !== 'map-generated'), provenanceTag]
      const overlay = scene.overlays[element.element_id]
      // Historical scenes contain captured symbol changes, not worktree text.
      // Never substitute today's checkout for a historical or imported file.
      const symbols = (overlay?.symbolDetails ?? []).filter(symbol => symbol.anchor).map(symbol => ({
        name: symbol.name, startLine: symbol.anchor!.startLine + 1, endLine: symbol.anchor!.endLine + 1,
      }))
      codeSources[element.element_id] = { status: 'ready', lines: [], tokens: [], symbols, startLine: 1,
        endLine: symbols.reduce((end, symbol) => Math.max(end, symbol.endLine), 1) }
      if (overlay) {
        overlays[element.element_id] = { change: overlay.change, path: overlay.path, linesAdded: overlay.linesAdded, linesRemoved: overlay.linesRemoved, symbols: overlay.symbols, symbolDetails: overlay.symbolDetails, reason: overlay.reason }
        if (overlay.change !== 'modified') tags.push(overlay.change)
      }
      // File leaves use the same code cards as the editor. Containers retain
      // their hierarchy and authored appearance. This is a presentation copy.
      const code = !element.has_view && subElementKind(element) === 'code'
      return { ...element, kind: code ? 'code' : element.kind, tags }
    }) }
  }
  // The scene is laid out for 180×85 overview cards; reserve room for 520×360
  // code cards without changing any saved workspace or frozen scene positions.
  for (const content of Object.values(data.views)) {
    if (!content.placements.some(element => element.kind === 'code')) continue
    content.placements = content.placements.map(element => ({
      ...element, position_x: element.position_x * 3, position_y: element.position_y * 4.5,
    }))
  }
  return { data, overlays, codeSources }
}
