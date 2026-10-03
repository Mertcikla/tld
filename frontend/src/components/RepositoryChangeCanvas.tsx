import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Box, Flex, Text } from '@chakra-ui/react'
import { api, type RepositoryImpact as ImpactDiagram } from '../api/client'
import { ZUICanvas, type ZUICanvasHandle } from './ZUI'
import type { ExploreData } from '../types'
import { repositoryChangeOverlay, REPOSITORY_CHANGE_TAG } from '../utils/repositoryChangeOverlay'

const colors = { added: '#48bb78', removed: '#fc8181', modified: '#ecc94b', unchanged: '#718096' }
const crossBranchSettings = { enabled: false, depth: 1, connectorBudget: 50, connectorPriority: 'external' as const }
export default function RepositoryChangeCanvas({ diagram, selectedPath, repositoryRoot }: {
  diagram: ImpactDiagram | null; selectedPath: string; repositoryRoot: string
}) {
  const canvas = useRef<ZUICanvasHandle>(null)
  const [workspace, setWorkspace] = useState<ExploreData | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true
    setError('')
    void api.explore.load().then((result) => { if (active) setWorkspace(result) }).catch((err: unknown) => { if (active) setError(err instanceof Error ? err.message : 'Could not load the workspace map') })
    return () => { active = false }
  }, [repositoryRoot, diagram?.version])
  const scene = useMemo(() => workspace && diagram ? repositoryChangeOverlay(workspace, diagram, repositoryRoot) : null, [workspace, diagram, repositoryRoot])
  const focusSelected = useCallback(() => {
    if (!scene || !selectedPath) return
    for (const [viewId, view] of Object.entries(scene.data.views)) {
      const element = view.placements.find((item) => item.file_path === selectedPath && scene.overlays[item.element_id])
      if (element && canvas.current?.focusElement(Number(viewId), element.element_id)) return
    }
  }, [scene, selectedPath])
  useEffect(focusSelected, [focusSelected])
  return (
    <Flex direction="column" flex={1} minW={0} minH="400px" data-testid="repository-change-overlay">
      {!diagram ? <Text p={6} fontSize="sm" color="gray.400">Select Base and Head, then compare to overlay changes on the map.</Text> : <Flex flex={1} minH={0} direction="column">
        {error && <Text p={3} color="red.300">{error}</Text>}
        {!diagram.nodes.length && <Text p={3} fontSize="sm" color="gray.400">No source changes in this comparison.</Text>}
        <Box minW={0} flex={1} minH={{ base: '420px', xl: '560px' }} data-testid="repository-change-canvas">
          {scene ? <ZUICanvas ref={canvas} data={scene.data} changeOverlays={scene.overlays} preserveCameraOnUpdate highlightedTags={diagram.nodes.length ? [REPOSITORY_CHANGE_TAG] : []} highlightColor={colors.modified} crossBranchSettings={crossBranchSettings} onReady={focusSelected} /> : !error && <Text p={6} fontSize="sm" color="gray.400">Loading workspace map…</Text>}
        </Box>
      </Flex>}
    </Flex>
  )
}
