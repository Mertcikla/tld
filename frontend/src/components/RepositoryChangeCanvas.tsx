import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Box, Flex, Text } from '@chakra-ui/react'
import { api, type RepositoryImpact as ImpactDiagram, type RepositoryImpactScene } from '../api/client'
import { ZUICanvas, type ZUICanvasHandle } from './ZUI'
import { repositoryChangeScene, REPOSITORY_CHANGE_TAG } from '../utils/repositoryChangeScene'
import { fitBlastRadius, MAX_BLAST_RADIUS } from '../utils/impactScope'
import {
  PANEL_COLLAPSE_SNAP,
  PANEL_OVERLAY_SNAP,
  useResizableColumn,
} from '../hooks/useResizableColumn'
import ColumnResizeHandle from './ColumnResizeHandle'
import RepositoryChangeMenu from './RepositoryChangeMenu'
import RepositoryChangeMermaid from './RepositoryChangeMermaid'

const MERMAID_PANE_DEFAULT_WIDTH = 380
const colors = { added: '#48bb78', removed: '#fc8181', modified: '#ecc94b', unchanged: '#718096' }
const crossBranchSettings = { enabled: false, depth: 1, connectorBudget: 50, connectorPriority: 'external' as const }
export default function RepositoryChangeCanvas({ diagram, selectedPath, emptyMessage = 'Select Base and Head, then compare to overlay changes on the map.', busy = false }: {
  diagram: ImpactDiagram | null; selectedPath: string; emptyMessage?: string
  busy?: boolean
}) {
  const canvas = useRef<ZUICanvasHandle>(null)
  const splitRef = useRef<HTMLDivElement | null>(null)
  const [scene, setScene] = useState<RepositoryImpactScene | null>(null)
  const [error, setError] = useState('')
  const [displayMode, setDisplayMode] = useState<'standard' | 'plain'>('standard')
  const [radius, setRadius] = useState(0)
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
  const maxRadius = diagram ? Math.max(0, Math.min(MAX_BLAST_RADIUS, diagram.maxRadius)) : 0
  const budgetRadius = diagram ? fitBlastRadius(diagram, maxRadius) : 0
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
  useEffect(() => {
    setRadius(0)
  }, [comparisonKey, version, maxRadius])
  const view = useMemo(() => scene ? repositoryChangeScene(scene, { radius, plain: displayMode === 'plain' }) : null, [scene, radius, displayMode])
  const warning = diagram != null && radius > budgetRadius
  const focusSelected = useCallback(() => {
    if (!view || !selectedPath) return
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
        {warning && (
          <Text px={3} py={1.5} fontSize="xs" color="orange.300" bg="orange.900" data-testid="repository-change-radius-warning">
            Blast radius {radius} exceeds the recommended size for this diagram. Pick a smaller radius to show less context.
          </Text>
        )}
        {!diagram.nodes.length && <Text p={3} fontSize="sm" color="gray.400">No source changes in this comparison.</Text>}
        <Flex flex={1} minH={0} direction={{ base: 'column', lg: 'row' }} position="relative" ref={splitRef} data-testid="repository-change-split">
          <Box position="relative" minW={0} flex={1} minH={{ base: '420px', xl: '560px' }} data-testid="repository-change-canvas">
            {view ? <ZUICanvas ref={canvas} data={view.data} changeOverlays={view.overlays} preserveCameraOnUpdate highlightedTags={diagram.nodes.length ? [REPOSITORY_CHANGE_TAG] : []} highlightColor={colors.modified} crossBranchSettings={crossBranchSettings} onReady={focusSelected} /> : !error && <Text p={6} fontSize="sm" color="gray.400">Loading change scene…</Text>}
            <RepositoryChangeMenu
              radius={radius}
              maxRadius={maxRadius}
              onRadiusChange={setRadius}
              busy={busy}
              viewMode={displayMode}
              onViewModeChange={setDisplayMode}
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
            radius={radius}
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
