import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Alert,
  AlertIcon,
  Badge,
  Box,
  Button,
  Center,
  Collapse,
  Code,
  Divider,
  Flex,
  FormControl,
  FormLabel,
  Grid,
  HStack,
  IconButton,
  Input,
  Select,
  Spinner,
  Text,
  VStack,
  Tooltip,
  useToast,
} from '@chakra-ui/react'
import { DeleteIcon } from '@chakra-ui/icons'
import { useNavigate } from 'react-router-dom'
import {
  api,
  type ImpactChangeType,
  type ImpactCommit,
  type ImpactCoverage,
  type ImpactFile,
  type ImpactReport,
  type ImpactRepository,
} from '../api/client'
import ImpactCanvas from '../components/ImpactCanvas'
import { buildImpactFileTree, flattenImpactFileTree } from '../utils/impactFileTree'

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
        Impact analysis needs an architecture representation at the base commit. Ask your agent to scan this repository with the{' '}
        <Code fontSize="xs">create-diagram-impact</Code> skill, then reload.
      </Text>
      <VStack align="stretch" spacing={2} mt={4}>
        <Text fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.500">
          Install the skill
        </Text>
        <Text fontSize="sm" color="gray.300">
          Place <Code fontSize="xs">skills/create-diagram-impact/SKILL.md</Code> from the tld repository into your agent&apos;s skills directory, e.g.{' '}
          <Code fontSize="xs">{SKILL_INSTALL_PATH}</Code>.
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
  const [showGaps, setShowGaps] = useState(false)

  return (
    <Box {...panelStyle} p={4}>
      <HStack justify="space-between" align="center">
        <HStack spacing={3}>
          <Badge colorScheme={coverageColor(coverage)} fontSize="sm" px={2} py={1} borderRadius="md">
            {coverageLabel(coverage)}
          </Badge>
          <Text fontSize="sm" color="gray.400">
            {coverage.applicable ? `${coverage.bound_source_files}/${coverage.source_files} changed source files owned` : 'Nothing to reconcile'}
          </Text>
        </HStack>
        <Text fontSize="xs" color="gray.500">
          {coverage.anchored_elements}/{coverage.total_elements} elements bound
        </Text>
      </HStack>
      {coverage.gaps.length > 0 && (
        <>
          <Divider my={3} borderColor="var(--border-main)" />
          <Button
            variant="ghost"
            size="sm"
            w="full"
            px={0}
            justifyContent="space-between"
            fontWeight="normal"
            onClick={() => setShowGaps((current) => !current)}
            aria-expanded={showGaps}
          >
            <Text fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.500">
              Binding gaps
            </Text>
            <Badge colorScheme="orange" fontSize="xs" borderRadius="full" px={2}>
              {coverage.gaps.length}
            </Badge>
          </Button>
          <Collapse in={showGaps} animateOpacity>
            <VStack align="stretch" spacing={2} mt={3}>
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
          </Collapse>
        </>
      )}
    </Box>
  )
}

type ChangeView = 'files' | 'elements'

function changeDotColor(change: ImpactChangeType | undefined): string {
  switch (change) {
    case 'added':
      return 'green.400'
    case 'deleted':
      return 'red.400'
    case 'modified':
      return 'yellow.400'
    default:
      return 'gray.400'
  }
}

function ChangeViewToggle({ view, onChange }: { view: ChangeView; onChange: (view: ChangeView) => void }) {
  const options: { value: ChangeView; label: string }[] = [
    { value: 'files', label: 'Files' },
    { value: 'elements', label: 'Elements' },
  ]

  return (
    <HStack spacing={0} border="1px solid" borderColor="var(--border-main)" borderRadius="md" overflow="hidden">
      {options.map((option) => (
        <Box
          key={option.value}
          as="button"
          type="button"
          aria-pressed={view === option.value}
          px={2}
          py={0.5}
          fontSize="2xs"
          textTransform="uppercase"
          letterSpacing="0.08em"
          color={view === option.value ? 'gray.100' : 'gray.500'}
          bg={view === option.value ? 'var(--bg-hover)' : 'transparent'}
          _hover={{ color: 'gray.200' }}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </Box>
      ))}
    </HStack>
  )
}

function ChangedFilesTree({ files }: { files: ImpactFile[] }) {
  const nodes = useMemo(() => flattenImpactFileTree(buildImpactFileTree(files)), [files])

  return (
    <>
      {nodes.map((node) => (
        <HStack
          key={`${node.isDir ? 'dir' : 'file'}:${node.path}`}
          pl={2 + node.depth * 3}
          pr={2}
          py={0.5}
          spacing={2}
          minW={0}
          borderRadius="sm"
          _hover={{ bg: 'var(--bg-hover)' }}
        >
          <Box w={2} h={2} borderRadius="sm" bg={node.isDir ? 'gray.500' : changeDotColor(node.change)} flexShrink={0} />
          <Text fontSize="sm" color={node.isDir ? 'gray.300' : 'gray.200'} isTruncated flex="1">
            {node.name}
          </Text>
          <HStack spacing={1.5} flexShrink={0} minW="56px" justify="flex-end" style={{ fontVariantNumeric: 'tabular-nums' }}>
            <Text fontSize="xs" color={node.isDir ? 'gray.500' : 'green.400'} visibility={node.added > 0 ? 'visible' : 'hidden'}>
              +{node.added}
            </Text>
            <Text fontSize="xs" color={node.isDir ? 'gray.500' : 'red.400'} visibility={node.removed > 0 ? 'visible' : 'hidden'}>
              -{node.removed}
            </Text>
          </HStack>
        </HStack>
      ))}
    </>
  )
}

function ChangeNavigator({ report, onOpenElement }: { report: ImpactReport; onOpenElement: (elementId: number) => void }) {
  const [view, setView] = useState<ChangeView>('files')
  const changedCount = report.changed.length + report.unmapped.length
  const fileCount = report.changed_files.length
  const renderElement = (element: ImpactReport['changed'][number], tone: 'green' | 'blue') => (
    <Button
      key={element.ref}
      variant="ghost"
      size="sm"
      justifyContent="flex-start"
      px={2}
      w="full"
      fontWeight="normal"
      onClick={() => element.element_id && onOpenElement(element.element_id)}
      isDisabled={!element.element_id}
    >
      <Box w={2} h={2} borderRadius="sm" bg={`${tone}.400`} mr={2} flexShrink={0} />
      <Text fontSize="sm" isTruncated>
        {element.name}
      </Text>
    </Button>
  )

  return (
    <Box {...panelStyle} p={2} minW={0} maxH={{ xl: '460px' }} overflowY="auto">
      <HStack justify="space-between" px={2} pt={1} pb={2}>
        <Text fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.500">
          Changes
        </Text>
        <HStack spacing={2}>
          <ChangeViewToggle view={view} onChange={setView} />
          <Badge fontSize="xs" borderRadius="full" px={2}>
            {view === 'files' ? fileCount : changedCount}
          </Badge>
        </HStack>
      </HStack>

      {view === 'files' ? (
        fileCount > 0 ? (
          <ChangedFilesTree files={report.changed_files} />
        ) : (
          <Text fontSize="sm" color="gray.500" px={2} py={3}>
            No changed files.
          </Text>
        )
      ) : (
        <>
          {report.changed.length > 0 && (
            <Box mb={2}>
              <Text fontSize="xs" color="gray.500" px={2} mb={1}>
                Changed architecture
              </Text>
              {report.changed.map((element) => renderElement(element, 'green'))}
            </Box>
          )}

          {report.unmapped.length > 0 && (
            <Box mb={2}>
              <Text fontSize="xs" color="gray.500" px={2} mb={1}>
                Needs binding
              </Text>
              {report.unmapped.map((file) => (
                <HStack key={file} px={2} py={1} spacing={2} minW={0}>
                  <Box w={2} h={2} borderRadius="sm" bg="orange.400" flexShrink={0} />
                  <Code fontSize="xs" color="gray.300" isTruncated>
                    {file}
                  </Code>
                </HStack>
              ))}
            </Box>
          )}

          {report.related.length > 0 && (
            <Box>
              <Text fontSize="xs" color="gray.500" px={2} mb={1}>
                Related context
              </Text>
              {report.related.map((element) => renderElement(element, 'blue'))}
            </Box>
          )}

          {changedCount === 0 && report.related.length === 0 && (
            <Text fontSize="sm" color="gray.500" px={2} py={3}>
              No changed files or architecture elements.
            </Text>
          )}
        </>
      )}
    </Box>
  )
}

function ImpactResults({ report, onOpenElement }: { report: ImpactReport; onOpenElement: (elementId: number) => void }) {
  return (
    <Grid templateColumns={{ base: '1fr', xl: '280px minmax(0, 1fr)' }} gap={4} alignItems="start">
      <ChangeNavigator report={report} onOpenElement={onOpenElement} />
      <VStack align="stretch" spacing={4} minW={0}>
        <ImpactCanvas report={report} onOpenElement={onOpenElement} />
        <CoveragePanel coverage={report.coverage} />
      </VStack>
    </Grid>
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
  const [status, setStatus] = useState<{
    repository: ImpactRepository
    commits: ImpactCommit[]
  } | null>(null)
  const [statusLoading, setStatusLoading] = useState(false)
  const [base, setBase] = useState('')
  const [head, setHead] = useState('')
  const [running, setRunning] = useState(false)
  const [report, setReport] = useState<ImpactReport | null>(null)
  const [linkPath, setLinkPath] = useState('')
  const [linking, setLinking] = useState(false)
  const [showAddForm, setShowAddForm] = useState(false)

  const selected = useMemo(() => repositories.find((repository) => repository.ref === selectedRef) ?? null, [repositories, selectedRef])

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
      .catch((statusError: unknown) => setError(statusError instanceof Error ? statusError.message : 'Could not load repository status'))
      .finally(() => setStatusLoading(false))
  }, [selectedRef])

  const addRepository = async () => {
    if (!addPath.trim()) return
    setAdding(true)
    try {
      const added = await api.impact.addRepository({
        path: addPath.trim(),
        name: addName.trim() || undefined,
      })
      setAddPath('')
      setAddName('')
      setShowAddForm(false)
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
      const updated = await api.impact.updateRepository({
        ref: repository.ref,
        path: linkPath.trim(),
      })
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
      const result = await api.impact.analyze({
        path,
        base,
        head: head || 'HEAD',
      })
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
      <VStack align="stretch" spacing={5} maxW="1600px" mx="auto" px={{ base: 4, md: 6, xl: 8 }} py={{ base: 5, md: 6 }}>
        <Flex justify="space-between" align={{ base: 'flex-start', md: 'center' }} gap={3} wrap="wrap">
          <Box>
            <Text fontSize="xl" fontWeight="semibold" color="gray.100">
              Repository impact
            </Text>
            <Text fontSize="sm" color="gray.400" mt={1}>
              Compare commits against the architecture diagram.
            </Text>
          </Box>
          {repositories.length > 0 && (
            <Button size="sm" variant="outline" onClick={() => setShowAddForm((current) => !current)}>
              {showAddForm ? 'Close' : 'Add repository'}
            </Button>
          )}
        </Flex>

        {error && (
          <Alert status="error" borderRadius="md">
            <AlertIcon />
            {error}
          </Alert>
        )}

        <Collapse in={repositories.length === 0 || showAddForm} animateOpacity>
          <Box {...panelStyle} p={4}>
            <FormLabel fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
              Add repository
            </FormLabel>
            <Flex gap={3} direction={{ base: 'column', md: 'row' }}>
              <Input flex="2" size="sm" placeholder="/path/to/local/git/repo" value={addPath} onChange={(event) => setAddPath(event.target.value)} />
              <Input flex="1" size="sm" placeholder="Name (optional)" value={addName} onChange={(event) => setAddName(event.target.value)} />
              <Button size="sm" colorScheme="blue" isLoading={adding} onClick={addRepository}>
                Add
              </Button>
            </Flex>
          </Box>
        </Collapse>

        {repositories.length === 0 ? (
          <Box {...panelStyle} p={8} textAlign="center">
            <Text fontSize="sm" color="gray.400">
              No repositories linked yet. Add a local git checkout above.
            </Text>
          </Box>
        ) : (
          <Grid templateColumns={{ base: '1fr', lg: '280px minmax(0, 1fr)' }} gap={5} alignItems="start">
            <VStack align="stretch" spacing={2} position={{ lg: 'sticky' }} top={{ lg: 4 }}>
              <Text fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.500" px={1}>
                Repositories
              </Text>
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
                    <Tooltip label="Remove repository" placement="top">
                      <IconButton
                        aria-label="Remove repository"
                        icon={<DeleteIcon />}
                        size="sm"
                        variant="ghost"
                        color="red.300"
                        flexShrink={0}
                        _hover={{ bg: 'red.900', color: 'red.200' }}
                        onClick={(event) => {
                          event.stopPropagation()
                          removeRepository(repository)
                        }}
                      />
                    </Tooltip>
                  </Flex>
                )
              })}
            </VStack>
            <Box minW={0}>
              {selected && (
                <VStack align="stretch" spacing={4}>
                  {statusLoading ? (
                    <Center py={8}>
                      <Spinner color="var(--accent)" />
                    </Center>
                  ) : !selected.has_architecture ? (
                    <RepositoryReadiness
                      repository={selected}
                      onCopied={() =>
                        toast({
                          title: 'Prompt copied',
                          status: 'success',
                          duration: 1500,
                        })
                      }
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
                        <Input size="sm" placeholder="/path/to/local/checkout" value={linkPath} onChange={(event) => setLinkPath(event.target.value)} />
                        <Button size="sm" colorScheme="blue" isLoading={linking} onClick={() => linkCheckout(selected)}>
                          Link checkout
                        </Button>
                      </Flex>
                    </Box>
                  ) : (
                    <>
                      <Box key={selected.ref} {...panelStyle} p={3}>
                        <Flex align={{ base: 'stretch', lg: 'center' }} gap={3} wrap="wrap">
                          <Box flex="1 1 180px" minW={0}>
                            <Text fontWeight="semibold" color="gray.100" fontSize="sm" isTruncated>
                              {selected.name}
                            </Text>
                            <Text fontSize="xs" color="gray.500" isTruncated>
                              {localPath}
                            </Text>
                          </Box>
                          <FormControl w={{ base: 'full', sm: '180px', lg: '160px' }}>
                            <FormLabel fontSize="2xs" color="gray.500" mb={0.5}>
                              Branch
                            </FormLabel>
                            <Input
                              size="sm"
                              defaultValue={selected.branch}
                              onBlur={(event) => {
                                const value = event.target.value.trim()
                                if (value && value !== selected.branch) updateBranch(selected, value)
                              }}
                            />
                          </FormControl>
                          <FormControl flex={{ base: '1 1 100%', lg: '1 1 220px' }} minW={0}>
                            <FormLabel fontSize="2xs" color="gray.500" mb={0.5}>
                              From
                            </FormLabel>
                            <Select size="sm" value={base} onChange={(event) => setBase(event.target.value)}>
                              {commits.map((commit) => (
                                <option key={commit.sha} value={commit.sha}>
                                  {commit.short_sha} · {commit.subject}
                                </option>
                              ))}
                            </Select>
                          </FormControl>
                          <Text color="gray.500" display={{ base: 'none', lg: 'block' }}>
                            →
                          </Text>
                          <FormControl flex={{ base: '1 1 100%', lg: '1 1 220px' }} minW={0}>
                            <FormLabel fontSize="2xs" color="gray.500" mb={0.5}>
                              To
                            </FormLabel>
                            <Select size="sm" value={head} onChange={(event) => setHead(event.target.value)}>
                              {commits.map((commit) => (
                                <option key={commit.sha} value={commit.sha}>
                                  {commit.short_sha} · {commit.subject}
                                </option>
                              ))}
                            </Select>
                          </FormControl>
                          <Button size="sm" colorScheme="blue" isLoading={running} onClick={runImpact} minW="110px" w={{ base: 'full', lg: 'auto' }}>
                            Run impact
                          </Button>
                        </Flex>
                      </Box>

                      {report && <ImpactResults report={report} onOpenElement={openElement} />}
                    </>
                  )}
                </VStack>
              )}
            </Box>
          </Grid>
        )}
      </VStack>
    </Box>
  )
}
