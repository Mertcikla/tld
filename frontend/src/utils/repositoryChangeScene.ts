import type { RepositoryImpactScene } from '../api/client'
import type { ExploreData } from '../types'
import type { ZUIChangeOverlay } from '../components/ZUI/types'
import { CHANGE_RING_COLORS } from '../components/ZUI/changeOverlay'
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
// This adapter only supplies renderer colors and annotations.
export function repositoryChangeScene(scene: RepositoryImpactScene) {
  const overlays: Record<number, ZUIChangeOverlay> = {}
  const data: ExploreData = { tree: scene.tree, views: {}, navigations: scene.navigations }
  for (const [viewId, content] of Object.entries(scene.views)) {
    data.views[viewId] = { ...content, placements: content.placements.map((element) => {
      const key = `placement|${viewId}|${element.element_id}`
      const origin = scene.placements?.[key]?.origin
      const provenanceTag = origin === 'raw' ? 'augmented' : origin === 'mapped' ? 'generated' : 'authored'
      const tags = [...(element.tags ?? []).filter((tag) => tag !== 'map-generated'), provenanceTag]
      const overlay = scene.overlays[element.element_id]
      if (overlay) {
        overlays[element.element_id] = { change: overlay.change, path: overlay.path, linesAdded: overlay.linesAdded, linesRemoved: overlay.linesRemoved, symbols: overlay.symbols, reason: overlay.reason }
        if (overlay.change !== 'modified') tags.push(overlay.change)
      }
      return { ...element, tags }
    }) }
  }
  return { data, overlays }
}
