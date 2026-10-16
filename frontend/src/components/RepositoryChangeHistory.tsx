import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { ChevronRightIcon } from '@chakra-ui/icons'
import { Box, Button, Flex, HStack, Text, Tooltip } from '@chakra-ui/react'
import type { RepositoryImpactScene } from '../api/client'
import { CHANGE_HISTORY_FILES, materializedSceneChanges, mergeChangeHistory, type MapChange, type MapChangeGroup, type MapChangeKind, type MapChangeTarget } from '../utils/repositoryChangeHistory'

const kinds: Array<{ kind: MapChangeKind; label: string; color: string; background: string }> = [
  { kind: 'element', label: 'Element', color: 'cyan.200', background: 'rgba(118, 228, 247, 0.08)' },
  { kind: 'view', label: 'View', color: 'purple.200', background: 'rgba(214, 188, 250, 0.08)' },
  { kind: 'connector', label: 'Connector', color: 'orange.200', background: 'rgba(251, 211, 141, 0.08)' },
]
const actionLabels = { added: '+', modified: '~', removed: '−' }
const pageSize = 12

const actionStyles = {
  removed: { color: 'red.200', bg: 'rgba(252, 129, 129, 0.10)' },
  added: { color: 'green.200', bg: 'rgba(104, 211, 145, 0.10)' },
  modified: { color: 'orange.200', bg: 'rgba(246, 173, 85, 0.10)' },
}

function ChangeChip({ change, onFocus, measure = false }: { change: MapChange; onFocus?: (targets: MapChangeTarget[]) => void; measure?: boolean }) {
  const kind = kinds.find(({ kind }) => kind === change.kind)!
  const content = <><Text as="span" fontFamily="mono" mr={1}>{actionLabels[change.action]}</Text><Text as="span" isTruncated>{change.name}</Text></>
  const style = { ...actionStyles[change.action], h: '23px', maxW: '220px', minW: 0, px: 1.5, fontSize: '12px', lineHeight: '16px', fontWeight: 'normal', flexShrink: 0 }
  if (measure) return <Box as="span" display="inline-flex" alignItems="center" {...style}>{content}</Box>
  return <Button {...style} size="xs" variant="ghost" _hover={{ bg: style.bg, filter: 'brightness(1.3)' }} title={`${kind.label} ${change.action}: ${change.name}${change.action === 'removed' ? ' · focuses surviving context' : ''}`} aria-label={`Focus ${kind.label.toLowerCase()} ${change.action}: ${change.name}`} isDisabled={!onFocus} onClick={() => onFocus?.([change.target])}>{content}</Button>
}

function ChangeHistoryRow({ group, expanded, page, onToggle, onPage, onFocus }: {
  group: MapChangeGroup; expanded: boolean; page: number
  onToggle: () => void; onPage: (page: number) => void; onFocus?: (targets: MapChangeTarget[]) => void
}) {
  const row = useRef<HTMLDivElement>(null)
  const metadata = useRef<HTMLDivElement>(null)
  const measure = useRef<HTMLDivElement>(null)
  const [fits, setInline] = useState(false)
  // Bound measurement work too: a large materialization stays paginated.
  const canInline = group.changes.length <= pageSize
  const inline = fits && canInline
  useLayoutEffect(() => {
    if (!canInline || !row.current || !metadata.current || !measure.current) {
      setInline(false)
      return
    }
    const update = () => {
      if (!row.current || !metadata.current || !measure.current) return
      const width = row.current.clientWidth
      // Preserve a readable file path, the chevron slot, gaps, and timestamp.
      const pathWidth = Math.min(360, width * 0.45)
      const available = width - 16 - 24 - 24 - pathWidth - metadata.current.offsetWidth
      setInline(measure.current.scrollWidth <= available)
    }
    update()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(update)
    observer.observe(row.current)
    observer.observe(metadata.current)
    observer.observe(measure.current)
    return () => observer.disconnect()
  }, [group, canInline])
  const open = expanded && !inline
  const pages = Math.ceil(group.changes.length / pageSize)
  const currentPage = Math.min(page, pages - 1)
  const focus = (changes: MapChange[]) => onFocus?.(changes.map((change) => change.target))
  return (
    <Box position="relative" isolation="isolate" borderBottom="1px solid" borderColor="whiteAlpha.100" _last={{ borderBottom: 0 }} data-history-path={group.path}>
      <Box
        key={group.updates}
        data-testid="history-change-flash"
        aria-hidden="true"
        position="absolute"
        inset={0}
        zIndex={-1}
        pointerEvents="none"
        animation="repository-history-row-flash 1.5s ease-out forwards"
        sx={{
          '@keyframes repository-history-row-flash': {
            '0%, 15%': { backgroundColor: 'rgba(var(--accent-rgb), 0.20)', boxShadow: 'inset 2px 0 0 var(--accent)' },
            '100%': { backgroundColor: 'transparent', boxShadow: 'inset 2px 0 0 transparent' },
          },
          '@media (prefers-reduced-motion: reduce)': { animation: 'none' },
        }}
      />
      <Flex ref={row} data-testid="history-change-row" align="center" gap={2} px={2} py={1} minH="30px" minW={0}>
        {inline ? <Box w={4} flexShrink={0} /> : <Button variant="unstyled" minW={4} h={5} lineHeight={1} display="flex" alignItems="center" justifyContent="center" aria-label={`${open ? 'Collapse' : 'Expand'} changes for ${group.path || 'repository map'}`} aria-expanded={open} onClick={onToggle}>
          <ChevronRightIcon boxSize={3.5} color="gray.500" transform={open ? 'rotate(90deg)' : undefined} />
        </Button>}
        <Button variant="unstyled" minW={0} h="auto" flex="1" textAlign="left" title={group.path || 'Repository map'} aria-label={`Focus changes in ${group.path || 'repository map'}`} onClick={() => focus(group.changes)} isDisabled={!onFocus}>
          <Text fontSize="xs" fontWeight="normal" isTruncated>{group.path || 'Repository map'}</Text>
        </Button>
        {inline ? <HStack spacing={1} flexShrink={0} data-testid="history-inline-changes">
          {group.changes.map((change) => <ChangeChip key={change.key} change={change} onFocus={onFocus} />)}
        </HStack> : <HStack spacing={1} flexShrink={0}>
          {kinds.filter(({ kind }) => group.counts[kind]).map(({ kind, label, color, background }) => <Tooltip key={kind} label={`Focus ${group.counts[kind]} ${label.toLowerCase()} ${group.counts[kind] === 1 ? 'change' : 'changes'}`}>
            <Button size="xs" h="20px" minW={0} px={1.5} fontSize="10px" fontWeight="medium" color={color} bg={background} _hover={{ bg: background, filter: 'brightness(1.4)' }} aria-label={`Focus ${label.toLowerCase()} changes in ${group.path || 'repository map'}`} isDisabled={!onFocus} onClick={() => focus(group.changes.filter((change) => change.kind === kind))}>
              <Text as="span" display={{ base: 'none', md: 'inline' }} mr={1}>{label}</Text><Text as="span" display={{ base: 'inline', md: 'none' }} mr={1}>{label[0]}</Text>{group.counts[kind]}
            </Button>
          </Tooltip>)}
        </HStack>}
        <HStack ref={metadata} data-testid="history-change-metadata" spacing={2} flexShrink={0}>
          {group.updates > 1 && <Text fontSize="10px" color="gray.500" flexShrink={0} title={`${group.updates} updates to this file`}>×{group.updates}</Text>}
          <Text as="time" display={{ base: 'none', md: 'block' }} dateTime={new Date(group.updatedAt).toISOString()} fontFamily="mono" fontSize="10px" color="gray.500" flexShrink={0}>{new Date(group.updatedAt).toLocaleTimeString([], { hour12: false })}</Text>
        </HStack>
      </Flex>
      {canInline && <Flex ref={measure} data-testid="history-change-measure" position="absolute" top={0} left={0} w="max-content" gap={1} visibility="hidden" pointerEvents="none" aria-hidden="true">
        {group.changes.map((change) => <ChangeChip key={change.key} change={change} measure />)}
      </Flex>}
      {open && <Box px={3} pb={2} data-testid="history-change-details">
        <Flex gap={1} flexWrap="wrap">
          {group.changes.slice(currentPage * pageSize, (currentPage + 1) * pageSize).map((change) => <ChangeChip key={change.key} change={change} onFocus={onFocus} />)}
        </Flex>
        {pages > 1 && <HStack spacing={2} mt={1} fontSize="10px" color="gray.500">
          <Button size="xs" h={5} variant="ghost" isDisabled={currentPage === 0} onClick={() => onPage(currentPage - 1)}>Previous</Button>
          <Text>{currentPage * pageSize + 1}–{Math.min((currentPage + 1) * pageSize, group.changes.length)} of {group.changes.length}</Text>
          <Button size="xs" h={5} variant="ghost" isDisabled={currentPage === pages - 1} onClick={() => onPage(currentPage + 1)}>Next</Button>
        </HStack>}
      </Box>}
    </Box>
  )
}

export default function RepositoryChangeHistory({ scene, onFocus, summary }: {
  scene: RepositoryImpactScene | null
  onFocus?: (targets: MapChangeTarget[]) => void
  summary?: ReactNode
}) {
  const observed = useRef<RepositoryImpactScene | null>(null)
  const [history, setHistory] = useState<MapChangeGroup[]>([])
  const [expanded, setExpanded] = useState<string | null>(null)
  const [page, setPage] = useState(0)
  useEffect(() => {
    if (!scene || observed.current?.version === scene.version && observed.current.repositoryId === scene.repositoryId) return
    const previous = observed.current
    const sameRepository = previous?.repositoryId === scene.repositoryId
    const changes = materializedSceneChanges(sameRepository ? previous : null, scene)
    observed.current = scene
    setHistory((old) => mergeChangeHistory(sameRepository ? old : [], changes, Date.now()))
    setPage(0)
    if (!sameRepository) setExpanded(null)
  }, [scene])

  return (
    <Box mt={3} mb={3} minW={0} data-testid="repository-change-history">
      <Flex align="center" justify="space-between" gap={2} wrap="wrap" mb={2}>
        <Tooltip label={`Live map changes observed this session. Repeated edits update the same file row. Keeps the latest ${CHANGE_HISTORY_FILES} files.`}>
          <HStack spacing={2} fontSize="10px" color="gray.500">
            <Text color="gray.400" fontWeight="semibold" textTransform="uppercase" letterSpacing="0.08em">Map changes</Text>
            <Text>{history.length ? `${history.length}${history.length === CHANGE_HISTORY_FILES ? ' recent' : ''} ${history.length === 1 ? 'entry' : 'entries'}` : 'This session'}</Text>
          </HStack>
        </Tooltip>
        {summary}
      </Flex>
      <Box maxH="96px" overflowY="auto" overflowX="hidden" overscrollBehavior="contain" border="1px solid" borderColor="whiteAlpha.100" borderRadius="md" bg="whiteAlpha.50" tabIndex={history.length ? 0 : undefined} aria-label="Recent materialized map changes" sx={{ scrollbarWidth: 'thin' }}>
        {!history.length && <Text px={2.5} py={2} fontSize="xs" color="gray.500">Materialized changes will appear here.</Text>}
        {history.map((group) => <ChangeHistoryRow key={group.path} group={group} expanded={expanded === group.path} page={page} onToggle={() => { setExpanded(expanded === group.path ? null : group.path); setPage(0) }} onPage={setPage} onFocus={onFocus} />)}
      </Box>
    </Box>
  )
}
