import { useCallback, useEffect, useMemo, useState, type Dispatch, type SetStateAction } from 'react'
import { api } from '../../api/client'
import { useTagFilters } from '../../hooks/useTagFilters'
import type { ExploreData, Tag, ViewLayer } from '../../types'
import { elementGroupTagForLayer, isElementGroupLayer, isElementGroupTag } from '../../utils/elementGroups'
import { countCanvasTags, type CanvasTagCounts } from '../../utils/canvasTags'

export interface ExploreTagsState extends CanvasTagCounts {
  allTags: string[]
  tagColors: Record<string, Tag>
  layers: ViewLayer[]
  layerElementCounts: Record<number, number>
  layerConnectorCounts: Record<number, number>
  highlightedTags: string[]
  setHighlightedTags: Dispatch<SetStateAction<string[]>>
  highlightColor: string
  setHighlightColor: Dispatch<SetStateAction<string>>
  hiddenTags: string[]
  selectedTags: string[]
  toggleTagSelection: (tag: string) => void
  resetFilters: () => void
  toggleLayerVisibility: (layer: ViewLayer) => void
  toggleTagVisibility: (tag: string) => void
}

export function deriveExploreTagMetrics(data: ExploreData | null, layers: ViewLayer[]): CanvasTagCounts & {
  allTags: string[]
  layerElementCounts: Record<number, number>
  layerConnectorCounts: Record<number, number>
} {
  if (!data || !data.views) {
    return { allTags: [], ...countCanvasTags([], []), layerElementCounts: {}, layerConnectorCounts: {} }
  }

  const metrics = countCanvasTags(
    Object.values(data.views).flatMap((view) => view?.placements ?? []),
    Object.values(data.views).flatMap((view) => view?.connectors ?? []),
  )

  const layerElementCounts: Record<number, number> = {}
  const layerConnectorCounts: Record<number, number> = {}
  for (const layer of layers) {
    let count = 0
    const viewsToCount = isElementGroupLayer(layer)
      ? [data.views[String(layer.diagram_id)]].filter((view) => view !== undefined)
      : Object.values(data.views)
    const groupTag = elementGroupTagForLayer(layer)
    viewsToCount.forEach((view) => {
      (view?.placements ?? []).forEach((placement) => {
        if ((placement.tags ?? []).some((tag) => groupTag ? tag === groupTag : layer.tags.includes(tag))) count++
      })
    })
    layerElementCounts[layer.id] = count
    layerConnectorCounts[layer.id] = groupTag ? 0 : viewsToCount.reduce((sum, view) => sum +
      (view?.connectors ?? []).filter((edge) => (edge.tags ?? []).some((tag) => layer.tags.includes(tag))).length, 0)
  }

  return {
    allTags: Object.keys(metrics.tagCounts).sort(),
    ...metrics,
    layerElementCounts,
    layerConnectorCounts,
  }
}

export function useExploreTags(data: ExploreData | null, sharedToken?: string): ExploreTagsState {
  const [tagColors, setTagColors] = useState<Record<string, Tag>>({})
  const [layers, setLayers] = useState<ViewLayer[]>([])
  const [highlightedTags, setHighlightedTags] = useState<string[]>([])
  const [highlightColor, setHighlightColor] = useState('')
  const filters = useTagFilters()
  const { toggleTagsVisibility } = filters

  useEffect(() => {
    if (!data || sharedToken) return
    let cancelled = false
    const tree = data.tree ?? []
    const rootIds = new Set(tree.filter((node) => !node.parent_view_id).map((node) => node.id))
    // The explore layout nests child diagrams inside their owner nodes, so only
    // root diagrams appear in `tree`. A group layer can live in any view, so
    // discover the views that actually contain a group tag from the full content
    // map rather than from the tree alone, then union in the root views.
    const viewIdSet = new Set<number>(rootIds)
    for (const [key, view] of Object.entries(data.views ?? {})) {
      const id = Number(key)
      if (Number.isNaN(id)) continue
      if ((view?.placements ?? []).some((placement) => (placement.tags ?? []).some(isElementGroupTag))) {
        viewIdSet.add(id)
      }
    }
    const viewIds = Array.from(viewIdSet)
    const fetchTagData = async () => {
      try {
        const [diagramLayers, colors] = await Promise.all([
          Promise.all(viewIds.map((id) => api.workspace.views.layers.list(id).catch(() => []))),
          api.workspace.orgs.tagColors.list().catch(() => ({} as Record<string, Tag>)),
        ])
        if (!cancelled) {
          // Explore lays out nested diagrams too, so fetch their groups as well as root view layers.
          const seen = new Set<number>()
          const unique = diagramLayers.flat().filter((layer) => {
            if (seen.has(layer.id)) return false
            seen.add(layer.id)
            return rootIds.has(layer.diagram_id) || isElementGroupLayer(layer)
          })
          setLayers(unique)
          setTagColors(colors)
        }
      } catch {
        // Public shared pages do not expose layer metadata.
      }
    }
    void fetchTagData()
    return () => { cancelled = true }
  }, [data, sharedToken])

  const metrics = useMemo(() => deriveExploreTagMetrics(data, layers), [data, layers])

  const toggleLayerVisibility = useCallback((layer: ViewLayer) => {
    toggleTagsVisibility(layer.tags)
  }, [toggleTagsVisibility])

  return {
    ...metrics,
    tagColors,
    layers,
    highlightedTags,
    setHighlightedTags,
    highlightColor,
    setHighlightColor,
    ...filters,
    toggleLayerVisibility,
  }
}
