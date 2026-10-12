import { useState } from 'react'
import { Box, Button, Flex, HStack, IconButton, Input, Spinner, Text, VStack } from '@chakra-ui/react'
import { ChevronLeftIcon, InfoIcon } from '@chakra-ui/icons'
import { api, type RepositoryImpactScene } from '../api/client'
import { copyTextToClipboard } from '../utils/clipboard'
import { toast } from '../utils/toast'

export default function RepositoryImpactInspector({ scene, selectedKey, onSelect, debug, onDebugChange, plain,
  collapsed = false, overlay = false, onExpand, onDock, hiddenChanges = 0,
}: {
  scene: RepositoryImpactScene | null
  selectedKey: string
  onSelect: (key: string) => void
  debug: boolean
  onDebugChange: (enabled: boolean) => void
  plain: boolean
  collapsed?: boolean
  overlay?: boolean
  onExpand?: () => void
  onDock?: () => void
  hiddenChanges?: number
}) {
  const [query, setQuery] = useState('')
  const [copying, setCopying] = useState(false)
  const parts = selectedKey.split('|')
  const selectedPlacement = scene?.placements?.[selectedKey]
  const selectedConnector = parts[0] === 'connector' ? scene?.views?.[parts[1]]?.connectors.find((entry) => entry.id === Number(parts[2])) : undefined
  const viewNames = (nodes: RepositoryImpactScene['tree']): Record<number, string> => Object.fromEntries(nodes.flatMap((view) => [[view.id, view.name], ...Object.entries(viewNames(view.children ?? []))]))
  const selectedView = parts[0] === 'view' ? viewNames(scene?.tree ?? [])[Number(parts[1])] : undefined
  const connectorTarget = scene?.connectorTargets?.[selectedKey]
  const connectorPaths = selectedConnector ? [
    scene?.placements?.[`placement|${parts[1]}|${selectedConnector.source_element_id}`],
    scene?.placements?.[`placement|${connectorTarget?.viewId ?? parts[1]}|${connectorTarget?.elementId ?? selectedConnector.target_element_id}`],
  ].flatMap((entry) => entry?.paths ?? []) : []
  const selected = selectedPlacement ?? (selectedConnector ? { name: selectedConnector.label || 'Relationship', origin: 'scene', grounding: 'see dependency evidence', role: 'relationship', paths: connectorPaths } : selectedView ? { name: selectedView, origin: 'workspace', grounding: 'see matching evidence', role: 'view', paths: [] } : undefined)
  const files = (scene?.coverage ?? []).filter((file) => file.path.toLowerCase().includes(query.toLowerCase()))
  const summary = scene?.summary
  const selectedPaths = selectedView ? Object.values(scene?.placements ?? {}).filter((entry) => entry.viewId === Number(parts[1])).flatMap((entry) => entry.paths) : selected?.paths ?? []
  const scopes = selectedPaths.map((path) => path.replace(/^\.\//, '').replace(/\/$/, ''))
  const relevantPath = (path: string) => !selected || scopes.some((scope) => path === scope || (!!scope && path.startsWith(`${scope}/`)))
  const auditName = (target: string) => {
    const ref = scene?.audit?.references?.[target]
    if (ref) return ref
    const match = /^v(-?\d+)\/e(-?\d+)$/.exec(target)
    return match ? scene?.placements?.[`placement|${match[1]}|${match[2]}`]?.name ?? target : target
  }
  const copyMarkdown = async () => {
    if (!scene) return
    setCopying(true)
    try {
      const result = await api.repositories.impactMermaid(scene.repositoryId, scene.comparisonKey, { plain, markdown: true })
      await copyTextToClipboard(result.markdown)
      toast({ status: 'success', title: 'Copied change diagram as Markdown' })
    } catch (error) {
      toast({ status: 'error', title: 'Copy failed', description: error instanceof Error ? error.message : 'Could not export the diagram' })
    } finally { setCopying(false) }
  }
  if (collapsed) return <Flex h="full" align="center" justify="center" bg="var(--bg-canvas)" data-testid="repository-impact-inspector">
    <IconButton aria-label="Expand impact inspector" icon={<InfoIcon />} size="sm" variant="ghost" onClick={onExpand} />
  </Flex>
  return <Flex direction="column" h="full" minH={0} bg="var(--bg-canvas)" borderLeft="1px solid" borderColor="whiteAlpha.100" data-testid="repository-impact-inspector">
    <HStack px={3} py={2} borderBottom="1px solid" borderColor="whiteAlpha.100" flexShrink={0}>
      <Text fontSize="sm" fontWeight="semibold" flex={1}>Impact inspector</Text>
      {overlay && <IconButton aria-label="Dock impact inspector" size="xs" variant="ghost" icon={<ChevronLeftIcon />} onClick={onDock} />}
      <Button size="xs" variant="ghost" isDisabled={!scene} isLoading={copying} onClick={() => void copyMarkdown()}>Copy Markdown</Button>
    </HStack>
    <Box overflowY="auto" flex={1} minH={0} p={3}>
      <Button size="xs" variant={debug ? 'solid' : 'outline'} onClick={() => onDebugChange(!debug)} aria-pressed={debug}>Impact debug</Button>
      {!scene ? <Flex py={8} justify="center"><Spinner size="sm" /></Flex> : <VStack align="stretch" spacing={4} mt={3}>
        <Box data-testid="impact-coverage-summary">
          <Text fontSize="sm" fontWeight="medium">{scene.coverage?.length ?? 0} changed files represented</Text>
          {summary && <Text fontSize="xs" color="gray.400">{summary.affectedPlacements} affected · {summary.dependencyNeighbours} dependency neighbours · {summary.authoredContext} authored context · {summary.containers} containers</Text>}
          <Text fontSize="xs" color="gray.400">Dependency radius {scene.effectiveRadius ?? 0} / {scene.requestedRadius ?? 0} requested</Text>
          {summary && <Text fontSize="xs" color="gray.400">{summary.indexedSymbolChanges} indexed symbol changes · {summary.displayedSymbolChanges} displayed · {summary.excludedSymbolChanges} excluded from canvas</Text>}
          {summary && !summary.lineCountsComplete && <Text fontSize="xs" color="orange.300">Line totals are incomplete; some counts are unknown.</Text>}
          {scene.manifestComplete === false && <Text fontSize="xs" color="orange.300">File manifest incomplete; coverage cannot be guaranteed.</Text>}
          {scene.coverage?.some((file) => file.factsUnavailable) && <Text fontSize="xs" color="orange.300">Some files lack captured sources; their dependency impact is unknown.</Text>}
          {!!scene.omittedContext?.length && <Text fontSize="xs" color="orange.300">{new Set(scene.omittedContext).size} context placements omitted by display selection</Text>}
          {hiddenChanges > 0 && <Text fontSize="xs" color="orange.300">{hiddenChanges} changed files hidden by tag filters</Text>}
          {(scene.warnings ?? []).map((warning) => <Text key={warning} mt={1} fontSize="xs" color="orange.300">{warning}</Text>)}
        </Box>
        {selected && <Box border="1px solid" borderColor="whiteAlpha.200" rounded="md" p={3} data-testid="impact-selected-provenance">
          <Text fontSize="sm" fontWeight="semibold">{selected.name}</Text>
          <Text fontSize="xs" color="gray.400">{selected.origin} · {selected.grounding} · {selected.role}</Text>
          {selected.paths.map((path) => <Text key={path} fontSize="xs" overflowWrap="anywhere">{path}</Text>)}
          {debug && scene.audit?.matches.filter((match) => !match.path && match.placement?.viewId === Number(parts[1]) && (selectedView || match.placement?.elementId === Number(parts[2]))).map((match, index) => <Text key={index} fontSize="xs" color="orange.300">{match.outcome}: {match.sourceLink} — {match.reason}</Text>)}
        </Box>}
        <Input aria-label="Search impact files" placeholder="Search changed files…" size="sm" value={query} onChange={(event) => setQuery(event.target.value)} />
        {files.map((file) => <Box key={file.path} data-testid="impact-file">
          <Text fontSize="xs" fontWeight="semibold" overflowWrap="anywhere">{file.path}</Text>
          {(() => {
            const detail = scene.files?.find((entry) => entry.path === file.path)
            return detail && <Text fontSize="xs" color="gray.400">{detail.binary ? 'binary' : `+${detail.linesAdded ?? '?'} / −${detail.linesRemoved ?? '?'}`} · analysis: base {detail.baseAnalysis ?? 'unknown'}, head {detail.headAnalysis ?? 'unknown'}{detail.metadataOnly ? ' · metadata only' : ''}</Text>
          })()}
          {file.placementKeys.map((key) => <Button key={key} size="xs" variant={selectedKey === key ? 'solid' : 'ghost'} onClick={() => onSelect(key)} data-testid="impact-primitive">{scene.placements?.[key]?.name ?? key}</Button>)}
          {debug && scene.audit?.matches.filter((match) => match.path === file.path).map((match, index) => <Text key={index} fontSize="xs" color={match.outcome === 'rejected' ? 'orange.300' : 'gray.400'} overflowWrap="anywhere">
            {match.outcome} · {match.kind} · {match.sourceLink || match.scope} · {match.side}{match.partial ? ' · partial' : ''} — {match.reason}
          </Text>)}
        </Box>)}
        {debug && <Box data-testid="impact-decisions">
          <Text fontSize="xs" fontWeight="semibold">Audit decisions</Text>
          {!scene.audit && <Text fontSize="xs" color="gray.400">Detailed evidence unavailable.</Text>}
          {scene.audit?.decisions.map((decision, index) => <Text key={index} fontSize="xs" color="gray.400" overflowWrap="anywhere">{decision.stage} · {auditName(decision.target)} · {decision.reason}</Text>)}
          {scene.audit?.witnesses.filter((witness) => witness.paths.some(relevantPath)).map((witness, index) => <Text key={index} fontSize="xs" color="gray.400" overflowWrap="anywhere">{witness.side}: {witness.paths.join(' → ')}</Text>)}
          {scene.audit?.dependencies.filter((dependency) => relevantPath(dependency.fromPath) || relevantPath(dependency.toPath)).map((dependency, index) => <Text key={index} fontSize="xs" color="gray.400" overflowWrap="anywhere">Observed in {dependency.side}: {dependency.fromPath} → {dependency.toPath} · {dependency.observations} observations</Text>)}
          {Object.entries(scene.audit?.excludedSymbols ?? {}).map(([symbol, reason]) => <Text key={symbol} fontSize="xs" color="gray.400">{symbol}: {reason}</Text>)}
        </Box>}
      </VStack>}
    </Box>
  </Flex>
}
