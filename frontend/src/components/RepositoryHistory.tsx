import { useEffect, useMemo, useState } from 'react'
import {
  Badge,
  Box,
  Button,
  Code,
  Flex,
  HStack,
  Input,
  Spinner,
  Text,
  VStack,
} from '@chakra-ui/react'
import {
  api,
  type RepositoryCommit,
  type RepositoryCommitDetails,
  type RepositoryGitHistory,
} from '../api/client'
import { GRAPH_LANE_COLORS, layoutCommitGraph, commitGraphPath } from '../utils/commitGraph'

export default function RepositoryHistory({
  repositoryId,
  history,
  base,
  head,
  collapsed,
  onToggle,
  onBase,
  onHead,
  onRange,
  disabled = false,
}: {
  repositoryId: string
  history: RepositoryGitHistory | null
  base: string
  head: string
  collapsed: boolean
  onToggle: () => void
  onBase: (commit: RepositoryCommit) => void
  onHead: (commit: RepositoryCommit) => void
  onRange: (base: RepositoryCommit, head: RepositoryCommit) => void
  disabled?: boolean
}) {
  const [query, setQuery] = useState('')
  const [pendingCommit, setPendingCommit] = useState('')
  const [inspected, setInspected] = useState('')
  const [details, setDetails] = useState<RepositoryCommitDetails | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
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
  const x = (lane: number) => 18 + lane * 18
  const y = (row: number) => row * rowHeight + rowHeight / 2
  const color = (lane: number) =>
    GRAPH_LANE_COLORS[lane % GRAPH_LANE_COLORS.length]
  return (
    <Box borderBottom="1px solid" borderColor="whiteAlpha.100">
      <Flex px={4} h="40px" gap={3} align="center">
        <Button
          size="xs"
          variant="ghost"
          onClick={onToggle}
          aria-expanded={!collapsed}
        >
          Commit history
        </Button>
        <Text fontSize="xs" color="gray.500">
          {layout.rows.length} commits
        </Text>
        <Box flex={1} />
        {!collapsed && (
          <Input
            size="xs"
            maxW="200px"
            placeholder="Filter commits…"
            aria-label="Filter commits"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        )}
      </Flex>
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
          <Flex maxH="240px" overflowY="auto">
            <Box
              flexShrink={0}
              borderRight="1px solid"
              borderColor="whiteAlpha.200"
              w={`${Math.max(44, layout.laneCount * 18 + 28)}px`}
            >
              <svg
                aria-label="Commit graph"
                width="100%"
                height={layout.rows.length * rowHeight}
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
                  sx={{
                    '&:hover > .commit-target': {
                      opacity: 1,
                      pointerEvents: 'auto',
                    },
                    '@media (hover: none)': {
                      '> .commit-target': { opacity: 1, pointerEvents: 'auto' },
                    },
                  }}
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
                  <Button
                    className="commit-target"
                    size="xs"
                    variant={commit.sha === base ? 'solid' : 'ghost'}
                    opacity={commit.sha === base ? 1 : 0}
                    pointerEvents={commit.sha === base ? 'auto' : 'none'}
                    aria-pressed={commit.sha === base}
                    _focusVisible={{ opacity: 1, pointerEvents: 'auto' }}
                    isDisabled={disabled || (headIndex >= 0 && index < headIndex)}
                    onClick={(event) => {
                      event.stopPropagation()
                      if (disabled || (headIndex >= 0 && index < headIndex)) return
                      setPendingCommit('')
                      onBase(commit)
                    }}
                    aria-label={`Set ${commit.sha.slice(0, 7)} as base`}
                  >
                    Base
                  </Button>
                  <Button
                    className="commit-target"
                    size="xs"
                    variant={commit.sha === head ? 'solid' : 'ghost'}
                    colorScheme={commit.sha === head ? 'green' : undefined}
                    opacity={commit.sha === head ? 1 : 0}
                    pointerEvents={commit.sha === head ? 'auto' : 'none'}
                    aria-pressed={commit.sha === head}
                    _focusVisible={{ opacity: 1, pointerEvents: 'auto' }}
                    isDisabled={disabled || (baseIndex >= 0 && index > baseIndex)}
                    onClick={(event) => {
                      event.stopPropagation()
                      if (disabled || (baseIndex >= 0 && index > baseIndex)) return
                      setPendingCommit('')
                      onHead(commit)
                    }}
                    aria-label={`Set ${commit.sha.slice(0, 7)} as head`}
                  >
                    Head
                  </Button>
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
    </Box>
  )
}
