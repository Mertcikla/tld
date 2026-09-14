import { useCallback, useEffect, useMemo, useState } from 'react'
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
  FormControl,
  FormLabel,
  HStack,
  IconButton,
  Input,
  Select,
  Spinner,
  Text,
  VStack,
  useToast,
} from '@chakra-ui/react'
import { useNavigate } from 'react-router-dom'
import {
  api,
  type ImpactCommit,
  type ImpactCoverage,
  type ImpactReport,
  type ImpactRepository,
} from '../api/client'
import ImpactCanvas from '../components/ImpactCanvas'

const panelStyle = {
  bg: 'var(--bg-panel)',
  border: '1px solid',
  borderColor: 'var(--border-main)',
  borderRadius: 'xl',
  boxShadow: 'panel',
} as const

const SKILL_INSTALL_PATH = '~/.agents/skills/create-diagram-impact/SKILL.md'

function coverageColor(coverage: ImpactCoverage): string {
  if (!coverage.applicable) return 'gray'
  if (coverage.confidence === 'high') return 'green'
  if (coverage.confidence === 'medium') return 'yellow'
  return 'red'
}

function coverageLabel(coverage: ImpactCoverage): string {
  if (!coverage.applicable) return 'No source code changed'
  return `${coverage.percent}% ${coverage.confidence}`
}

function RepositoryReadiness({ repository, onCopied }: { repository: ImpactRepository; onCopied: () => void }) {
  const prompt = `Use the create-diagram-impact skill to diagram ${repository.local_path || repository.name} at commit ${
    repository.head_commit || repository.branch || 'HEAD'
  }, binding elements to code so tld impact works.`

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(prompt)
      onCopied()
    } catch {
      onCopied()
    }
  }

  return (
    <Box {...panelStyle} p={5}>
      <Text fontSize="sm" fontWeight="semibold" color="gray.100">
        No architecture for this repository yet
      </Text>
      <Text mt={1} fontSize="sm" color="gray.400">
        Impact analysis needs an architecture representation at the base commit. Ask your agent to scan this
        repository with the <Code fontSize="xs">create-diagram-impact</Code> skill, then reload.
      </Text>
      <VStack align="stretch" spacing={2} mt={4}>
        <Text fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.500">
          Install the skill
        </Text>
        <Text fontSize="sm" color="gray.300">
          Place <Code fontSize="xs">skills/create-diagram-impact/SKILL.md</Code> from the tld repository into your
          agent&apos;s skills directory, e.g. <Code fontSize="xs">{SKILL_INSTALL_PATH}</Code>.
        </Text>
        <Text fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.500" mt={2}>
          Then run this prompt
        </Text>
        <Code display="block" whiteSpace="pre-wrap" p={3} borderRadius="md" fontSize="xs" color="gray.200">
          {prompt}
        </Code>
        <HStack>
          <Button size="sm" colorScheme="blue" onClick={copy}>
            Copy prompt
          </Button>
        </HStack>
      </VStack>
    </Box>
  )
}

function CoveragePanel({ coverage }: { coverage: ImpactCoverage }) {
  return (
    <Box {...panelStyle} p={4}>
      <HStack justify="space-between" align="center">
        <HStack spacing={3}>
          <Badge colorScheme={coverageColor(coverage)} fontSize="sm" px={2} py={1} borderRadius="md">
            {coverageLabel(coverage)}
          </Badge>
          <Text fontSize="sm" color="gray.400">
            {coverage.applicable
              ? `${coverage.bound_source_files}/${coverage.source_files} changed source files owned`
              : 'Nothing to reconcile'}
          </Text>
        </HStack>
        <Text fontSize="xs" color="gray.500">
          {coverage.anchored_elements}/{coverage.total_elements} elements bound
        </Text>
      </HStack>
      {coverage.gaps.length > 0 && (
        <>
          <Divider my={3} borderColor="var(--border-main)" />
          <Text fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.500" mb={2}>
            Binding gaps
          </Text>
          <VStack align="stretch" spacing={2}>
            {coverage.gaps.map((gap) => (
              <Box key={gap.file} fontSize="sm">
                <Text color="gray.200">
                  <Code fontSize="xs">{gap.file}</Code>
                  {gap.change ? ` (${gap.change})` : ''} — {gap.reason}
                </Text>
                <Text fontSize="xs" color="gray.500" mt={0.5}>
                  {gap.suggested_element_name
                    ? gap.suggested_element_pattern
                      ? `suggested owner ${gap.suggested_element_name} already owns ${gap.suggested_element_pattern}; extend that binding`
                      : `bind: tld bind ${gap.suggested_element_ref} --file "${gap.file}"`
                    : ''}
                  {gap.suggested_new_element ? `  •  or add: tld add "${gap.suggested_new_element}" --file "${gap.file}"` : ''}
                </Text>
              </Box>
            ))}
          </VStack>
        </>
      )}
    </Box>
  )
}

function ImpactResults({ report, onOpenElement }: { report: ImpactReport; onOpenElement: (elementId: number) => void }) {
  return (
    <VStack align="stretch" spacing={4}>
      <CoveragePanel coverage={report.coverage} />
      <ImpactCanvas report={report} onOpenElement={onOpenElement} />
      <Box {...panelStyle} p={4}>
        <HStack spacing={6} align="flex-start" flexWrap="wrap">
          <Box flex="1" minW="220px">
            <Text fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.500" mb={2}>
              Changed
            </Text>
            {report.changed.length === 0 ? (
              <Text fontSize="sm" color="gray.500">
                none
              </Text>
            ) : (
              report.changed.map((element) => (
                <Text key={element.ref} fontSize="sm" color="gray.200">
                  {element.name}
                </Text>
              ))
            )}
          </Box>
          <Box flex="1" minW="220px">
            <Text fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.500" mb={2}>
              Related
            </Text>
            {report.related.length === 0 ? (
              <Text fontSize="sm" color="gray.500">
                none
              </Text>
            ) : (
              report.related.map((element) => (
                <Text key={element.ref} fontSize="sm" color="gray.400">
                  {element.name}
                </Text>
              ))
            )}
          </Box>
          <Box flex="1" minW="220px">
            <Text fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.500" mb={2}>
              Unmapped
            </Text>
            {report.unmapped.length === 0 ? (
              <Text fontSize="sm" color="gray.500">
                none
              </Text>
            ) : (
              report.unmapped.map((file) => (
                <Text key={file} fontSize="sm" color="gray.400">
                  <Code fontSize="xs">{file}</Code>
                </Text>
              ))
            )}
          </Box>
        </HStack>
        {report.findings.length > 0 && (
          <>
            <Divider my={3} borderColor="var(--border-main)" />
            <Text fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.500" mb={2}>
              Findings
            </Text>
            <VStack align="stretch" spacing={1}>
              {report.findings.map((finding, index) => (
                <Text key={`${finding.type}-${index}`} fontSize="sm" color="gray.300">
                  <Badge mr={2} colorScheme={finding.severity === 'warning' ? 'orange' : 'gray'} fontSize="2xs">
                    {finding.observed ? 'observed' : 'inferred'}
                  </Badge>
                  {finding.message}
                </Text>
              ))}
            </VStack>
          </>
        )}
        {report.summary && (
          <>
            <Divider my={3} borderColor="var(--border-main)" />
            <Text fontSize="sm" color="gray.300">
              {report.summary}
            </Text>
          </>
        )}
      </Box>
    </VStack>
  )
}

export default function Repositories() {
  const navigate = useNavigate()
  const toast = useToast()
  const [repositories, setRepositories] = useState<ImpactRepository[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedRef, setSelectedRef] = useState<string | null>(null)
  const [addPath, setAddPath] = useState('')
  const [addName, setAddName] = useState('')
  const [adding, setAdding] = useState(false)
  const [status, setStatus] = useState<{ repository: ImpactRepository; commits: ImpactCommit[] } | null>(null)
  const [statusLoading, setStatusLoading] = useState(false)
  const [base, setBase] = useState('')
  const [head, setHead] = useState('')
  const [running, setRunning] = useState(false)
  const [report, setReport] = useState<ImpactReport | null>(null)
  const [linkPath, setLinkPath] = useState('')
  const [linking, setLinking] = useState(false)

  const selected = useMemo(
    () => repositories.find((repository) => repository.ref === selectedRef) ?? null,
    [repositories, selectedRef],
  )

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    api.impact
      .listRepositories()
      .then((items) => {
        setRepositories(items)
        setSelectedRef((current) => current ?? items[0]?.ref ?? null)
      })
      .catch((loadError: unknown) => setError(loadError instanceof Error ? loadError.message : 'Could not load repositories'))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    if (!selectedRef) {
      setStatus(null)
      return
    }
    setStatusLoading(true)
    setReport(null)
    api.impact
      .getRepositoryStatus(selectedRef)
      .then((result) => {
        setStatus(result)
        const commits = result.commits
        setHead(commits[0]?.sha ?? 'HEAD')
        setBase(commits[1]?.sha ?? commits[0]?.sha ?? 'HEAD~1')
      })
      .catch((statusError: unknown) =>
        setError(statusError instanceof Error ? statusError.message : 'Could not load repository status'),
      )
      .finally(() => setStatusLoading(false))
  }, [selectedRef])

  const addRepository = async () => {
    if (!addPath.trim()) return
    setAdding(true)
    try {
      const added = await api.impact.addRepository({ path: addPath.trim(), name: addName.trim() || undefined })
      setAddPath('')
      setAddName('')
      await load()
      setSelectedRef(added.ref)
    } catch (addError) {
      toast({
        title: 'Could not add repository',
        description: addError instanceof Error ? addError.message : 'Unknown error',
        status: 'error',
      })
    } finally {
      setAdding(false)
    }
  }

  const removeRepository = async (repository: ImpactRepository) => {
    try {
      await api.impact.removeRepository(repository.ref)
      if (selectedRef === repository.ref) setSelectedRef(null)
      await load()
    } catch (removeError) {
      toast({
        title: 'Could not remove repository',
        description: removeError instanceof Error ? removeError.message : 'Unknown error',
        status: 'error',
      })
    }
  }

  const updateBranch = async (repository: ImpactRepository, branch: string) => {
    try {
      await api.impact.updateRepository({ ref: repository.ref, branch })
      await load()
    } catch (updateError) {
      toast({
        title: 'Could not update branch',
        description: updateError instanceof Error ? updateError.message : 'Unknown error',
        status: 'error',
      })
    }
  }

  const linkCheckout = async (repository: ImpactRepository) => {
    if (!linkPath.trim()) return
    setLinking(true)
    try {
      const updated = await api.impact.updateRepository({ ref: repository.ref, path: linkPath.trim() })
      setLinkPath('')
      await load()
      setSelectedRef(updated.ref)
      const result = await api.impact.getRepositoryStatus(updated.ref)
      setStatus(result)
      setHead(result.commits[0]?.sha ?? 'HEAD')
      setBase(result.commits[1]?.sha ?? result.commits[0]?.sha ?? 'HEAD~1')
    } catch (linkError) {
      toast({
        title: 'Could not link checkout',
        description: linkError instanceof Error ? linkError.message : 'Unknown error',
        status: 'error',
      })
    } finally {
      setLinking(false)
    }
  }

  const runImpact = async () => {
    const path = status?.repository.local_path || selected?.local_path
    if (!path || !base) return
    setRunning(true)
    setError(null)
    try {
      const result = await api.impact.analyze({ path, base, head: head || 'HEAD' })
      setReport(result)
    } catch (runError) {
      setError(runError instanceof Error ? runError.message : 'Impact analysis failed')
    } finally {
      setRunning(false)
    }
  }

  const openElement = useCallback(
    async (elementId: number) => {
      if (elementId <= 0) return
      try {
        const placements = await api.elements.placements(elementId)
        const viewId = placements[0]?.view_id
        if (viewId) {
          navigate(`/views/${viewId}?element=${elementId}`)
          return
        }
      } catch {
        // fall through to the inventory view
      }
      navigate(`/inventory?object=element:${elementId}`)
    },
    [navigate],
  )

  if (loading) {
    return (
      <Center h="full" bg="var(--bg-canvas)">
        <Spinner size="xl" color="var(--accent)" />
      </Center>
    )
  }

  const commits = status?.commits ?? []
  const localPath = status?.repository.local_path || selected?.local_path || ''

  return (
    <Box h="full" overflowY="auto" bg="var(--bg-canvas)">
      <VStack align="stretch" spacing={6} maxW="1100px" mx="auto" px={{ base: 4, md: 8 }} py={8}>
        <Box>
          <Text fontSize="xl" fontWeight="semibold" color="gray.100">
            Repositories
          </Text>
          <Text fontSize="sm" color="gray.400" mt={1}>
            Link a local repository, then compare two commits to see how a change touches your architecture.
          </Text>
        </Box>

        {error && (
          <Alert status="error" borderRadius="md">
            <AlertIcon />
            {error}
          </Alert>
        )}

        <Box {...panelStyle} p={4}>
          <FormLabel fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
            Add repository
          </FormLabel>
          <Flex gap={3} direction={{ base: 'column', md: 'row' }}>
            <Input
              flex="2"
              size="sm"
              placeholder="/path/to/local/git/repo"
              value={addPath}
              onChange={(event) => setAddPath(event.target.value)}
            />
            <Input
              flex="1"
              size="sm"
              placeholder="Name (optional)"
              value={addName}
              onChange={(event) => setAddName(event.target.value)}
            />
            <Button size="sm" colorScheme="blue" isLoading={adding} onClick={addRepository}>
              Add
            </Button>
          </Flex>
        </Box>

        {repositories.length === 0 ? (
          <Box {...panelStyle} p={8} textAlign="center">
            <Text fontSize="sm" color="gray.400">
              No repositories linked yet. Add a local git checkout above.
            </Text>
          </Box>
        ) : (
          <VStack align="stretch" spacing={2}>
            {repositories.map((repository) => {
              const active = repository.ref === selectedRef
              return (
                <Flex
                  key={repository.ref}
                  {...panelStyle}
                  p={3}
                  align="center"
                  gap={3}
                  borderColor={active ? 'var(--accent)' : 'var(--border-main)'}
                  cursor="pointer"
                  onClick={() => setSelectedRef(repository.ref)}
                >
                  <Box flex="1" minW={0}>
                    <HStack spacing={2}>
                      <Text fontWeight="semibold" color="gray.100" fontSize="sm">
                        {repository.name}
                      </Text>
                      {!repository.has_architecture && (
                        <Badge colorScheme="orange" fontSize="2xs">
                          no architecture
                        </Badge>
                      )}
                    </HStack>
                    <Text fontSize="xs" color="gray.500" isTruncated>
                      {repository.local_path || repository.remote_url || repository.ref}
                      {repository.branch ? ` · ${repository.branch}` : ''}
                    </Text>
                  </Box>
                  <IconButton
                    aria-label="Remove repository"
                    size="xs"
                    variant="ghost"
                    onClick={(event) => {
                      event.stopPropagation()
                      removeRepository(repository)
                    }}
                  >
                    ×
                  </IconButton>
                </Flex>
              )
            })}
          </VStack>
        )}

        {selected && (
          <VStack align="stretch" spacing={4}>
            {statusLoading ? (
              <Center py={8}>
                <Spinner color="var(--accent)" />
              </Center>
            ) : !selected.has_architecture ? (
              <RepositoryReadiness
                repository={selected}
                onCopied={() => toast({ title: 'Prompt copied', status: 'success', duration: 1500 })}
              />
            ) : !localPath ? (
              <Box {...panelStyle} p={5}>
                <Text fontSize="sm" fontWeight="semibold" color="gray.100">
                  Link a local checkout
                </Text>
                <Text mt={1} fontSize="sm" color="gray.400">
                  Point this repository at its local git checkout to compare commits and run impact analysis.
                </Text>
                <Flex mt={3} gap={3} direction={{ base: 'column', md: 'row' }}>
                  <Input
                    size="sm"
                    placeholder="/path/to/local/checkout"
                    value={linkPath}
                    onChange={(event) => setLinkPath(event.target.value)}
                  />
                  <Button size="sm" colorScheme="blue" isLoading={linking} onClick={() => linkCheckout(selected)}>
                    Link checkout
                  </Button>
                </Flex>
              </Box>
            ) : (
              <>
                <Box {...panelStyle} p={4}>
                  <FormControl>
                    <FormLabel fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
                      Tracked branch
                    </FormLabel>
                    <Input
                      size="sm"
                      maxW="320px"
                      defaultValue={selected.branch}
                      onBlur={(event) => {
                        const value = event.target.value.trim()
                        if (value && value !== selected.branch) updateBranch(selected, value)
                      }}
                    />
                  </FormControl>
                  <Flex mt={4} gap={3} align="flex-end" direction={{ base: 'column', md: 'row' }}>
                    <FormControl>
                      <FormLabel fontSize="xs" color="gray.500">
                        Base commit
                      </FormLabel>
                      <Select size="sm" value={base} onChange={(event) => setBase(event.target.value)}>
                        {commits.map((commit) => (
                          <option key={commit.sha} value={commit.sha}>
                            {commit.short_sha} · {commit.subject}
                          </option>
                        ))}
                      </Select>
                    </FormControl>
                    <FormControl>
                      <FormLabel fontSize="xs" color="gray.500">
                        Head commit
                      </FormLabel>
                      <Select size="sm" value={head} onChange={(event) => setHead(event.target.value)}>
                        {commits.map((commit) => (
                          <option key={commit.sha} value={commit.sha}>
                            {commit.short_sha} · {commit.subject}
                          </option>
                        ))}
                      </Select>
                    </FormControl>
                    <Button size="sm" colorScheme="blue" isLoading={running} onClick={runImpact} minW="120px">
                      Run impact
                    </Button>
                  </Flex>
                </Box>

                {report && <ImpactResults report={report} onOpenElement={openElement} />}
              </>
            )}
          </VStack>
        )}
      </VStack>
    </Box>
  )
}
