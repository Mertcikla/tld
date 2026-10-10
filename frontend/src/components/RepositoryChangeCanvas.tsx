import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Box, Flex, Text } from '@chakra-ui/react'
import { api, type RepositoryImpact as ImpactDiagram, type RepositoryImpactScene } from '../api/client'
import type { Tag } from '../types'
import { isElementGroupTag } from '../utils/elementGroups'
import { isGroundingIgnoreTag } from '../utils/groundingTags'
import { ZUICanvas, type ZUICanvasHandle } from './ZUI'
import { REPOSITORY_TAG_COLORS, repositoryChangeScene } from '../utils/repositoryChangeScene'
import {
  PANEL_COLLAPSE_SNAP,
  PANEL_OVERLAY_SNAP,
  useResizableColumn,
} from '../hooks/useResizableColumn'
import ColumnResizeHandle from './ColumnResizeHandle'
import RepositoryChangeMenu from './RepositoryChangeMenu'
import RepositoryChangeMermaid from './RepositoryChangeMermaid'

const MERMAID_PANE_DEFAULT_WIDTH = 380
const crossBranchSettings = { enabled: false, depth: 1, connectorBudget: 50, connectorPriority: 'external' as const }
const seedTagColors = (): Record<string, Tag> =>
  Object.fromEntries(
    Object.entries(REPOSITORY_TAG_COLORS).map(([name, color]) => [name, { name, color, description: null }]),
  )
export default function RepositoryChangeCanvas({ diagram, selectedPath, emptyMessage = 'Select Base and Head, then compare to overlay changes on the map.', busy = false }: {
  diagram: ImpactDiagram | null; selectedPath: string; emptyMessage?: string
  busy?: boolean
}) {
  const canvas = useRef<ZUICanvasHandle>(null)
  const splitRef = useRef<HTMLDivElement | null>(null)
  const [scene, setScene] = useState<RepositoryImpactScene | null>(null)
  const [error, setError] = useState('')
  const [displayMode, setDisplayMode] = useState<'standard' | 'plain'>('standard')
  const [tagColors, setTagColors] = useState<Record<string, Tag>>(seedTagColors)
  const [highlightedTags, setHighlightedTags] = useState<string[]>([])
  const [hiddenTags, setHiddenTags] = useState<string[]>([])
  const mermaidPane = useResizableColumn({
    storageKey: 'tld:repositories:mermaidPaneWidth',
    defaultWidth: MERMAID_PANE_DEFAULT_WIDTH,
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
    setError('')
    setScene(null)
    if (!repositoryId || !comparisonKey) return undefined
    void api.repositories.impactScene(repositoryId, comparisonKey)
      .then((result) => { if (active) setScene(result) })
      .catch((err: unknown) => { if (active) setError(err instanceof Error ? err.message : 'Could not load the change scene') })
    return () => { active = false }
  }, [repositoryId, comparisonKey, version])
  const view = useMemo(() => scene ? repositoryChangeScene(scene, { plain: displayMode === 'plain' }) : null, [scene, displayMode])
  useEffect(() => {
    if (!diagram) return
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
  }, [diagram])
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
  const handleToggleTagVisibility = useCallback((tag: string) => {
    setHiddenTags((prev) => prev.includes(tag) ? prev.filter((item) => item !== tag) : [...prev, tag])
  }, [])
  const focusSelected = useCallback(() => {    if (!view || !selectedPath) return
    for (const [viewId, data] of Object.entries(view.data.views)) {
      const element = data.placements.find((item) => item.file_path === selectedPath && view.overlays[item.element_id])
      if (element && canvas.current?.focusElement(Number(viewId), element.element_id)) return
    }
  }, [view, selectedPath])
  useEffect(focusSelected, [focusSelected])
  return (
    <Flex direction="column" flex={1} minW={0} minH="240px" data-testid="repository-change-overlay">
      {!diagram ? <Text p={6} fontSize="sm" color="gray.400">{emptyMessage}</Text> : <Flex flex={1} minH={0} direction="column">
        {error && <Text p={3} color="red.300">{error}</Text>}
        {!diagram.nodes.length && <Text p={3} fontSize="sm" color="gray.400">No source changes in this comparison.</Text>}
        <Flex flex={1} minH={0} direction={{ base: 'column', lg: 'row' }} position="relative" ref={splitRef} data-testid="repository-change-split">
          <Box position="relative" minW={0} flex={1} minH={{ base: '420px', xl: '560px' }} data-testid="repository-change-canvas">
            {view ? <ZUICanvas ref={canvas} data={view.data} changeOverlays={view.overlays} preserveCameraOnUpdate highlightedTags={highlightedTags} hiddenTags={hiddenTags} tagColors={tagColors} crossBranchSettings={crossBranchSettings} hoverLocked={highlightedTags.length > 0} onReady={focusSelected} /> : !error && <Text p={6} fontSize="sm" color="gray.400">Loading change scene…</Text>}
            <RepositoryChangeMenu
              busy={busy}
              viewMode={displayMode}
              onViewModeChange={setDisplayMode}
              allTags={tagMetrics.allTags}
              tagCounts={tagMetrics.tagCounts}
              tagColors={tagColors}
              hiddenTags={hiddenTags}
              onHoverTag={handleHoverTag}
              onToggleTagVisibility={handleToggleTagVisibility}
            />
          </Box>
          {/* The change diagram is always docked beside the canvas; shrinking it
              below the collapse threshold is what hides its contents. */}
          <ColumnResizeHandle
          label="Resize change diagram"
          dataTestId="repository-change-mermaid-resize"
          display={{ base: 'none', lg: 'block' }}
          zIndex={mermaidPane.isOverlay ? 40 : 0}
          isActive={mermaidPane.isResizing}
          onPointerDown={mermaidPane.startResize}
        />
        <Box
          data-testid="repository-change-mermaid-slot"
          w={{ base: 'full', lg: `${mermaidPane.renderedWidth}px` }}
          h={{ base: '360px', lg: 'auto' }}
          minH={0}
          flexShrink={0}
          position={mermaidPane.isOverlay ? { base: 'relative', lg: 'absolute' } : 'relative'}
          top={{ lg: 0 }}
          right={{ lg: 0 }}
          bottom={{ lg: 0 }}
          zIndex={mermaidPane.isOverlay ? 30 : undefined}
          boxShadow={mermaidPane.isOverlay ? '-12px 0 32px rgba(0,0,0,0.55)' : undefined}
          borderTop={{ base: '1px solid', lg: 'none' }}
          borderColor="whiteAlpha.100"
        >
          <RepositoryChangeMermaid
            repositoryId={diagram.repositoryId}
            comparisonKey={diagram.comparisonKey}
            plain={displayMode === 'plain'}
            open
            collapsed={mermaidPane.isCollapsed}
            overlay={mermaidPane.isOverlay}
            onExpand={mermaidPane.expand}
            onDock={mermaidPane.dock}
          />
        </Box>
        </Flex>
      </Flex>}
    </Flex>
  )
}
