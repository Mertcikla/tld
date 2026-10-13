import { computeLayout } from './ZUI/layout'
import { filterLayoutByTags, visiblePlacementKeys } from './ZUI/tagVisibility'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Box, Flex, Text } from '@chakra-ui/react'
import { useTagFilters } from '../hooks/useTagFilters'
import { api, type RepositoryImpact as ImpactDiagram, type RepositoryImpactScene } from '../api/client'
import type { Tag } from '../types'
import { isElementGroupTag } from '../utils/elementGroups'
import { isGroundingIgnoreTag } from '../utils/groundingTags'
import { ZUICanvas, type ZUICanvasHandle } from './ZUI'
import { REPOSITORY_TAG_COLORS, repositoryChangeScene } from '../utils/repositoryChangeScene'
import {
  PANEL_COLLAPSE_SNAP,
  PANEL_OVERLAY_SNAP,
  PANEL_RAIL_WIDTH,
  useResizableColumn,
} from '../hooks/useResizableColumn'
import ColumnResizeHandle from './ColumnResizeHandle'
import RepositoryChangeMenu from './RepositoryChangeMenu'
import RepositoryImpactInspector from './RepositoryImpactInspector'
import RepositoryChangeMermaid from './RepositoryChangeMermaid'

const SIDE_PANE_DEFAULT_WIDTH = 380
const crossBranchSettings = { enabled: true, depth: 2, connectorBudget: Number.MAX_SAFE_INTEGER, connectorPriority: 'external' as const }
const seedTagColors = (): Record<string, Tag> =>
  Object.fromEntries(
    Object.entries(REPOSITORY_TAG_COLORS).map(([name, color]) => [name, { name, color, description: null }]),
  )
export default function RepositoryChangeCanvas({ diagram, selectedPath, emptyMessage = 'Select Base and Head, then compare to overlay changes on the map.', busy = false, embedded }: {
  diagram: ImpactDiagram | null; selectedPath: string; emptyMessage?: string
  busy?: boolean
  /** A frozen imported scene; disables all API reads and backend selection. */
  embedded?: { scene: RepositoryImpactScene; markdown: string }
}) {
  const canvas = useRef<ZUICanvasHandle>(null)
  const splitRef = useRef<HTMLDivElement | null>(null)
  const [loadedScene, setScene] = useState<RepositoryImpactScene | null>(null)
  const scene = embedded?.scene ?? loadedScene
  const [error, setError] = useState('')
  const [selectedKey, setSelectedKey] = useState('')
  const [inspectorEnabled] = useState(() => !embedded && new URLSearchParams(typeof window !== 'undefined' ? window.location?.search ?? '' : '').get('debugImpact') === '1')
  const [debug, setDebug] = useState(inspectorEnabled)
  const [displayMode, setDisplayMode] = useState<'standard' | 'plain'>('standard')
  const [tagColors, setTagColors] = useState<Record<string, Tag>>(seedTagColors)
  const [highlightedTags, setHighlightedTags] = useState<string[]>([])
  const { hiddenTags, selectedTags, toggleTagSelection, toggleTagVisibility, resetFilters } = useTagFilters()
  const sidePane = useResizableColumn({
    storageKey: inspectorEnabled ? 'tld:repositories:impactInspectorWidth' : 'tld:repositories:mermaidPaneWidth',
    defaultWidth: SIDE_PANE_DEFAULT_WIDTH,
    collapseBelow: PANEL_COLLAPSE_SNAP,
    overlayAbove: PANEL_OVERLAY_SNAP,
    side: 'end',
    maxWidth: (containerWidth) => containerWidth,
    containerRef: splitRef,
  })
  const repositoryId = diagram?.repositoryId ?? ''
  const comparisonKey = diagram?.comparisonKey ?? ''
  const version = diagram?.version ?? ''
  useEffect(() => {
    let active = true
    const controller = new AbortController()
    setError('')
    setScene(null)
    if (embedded || !repositoryId || !comparisonKey) return undefined
    void api.repositories.impactScene(repositoryId, comparisonKey, controller.signal, { debugProvenance: debug, plain: displayMode === 'plain' })
      .then((result) => { if (active) setScene(result) })
      .catch((err: unknown) => { if (active && !controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not load the change scene') })
    return () => { active = false; controller.abort() }
  }, [repositoryId, comparisonKey, version, debug, displayMode, embedded])
  const view = useMemo(() => scene ? repositoryChangeScene(scene) : null, [scene])
  useEffect(() => {
    if (!diagram || embedded) return
    let cancelled = false
    try {
      const pending = api.workspace?.orgs?.tagColors?.list?.()
      if (pending && typeof (pending as Promise<Record<string, Tag>>).then === 'function') {
        void (pending as Promise<Record<string, Tag>>)
          .then((colors) => { if (!cancelled) setTagColors({ ...seedTagColors(), ...colors }) })
          .catch(() => { if (!cancelled) setTagColors(seedTagColors()) })
      }
    } catch {
      if (!cancelled) setTagColors({})
    }
    return () => { cancelled = true }
  }, [diagram, embedded])
  const tagMetrics = useMemo(() => {
    const tagSet = new Set<string>()
    const tagCounts: Record<string, number> = {}
    if (view) {
      for (const data of Object.values(view.data.views)) {
        for (const placement of data?.placements ?? []) {
          for (const tag of placement.tags ?? []) {
            if (isElementGroupTag(tag) || isGroundingIgnoreTag(tag)) continue
            tagSet.add(tag)
            tagCounts[tag] = (tagCounts[tag] ?? 0) + 1
          }
        }
      }
    }
    return { allTags: Array.from(tagSet).sort(), tagCounts }
  }, [view])
  const handleHoverTag = useCallback((tag: string | null) => {
    setHighlightedTags(tag ? [tag] : [])
  }, [])
  const selectPrimitive = useCallback((key: string) => {
    setSelectedKey(key)
    const entry = scene?.placements?.[key]
    if (!entry) return
    canvas.current?.focusElement(entry.viewId, entry.resourceId)
  }, [scene])
  const focusSelected = useCallback(() => {
    if (!view || !selectedPath) return
    const key = scene?.coverage?.find((entry) => entry.path === selectedPath)?.placementKeys[0]
    if (key) { selectPrimitive(key); return }
    for (const [viewId, data] of Object.entries(view.data.views)) {
      const element = data.placements.find((item) => item.file_path === selectedPath && view.overlays[item.element_id])
      if (element && canvas.current?.focusElement(Number(viewId), element.element_id)) { setSelectedKey(`placement|${viewId}|${element.element_id}`); return }
    }
  }, [view, selectedPath, scene, selectPrimitive])
  useEffect(focusSelected, [focusSelected])
  const coverageLayout = useMemo(() => view && inspectorEnabled ? computeLayout(view.data) : null, [view, inspectorEnabled])
  const hiddenChanges = useMemo(() => {
    if (!coverageLayout) return 0
    const visible = visiblePlacementKeys(filterLayoutByTags(coverageLayout, selectedTags, hiddenTags))
    return (scene?.coverage ?? []).filter((entry) => entry.placementKeys.every((key) => {
      const placement = scene?.placements?.[key]
      return !placement || !visible.has(`${placement.viewId}:${placement.resourceId}`)
    })).length
  }, [scene, coverageLayout, selectedTags, hiddenTags])

  return (
    <Flex direction="column" flex={1} minW={0} minH="240px" data-testid="repository-change-overlay">
      {!diagram && !embedded ? <Text p={6} fontSize="sm" color="gray.400">{emptyMessage}</Text> : <Flex flex={1} minH={0} direction="column">
        {error && <Text p={3} color="red.300">{error}</Text>}
        {!(embedded ? embedded.scene.files?.length : diagram?.nodes.length) && <Text p={3} fontSize="sm" color="gray.400">No source changes in this comparison.</Text>}
        <Flex flex={1} minH={0} direction={{ base: 'column', lg: 'row' }} position="relative" ref={splitRef} data-testid="repository-change-split">
          <Box position="relative" minW={0} flex={1} minH={{ base: '420px', xl: '560px' }} data-testid="repository-change-canvas">
            {view ? <ZUICanvas ref={canvas} data={view.data} changeOverlays={view.overlays} preserveCameraOnUpdate highlightedTags={highlightedTags} hiddenTags={hiddenTags} selectedTags={selectedTags} tagColors={tagColors} crossBranchSettings={crossBranchSettings} hoverLocked={highlightedTags.length > 0} onReady={focusSelected} onSelectObject={inspectorEnabled ? (selection) => setSelectedKey(selection ? selection.kind === 'view' ? `view|${selection.viewId}` : `${selection.kind}|${selection.viewId}|${selection.resourceId}` : '') : undefined} /> : !error && <Text p={6} fontSize="sm" color="gray.400">Loading change scene…</Text>}
            <RepositoryChangeMenu
              showViewMode={!embedded}
              busy={busy}
              viewMode={displayMode}
              onViewModeChange={setDisplayMode}
              allTags={tagMetrics.allTags}
              tagCounts={tagMetrics.tagCounts}
              tagColors={tagColors}
              hiddenTags={hiddenTags}
              selectedTags={selectedTags}
              onToggleTagSelection={toggleTagSelection}
              onResetTagFilters={resetFilters}
              onHoverTag={handleHoverTag}
              onToggleTagVisibility={toggleTagVisibility}
            />
          </Box>
          {!sidePane.isCollapsed && <ColumnResizeHandle
          label={inspectorEnabled ? 'Resize impact inspector' : 'Resize change diagram'}
          dataTestId={inspectorEnabled ? 'repository-impact-inspector-resize' : 'repository-change-mermaid-resize'}
          display={{ base: 'none', lg: 'block' }}
          zIndex={sidePane.isOverlay ? 40 : 0}
          overlayWidth={sidePane.isOverlay ? sidePane.renderedWidth : undefined}
          isActive={sidePane.isResizing}
          onPointerDown={sidePane.startResize}
        />}
        <Box
          data-testid={inspectorEnabled ? 'repository-impact-inspector-slot' : 'repository-change-mermaid-slot'}
          w={{ base: 'full', lg: `${sidePane.renderedWidth}px` }}
          h={{ base: sidePane.isCollapsed ? `${PANEL_RAIL_WIDTH}px` : '360px', lg: 'auto' }}
          minH={0}
          flexShrink={0}
          position={sidePane.isOverlay ? { base: 'relative', lg: 'absolute' } : 'relative'}
          top={{ lg: 0 }}
          right={{ lg: 0 }}
          bottom={{ lg: 0 }}
          zIndex={sidePane.isOverlay ? 30 : undefined}
          borderTop={{ base: '1px solid', lg: 'none' }}
          borderColor="whiteAlpha.100"
        >
          {inspectorEnabled ? <RepositoryImpactInspector
            scene={scene}
            selectedKey={selectedKey}
            onSelect={selectPrimitive}
            debug={debug}
            onDebugChange={setDebug}
            plain={displayMode === 'plain'}
            hiddenChanges={hiddenChanges}
            collapsed={sidePane.isCollapsed}
            overlay={sidePane.isOverlay}
            onExpand={sidePane.expand}
            onDock={sidePane.dock}
          /> : <RepositoryChangeMermaid
            embeddedMarkdown={embedded?.markdown}
            repositoryId={repositoryId}
            comparisonKey={comparisonKey}
            plain={displayMode === 'plain'}
            open
            collapsed={sidePane.isCollapsed}
            overlay={sidePane.isOverlay}
            onExpand={sidePane.expand}
            onDock={sidePane.collapse}
          />}
        </Box>
        </Flex>
      </Flex>}
    </Flex>
  )
}
