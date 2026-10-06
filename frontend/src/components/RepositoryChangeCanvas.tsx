import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Box, Flex, Text } from '@chakra-ui/react'
import { api, type RepositoryImpact as ImpactDiagram, type RepositoryImpactScene } from '../api/client'
import { ZUICanvas, type ZUICanvasHandle } from './ZUI'
import { repositoryChangeScene, REPOSITORY_CHANGE_TAG } from '../utils/repositoryChangeScene'
import { fitBlastRadius, MAX_BLAST_RADIUS } from '../utils/impactScope'
import RepositoryChangeMenu from './RepositoryChangeMenu'
import RepositoryChangeMermaid from './RepositoryChangeMermaid'

const colors = { added: '#48bb78', removed: '#fc8181', modified: '#ecc94b', unchanged: '#718096' }
const crossBranchSettings = { enabled: false, depth: 1, connectorBudget: 50, connectorPriority: 'external' as const }
export default function RepositoryChangeCanvas({ diagram, selectedPath, emptyMessage = 'Select Base and Head, then compare to overlay changes on the map.', busy = false }: {
  diagram: ImpactDiagram | null; selectedPath: string; emptyMessage?: string
  busy?: boolean
}) {
  const canvas = useRef<ZUICanvasHandle>(null)
  const [scene, setScene] = useState<RepositoryImpactScene | null>(null)
  const [error, setError] = useState('')
  const [mermaidOpen, setMermaidOpen] = useState(true)
  const [displayMode, setDisplayMode] = useState<'standard' | 'plain'>('standard')
  const [radius, setRadius] = useState(0)
  const [limited, setLimited] = useState(false)
  const hasDiagram = diagram != null
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
    setRadius(budgetRadius)
    setLimited(hasDiagram && budgetRadius < maxRadius)
  }, [comparisonKey, version, hasDiagram, budgetRadius, maxRadius])
  const view = useMemo(() => scene ? repositoryChangeScene(scene, { radius, plain: displayMode === 'plain' }) : null, [scene, radius, displayMode])
  const warning = diagram != null && (limited || radius > budgetRadius)
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
            Showing blast radius {radius} of {maxRadius} because the diagram is large. Pick a larger radius to include more context.
          </Text>
        )}
        {!diagram.nodes.length && <Text p={3} fontSize="sm" color="gray.400">No source changes in this comparison.</Text>}
        <Flex flex={1} minH={0} direction={{ base: 'column', lg: 'row' }}>
          <Box position="relative" minW={0} flex={1} minH={{ base: '420px', xl: '560px' }} data-testid="repository-change-canvas">
            {view ? <ZUICanvas ref={canvas} data={view.data} changeOverlays={view.overlays} preserveCameraOnUpdate highlightedTags={diagram.nodes.length ? [REPOSITORY_CHANGE_TAG] : []} highlightColor={colors.modified} crossBranchSettings={crossBranchSettings} onReady={focusSelected} /> : !error && <Text p={6} fontSize="sm" color="gray.400">Loading change scene…</Text>}
            <RepositoryChangeMenu
              radius={radius}
              maxRadius={maxRadius}
              onRadiusChange={setRadius}
              busy={busy}
              viewMode={displayMode}
              onViewModeChange={setDisplayMode}
              mermaidOpen={mermaidOpen}
              onToggleMermaid={() => setMermaidOpen((open) => !open)}
              hasMermaid
            />
          </Box>
          {mermaidOpen && (
            <Box
              w={{ base: 'full', lg: '380px' }}
              h={{ base: '360px', lg: 'auto' }}
              minH={0}
              flexShrink={0}
              borderTop={{ base: '1px solid', lg: 'none' }}
              borderColor="whiteAlpha.100"
            >
              <RepositoryChangeMermaid
                repositoryId={diagram.repositoryId}
                comparisonKey={diagram.comparisonKey}
                radius={radius}
                open={mermaidOpen}
              />
            </Box>
          )}
        </Flex>
      </Flex>}
    </Flex>
  )
}
