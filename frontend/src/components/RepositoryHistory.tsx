import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import {
  Badge,
  Box,
  Button,
  Code,
  Flex,
  Grid,
  HStack,
  IconButton,
  Input,
  InputGroup,
  InputRightElement,
  Spinner,
  Text,
  VStack,
} from '@chakra-ui/react'
import { ChevronDownIcon } from '@chakra-ui/icons'
import {
  api,
  type RepositoryCommit,
  type RepositoryCommitDetails,
  type RepositoryGitHistory,
} from '../api/client'
import { GRAPH_LANE_COLORS, layoutCommitGraph, commitGraphPath } from '../utils/commitGraph'
import { ShortcutHint } from './PanelUI'

export default function RepositoryHistory({
  repositoryId,
  history,
  base,
  head,
  collapsed,
  onToggle,
  onRange,
  footerContent,
  disabled = false,
}: {
  repositoryId: string
  history: RepositoryGitHistory | null
  base: string
  head: string
  collapsed: boolean
  onToggle: () => void
  onRange: (base: RepositoryCommit, head: RepositoryCommit) => void
  footerContent?: ReactNode
  disabled?: boolean
}) {
  const [query, setQuery] = useState('')
  const [pendingCommit, setPendingCommit] = useState('')
  const [inspected, setInspected] = useState('')
  const [details, setDetails] = useState<RepositoryCommitDetails | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const graphScrollRef = useRef<HTMLDivElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (typeof window === 'undefined') return
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null
      const isInput = target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' || target?.isContentEditable
      if (isInput) return
      if (e.key.toLowerCase() !== 'k' || (!e.metaKey && !e.ctrlKey) || e.altKey || e.shiftKey) return
      e.preventDefault()
      if (collapsed) onToggle()
      window.setTimeout(() => searchInputRef.current?.focus(), 50)
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [collapsed, onToggle])
  const layout = useMemo(
    () => layoutCommitGraph(history?.commits ?? []),
    [history],
  )
  useEffect(() => { setPendingCommit('') }, [repositoryId, history])
  const baseIndex = layout.rows.findIndex((row) => row.commit.sha === base)
  const headIndex = layout.rows.findIndex((row) => row.commit.sha === head)
  const inRange = (index: number) => baseIndex >= 0 && headIndex >= 0 && index >= headIndex && index <= baseIndex
  const selectCommit = (commit: RepositoryCommit) => {
    if (disabled) return
    const pending = layout.rows.find((row) => row.commit.sha === pendingCommit)?.commit
    if (!pending) {
      setPendingCommit(commit.sha)
      onRange(commit, commit)
    } else {
      const firstIndex = layout.rows.findIndex((row) => row.commit.sha === pending.sha)
      const secondIndex = layout.rows.findIndex((row) => row.commit.sha === commit.sha)
      onRange(firstIndex >= secondIndex ? pending : commit, firstIndex >= secondIndex ? commit : pending)
      setPendingCommit('')
    }
  }
  useEffect(() => {
    setInspected('')
    setQuery('')
  }, [repositoryId])
  useEffect(() => {
    let cancelled = false
    setDetails(null)
    setError('')
    setLoading(false)
    if (!inspected) return
    setLoading(true)
    api.repositories
      .commitDetails(repositoryId, inspected)
      .then((next) => {
        if (!cancelled) setDetails(next)
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(
            err instanceof Error
              ? err.message
              : 'Could not load commit details',
          )
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [repositoryId, inspected])
  const rowHeight = 36
  const graphWidth = Math.max(44, layout.laneCount * 18 + 28)
  const historyHeight = layout.rows.length * rowHeight
  const x = (lane: number) => 18 + lane * 18
  const y = (row: number) => row * rowHeight + rowHeight / 2
  const color = (lane: number) =>
    GRAPH_LANE_COLORS[lane % GRAPH_LANE_COLORS.length]
  return (
    <Box borderBottom="1px solid" borderColor="whiteAlpha.100">
      {!collapsed && <Grid
        px={4}
        h="40px"
        gap={{ base: 2, md: 3 }}
        alignItems="center"
        cursor="pointer"
        role="button"
        tabIndex={0}
        aria-expanded={!collapsed}
        data-testid="repositories-history-summary"
        onClick={onToggle}
        onKeyDown={(event) => {
          if (event.target !== event.currentTarget) return
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            onToggle()
          }
        }}
        _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '-2px' }}
        templateColumns="minmax(0, 1fr) minmax(0, 200px) minmax(0, 1fr)"
      >
        <HStack spacing={1} minW={0} justifySelf="start">
          <Text fontSize="sm" fontWeight="semibold" color="gray.200" isTruncated>
            Commit history
          </Text>
        </HStack>
        <Box minW={0} justifySelf="center" w="full" onClick={(event) => event.stopPropagation()}>
            <InputGroup size="xs" w="full" maxW="200px">
              <Input
                ref={searchInputRef}
                size="xs"
                placeholder="Search commits…"
                aria-label="Filter commits"
                value={query}
                pr={query ? undefined : 8}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Escape') { e.preventDefault(); e.currentTarget.blur() } }}
              />
              {!query && (
                <InputRightElement w="auto" pr={2} pointerEvents="none">
                  <ShortcutHint keys={['mod', 'K']} opacity={0.6} />
                </InputRightElement>
              )}
            </InputGroup>
        </Box>
        <HStack spacing={2} minW={0} justifySelf="end" onClick={(event) => event.stopPropagation()}>
          <Text fontSize="xs" color="gray.500" whiteSpace="nowrap">
            {layout.rows.length} commits
          </Text>
        </HStack>
      </Grid>}
      {!collapsed && (
        <>
          {!history?.isGit && (
            <Text px={4} py={3} fontSize="sm" color="gray.500">
              Git history is unavailable. Recorded snapshots can still be
              compared.
            </Text>
          )}
          {history?.isGit && !layout.rows.length && (
            <Text px={4} py={3} fontSize="sm" color="gray.500">
              No commits yet.
            </Text>
          )}
          <Flex
            maxH="240px"
            overflowY="auto"
            onScroll={(event) => {
              if (graphScrollRef.current) graphScrollRef.current.scrollTop = event.currentTarget.scrollTop
            }}
          >
            <Box
              flexShrink={0}
              h={`${historyHeight}px`}
              borderRight="1px solid"
              borderColor="whiteAlpha.200"
              w={`${Math.min(graphWidth, 140)}px`}
              maxW={{ base: '80px', md: '140px' }}
            >
              <Box
                ref={graphScrollRef}
                position="sticky"
                top={0}
                h={`${Math.min(historyHeight, 240)}px`}
                overflowX="auto"
                overflowY="hidden"
                tabIndex={0}
                role="region"
                aria-label="Scrollable commit graph"
                _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '-2px' }}
              >
                <svg
                  aria-label="Commit graph"
                  style={{ display: 'block' }}
                  width={graphWidth}
                  height={historyHeight}
                >
                  {layout.rows.map((row, i) => inRange(i) && (
                    <rect key={row.commit.sha} x={0} y={i * rowHeight} width="100%" height={rowHeight} fill="rgba(var(--accent-rgb), 0.12)" />
                  ))}
                  {layout.edges.map((edge, i) => (
                    <path
                      key={i}
                      d={commitGraphPath(edge, layout.rows.length, rowHeight)}
                      stroke={color(edge.railLane)}
                      strokeWidth={2}
                      fill="none"
                    />
                  ))}
                  {layout.rows.map((row, i) => (
                    <g key={row.commit.sha}>
                      <circle
                        cx={x(row.lane)}
                        cy={y(i)}
                        r={row.isMerge ? 6 : 4}
                        fill={color(row.lane)}
                      >
                        <title>{row.commit.subject}</title>
                      </circle>
                      {(base === row.commit.sha || head === row.commit.sha) && (
                        <circle
                          cx={x(row.lane)}
                          cy={y(i)}
                          r={9}
                          fill="none"
                          stroke={base === row.commit.sha ? '#A0AEC0' : '#68D391'}
                        />
                      )}
                    </g>
                  ))}
                </svg>
              </Box>
            </Box>
            <VStack spacing={0} flex={1} minW={0} align="stretch">
              {layout.rows.map(({ commit }, index) => (
                <Flex
                  key={commit.sha}
                  role="group"
                  aria-label={`Select commit ${commit.sha.slice(0, 7)}`}
                  aria-disabled={disabled}
                  data-testid={`commit-row-${commit.sha}`}
                  tabIndex={disabled ? -1 : 0}
                  cursor={disabled ? 'default' : 'pointer'}
                  onClick={() => selectCommit(commit)}
                  onKeyDown={(event) => {
                    if (event.target !== event.currentTarget) return
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault()
                      selectCommit(commit)
                    }
                  }}
                  _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '-2px' }}
                  h={`${rowHeight}px`}
                  flexShrink={0}
                  px={2}
                  align="center"
                  gap={2}
                  borderBottom="1px solid"
                  borderColor="whiteAlpha.50"
                  opacity={
                    [commit.sha, commit.subject, commit.author, ...commit.refs]
                      .join(' ')
                      .toLowerCase()
                      .includes(query.toLowerCase())
                      ? 1
                      : 0.3
                  }
                  bg={
                    inRange(index)
                      ? 'rgba(var(--accent-rgb), 0.12)'
                      : undefined
                  }
                >
                  <Button
                    variant="link"
                    size="xs"
                    fontFamily="mono"
                    color="gray.300"
                    onClick={(event) => {
                      event.stopPropagation()
                      setInspected(inspected === commit.sha ? '' : commit.sha)
                    }}
                    aria-label={`Inspect ${commit.sha.slice(0, 7)}`}
                  >
                    {commit.sha.slice(0, 7)}
                  </Button>
                  <Text
                    fontSize="sm"
                    color="gray.100"
                    isTruncated
                    flex={1}
                    title={commit.subject}
                  >
                    {commit.subject}
                  </Text>
                  <HStack display={{ base: 'none', md: 'flex' }} spacing={1}>
                    {commit.refs.map((ref) => (
                      <Badge
                        key={ref}
                        fontSize="2xs"
                        colorScheme={ref.startsWith('tag:') ? 'purple' : 'blue'}
                        maxW="130px"
                        isTruncated
                        title={ref}
                      >
                        {ref.replace('HEAD -> ', '')}
                      </Badge>
                    ))}
                  </HStack>
                  <Text
                    fontSize="xs"
                    color="gray.500"
                    display={{ base: 'none', xl: 'block' }}
                  >
                    {commit.author} ·{' '}
                    {new Date(commit.createdUnix * 1000).toLocaleDateString()}
                  </Text>
                  {(commit.sha === base || commit.sha === head) && (
                    <HStack spacing={2} flexShrink={0}>
                      {commit.sha === base && (
                        <Text fontSize="xs" color="gray.400">Base</Text>
                      )}
                      {commit.sha === head && (
                        <Text fontSize="xs" color="green.300">Head</Text>
                      )}
                    </HStack>
                  )}
                </Flex>
              ))}
            </VStack>
          </Flex>
          {inspected && (
            <Box p={4} bg="whiteAlpha.50">
              <Flex gap={3} align="center">
                <Code fontSize="xs">{inspected}</Code>
                <Box flex={1} />
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() => setInspected('')}
                >
                  Close details
                </Button>
              </Flex>
              {loading && <Spinner size="sm" />}
              {error && (
                <Text color="red.300" fontSize="sm">
                  {error}
                </Text>
              )}
              {details?.commit && (
                <>
                  <Text mt={2} fontSize="sm" fontWeight="semibold">
                    {details.commit.subject}
                  </Text>
                  <Text fontSize="xs" color="gray.400" whiteSpace="pre-wrap">
                    {details.commit.body}
                  </Text>
                </>
              )}
              {details?.files.map((file) => (
                <Flex key={file.path} gap={3} mt={1} fontSize="xs">
                  <Text flex={1} isTruncated>
                    {file.path}
                  </Text>
                  <Text color="green.300">
                    {file.binary ? 'binary' : `+${file.added}`}
                  </Text>
                  {!file.binary && <Text color="red.300">−{file.removed}</Text>}
                </Flex>
              ))}
            </Box>
          )}
        </>
      )}
      {footerContent && (
        <Grid
          px={4}
          py={2}
          gap={2}
          alignItems="center"
          templateColumns="24px minmax(0, 1fr)"
          borderTop="1px solid"
          borderColor="whiteAlpha.100"
          data-testid="repositories-history-footer"
        >
          <IconButton
            aria-label={collapsed ? 'Expand commit history' : 'Collapse commit history'}
            size="xs"
            variant="ghost"
            flexShrink={0}
            icon={<ChevronDownIcon boxSize="14px" transform={collapsed ? 'rotate(-90deg)' : undefined} transition="transform 0.2s" />}
            onClick={onToggle}
          />
          <Box minW={0}>{footerContent}</Box>
        </Grid>
      )}
    </Box>
  )
}
