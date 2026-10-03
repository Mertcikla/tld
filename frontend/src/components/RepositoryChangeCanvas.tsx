import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Box, Button, Flex, HStack, Text } from '@chakra-ui/react'
import { api, type RepositoryImpact as ImpactDiagram } from '../api/client'
import { ZUICanvas, type ZUICanvasHandle } from './ZUI'
import type { ExploreData } from '../types'
import { repositoryChangeOverlay, REPOSITORY_CHANGE_TAG } from '../utils/repositoryChangeOverlay'

const colors = { added: '#48bb78', removed: '#fc8181', modified: '#ecc94b', unchanged: '#718096' }
const crossBranchSettings = { enabled: false, depth: 1, connectorBudget: 50, connectorPriority: 'external' as const }
export default function RepositoryChangeCanvas({ diagram, busy, selectedPath, onRadius, repositoryRoot }: {
  diagram: ImpactDiagram | null; busy: boolean; selectedPath: string
  onRadius: (radius: number) => void; repositoryRoot: string
}) {
  const [draft, setDraft] = useState<number | null>(null)
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
  const selected = diagram?.nodes.find((node) => node.path === selectedPath)
  const commitRadius = (value: number) => { setDraft(null); if (value !== diagram?.radius) onRadius(value) }
  return (
    <Flex direction="column" flex={1} minW={0} minH="400px" h="fit-content" data-testid="repository-change-overlay">
      {diagram && <Flex p={3} gap={3} align="center" wrap="wrap" borderBottom="1px solid" borderColor="whiteAlpha.100">
        <Box><Text as="label" htmlFor="impact-radius" fontSize="xs">Blast radius · {draft ?? diagram.radius} hops</Text><Box mt={1}>
          <input id="impact-radius" data-testid="impact-radius" aria-label="Blast radius" type="range" min={0} max={diagram.maxRadius} step={1} value={draft ?? diagram.radius} disabled={busy || !diagram.maxRadius} onChange={(event) => setDraft(Number(event.target.value))} onPointerUp={(event) => commitRadius(Number(event.currentTarget.value))} onKeyUp={(event) => commitRadius(Number(event.currentTarget.value))} onBlur={(event) => commitRadius(Number(event.currentTarget.value))} />
        </Box></Box>
        <HStack spacing={2} fontSize="xs" flexWrap="wrap">{Object.entries(colors).map(([kind, color]) => <Text key={kind} color={color}>{kind === 'unchanged' ? 'Context' : kind}</Text>)}</HStack>
        <Button ml="auto" size="xs" variant="outline" onClick={() => canvas.current?.fitView()}>Fit map</Button>
      </Flex>}
      {!diagram ? <Text p={6} fontSize="sm" color="gray.400">Select Base and Head, then compare to overlay changes on the map.</Text> : <Flex minH="360px" direction="column">
        {error && <Text p={3} color="red.300">{error}</Text>}
        {!diagram.nodes.length && <Text p={3} fontSize="sm" color="gray.400">No source changes in this comparison.</Text>}
        <Box minW={0} flexShrink={0} h={{ base: '420px', xl: '560px' }} data-testid="repository-change-canvas">
          {scene ? <ZUICanvas ref={canvas} data={scene.data} changeOverlays={scene.overlays} preserveCameraOnUpdate highlightedTags={diagram.nodes.length ? [REPOSITORY_CHANGE_TAG] : []} highlightColor={colors.modified} crossBranchSettings={crossBranchSettings} onReady={focusSelected} /> : !error && <Text p={6} fontSize="sm" color="gray.400">Loading workspace map…</Text>}
        </Box>
        {selected && <Box w="full" flexShrink={0} maxH="280px" overflowY="auto" p={3} borderTop="1px solid" borderColor="whiteAlpha.100" data-testid="impact-symbol-details">
          <Text fontWeight="semibold" fontSize="sm" wordBreak="break-all">{selected.path}</Text><Text fontSize="xs" color="gray.400" my={2}>{selected.context ? 'Unchanged context' : 'Changed symbols'}</Text>
          {(['added', 'removed', 'modified'] as const).flatMap((kind) => selected.symbols[kind].map((symbol) => <Box as="details" key={`${kind}:${symbol.logicalKey || symbol.id}`} mb={3}><Text as="summary" fontSize="xs" color={colors[kind]} cursor="pointer">{kind} · {symbol.name}</Text><Text as="pre" whiteSpace="pre-wrap" wordBreak="break-word" fontSize="10px" mt={2}>{symbol.code || symbol.signature}</Text></Box>))}
        </Box>}
      </Flex>}
    </Flex>
  )
}
