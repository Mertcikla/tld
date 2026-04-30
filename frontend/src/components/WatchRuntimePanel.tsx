import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  Badge,
  Box,
  HStack,
  IconButton,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import { CloseIcon, RepeatIcon } from '@chakra-ui/icons'
import { api, type WatchDiff, type WatchEvent, type WatchLock, type WatchRepository, type WatchVersion } from '../api/client'

export const WATCH_REPRESENTATION_UPDATED_EVENT = 'tld:watch-representation-updated'

type WatchLine = {
  id: number
  at: string
  text: string
  tone: 'info' | 'success' | 'warning' | 'error'
}

function PauseGlyph() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
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
  if (type === 'version.created' || type === 'representation.updated') return null
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

function summarizeDiff(diff: WatchDiff, at: string, index: number): WatchLine | null {
  const target = diff.resource_type === 'connector'
    ? connectorLabel(diff)
    : shortPath(diff.summary || diff.owner_key)
  if (!target || diff.owner_type === 'repository') return null

  const added = Math.max(0, diff.added_lines ?? 0)
  const removed = Math.max(0, diff.removed_lines ?? 0)
  const delta = [
    added > 0 ? `+${added}` : '',
    removed > 0 ? `-${removed}` : '',
  ].filter(Boolean).join(' ')
  const verb = diff.change_type === 'added' ? 'added' : diff.change_type === 'deleted' ? 'removed' : 'changed'
  const text = `${verb} ${target}${delta ? ` ${delta}` : ''}`
  return {
    id: Date.now() + index + Math.random(),
    at,
    text,
    tone: diff.change_type === 'deleted' ? 'warning' : 'success',
  }
}

function connectorLabel(diff: WatchDiff): string {
  const value = (diff.summary || diff.owner_key || '').trim()
  if (!value) return 'connector'
  return value
}

function shortPath(path: string | undefined): string {
  if (!path) return 'repository'
  const parts = path.split(/[\\/]/).filter(Boolean)
  return parts.slice(-2).join('/') || path
}

export default function WatchRuntimePanel() {
  const queryClient = useQueryClient()
  const socketRef = useRef<WebSocket | null>(null)
  const reconnectTimerRef = useRef<number | null>(null)
  const lastRepresentationHashRef = useRef('')
  const [active, setActive] = useState(false)
  const [paused, setPaused] = useState(false)
  const [repository, setRepository] = useState<WatchRepository | null>(null)
  const [lock, setLock] = useState<WatchLock | null>(null)
  const [connected, setConnected] = useState(false)
  const [watcherMode, setWatcherMode] = useState('')
  const [languages, setLanguages] = useState<string[]>([])
  const [lines, setLines] = useState<WatchLine[]>([])

  const addLine = useCallback((line: WatchLine | null) => {
    if (!line) return
    setLines((current) => {
      if (current[0]?.text === line.text && current[0]?.tone === line.tone) return current
      return [line, ...current].slice(0, 8)
    })
  }, [])

  const addLines = useCallback((nextLines: WatchLine[]) => {
    if (nextLines.length === 0) return
    setLines((current) => {
      const merged = [...nextLines, ...current]
      const seen = new Set<string>()
      return merged.filter((line) => {
        const key = `${line.text}:${line.tone}`
        if (seen.has(key)) return false
        seen.add(key)
        return true
      }).slice(0, 8)
    })
  }, [])

  const refreshWorkspace = useCallback((event: WatchEvent) => {
    const data = event.data as { representation_hash?: string } | undefined
    const hash = data?.representation_hash ?? ''
    if (hash && hash === lastRepresentationHashRef.current) return
    if (hash) lastRepresentationHashRef.current = hash

    void queryClient.invalidateQueries({ queryKey: ['workspace', 'views'] })
    void queryClient.invalidateQueries({ queryKey: ['elements', 'list'] })
    window.dispatchEvent(new CustomEvent(WATCH_REPRESENTATION_UPDATED_EVENT, { detail: event }))
  }, [queryClient])

  const loadVersionDiffLines = useCallback(async (event: WatchEvent) => {
    const data = event.data as { diffs?: WatchDiff[] } | undefined
    let diffs = data?.diffs ?? []
    if (event.type === 'version.created' && diffs.length === 0) {
      const version = event.data as Partial<WatchVersion> | undefined
      if (!version?.id) return
      diffs = await api.watch.diffs(version.id).catch(() => [])
    }
    const preferred = diffs.filter((diff) => diff.resource_type === 'file' || diff.resource_type === 'connector')
    const displayDiffs = preferred.length > 0 ? preferred : diffs.filter((diff) => diff.owner_type !== 'repository')
    addLines(displayDiffs.map((diff, index) => summarizeDiff(diff, event.at, index)).filter((line): line is WatchLine => Boolean(line)))
  }, [addLines])

  const handleEvent = useCallback((event: WatchEvent) => {
    const eventLock = event.data && typeof event.data === 'object' && 'status' in event.data
      ? event.data as WatchLock
      : null

    if (event.repository_id) {
      setLock((current) => eventLock ?? current)
    }
    if (eventLock) setPaused(eventLock.status === 'paused')
    if (event.watcher_mode) setWatcherMode(event.watcher_mode)
    if (event.languages?.length) setLanguages(event.languages)
    if (event.type === 'watch.paused') setPaused(true)
    if (event.type === 'watch.heartbeat') {
      setActive(true)
      if (eventLock) setPaused(eventLock.status === 'paused')
    }
    if (event.type === 'watch.stopped') {
      setActive(false)
      setPaused(false)
    }
    if (event.type === 'representation.updated') refreshWorkspace(event)
    if (event.type === 'representation.updated' || event.type === 'version.created') void loadVersionDiffLines(event)

    if (event.type !== 'watch.stopped' || active) addLine(summarizeEvent(event))
  }, [active, addLine, loadVersionDiffLines, refreshWorkspace])

  useEffect(() => {
    let cancelled = false
    const loadStatus = async () => {
      const status = await api.watch.status().catch(() => null)
      if (!status || cancelled) return
      setActive(status.active)
      setRepository(status.repository ?? null)
      setLock(status.lock ?? null)
      setPaused(status.lock?.status === 'paused')
    }
    void loadStatus()
    const interval = window.setInterval(loadStatus, 5000)
    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [])

  useEffect(() => {
    let disposed = false

    const connect = () => {
      if (disposed) return
      const socket = new WebSocket(api.watch.websocketUrl())
      socketRef.current = socket
      socket.onopen = () => setConnected(true)
      socket.onclose = () => {
        setConnected(false)
        if (!disposed) reconnectTimerRef.current = window.setTimeout(connect, 1500)
      }
      socket.onerror = () => socket.close()
      socket.onmessage = (message) => {
        try {
          handleEvent(JSON.parse(message.data) as WatchEvent)
        } catch {
          // Ignore malformed websocket frames.
        }
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
    socket.send(JSON.stringify({ type, repository_id: lock?.repository_id ?? repository?.id ?? 0 }))
    if (type === 'watch.pause') setPaused(true)
    if (type === 'watch.resume') setPaused(false)
    if (type === 'watch.stop') setActive(false)
  }, [lock?.repository_id, repository?.id])

  const statusColor = !active ? 'gray' : paused ? 'yellow' : connected ? 'green' : 'orange'
  const statusLabel = !active ? 'Stopped' : paused ? 'Paused' : 'Watching'
  const title = useMemo(() => shortPath(repository?.repo_root), [repository?.repo_root])
  const mode = [watcherMode || (connected ? 'live' : 'connecting'), languages.length ? languages.join(', ') : ''].filter(Boolean).join(' · ')

  if (!active && lines.length === 0) return null

  return (
    <Box
      position="absolute"
      right={{ base: 3, md: 4 }}
      bottom={{ base: 'calc(var(--bottomnav-container-h) + 12px)', md: 4 }}
      zIndex={1000}
      w={{ base: 'calc(100vw - 24px)', md: '360px' }}
      maxW="360px"
      bg="rgba(var(--bg-main-rgb), 0.94)"
      border="1px solid"
      borderColor="whiteAlpha.200"
      borderRadius="lg"
      boxShadow="0 18px 48px rgba(0,0,0,0.45)"
      backdropFilter="blur(18px)"
      overflow="hidden"
    >
      <HStack px={3} py={2.5} justify="space-between" borderBottom="1px solid" borderColor="whiteAlpha.100">
        <HStack spacing={2} minW={0}>
          <Badge colorScheme={statusColor} variant="subtle" borderRadius="md">{statusLabel}</Badge>
              <Text fontSize="12px" fontWeight="700" color="gray.100" noOfLines={1}>{title}</Text>
              <Text fontSize="10px" color="gray.500" noOfLines={1}>{mode}</Text>
        </HStack>
        <HStack spacing={1}>
          {active && (
            <>
              <Tooltip label={paused ? 'Resume watch' : 'Pause watch'} placement="top">
                <IconButton
                  aria-label={paused ? 'Resume watch' : 'Pause watch'}
                  icon={paused ? <RepeatIcon boxSize={3.5} /> : <PauseGlyph />}
                  size="xs"
                  variant="ghost"
                  color="whiteAlpha.800"
                  onClick={() => sendControl(paused ? 'watch.resume' : 'watch.pause')}
                />
              </Tooltip>
              <Tooltip label="Stop watch" placement="top">
                <IconButton
                  aria-label="Stop watch"
                  icon={<CloseIcon boxSize={2.5} />}
                  size="xs"
                  variant="ghost"
                  color="whiteAlpha.800"
                  onClick={() => sendControl('watch.stop')}
                />
              </Tooltip>
            </>
          )}
        </HStack>
      </HStack>
      <VStack align="stretch" spacing={0} maxH="180px" overflowY="auto">
        {lines.length === 0 ? (
          <Text px={3} py={3} fontSize="12px" color="gray.400">Waiting for watch output...</Text>
        ) : lines.map((line) => (
          <HStack key={line.id} px={3} py={2} spacing={2} borderTop="1px solid" borderColor="whiteAlpha.50" align="flex-start">
            <Text fontSize="10px" color="gray.500" fontFamily="mono" flexShrink={0}>
              {new Date(line.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
            </Text>
            <Text fontSize="12px" color={line.tone === 'error' ? 'red.200' : line.tone === 'warning' ? 'yellow.200' : line.tone === 'success' ? 'green.200' : 'gray.300'} noOfLines={2}>
              {line.text}
            </Text>
          </HStack>
        ))}
      </VStack>
    </Box>
  )
}
