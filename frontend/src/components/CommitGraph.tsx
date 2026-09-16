import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Badge,
  Box,
  Button,
  Code,
  Flex,
  HStack,
  IconButton,
  Input,
  InputGroup,
  InputLeftElement,
  InputRightElement,
  Spinner,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import { CopyIcon, SearchIcon, SmallCloseIcon } from '@chakra-ui/icons'
import {
  api,
  type ImpactCommit,
  type ImpactCommitDetails,
  type ImpactRangeStats,
} from '../api/client'
import { toast } from '../utils/toast'
import { GRAPH_LANE_COLORS, layoutCommitGraph, parseCommitRefs } from '../utils/commitGraph'

const ROW_H = 32
const LANE_W = 18
const RAIL_PAD = 10
const HISTORY_LIMIT = 100
const HISTORY_MAX = 200

function laneColor(lane: number): string {
  return GRAPH_LANE_COLORS[lane % GRAPH_LANE_COLORS.length]
}

function rowMatches(commit: ImpactCommit, query: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  return (
    commit.subject.toLowerCase().includes(q) ||
    commit.author.toLowerCase().includes(q) ||
    commit.sha.toLowerCase().includes(q) ||
    commit.short_sha.toLowerCase().includes(q)
  )
}

function RefBadges({ commit }: { commit: ImpactCommit }) {
  const parsed = useMemo(() => parseCommitRefs(commit.refs), [commit.refs])
  if (!commit.refs.length) return null
  return (
    <HStack spacing={1} flexShrink={0}>
      {parsed.isHead && (
        <Badge variant="subtle" colorScheme="green" fontSize="2xs" borderRadius="full" px={1.5}>
          HEAD
        </Badge>
      )}
      {parsed.branches.slice(0, 2).map((branch) => (
        <Badge key={branch} variant="subtle" colorScheme="blue" fontSize="2xs" borderRadius="full" px={1.5} maxW="120px" isTruncated title={branch}>
          {branch}
        </Badge>
      ))}
      {parsed.tags.slice(0, 2).map((tag) => (
        <Badge key={tag} variant="subtle" colorScheme="purple" fontSize="2xs" borderRadius="full" px={1.5} maxW="120px" isTruncated title={tag}>
          {tag}
        </Badge>
      ))}
      {parsed.remotes.length > 0 && (
        <Badge variant="subtle" colorScheme="gray" fontSize="2xs" borderRadius="full" px={1.5}>
          +{parsed.remotes.length}
        </Badge>
      )}
    </HStack>
  )
}

function CommitDetailsPanel({ details, onClose }: { details: ImpactCommitDetails; onClose: () => void }) {
  const copySha = async () => {
    try {
      await navigator.clipboard.writeText(details.commit.sha)
      toast({ title: 'Commit SHA copied', status: 'success', duration: 1500 })
    } catch {
      toast({ title: 'Copy failed', status: 'warning', duration: 1500 })
    }
  }

  return (
    <Box borderTop="1px solid" borderColor="whiteAlpha.100">
      <Flex px={4} h="40px" align="center" gap={2} borderBottom="1px solid" borderColor="whiteAlpha.100" flexShrink={0}>
        <Text fontSize="10px" color="gray.500" fontWeight="bold" textTransform="uppercase" flex={1} isTruncated>
          Commit
        </Text>
        <Code fontSize="xs" color="whiteAlpha.600" title={details.commit.sha}>
          {details.commit.short_sha}
        </Code>
        <Tooltip label="Copy full SHA" placement="top">
          <IconButton aria-label="Copy full SHA" icon={<CopyIcon />} size="xs" variant="ghost" color="gray.500" _hover={{ color: 'gray.200' }} onClick={copySha} />
        </Tooltip>
        <Tooltip label="Close details" placement="top">
          <IconButton aria-label="Close details" icon={<SmallCloseIcon />} size="xs" variant="ghost" color="gray.500" _hover={{ color: 'gray.200' }} onClick={onClose} />
        </Tooltip>
      </Flex>
      <Box px={4} py={3}>
        <Text fontSize="sm" fontWeight="semibold" color="gray.100">
          {details.commit.subject || '(no commit message)'}
        </Text>
        {details.commit.body && (
          <Text fontSize="sm" color="gray.400" whiteSpace="pre-wrap" mt={1}>
            {details.commit.body}
          </Text>
        )}
        <HStack mt={2} spacing={2} wrap="wrap" fontSize="xs" color="gray.500">
          <Text>{details.commit.author}{details.commit.author_email ? ` <${details.commit.author_email}>` : ''}</Text>
          <Text>{details.commit.date}</Text>
          <Text color="green.400">+{details.added}</Text>
          <Text color="red.400">−{details.removed}</Text>
        </HStack>
        {details.commit.parents.length > 0 && (
          <HStack mt={2} spacing={1.5} wrap="wrap">
            <Text fontSize="10px" color="gray.500" fontWeight="bold" textTransform="uppercase">
              Parents
            </Text>
            {details.commit.parents.map((parent) => (
              <Code key={parent} fontSize="xs" color="gray.400" title={parent}>
                {parent.slice(0, 7)}
              </Code>
            ))}
          </HStack>
        )}
        {details.files.length > 0 && (
          <VStack align="stretch" spacing={0} mt={3}>
            {details.files.map((file) => (
              <HStack key={file.path} py={0.5} spacing={2} minW={0}>
                <Box
                  w={2}
                  h={2}
                  borderRadius="sm"
                  flexShrink={0}
                  bg={file.change === 'added' ? 'green.400' : file.change === 'deleted' ? 'red.400' : file.change === 'modified' ? 'yellow.400' : 'gray.400'}
                />
                <Code fontSize="xs" color="gray.300" isTruncated flex="1" title={file.path}>
                  {file.path}
                </Code>
                <HStack spacing={1.5} flexShrink={0} style={{ fontVariantNumeric: 'tabular-nums' }}>
                  <Text fontSize="xs" color="green.400" visibility={file.added > 0 ? 'visible' : 'hidden'}>
                    +{file.added}
                  </Text>
                  <Text fontSize="xs" color="red.400" visibility={file.removed > 0 ? 'visible' : 'hidden'}>
                    −{file.removed}
                  </Text>
                </HStack>
              </HStack>
            ))}
          </VStack>
        )}
      </Box>
    </Box>
  )
}

export default function CommitHistoryPanel({
  path,
  base,
  head,
  onSelectBase,
  onSelectHead,
  onRun,
  running,
}: {
  path: string
  base: string
  head: string
  onSelectBase: (sha: string) => void
  onSelectHead: (sha: string) => void
  onRun: () => void
  running: boolean
}) {
  const [history, setHistory] = useState<ImpactCommit[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)
  const [historyError, setHistoryError] = useState<string | null>(null)
  const [limit, setLimit] = useState(HISTORY_LIMIT)
  const [reloadNonce, setReloadNonce] = useState(0)
  const [query, setQuery] = useState('')
  const [inspectedSha, setInspectedSha] = useState<string | null>(null)
  const [details, setDetails] = useState<ImpactCommitDetails | null>(null)
  const [detailsLoading, setDetailsLoading] = useState(false)
  const [rangeStats, setRangeStats] = useState<ImpactRangeStats | null>(null)
  const [rangeStatsLoading, setRangeStatsLoading] = useState(false)
  const statsTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (!path) return
    setHistoryLoading(true)
    setHistoryError(null)
    api.impact
      .listCommitGraph(path, limit)
      .then(setHistory)
      .catch((err: unknown) => setHistoryError(err instanceof Error ? err.message : 'Could not load commit history'))
      .finally(() => setHistoryLoading(false))
  }, [path, limit, reloadNonce])

  useEffect(() => {
    setInspectedSha(null)
    setDetails(null)
    setLimit(HISTORY_LIMIT)
  }, [path])

  useEffect(() => {
    if (!path || !inspectedSha) {
      setDetails(null)
      return
    }
    setDetailsLoading(true)
    api.impact
      .getCommitDetails(path, inspectedSha)
      .then(setDetails)
      .catch((err: unknown) =>
        toast({ title: 'Could not load commit details', description: err instanceof Error ? err.message : 'Unknown error', status: 'error' }),
      )
      .finally(() => setDetailsLoading(false))
  }, [path, inspectedSha])

  useEffect(() => {
    if (statsTimer.current !== null) clearTimeout(statsTimer.current)
    if (!path || !base || !head) {
      setRangeStats(null)
      return
    }
    if (base === head) {
      setRangeStats({ commits: 0, files_changed: 0, added: 0, removed: 0 })
      return
    }
    setRangeStatsLoading(true)
    statsTimer.current = setTimeout(() => {
      api.impact
        .getRangeStats(path, base, head)
        .then(setRangeStats)
        .catch(() => setRangeStats(null))
        .finally(() => setRangeStatsLoading(false))
    }, 350)
    return () => {
      if (statsTimer.current !== null) clearTimeout(statsTimer.current)
    }
  }, [path, base, head])

  const layout = useMemo(() => layoutCommitGraph(history), [history])
  const railWidth = Math.max(1, layout.laneCount) * LANE_W + RAIL_PAD * 2
  const indexBySha = useMemo(() => {
    const map = new Map<string, number>()
    layout.rows.forEach((row, index) => {
      if (!map.has(row.commit.sha)) map.set(row.commit.sha, index)
    })
    return map
  }, [layout])
  const baseIndex = base ? (indexBySha.get(base) ?? -1) : -1
  const headIndex = head ? (indexBySha.get(head) ?? -1) : -1
  const rangeFrom = baseIndex >= 0 && headIndex >= 0 ? Math.min(baseIndex, headIndex) : -1
  const rangeTo = baseIndex >= 0 && headIndex >= 0 ? Math.max(baseIndex, headIndex) : -1

  const nodeY = (row: number) => row * ROW_H + ROW_H / 2
  const nodeX = (lane: number) => RAIL_PAD + lane * LANE_W + LANE_W / 2
  const edgePath = (x1: number, y1: number, x2: number, y2: number, railX: number) => {
    const distance = Math.abs(y2 - y1)
    const bend = Math.min(ROW_H, distance / 2)
    const direction = Math.sign(y2 - y1) || 1
    const startY = y1 + direction * bend
    const endY = y2 - direction * bend
    return `M ${x1} ${y1} C ${x1} ${y1 + direction * bend / 2}, ${railX} ${y1 + direction * bend / 2}, ${railX} ${startY} L ${railX} ${endY} C ${railX} ${y2 - direction * bend / 2}, ${x2} ${y2 - direction * bend / 2}, ${x2} ${y2}`
  }

  return (
    <Box borderBottom="1px solid" borderColor="whiteAlpha.100">
      <Flex px={4} h="40px" align="center" gap={2} borderBottom="1px solid" borderColor="whiteAlpha.100" flexShrink={0}>
        <Text fontSize="10px" color="gray.500" fontWeight="bold" textTransform="uppercase" flexShrink={0}>
          Commit history
        </Text>
        {!historyLoading && history.length > 0 && (
          <Box fontSize="xs" color="gray.500" whiteSpace="nowrap">
            <Box as="span" color="gray.300" fontWeight="medium">
              {history.length}
            </Box>
            {' '}commits
          </Box>
        )}
        <Box flex={1} />
        <InputGroup size="xs" maxW="200px">
          <InputLeftElement pointerEvents="none" color="gray.600">
            <SearchIcon boxSize={3} />
          </InputLeftElement>
          <Input
            placeholder="Filter…"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            variant="filled"
            bg="whiteAlpha.50"
            _focus={{ bg: 'whiteAlpha.100' }}
            borderRadius="md"
            _placeholder={{ color: 'gray.600' }}
          />
          {query && (
            <InputRightElement>
              <IconButton aria-label="Clear commit filter" icon={<SmallCloseIcon />} size="xs" variant="ghost" color="gray.600" onClick={() => setQuery('')} />
            </InputRightElement>
          )}
        </InputGroup>
      </Flex>

      {historyLoading && history.length === 0 ? (
        <Flex py={8} align="center" justify="center" direction="column" gap={3} color="gray.600">
          <Spinner size="lg" color="var(--accent)" />
          <Text fontSize="sm">Loading commit history…</Text>
        </Flex>
      ) : historyError && history.length === 0 ? (
        <Box px={4} py={6} textAlign="center">
          <Text fontSize="sm" color="gray.400">
            {historyError}
          </Text>
          <Button size="xs" variant="outline" mt={3} onClick={() => setReloadNonce((current) => current + 1)}>
            Retry
          </Button>
        </Box>
      ) : history.length === 0 ? (
        <Box px={4} py={6} textAlign="center">
          <Text fontSize="sm" color="gray.500">
            No commits found for this checkout.
          </Text>
        </Box>
      ) : (
        <Box maxH="380px" overflowY="auto">
          <Flex align="stretch">
            <Box w={`${railWidth}px`} flexShrink={0} position="relative">
              <svg width={railWidth} height={layout.rows.length * ROW_H} style={{ display: 'block' }}>
                {layout.edges.map((edge, index) => {
                  const x1 = nodeX(edge.fromLane)
                  const y1 = nodeY(edge.fromRow)
                  const x2 = nodeX(edge.toLane)
                  const y2 = edge.toRow !== null ? nodeY(edge.toRow) : layout.rows.length * ROW_H
                  const color = laneColor(edge.railLane)
                  return x1 === x2 && x1 === nodeX(edge.railLane) ? (
                    <line key={index} x1={x1} y1={y1} x2={x2} y2={y2} stroke={color} strokeWidth={2.5} strokeLinecap="round" />
                  ) : (
                    <path
                      key={index}
                      d={edgePath(x1, y1, x2, y2, nodeX(edge.railLane))}
                      stroke={color}
                      strokeWidth={2.5}
                      strokeLinecap="round"
                      fill="none"
                    />
                  )
                })}
                {layout.rows.map((row, index) => (
                  <g key={row.commit.sha}>
                    <circle
                      cx={nodeX(row.lane)}
                      cy={nodeY(index)}
                      r={row.isMerge ? 6 : 4.5}
                      fill={laneColor(row.lane)}
                      stroke="var(--bg-canvas)"
                      strokeWidth={2}
                    >
                      <title>{`${row.commit.short_sha} · ${row.commit.subject}${row.isMerge ? ' (merge)' : ''}`}</title>
                    </circle>
                    {(row.commit.sha === base || row.commit.sha === head) && (
                      <circle
                        cx={nodeX(row.lane)}
                        cy={nodeY(index)}
                        r={9}
                        fill="none"
                        stroke={row.commit.sha === base ? '#A0AEC0' : '#68D391'}
                        strokeWidth={1.5}
                      />
                    )}
                  </g>
                ))}
              </svg>
            </Box>
            <VStack align="stretch" spacing={0} flex={1} minW={0}>
              {layout.rows.map((row, index) => {
                const commit = row.commit
                const isBase = commit.sha === base
                const isHead = commit.sha === head
                const inRange = rangeFrom >= 0 && index >= rangeFrom && index <= rangeTo
                const dimmed = !rowMatches(commit, query)
                return (
                  <Flex
                    key={commit.sha}
                    h={`${ROW_H}px`}
                    px={2}
                    align="center"
                    gap={2}
                    minW={0}
                    borderBottom="1px solid"
                    borderColor="whiteAlpha.50"
                    bg={isBase || isHead ? 'rgba(var(--accent-rgb), 0.12)' : inRange ? 'rgba(var(--accent-rgb), 0.05)' : 'transparent'}
                    opacity={dimmed ? 0.35 : 1}
                    cursor="pointer"
                    role="group"
                    transition="background 0.1s"
                    _hover={{ bg: isBase || isHead ? 'rgba(var(--accent-rgb), 0.16)' : 'whiteAlpha.50' }}
                    onClick={() => setInspectedSha((current) => (current === commit.sha ? null : commit.sha))}
                  >
                    <Box flex={1} minW={0}>
                      <HStack spacing={1.5} minW={0}>
                        {isBase && (
                          <Badge variant="subtle" colorScheme="gray" fontSize="2xs" borderRadius="full" px={1.5} flexShrink={0}>
                            BASE
                          </Badge>
                        )}
                        {isHead && (
                          <Badge variant="subtle" colorScheme="green" fontSize="2xs" borderRadius="full" px={1.5} flexShrink={0}>
                            HEAD
                          </Badge>
                        )}
                        <Code fontSize="xs" color="gray.300" flexShrink={0}>
                          {commit.short_sha}
                        </Code>
                        <Text fontSize="sm" color="gray.100" isTruncated flex={1} title={`${commit.subject} — ${[commit.author, commit.date].filter(Boolean).join(' · ')}`}>
                          {commit.subject || '(no commit message)'}
                        </Text>
                        <RefBadges commit={commit} />
                        <Text fontSize="xs" color="gray.500" flexShrink={0} whiteSpace="nowrap">
                          {[commit.author, commit.date].filter(Boolean).join(' · ')}
                          {row.isMerge ? ' · merge' : ''}
                        </Text>
                      </HStack>
                    </Box>
                    <HStack spacing={1} flexShrink={0} display={{ base: 'flex', lg: 'none' }} _groupHover={{ display: 'flex' }}>
                      <Tooltip label="Set as base" placement="top">
                        <Button
                          size="xs"
                          variant="ghost"
                          color="gray.400"
                          _hover={{ color: 'white', bg: 'whiteAlpha.100' }}
                          onClick={(event) => {
                            event.stopPropagation()
                            onSelectBase(commit.sha)
                          }}
                        >
                          Base
                        </Button>
                      </Tooltip>
                      <Tooltip label="Set as head" placement="top">
                        <Button
                          size="xs"
                          variant="ghost"
                          color="gray.400"
                          _hover={{ color: 'white', bg: 'whiteAlpha.100' }}
                          onClick={(event) => {
                            event.stopPropagation()
                            onSelectHead(commit.sha)
                          }}
                        >
                          Head
                        </Button>
                      </Tooltip>
                    </HStack>
                  </Flex>
                )
              })}
            </VStack>
          </Flex>
          {history.length >= limit && limit < HISTORY_MAX && (
            <Button size="xs" variant="ghost" w="full" borderRadius={0} color="gray.400" _hover={{ color: 'white' }} onClick={() => setLimit(HISTORY_MAX)} py={4}>
              Show older commits
            </Button>
          )}
        </Box>
      )}

      <Flex px={4} py={2} gap={2} align="center" borderTop="1px solid" borderColor="whiteAlpha.100" wrap="wrap">
        {base || head ? (
          <HStack spacing={1.5} minW={0} wrap="wrap">
            {base && (
              <Badge variant="subtle" colorScheme="gray" borderRadius="full" px={2} py={0.5} fontSize="2xs">
                BASE {base.slice(0, 7)}
              </Badge>
            )}
            {head && (
              <Badge variant="subtle" colorScheme="green" borderRadius="full" px={2} py={0.5} fontSize="2xs">
                HEAD {head.slice(0, 7)}
              </Badge>
            )}
            <Button
                size="xs"
                variant="ghost"
                color="gray.500"
                _hover={{ color: 'gray.200' }}
                onClick={() => {
                  onSelectBase('')
                  onSelectHead('')
                }}
              >
                Clear
              </Button>
          </HStack>
        ) : (
          <Text fontSize="xs" color="gray.600">
            Pick base and head from the graph, or use the selects above.
          </Text>
        )}
        <Box flex={1} />
        {rangeStatsLoading ? (
          <Spinner size="xs" color="var(--accent)" />
        ) : (
          rangeStats &&
          base &&
          head && (
            <Text fontSize="xs" color="gray.500" whiteSpace="nowrap">
              <Box as="span" color="gray.300" fontWeight="medium">
                {rangeStats.commits}
              </Box>{' '}
              commits ·{' '}
              <Box as="span" color="gray.300" fontWeight="medium">
                {rangeStats.files_changed}
              </Box>{' '}
              files · <Box as="span" color="green.400">+{rangeStats.added}</Box> <Box as="span" color="red.400">−{rangeStats.removed}</Box>
            </Text>
          )
        )}
        <Button size="xs" colorScheme="blue" isLoading={running} isDisabled={!base} onClick={onRun}>
          Run impact
        </Button>
      </Flex>

      {detailsLoading && !details && (
        <Flex py={4} align="center" justify="center" gap={2} color="gray.600" borderTop="1px solid" borderColor="whiteAlpha.100">
          <Spinner size="xs" color="var(--accent)" />
          <Text fontSize="xs">Loading commit details…</Text>
        </Flex>
      )}
      {details && <CommitDetailsPanel details={details} onClose={() => { setInspectedSha(null); setDetails(null) }} />}
    </Box>
  )
}
