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
  SettingsIcon,
} from '@chakra-ui/icons'
import {
  faCamera,
  faCodeCompare,
  faCodePullRequest,
  faEye,
  faFolder,
  faFolderOpen,
  faFolderTree,
  type IconDefinition,
} from '@fortawesome/free-solid-svg-icons'
import {
  api,
  type CodeSnapshot,
  type CompletedRepositoryMap,
  type IndexedRepository,
  type RepositoryGitHistory,
  type RepositoryPullRequest,
  type OpenRepositoryPullRequest,
  type RepositoryIndexerCheck,
  type RepositoryMapProgress,
  type SnapshotDiff,
  type RepositoryImpact as RepositoryImpactResult,
  type LiveRepositoryImpact,
  type RepositoryWatchStatus,
  type SnapshotSourceChange,
} from '../api/client'
import ConfirmDialog from '../components/ConfirmDialog'
import RepositorySnapshotsPanel from '../components/RepositorySnapshotsPanel'
import '../styles/editor-panels.css'
import RepositorySettings from './RepositorySettings'
import RepositoryHistory from '../components/RepositoryHistory'
import RepositoryTargetPicker from '../components/RepositoryTargetPicker'
import RepositoryChangeCanvas from '../components/RepositoryChangeCanvas'
import RepositorySymbols from '../components/RepositorySymbols'
import RepositoryWatcherPanel from '../components/RepositoryWatcherPanel'
import RepositoryPullRequestPanel from '../components/RepositoryPullRequestPanel'
import ColumnResizeHandle from '../components/ColumnResizeHandle'
import {
  PANEL_COLLAPSE_SNAP,
  PANEL_RAIL_WIDTH,
  useResizableColumn,
} from '../hooks/useResizableColumn'
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
const FILES_PANEL_DEFAULT_WIDTH = 280
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
function targetSummary(value: string, snapshots: CodeSnapshot[]) {
  if (!value) return 'Not selected'
  if (value === 'working_tree') return 'Working tree'
  if (value.startsWith('commit:')) return short(value.slice(7))
  const snapshot = snapshotForTarget(value, snapshots)
  if (snapshot) return snapshot.gitBranch || short(snapshot.gitRevision)
  return value.startsWith('snapshot:') ? short(value.slice(9)) : value
}
function targetStatus(value: string, snapshots: CodeSnapshot[], maps: CompletedRepositoryMap[]) {
  const snapshot = snapshotForTarget(value, snapshots)
  const mapped = !!snapshot && maps.some((map) => map.result.snapshotId === snapshot.id)
  return mapped
    ? { label: 'Mapped', colorScheme: 'green' }
    : snapshot
      ? { label: 'Indexed', colorScheme: 'blue' }
      : { label: 'Not captured', colorScheme: 'gray' }
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
function RepositoryModeIcon({ mode }: { mode: 'snapshots' | 'compare' | 'live' | 'pr' }) {
  const icon = mode === 'snapshots' ? faCamera : mode === 'compare' ? faCodeCompare : mode === 'live' ? faEye : faCodePullRequest
  return <SolidIcon icon={icon} />
}
function SolidIcon({ icon, size = 13 }: { icon: IconDefinition; size?: number }) {
  const [width, height, , , pathData] = icon.icon
  const paths = Array.isArray(pathData) ? pathData : [pathData]
  return (
    <svg width={size} height={size} viewBox={`0 0 ${width} ${height}`} fill="currentColor" aria-hidden="true" style={{ flexShrink: 0 }}>
      {paths.map((path, index) => <path key={index} d={path} />)}
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
  if (directory) {
    const icon = expanded ? faFolderOpen : faFolder
    const [width, height, , , pathData] = icon.icon
    const paths = Array.isArray(pathData) ? pathData : [pathData]
    return (
      <svg width="16" height="16" viewBox={`0 0 ${width} ${height}`} fill="currentColor" aria-hidden="true" style={{ flexShrink: 0 }}>
        {paths.map((path, index) => <path key={index} d={path} />)}
      </svg>
    )
  }
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true" style={{ flexShrink: 0 }}>
      <g stroke={change === 'added' ? '#a6e22e' : change === 'removed' ? '#ff656d' : '#ffb340'} strokeWidth="2.5">
        <rect x="3" y="3" width="18" height="18" rx="2" />
        {change === 'added' ? <path d="M8 12h8M12 8v8" /> : change === 'removed' ? <path d="M8 12h8" /> : <circle cx="12" cy="12" r="1.5" fill="#ffb340" />}
      </g>
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

function FilesCollapsedRail({ fileCount, symbolCount, onExpand }: {
  fileCount: number
  symbolCount: number
  onExpand: () => void
}) {
  return (
    <Tooltip label="Expand files and symbols" placement="right" openDelay={200}>
      <Flex
        as="button"
        type="button"
        onClick={onExpand}
        role="button"
        aria-expanded={false}
        aria-label={`Expand files and symbols panel, ${fileCount} files, ${symbolCount} symbols`}
        title={`${fileCount} files · ${symbolCount} symbols`}
        w={`${PANEL_RAIL_WIDTH}px`}
        flexShrink={0}
        gap={4}
        py={2}
        align="center"
        justify="center"
        flexDir="column"
        borderRight="1px solid"
        borderBottom={{ base: '1px solid', lg: 'none' }}
        borderColor="whiteAlpha.100"
        bg="transparent"
        color="gray.500"
        _hover={{ bg: 'whiteAlpha.50', color: 'gray.300' }}
        _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '-2px' }}
        data-testid="repositories-files-collapsed"
      >
        <SolidIcon icon={faFolderTree} size={20} />
      </Flex>
    </Tooltip>
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
  const [capturing, setCapturing] = useState(false)
  const [progress, setProgress] = useState<RepositoryMapProgress | null>(null)
  const [comparison, setComparison] = useState<RepositoryImpactResult | null>(null)
  const [live, setLive] = useState<LiveRepositoryImpact | null>(null)
  const [watch, setWatch] = useState<RepositoryWatchStatus | null>(null)
  const [watchBusy, setWatchBusy] = useState(false)
  const [mode, setMode] = useState(() => params.get('mode') === 'snapshots' ? 'snapshots' : params.get('mode') === 'live' || params.get('mode') === 'watch' ? 'live' : params.get('mode') === 'pr' ? 'pr' : 'compare')
  const [watchEnabled, setWatchEnabled] = useState(true)
  // Self-hosted servers disable watching; hide the Watch tab entirely.
  useEffect(() => {
    let cancelled = false
    void api.system.capabilities().then((caps) => {
      if (cancelled) return
      setWatchEnabled(caps.watch)
      if (!caps.watch) setMode((current) => (current === 'live' ? 'compare' : current))
    })
    return () => { cancelled = true }
  }, [])
  const compareTargets = useRef<{ base: string; head: string; baseBranch: string; headBranch: string } | null>(null)
  const [openPullRequests, setOpenPullRequests] = useState<OpenRepositoryPullRequest[] | null>(null)
  const [prListLoading, setPrListLoading] = useState(false)
  const [prListError, setPrListError] = useState('')
  const prListOperation = useRef<AbortController | null>(null)
  const [prInput, setPrInput] = useState('')
  const [pullRequest, setPullRequest] = useState<RepositoryPullRequest | null>(null)
  const [selectedPath, setSelectedPath] = useState('')
  const [filesTab, setFilesTab] = useState<'files' | 'symbols'>('files')
  const splitRef = useRef<HTMLDivElement | null>(null)
  const liveVersion = useRef('')
  const shownImpact = mode === 'live' ? live?.diagram ?? null : comparison
  const diff: SnapshotDiff | null = shownImpact?.diff ?? null
  const symbolCount = diff ? diff.facts.added + diff.facts.removed + diff.facts.modified : 0
  const filesPanel = useResizableColumn({
    storageKey: 'tld:repositories:filesPanelWidth',
    defaultWidth: FILES_PANEL_DEFAULT_WIDTH,
    collapseBelow: PANEL_COLLAPSE_SNAP,
    side: 'start',
    maxWidth: (containerWidth) => containerWidth * 0.6,
    containerRef: splitRef,
  })
  const filesCollapsed = filesPanel.isCollapsed
  const filesPanelWidth = filesPanel.renderedWidth
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
  const [adding, setAdding] = useState(false)
  const [addError, setAddError] = useState('')
  const [indexerCheck, setIndexerCheck] = useState<RepositoryIndexerCheck | null>(null)
  const [indexersOpen, setIndexersOpen] = useState(true)
  const [checkingIndexers, setCheckingIndexers] = useState(false)
  const initialized = useRef('')
  const operation = useRef<AbortController | null>(null)
  const selectedRef = useRef(selectedId)
  selectedRef.current = selectedId
  const restored = useRef({ base, head })
  const selected = repositories.find((r) => r.id === selectedId)
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
    setBusy(false); setCapturing(false); setProgress(null); setComparison(null); setOperationError(''); setSelectedPath(''); setFilesTab('files'); setHistoryCollapsed(false); setMode(next)
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
    const context = value.startsWith('commit:') ? branch || history?.currentBranch || '' : ''
    if (side === 'base') {
      setBase(value)
      setBaseBranch(context)
    } else {
      setHead(value)
      setHeadBranch(context)
    }
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
    setCapturing(false)
    setProgress(null)
    setSnapshots([])
    setMaps([])
    setIndexerCheck(null)
    setIndexersOpen(true)
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
    setHistoryCollapsed(false)
    initialized.current = ''
    restored.current = { base: '', head: '' }
    setSelectedId(id)
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
        if (isActive()) {
          setComparison(result)
          setHistoryCollapsed(true)
        }
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
  const handleCaptureSnapshot = async (options: { workingTree: boolean }) => {
    if (!selected || busy) return
    const repositoryId = selected.id
    const controller = new AbortController()
    operation.current = controller
    setBusy(true)
    setCapturing(true)
    setOperationError('')
    setProgress(null)
    const isActive = () =>
      !controller.signal.aborted &&
      selectedRef.current === repositoryId &&
      operation.current === controller
    try {
      await api.repositories.captureSnapshot(repositoryId, {
        workingTree: options.workingTree,
        signal: controller.signal,
        onProgress: (next) => {
          if (isActive()) setProgress(next)
        },
      })
      if (isActive()) {
        toast({
          title: options.workingTree ? 'Working tree snapshot captured' : 'Commit snapshot captured',
          status: 'success',
        })
      }
    } catch (err) {
      if (!controller.signal.aborted && selectedRef.current === repositoryId)
        setOperationError(err instanceof Error ? err.message : 'Could not capture a snapshot')
    } finally {
      if (selectedRef.current === repositoryId && operation.current === controller) {
        operation.current = null
        setBusy(false)
        setCapturing(false)
        setProgress(null)
        setNonce((n) => n + 1)
        void reload()
      }
    }
  }
  const handleMapSnapshot = async (snapshot: CodeSnapshot) => {
    if (!selected || busy) return
    const repositoryId = selected.id
    const controller = new AbortController()
    operation.current = controller
    setBusy(true)
    setOperationError('')
    setProgress(null)
    const isActive = () =>
      !controller.signal.aborted &&
      selectedRef.current === repositoryId &&
      operation.current === controller
    try {
      await api.repositories.map(repositoryId, {
        snapshotId: snapshot.id,
        signal: controller.signal,
        onProgress: (next) => {
          if (isActive()) setProgress(next)
        },
      })
      if (isActive()) toast({ title: 'Snapshot mapped into workspace', status: 'success' })
    } catch (err) {
      if (!controller.signal.aborted && selectedRef.current === repositoryId)
        setOperationError(err instanceof Error ? err.message : 'Mapping failed')
    } finally {
      if (selectedRef.current === repositoryId && operation.current === controller) {
        operation.current = null
        setBusy(false)
        setProgress(null)
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
      const target = `snapshot:${snapshotToDelete.id}`
      if (target === base) setBase('')
      if (target === head) setHead('')
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
  // checkRepositoryIndexers scouts the selected repository's checkout for the
  // external SCIP indexers it needs. It backs the checklist on the Snapshots
  // tab; the add dialog stays low friction and lets the server reject a
  // repository whose indexers are missing.
  const checkRepositoryIndexers = async () => {
    const repository = selected
    if (!repository || checkingIndexers) return
    setCheckingIndexers(true)
    setOperationError('')
    try {
      const check = await api.repositories.checkIndexers({ path: repository.root })
      if (selectedRef.current !== repository.id) return
      setIndexerCheck(check)
      // Collapse the checklist once every required indexer is present and at a
      // supported version; keep it open while anything needs attention.
      setIndexersOpen(!check.indexers.every((indexer) => indexer.installed && !indexer.belowMinimum))
    } catch (err) {
      if (selectedRef.current === repository.id) {
        setIndexerCheck(null)
        setOperationError(
          err instanceof Error ? err.message : 'Could not check required indexers',
        )
      }
    } finally {
      setCheckingIndexers(false)
    }
  }
  const handleAddRepository = async () => {
    if (adding) return
    const value = addPath.trim()
    if (!value) {
      setAddError('Enter a repository path or URL')
      return
    }
    setAdding(true)
    setAddError('')
    setAddOpen(false)
    setShowRepositorySettings(false)
    changeMode('snapshots')
    setBusy(true)
    setProgress(null)
    try {
      const added = await api.repositories.add(
        parseRepositoryAddInput(value),
        {
          materialize: false,
          onProgress: (next) => {
            setProgress(next)
          },
        },
      )
      setAddPath('')
      setIndexerCheck(null)
      toast({
        title: 'Repository added',
        description: value,
        status: 'success',
      })
      await reload()
      selectRepo(added.id)
    } catch (err) {
      setAddError(
        err instanceof Error ? err.message : 'Could not add repository',
      )
      setAddOpen(true)
    } finally {
      setAdding(false)
      setBusy(false)
      setProgress(null)
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
                    <RepositorySnapshotsPanel
                      snapshots={snapshots}
                      maps={maps}
                      loading={dataLoading}
                      deleting={deletingSnapshot}
                      onDelete={setSnapshotToDelete}
                    />
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
                }}
                onClose={() => {
                  if (!adding) setAddOpen(false)
                }}
                placement="right-start"
                isLazy
                closeOnBlur={!adding}
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
                  setAddOpen(true)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    setAddError('')
                    setAddOpen(true)
                  }
                }}
              >
                <Center
                  w="28px"
                  h="28px"
                  flexShrink={0}
                  bg="whiteAlpha.100"
                  borderRadius="md"
                  color="gray.400"
                >
                  <AddIcon boxSize="12px" color="var(--accent)"/>
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
                          Index a local repository or clone a remote one
                          (owner/repo or Git URL).
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
                            setAddError('')
                          }}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') void handleAddRepository()
                          }}
                        />
                      </FormControl>
                      {addError && (
                        <Alert status="error" mt={3} borderRadius="md">
                          <AlertIcon />
                          <Text fontSize="sm">{addError}</Text>
                        </Alert>
                      )}
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
                      <Button
                        size="sm"
                        style={accentStyle}
                        data-testid="repositories-add-submit"
                        isLoading={adding}
                        isDisabled={!addPath.trim()}
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
                    {([['snapshots', 'Snapshots'], ['compare', 'Compare'], ['live', 'Watch'], ['pr', 'PR Review']] as const).filter(([value]) => value !== 'live' || watchEnabled).map(([value, label]) => (
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
                  footerContent={(
                    <Flex gap={3} align="center" wrap="wrap" w="full" aria-label="Comparison range">
                      <Flex flex="1 1 420px" minW={0} gap={2} align="center" wrap={{ base: 'wrap', md: 'nowrap' }}>
                        {(['base', 'head'] as const).map((side) => {
                          const value = side === 'base' ? base : head
                          const status = targetStatus(value, snapshots, maps)
                          const snapshot = snapshotForTarget(value, snapshots)
                          const branchName = (side === 'base' ? baseBranch : headBranch) || snapshot?.gitBranch
                          return (
                            <Flex key={side} flex="1 1 180px" minW={0} gap={2} align="center">
                              {side === 'head' && <Box color="gray.500" flexShrink={0} display={{ base: 'none', md: 'flex' }}><SolidIcon icon={faCodeCompare} /></Box>}
                              <Box flex={1} minW={0}>
                                <RepositoryTargetPicker
                                  card
                                  aria-label={side === 'base' ? 'Base target' : 'Head target'}
                                  data-testid={`repositories-${side}-target`}
                                  value={value}
                                  isDisabled={busy || dataLoading || mode === 'pr'}
                                  onChange={(next) => chooseTarget(side, next)}
                                  triggerContent={(
                                    <VStack spacing={1} align="stretch" minW={0}>
                                      <HStack spacing={2} justify="space-between">
                                        <Text fontSize="10px" fontWeight="bold" color={side === 'base' ? 'gray.400' : 'green.300'} textTransform="uppercase" letterSpacing="0.08em">{side}</Text>
                                        <Badge colorScheme={status.colorScheme} fontSize="9px" borderRadius="sm" px={1.5} textTransform="none">{status.label}</Badge>
                                      </HStack>
                                      <HStack spacing={2} minW={0}>
                                        <Text fontSize="sm" fontWeight="semibold" color="gray.100" fontFamily={value === 'working_tree' ? undefined : 'mono'} isTruncated title={targetSummary(value, snapshots)}>{targetSummary(value, snapshots)}</Text>
                                        {branchName && value !== 'working_tree' && <Text fontSize="xs" color="gray.500" isTruncated title={branchName}>{branchName}</Text>}
                                      </HStack>
                                    </VStack>
                                  )}
                                  groups={[
                                    { options: [
                                      { value: 'working_tree', label: 'Working tree · local contents' },
                                      ...(!value || !value.startsWith('commit:') ? [] : [{ value, label: targetSummary(value, snapshots) }]),
                                    ] },
                                    { label: 'Saved snapshots', options: [...snapshots].reverse().map((snapshot) => ({
                                      value: `snapshot:${snapshot.id}`,
                                      label: `${short(snapshot.gitRevision)} · ${snapshot.gitBranch || 'detached / non-Git'}${snapshot.commitMessage ? ` · ${snapshot.commitMessage}` : ''} · ${age(snapshot.createdUnix)}`,
                                    })) },
                                  ]}
                                />
                              </Box>
                            </Flex>
                          )
                        })}
                      </Flex>
                      <HStack spacing={2} flexShrink={0} ml="auto">
                        <Button
                          {...accentStyle}
                          size="sm"
                          h="40px"
                          px={4}
                          borderRadius="md"
                          leftIcon={<RepositoryModeIcon mode="compare" />}
                          data-testid="repositories-compare"
                          isLoading={busy}
                          loadingText="Comparing…"
                          isDisabled={!base || !head || dataLoading}
                          onClick={() => void run('compare')}
                        >
                          Compare
                        </Button>
                        {busy && <Button size="sm" variant="ghost" onClick={() => operation.current?.abort()}>Cancel</Button>}
                      </HStack>
                    </Flex>
                  )}
                  collapsed={historyCollapsed}
                  onToggle={() => setHistoryCollapsed(!historyCollapsed)}
                />}
                {mode === 'live' && (
                  <>
                    <RepositoryWatcherPanel status={watch} repositoryRoot={selected.root} branch={watch?.gitBranch || live?.gitBranch || ''} revision={watch?.gitRevision || live?.gitRevision || ''} busy={watchBusy} onStart={() => void startWatch()} onStop={() => void stopWatch()} onRestart={() => void restartWatch()} onRefresh={() => void refreshWatch()} />
                    <ErrorMessage message={live?.error || ''} />
                  </>
                )}
                {busy && (
                  <Box p={3} data-testid="repositories-operation-progress">
                    <Text fontSize="xs" color="gray.400" mb={2}>
                      {progress
                        ? `${mode === 'snapshots' ? indexStageLabel(progress.stage) || progress.stage : progress.stage} · ${progress.detail}${progress.total ? ` · ${progress.current}/${progress.total}` : ''}`
                        : mode === 'snapshots' ? 'Saving snapshot…' : 'Preparing maps…'}
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

                {mode === 'snapshots' ? (
                  <Box px={{ base: 3, md: 4 }} py={4} maxW="1100px" w="full" mx="auto">
                    <RepositorySnapshotsPanel
                      key={selectedId}
                      snapshots={snapshots}
                      maps={maps}
                      loading={dataLoading}
                      busy={busy}
                      capturing={capturing}
                      deleting={deletingSnapshot}
                      isGit={history?.isGit}
                      onDelete={setSnapshotToDelete}
                      onCapture={(options) => void handleCaptureSnapshot(options)}
                      onMap={(snapshot) => void handleMapSnapshot(snapshot)}
                      onOpenMap={(viewId) => navigate(`/views/${viewId}`)}
                      onCancel={() => operation.current?.abort()}
                      indexerCheck={indexerCheck}
                      checkingIndexers={checkingIndexers}
                      indexersOpen={indexersOpen}
                      onCheckIndexers={() => void checkRepositoryIndexers()}
                      onToggleIndexers={() => setIndexersOpen((open) => !open)}
                    />
                  </Box>
                ) : (
                <Flex
                  flex={1}
                  minH="260px"
                  direction={{ base: 'column', lg: 'row' }}
                  ref={splitRef}
                  data-testid="repositories-split"
                >
                  {filesCollapsed ? (
                    <FilesCollapsedRail fileCount={diff?.sources.length ?? 0} symbolCount={symbolCount} onExpand={filesPanel.expand} />
                  ) : (
                    <>
                      <Box
                        w={{ base: 'full', lg: `${filesPanelWidth}px` }}
                        maxW={{ lg: '60%' }}
                        position="relative"
                        flexShrink={0}
                        borderRight="1px solid"
                        borderBottom={{ base: '1px solid', lg: 'none' }}
                        borderColor="whiteAlpha.100"
                        data-testid="repositories-files-panel"
                      >
                        <Flex p={2} gap={1} role="tablist" aria-label="Repository details">
                          <Button size="xs" role="tab" aria-selected={filesTab === 'files'} variant={filesTab === 'files' ? 'solid' : 'ghost'} onClick={() => setFilesTab('files')}>Files <Badge ml={2} fontSize="2xs">{diff?.sources.length ?? 0}</Badge></Button>
                          <Button size="xs" role="tab" aria-selected={filesTab === 'symbols'} variant={filesTab === 'symbols' ? 'solid' : 'ghost'} onClick={() => setFilesTab('symbols')}>Symbols <Badge ml={2} fontSize="2xs" aria-label="Changed symbols count">{symbolCount}</Badge></Button>
                          <Box flex={1} />
                          <Tooltip label="Collapse files and symbols" placement="top" openDelay={200}>
                            <IconButton
                              size="xs"
                              variant="ghost"
                              aria-label="Collapse files and symbols panel"
                              data-testid="repositories-files-collapse"
                              icon={<ChevronLeftIcon />}
                              onClick={filesPanel.collapse}
                            />
                          </Tooltip>
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
                      <ColumnResizeHandle
                          label="Resize files and symbols panel"
                          dataTestId="repositories-files-resize"
                          display={{ base: 'none', lg: 'block' }}
                          isActive={filesPanel.isResizing}
                          onPointerDown={filesPanel.startResize}
                        />
                    </>
                  )}
                  <RepositoryChangeCanvas key={`${selectedId}:${mode}:${shownImpact?.comparisonKey ?? ''}`} diagram={shownImpact} selectedPath={selectedPath} busy={busy} emptyMessage={mode === 'pr' ? pullRequest ? 'Compare the PR maps to overlay changes on the workspace.' : 'Select an open PR or enter its number or URL to start a review.' : mode === 'live' ? 'Waiting for the watcher to prepare the live map.' : undefined} />
                </Flex>
                )}
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
