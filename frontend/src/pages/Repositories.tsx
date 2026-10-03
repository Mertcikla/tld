import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import {
  Alert,
  AlertIcon,
  Badge,
  Box,
  Button,
  Center,
  Code,
  Flex,
  Grid,
  HStack,
  IconButton,
  Progress,
  Select,
  Spinner,
  Switch,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  CopyIcon,
  DeleteIcon,
  RepeatIcon,
} from '@chakra-ui/icons'
import {
  api,
  type CodeSnapshot,
  type CompletedRepositoryMap,
  type IndexedRepository,
  type RepositoryGitHistory,
  type RepositoryMapProgress,
  type SnapshotDiff,
  type RepositoryImpact as RepositoryImpactResult,
  type LiveRepositoryImpact,
  type SnapshotSourceChange,
} from '../api/client'
import ConfirmDialog from '../components/ConfirmDialog'
import RepositoryHistory from '../components/RepositoryHistory'
import RepositoryChangeCanvas from '../components/RepositoryChangeCanvas'
import {
  defaultRepositoryTargets,
  snapshotForTarget,
  targetMapOptions,
} from '../utils/repositoryTargets'
import { toast } from '../utils/toast'

const accentStyle = {
  bg: 'var(--accent)',
  color: 'white',
  _hover: { bg: 'var(--accent)', filter: 'brightness(1.08)' },
}
const showIdsKey = 'tld:repositories:showIds'
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

function FileTree({ files, onSelect }: { files: SnapshotSourceChange[]; onSelect: (path: string) => void }) {
  const [closed, setClosed] = useState<Set<string>>(new Set())
  const entries = useMemo(() => {
    const folders = new Set<string>()
    const rows: {
      path: string
      directory: boolean
      linesAdded?: number
      linesRemoved?: number
    }[] = []
    for (const file of [...files].sort((a, b) =>
      a.path.localeCompare(b.path),
    )) {
      const parts = file.path.split('/')
      for (let i = 1; i < parts.length; i++) {
        const folder = parts.slice(0, i).join('/')
        if (!folders.has(folder)) {
          folders.add(folder)
          rows.push({ path: folder, directory: true })
        }
      }
      rows.push({ path: file.path, directory: false, linesAdded: file.linesAdded, linesRemoved: file.linesRemoved })
    }
    return rows
  }, [files])
  return (
    <Box p={2} overflowY="auto" maxH={{ base: '220px', lg: 'none' }}>
      {!files.length && (
        <Text p={3} fontSize="sm" color="gray.500">
          No source changes.
        </Text>
      )}
      {entries
        .filter(
          (entry) =>
            ![...closed].some((folder) => entry.path.startsWith(`${folder}/`)),
        )
        .map((entry) => (
          <Flex
            key={entry.path}
            pl={`${(entry.path.split('/').length - 1) * 12 + 4 + (entry.directory ? 0 : 24)}px`}
            py={0.5}
            minH="22px"
            align="center"
            gap={2}
          >
            {entry.directory ? (
              <Button
                variant="ghost"
                size="xs"
                h="18px"
                p={0}
                minW="16px"
                aria-label={`${closed.has(entry.path) ? 'Expand' : 'Collapse'} ${entry.path}`}
                onClick={() =>
                  setClosed((old) => {
                    const next = new Set(old)
                    if (next.has(entry.path)) next.delete(entry.path)
                    else next.add(entry.path)
                    return next
                  })
                }
              >
                {closed.has(entry.path) ? (
                  <ChevronRightIcon />
                ) : (
                  <ChevronLeftIcon transform="rotate(-90deg)" />
                )}
              </Button>
            ) : null}
            <Text as={entry.directory ? 'span' : 'button'} onClick={() => { if (!entry.directory) onSelect(entry.path) }} flex={1} minW={0} fontSize="xs" lineHeight="18px" color="gray.300" textAlign="left" isTruncated title={entry.path}>
              {entry.path.split('/').pop()}
            </Text>
            {!entry.directory && entry.linesAdded !== undefined && entry.linesRemoved !== undefined && (
              <HStack spacing={1.5} flexShrink={0} fontSize="10px" lineHeight="18px" fontFamily="mono" aria-label={`${entry.linesAdded} lines added, ${entry.linesRemoved} lines removed`}>
                <Text color="green.300">+{entry.linesAdded}</Text>
                <Text color="red.300">−{entry.linesRemoved}</Text>
              </HStack>
            )}
          </Flex>
        ))}
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
  includeImports,
  showIds,
  disabled,
  onChange,
  onMap,
}: {
  side: 'Base' | 'Head'
  value: string
  branch: string
  snapshots: CodeSnapshot[]
  history: RepositoryGitHistory | null
  maps: CompletedRepositoryMap[]
  includeImports: boolean
  showIds: boolean
  disabled: boolean
  onChange: (value: string) => void
  onMap: () => void
}) {
  const snapshot = snapshotForTarget(value, snapshots)
  const mapped =
    !!snapshot &&
    maps.some(
      (m) =>
        m.result.snapshotId === snapshot?.id &&
        m.includeImports === includeImports,
    )
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
      <Select
        aria-label={`${side} target`}
        data-testid={`repositories-${side.toLowerCase()}-target`}
        size="sm"
        value={value}
        isDisabled={disabled}
        onChange={(e) => onChange(e.target.value)}
      >
        <option value="">Select a target</option>
        <option value="working_tree">Working tree · local contents</option>
        {!known && value && (
          <option value={value}>
            {value.startsWith('commit:')
              ? `Commit ${short(value.slice(7))}`
              : value}
          </option>
        )}
        <optgroup label="Saved snapshots">
          {[...snapshots].reverse().map((s) => (
            <option key={s.id} value={`snapshot:${s.id}`}>
              {short(s.gitRevision)} · {s.gitBranch || 'detached / non-Git'} ·{' '}
              {s.provenance === 'working_tree'
                ? 'local contents'
                : s.provenance === 'commit'
                  ? 'commit'
                  : 'unknown provenance'}{' '}
              · {age(s.createdUnix)}
              {showIds ? ` · ${short(s.id)}` : ''}
            </option>
          ))}
        </optgroup>
        <optgroup label="Commits">
          {history?.commits.map((c) => (
            <option key={c.sha} value={`commit:${c.sha}`}>
              {c.sha.slice(0, 7)} · {c.subject}
            </option>
          ))}
        </optgroup>
      </Select>
      <Flex mt={2} gap={2} align="center">
        <Text fontSize="xs" color="gray.500" flex={1} isTruncated>
          {value === 'working_tree'
            ? 'Includes staged, unstaged, and untracked source files'
            : snapshot
              ? `${snapshot.gitBranch || 'No captured branch'} · ${snapshot.embeddingStatus || 'Embedding status unknown'}`
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
  const [collapsed, setCollapsed] = useState(false)
  const [historyCollapsed, setHistoryCollapsed] = useState(false)
  const [compareCollapsed, setCompareCollapsed] = useState(false)
  const [showIds] = useState(readShowIds)
  const [includeImports, setIncludeImports] = useState(false)
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
  const [mode, setMode] = useState(() => params.get('mode') === 'live' ? 'live' : 'compare')
  const [selectedPath, setSelectedPath] = useState('')
  const liveVersion = useRef('')
  const shownImpact = mode === 'live' ? live?.diagram ?? null : comparison
  const diff: SnapshotDiff | null = shownImpact?.diff ?? null
  const [nonce, setNonce] = useState(0)
  const [limit, setLimit] = useState(50)
  const [repoToDelete, setRepoToDelete] = useState<IndexedRepository | null>(
    null,
  )
  const [deleteMaterialized, setDeleteMaterialized] = useState(false)
  const [deletingRepo, setDeletingRepo] = useState(false)
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
          mode,
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
  }, [selectedId, base, head, branch, baseBranch, headBranch, mode, setParams])
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
      api.repositories.history(selectedId, branch, limit),
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
  }, [selectedId, branch, limit, nonce])
  useEffect(() => {
    setComparison(null); setSelectedPath('')
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
          const result = await api.repositories.liveImpact(selectedId, controller.signal)
          if (!controller.signal.aborted && !operation.current) {
            setLive((old) => ({ ...result, diagram: result.diagram?.version === old?.diagram?.version ? old?.diagram ?? null : result.diagram }))
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
    operation.current?.abort(); operation.current = null
    setBusy(false); setProgress(null); setOperationError(''); setSelectedPath(''); setMode(next)
  }
  const changeRadius = async (radius: number) => {
    if (!shownImpact || busy) return
    const repositoryId = selectedId
    const key = shownImpact.comparisonKey
    const controller = new AbortController()
    operation.current = controller; setBusy(true); setOperationError('')
    try {
      const result = await api.repositories.impactRadius(repositoryId, key, radius, controller.signal)
      if (!controller.signal.aborted && selectedRef.current === repositoryId && operation.current === controller) {
        if (mode === 'live') setLive((old) => old ? { ...old, diagram: result } : old)
        else setComparison(result)
      }
    } catch (err) {
      if (!controller.signal.aborted && selectedRef.current === repositoryId) setOperationError(err instanceof Error ? err.message : 'Could not update blast radius')
    } finally { if (operation.current === controller) { operation.current = null; setBusy(false) } }
  }
  const selectRepo = (id: string) => {
    if (id === selectedId) return
    operation.current?.abort()
    operation.current = null
    setBusy(false)
    setProgress(null)
    setSnapshots([])
    setMaps([])
    setLive(null)
    liveVersion.current = ''
    setHistory(null)
    setBase('')
    setHead('')
    setBranch('')
    setBaseBranch('')
    setHeadBranch('')
    setLimit(50)
    setComparison(null); setSelectedPath('')
    initialized.current = ''
    restored.current = { base: '', head: '' }
    setSelectedId(id)
  }
  const chooseTarget = (side: 'base' | 'head', value: string) => {
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
    if (!selected || busy) return
    const repositoryId = selected.id
    const controller = new AbortController()
    operation.current = controller
    setBusy(true)
    setOperationError('')
    setProgress(null)
    setComparison(null); setSelectedPath('')
    const isActive = () =>
      !controller.signal.aborted &&
      selectedRef.current === repositoryId &&
      operation.current === controller
    const map = (target: string, context: string) =>
      api.repositories.map(repositoryId, {
        ...targetMapOptions(target, context),
        includeImports,
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
  const copy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value)
      toast({ title: 'Copied', status: 'success' })
    } catch {
      toast({ title: 'Copy failed', status: 'warning' })
    }
  }
  const handleDelete = async () => {
    if (!repoToDelete) return
    setDeletingRepo(true)
    try {
      await api.repositories.delete(repoToDelete.id, { deleteMaterialized })
      if (repoToDelete.id === selectedId) selectRepo('')
      setRepoToDelete(null)
      setDeleteMaterialized(false)
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
      ) : !repositories.length ? (
        <Center flex={1}>
          <VStack p={4}>
            <Text fontWeight="semibold">No repositories indexed yet</Text>
            <Text fontSize="sm" color="gray.400">
              Run <Code>tld index &lt;path&gt;</Code> to register a repository.
            </Text>
            <Button size="sm" onClick={() => void reload()}>
              Reload
            </Button>
          </VStack>
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
            maxH={{ base: collapsed ? '48px' : '32vh', lg: 'none' }}
            overflowY="auto"
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
            {repositories.map((repo) => (
              <Box
                key={repo.id}
                role="group"
                borderBottom="1px solid"
                borderColor="whiteAlpha.100"
              >
                <Flex
                  px={collapsed ? 0 : 4}
                  py={3}
                  align="center"
                  justify={collapsed ? 'center' : undefined}
                  gap={3}
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
                    aria-label={`Select ${nameOf(repo.root)}`}
                    onClick={() => selectRepo(repo.id)}
                  >
                    <Glyph name={nameOf(repo.root)} />
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
                          {nameOf(repo.root)}
                        </Text>
                        <Text
                          fontSize="xs"
                          color="gray.500"
                          isTruncated
                          title={repo.root}
                        >
                          {repo.root}
                        </Text>
                        <Badge
                          fontSize="2xs"
                          colorScheme={repo.latestSnapshotId ? 'green' : 'gray'}
                        >
                          {repo.latestSnapshotId ? 'Indexed' : 'Not indexed'}
                        </Badge>
                      </Box>
                      <IconButton
                        data-testid={`repositories-delete-${repo.id}`}
                        aria-label={`Delete ${nameOf(repo.root)}`}
                        icon={<DeleteIcon />}
                        size="xs"
                        variant="ghost"
                        isDisabled={busy}
                        onClick={(e) => {
                          e.stopPropagation()
                          setRepoToDelete(repo)
                          setDeleteMaterialized(false)
                        }}
                      />
                    </>
                  )}
                </Flex>
                {!collapsed && repo.id === selectedId && (
                  <Box px={4} pb={3}>
                    <HStack
                      mb={2}
                      sx={{
                        '&:hover > .repository-copy, &:focus-within > .repository-copy':
                          {
                            opacity: 1,
                            pointerEvents: 'auto',
                          },
                        '@media (hover: none)': {
                          '> .repository-copy': {
                            opacity: 1,
                            pointerEvents: 'auto',
                          },
                        },
                      }}
                    >
                      <Code
                        fontSize="2xs"
                        isTruncated
                        flex={1}
                        title={repo.root}
                        bg="transparent"
                        p={0}
                      >
                        {repo.root}
                      </Code>
                      <IconButton
                        aria-label="Copy local path"
                        className="repository-copy"
                        opacity={0}
                        pointerEvents="none"
                        icon={<CopyIcon />}
                        size="xs"
                        variant="ghost"
                        onClick={() => void copy(repo.root)}
                      />
                    </HStack>
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
                      {[...snapshots].reverse().map((s) => (
                        <Box
                          key={s.id}
                          title={
                            s.provenance === 'commit'
                              ? 'Commit snapshot'
                              : s.provenance === 'working_tree'
                                ? 'Working tree snapshot'
                                : 'Unknown provenance'
                          }
                          px={2}
                          py={1.5}
                          bg="whiteAlpha.50"
                          borderRadius="md"
                          border="1px solid"
                          borderColor="whiteAlpha.100"
                        >
                          <Flex gap={2} align="center">
                            <Text
                              fontSize="xs"
                              color="gray.300"
                              flex={1}
                              isTruncated
                              title={s.gitBranch}
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
                          </Flex>
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
                        </Box>
                      ))}
                    </VStack>
                  </Box>
                )}
              </Box>
            ))}
          </Box>
          <Box
            flex={1}
            minW={0}
            minH={0}
            overflowY="auto"
            display="flex"
            flexDir="column"
          >
            {selected && (
              <>
                <Flex
                  px={4}
                  py={2}
                  h={{ base: 'auto', lg: '44px' }}
                  flexShrink={0}
                  gap={3}
                  align="center"
                  wrap="wrap"
                  borderBottom="1px solid"
                  borderColor="whiteAlpha.100"
                >
                  <Text fontSize="sm" fontWeight="semibold">
                    {nameOf(selected.root)}
                  </Text>
                  <Box flex={1} />
                  <Select
                    aria-label="History branch"
                    size="xs"
                    w="200px"
                    value={branch}
                    isDisabled={busy || !history?.isGit}
                    onChange={(e) => {
                      setBranch(e.target.value)
                      setLimit(50)
                    }}
                  >
                    <option value="">
                      Current HEAD
                      {history?.currentBranch
                        ? ` · ${history.currentBranch}`
                        : ''}
                    </option>
                    {branch &&
                      !history?.branches.some((b) => b.name === branch) && (
                        <option value={branch}>{branch}</option>
                      )}
                    {history?.branches.map((b) => (
                      <option key={b.name} value={b.name}>
                        {b.name}
                      </option>
                    ))}
                  </Select>
                  <Button
                    size="xs"
                    variant="outline"
                    isDisabled={!currentViewId || busy}
                    onClick={() => navigate(`/views/${currentViewId}`)}
                  >
                    Open map
                  </Button>
                </Flex>
                {historyError && (
                  <Text p={3} fontSize="xs" color="orange.300">
                    {historyError} · Saved snapshots remain available.
                  </Text>
                )}
                <ErrorMessage message={dataError} />
                <RepositoryHistory
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
                  collapsed={historyCollapsed}
                  onToggle={() => setHistoryCollapsed(!historyCollapsed)}
                  onBase={(c) => {
                    if (!busy) chooseTarget('base', `commit:${c.sha}`)
                  }}
                  onHead={(c) => {
                    if (!busy) chooseTarget('head', `commit:${c.sha}`)
                  }}
                  onMore={() => setLimit((n) => n + 50)}
                />
                <Flex
                  px={4}
                  h="40px"
                  align="center"
                  gap={3}
                  flexShrink={0}
                  borderBottom="1px solid"
                  borderColor="whiteAlpha.100"
                >
                  <Button size="xs" variant={mode === 'compare' ? 'solid' : 'ghost'} data-testid="repositories-compare-tab" aria-pressed={mode === 'compare'} onClick={() => changeMode('compare')}>Compare</Button>
                  <Button size="xs" variant={mode === 'live' ? 'solid' : 'ghost'} data-testid="repositories-live-tab" aria-pressed={mode === 'live'} onClick={() => changeMode('live')}>Live changes</Button>
                  <Button
                    size="xs"
                    variant="ghost"
                    aria-expanded={!compareCollapsed}
                    onClick={() => setCompareCollapsed(!compareCollapsed)}
                  >
                    {compareCollapsed ? 'Expand' : 'Collapse'}
                  </Button>
                  <Box flex={1} />
                  <Text fontSize="xs" color="gray.500">
                    {mode === 'live' ? 'Current commit → pending changes' : 'Select snapshots or Git revisions'}
                  </Text>
                </Flex>
                {mode === 'live' && (
                  <Box p={4} borderBottom="1px solid" borderColor="whiteAlpha.100">
                    <HStack mb={2}><Badge colorScheme={live?.watching ? 'green' : 'gray'}>{live?.watching ? 'Watching' : 'Watcher inactive'}</Badge><Text fontSize="xs">{live?.gitBranch || 'Detached HEAD'} · {short(live?.gitRevision || '')}</Text></HStack>
                    <Text fontSize="xs" color="gray.400">Base is the current commit. Head includes staged, unstaged, and non-ignored untracked files.</Text>
                    {!live?.watching && <Text mt={2} fontSize="xs" color="gray.400">Run <Code>tld index {selected?.root} --watch</Code> to keep this diagram updated.</Text>}
                    <ErrorMessage message={live?.error || ''} />
                  </Box>
                )}
                {mode === 'compare' && !compareCollapsed && (
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
                        includeImports={includeImports}
                        showIds={showIds}
                        disabled={busy || dataLoading}
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
                        includeImports={includeImports}
                        showIds={showIds}
                        disabled={busy || dataLoading}
                        onChange={(value) => chooseTarget('head', value)}
                        onMap={() => void run('head')}
                      />
                    </Grid>
                    <Flex mt={3} gap={3} align="center" wrap="wrap">
                      <HStack>
                        <Switch
                          size="sm"
                          data-testid="repositories-include-imports"
                          isDisabled={busy}
                          isChecked={includeImports}
                          onChange={(e) => setIncludeImports(e.target.checked)}
                        />
                        <Text fontSize="xs" color="gray.400">
                          Include external imports in full maps
                        </Text>
                      </HStack>
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
                {diff && (
                  <Grid
                    px={4}
                    py={3}
                    templateColumns="repeat(4, 1fr)"
                    gap={2}
                    borderBottom="1px solid"
                    borderColor="whiteAlpha.100"
                  >
                    {[
                      ['Sources', diff.sources.length],
                      ['Facts added', diff.facts.added],
                      ['Facts removed', diff.facts.removed],
                      ['Facts modified', diff.facts.modified],
                    ].map(([label, count]) => (
                      <Box
                        key={label}
                        p={2}
                        bg="whiteAlpha.50"
                        borderRadius="md"
                      >
                        <Text fontSize="lg" fontFamily="mono">
                          {count}
                        </Text>
                        <Label>{label}</Label>
                      </Box>
                    ))}
                  </Grid>
                )}
                <Flex
                  flex={1}
                  minH="260px"
                  direction={{ base: 'column', lg: 'row' }}
                >
                  <Box
                    w={{ base: 'full', lg: '280px' }}
                    flexShrink={0}
                    borderRight="1px solid"
                    borderBottom={{ base: '1px solid', lg: 'none' }}
                    borderColor="whiteAlpha.100"
                  >
                    <Flex p={3} gap={2}>
                      <Label>Files</Label>
                      <Badge fontSize="2xs">{diff?.sources.length ?? 0}</Badge>
                    </Flex>
                    {diff ? (
                      <FileTree files={diff.sources} onSelect={setSelectedPath} />
                    ) : (
                      <Text px={3} fontSize="xs" color="gray.500">
                        Compare maps to see changed source files.
                      </Text>
                    )}
                  </Box>
                  <RepositoryChangeCanvas key={`${selectedId}:${mode}:${shownImpact?.comparisonKey ?? ''}`} diagram={shownImpact} busy={busy} selectedPath={selectedPath} onRadius={(radius) => void changeRadius(radius)} repositoryRoot={selected.root} />
                </Flex>
              </>
            )}
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
      </ConfirmDialog>
    </Box>
  )
}
