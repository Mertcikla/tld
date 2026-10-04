import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import {
  Alert,
  AlertIcon,
  Badge,
  Box,
  Button,
  Center,
  Flex,
  FormControl,
  Grid,
  HStack,
  IconButton,
  Input,
  Popover,
  PopoverArrow,
  PopoverBody,
  PopoverCloseButton,
  PopoverContent,
  PopoverFooter,
  PopoverHeader,
  PopoverTrigger,
  Portal,
  Progress,
  Spinner,
  Switch,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import {
  AddIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  DeleteIcon,
  RepeatIcon,
  SettingsIcon,
} from '@chakra-ui/icons'
import {
  api,
  type CodeSnapshot,
  type CompletedRepositoryMap,
  type IndexedRepository,
  type RepositoryGitHistory,
  type RepositoryPullRequest,
  type OpenRepositoryPullRequest,
  type RepositoryIndexProgress,
  type RepositoryIndexerCheck,
  type RepositoryMapProgress,
  type SnapshotDiff,
  type RepositoryImpact as RepositoryImpactResult,
  type LiveRepositoryImpact,
  type RepositoryWatchStatus,
  type SnapshotSourceChange,
} from '../api/client'
import ConfirmDialog from '../components/ConfirmDialog'
import RepositorySettings from './RepositorySettings'
import RepositoryHistory from '../components/RepositoryHistory'
import RepositoryTargetPicker from '../components/RepositoryTargetPicker'
import RepositoryChangeCanvas from '../components/RepositoryChangeCanvas'
import RepositorySymbols from '../components/RepositorySymbols'
import RepositoryWatcherPanel from '../components/RepositoryWatcherPanel'
import RepositoryPullRequestPanel from '../components/RepositoryPullRequestPanel'
import {
  defaultRepositoryTargets,
  snapshotForTarget,
  targetMapOptions,
} from '../utils/repositoryTargets'
import { indexStageLabel } from '../utils/repositoryWatcher'
import { invalidateIndexedRepositories, parseRepositoryAddInput } from '../utils/repositoryResolver'
import { toast } from '../utils/toast'

const accentStyle = {
  bg: 'var(--accent)',
  color: 'white',
  _hover: { bg: 'var(--accent)', filter: 'brightness(1.08)' },
}
const showIdsKey = 'tld:repositories:showIds'
const snapshotPageSize = 5
function readShowIds() {
  try {
    return (
      typeof localStorage !== 'undefined' &&
      localStorage.getItem(showIdsKey) === 'true'
    )
  } catch {
    return false
  }
}
const nameOf = (root: string) =>
  root.split(/[/\\]/).filter(Boolean).pop() || 'repository'
const short = (id: string) => (id ? id.slice(0, 12) : '—')
function age(unix: number) {
  const minutes = Math.max(0, Math.floor((Date.now() / 1000 - unix) / 60))
  return minutes < 1
    ? 'just now'
    : minutes < 60
      ? `${minutes}m ago`
      : minutes < 1440
        ? `${Math.floor(minutes / 60)}h ago`
        : `${Math.floor(minutes / 1440)}d ago`
}
function Glyph({ name }: { name: string }) {
  return (
    <Center
      w="28px"
      h="28px"
      flexShrink={0}
      bg="whiteAlpha.100"
      borderRadius="md"
      fontWeight="semibold"
    >
      {name[0]?.toUpperCase()}
    </Center>
  )
}
function RepositoryModeIcon({ mode }: { mode: 'compare' | 'live' | 'pr' }) {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" style={{ flexShrink: 0 }}>
      {mode === 'compare' ? (
        <>
          <path d="M4 7h16m-4-4 4 4-4 4M20 17H4m4-4-4 4 4 4" />
        </>
      ) : mode === 'live' ? (
        <path d="M2 12h4l3-8 6 16 3-8h4" />
      ) : (
        <>
          <circle cx="6" cy="5" r="3" />
          <circle cx="6" cy="19" r="3" />
          <circle cx="18" cy="19" r="3" />
          <path d="M6 8v8M18 16V9a4 4 0 0 0-4-4h-1m3-3-3 3 3 3" />
        </>
      )}
    </svg>
  )
}
function Label({ children }: { children: React.ReactNode }) {
  return (
    <Text
      fontSize="10px"
      fontWeight="bold"
      color="gray.500"
      textTransform="uppercase"
      letterSpacing="0.06em"
    >
      {children}
    </Text>
  )
}
function ErrorMessage({ message }: { message: string }) {
  return message ? (
    <Alert status="error" borderRadius="md">
      <AlertIcon />
      <Text fontSize="sm">{message}</Text>
    </Alert>
  ) : null
}

function FileTreeIcon({ directory, expanded, change }: { directory?: boolean; expanded?: boolean; change?: SnapshotSourceChange['change'] }) {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true" style={{ flexShrink: 0 }}>
      {directory ? expanded ? (
        <>
          <path d="M3 7V5a2 2 0 0 1 2-2h5l3 3h6a2 2 0 0 1 2 2v2" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
          <path d="M3 8h18l-2 12H5a2 2 0 0 1-2-2V8Z" fill="currentColor" fillOpacity="0.25" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
        </>
      ) : (
        <path d="M3 7V5a2 2 0 0 1 2-2h5l3 3h6a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z" fill="currentColor" fillOpacity="0.25" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />
      ) : (
        <g stroke={change === 'added' ? '#a6e22e' : change === 'removed' ? '#ff656d' : '#ffb340'} strokeWidth="2.5">
          <rect x="3" y="3" width="18" height="18" rx="2" />
          {change === 'added' ? <path d="M8 12h8M12 8v8" /> : change === 'removed' ? <path d="M8 12h8" /> : <circle cx="12" cy="12" r="1.5" fill="#ffb340" />}
        </g>
      )}
    </svg>
  )
}

function FileTree({ files, onSelect, diagram, repositoryRoot }: { files: SnapshotSourceChange[]; onSelect: (path: string) => void; diagram: RepositoryImpactResult; repositoryRoot: string }) {
  const [expandedFiles, setExpandedFiles] = useState<Set<string>>(new Set())
  const [closed, setClosed] = useState<Set<string>>(new Set())
  const entries = useMemo(() => {
    type Entry = { name: string; path: string; children: Map<string, Entry>; file?: SnapshotSourceChange }
    const root: Entry = { name: '', path: '', children: new Map() }
    for (const file of [...files].sort((a, b) => a.path.localeCompare(b.path))) {
      let parent = root
      for (const name of file.path.split('/')) {
        let entry = parent.children.get(name)
        if (!entry) {
          entry = { name, path: parent.path ? `${parent.path}/${name}` : name, children: new Map() }
          parent.children.set(name, entry)
        }
        parent = entry
      }
      parent.file = file
    }
    return root.children
  }, [files])

  function renderEntries(children: typeof entries): React.ReactNode {
    return [...children.values()].map((initial) => {
      let entry = initial
      let label = entry.name
      while (!entry.file && entry.children.size === 1) {
        const child = [...entry.children.values()][0]
        if (child.file) break
        label += `/${child.name}`
        entry = child
      }
      const { path, file } = entry
      const directory = !file
      const expanded = directory ? !closed.has(path) : expandedFiles.has(path)
      return (
        <Box key={path}>
          <Flex
            as="button"
            type="button"
            w="full"
            minW={0}
            minH="28px"
            py={1}
            gap={2}
            align="center"
            textAlign="left"
            color={directory ? '#90918e' : '#c4c4bf'}
            _hover={{ bg: 'whiteAlpha.50' }}
            _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '-2px' }}
            aria-label={directory ? `${expanded ? 'Collapse' : 'Expand'} ${path}` : path}
            aria-expanded={expanded}
            title={path}
            onClick={() => {
              if (!directory) {
                onSelect(path)
                setExpandedFiles((old) => {
                  const next = new Set(old)
                  if (next.has(path)) next.delete(path)
                  else next.add(path)
                  return next
                })
                return
              }
              setClosed((old) => {
                const next = new Set(old)
                if (next.has(path)) next.delete(path)
                else next.add(path)
                return next
              })
            }}
          >
            <FileTreeIcon directory={directory} expanded={expanded} change={file?.change} />
            <Text as="span" flex={1} minW={0} fontSize="sm" lineHeight="20px" isTruncated>{label}</Text>
            {file && file.linesAdded !== undefined && file.linesRemoved !== undefined && (
              <HStack as="span" spacing={1} flexShrink={0} fontSize="xs" lineHeight="20px" aria-label={`${file.linesAdded} lines added, ${file.linesRemoved} lines removed`}>
                <Text as="span" color="#a6e22e">+{file.linesAdded}</Text>
                <Text as="span" color="#ff656d">−{file.linesRemoved}</Text>
              </HStack>
            )}
          </Flex>
          {expanded && (
            <Box ml="8px" pl="15px" borderLeft="1px solid" borderColor="whiteAlpha.200">
              {directory ? renderEntries(entry.children) : <RepositorySymbols inline repositoryRoot={repositoryRoot} path={path} diagram={diagram} />}
            </Box>
          )}
        </Box>
      )
    })
  }

  return (
    <Box px={3} pb={3} overflowY="auto" maxH={{ base: '220px', lg: 'none' }}>
      {!files.length && (
        <Text p={3} fontSize="sm" color="gray.500">No source changes.</Text>
      )}
      {renderEntries(entries)}
    </Box>
  )
}

function CompareSide({
  side,
  value,
  branch,
  snapshots,
  history,
  maps,
  showIds,
  disabled,
  locked = false,
  onChange,
  onMap,
}: {
  side: 'Base' | 'Head'
  value: string
  branch: string
  snapshots: CodeSnapshot[]
  history: RepositoryGitHistory | null
  maps: CompletedRepositoryMap[]
  showIds: boolean
  disabled: boolean
  locked?: boolean
  onChange: (value: string) => void
  onMap: () => void
}) {
  const snapshot = snapshotForTarget(value, snapshots)
  const mapped =
    !!snapshot &&
    maps.some((m) => m.result.snapshotId === snapshot?.id)
  const known =
    value === 'working_tree' ||
    snapshots.some((s) => value === `snapshot:${s.id}`) ||
    history?.commits.some((c) => value === `commit:${c.sha}`)
  return (
    <Box
      flex={1}
      minW={0}
      p={3}
      bg="whiteAlpha.50"
      border="1px solid"
      borderColor="whiteAlpha.100"
      borderRadius="lg"
    >
      <HStack mb={2}>
        <Box
          w={2}
          h={2}
          borderRadius="full"
          bg={side === 'Base' ? 'gray.400' : 'green.400'}
        />
        <Label>{side}</Label>
        <Box flex={1} />
        <Badge
          colorScheme={mapped ? 'green' : snapshot ? 'blue' : 'gray'}
          fontSize="2xs"
        >
          {mapped ? 'Mapped' : snapshot ? 'Indexed' : 'Not captured'}
        </Badge>
      </HStack>
      <RepositoryTargetPicker
        aria-label={`${side} target`}
        data-testid={`repositories-${side.toLowerCase()}-target`}
        value={value}
        isDisabled={disabled || locked}
        onChange={onChange}
        groups={[
          { options: [
            { value: '', label: 'Select a target' },
            { value: 'working_tree', label: 'Working tree · local contents' },
            ...(!known && value ? [{ value, label: value.startsWith('commit:') ? `Commit ${short(value.slice(7))}` : value }] : []),
          ] },
          { label: 'Saved snapshots', options: [...snapshots].reverse().map((s) => ({
            value: `snapshot:${s.id}`,
            label: `${short(s.gitRevision)} · ${s.gitBranch || 'detached / non-Git'}${s.commitMessage ? ` · ${s.commitMessage}` : ''} · ${age(s.createdUnix)}${showIds ? ` · ${short(s.id)}` : ''}`,
          })) },
          { label: 'Commits', options: (history?.commits ?? []).map((c) => ({
            value: `commit:${c.sha}`, label: `${c.sha.slice(0, 7)} · ${c.subject}`,
          })) },
        ]}
      />
      <Flex mt={2} gap={2} align="center">
        <Text fontSize="xs" color="gray.500" flex={1} isTruncated>
          {value === 'working_tree'
            ? 'Includes staged, unstaged, and untracked source files'
            : snapshot
              ? snapshot.gitBranch || 'No captured branch'
              : branch
                ? `Branch context: ${branch}`
                : 'Exact committed revision'}
        </Text>
        <Button
          size="xs"
          variant="outline"
          data-testid={
            side === 'Head' ? 'repositories-map' : 'repositories-map-base'
          }
          isDisabled={disabled || !value}
          onClick={onMap}
        >
          Map {side.toLowerCase()}
        </Button>
      </Flex>
      {snapshot?.warnings.length ? (
        <Tooltip label={snapshot.warnings.join('\n')}>
          <Text fontSize="xs" color="orange.300" mt={1}>
            {snapshot.warnings.length} indexing warning(s)
          </Text>
        </Tooltip>
      ) : null}
    </Box>
  )
}

export default function Repositories() {
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const [repositories, setRepositories] = useState<IndexedRepository[]>([])
  const [selectedId, setSelectedId] = useState(() => params.get('repo') || '')
  const [base, setBase] = useState(() => params.get('base') || '')
  const [head, setHead] = useState(() => params.get('head') || '')
  const [baseBranch, setBaseBranch] = useState(
    () => params.get('baseBranch') || '',
  )
  const [headBranch, setHeadBranch] = useState(
    () => params.get('headBranch') || '',
  )
  const [branch, setBranch] = useState(() => params.get('branch') || '')
  const [showRepositorySettings, setShowRepositorySettings] = useState(() => params.get('page') === 'settings')
  const [collapsed, setCollapsed] = useState(false)
  const [historyCollapsed, setHistoryCollapsed] = useState(false)
  const [showIds] = useState(readShowIds)
  const [snapshots, setSnapshots] = useState<CodeSnapshot[]>([])
  const [maps, setMaps] = useState<CompletedRepositoryMap[]>([])
  const [history, setHistory] = useState<RepositoryGitHistory | null>(null)
  const [loading, setLoading] = useState(true)
  const [dataLoading, setDataLoading] = useState(false)
  const [error, setError] = useState('')
  const [dataError, setDataError] = useState('')
  const [historyError, setHistoryError] = useState('')
  const [operationError, setOperationError] = useState('')
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState<RepositoryMapProgress | null>(null)
  const [comparison, setComparison] = useState<RepositoryImpactResult | null>(null)
  const [live, setLive] = useState<LiveRepositoryImpact | null>(null)
  const [watch, setWatch] = useState<RepositoryWatchStatus | null>(null)
  const [watchBusy, setWatchBusy] = useState(false)
  const [mode, setMode] = useState(() => params.get('mode') === 'live' || params.get('mode') === 'watch' ? 'live' : params.get('mode') === 'pr' ? 'pr' : 'compare')
  const compareTargets = useRef<{ base: string; head: string; baseBranch: string; headBranch: string } | null>(null)
  const [openPullRequests, setOpenPullRequests] = useState<OpenRepositoryPullRequest[] | null>(null)
  const [prListLoading, setPrListLoading] = useState(false)
  const [prListError, setPrListError] = useState('')
  const prListOperation = useRef<AbortController | null>(null)
  const [prInput, setPrInput] = useState('')
  const [pullRequest, setPullRequest] = useState<RepositoryPullRequest | null>(null)
  const [selectedPath, setSelectedPath] = useState('')
  const [filesTab, setFilesTab] = useState<'files' | 'symbols'>('files')
  const liveVersion = useRef('')
  const shownImpact = mode === 'live' ? live?.diagram ?? null : comparison
  const diff: SnapshotDiff | null = shownImpact?.diff ?? null
  const [nonce, setNonce] = useState(0)
  const [repoToDelete, setRepoToDelete] = useState<IndexedRepository | null>(
    null,
  )
  const [deleteMaterialized, setDeleteMaterialized] = useState(false)
  const [deleteClone, setDeleteClone] = useState(false)
  const [deletingRepo, setDeletingRepo] = useState(false)
  const [snapshotToDelete, setSnapshotToDelete] = useState<CodeSnapshot | null>(
    null,
  )
  const [deletingSnapshot, setDeletingSnapshot] = useState(false)
  const [addOpen, setAddOpen] = useState(false)
  const [addPath, setAddPath] = useState('')
  const [addWatch, setAddWatch] = useState(false)
  const [addMap, setAddMap] = useState(true)
  const [addRequirements, setAddRequirements] =
    useState<RepositoryIndexerCheck | null>(null)
  const [checkingIndexers, setCheckingIndexers] = useState(false)
  const [adding, setAdding] = useState(false)
  const [addProgress, setAddProgress] = useState<RepositoryIndexProgress | null>(
    null,
  )
  const [addError, setAddError] = useState('')
  const [visibleSnapshots, setVisibleSnapshots] = useState(snapshotPageSize)
  const initialized = useRef('')
  const operation = useRef<AbortController | null>(null)
  const selectedRef = useRef(selectedId)
  selectedRef.current = selectedId
  const restored = useRef({ base, head })
  const selected = repositories.find((r) => r.id === selectedId)
  const newestSnapshots = useMemo(() => [...snapshots].reverse(), [snapshots])
  const visibleSnapshotList = useMemo(
    () => newestSnapshots.slice(0, visibleSnapshots),
    [newestSnapshots, visibleSnapshots],
  )
  const loadMoreSnapshots = () =>
    setVisibleSnapshots((count) =>
      Math.min(count + snapshotPageSize, snapshots.length),
    )
  const collapseSnapshots = () => setVisibleSnapshots(snapshotPageSize)
  const reload = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const items = await api.repositories.list()
      invalidateIndexedRepositories()
      setRepositories(items)
      setSelectedId((id) =>
        items.some((r) => r.id === id) ? id : items[0]?.id || '',
      )
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Could not load repositories',
      )
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => {
    void reload()
  }, [reload])
  useEffect(() => {
    setVisibleSnapshots(snapshotPageSize)
  }, [selectedId])
  useEffect(() => {
    setParams(
      (old) => {
        const next = new URLSearchParams(old)
        for (const [key, value] of Object.entries({
          repo: selectedId,
          page: showRepositorySettings ? 'settings' : '',
          mode: mode === 'live' ? 'watch' : mode,
          base,
          head,
          branch,
          baseBranch,
          headBranch,
        })) {
          if (value) next.set(key, value)
          else next.delete(key)
        }
        return next
      },
      { replace: true },
    )
  }, [selectedId, base, head, branch, baseBranch, headBranch, mode, showRepositorySettings, setParams])
  useEffect(() => {
    let stale = false
    setDataError('')
    setHistoryError('')
    if (!selectedId) {
      setSnapshots([])
      setMaps([])
      setHistory(null)
      return
    }
    setDataLoading(true)
    Promise.allSettled([
      api.repositories.snapshots(selectedId),
      api.repositories.maps(selectedId),
      api.repositories.history(selectedId, mode === 'pr' && pullRequest ? pullRequest.headSha : branch, 0),
    ])
      .then(([snapshotResult, mapResult, historyResult]) => {
        if (stale) return
        const nextSnapshots =
          snapshotResult.status === 'fulfilled' ? snapshotResult.value : []
        const nextHistory =
          historyResult.status === 'fulfilled' ? historyResult.value : null
        setSnapshots(nextSnapshots)
        setMaps(mapResult.status === 'fulfilled' ? mapResult.value : [])
        setHistory(nextHistory)
        if (
          snapshotResult.status === 'rejected' ||
          mapResult.status === 'rejected'
        )
          setDataError(
            String(
              snapshotResult.status === 'rejected'
                ? snapshotResult.reason
                : mapResult.status === 'rejected'
                  ? mapResult.reason
                  : '',
            ),
          )
        if (historyResult.status === 'rejected')
          setHistoryError(
            historyResult.reason instanceof Error
              ? historyResult.reason.message
              : 'Git history unavailable',
          )
        if (initialized.current !== selectedId) {
          initialized.current = selectedId
          const defaults = defaultRepositoryTargets(nextSnapshots, nextHistory)
          setBase(restored.current.base || defaults[0])
          setHead(restored.current.head || defaults[1])
          restored.current = { base: '', head: '' }
          const context = branch || nextHistory?.currentBranch || ''
          setBaseBranch((old) => old || context)
          setHeadBranch((old) => old || context)
        }
      })
      .finally(() => {
        if (!stale) setDataLoading(false)
      })
    return () => {
      stale = true
    }
  }, [selectedId, branch, nonce, mode, pullRequest])
  useEffect(() => {
    setComparison(null); setSelectedPath(''); setFilesTab('files')
    setOperationError('')
  }, [selectedId, base, head])
  useEffect(
    () => () => {
      operation.current?.abort()
    },
    [],
  )
  useEffect(() => {
    if (mode !== 'live' || !selectedId) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    let fetching = false
    const poll = async () => {
      if (controller.signal.aborted || fetching) return
      if (typeof document !== 'undefined' && document.hidden) return
      fetching = true
      try {
        if (!operation.current) {
          const [result, status] = await Promise.all([
            api.repositories.liveImpact(selectedId, controller.signal),
            api.repositories.watchStatus(selectedId, controller.signal).catch(() => null),
          ])
          if (!controller.signal.aborted && !operation.current) {
            setLive((old) => ({ ...result, diagram: result.diagram?.version === old?.diagram?.version ? old?.diagram ?? null : result.diagram }))
            if (status) setWatch(status)
            setOperationError('')
            if (result.diagram && result.diagram.version !== liveVersion.current) {
              liveVersion.current = result.diagram.version
              setNonce((n) => n + 1)
              void reload()
            }
          }
        }
      } catch (err) {
        if (!controller.signal.aborted) setOperationError(err instanceof Error ? err.message : 'Could not load live changes')
      } finally {
        fetching = false
        if (!controller.signal.aborted) timer = setTimeout(() => void poll(), 2000)
      }
    }
    const visibility = () => { if (!document.hidden) { clearTimeout(timer); void poll() } }
    void poll()
    if (typeof document !== 'undefined') document.addEventListener('visibilitychange', visibility)
    return () => { controller.abort(); clearTimeout(timer); if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', visibility) }
  }, [mode, selectedId, reload])
  const changeMode = (next: string) => {
    if (next === mode) return
    setPrListLoading(false)
    if (next === 'pr') {
      compareTargets.current = { base, head, baseBranch, headBranch }
      if (pullRequest) {
        setBase(`commit:${pullRequest.baseSha}`); setHead(`commit:${pullRequest.headSha}`)
        setBaseBranch(pullRequest.baseBranch); setHeadBranch(pullRequest.headBranch)
      }
    } else if (mode === 'pr' && compareTargets.current) {
      const previous = compareTargets.current
      setBase(previous.base); setHead(previous.head)
      setBaseBranch(previous.baseBranch); setHeadBranch(previous.headBranch)
      compareTargets.current = null
    }
    operation.current?.abort(); operation.current = null
    setBusy(false); setProgress(null); setComparison(null); setOperationError(''); setSelectedPath(''); setFilesTab('files'); setMode(next)
  }
  useEffect(() => {
    return () => { prListOperation.current?.abort(); prListOperation.current = null }
  }, [selectedId, mode])
  const loadOpenPullRequests = async () => {
    prListOperation.current?.abort()
    const controller = new AbortController()
    prListOperation.current = controller
    const repositoryId = selectedId
    setPrListLoading(true); setPrListError('')
    try {
      const items = await api.repositories.openPullRequests(repositoryId, controller.signal)
      if (!controller.signal.aborted && selectedRef.current === repositoryId) setOpenPullRequests(items)
    } catch (err) {
      if (!controller.signal.aborted && selectedRef.current === repositoryId) setPrListError(err instanceof Error ? err.message : 'Could not load open PRs')
    } finally {
      if (prListOperation.current === controller) { prListOperation.current = null; setPrListLoading(false) }
    }
  }
  const loadPullRequest = async (input = prInput) => {
    if (!selectedId || !input.trim() || busy) return
    const repositoryId = selectedId
    const controller = new AbortController()
    operation.current = controller
    setBusy(true); setOperationError(''); setComparison(null); setPullRequest(null)
    try {
      const result = await api.repositories.pullRequest(repositoryId, input.trim(), controller.signal)
      if (controller.signal.aborted || selectedRef.current !== repositoryId || operation.current !== controller) return
      setPullRequest(result)
      setBase(`commit:${result.baseSha}`); setHead(`commit:${result.headSha}`)
      setBaseBranch(result.baseBranch); setHeadBranch(result.headBranch)
    } catch (err) {
      if (!controller.signal.aborted && selectedRef.current === repositoryId) setOperationError(err instanceof Error ? err.message : 'Could not load pull request')
    } finally {
      if (operation.current === controller) { operation.current = null; setBusy(false) }
    }
  }
  const changeRadius = async (radius: number) => {
    if (!shownImpact || busy) return
    const nextRadius = Math.min(radius, 3)
    const repositoryId = selectedId
    const key = shownImpact.comparisonKey
    const controller = new AbortController()
    operation.current = controller; setBusy(true); setOperationError('')
    try {
      const result = await api.repositories.impactRadius(repositoryId, key, nextRadius, controller.signal)
      if (!controller.signal.aborted && selectedRef.current === repositoryId && operation.current === controller) {
        if (mode === 'live') setLive((old) => old ? { ...old, diagram: result } : old)
        else setComparison(result)
      }
    } catch (err) {
      if (!controller.signal.aborted && selectedRef.current === repositoryId) setOperationError(err instanceof Error ? err.message : 'Could not update blast radius')
    } finally { if (operation.current === controller) { operation.current = null; setBusy(false) } }
  }
  const startWatch = async () => {
    const repositoryId = selectedId
    setWatchBusy(true)
    setOperationError('')
    try {
      const status = await api.repositories.startWatch(repositoryId, { materialize: false })
      if (selectedRef.current === repositoryId) setWatch(status)
    } catch (err) {
      if (selectedRef.current === repositoryId) setOperationError(err instanceof Error ? err.message : 'Could not start watcher')
    } finally {
      setWatchBusy(false)
    }
  }
  const stopWatch = async () => {
    const repositoryId = selectedId
    setWatchBusy(true)
    setOperationError('')
    try {
      const status = await api.repositories.stopWatch(repositoryId)
      if (selectedRef.current === repositoryId) setWatch(status)
    } catch (err) {
      if (selectedRef.current === repositoryId) setOperationError(err instanceof Error ? err.message : 'Could not stop watcher')
    } finally {
      setWatchBusy(false)
    }
  }
  const restartWatch = async () => {
    const repositoryId = selectedId
    setWatchBusy(true); setOperationError('')
    try {
      const stopped = await api.repositories.stopWatch(repositoryId)
      if (selectedRef.current !== repositoryId) return
      setWatch(stopped)
      const started = await api.repositories.startWatch(repositoryId, { materialize: false })
      if (selectedRef.current === repositoryId) setWatch(started)
    } catch (err) {
      if (selectedRef.current === repositoryId) setOperationError(err instanceof Error ? err.message : 'Could not restart watcher')
    } finally { setWatchBusy(false) }
  }
  const refreshWatch = async () => {
    const repositoryId = selectedId
    setWatchBusy(true); setOperationError('')
    try {
      const [status, result] = await Promise.all([api.repositories.watchStatus(repositoryId), api.repositories.liveImpact(repositoryId)])
      if (selectedRef.current === repositoryId) { setWatch(status); setLive(result) }
    } catch (err) {
      if (selectedRef.current === repositoryId) setOperationError(err instanceof Error ? err.message : 'Could not refresh watcher')
    } finally { setWatchBusy(false) }
  }
  const selectRepo = (id: string) => {
    setShowRepositorySettings(false)
    if (id === selectedId) return
    operation.current?.abort()
    operation.current = null
    setBusy(false)
    setProgress(null)
    setSnapshots([])
    setMaps([])
    setLive(null)
    setPullRequest(null); setPrInput(''); compareTargets.current = null
    setOpenPullRequests(null); setPrListLoading(false); setPrListError('')
    setWatch(null)
    liveVersion.current = ''
    setHistory(null)
    setBase('')
    setHead('')
    setBranch('')
    setBaseBranch('')
    setHeadBranch('')
    setComparison(null); setSelectedPath(''); setFilesTab('files')
    initialized.current = ''
    restored.current = { base: '', head: '' }
    setSelectedId(id)
  }
  const chooseTarget = (side: 'base' | 'head', value: string) => {
    if (mode === 'pr') return
    const revision = (target: string) => target.startsWith('commit:') ? target.slice(7)
      : target === 'working_tree' ? history?.headSha : snapshotForTarget(target, snapshots)?.gitRevision
    const baseRevision = revision(side === 'base' ? value : base)
    const headRevision = revision(side === 'head' ? value : head)
    const baseIndex = history?.commits.findIndex((commit) => commit.sha === baseRevision) ?? -1
    const headIndex = history?.commits.findIndex((commit) => commit.sha === headRevision) ?? -1
    if (baseIndex >= 0 && headIndex >= 0 && baseIndex < headIndex) return

    const context = value.startsWith('commit:')
      ? branch || history?.currentBranch || ''
      : ''
    if (side === 'base') {
      setBase(value)
      setBaseBranch(context)
    } else {
      setHead(value)
      setHeadBranch(context)
    }
  }
  const run = async (kind: 'base' | 'head' | 'compare') => {
    if (!selected || busy || (mode === 'pr' && !pullRequest)) return
    const repositoryId = selected.id
    const controller = new AbortController()
    operation.current = controller
    setBusy(true)
    setOperationError('')
    setProgress(null)
    setComparison(null); setSelectedPath(''); setFilesTab('files')
    const isActive = () =>
      !controller.signal.aborted &&
      selectedRef.current === repositoryId &&
      operation.current === controller
    const map = (target: string, context: string) =>
      api.repositories.map(repositoryId, {
        ...targetMapOptions(target, context),
        signal: controller.signal,
        onProgress: (next) => {
          if (isActive()) setProgress(next)
        },
      })
    try {
      if (kind === 'compare') {
        const result = await api.repositories.compare(repositoryId, {
          base: targetMapOptions(base, baseBranch), head: targetMapOptions(head, headBranch),
          signal: controller.signal, onProgress: (next) => { if (isActive()) setProgress(next) },
        })
        if (isActive()) setComparison(result)
      } else {
        await map(
          kind === 'base' ? base : head,
          kind === 'base' ? baseBranch : headBranch,
        )
        if (isActive()) toast({ title: 'Repository mapped', status: 'success' })
      }
    } catch (err) {
      if (isActive())
        setOperationError(err instanceof Error ? err.message : 'Mapping failed')
    } finally {
      if (
        selectedRef.current === repositoryId &&
        operation.current === controller
      ) {
        setBusy(false)
        setProgress(null)
        operation.current = null
        setNonce((n) => n + 1)
        void reload()
      }
    }
  }
  const currentViewId = maps[0]?.result.viewId
  const handleDelete = async () => {
    if (!repoToDelete) return
    setDeletingRepo(true)
    try {
      await api.repositories.delete(repoToDelete.id, { deleteMaterialized, deleteClone })
      if (repoToDelete.id === selectedId) selectRepo('')
      setRepoToDelete(null)
      setDeleteMaterialized(false)
      setDeleteClone(false)
      await reload()
    } catch (err) {
      toast({
        title: 'Delete failed',
        description: err instanceof Error ? err.message : '',
        status: 'error',
      })
    } finally {
      setDeletingRepo(false)
    }
  }
  const handleDeleteSnapshot = async () => {
    if (!snapshotToDelete) return
    setDeletingSnapshot(true)
    try {
      await api.repositories.deleteSnapshot(snapshotToDelete.id)
      if (snapshotToDelete.id === base) setBase('')
      if (snapshotToDelete.id === head) setHead('')
      setSnapshotToDelete(null)
      setNonce((n) => n + 1)
      await reload()
    } catch (err) {
      toast({
        title: 'Delete failed',
        description: err instanceof Error ? err.message : '',
        status: 'error',
      })
    } finally {
      setDeletingSnapshot(false)
    }
  }
  const missingIndexers = useMemo(
    () => addRequirements?.indexers.filter((indexer) => !indexer.installed) ?? [],
    [addRequirements],
  )
  const checkAddIndexers = async (): Promise<RepositoryIndexerCheck | null> => {
    const value = addPath.trim()
    if (!value) {
      setAddError('Enter a repository path or URL')
      return null
    }
    setCheckingIndexers(true)
    setAddError('')
    try {
      const check = await api.repositories.checkIndexers(
        parseRepositoryAddInput(value),
      )
      setAddRequirements(check)
      return check
    } catch (err) {
      setAddRequirements(null)
      setAddError(
        err instanceof Error ? err.message : 'Could not check required indexers',
      )
      return null
    } finally {
      setCheckingIndexers(false)
    }
  }
  const handleAddRepository = async () => {
    if (adding || checkingIndexers) return
    const value = addPath.trim()
    if (!value) {
      setAddError('Enter a repository path or URL')
      return
    }
    if (missingIndexers.length > 0) {
      await checkAddIndexers()
      return
    }
    const check = addRequirements?.ready ? addRequirements : await checkAddIndexers()
    if (!check?.ready) return
    setAdding(true)
    setAddError('')
    setAddProgress(null)
    try {
      const added = await api.repositories.add(
        parseRepositoryAddInput(value),
        { materialize: addMap, onProgress: setAddProgress },
      )
      setAddOpen(false)
      setAddPath('')
      setAddProgress(null)
      setAddRequirements(null)
      setAddMap(true)
      toast({
        title: addMap
          ? 'Repository added and mapped'
          : 'Repository added',
        description: value,
        status: 'success',
      })
      await reload()
      setSelectedId(added.id)
      if (addWatch) {
        try {
          await api.repositories.startWatch(added.id, {
            materialize: addMap,
          })
        } catch (err) {
          toast({
            title: 'Repository added without a watcher',
            description: err instanceof Error ? err.message : '',
            status: 'warning',
          })
        }
      }
      setAddWatch(false)
    } catch (err) {
      setAddError(
        err instanceof Error ? err.message : 'Could not add repository',
      )
    } finally {
      setAdding(false)
    }
  }
  const repositoryDetails = (
                  <Box px={4} pb={3}>
                    <VStack align="stretch" spacing={2} mb={4}>
                  <RepositoryTargetPicker
                    aria-label="History branch"
                    value={branch}
                    isDisabled={busy || mode === 'pr' || !history?.isGit}
                    onChange={setBranch}
                    groups={[{ options: [
                      { value: '', label: `Current HEAD${history?.currentBranch ? ` · ${history.currentBranch}` : ''}` },
                      ...(branch && !history?.branches.some((b) => b.name === branch) ? [{ value: branch, label: branch }] : []),
                      ...(history?.branches ?? []).map((b) => ({ value: b.name, label: b.name })),
                    ] }]}
                  />
                  <Button
                    size="xs"
                    variant="outline"
                    isDisabled={!currentViewId || busy}
                    onClick={() => navigate(`/views/${currentViewId}`)}
                  >
                    Open map
                  </Button>
                    </VStack>
                    <Box mb={2}>
                      <Label>Snapshots · {snapshots.length}</Label>
                    </Box>
                    {dataLoading && !snapshots.length && <Spinner size="xs" />}
                    {!dataLoading && !snapshots.length && (
                      <Text fontSize="xs" color="gray.500">
                        No snapshots recorded.
                      </Text>
                    )}
                    <VStack align="stretch" spacing={1}>
                      {visibleSnapshotList.map((s) => (
                        <Box
                          key={s.id}
                          title={
                            s.provenance === 'commit'
                              ? 'Commit snapshot'
                              : s.provenance === 'working_tree'
                                ? 'Working tree snapshot'
                                : 'Unknown provenance'
                          }
                          position="relative"
                          px={2}
                          py={1.5}
                          bg="whiteAlpha.50"
                          borderRadius="md"
                          border="1px solid"
                          borderColor="whiteAlpha.100"
                        >
                          <Flex
                            gap={2}
                            align="center"
                            sx={{
                              '&:hover > .snapshot-delete, &:focus-within > .snapshot-delete':
                                { opacity: 1, pointerEvents: 'auto' },
                              '@media (hover: none)': {
                                '> .snapshot-delete': {
                                  opacity: 1,
                                  pointerEvents: 'auto',
                                },
                              },
                            }}
                          >
                            <Text
                              fontSize="xs"
                              color="gray.300"
                              flex={1}
                              isTruncated
                              title={s.gitBranch || short(s.gitRevision)}
                            >
                              {s.gitBranch || 'No captured branch'}
                            </Text>
                            <Text
                              fontSize="10px"
                              color="gray.500"
                              whiteSpace="nowrap"
                              title={new Date(
                                s.createdUnix * 1000,
                              ).toLocaleString()}
                            >
                              {age(s.createdUnix)}
                            </Text>
                            {s.ingestionStatus !== 'complete' && (
                              <Badge fontSize="2xs" colorScheme="orange">
                                {s.ingestionStatus || 'Incomplete'}
                              </Badge>
                            )}
                            <IconButton
                              aria-label={`Delete snapshot ${s.id}`}
                              data-testid={`repositories-snapshot-delete-${s.id}`}
                              className="snapshot-delete"
                              icon={<DeleteIcon boxSize="12px" />}
                              position="absolute"
                              top="50%"
                              right="6px"
                              transform="translateY(-50%)"
                              size="xs"
                              variant="ghost"
                              color="red.400"
                              bg="var(--bg-element)"
                              _hover={{ bg: 'var(--bg-element)' }}
                              _active={{ bg: 'var(--bg-element)' }}
                              borderRadius="md"
                              opacity={0}
                              pointerEvents="none"
                              _focusVisible={{
                                opacity: 1,
                                pointerEvents: 'auto',
                              }}
                              isDisabled={deletingSnapshot}
                              onClick={(e) => {
                                e.stopPropagation()
                                setSnapshotToDelete(s)
                              }}
                            />
                          </Flex>
                          {s.commitMessage && (
                            <Text
                              fontSize="xs"
                              color="gray.200"
                              mt={1}
                              isTruncated
                              title={s.commitMessage}
                            >
                              {s.commitMessage}
                            </Text>
                          )}
                          {s.statistics ? (
                            <Text fontSize="10px" color="gray.400" mt={1}>
                              {s.statistics.facts.toLocaleString()} facts ·{' '}
                              {s.statistics.edges.toLocaleString()} edges ·{' '}
                              {s.statistics.sources.toLocaleString()} files ·{' '}
                              {s.statistics.chunks.toLocaleString()} chunks
                            </Text>
                          ) : (
                            <Text fontSize="10px" color="gray.500" mt={1}>
                              Statistics unavailable
                            </Text>
                          )}
                          <Box as="details" mt={2} fontSize="xs" color="gray.400">
                            <Text as="summary" cursor="pointer">Snapshot details</Text>
                            <Text mt={2} overflowWrap="anywhere">ID: {s.id}</Text>
                            <Text overflowWrap="anywhere">Revision: {s.gitRevision || '—'} · {s.provenance || 'Unknown provenance'}</Text>
                            <Text>Captured: {new Date(s.createdUnix * 1000).toLocaleString()} · {s.ingestionStatus || 'Unknown status'}</Text>
                            <Text overflowWrap="anywhere">Content fingerprint: {s.contentFingerprint || '—'}</Text>
                            {s.projects.map((project) => <Text key={`${project.root}:${project.configPath}`} overflowWrap="anywhere">{project.language} · {project.root} · {project.configPath}</Text>)}
                            {s.warnings.map((warning, index) => <Text key={index} color="orange.300" overflowWrap="anywhere">{warning}</Text>)}
                          </Box>
                        </Box>
                      ))}
                    </VStack>
                    {(visibleSnapshots < snapshots.length ||
                      visibleSnapshots > snapshotPageSize) && (
                      <HStack
                        data-testid="repositories-snapshots-pagination"
                        mt={2}
                        spacing={2}
                      >
                        {visibleSnapshots > snapshotPageSize && (
                          <Button
                            data-testid="repositories-snapshots-show-less"
                            size="xs"
                            variant="ghost"
                            flex={1}
                            color="gray.400"
                            onClick={collapseSnapshots}
                          >
                            Show newest {snapshotPageSize}
                          </Button>
                        )}
                        {visibleSnapshots < snapshots.length && (
                          <Button
                            data-testid="repositories-snapshots-load-more"
                            size="xs"
                            variant="ghost"
                            flex={1}
                            color="gray.400"
                            onClick={loadMoreSnapshots}
                          >
                            Load{' '}
                            {Math.min(
                              snapshotPageSize,
                              snapshots.length - visibleSnapshots,
                            )}{' '}
                            more
                          </Button>
                        )}
                      </HStack>
                    )}
                  </Box>
  )
  return (
    <Box
      h="full"
      bg="var(--bg-canvas)"
      display="flex"
      flexDir="column"
      overflow="hidden"
    >
      <ErrorMessage message={error} />
      {loading && !repositories.length ? (
        <Center flex={1}>
          <Spinner color="var(--accent)" />
        </Center>
      ) : (
        <Flex
          flex={1}
          minH={0}
          direction={{ base: 'column', lg: 'row' }}
          overflow="hidden"
        >
          <Box
            w={{ base: 'full', lg: collapsed ? '52px' : '320px' }}
            maxH={{ base: collapsed ? '96px' : '32vh', lg: 'none' }}
            display="flex"
            flexDir="column"
            flexShrink={0}
            borderRight="1px solid"
            borderBottom={{ base: '1px solid', lg: 'none' }}
            borderColor="whiteAlpha.100"
          >
            <Flex
              px={collapsed ? 0 : 3}
              h="44px"
              flexShrink={0}
              gap={2}
              align="center"
              justify={collapsed ? 'center' : undefined}
              borderBottom="1px solid"
              borderColor="whiteAlpha.100"
            >
              {!collapsed && (
                <>
                  <Label>Repositories</Label>
                  <Box flex={1} />
                </>
              )}
              <IconButton
                size="xs"
                variant="ghost"
                aria-label={
                  collapsed ? 'Expand repositories' : 'Collapse repositories'
                }
                icon={collapsed ? <ChevronRightIcon /> : <ChevronLeftIcon />}
                onClick={() => setCollapsed(!collapsed)}
              />
              {!collapsed && (
                <IconButton
                  aria-label="Reload repositories"
                  size="xs"
                  variant="ghost"
                  icon={<RepeatIcon />}
                  onClick={() => {
                    void reload()
                    setNonce((n) => n + 1)
                  }}
                />
              )}
            </Flex>
            <Box flex="0 1 auto" minH={0} overflowY="auto">
            {repositories.map((repo) => (
              <Box
                key={repo.id}
                role="group"
                borderBottom="1px solid"
                borderColor="whiteAlpha.100"
              >
                <Flex
                  position="relative"
                  px={collapsed ? 0 : 4}
                  py={3}
                  align="center"
                  justify={collapsed ? 'center' : undefined}
                  gap={3}
                  sx={{
                    '&:hover > .repository-settings, &:focus-within > .repository-settings':
                      { opacity: 1, pointerEvents: 'auto' },
                    '@media (hover: none)': {
                      '> .repository-settings': {
                        opacity: 1,
                        pointerEvents: 'auto',
                      },
                    },
                  }}
                >
                  <Button
                      p={0}
                      h={collapsed ? '28px' : undefined}
                      display="flex"
                      alignItems="center"
                      justifyContent="center"
                    minW="28px"
                    size="sm"
                    variant="unstyled"
                    aria-label={`Select ${repo.name || nameOf(repo.root)}`}
                    onClick={() => selectRepo(repo.id)}
                  >
                    <Glyph name={repo.name || nameOf(repo.root)} />
                  </Button>
                  {!collapsed && (
                    <>
                      <Box
                        flex={1}
                        minW={0}
                        onClick={() => selectRepo(repo.id)}
                        cursor="pointer"
                      >
                        <Text fontSize="sm" fontWeight="semibold" isTruncated>
                          {repo.name || nameOf(repo.root)}
                        </Text>
                        <Text
                          fontSize="xs"
                          color="gray.500"
                          isTruncated
                          title={repo.remoteUrl || repo.root}
                        >
                          {repo.remoteUrl
                            ? repo.remoteUrl.replace(/^https?:\/\//, '')
                            : repo.root}
                        </Text>
                      </Box>
                      <IconButton
                        data-testid={`repositories-settings-${repo.id}`}
                        aria-label={`Settings for ${repo.name || nameOf(repo.root)}`}
                        className="repository-settings"
                        icon={<SettingsIcon boxSize="12px" />}
                        position="absolute"
                        top="50%"
                        right="6px"
                        transform="translateY(-50%)"
                        size="xs"
                        variant="ghost"
                        color="gray.400"
                        bg="var(--bg-element)"
                        _hover={{ bg: 'var(--bg-element)' }}
                        _active={{ bg: 'var(--bg-element)' }}
                        borderRadius="md"
                        opacity={1}
                        pointerEvents="auto"
                        _focusVisible={{ opacity: 1, pointerEvents: 'auto' }}
                        isDisabled={busy}
                        onClick={(e) => {
                          e.stopPropagation()
                          selectRepo(repo.id)
                          setShowRepositorySettings(true)
                        }}
                      />
                    </>
                  )}
                </Flex>

              </Box>
            ))}
            </Box>
            <Box flexShrink={0} borderTop="1px solid" borderColor="whiteAlpha.100">
              <Popover
                isOpen={addOpen}
                onOpen={() => {
                  setAddError('')
                  setAddProgress(null)
                  setAddRequirements(null)
                }}
                onClose={() => {
                  if (!adding) setAddOpen(false)
                }}
                placement="right-start"
                isLazy
                closeOnBlur={!adding && !checkingIndexers}
                returnFocusOnClose={false}
              >
                <PopoverTrigger>
              <Flex
                role="button"
                tabIndex={0}
                px={collapsed ? 0 : 4}
                py={3}
                align="center"
                justify={collapsed ? 'center' : undefined}
                gap={3}
                cursor="pointer"
                data-testid="repositories-add"
                _hover={{ bg: 'whiteAlpha.50' }}
                onClick={() => {
                  setAddError('')
                  setAddProgress(null)
                  setAddOpen(true)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    setAddError('')
                    setAddProgress(null)
                    setAddOpen(true)
                  }
                }}
              >
                <Center
                  w="28px"
                  h="28px"
                  flexShrink={0}
                  border="1px dashed"
                  borderColor="whiteAlpha.400"
                  borderRadius="md"
                  color="gray.400"
                >
                  <AddIcon boxSize="12px" />
                </Center>
                {!collapsed && (
                  <Box flex={1} minW={0}>
                    <Text fontSize="sm" fontWeight="semibold" color="gray.300">
                      Add repository
                    </Text>
                    <Text fontSize="xs" color="gray.500" isTruncated>
                      Index a local or remote repository
                    </Text>
                  </Box>
                )}
              </Flex>
                </PopoverTrigger>
                <Portal>
                  <PopoverContent w="320px" maxW="calc(100vw - 24px)">
                    <PopoverArrow />
                    <PopoverCloseButton isDisabled={adding} />
                    <PopoverHeader fontWeight="semibold">
                      Add repository
                    </PopoverHeader>
                    <PopoverBody>
                      <FormControl>
                        <Text fontSize="sm" mb={2} color="gray.400">
                          Index a local repository directory or clone a remote
                          repository (owner/repo or Git URL).
                        </Text>
                        <Input
                          autoFocus
                          size="sm"
                          placeholder="/path/to/repository or owner/repo"
                          value={addPath}
                          data-testid="repositories-add-path"
                          isDisabled={adding}
                          onChange={(e) => {
                            setAddPath(e.target.value)
                            setAddRequirements(null)
                            setAddError('')
                          }}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') void handleAddRepository()
                          }}
                        />
                      </FormControl>
                      {(adding || checkingIndexers) && (
                        <Box
                          mt={3}
                          p={3}
                          borderRadius="md"
                          bg="whiteAlpha.50"
                          data-testid="repositories-add-status"
                        >
                          <HStack spacing={2}>
                            <Spinner size="xs" color="var(--accent)" />
                            <Text
                              fontSize="sm"
                              fontWeight="semibold"
                              aria-live="polite"
                            >
                              {checkingIndexers
                                ? 'Checking required indexers…'
                                : indexStageLabel(addProgress?.stage || '') ||
                                  'Preparing index…'}
                            </Text>
                            {!!addProgress?.total && !checkingIndexers && (
                              <Text ml="auto" fontSize="xs" color="gray.400">
                                {addProgress.current}/{addProgress.total}
                              </Text>
                            )}
                          </HStack>
                          <Progress
                            mt={2}
                            size="xs"
                            borderRadius="full"
                            isIndeterminate={
                              checkingIndexers || !addProgress?.total
                            }
                            value={
                              addProgress?.total
                                ? (addProgress.current / addProgress.total) *
                                  100
                                : undefined
                            }
                          />
                          {addProgress?.detail && !checkingIndexers && (
                            <Text
                              mt={2}
                              fontSize="xs"
                              color="gray.500"
                              isTruncated
                              title={addProgress.detail}
                            >
                              {addProgress.detail}
                            </Text>
                          )}
                        </Box>
                      )}
                      {addError && (
                        <Alert status="error" mt={3} borderRadius="md">
                          <AlertIcon />
                          <Text fontSize="sm">{addError}</Text>
                        </Alert>
                      )}
                      {missingIndexers.length > 0 && (
                        <Alert
                          status="warning"
                          mt={3}
                          borderRadius="md"
                          alignItems="flex-start"
                          data-testid="repositories-add-indexers"
                        >
                          <AlertIcon />
                          <Box minW={0} flex={1}>
                            <Text fontSize="sm" fontWeight="semibold">
                              Install required indexers
                            </Text>
                            <Text fontSize="xs" mt={1} color="gray.600">
                              This repository needs these tools before it can be
                              indexed.
                            </Text>
                            <VStack
                              mt={2}
                              spacing={2}
                              align="stretch"
                              maxH="180px"
                              overflowY="auto"
                            >
                              {missingIndexers.map((indexer) => (
                                <Box key={indexer.tool}>
                                  <Text fontSize="xs" fontWeight="semibold">
                                    {indexer.tool}
                                  </Text>
                                  <Text
                                    as="code"
                                    display="block"
                                    mt={0.5}
                                    px={2}
                                    py={1}
                                    borderRadius="sm"
                                    bg="blackAlpha.100"
                                    fontSize="xs"
                                    wordBreak="break-all"
                                  >
                                    {indexer.installHint}
                                  </Text>
                                </Box>
                              ))}
                            </VStack>
                          </Box>
                        </Alert>
                      )}
                      <HStack mt={4} align="flex-start">
                        <Switch
                          size="sm"
                          data-testid="repositories-add-map"
                          isChecked={addMap}
                          isDisabled={adding}
                          onChange={(e) => setAddMap(e.target.checked)}
                        />
                        <Box>
                          <Text fontSize="sm">Map into workspace</Text>
                          <Text fontSize="xs" color="gray.500">
                            Group this repository into a map view with components
                            and dependencies.
                          </Text>
                        </Box>
                      </HStack>
                      <HStack mt={3} align="flex-start">
                        <Switch
                          size="sm"
                          data-testid="repositories-add-watch"
                          isChecked={addWatch}
                          isDisabled={adding}
                          onChange={(e) => setAddWatch(e.target.checked)}
                        />
                        <Box>
                          <Text fontSize="sm">Watch for changes</Text>
                          <Text fontSize="xs" color="gray.500">
                            Keep the repository updated as you work. Requires
                            Git.
                          </Text>
                        </Box>
                      </HStack>
                    </PopoverBody>
                    <PopoverFooter
                      display="flex"
                      justifyContent="flex-end"
                      gap={2}
                    >
                      <Button
                        size="sm"
                        variant="ghost"
                        isDisabled={adding}
                        onClick={() => setAddOpen(false)}
                      >
                        Cancel
                      </Button>
                      {missingIndexers.length > 0 && (
                        <Button
                          size="sm"
                          variant="outline"
                          data-testid="repositories-add-recheck"
                          isLoading={checkingIndexers}
                          onClick={() => void checkAddIndexers()}
                        >
                          Re-check
                        </Button>
                      )}
                      <Button
                        size="sm"
                        style={accentStyle}
                        data-testid="repositories-add-submit"
                        isLoading={adding}
                        isDisabled={
                          missingIndexers.length > 0 || checkingIndexers
                        }
                        onClick={() => void handleAddRepository()}
                      >
                        Add repository
                      </Button>
                    </PopoverFooter>
                  </PopoverContent>
                </Portal>
              </Popover>
            </Box>
          </Box>
          <Box
            flex={1}
            minW={0}
            minH={0}
            overflowY="auto"
            display="flex"
            flexDir="column"
          >
            {selected && (showRepositorySettings ? (
              <RepositorySettings key={selected.id} repository={selected} snapshots={snapshots} maps={maps} history={history} busy={busy} dataError={dataError || historyError} onBack={() => setShowRepositorySettings(false)} onDelete={() => { setRepoToDelete(selected); setDeleteMaterialized(false); setDeleteClone(selected.managed) }} onUpdated={() => { setNonce(n => n + 1); void reload() }}>
                {repositoryDetails}
              </RepositorySettings>
            ) : (
              <>
                <Flex
                  px={2}
                  py={0}
                  h="44px"
                  flexShrink={0}
                  gap={3}
                  align="center"
                  justify="center"
                  wrap="wrap"
                  borderBottom="1px solid"
                  borderColor="whiteAlpha.100"
                >
                  <HStack spacing={0.5} p={0.5} bg="blackAlpha.200" border="1px solid" borderColor="whiteAlpha.50" borderRadius="lg" aria-label="Repository mode">
                    {([['compare', 'Compare'], ['live', 'Watch'], ['pr', 'PR Review']] as const).map(([value, label]) => (
                      <Button key={value} size="sm" variant="ghost" borderRadius="md" px={3} h="28px" minW="auto" leftIcon={<RepositoryModeIcon mode={value} />} iconSpacing={1.5} fontSize="11px" fontWeight="semibold" bg={mode === value ? 'var(--bg-element)' : 'transparent'} color={mode === value ? 'white' : 'gray.500'} _hover={{ bg: mode === value ? 'var(--bg-element)' : 'whiteAlpha.50' }} _active={{ bg: 'var(--bg-element)' }} transition="color 0.2s" data-testid={`repositories-${value}-tab`} aria-pressed={mode === value} onClick={() => changeMode(value)}>{label}</Button>
                    ))}
                  </HStack>
                </Flex>
                {mode !== 'live' && historyError && (
                  <Text p={3} fontSize="xs" color="orange.300">
                    {historyError} · Saved snapshots remain available.
                  </Text>
                )}
                <ErrorMessage message={dataError} />
                {mode === 'pr' && (
                  <RepositoryPullRequestPanel
                    repositoryUrl={history?.repositoryUrl}
                    input={prInput}
                    requests={openPullRequests}
                    selected={pullRequest}
                    busy={busy}
                    loading={prListLoading}
                    error={prListError}
                    onInput={setPrInput}
                    onSelect={(value) => { setPrInput(value); if (value) void loadPullRequest(value) }}
                    onLoad={() => void loadPullRequest()}
                    onRefresh={() => void loadOpenPullRequests()}
                  />
                )}
                {(mode === 'compare' || (mode === 'pr' && pullRequest)) && <RepositoryHistory
                  repositoryId={selectedId}
                  history={history}
                  base={
                    base.startsWith('commit:')
                      ? base.slice(7)
                      : snapshotForTarget(base, snapshots)?.gitRevision || ''
                  }
                  head={
                    head.startsWith('commit:')
                      ? head.slice(7)
                      : snapshotForTarget(head, snapshots)?.gitRevision || ''
                  }
                  disabled={busy || mode === 'pr'}
                  onRange={(older, newer) => {
                    if (busy || mode === 'pr') return
                    const context = branch || history?.currentBranch || ''
                    setBase(`commit:${older.sha}`)
                    setHead(`commit:${newer.sha}`)
                    setBaseBranch(context)
                    setHeadBranch(context)
                  }}
                  collapsed={historyCollapsed}
                  onToggle={() => setHistoryCollapsed(!historyCollapsed)}
                />}
                {shownImpact && <Flex
                  px={4}
                  h="40px"
                  align="center"
                  gap={3}
                  flexShrink={0}
                  borderBottom="1px solid"
                  borderColor="whiteAlpha.100"
                >
                  {shownImpact && (
                    <HStack
                      spacing={1}
                      align="center"
                      data-testid="repositories-radius"
                    >
                      <Text fontSize="xs" color="gray.500">
                        Radius
                      </Text>
                      {Array.from(
                        { length: Math.min(3, shownImpact.maxRadius) + 1 },
                        (_, r) => (
                          <Button
                            key={r}
                            data-testid={`repositories-radius-${r}`}
                            size="xs"
                            variant={shownImpact.radius === r ? 'solid' : 'ghost'}
                            isDisabled={busy}
                            onClick={() => {
                              if (r !== shownImpact.radius) void changeRadius(r)
                            }}
                          >
                            {r}
                          </Button>
                        ),
                      )}
                    </HStack>
                  )}
                </Flex>}
                {mode === 'live' && (
                  <>
                    <RepositoryWatcherPanel status={watch} repositoryRoot={selected.root} branch={watch?.gitBranch || live?.gitBranch || ''} revision={watch?.gitRevision || live?.gitRevision || ''} busy={watchBusy} onStart={() => void startWatch()} onStop={() => void stopWatch()} onRestart={() => void restartWatch()} onRefresh={() => void refreshWatch()} />
                    <ErrorMessage message={live?.error || ''} />
                  </>
                )}
                {(mode === 'compare' || (mode === 'pr' && pullRequest)) && (
                  <Box
                    p={4}
                    borderBottom="1px solid"
                    borderColor="whiteAlpha.100"
                  >
                    <Grid
                      templateColumns={{ base: '1fr', md: '1fr 1fr' }}
                      gap={3}
                    >
                      <CompareSide
                        side="Base"
                        value={base}
                        branch={baseBranch}
                        snapshots={snapshots}
                        maps={maps}
                        history={history}
                        showIds={showIds}
                        disabled={busy || dataLoading}
                        locked={mode === 'pr'}
                        onChange={(value) => chooseTarget('base', value)}
                        onMap={() => void run('base')}
                      />
                      <CompareSide
                        side="Head"
                        value={head}
                        branch={headBranch}
                        snapshots={snapshots}
                        maps={maps}
                        history={history}
                        showIds={showIds}
                        disabled={busy || dataLoading}
                        locked={mode === 'pr'}
                        onChange={(value) => chooseTarget('head', value)}
                        onMap={() => void run('head')}
                      />
                    </Grid>
                    <Flex mt={3} gap={3} align="center" wrap="wrap">
                      <Box flex={1} />
                      <Button
                        {...accentStyle}
                        size="sm"
                        data-testid="repositories-compare"
                        isLoading={busy}
                        loadingText="Comparing…"
                        isDisabled={!base || !head || dataLoading}
                        onClick={() => void run('compare')}
                      >
                        Compare maps
                      </Button>
                      {busy && (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => {
                            operation.current?.abort()
                          }}
                        >
                          Cancel
                        </Button>
                      )}
                    </Flex>
                  </Box>
                )}
                {busy && (
                  <Box p={3}>
                    <Text fontSize="xs" color="gray.400" mb={2}>
                      {progress
                        ? `${progress.stage} · ${progress.detail}${progress.total ? ` · ${progress.current}/${progress.total}` : ''}`
                        : 'Preparing maps…'}
                    </Text>
                    <Progress
                      size="xs"
                      isIndeterminate={!progress?.total}
                      value={
                        progress?.total
                          ? (progress.current / progress.total) * 100
                          : undefined
                      }
                    />
                  </Box>
                )}
                <ErrorMessage message={operationError} />

                <Flex
                  flex={1}
                  minH="260px"
                  direction={{ base: 'column', lg: 'row' }}
                >
                  <Box
                    w={{ base: 'full', lg: '280px' }}
                    position="relative"
                    flexShrink={0}
                    borderRight="1px solid"
                    borderBottom={{ base: '1px solid', lg: 'none' }}
                    borderColor="whiteAlpha.100"
                  >
                    <Flex p={2} gap={1} role="tablist" aria-label="Repository details">
                      <Button size="xs" role="tab" aria-selected={filesTab === 'files'} variant={filesTab === 'files' ? 'solid' : 'ghost'} onClick={() => setFilesTab('files')}>Files <Badge ml={2} fontSize="2xs">{diff?.sources.length ?? 0}</Badge></Button>
                      <Button size="xs" role="tab" aria-selected={filesTab === 'symbols'} variant={filesTab === 'symbols' ? 'solid' : 'ghost'} onClick={() => setFilesTab('symbols')}>Symbols <Badge ml={2} fontSize="2xs" aria-label="Changed symbols count">{diff ? diff.facts.added + diff.facts.removed + diff.facts.modified : 0}</Badge></Button>
                    </Flex>
                    <Box position={{ base: 'relative', lg: 'absolute' }} top={{ lg: '40px' }} bottom={{ lg: 0 }} w="full" overflowY="auto">
                    {filesTab === 'symbols' ? (
                      <RepositorySymbols key={`${selectedId}:${selectedPath}:${diff?.fromSnapshotId}:${diff?.toSnapshotId}`} repositoryRoot={selected.root} path={selectedPath} diagram={shownImpact} />
                    ) : diff ? (
                      <FileTree files={diff.sources} onSelect={setSelectedPath} diagram={shownImpact!} repositoryRoot={selected.root} />
                    ) : (
                      <Text px={3} fontSize="xs" color="gray.500">
                        {mode === 'pr' && !pullRequest ? 'Choose a PR to see its changed files.' : 'Compare maps to see changed source files.'}
                      </Text>
                    )}
                    </Box>
                  </Box>
                  <RepositoryChangeCanvas key={`${selectedId}:${mode}:${shownImpact?.comparisonKey ?? ''}`} diagram={shownImpact} selectedPath={selectedPath} repositoryRoot={selected.root} emptyMessage={mode === 'pr' ? pullRequest ? 'Compare the PR maps to overlay changes on the workspace.' : 'Select an open PR or enter its number or URL to start a review.' : mode === 'live' ? 'Waiting for the watcher to prepare the live map.' : undefined} />
                </Flex>
              </>
            ))}
          </Box>
        </Flex>
      )}
      <ConfirmDialog
        isOpen={!!repoToDelete}
        onClose={() => {
          if (!deletingRepo) setRepoToDelete(null)
        }}
        onConfirm={() => void handleDelete()}
        title="Delete repository"
        body={
          repoToDelete
            ? `Delete "${nameOf(repoToDelete.root)}" and all of its indexed snapshots? This cannot be undone.`
            : ''
        }
        confirmLabel="Delete"
        confirmColorScheme="red"
        isLoading={deletingRepo}
      >
        <HStack mt={4} align="flex-start">
          <Switch
            size="sm"
            colorScheme="red"
            data-testid="repositories-delete-materialized"
            isChecked={deleteMaterialized}
            isDisabled={deletingRepo}
            onChange={(e) => setDeleteMaterialized(e.target.checked)}
          />
          <Box>
            <Text fontSize="sm">
              Also delete materialized workspace resources
            </Text>
            <Text fontSize="xs" color="gray.500">
              Removes views, elements, and connectors created by Map for this
              repository.
            </Text>
          </Box>
        </HStack>
        {repoToDelete?.managed && (
          <HStack mt={3} align="flex-start">
            <Switch
              size="sm"
              colorScheme="red"
              data-testid="repositories-delete-clone"
              isChecked={deleteClone}
              isDisabled={deletingRepo}
              onChange={(e) => setDeleteClone(e.target.checked)}
            />
            <Box>
              <Text fontSize="sm">Also delete the cloned checkout</Text>
              <Text fontSize="xs" color="gray.500">
                Removes the tld-managed clone from the data directory.
              </Text>
            </Box>
          </HStack>
        )}
      </ConfirmDialog>
      <ConfirmDialog
        isOpen={!!snapshotToDelete}
        onClose={() => {
          if (!deletingSnapshot) setSnapshotToDelete(null)
        }}
        onConfirm={() => void handleDeleteSnapshot()}
        title="Delete snapshot"
        body={
          snapshotToDelete
            ? `Delete the snapshot from "${snapshotToDelete.gitBranch || 'no captured branch'}" taken ${age(snapshotToDelete.createdUnix)}? This cannot be undone.`
            : ''
        }
        confirmLabel="Delete"
        confirmColorScheme="red"
        isLoading={deletingSnapshot}
      />
    </Box>
  )
}
