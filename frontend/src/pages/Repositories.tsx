import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import {
  Alert,
  AlertIcon,
  Badge,
  Box,
  Button,
  Center,
  Code,
  Divider,
  Flex,
  Grid,
  HStack,
  IconButton,
  Spinner,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import type { ButtonProps } from '@chakra-ui/react'
import { CopyIcon, RepeatIcon } from '@chakra-ui/icons'
import type { IndexedRepository } from '../api/client'
import { api } from '../api/client'
import { toast } from '../utils/toast'

type RepoFilter = 'all' | 'indexed' | 'empty'

const FILTER_OPTIONS: { value: RepoFilter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'indexed', label: 'Indexed' },
  { value: 'empty', label: 'Not indexed' },
]

const accentCtaStyle: ButtonProps = {
  bg: 'var(--accent)',
  color: 'white',
  border: '1px solid',
  borderColor: 'rgba(var(--accent-rgb), 0.55)',
  boxShadow: '0 0 22px rgba(var(--accent-rgb), 0.28)',
  transition: 'transform 0.18s ease, filter 0.18s ease',
  _hover: { bg: 'var(--accent)', filter: 'brightness(1.08)', transform: 'translateY(-1px)' },
  _active: { transform: 'translateY(0)', filter: 'brightness(0.92)' },
}

const accentOutlineStyle: ButtonProps = {
  bg: 'rgba(var(--accent-rgb), 0.1)',
  color: 'var(--accent)',
  border: '1px solid',
  borderColor: 'rgba(var(--accent-rgb), 0.4)',
  _hover: { bg: 'rgba(var(--accent-rgb), 0.18)', borderColor: 'rgba(var(--accent-rgb), 0.6)' },
}

function repoName(root: string): string {
  const parts = root.split(/[/\\]/).filter(Boolean)
  return parts[parts.length - 1] || root || 'repository'
}

function shortId(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) return '—'
  return trimmed.length > 12 ? trimmed.slice(0, 12) : trimmed
}

function snapshotAge(unixSeconds: number): string {
  if (!unixSeconds) return 'never'
  const seconds = Math.max(0, Math.floor(Date.now() / 1000 - unixSeconds))
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  return new Date(unixSeconds * 1000).toLocaleDateString()
}

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

function SegmentedControl<T extends string>({
  options,
  value,
  onChange,
}: {
  options: { value: T; label: string; count?: number }[]
  value: T
  onChange: (value: T) => void
}) {
  return (
    <Box p={1} bg="whiteAlpha.50" borderRadius="xl">
      <HStack spacing={1}>
        {options.map((option) => {
          const active = option.value === value
          return (
            <Box
              key={option.value}
              flex={1}
              as="button"
              type="button"
              aria-pressed={active}
              py={1.5}
              px={2}
              fontSize="10px"
              fontWeight="800"
              letterSpacing="0.08em"
              textTransform="uppercase"
              cursor="pointer"
              bg={active ? 'whiteAlpha.200' : 'transparent'}
              color={active ? 'white' : 'whiteAlpha.500'}
              borderRadius="lg"
              _hover={{ bg: active ? 'whiteAlpha.200' : 'whiteAlpha.100', color: 'white' }}
              transition="all 0.2s"
              onClick={() => onChange(option.value)}
              whiteSpace="nowrap"
            >
              {option.label}
              {typeof option.count === 'number' ? ` · ${option.count}` : ''}
            </Box>
          )
        })}
      </HStack>
    </Box>
  )
}

function MicroLabel({ children }: { children: ReactNode }) {
  return (
    <Text fontSize="10px" fontWeight="700" color="gray.500" textTransform="uppercase" letterSpacing="0.08em">
      {children}
    </Text>
  )
}

function RepoGlyph({ name, size = 'sm' }: { name: string; size?: 'sm' | 'lg' }) {
  const initial = name.trim() ? name.trim()[0].toUpperCase() : '?'
  return (
    <Flex
      w={size === 'sm' ? '28px' : '48px'}
      h={size === 'sm' ? '28px' : '48px'}
      align="center"
      justify="center"
      flexShrink={0}
      bg="whiteAlpha.100"
      rounded="md"
    >
      <Text fontSize={size === 'sm' ? 'sm' : 'xl'} fontWeight="semibold" color="whiteAlpha.900">
        {initial}
      </Text>
    </Flex>
  )
}

function StatCard({ label, value }: { label: string; value: number }) {
  return (
    <Box px={4} py={3} border="1px solid" borderColor="whiteAlpha.100" borderRadius="md" bg="whiteAlpha.50">
      <Text fontSize="2xl" fontWeight="700" color="gray.100" fontFamily="mono" lineHeight="1.1">
        {value.toLocaleString()}
      </Text>
      <MicroLabel>{label}</MicroLabel>
    </Box>
  )
}

export default function Repositories() {
  const [repositories, setRepositories] = useState<IndexedRepository[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = useState<RepoFilter>('all')
  const [selectedId, setSelectedId] = useState<string>('')

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const next = await api.repositories.list()
      setRepositories(next)
      setSelectedId((current) => current || next[0]?.id || '')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load repositories')
      setRepositories([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const isIndexed = (repo: IndexedRepository) => repo.latestSnapshotId !== ''
  const filterCounts = useMemo(() => ({
    all: repositories.length,
    indexed: repositories.filter(isIndexed).length,
    empty: repositories.filter((repo) => !isIndexed(repo)).length,
  }), [repositories])

  const filtered = useMemo(() => repositories.filter((repo) => {
    if (filter === 'indexed') return isIndexed(repo)
    if (filter === 'empty') return !isIndexed(repo)
    return true
  }), [filter, repositories])

  const selected = repositories.find((repo) => repo.id === selectedId) ?? null

  const copy = useCallback(async (text: string, label: string) => {
    const ok = await copyText(text)
    toast({ title: ok ? `${label} copied` : 'Copy failed — select the text manually', status: ok ? 'success' : 'warning', duration: 1800 })
  }, [])

  const indexCommand = selected ? `tld index "${selected.root}" --watch` : ''

  return (
    <Box h="full" bg="var(--bg-canvas)" display="flex" flexDir="column" overflow="hidden">
      {error && (
        <Alert status="error" borderRadius={0} flexShrink={0}>
          <AlertIcon />
          <Text flex="1" fontSize="sm">{error}</Text>
          <Button size="xs" variant="outline" ml={2} onClick={() => { void load() }}>Retry</Button>
        </Alert>
      )}

      <Flex px={4} py={3} align="center" gap={3} borderBottom="1px solid" borderColor="whiteAlpha.100" flexShrink={0}>
        <Text fontSize="sm" fontWeight="700" color="gray.100" textTransform="uppercase" letterSpacing="0.06em">
          Repositories
        </Text>
        <Badge variant="subtle" colorScheme="purple" fontSize="2xs" borderRadius="full" px={2}>
          {repositories.length}
        </Badge>
        <Box flex={1} />
        <Tooltip label="Reload repositories" placement="top">
          <IconButton aria-label="Reload repositories" icon={<RepeatIcon />} size="sm" variant="ghost" color="gray.400" onClick={() => { void load() }} />
        </Tooltip>
      </Flex>

      {loading && repositories.length === 0 ? (
        <Center flex={1}>
          <VStack spacing={3} color="gray.600">
            <Spinner size="lg" color="var(--accent)" />
            <Text fontSize="sm">Loading repositories…</Text>
          </VStack>
        </Center>
      ) : repositories.length === 0 ? (
        <Flex flex={1} align="center" justify="center" direction="column" gap={3} px={4} textAlign="center">
          <RepoGlyph name="R" size="lg" />
          <Text fontSize="md" fontWeight="semibold" color="gray.100">No repositories indexed yet</Text>
          <Text fontSize="sm" color="gray.400" maxW="460px">
            Run <Code fontSize="xs">tld index &lt;path&gt;</Code> to extract a code graph and materialize an indexed diagram.
          </Text>
        </Flex>
      ) : (
        <Flex flex={1} minH={0} overflow="hidden" direction={{ base: 'column', lg: 'row' }}>
          <Box
            w={{ base: 'full', lg: '320px' }}
            display="flex"
            flexDir="column"
            borderRight="1px solid"
            borderBottom={{ base: '1px solid', lg: 'none' }}
            borderColor="whiteAlpha.100"
            flexShrink={0}
            minH={0}
            maxH={{ base: '40vh', lg: 'none' }}
            overflow="hidden"
          >
            <Box px={3} py={3} flexShrink={0}>
              <SegmentedControl
                options={FILTER_OPTIONS.map((option) => ({ ...option, count: filterCounts[option.value] }))}
                value={filter}
                onChange={setFilter}
              />
            </Box>
            <Box flex={1} minH={0} overflowY="auto">
              {filtered.map((repo) => {
                const active = repo.id === selectedId
                const indexed = isIndexed(repo)
                return (
                  <Box
                    key={repo.id}
                    borderBottom="1px solid"
                    borderColor="whiteAlpha.50"
                    bg={active ? 'rgba(var(--accent-rgb), 0.08)' : 'transparent'}
                    cursor="pointer"
                    transition="background 0.1s"
                    _hover={{ bg: active ? 'rgba(var(--accent-rgb), 0.12)' : 'whiteAlpha.50' }}
                    onClick={() => setSelectedId(repo.id)}
                  >
                    <Flex px={4} py={2.5} align="center" gap={3}>
                      <Box position="relative" flexShrink={0}>
                        <RepoGlyph name={repoName(repo.root)} />
                        <Box position="absolute" bottom={-1} right={-1} w={2.5} h={2.5} borderRadius="full" bg={indexed ? 'green.400' : 'gray.400'} border="2px solid" borderColor="var(--bg-canvas)" />
                      </Box>
                      <Box flex="1" minW={0}>
                        <HStack spacing={2} minW={0}>
                          <Text fontWeight="semibold" color="gray.100" fontSize="sm" isTruncated>
                            {repoName(repo.root)}
                          </Text>
                          <Badge variant="subtle" colorScheme={indexed ? 'green' : 'gray'} fontSize="2xs" borderRadius="full" px={2} flexShrink={0}>
                            {indexed ? 'Indexed' : 'Not indexed'}
                          </Badge>
                        </HStack>
                        <Text fontSize="xs" color="gray.500" isTruncated title={repo.root}>
                          {repo.root}
                        </Text>
                      </Box>
                    </Flex>
                  </Box>
                )
              })}
              {filtered.length === 0 && (
                <Box p={4}>
                  <Text fontSize="sm" color="gray.500">No matching repositories</Text>
                  <Button size="xs" variant="ghost" mt={2} color="gray.500" onClick={() => setFilter('all')}>Clear filters</Button>
                </Box>
              )}
            </Box>
          </Box>

          <Box flex={1} minW={0} minH={0} overflowY="auto" px={4} py={4}>
            {!selected ? (
              <Center h="100%" color="gray.600">
                <Text fontSize="sm">Select a repository to inspect its index.</Text>
              </Center>
            ) : (
              <VStack align="stretch" spacing={5} maxW="760px">
                <HStack spacing={3} align="center">
                  <RepoGlyph name={repoName(selected.root)} size="lg" />
                  <Box minW={0} flex={1}>
                    <HStack spacing={2}>
                      <Text fontSize="lg" fontWeight="700" color="gray.50" isTruncated>{repoName(selected.root)}</Text>
                      <Badge variant="subtle" colorScheme={isIndexed(selected) ? 'green' : 'gray'} fontSize="2xs" borderRadius="full" px={2}>
                        {isIndexed(selected) ? 'Indexed' : 'Not indexed'}
                      </Badge>
                    </HStack>
                    <HStack spacing={1} mt={1} minW={0}>
                      <Code fontSize="2xs" color="gray.400" isTruncated flex={1} title={selected.root}>{selected.root}</Code>
                      <IconButton aria-label="Copy root path" icon={<CopyIcon />} size="xs" variant="ghost" onClick={() => { void copy(selected.root, 'Path') }} />
                    </HStack>
                  </Box>
                </HStack>

                {isIndexed(selected) ? (
                  <>
                    <Grid templateColumns={{ base: 'repeat(2, 1fr)', md: 'repeat(4, 1fr)' }} gap={3}>
                      <StatCard label="Facts" value={selected.facts} />
                      <StatCard label="Chunks" value={selected.chunks} />
                      <StatCard label="Edges" value={selected.edges} />
                      <StatCard label="Sources" value={selected.sources} />
                    </Grid>

                    <Box border="1px solid" borderColor="whiteAlpha.100" borderRadius="md" overflow="hidden">
                      <Flex px={4} py={2.5} borderBottom="1px solid" borderColor="whiteAlpha.100" align="center">
                        <MicroLabel>Latest snapshot</MicroLabel>
                        <Box flex={1} />
                        <Text fontSize="xs" color="gray.500">{snapshotAge(selected.latestCreatedUnix)}</Text>
                      </Flex>
                      <VStack align="stretch" spacing={0}>
                        <Flex px={4} py={2.5} align="center" gap={3} borderBottom="1px solid" borderColor="whiteAlpha.50">
                          <Text fontSize="xs" color="gray.500" w="88px" flexShrink={0}>Snapshot</Text>
                          <Code fontSize="xs" color="gray.300" flex={1} isTruncated>{shortId(selected.latestSnapshotId)}</Code>
                          <IconButton aria-label="Copy snapshot id" icon={<CopyIcon />} size="xs" variant="ghost" onClick={() => { void copy(selected.latestSnapshotId, 'Snapshot id') }} />
                        </Flex>
                        <Flex px={4} py={2.5} align="center" gap={3}>
                          <Text fontSize="xs" color="gray.500" w="88px" flexShrink={0}>Revision</Text>
                          <Code fontSize="xs" color="gray.300" flex={1} isTruncated>{selected.gitRevision ? shortId(selected.gitRevision) : '—'}</Code>
                          {selected.gitBranch && (
                            <Badge variant="subtle" colorScheme="blue" fontSize="2xs" borderRadius="full" px={2}>{selected.gitBranch}</Badge>
                          )}
                        </Flex>
                      </VStack>
                    </Box>
                  </>
                ) : (
                  <Alert status="info" borderRadius="md" alignItems="flex-start">
                    <AlertIcon />
                    <Box>
                      <Text fontSize="sm" fontWeight="semibold">This repository has no snapshot yet.</Text>
                      <Text fontSize="sm" color="gray.400" mt={1}>
                        Run the indexer to extract its code graph and materialize a diagram.
                      </Text>
                    </Box>
                  </Alert>
                )}

                <Divider borderColor="whiteAlpha.100" />

                <Box>
                  <MicroLabel>Index command</MicroLabel>
                  <HStack mt={2} spacing={2}>
                    <Code
                      display="block"
                      whiteSpace="pre-wrap"
                      flex={1}
                      p={3}
                      borderRadius="md"
                      fontSize="xs"
                      color="gray.200"
                      bg="whiteAlpha.50"
                      border="1px solid"
                      borderColor="whiteAlpha.100"
                    >
                      {indexCommand}
                    </Code>
                    <IconButton aria-label="Copy index command" icon={<CopyIcon />} size="sm" variant="ghost" onClick={() => { void copy(indexCommand, 'Command') }} />
                    <Button {...accentCtaStyle} size="sm">Re-index</Button>
                  </HStack>
                  <Text mt={2} fontSize="xs" color="gray.500">
                    Indexing runs locally. Use the CLI or the <Code fontSize="2xs">--watch</Code> flag to keep the graph current.
                  </Text>
                </Box>

                <Button {...accentOutlineStyle} size="sm" alignSelf="flex-start" onClick={() => { void load() }}>
                  Reload
                </Button>
              </VStack>
            )}
          </Box>
        </Flex>
      )}
    </Box>
  )
}
