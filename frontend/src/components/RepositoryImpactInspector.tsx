import { useMemo, useState } from 'react'
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
  const [stage, setStage] = useState('')
  const [copying, setCopying] = useState(false)
  const entries = useMemo(() => Object.values(scene?.provenance ?? {}).sort((a, b) => a.name.localeCompare(b.name) || a.key.localeCompare(b.key)), [scene])
  const selected = scene?.provenance?.[selectedKey]
  const decisions = scene?.trace?.decisions ?? []
  const history = selectedKey ? decisions.filter((decision) => decision.key === selectedKey) : []
  const filtered = entries.filter((entry) => (!stage || entry.stage === stage || decisions.some((decision) => decision.stage === stage && decision.key === entry.key)) && `${entry.name} ${entry.paths.join(' ')} ${entry.origin}`.toLowerCase().includes(query.toLowerCase()))
  const stageDecisions = stage ? decisions.filter((decision) => decision.stage === stage) : []
  const counts = entries.filter((entry) => entry.objectKind === 'placement').reduce<Record<string, number>>((result, entry) => { result[entry.origin] = (result[entry.origin] ?? 0) + 1; return result }, {})
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
      <Button size="xs" variant={debug ? 'solid' : 'outline'} onClick={() => onDebugChange(!debug)} aria-pressed={debug} data-testid="impact-debug-toggle">Impact debug</Button>
      {!scene ? <Flex py={8} justify="center"><Spinner size="sm" /></Flex> : <VStack align="stretch" spacing={4} mt={3}>
        <Box data-testid="impact-coverage-summary">
          {scene.schemaVersion !== '2' ? <Text fontSize="xs" color="orange.300">Legacy comparison. Coverage and primitive provenance are unavailable; run a new comparison with the updated backend.</Text> : <>
          <Text fontSize="sm" fontWeight="medium">{scene.coverage?.length ?? 0} changed files represented</Text>
          <Text fontSize="xs" color="gray.400">{counts.authored ?? 0} authored · {counts.mapped ?? 0} mapped · {counts.raw ?? 0} raw</Text>
          <Text fontSize="xs" color="gray.400">Dependency radius {scene.effectiveRadius ?? 0} / {scene.requestedRadius ?? 0} requested</Text>
          {!!scene.omittedContext?.length && <Text fontSize="xs" color="orange.300">{new Set(scene.omittedContext).size} context files omitted by the output budget</Text>}
          {hiddenChanges > 0 && <Text fontSize="xs" color="orange.300">{hiddenChanges} changed files hidden by tag filters</Text>}
          {scene.manifestComplete === false && <Text fontSize="xs" color="orange.300">Git manifest incomplete; file coverage cannot be guaranteed.</Text>}
          {!!scene.coverage?.some((entry) => entry.factsUnavailable) && <Text fontSize="xs" color="orange.300">Some files have no indexed facts. Their dependency impact is unknown.</Text>}
          </>}
          {(scene.warnings ?? []).map((warning) => <Text key={warning} mt={1} fontSize="xs" color="orange.300">{warning}</Text>)}
        </Box>

        {selected && <Box border="1px solid" borderColor="whiteAlpha.200" rounded="md" p={3} data-testid="impact-selected-provenance">
          <Text fontSize="sm" fontWeight="semibold">{selected.name || selected.objectKind}</Text>
          <Text fontSize="xs" color="gray.400">{selected.origin} · {selected.grounding} · {selected.role}</Text>
          <Text mt={2} fontSize="xs">{selected.reason}</Text>
          <Text mt={1} fontSize="xs" color="gray.400">Introduced by {selected.stage}{selected.distance > 0 ? ` · ${selected.distance} dependency hops` : ''}</Text>
          {selected.partial && <Text mt={1} fontSize="xs" color="orange.300">Partial anchor coverage. Remaining file edits have a separate representative.</Text>}
          {selected.paths.map((path) => <Text key={path} mt={1} fontSize="xs" overflowWrap="anywhere">{path}</Text>)}
          {selected.witnesses.filter((witness) => witness.paths.length > 1).map((witness) => <Text key={`${witness.snapshotId}:${witness.seedPath}`} mt={2} fontSize="xs" color="gray.400" overflowWrap="anywhere">{witness.snapshotId === undefined ? '' : `${witness.snapshotId.slice(0, 8)}: `}{witness.paths.join(' → ')}</Text>)}
          {selected.evidence.map((evidence) => <Text key={evidence} mt={1} fontSize="xs" color="gray.400" overflowWrap="anywhere">{evidence}</Text>)}
          {debug && selected.logicalKey && <Text mt={2} fontSize="xs" color="gray.400" overflowWrap="anywhere">Map: {selected.logicalKey} · snapshot {selected.mapSnapshotId}</Text>}
        </Box>}

        {debug && <Box data-testid="impact-pipeline-stages">
          <Text mb={2} fontSize="xs" fontWeight="semibold">Pipeline stages</Text>
          {!scene.trace ? <Text fontSize="xs" color="gray.400">Detailed trace unavailable for this saved comparison.</Text> : scene.trace.stages.map((entry, index) => <Button key={entry.id} variant={stage === entry.id ? 'solid' : 'ghost'} w="full" h="auto" py={2} justifyContent="start" textAlign="left" whiteSpace="normal" onClick={() => setStage(stage === entry.id ? '' : entry.id)}>
            <Box><Text fontSize="xs">{index}. {entry.name}</Text><Text fontSize="10px" color="gray.400" fontWeight="normal">{entry.added} added · {entry.annotated} annotated · {entry.reused} reused · {entry.suppressed} suppressed</Text></Box>
          </Button>)}
        </Box>}

        <Box>
          <Input aria-label="Search impact primitives" placeholder="Search elements, views, connectors…" size="sm" value={query} onChange={(event) => setQuery(event.target.value)} />
          <VStack align="stretch" spacing={1} mt={2} maxH="300px" overflowY="auto">
            {filtered.map((entry) => <Button key={entry.key} size="xs" h="auto" py={2} variant={selectedKey === entry.key ? 'solid' : 'ghost'} justifyContent="start" textAlign="left" whiteSpace="normal" onClick={() => onSelect(entry.key)} data-testid="impact-primitive">
              <Box minW={0}><Text fontSize="xs">{entry.name || entry.objectKind}</Text><Text fontSize="10px" color="gray.400" fontWeight="normal">{entry.objectKind} · {entry.origin} · {entry.role}</Text></Box>
            </Button>)}
            {filtered.length === 0 && <Text fontSize="xs" color="gray.400">No matching primitives.</Text>}
          </VStack>
        </Box>

        {debug && (history.length > 0 || stageDecisions.length > 0) && <Box data-testid="impact-decisions">
          <Text mb={2} fontSize="xs" fontWeight="semibold">{stage ? 'Stage decisions' : 'Selected primitive history'}</Text>
          {(stage ? stageDecisions : history).map((decision, index) => <Box key={`${decision.key}:${index}`} py={2} borderBottom="1px solid" borderColor="whiteAlpha.100">
            <Button size="xs" variant="link" whiteSpace="normal" textAlign="left" onClick={() => onSelect(decision.representativeKeys[0] ?? decision.key)}>{decision.action} · {decision.key}</Button>
            <Text mt={1} fontSize="xs" color="gray.400">{decision.reason}</Text>
            {decision.paths.map((path) => <Text key={path} fontSize="10px" overflowWrap="anywhere">{path}</Text>)}
          </Box>)}
        </Box>}
      </VStack>}
    </Box>
  </Flex>
}
