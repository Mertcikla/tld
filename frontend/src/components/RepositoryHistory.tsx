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
  InputLeftElement,
  InputRightElement,
  Select,
  Spinner,
  Text,
  VStack,
} from '@chakra-ui/react'
import { ChevronUpIcon, SearchIcon } from '@chakra-ui/icons'
import {
  api,
  type RepositoryCommit,
  type RepositoryCommitDetails,
  type RepositoryGitHistory,
} from '../api/client'
import { GRAPH_LANE_COLORS, layoutCommitGraph, commitGraphPath } from '../utils/commitGraph'
import { ShortcutHint } from './PanelUI'
import { useRepositoryHistorySearch } from '../hooks/useRepositoryHistorySearch'

export default function RepositoryHistory({
  repositoryId,
  history: sourceHistory,
  branch = '',
  onBranchChange,
  base,
  head,
  collapsed,
  onToggle,
  onRange,
  footerContent,
  disabled = false,
  onLoadMore,
  loadingMore = false,
  loadMoreError = '',
}: {
  repositoryId: string
  history: RepositoryGitHistory | null
  branch?: string
  onBranchChange?: (branch: string) => void
  base: string
  head: string
  collapsed: boolean
  onToggle: () => void
  onRange: (base: RepositoryCommit, head: RepositoryCommit, branches?: { base: string; head: string }) => void
  footerContent?: ReactNode
  disabled?: boolean
  onLoadMore?: () => void
  loadingMore?: boolean
  loadMoreError?: string
}) {
  const [query, setQuery] = useState('')
  const [author, setAuthor] = useState('')
  const [tag, setTag] = useState('')
  const [filters, setFilters] = useState<{ authors: string[]; tags: string[] }>({ authors: [], tags: [] })
  const [filtersError, setFiltersError] = useState('')
  const search = useRepositoryHistorySearch(repositoryId, branch, sourceHistory?.headSha ?? '', query, author, tag)
  const history = search.active ? search.history : sourceHistory
  const historyLoading = search.active ? search.loading : loadingMore
  const historyError = search.active ? search.error : loadMoreError
  const fetchMore = search.active ? () => { void search.loadMore() } : onLoadMore
  const [pendingSelection, setPendingSelection] = useState<{
    commit: RepositoryCommit
    order: string[]
    repositoryId: string
    branch: string
  } | null>(null)
  const pending = pendingSelection?.repositoryId === repositoryId ? pendingSelection : null
  const [resolvingRange, setResolvingRange] = useState(false)
  const [selectionError, setSelectionError] = useState('')
  const rangeRequest = useRef<AbortController | null>(null)
  const selectionDisabled = disabled || resolvingRange
  useEffect(() => {
    setPendingSelection(null)
    setSelectionError('')
  }, [repositoryId])
  useEffect(() => {
    setResolvingRange(false)
    return () => {
      rangeRequest.current?.abort()
      rangeRequest.current = null
    }
  }, [repositoryId, branch, disabled])
  const [inspected, setInspected] = useState('')
  const [details, setDetails] = useState<RepositoryCommitDetails | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const graphScrollRef = useRef<HTMLDivElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const controller = new AbortController()
    setFilters({ authors: [], tags: [] })
    setFiltersError('')
    if (!sourceHistory?.isGit) return
    void api.repositories.historyFilters(repositoryId, branch, controller.signal).then((next) => {
      if (!controller.signal.aborted) setFilters(next)
    }).catch((err: unknown) => {
      if (!controller.signal.aborted) setFiltersError(err instanceof Error ? err.message : 'Could not load history filters')
    })
    return () => controller.abort()
  }, [repositoryId, branch, sourceHistory?.isGit, sourceHistory?.headSha, sourceHistory?.branches])
  const term = query.trim()
  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0
    if (graphScrollRef.current) graphScrollRef.current.scrollTop = 0
  }, [repositoryId, branch, term, author, tag])

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
  const baseIndex = layout.rows.findIndex((row) => row.commit.sha === base)
  const headIndex = layout.rows.findIndex((row) => row.commit.sha === head)
  const inRange = (index: number) => {
    const sha = layout.rows[index].commit.sha
    return sha === base || sha === head || (baseIndex >= 0 && headIndex >= 0 && index >= headIndex && index <= baseIndex)
  }
  const selectCommit = (commit: RepositoryCommit) => {
    if (selectionDisabled || rangeRequest.current) return
    setSelectionError('')
    if (!pending) {
      setPendingSelection({ commit, order: layout.rows.map((row) => row.commit.sha), repositoryId, branch: branch || sourceHistory?.currentBranch || '' })
      onRange(commit, commit)
      return
    }
    const finish = (pendingIsOlder: boolean) => {
      const currentBranch = branch || sourceHistory?.currentBranch || ''
      const older = pendingIsOlder ? pending.commit : commit
      const newer = pendingIsOlder ? commit : pending.commit
      if (pending.branch === currentBranch) onRange(older, newer)
      else onRange(older, newer, { base: pendingIsOlder ? pending.branch : currentBranch, head: pendingIsOlder ? currentBranch : pending.branch })
      setPendingSelection(null)
    }
    // Use Git's topological ordering whenever both commits appeared in the
    // same result set, including the one that contained the first selection.
    for (const order of [sourceHistory?.commits.map((item) => item.sha) ?? [], pending.order, layout.rows.map((row) => row.commit.sha)]) {
      const first = order.indexOf(pending.commit.sha)
      const second = order.indexOf(commit.sha)
      if (first >= 0 && second >= 0) {
        finish(first >= second)
        return
      }
    }
    // Separate search results may have no shared ordering. Ask Git which side
    // has commits beyond the other instead of relying on commit timestamps.
    const controller = new AbortController()
    rangeRequest.current = controller
    setResolvingRange(true)
    void api.repositories.rangeSummary(repositoryId, { base: pending.commit.sha, head: commit.sha, signal: controller.signal }).then((summary) => {
      if (!controller.signal.aborted) finish(summary.commits > 0)
    }).catch((err: unknown) => {
      if (!controller.signal.aborted) setSelectionError(err instanceof Error ? err.message : 'Could not select the range. Select the second commit to retry.')
    }).finally(() => {
      if (rangeRequest.current === controller) {
        rangeRequest.current = null
        setResolvingRange(false)
      }
    })
  }
  useEffect(() => {
    setInspected('')
    setQuery('')
    setAuthor('')
    setTag('')
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
      {!collapsed && <Flex
        px={4}
        py={3}
        gap={3}
        align="center"
        wrap="wrap"
        borderBottom="1px solid"
        borderColor="whiteAlpha.100"
      >
        <HStack spacing={3} rowGap={1} flexWrap="wrap" minW={0} flex="1 1 260px">
          <Button
            size="sm"
            variant="ghost"
            px={0}
            minW={0}
            h="32px"
            color="gray.200"
            fontWeight="semibold"
            rightIcon={<ChevronUpIcon color="gray.500" />}
            aria-label="Collapse commit history"
            aria-expanded={!collapsed}
            data-testid="repositories-history-summary"
            onClick={onToggle}
            _hover={{ color: 'white', bg: 'transparent' }}
            _focusVisible={{ boxShadow: '0 0 0 2px var(--accent)' }}
          >
            Commit history
          </Button>
          {history && (
            <Text fontSize="xs" color="gray.500" whiteSpace="nowrap" role={search.active ? 'status' : undefined}>
              {layout.rows.length}{history.hasMore ? '+' : ''}{search.active ? ' matching ' : ' '}{layout.rows.length === 1 && !history.hasMore ? 'commit' : 'commits'}
            </Text>
          )}
          {historyLoading && (
            <HStack role="status" spacing={2} minW={0}>
              <Spinner size="xs" color="gray.400" flexShrink={0} />
              <Text fontSize="xs" color="gray.400" isTruncated>{search.active && !history ? 'Searching commits…' : 'Loading older commits…'}</Text>
            </HStack>
          )}
        </HStack>
        <Flex gap={2} align="center" wrap="wrap" flex="1 1 720px" minW={0}>
          <InputGroup size="sm" flex="2 1 220px" minW={0}>
            <InputLeftElement pointerEvents="none" color="gray.500" h="32px">
              <SearchIcon boxSize={3} />
            </InputLeftElement>
            <Input
              ref={searchInputRef}
              size="sm"
              h="32px"
              fontSize="xs"
              borderRadius="md"
              bg="whiteAlpha.50"
              borderColor="whiteAlpha.200"
              _placeholder={{ color: 'gray.500' }}
              _hover={{ borderColor: 'whiteAlpha.400' }}
              placeholder="Search commits…"
              aria-label="Filter commits"
              value={query}
              pr={query ? 3 : 12}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Escape') { e.preventDefault(); e.currentTarget.blur() } }}
            />
            {!query && (
              <InputRightElement w="auto" h="32px" pr={2} pointerEvents="none">
                <ShortcutHint keys={['mod', 'K']} opacity={0.6} bg="whiteAlpha.100" color="gray.400" />
              </InputRightElement>
            )}
          </InputGroup>
          <Grid
            gap={2}
            flex="3 1 440px"
            minW={0}
            templateColumns={{ base: 'repeat(2, minmax(0, 1fr))', sm: 'minmax(0, 1.6fr) minmax(0, 1fr) minmax(0, 1fr)' }}
            sx={{
              select: {
                height: '32px',
                fontSize: 'xs',
                borderRadius: 'md',
                bg: 'whiteAlpha.50',
                borderColor: 'whiteAlpha.200',
                textOverflow: 'ellipsis',
                _hover: { borderColor: 'whiteAlpha.400' },
                '&[data-active=true]': { borderColor: 'var(--accent)' },
              },
            }}
          >
            <Box minW={0} gridColumn={{ base: '1 / -1', sm: 'auto' }}>
              <Select size="sm" minW={0} aria-label="Filter history by branch" title={branch || sourceHistory?.currentBranch || 'Current branch'} value={branch} isDisabled={!onBranchChange} onChange={(event) => onBranchChange?.(event.target.value)}>
                <option value="">{sourceHistory?.currentBranch || 'Current branch'}</option>
                {branch && !sourceHistory?.branches.some((item) => item.name === branch) && <option value={branch}>{branch}</option>}
                {sourceHistory?.branches.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
              </Select>
            </Box>
            <Select size="sm" minW={0} aria-label="Filter history by author" title={author || 'All authors'} value={author} data-active={author ? 'true' : undefined} onChange={(event) => setAuthor(event.target.value)}>
              <option value="">All authors</option>
              {filters.authors.map((item) => <option key={item} value={item}>{item}</option>)}
            </Select>
            <Select size="sm" minW={0} aria-label="Filter history by tag" title={tag || 'All tags'} value={tag} data-active={tag ? 'true' : undefined} onChange={(event) => setTag(event.target.value)}>
              <option value="">All tags</option>
              {filters.tags.map((item) => <option key={item} value={item}>{item}</option>)}
            </Select>
          </Grid>
          {(query || author || tag) && (
            <Button size="xs" variant="ghost" color="gray.400" flexShrink={0} onClick={() => { setQuery(''); setAuthor(''); setTag('') }}>
              Clear filters
            </Button>
          )}
        </Flex>
      </Flex>}
      {!collapsed && (
        <>
          {selectionError && <Text px={4} py={2} fontSize="xs" color="red.300" role="alert">{selectionError}</Text>}
          {filtersError && <Text px={4} py={2} fontSize="xs" color="red.300" role="alert">{filtersError}</Text>}
          {!search.active && !history?.isGit && (
            <Text px={4} py={3} fontSize="sm" color="gray.500">
              Git history is unavailable. Recorded snapshots can still be
              compared.
            </Text>
          )}
          {!historyLoading && !historyError && (search.active || history?.isGit) && !layout.rows.length && (
            <Text px={4} py={3} fontSize="sm" color="gray.500">
              {search.active ? 'No matching commits.' : 'No commits yet.'}
            </Text>
          )}
          <Flex
            ref={scrollRef}
            maxH="240px"
            overflowY="auto"
            data-testid="repositories-history-scroll"
            onScroll={(event) => {
              const { scrollTop, scrollHeight, clientHeight } = event.currentTarget
              if (graphScrollRef.current) graphScrollRef.current.scrollTop = scrollTop
              if (history?.nextPageToken && !historyLoading && scrollHeight - scrollTop <= clientHeight + 1) fetchMore?.()
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
                  aria-disabled={selectionDisabled}
                  data-testid={`commit-row-${commit.sha}`}
                  tabIndex={selectionDisabled ? -1 : 0}
                  cursor={selectionDisabled ? 'default' : 'pointer'}
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
          {historyError && (
            <Flex px={4} py={2} gap={3} align="center" justify="flex-end">
              <Text flex={1} fontSize="xs" color="red.300" role="alert">{historyError}{!search.active && ' · Scroll to the bottom to retry.'}</Text>
              {search.active && <Button size="xs" variant="ghost" onClick={() => { if (search.history) void search.loadMore(); else search.retry() }}>Retry search</Button>}
            </Flex>
          )}
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
          gap={4}
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
            icon={<ChevronUpIcon boxSize="24px" transform={collapsed ? 'rotate(180deg)' : undefined} transition="transform 0.2s" />}
            onClick={onToggle}
          />
          <Box minW={0}>{footerContent}</Box>
        </Grid>
      )}
    </Box>
  )
}
