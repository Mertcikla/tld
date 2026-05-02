import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import {
  Badge,
  Box,
  Button,
  Collapse,
  HStack,
  IconButton,
  Menu,
  MenuButton,
  MenuList,
  MenuItem,
  Portal,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import { ChevronDownIcon, ChevronLeftIcon, ChevronRightIcon, CloseIcon, RepeatIcon, TimeIcon, ViewIcon, ViewOffIcon } from '@chakra-ui/icons'
import {
  api,
  type WatchDiff,
  type WatchEvent,
  type WatchLock,
  type WatchRepresentationSummary,
  type WatchRepository,
  type WatchVersion,
  type WorkspaceVersion,
} from '../api/client'
import { buildWorkspaceVersionPreview, useWorkspaceVersionPreview } from '../context/WorkspaceVersionContext'
import {
  buildWatchDiffLocations,
  formatTldStatLine,
  summarizeWatchDiffs,
  type WatchDiffLocation,
} from '../utils/watchDiffSummary'

export const WATCH_REPRESENTATION_UPDATED_EVENT = 'tld:watch-representation-updated'

// ─── Watch helpers ────────────────────────────────────────────────────────────

type WatchLine = {
  id: number
  at: string
  text: string
  tone: 'info' | 'success' | 'warning' | 'error'
}

function PauseGlyph() {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <rect x="6" y="5" width="4" height="14" rx="1" />
      <rect x="14" y="5" width="4" height="14" rx="1" />
    </svg>
  )
}

function summarizeEvent(event: WatchEvent): WatchLine | null {
  const id = Date.now() + Math.random()
  const at = event.at || new Date().toISOString()
  const type = event.type
  if (type === 'watch.heartbeat') return null
  if (type === 'watch.paused') return { id, at, text: 'Watch paused', tone: 'warning' }
  if (type === 'watch.stopped') return { id, at, text: 'Watch stopped', tone: 'warning' }
  if (type === 'watch.error') return { id, at, text: event.message || 'Watch error', tone: 'error' }
  if (type === 'lock.disabled') return null
  if (type === 'lock.enabled') return { id, at, text: 'Workspace locked for watch updates', tone: 'info' }
  if (type === 'version.created') return null
  if (type === 'representation.updated') {
    const data = event.data as Partial<WatchRepresentationSummary> | undefined
    const changed = [
      data?.views_created ? `views +${data.views_created}` : '',
      data?.elements_created || data?.elements_updated ? `elements +${data.elements_created ?? 0}/${data.elements_updated ?? 0}` : '',
      data?.connectors_created || data?.connectors_updated ? `connectors +${data.connectors_created ?? 0}/${data.connectors_updated ?? 0}` : '',
    ].filter(Boolean).join(', ')
    return { id, at, text: changed ? `Workspace updated: ${changed}` : 'Workspace refreshed', tone: 'success' }
  }
  if (type === 'scan.started') {
    const files = event.changed_files ? ` · ${event.changed_files} files` : ''
    return { id, at, text: `Scanning${files}`, tone: 'info' }
  }
  if (type === 'scan.completed') {
    const warnings = event.warnings?.length ? ` · ${event.warnings[0]}` : ''
    return { id, at, text: `Scan complete${warnings}`, tone: event.warnings?.length ? 'warning' : 'success' }
  }
  if (type === 'source.changed') {
    const data = event.data as { change?: { path?: string; change_type?: string }; representation_changed?: boolean } | undefined
    const path = data?.change?.path ?? 'source file'
    const suffix = data?.representation_changed ? 'changed the diagram' : 'did not change the diagram'
    return { id, at, text: `${path} ${suffix}`, tone: data?.representation_changed ? 'success' : 'info' }
  }
  return { id, at, text: type, tone: 'info' }
}

function shortPath(path: string | undefined): string {
  if (!path) return 'repository'
  const parts = path.split(/[/\\]/).filter(Boolean)
  return parts.slice(-2).join('/') || path
}

function versionLabel(version: WatchVersion) {
  const subject = version.commit_message?.trim()
  return subject || `Version ${new Date(version.created_at).toLocaleTimeString()}`
}

function changeLabel(diffs: WatchDiff[]) {
  const summary = summarizeWatchDiffs(diffs)
  const total = summary.elements.added + summary.elements.updated + summary.elements.deleted + summary.elements.changed +
    summary.connectors.added + summary.connectors.updated + summary.connectors.deleted + summary.connectors.changed
  return total > 0 ? formatTldStatLine(summary) : 'No materialized changes'
}

function normalizeDiffs(value: WatchDiff[] | null | undefined): WatchDiff[] {
  return Array.isArray(value) ? value : []
}

// ─── Themed dropdown ──────────────────────────────────────────────────────────

interface ThemedSelectProps<T extends string | number> {
  value: T | ''
  options: { value: T; label: string }[]
  placeholder?: string
  onChange: (value: T | '') => void
  isDisabled?: boolean
  flex?: number
}

function ThemedSelect<T extends string | number>({ value, options, placeholder, onChange, isDisabled, flex }: ThemedSelectProps<T>) {
  const selected = options.find((o) => o.value === value)
  return (
    <Menu placement="top-start" strategy="fixed">
      <MenuButton
        as={Button}
        rightIcon={<ChevronDownIcon />}
        size="xs"
        variant="ghost"
        isDisabled={isDisabled}
        flex={flex}
        minW={0}
        h="26px"
        px={2}
        fontSize="11px"
        fontWeight="500"
        color={selected ? 'gray.100' : 'gray.500'}
        bg="whiteAlpha.50"
        border="1px solid"
        borderColor="whiteAlpha.100"
        borderRadius="md"
        _hover={{ bg: 'whiteAlpha.100', borderColor: 'whiteAlpha.200' }}
        _active={{ bg: 'whiteAlpha.150' }}
        textAlign="left"
        justifyContent="flex-start"
        overflow="hidden"
        sx={{ '> span:first-of-type': { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' } }}
      >
        {selected?.label ?? placeholder ?? '—'}
      </MenuButton>
      <Portal>
        <MenuList
          data-zui-native-wheel="true"
          bg="rgba(var(--bg-main-rgb), 0.98)"
          border="1px solid"
          borderColor="whiteAlpha.200"
          borderRadius="lg"
          boxShadow="0 12px 32px rgba(0,0,0,0.5)"
          backdropFilter="blur(18px)"
          minW="200px"
          maxH="240px"
          overflowY="auto"
          zIndex={2000}
          py={1}
          sx={{ overscrollBehavior: 'contain', WebkitOverflowScrolling: 'touch', touchAction: 'pan-y' }}
        >
          {options.length === 0 && (
            <MenuItem isDisabled fontSize="11px" color="gray.500" bg="transparent">No options</MenuItem>
          )}
          {options.map((opt) => (
            <MenuItem
              key={String(opt.value)}
              fontSize="11px"
              color={opt.value === value ? 'var(--accent)' : 'gray.200'}
              fontWeight={opt.value === value ? '600' : '400'}
              bg="transparent"
              _hover={{ bg: 'whiteAlpha.100' }}
              _focus={{ bg: 'whiteAlpha.100' }}
              py={1.5}
              px={3}
              onClick={() => onChange(opt.value)}
            >
              {opt.label}
            </MenuItem>
          ))}
        </MenuList>
      </Portal>
    </Menu>
  )
}

// ─── Main combined panel ──────────────────────────────────────────────────────

export default function WorkspacePanel() {
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()

  // ── Version state ─────────────────────────────────────────────────────────
  const { preview, setPreview, clearPreview, requestFollow } = useWorkspaceVersionPreview()
  const [versionsOpen, setVersionsOpen] = useState(false)
  const [diffVisible, setDiffVisible] = useState(true)
  const [repos, setRepos] = useState<WatchRepository[]>([])
  const [versions, setVersions] = useState<WatchVersion[]>([])
  const [workspaceVersions, setWorkspaceVersions] = useState<WorkspaceVersion[]>([])
  const [repoId, setRepoId] = useState<number | ''>('')
  const [versionId, setVersionId] = useState<number | ''>('')
  const [diffs, setDiffs] = useState<WatchDiff[]>([])
  const [diffLocations, setDiffLocations] = useState<WatchDiffLocation[]>([])
  const [activeDiffLocationKey, setActiveDiffLocationKey] = useState<string | null>(null)

  const selectedRepo = useMemo(() => repos.find((r) => r.id === repoId) ?? null, [repos, repoId])
  const selectedVersion = useMemo(() => versions.find((v) => v.id === versionId) ?? null, [versions, versionId])

  const selectLatestWatchVersion = useCallback(async (targetRepoId: number) => {
    const nextVersions = await api.watch.versions(targetRepoId)
    setVersions(nextVersions)
    const latest = nextVersions[0] ?? null
    setVersionId(latest?.id ?? '')
    if (!latest) {
      setDiffs([])
      return
    }
    const latestDiffs = await api.watch.diffs(latest.id).catch(() => [] as WatchDiff[])
    setDiffs(normalizeDiffs(latestDiffs))
  }, [])

  const loadVersions = useCallback(async () => {
    const [nextRepos, nextWsVersions] = await Promise.all([
      api.watch.repositories().catch(() => [] as WatchRepository[]),
      api.versions.list(50).catch(() => [] as WorkspaceVersion[]),
    ])
    setRepos(nextRepos)
    setWorkspaceVersions(nextWsVersions)
    const nextRepoId = repoId || nextRepos[0]?.id || ''
    setRepoId(nextRepoId)
    if (nextRepoId) {
      const nextVersions = await api.watch.versions(nextRepoId)
      setVersions(nextVersions)
      setVersionId(versionId || nextVersions[0]?.id || '')
    }
  }, [repoId, versionId])

  useEffect(() => {
    if (!versionsOpen && !preview) return
    void loadVersions()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [versionsOpen])

  useEffect(() => {
    if (!repoId) { setVersions([]); setVersionId(''); return }
    api.watch.versions(repoId).then((next) => {
      setVersions(next)
      setVersionId(next[0]?.id ?? '')
    }).catch(() => { setVersions([]); setVersionId('') })
  }, [repoId])

  useEffect(() => {
    if (!versionId) { setDiffs([]); return }
    api.watch.diffs(versionId).then((next) => setDiffs(normalizeDiffs(next))).catch(() => setDiffs([]))
  }, [versionId])

  useEffect(() => {
    if (!diffs.length) {
      setDiffLocations([])
      return
    }
    let cancelled = false
    api.explore.load().then((data) => {
      if (!cancelled) setDiffLocations(buildWatchDiffLocations(data, diffs))
    }).catch(() => {
      if (!cancelled) setDiffLocations([])
    })
    return () => { cancelled = true }
  }, [diffs])

  const displayedDiffLocations = useMemo(() => diffLocations.slice(0, 24), [diffLocations])
  const navigableDiffLocations = useMemo(() => {
    const elementLocations = diffLocations.filter((target) => target.resourceType === 'element')
    return elementLocations.length > 0 ? elementLocations : diffLocations
  }, [diffLocations])
  const activeDiffLocationIndex = useMemo(() => {
    if (!activeDiffLocationKey) return -1
    const index = navigableDiffLocations.findIndex((target) => target.key === activeDiffLocationKey)
    return index >= 0 ? index : -1
  }, [activeDiffLocationKey, navigableDiffLocations])

  useEffect(() => {
    if (!selectedVersion || !diffVisible) {
      clearPreview()
      return
    }
    setPreview(buildWorkspaceVersionPreview({ repository: selectedRepo, version: selectedVersion, workspaceVersions, diffs }))
  }, [clearPreview, diffVisible, diffs, selectedRepo, selectedVersion, setPreview, workspaceVersions])

  const navigateToDiffLocation = useCallback((target: WatchDiffLocation) => {
    setActiveDiffLocationKey(target.key)
    requestFollow({
      resourceType: target.resourceType,
      resourceId: target.resourceId,
      viewId: target.viewId,
      changeType: target.changeType,
    })
    if (location.pathname === '/dependencies' && target.resourceType === 'element' && target.resourceId) {
      navigate(`/dependencies?element=${target.resourceId}`)
      return
    }
    if (location.pathname.startsWith('/views/') && !location.pathname.startsWith('/views?')) {
      const elementQuery = target.resourceType === 'element' && target.resourceId ? `?element=${target.resourceId}` : ''
      navigate(`/views/${target.viewId}${elementQuery}`)
      return
    }
    const elementQuery = target.resourceType === 'element' && target.resourceId ? `&element=${target.resourceId}` : ''
    navigate(`/views?view=explore&focus=${target.viewId}${elementQuery}`)
  }, [location.pathname, navigate, requestFollow])

  const navigateDiffLocationByOffset = useCallback((offset: number) => {
    if (navigableDiffLocations.length === 0) return
    const nextIndex = activeDiffLocationIndex < 0
      ? offset > 0 ? 0 : navigableDiffLocations.length - 1
      : (activeDiffLocationIndex + offset + navigableDiffLocations.length) % navigableDiffLocations.length
    navigateToDiffLocation(navigableDiffLocations[nextIndex])
  }, [activeDiffLocationIndex, navigableDiffLocations, navigateToDiffLocation])

  const compactSummary = preview ? changeLabel(preview.diffs) : diffs.length > 0 ? changeLabel(diffs) : 'Workspace versions'
  const activeVersion = preview?.version ?? selectedVersion
  const activeRepo = preview?.repository ?? selectedRepo
  const diffSummary = useMemo(() => summarizeWatchDiffs(diffs), [diffs])
  const totalFileChanges = diffSummary.files.added + diffSummary.files.updated + diffSummary.files.deleted + diffSummary.files.changed
  const totalTldChanges = diffSummary.elements.added + diffSummary.elements.updated + diffSummary.elements.deleted + diffSummary.elements.changed +
    diffSummary.connectors.added + diffSummary.connectors.updated + diffSummary.connectors.deleted + diffSummary.connectors.changed
  const activeDiffLocation = activeDiffLocationIndex >= 0 ? navigableDiffLocations[activeDiffLocationIndex] : null

  // ── Watch state ───────────────────────────────────────────────────────────
  const socketRef = useRef<WebSocket | null>(null)
  const reconnectTimerRef = useRef<number | null>(null)
  const lastRepresentationHashRef = useRef('')
  const [watchActive, setWatchActive] = useState(false)
  const [watchPaused, setWatchPaused] = useState(false)
  const [watchRepository, setWatchRepository] = useState<WatchRepository | null>(null)
  const [watchLock, setWatchLock] = useState<WatchLock | null>(null)
  const [watchConnected, setWatchConnected] = useState(false)
  const [watcherMode, setWatcherMode] = useState('')
  const [languages, setLanguages] = useState<string[]>([])
  const [watchLines, setWatchLines] = useState<WatchLine[]>([])
  const [runtimeOpen, setRuntimeOpen] = useState(true)

  const addLine = useCallback((line: WatchLine | null) => {
    if (!line) return
    setWatchLines((current) => {
      if (current[0]?.text === line.text && current[0]?.tone === line.tone) return current
      return [line, ...current].slice(0, 8)
    })
  }, [])

  const refreshWorkspace = useCallback((event: WatchEvent) => {
    const data = event.data as Partial<WatchRepresentationSummary> | undefined
    const hash = data?.representation_hash ?? ''
    if (hash && hash === lastRepresentationHashRef.current) return
    if (hash) lastRepresentationHashRef.current = hash
    void queryClient.invalidateQueries({ queryKey: ['workspace', 'views'] })
    void queryClient.invalidateQueries({ queryKey: ['elements', 'list'] })
    window.dispatchEvent(new CustomEvent(WATCH_REPRESENTATION_UPDATED_EVENT, { detail: event }))
  }, [queryClient])

  const handleEvent = useCallback((event: WatchEvent) => {
    const eventLock = event.data && typeof event.data === 'object' && 'status' in event.data
      ? event.data as WatchLock : null
    if (event.repository_id) setWatchLock((current) => eventLock ?? current)
    if (eventLock) setWatchPaused(eventLock.status === 'paused')
    if (event.watcher_mode) setWatcherMode(event.watcher_mode)
    if (event.languages?.length) setLanguages(event.languages)
    if (event.type === 'watch.paused') setWatchPaused(true)
    if (event.type === 'watch.heartbeat') {
      setWatchActive(true)
      if (eventLock) setWatchPaused(eventLock.status === 'paused')
    }
    if (event.type === 'watch.stopped') { setWatchActive(false); setWatchPaused(false) }
    if (event.type === 'representation.updated') {
      const data = event.data as Partial<WatchRepresentationSummary> | undefined
      if ('diffs' in (data ?? {})) setDiffs(normalizeDiffs(data?.diffs))
      refreshWorkspace(event)
    }
    if (event.type === 'version.created') {
      const version = event.data as Partial<WatchVersion> | undefined
      const targetRepoId = event.repository_id || version?.repository_id || watchLock?.repository_id || watchRepository?.id || 0
      clearPreview()
      setDiffs([])
      if (targetRepoId > 0) {
        setRepoId(targetRepoId)
        void selectLatestWatchVersion(targetRepoId)
      }
    }
    if (event.type !== 'watch.stopped' || watchActive) addLine(summarizeEvent(event))
  }, [watchActive, addLine, clearPreview, refreshWorkspace, selectLatestWatchVersion, watchLock?.repository_id, watchRepository?.id])

  useEffect(() => {
    let cancelled = false
    const poll = async () => {
      const status = await api.watch.status().catch(() => null)
      if (!status || cancelled) return
      setWatchActive(status.active)
      setWatchRepository(status.repository ?? null)
      setWatchLock(status.lock ?? null)
      setWatchPaused(status.lock?.status === 'paused')
    }
    void poll()
    const interval = window.setInterval(poll, 5000)
    return () => { cancelled = true; window.clearInterval(interval) }
  }, [])

  useEffect(() => {
    let disposed = false
    const connect = () => {
      if (disposed) return
      const socket = new WebSocket(api.watch.websocketUrl())
      socketRef.current = socket
      socket.onopen = () => setWatchConnected(true)
      socket.onclose = () => {
        setWatchConnected(false)
        if (!disposed) reconnectTimerRef.current = window.setTimeout(connect, 1500)
      }
      socket.onerror = () => socket.close()
      socket.onmessage = (msg) => {
        try { handleEvent(JSON.parse(msg.data) as WatchEvent) } catch { /* ignore */ }
      }
    }
    connect()
    return () => {
      disposed = true
      if (reconnectTimerRef.current !== null) window.clearTimeout(reconnectTimerRef.current)
      socketRef.current?.close()
      socketRef.current = null
    }
  }, [handleEvent])

  const sendControl = useCallback((type: 'watch.pause' | 'watch.resume' | 'watch.stop') => {
    const socket = socketRef.current
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    socket.send(JSON.stringify({ type, repository_id: watchLock?.repository_id ?? watchRepository?.id ?? 0 }))
    if (type === 'watch.pause') setWatchPaused(true)
    if (type === 'watch.resume') setWatchPaused(false)
    if (type === 'watch.stop') setWatchActive(false)
  }, [watchLock?.repository_id, watchRepository?.id])

  const watchStatusColor = !watchActive ? 'gray' : watchPaused ? 'yellow' : watchConnected ? 'green' : 'orange'
  const watchStatusLabel = !watchActive ? 'Stopped' : watchPaused ? 'Paused' : 'Live'
  const watchTitle = useMemo(() => shortPath(watchRepository?.repo_root), [watchRepository?.repo_root])
  const watchMode = [watcherMode || (watchConnected ? 'live' : 'connecting'), languages.length ? languages.join(', ') : ''].filter(Boolean).join(' · ')

  const showRuntimeSection = watchActive || watchLines.length > 0

  // ── Render ────────────────────────────────────────────────────────────────
  return (
    <Box
      data-zui-native-wheel="true"
      position="absolute"
      left={{ base: 3, md: 4 }}
      bottom={{ base: 'calc(var(--bottomnav-container-h) + 12px)', md: 4 }}
      zIndex={1001}
      pointerEvents="auto"
      w={{ base: 'calc(100vw - 24px)', md: '360px' }}
      maxW="360px"
      sx={{
        overscrollBehavior: 'contain',
        WebkitOverflowScrolling: 'touch',
        touchAction: 'pan-y',
      }}
    >
      <Box
        bg="rgba(var(--bg-main-rgb), 0.94)"
        border="1px solid"
        borderColor={preview ? 'rgba(var(--accent-rgb), 0.45)' : 'whiteAlpha.200'}
        borderRadius="lg"
        boxShadow="0 18px 48px rgba(0,0,0,0.45)"
        backdropFilter="blur(18px)"
        overflow="hidden"
      >
        {/* ── Versions header ── */}
        <VStack align="stretch" spacing={2} px={3} py={3}>
          <HStack spacing={2} justify="space-between" align="flex-start">
            <HStack spacing={2} minW={0} flex={1}>
              <Box
                display="inline-flex"
                alignItems="center"
                justifyContent="center"
                w="24px"
                h="24px"
                flexShrink={0}
                borderRadius="md"
                color={preview ? 'blue.200' : 'gray.400'}
                bg={preview ? 'blue.500' : 'whiteAlpha.100'}
                boxShadow={preview ? '0 0 12px rgba(66, 153, 225, 0.35)' : 'none'}
              >
                <TimeIcon boxSize={3.5} />
              </Box>
              <Box minW={0} flex={1}>
                {activeRepo?.display_name ?? 'Workspace'}

                <Text fontSize="10px" color="gray.500" noOfLines={1}>
                  {totalTldChanges > 0 ? `${totalTldChanges} changed elements and connectors` : 'Workspace versions'}
                </Text>
              </Box>
            </HStack>
            <HStack spacing={0.5}>
              {activeVersion && (
                <Tooltip label={diffVisible ? 'Hide diffs' : 'Show diffs'} placement="top">
                  <Button
                    aria-label={diffVisible ? 'Hide diffs' : 'Show diffs'}
                    leftIcon={diffVisible ? <ViewOffIcon boxSize={3} /> : <ViewIcon boxSize={3} />}
                    size="xs"
                    variant="outline"
                    h="24px"
                    px={2}
                    fontSize="10px"
                    color={diffVisible ? 'white' : 'whiteAlpha.700'}
                    borderColor={diffVisible ? 'rgba(var(--accent-rgb), 0.45)' : 'whiteAlpha.200'}
                    bg={diffVisible ? 'rgba(var(--accent-rgb), 0.18)' : 'whiteAlpha.50'}
                    _hover={{ bg: 'whiteAlpha.100', color: 'white', borderColor: 'whiteAlpha.300' }}
                    onClick={() => setDiffVisible((visible) => !visible)}
                  >
                    {diffVisible ? 'Hide diffs' : 'Show diffs'}
                  </Button>
                </Tooltip>
              )}
              <Tooltip label={versionsOpen ? 'Collapse versions' : 'Expand versions'} placement="top">
                <IconButton
                  aria-label="Workspace versions"
                  icon={<ChevronDownIcon boxSize={3} />}
                  size="xs"
                  variant="ghost"
                  color={versionsOpen ? 'var(--accent)' : 'whiteAlpha.700'}
                  transform={versionsOpen ? 'rotate(180deg)' : 'rotate(0deg)'}
                  transition="transform 0.2s, color 0.2s"
                  onClick={() => setVersionsOpen((v) => !v)}
                />
              </Tooltip>
            </HStack>
          </HStack>

          <Box
            px={2}
            py={2}
            border="1px solid"
            borderColor="whiteAlpha.100"
            borderRadius="md"
            bg="whiteAlpha.50"
          >
            <HStack spacing={1.5} fontFamily="mono" fontSize="10px" minW={0} opacity={0.85}>
              <Text color="gray.300" noOfLines={1} flex={1} minW={0}>{compactSummary}</Text>
            </HStack>
            <HStack spacing={2} mt={1.5} minW={0}>
              <Badge variant="subtle" colorScheme="blue" fontSize="8px" px={1}>TLD</Badge>
              <Text fontSize="10px" color="green.300" fontFamily="mono">+{diffSummary.elements.added} / {diffSummary.connectors.added}</Text>
              <Text fontSize="10px" color="red.300" fontFamily="mono">-{diffSummary.elements.deleted} / {diffSummary.connectors.deleted}</Text>
              <Text fontSize="10px" color="gray.500" noOfLines={1} flex={1} minW={0}>
                {activeDiffLocation ? `${activeDiffLocationIndex + 1}/${navigableDiffLocations.length} ${activeDiffLocation.label}` : `${totalTldChanges} changes`}
              </Text>
              <HStack spacing={0.5} flexShrink={0}>
                <Tooltip label="Previous changed element" placement="top">
                  <IconButton
                    aria-label="Previous changed element"
                    icon={<ChevronLeftIcon boxSize={3.5} />}
                    size="xs"
                    variant="ghost"
                    h="22px"
                    minW="22px"
                    color="whiteAlpha.700"
                    isDisabled={navigableDiffLocations.length === 0}
                    onClick={() => navigateDiffLocationByOffset(-1)}
                  />
                </Tooltip>
                <Tooltip label="Next changed element" placement="top">
                  <IconButton
                    aria-label="Next changed element"
                    icon={<ChevronRightIcon boxSize={3.5} />}
                    size="xs"
                    variant="ghost"
                    h="22px"
                    minW="22px"
                    color="whiteAlpha.700"
                    isDisabled={navigableDiffLocations.length === 0}
                    onClick={() => navigateDiffLocationByOffset(1)}
                  />
                </Tooltip>
              </HStack>
            </HStack>
          </Box>
        </VStack>

        {/* ── Versions body ── */}
        <Collapse in={versionsOpen} animateOpacity>
          <VStack align="stretch" spacing={2} px={3} pb={3} borderTop="1px solid" borderColor="whiteAlpha.100">
            <Box pt={2.5}>
              <ThemedSelect<number>
                value={repoId}
                placeholder="Select repository"
                options={repos.map((r) => ({ value: r.id, label: r.display_name }))}
                onChange={(v) => setRepoId(v)}
              />
            </Box>

            <ThemedSelect<number>
              value={versionId}
              placeholder="Select version"
              options={versions.map((v) => ({
                value: v.id,
                label: `${versionLabel(v)} · ${v.branch || 'detached'} · ${new Date(v.created_at).toLocaleString()}`,
              }))}
              onChange={(v) => setVersionId(v)}
            />

            <Box
              px={2}
              py={2}
              border="1px solid"
              borderColor="whiteAlpha.100"
              borderRadius="md"
              bg="whiteAlpha.50"
            >
              <Text fontSize="10px" color="gray.500" noOfLines={1}>
                {activeVersion ? `${activeVersion.branch || 'detached'} · ${new Date(activeVersion.created_at).toLocaleString()}` : activeRepo?.display_name ?? 'Repository'}
              </Text>
              <HStack spacing={1} mt={1} fontFamily="mono" fontSize="10px" minW={0} opacity={0.85}>
                <Text color="gray.300" noOfLines={1}>
                  {totalFileChanges} files changed
                </Text>
                <Text color="green.300">+{diffSummary.files.addedLines}</Text>
                <Text color="red.300">-{diffSummary.files.removedLines}</Text>
                <Text color="gray.500" ml="auto">{workspaceVersions.length} snapshots</Text>
              </HStack>
            </Box>

            {displayedDiffLocations.length > 0 && (
              <VStack
                data-zui-native-wheel="true"
                align="stretch"
                spacing={1}
                maxH="160px"
                overflowY="auto"
                borderTop="1px solid"
                borderColor="whiteAlpha.100"
                pt={2.5}
                sx={{ overscrollBehavior: 'contain', WebkitOverflowScrolling: 'touch', touchAction: 'pan-y' }}
              >
                {displayedDiffLocations.map((target) => (
                  <Button
                    key={target.key}
                    variant="ghost"
                    size="xs"
                    h="auto"
                    minH="28px"
                    justifyContent="flex-start"
                    px={2}
                    py={1}
                    fontSize="10px"
                    color={activeDiffLocationKey === target.key ? 'white' : 'gray.200'}
                    bg={activeDiffLocationKey === target.key ? 'whiteAlpha.100' : 'transparent'}
                    onClick={() => navigateToDiffLocation(target)}
                  >
                    <HStack w="full" spacing={2} minW={0}>
                      <Badge
                        colorScheme={target.changeType === 'added' ? 'green' : target.changeType === 'deleted' ? 'red' : 'yellow'}
                        fontSize="8px"
                      >
                        {target.resourceType}
                      </Badge>
                      <Box minW={0} flex={1} textAlign="left">
                        <Text noOfLines={1}>{target.summary || target.label}</Text>
                        <Text color="gray.500" noOfLines={1}>{target.viewName}</Text>
                      </Box>
                      {(target.addedLines > 0 || target.removedLines > 0) && (
                        <HStack spacing={1} flexShrink={0}>
                          {target.addedLines > 0 && <Text color="green.300">+{target.addedLines}</Text>}
                          {target.removedLines > 0 && <Text color="red.300">-{target.removedLines}</Text>}
                        </HStack>
                      )}
                    </HStack>
                  </Button>
                ))}
              </VStack>
            )}
          </VStack>
        </Collapse>

        {/* ── Runtime section (collapsible) ── */}
        {showRuntimeSection && (
          <>
            <HStack
              px={3}
              py={1.5}
              justify="space-between"
              borderTop="1px solid"
              borderColor="whiteAlpha.100"
              cursor="pointer"
              onClick={() => setRuntimeOpen((v) => !v)}
              _hover={{ bg: 'whiteAlpha.50' }}
              transition="background 0.15s"
            >
              <HStack spacing={2} minW={0} flex={1}>
                <Badge colorScheme={watchStatusColor} variant="subtle" borderRadius="md" fontSize="9px" px={1.5}>{watchStatusLabel}</Badge>
                <Text fontSize="10px" fontWeight="600" color="gray.300" noOfLines={1}>{watchTitle}</Text>
                {watchMode ? <Text fontSize="9px" color="gray.500" noOfLines={1}>{watchMode}</Text> : null}
              </HStack>
              <HStack spacing={0.5} onClick={(e) => e.stopPropagation()}>
                {watchActive && (
                  <>
                    <Tooltip label={watchPaused ? 'Resume watch' : 'Pause watch'} placement="top">
                      <IconButton
                        aria-label={watchPaused ? 'Resume watch' : 'Pause watch'}
                        icon={watchPaused ? <RepeatIcon boxSize={3} /> : <PauseGlyph />}
                        size="xs"
                        variant="ghost"
                        color="whiteAlpha.700"
                        onClick={() => sendControl(watchPaused ? 'watch.resume' : 'watch.pause')}
                      />
                    </Tooltip>
                    <Tooltip label="Stop watch" placement="top">
                      <IconButton
                        aria-label="Stop watch"
                        icon={<CloseIcon boxSize={2} />}
                        size="xs"
                        variant="ghost"
                        color="whiteAlpha.700"
                        onClick={() => sendControl('watch.stop')}
                      />
                    </Tooltip>
                  </>
                )}
                <IconButton
                  aria-label={runtimeOpen ? 'Collapse runtime' : 'Expand runtime'}
                  icon={<ChevronDownIcon boxSize={3} />}
                  size="xs"
                  variant="ghost"
                  color="whiteAlpha.400"
                  transform={runtimeOpen ? 'rotate(180deg)' : 'rotate(0deg)'}
                  transition="transform 0.2s, color 0.2s"
                  _hover={{ color: 'whiteAlpha.700', bg: 'whiteAlpha.100' }}
                  onClick={() => setRuntimeOpen((v) => !v)}
                />
              </HStack>
            </HStack>

            <Collapse in={runtimeOpen} animateOpacity>
              <VStack
                data-zui-native-wheel="true"
                align="stretch"
                spacing={0}
                maxH="140px"
                overflowY="auto"
                sx={{ overscrollBehavior: 'contain', WebkitOverflowScrolling: 'touch', touchAction: 'pan-y' }}
              >
                {watchLines.length === 0 ? (
                  <Text px={3} py={2} fontSize="11px" color="gray.500">Waiting for watch output…</Text>
                ) : watchLines.map((line) => (
                  <HStack key={line.id} px={3} py={1.5} spacing={2} borderTop="1px solid" borderColor="whiteAlpha.50" align="flex-start">
                    <Text fontSize="9px" color="gray.600" fontFamily="mono" flexShrink={0}>
                      {new Date(line.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                    </Text>
                    <Text
                      fontSize="11px"
                      color={line.tone === 'error' ? 'red.200' : line.tone === 'warning' ? 'yellow.200' : line.tone === 'success' ? 'green.200' : 'gray.400'}
                      noOfLines={2}
                    >
                      {line.text}
                    </Text>
                  </HStack>
                ))}
              </VStack>
            </Collapse>
          </>
        )}
      </Box>
    </Box>
  )
}
