import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import {
  Alert,
  AlertIcon,
  Badge,
  Box,
  Button,
  Center,
  Code,
  Collapse,
  Divider,
  Flex,
  Grid,
  HStack,
  IconButton,
  Input,
  InputGroup,
  InputLeftElement,
  InputRightElement,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Progress,
  Select,
  Spinner,
  Tab,
  TabList,
  TabPanel,
  TabPanels,
  Tabs,
  Text,
  VStack,
  Tooltip,
  useDisclosure,
} from '@chakra-ui/react'
import { AddIcon, ArrowUpDownIcon, ChevronDownIcon, ChevronLeftIcon, ChevronRightIcon, CopyIcon, DeleteIcon, ExternalLinkIcon, RepeatIcon, SearchIcon, SmallCloseIcon } from '@chakra-ui/icons'
import { useNavigate, useSearchParams } from 'react-router-dom'
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
import ConfirmDialog from '../components/ConfirmDialog'
import { MarkdownPreview } from '../components/ViewMarkdownPanel/MarkdownPreview'
import { markdownPanelBodySx } from '../components/ViewMarkdownPanel/styles'
import { toast } from '../utils/toast'
import { buildImpactFileTree, flattenImpactFileTree } from '../utils/impactFileTree'
import { impactMarkdown } from '../utils/impactMarkdown'

const SKILL_INSTALL_PATH = '~/.agents/skills/create-diagram-impact/SKILL.md'

type RepoFilter = 'all' | 'ready' | 'setup'
type ResultTab = 'architecture' | 'files' | 'coverage'
type FileChangeFilter = 'all' | ImpactChangeType
type ArchitectureView = 'diagram' | 'markdown'

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

type RepoStatus = 'ready' | 'needs-architecture' | 'needs-checkout'

function repoStatus(repository: ImpactRepository): RepoStatus {
  if (!repository.has_architecture) return 'needs-architecture'
  if (!repository.local_path) return 'needs-checkout'
  return 'ready'
}

function statusMeta(status: RepoStatus): { label: string; scheme: string; dot: string } {
  switch (status) {
    case 'ready':
      return { label: 'Ready', scheme: 'green', dot: 'green.400' }
    case 'needs-architecture':
      return { label: 'No architecture', scheme: 'orange', dot: 'orange.400' }
    default:
      return { label: 'Needs checkout', scheme: 'gray', dot: 'gray.400' }
  }
}

function formatCommitDate(value: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

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

type ChangeView = 'files' | 'elements'

function SegmentedControl<T extends string>({
  options,
  value,
  onChange,
  ariaLabel,
}: {
  options: { value: T; label: string; count?: number }[]
  value: T
  onChange: (value: T) => void
  ariaLabel?: string
}) {
  return (
    <Box p={1} bg="whiteAlpha.50" borderRadius="xl" aria-label={ariaLabel}>
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

function StatCell({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <Box px={4} py={2} minW={0} flex={1}>
      <MicroLabel>{label}</MicroLabel>
      <Text fontSize="md" fontWeight="semibold" color="gray.100" mt={0.5} isTruncated>
        {value}
      </Text>
      {sub && (
        <Text fontSize="xs" color="gray.500" isTruncated>
          {sub}
        </Text>
      )}
    </Box>
  )
}

function SidebarSection({ title, defaultOpen = true, children }: { title: string; defaultOpen?: boolean; children: ReactNode }) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <Box borderBottom="1px solid" borderColor="whiteAlpha.100" flexShrink={0}>
      <Flex
        px={3}
        py={2}
        align="center"
        cursor="pointer"
        onClick={() => setOpen((current) => !current)}
        _hover={{ bg: 'whiteAlpha.50' }}
        userSelect="none"
      >
        <Text fontSize="10px" color="gray.500" fontWeight="bold" textTransform="uppercase" flex={1}>
          {title}
        </Text>
        <Box color="gray.600" transform={open ? 'rotate(90deg)' : 'rotate(0deg)'} transition="transform 0.15s">
          <ChevronRightIcon boxSize={3} />
        </Box>
      </Flex>
      {open && <Box px={3} pb={3}>{children}</Box>}
    </Box>
  )
}

function RepositoryReadiness({
  repository,
  reloading,
  onReload,
}: {
  repository: ImpactRepository
  reloading: boolean
  onReload: () => void
}) {
  const prompt = `Use the create-diagram-impact skill to diagram ${repository.local_path || repository.name} at commit ${
    repository.head_commit || repository.branch || 'HEAD'
  }, binding elements to code so tld impact works.`

  const copy = async () => {
    const ok = await copyText(prompt)
    toast({
      title: ok ? 'Prompt copied' : 'Copy failed — select the text manually',
      status: ok ? 'success' : 'warning',
      duration: 1800,
    })
  }

  const steps = [
    {
      title: 'Install the skill',
      body: (
        <Text fontSize="sm" color="gray.300">
          Place <Code fontSize="xs">skills/create-diagram-impact/SKILL.md</Code> from the tld repository into your agent&apos;s skills directory, e.g.{' '}
          <Code fontSize="xs">{SKILL_INSTALL_PATH}</Code>.
        </Text>
      ),
    },
    {
      title: 'Ask your agent to scan this checkout',
      body: (
        <VStack align="stretch" spacing={2}>
          <Code display="block" whiteSpace="pre-wrap" p={3} borderRadius="md" fontSize="xs" color="gray.200">
            {prompt}
          </Code>
          <HStack>
            <Button size="sm" colorScheme="blue" leftIcon={<CopyIcon />} onClick={copy}>
              Copy prompt
            </Button>
          </HStack>
        </VStack>
      ),
    },
    {
      title: 'Reload and compare',
      body: (
        <VStack align="stretch" spacing={2}>
          <Text fontSize="sm" color="gray.400">
            Once the architecture exists at the base commit, reload status and run impact.
          </Text>
          <HStack>
            <Button size="sm" variant="outline" leftIcon={<RepeatIcon />} isLoading={reloading} onClick={onReload}>
              Reload status
            </Button>
          </HStack>
        </VStack>
      ),
    },
  ]

  return (
    <Box>
      <HStack spacing={2} align="center">
        <Badge variant="subtle" colorScheme="orange">No architecture</Badge>
        <Text fontSize="sm" fontWeight="semibold" color="gray.100">
          Bind this repository before comparing
        </Text>
      </HStack>
      <Text mt={2} fontSize="sm" color="gray.400">
        Impact analysis needs an architecture representation at the base commit. Follow these steps, then reload.
      </Text>
      <VStack align="stretch" spacing={4} mt={5}>
        {steps.map((step, index) => (
          <HStack key={step.title} align="flex-start" spacing={3}>
            <Center w={6} h={6} borderRadius="full" bg="whiteAlpha.100" flexShrink={0}>
              <Text fontSize="xs" fontWeight="bold" color="white">
                {index + 1}
              </Text>
            </Center>
            <Box flex="1" minW={0}>
              <Text fontSize="sm" fontWeight="semibold" color="gray.200">
                {step.title}
              </Text>
              <Box mt={1.5}>{step.body}</Box>
            </Box>
          </HStack>
        ))}
      </VStack>
    </Box>
  )
}

function CoveragePanel({ coverage }: { coverage: ImpactCoverage }) {
  const [showGaps, setShowGaps] = useState(false)

  const copyGapCommand = async (command: string) => {
    const ok = await copyText(command)
    toast({ title: ok ? 'Command copied' : 'Copy failed', status: ok ? 'success' : 'warning', duration: 1500 })
  }

  return (
    <Box>
      <Flex justify="space-between" align={{ base: 'flex-start', md: 'center' }} gap={3} wrap="wrap">
        <HStack spacing={3} wrap="wrap">
          <Badge variant="subtle" colorScheme={coverageColor(coverage)} fontSize="sm" px={2} py={1} borderRadius="md">
            {coverageLabel(coverage)}
          </Badge>
          <Text fontSize="sm" color="gray.400">
            {coverage.applicable ? `${coverage.bound_source_files}/${coverage.source_files} changed source files owned` : 'Nothing to reconcile'}
          </Text>
        </HStack>
        <Text fontSize="xs" color="gray.500">
          {coverage.anchored_elements}/{coverage.total_elements} elements bound
        </Text>
      </Flex>
      {coverage.applicable && (
        <Progress value={coverage.percent} size="sm" mt={3} borderRadius="full" colorScheme={coverageColor(coverage)} bg="whiteAlpha.100" />
      )}
      <HStack mt={2} spacing={4} fontSize="xs" color="gray.500" wrap="wrap">
        <Text>Weak: {coverage.weak_source_files}</Text>
        <Text>Unmapped: {coverage.unmapped_source_files}</Text>
        <Text>Non-source: {coverage.non_source_files}</Text>
      </HStack>
      {coverage.gaps.length > 0 && (
        <>
          <Divider my={3} borderColor="whiteAlpha.100" />
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
            <MicroLabel>Binding gaps</MicroLabel>
            <Badge variant="subtle" colorScheme="orange" fontSize="xs" borderRadius="full" px={2}>
              {coverage.gaps.length}
            </Badge>
          </Button>
          {showGaps && (
            <VStack align="stretch" spacing={2} mt={3}>
              {coverage.gaps.map((gap) => {
                const bindCommand = gap.suggested_element_ref ? `tld bind ${gap.suggested_element_ref} --file "${gap.file}"` : ''
                const addCommand = gap.suggested_new_element ? `tld add "${gap.suggested_new_element}" --file "${gap.file}"` : ''
                return (
                  <Box key={gap.file} fontSize="sm" bg="whiteAlpha.50" border="1px solid" borderColor="whiteAlpha.100" borderRadius="md" p={3}>
                    <Text color="gray.200">
                      <Code fontSize="xs">{gap.file}</Code>
                      {gap.change ? ` (${gap.change})` : ''} — {gap.reason}
                    </Text>
                    {(bindCommand || addCommand) && (
                      <VStack align="stretch" spacing={1.5} mt={2}>
                        {bindCommand && (
                          <HStack justify="space-between" gap={2}>
                            <Code fontSize="xs" color="gray.300" isTruncated flex="1">
                              {bindCommand}
                            </Code>
                            <IconButton aria-label="Copy bind command" icon={<CopyIcon />} size="xs" variant="ghost" onClick={() => copyGapCommand(bindCommand)} />
                          </HStack>
                        )}
                        {addCommand && (
                          <HStack justify="space-between" gap={2}>
                            <Code fontSize="xs" color="gray.300" isTruncated flex="1">
                              {addCommand}
                            </Code>
                            <IconButton aria-label="Copy add command" icon={<CopyIcon />} size="xs" variant="ghost" onClick={() => copyGapCommand(addCommand)} />
                          </HStack>
                        )}
                      </VStack>
                    )}
                  </Box>
                )
              })}
            </VStack>
          )}
        </>
      )}
    </Box>
  )
}

function ChangedFilesTree({ files, query, changeFilter }: { files: ImpactFile[]; query: string; changeFilter: FileChangeFilter }) {
  const nodes = useMemo(() => {
    const q = query.trim().toLowerCase()
    const filtered = files.filter((file) => {
      if (changeFilter !== 'all' && file.change !== changeFilter) return false
      if (q && !file.path.toLowerCase().includes(q)) return false
      return true
    })
    return flattenImpactFileTree(buildImpactFileTree(filtered))
  }, [files, query, changeFilter])

  if (nodes.length === 0) {
    return (
      <Text fontSize="sm" color="gray.500" px={2} py={3}>
        No files match this filter.
      </Text>
    )
  }

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
          _hover={{ bg: 'whiteAlpha.50' }}
        >
          <Box w={2} h={2} borderRadius="sm" bg={node.isDir ? 'gray.500' : changeDotColor(node.change)} flexShrink={0} />
          <Text fontSize="sm" color={node.isDir ? 'gray.300' : 'gray.200'} isTruncated flex="1" title={node.path}>
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

function CompareSide({
  label,
  dot,
  value,
  commits,
  selected,
  onSelect,
}: {
  label: string
  dot: string
  value: string
  commits: ImpactCommit[]
  selected: ImpactCommit | null
  onSelect: (sha: string) => void
}) {
  const copySha = async () => {
    if (!selected) return
    const ok = await copyText(selected.sha)
    toast({ title: ok ? `${label} SHA copied` : 'Copy failed', status: ok ? 'success' : 'warning', duration: 1500 })
  }

  return (
    <Box flex="1" minW={0} w="full" border="1px solid" borderColor="whiteAlpha.100" borderRadius="lg" bg="whiteAlpha.50" p={3}>
      <HStack spacing={2} mb={2}>
        <Box w={2} h={2} borderRadius="full" bg={dot} flexShrink={0} />
        <MicroLabel>{label}</MicroLabel>
      </HStack>
      <Select
        size="sm"
        value={value}
        onChange={(event) => onSelect(event.target.value)}
        placeholder={commits.length ? undefined : 'No commits'}
      >
        {commits.map((commit) => (
          <option key={commit.sha} value={commit.sha}>
            {commit.short_sha} · {commit.subject.slice(0, 60)}{commit.author ? ` — ${commit.author}` : ''}
          </option>
        ))}
      </Select>
      {selected ? (
        <Box mt={2} minW={0}>
          <Text fontSize="sm" color="gray.100" fontWeight="medium" isTruncated title={selected.subject}>
            {selected.subject || '(no commit message)'}
          </Text>
          <HStack spacing={1.5} mt={1} minW={0}>
            <Code fontSize="xs" color="gray.300" flexShrink={0}>
              {selected.short_sha}
            </Code>
            <IconButton aria-label={`Copy ${label} SHA`} icon={<CopyIcon />} size="xs" variant="ghost" onClick={copySha} flexShrink={0} />
            <Text fontSize="xs" color="gray.500" isTruncated flex="1">
              {[selected.author, formatCommitDate(selected.date)].filter(Boolean).join(' · ')}
            </Text>
          </HStack>
        </Box>
      ) : (
        <Text fontSize="xs" color="gray.600" mt={2}>
          No commit selected
        </Text>
      )}
    </Box>
  )
}

function AddRepositoryModal({
  isOpen,
  onClose,
  onAdded,
}: {
  isOpen: boolean
  onClose: () => void
  onAdded: (repository: ImpactRepository) => void
}) {
  const [path, setPath] = useState('')
  const [name, setName] = useState('')
  const [branch, setBranch] = useState('')
  const [adding, setAdding] = useState(false)

  const reset = () => {
    setPath('')
    setName('')
    setBranch('')
  }

  const submit = async () => {
    if (!path.trim()) return
    setAdding(true)
    try {
      const added = await api.impact.addRepository({
        path: path.trim(),
        name: name.trim() || undefined,
        branch: branch.trim() || undefined,
      })
      reset()
      onClose()
      onAdded(added)
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

  return (
    <Modal isOpen={isOpen} onClose={onClose} isCentered size="md">
      <ModalOverlay bg="blackAlpha.700" backdropFilter="blur(4px)" />
      <ModalContent bg="var(--bg-panel)" border="1px solid" borderColor="whiteAlpha.100" borderRadius="xl">
        <ModalHeader color="gray.100" fontSize="md" pb={1}>
          Add repository
        </ModalHeader>
        <ModalBody>
          <VStack align="stretch" spacing={3} pt={2}>
            <Box>
              <Text fontSize="xs" color="gray.500" textTransform="uppercase" letterSpacing="0.05em" mb={1}>
                Local git path
              </Text>
              <Input
                size="sm"
                placeholder="/path/to/local/git/repo"
                value={path}
                onChange={(event) => setPath(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') void submit()
                }}
                autoFocus
              />
            </Box>
            <Grid templateColumns={{ base: '1fr', sm: '1fr 1fr' }} gap={3}>
              <Box>
                <Text fontSize="xs" color="gray.500" textTransform="uppercase" letterSpacing="0.05em" mb={1}>
                  Name (optional)
                </Text>
                <Input size="sm" placeholder="my-service" value={name} onChange={(event) => setName(event.target.value)} />
              </Box>
              <Box>
                <Text fontSize="xs" color="gray.500" textTransform="uppercase" letterSpacing="0.05em" mb={1}>
                  Branch (optional)
                </Text>
                <Input size="sm" placeholder="main" value={branch} onChange={(event) => setBranch(event.target.value)} />
              </Box>
            </Grid>
            <Text fontSize="xs" color="gray.500">
              Point at a local git checkout. The branch tracks which line of history status reads by default.
            </Text>
          </VStack>
        </ModalBody>
        <ModalFooter gap={2}>
          <Button size="sm" variant="ghost" color="gray.500" _hover={{ color: 'white', bg: 'whiteAlpha.100' }} onClick={() => { reset(); onClose() }}>
            Cancel
          </Button>
          <Button size="sm" colorScheme="blue" isLoading={adding} isDisabled={!path.trim()} onClick={submit}>
            Add repository
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  )
}

const FILTER_OPTIONS: { value: RepoFilter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'ready', label: 'Ready' },
  { value: 'setup', label: 'Needs setup' },
]

export default function Repositories() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const [repositories, setRepositories] = useState<ImpactRepository[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedRef, setSelectedRef] = useState<string | null>(() => searchParams.get('repo'))
  const [sidebarQuery, setSidebarQuery] = useState('')
  const [repoFilter, setRepoFilter] = useState<RepoFilter>('all')
  const [status, setStatus] = useState<{ repository: ImpactRepository; commits: ImpactCommit[] } | null>(null)
  const [statusLoading, setStatusLoading] = useState(false)
  const [base, setBase] = useState('')
  const [head, setHead] = useState('')
  const [running, setRunning] = useState(false)
  const [report, setReport] = useState<ImpactReport | null>(null)
  const [linkPath, setLinkPath] = useState('')
  const [linking, setLinking] = useState(false)
  const [branchDraft, setBranchDraft] = useState('')
  const [branchSaving, setBranchSaving] = useState(false)
  const [resultTab, setResultTab] = useState<ResultTab>('architecture')
  const [architectureView, setArchitectureView] = useState<ArchitectureView>('diagram')
  const [showRangeCommits, setShowRangeCommits] = useState(true)
  const [changeView, setChangeView] = useState<ChangeView>('files')
  const [fileQuery, setFileQuery] = useState('')
  const [fileChangeFilter, setFileChangeFilter] = useState<FileChangeFilter>('all')
  const [pendingDelete, setPendingDelete] = useState<ImpactRepository | null>(null)
  const [removing, setRemoving] = useState(false)
  const addDisclosure = useDisclosure()
  const deleteDisclosure = useDisclosure()
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const sidebarInitRef = useRef(false)

  const selectRepo = useCallback(
    (ref: string | null) => {
      setSelectedRef(ref)
      setSearchParams(
        (previous) => {
          const next = new URLSearchParams(previous)
          if (ref) next.set('repo', ref)
          else next.delete('repo')
          return next
        },
        { replace: true },
      )
    },
    [setSearchParams],
  )

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const items = await api.impact.listRepositories()
      setRepositories(items)
      setSelectedRef((current) => {
        if (current && items.some((item) => item.ref === current)) return current
        return items[0]?.ref ?? null
      })
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Could not load repositories')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (!loading && !sidebarInitRef.current) {
      sidebarInitRef.current = true
      if (repositories.length > 0) setSidebarCollapsed(true)
    }
  }, [loading, repositories.length])

  useEffect(() => {
    setSearchParams(
      (previous) => {
        const next = new URLSearchParams(previous)
        if (selectedRef) next.set('repo', selectedRef)
        else next.delete('repo')
        return next
      },
      { replace: true },
    )
  }, [selectedRef, setSearchParams])

  const reloadStatus = useCallback(
    async (ref: string) => {
      setStatusLoading(true)
      try {
        const result = await api.impact.getRepositoryStatus(ref)
        setStatus(result)
        const commits = result.commits
        const path = result.repository?.local_path || ''
        const latest = path ? await api.impact.latest(path) : null
        setReport(latest)
        if (latest) {
          setBase(latest.base)
          setHead(latest.head)
        } else {
          setHead((current) => current || commits[0]?.sha || 'HEAD')
          setBase((current) => current || commits[1]?.sha || commits[0]?.sha || 'HEAD~1')
        }
      } catch (statusError) {
        setError(statusError instanceof Error ? statusError.message : 'Could not load repository status')
      } finally {
        setStatusLoading(false)
      }
    },
    [],
  )

  useEffect(() => {
    if (!selectedRef) {
      setStatus(null)
      return
    }
    setReport(null)
    setFileQuery('')
    void reloadStatus(selectedRef)
  }, [selectedRef, reloadStatus])

  const selected = useMemo(
    () => repositories.find((repository) => repository.ref === selectedRef) ?? null,
    [repositories, selectedRef],
  )
  const effectiveRepo = status?.repository ?? selected
  const commits = useMemo(() => status?.commits ?? [], [status])
  const localPath = effectiveRepo?.local_path || ''

  useEffect(() => {
    setBranchDraft(effectiveRepo?.branch ?? '')
  }, [effectiveRepo?.ref, effectiveRepo?.branch])

  const filteredRepos = useMemo(() => {
    const q = sidebarQuery.trim().toLowerCase()
    return repositories.filter((repository) => {
      if (repoFilter !== 'all') {
        const statusValue = repoStatus(repository)
        if (repoFilter === 'ready' && statusValue !== 'ready') return false
        if (repoFilter === 'setup' && statusValue === 'ready') return false
      }
      if (!q) return true
      return (
        repository.name.toLowerCase().includes(q) ||
        repository.local_path.toLowerCase().includes(q) ||
        repository.branch.toLowerCase().includes(q)
      )
    })
  }, [repositories, sidebarQuery, repoFilter])

  const readyCount = useMemo(() => repositories.filter((repository) => repoStatus(repository) === 'ready').length, [repositories])
  const setupCount = repositories.length - readyCount
  const filterCounts: Record<RepoFilter, number> = useMemo(
    () => ({ all: repositories.length, ready: readyCount, setup: setupCount }),
    [repositories.length, readyCount, setupCount],
  )

  const baseIndex = useMemo(() => commits.findIndex((commit) => commit.sha === base), [commits, base])
  const headIndex = useMemo(() => commits.findIndex((commit) => commit.sha === head), [commits, head])
  const commitsApart = baseIndex >= 0 && headIndex >= 0 ? Math.abs(baseIndex - headIndex) : 0
  const rangeCommits = useMemo(() => {
    if (baseIndex < 0 || headIndex < 0) return []
    const [from, to] = baseIndex < headIndex ? [baseIndex, headIndex] : [headIndex, baseIndex]
    return commits.slice(from, to + 1)
  }, [commits, baseIndex, headIndex])
  const rangeLabel = useMemo(() => {
    if (!rangeCommits.length) return 'Select two commits'
    if (commitsApart === 0) return 'Same commit selected'
    return `${commitsApart} commit${commitsApart === 1 ? '' : 's'} apart`
  }, [rangeCommits, commitsApart])
  const baseCommit = useMemo(() => commits.find((commit) => commit.sha === base) ?? null, [commits, base])
  const headCommit = useMemo(() => commits.find((commit) => commit.sha === head) ?? null, [commits, head])

  const reportStats = useMemo(() => {
    if (!report) return null
    const added = report.changed_files.reduce((sum, file) => sum + file.added, 0)
    const removed = report.changed_files.reduce((sum, file) => sum + file.removed, 0)
    return {
      elements: report.changed.length,
      files: report.changed_files.length,
      added,
      removed,
      gaps: report.coverage.gaps.length,
    }
  }, [report])

  const impactMarkdownText = useMemo(() => (report ? impactMarkdown(report) : ''), [report])

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

  const confirmRemove = async () => {
    if (!pendingDelete) return
    setRemoving(true)
    try {
      await api.impact.removeRepository(pendingDelete.ref)
      if (selectedRef === pendingDelete.ref) selectRepo(null)
      setPendingDelete(null)
      deleteDisclosure.onClose()
      await load()
    } catch (removeError) {
      toast({
        title: 'Could not remove repository',
        description: removeError instanceof Error ? removeError.message : 'Unknown error',
        status: 'error',
      })
    } finally {
      setRemoving(false)
    }
  }

  const saveBranch = async () => {
    if (!effectiveRepo) return
    const value = branchDraft.trim()
    if (!value || value === effectiveRepo.branch) return
    setBranchSaving(true)
    try {
      await api.impact.updateRepository({ ref: effectiveRepo.ref, branch: value })
      await load()
    } catch (updateError) {
      toast({
        title: 'Could not update branch',
        description: updateError instanceof Error ? updateError.message : 'Unknown error',
        status: 'error',
      })
    } finally {
      setBranchSaving(false)
    }
  }

  const linkCheckout = async () => {
    if (!effectiveRepo || !linkPath.trim()) return
    setLinking(true)
    try {
      const updated = await api.impact.updateRepository({ ref: effectiveRepo.ref, path: linkPath.trim() })
      setLinkPath('')
      await load()
      selectRepo(updated.ref)
      await reloadStatus(updated.ref)
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
    const path = localPath || selected?.local_path
    if (!path || !base) return
    setRunning(true)
    setError(null)
    try {
      const result = await api.impact.analyze({ path, base, head: head || 'HEAD' })
      setReport(result)
      setResultTab('architecture')
    } catch (runError) {
      setError(runError instanceof Error ? runError.message : 'Impact analysis failed')
    } finally {
      setRunning(false)
    }
  }

  const reloadAll = useCallback(() => {
    void load()
    if (selectedRef) void reloadStatus(selectedRef)
  }, [load, reloadStatus, selectedRef])

  const swapRange = () => {
    setBase(head)
    setHead(base)
  }

  const applyPreset = (depth: number) => {
    if (commits.length === 0) return
    setHead(commits[0].sha)
    setBase(commits[Math.min(depth, commits.length - 1)].sha)
  }

  const copyPath = async (text: string, label: string) => {
    const ok = await copyText(text)
    toast({ title: ok ? `${label} copied` : 'Copy failed', status: ok ? 'success' : 'warning', duration: 1500 })
  }

  if (loading) {
    return (
      <Flex h="full" bg="var(--bg-canvas)" align="center" justify="center" direction="column" gap={3} color="gray.600">
        <Spinner size="lg" color="var(--accent)" />
        <Text fontSize="sm">Loading repositories…</Text>
      </Flex>
    )
  }

  const selectedStatus = effectiveRepo ? repoStatus(effectiveRepo) : null

  return (
    <Box h="full" bg="var(--bg-canvas)" display="flex" flexDir="column" overflow="hidden">
      <Flex px={4} py={2.5} gap={3} align="center" borderBottom="1px solid" borderColor="whiteAlpha.100" flexShrink={0}>
        <InputGroup size="sm" maxW={{ base: 'none', md: '480px' }} w="full" flex={1} minW={0}>
          <InputLeftElement pointerEvents="none" color="gray.500">
            <SearchIcon boxSize={3.5} />
          </InputLeftElement>
          <Input
            placeholder="Filter repositories…"
            value={sidebarQuery}
            onChange={(event) => setSidebarQuery(event.target.value)}
            variant="elevated"
            _placeholder={{ color: 'gray.600' }}
          />
          {sidebarQuery && (
            <InputRightElement>
              <IconButton
                aria-label="Clear filter"
                icon={<SmallCloseIcon />}
                size="xs"
                variant="ghost"
                color="gray.500"
                _hover={{ color: 'gray.200' }}
                onClick={() => setSidebarQuery('')}
              />
            </InputRightElement>
          )}
        </InputGroup>
        <Flex align="center" gap={2} flexShrink={0} ml={{ md: 'auto' }}>
          <Box fontSize="xs" color="gray.500" whiteSpace="nowrap" display={{ base: 'none', sm: 'block' }}>
            <Box as="span" color="gray.300" fontWeight="medium">
              {repositories.length}
            </Box>
            {' '}linked
          </Box>
          <Button size="sm" colorScheme="blue" leftIcon={<AddIcon />} onClick={addDisclosure.onOpen}>
            Add
          </Button>
        </Flex>
      </Flex>

      {error && (
        <Alert status="error" borderRadius={0} flexShrink={0}>
          <AlertIcon />
          <Text flex="1" fontSize="sm">
            {error}
          </Text>
          <Button size="xs" variant="outline" ml={2} onClick={() => { void load() }}>
            Retry
          </Button>
        </Alert>
      )}

      {repositories.length === 0 ? (
        <Flex flex={1} align="center" justify="center" direction="column" gap={3} px={4} textAlign="center">
          <RepoGlyph name="R" size="lg" />
          <Text fontSize="md" fontWeight="semibold" color="gray.100">
            No repositories linked yet
          </Text>
          <Text fontSize="sm" color="gray.400" maxW="420px">
            Link a local git checkout to compare its history against your architecture and see which elements each change touches.
          </Text>
          <Button size="sm" colorScheme="blue" leftIcon={<AddIcon />} onClick={addDisclosure.onOpen}>
            Add your first repository
          </Button>
        </Flex>
      ) : (
        <Flex flex={1} minH={0} overflow="hidden" direction={{ base: 'column', lg: 'row' }}>
          {sidebarCollapsed ? (
            <Box
              w={{ base: 'full', lg: '60px' }}
              flexShrink={0}
              borderRight={{ lg: '1px solid' }}
              borderBottom={{ base: '1px solid', lg: 'none' }}
              borderColor="whiteAlpha.100"
              p={2}
              overflow="hidden"
            >
              <Flex direction={{ base: 'row', lg: 'column' }} align="center" gap={2}>
                <Tooltip label="Expand repositories" placement="right">
                  <IconButton aria-label="Expand repositories" icon={<ChevronRightIcon />} size="sm" variant="ghost" color="gray.500" _hover={{ color: 'gray.200' }} onClick={() => setSidebarCollapsed(false)} />
                </Tooltip>
                <Box display={{ base: 'none', lg: 'block' }} overflowY="auto" flex="1" minH={0} w="full">
                  <VStack spacing={2}>
                    {repositories.map((repository) => (
                      <Tooltip key={repository.ref} label={repository.name} placement="right">
                        <Box
                          as="button"
                          type="button"
                          onClick={() => selectRepo(repository.ref)}
                          borderRadius="full"
                          border="2px solid"
                          borderColor={repository.ref === selectedRef ? 'var(--accent)' : 'transparent'}
                          _hover={{ borderColor: 'gray.500' }}
                        >
                          <RepoGlyph name={repository.name} />
                        </Box>
                      </Tooltip>
                    ))}
                  </VStack>
                </Box>
                {selected && (
                  <Text display={{ base: 'block', lg: 'none' }} fontSize="sm" color="gray.200" isTruncated flex="1">
                    {selected.name}
                  </Text>
                )}
                <Tooltip label="Reload repositories and status" placement="right">
                  <IconButton aria-label="Reload repositories" icon={<RepeatIcon />} size="sm" variant="ghost" color="gray.500" _hover={{ color: 'gray.200' }} onClick={reloadAll} />
                </Tooltip>
              </Flex>
            </Box>
          ) : (
            <Box
              w={{ base: 'full', lg: '320px' }}
              display="flex"
              flexDir="column"
              borderRight={{ lg: '1px solid' }}
              borderBottom={{ base: '1px solid', lg: 'none' }}
              borderColor="whiteAlpha.100"
              flexShrink={0}
              minH={0}
              maxH={{ base: '38vh', lg: 'none' }}
              overflow="hidden"
            >
              <Flex px={4} py={2} align="center" gap={1} borderBottom="1px solid" borderColor="whiteAlpha.100" flexShrink={0}>
                <Text fontSize="10px" color="gray.500" fontWeight="bold" textTransform="uppercase" flex={1}>
                  Repositories
                </Text>
                <Tooltip label="Collapse sidebar" placement="top">
                  <IconButton aria-label="Collapse sidebar" icon={<ChevronLeftIcon />} size="xs" variant="ghost" color="gray.500" _hover={{ color: 'gray.200' }} onClick={() => setSidebarCollapsed(true)} />
                </Tooltip>
                <Tooltip label="Reload repositories and status" placement="top">
                  <IconButton aria-label="Reload repositories" icon={<RepeatIcon />} size="xs" variant="ghost" color="gray.500" _hover={{ color: 'gray.200' }} onClick={reloadAll} />
                </Tooltip>
              </Flex>
              <SidebarSection title="Status">
                <VStack align="stretch" spacing={0.5}>
                  {FILTER_OPTIONS.map((option) => {
                    const isActive = repoFilter === option.value
                    const count = filterCounts[option.value]
                    return (
                      <Flex
                        key={option.value}
                        align="center"
                        px={2.5}
                        py={1.5}
                        borderRadius="md"
                        cursor="pointer"
                        bg={isActive ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
                        _hover={{ bg: isActive ? 'rgba(var(--accent-rgb), 0.16)' : 'whiteAlpha.50' }}
                        onClick={() => setRepoFilter(option.value)}
                        userSelect="none"
                      >
                        <Text
                          fontSize="sm"
                          fontWeight={isActive ? 'semibold' : 'normal'}
                          color={isActive ? 'var(--accent)' : 'gray.400'}
                          flex={1}
                        >
                          {option.label}
                        </Text>
                        <Text
                          fontSize="10px"
                          fontWeight="bold"
                          color={isActive ? 'var(--accent)' : 'gray.600'}
                          bg={isActive ? 'rgba(var(--accent-rgb), 0.15)' : 'whiteAlpha.100'}
                          px={1.5}
                          py={0.5}
                          borderRadius="full"
                          minW="22px"
                          textAlign="center"
                        >
                          {count}
                        </Text>
                      </Flex>
                    )
                  })}
                </VStack>
              </SidebarSection>
              <Box flex={1} minH={0} overflowY="auto">
                {filteredRepos.map((repository) => {
                  const active = repository.ref === selectedRef
                  const meta = statusMeta(repoStatus(repository))
                  return (
                    <Box
                      key={repository.ref}
                      borderBottom="1px solid"
                      borderColor="whiteAlpha.50"
                      bg={active ? 'rgba(var(--accent-rgb), 0.08)' : 'transparent'}
                      cursor="pointer"
                      role="group"
                      transition="background 0.1s"
                      _hover={{ bg: active ? 'rgba(var(--accent-rgb), 0.12)' : 'whiteAlpha.50' }}
                      onClick={() => selectRepo(repository.ref)}
                    >
                      <Flex px={4} py={2.5} align="center" gap={3}>
                        <Box position="relative" flexShrink={0}>
                          <RepoGlyph name={repository.name} />
                          <Box position="absolute" bottom={-1} right={-1} w={2.5} h={2.5} borderRadius="full" bg={meta.dot} border="2px solid" borderColor="var(--bg-canvas)" />
                        </Box>
                        <Box flex="1" minW={0}>
                          <HStack spacing={2} minW={0}>
                            <Text fontWeight="semibold" color="gray.100" fontSize="sm" isTruncated>
                              {repository.name}
                            </Text>
                            <Badge variant="subtle" colorScheme={meta.scheme} fontSize="2xs" borderRadius="full" px={2} flexShrink={0}>
                              {meta.label}
                            </Badge>
                          </HStack>
                          <Text fontSize="xs" color="gray.500" isTruncated title={repository.local_path || repository.remote_url || repository.ref}>
                            {repository.local_path || repository.remote_url || repository.ref}
                          </Text>
                          {repository.branch && (
                            <Code fontSize="2xs" color="gray.400" maxW="160px" isTruncated>
                              {repository.branch}
                            </Code>
                          )}
                        </Box>
                        <Tooltip label="Remove repository" placement="top">
                          <IconButton
                            aria-label="Remove repository"
                            icon={<DeleteIcon />}
                            size="xs"
                            variant="ghost"
                            color="red.400"
                            display={{ base: 'flex', lg: 'none' }}
                            _groupHover={{ display: 'flex' }}
                            flexShrink={0}
                            onClick={(event) => {
                              event.stopPropagation()
                              setPendingDelete(repository)
                              deleteDisclosure.onOpen()
                            }}
                          />
                        </Tooltip>
                      </Flex>
                      {active && effectiveRepo && effectiveRepo.ref === repository.ref && (
                        <Box px={4} pb={3} onClick={(event) => event.stopPropagation()}>
                          <VStack align="stretch" spacing={2} pl="40px">
                            {effectiveRepo.local_path ? (
                              <HStack spacing={1} minW={0}>
                                <Code fontSize="2xs" color="gray.300" isTruncated flex="1" title={effectiveRepo.local_path}>
                                  {effectiveRepo.local_path}
                                </Code>
                                <IconButton aria-label="Copy local path" icon={<CopyIcon />} size="xs" variant="ghost" onClick={() => copyPath(effectiveRepo.local_path, 'Path')} />
                              </HStack>
                            ) : (
                              <Text fontSize="xs" color="orange.300">
                                No local checkout linked yet.
                              </Text>
                            )}
                            {effectiveRepo.head_commit && (
                              <HStack spacing={1} minW={0}>
                                <Text fontSize="2xs" color="gray.500" flexShrink={0}>
                                  HEAD
                                </Text>
                                <Code fontSize="2xs" color="gray.400" isTruncated flex="1">
                                  {effectiveRepo.head_commit.slice(0, 12)}
                                </Code>
                                <IconButton aria-label="Copy HEAD commit" icon={<CopyIcon />} size="xs" variant="ghost" onClick={() => copyPath(effectiveRepo.head_commit, 'HEAD')} />
                              </HStack>
                            )}
                            <HStack spacing={1}>
                              <Input
                                size="xs"
                                placeholder="Branch"
                                value={branchDraft}
                                onChange={(event) => setBranchDraft(event.target.value)}
                                onKeyDown={(event) => {
                                  if (event.key === 'Enter') void saveBranch()
                                }}
                              />
                              <Button
                                size="xs"
                                variant="outline"
                                flexShrink={0}
                                isLoading={branchSaving}
                                isDisabled={!branchDraft.trim() || branchDraft.trim() === effectiveRepo.branch}
                                onClick={saveBranch}
                              >
                                Save
                              </Button>
                            </HStack>
                            <HStack spacing={1} wrap="wrap">
                              <Button size="xs" variant="outline" leftIcon={<RepeatIcon />} isLoading={statusLoading} onClick={() => void reloadStatus(effectiveRepo.ref)}>
                                Reload status
                              </Button>
                              {effectiveRepo.element_id > 0 && (
                                <Button size="xs" variant="outline" rightIcon={<ExternalLinkIcon />} onClick={() => openElement(effectiveRepo.element_id)}>
                                  Open diagram
                                </Button>
                              )}
                            </HStack>
                          </VStack>
                        </Box>
                      )}
                    </Box>
                  )
                })}
                {filteredRepos.length === 0 && (
                  <Box p={4}>
                    <Text fontSize="sm" color="gray.500">
                      No matching repositories
                    </Text>
                    <Text fontSize="xs" color="gray.700" mt={1}>
                      Try adjusting your filter
                    </Text>
                    <Button size="xs" variant="ghost" mt={2} color="gray.500" onClick={() => { setSidebarQuery(''); setRepoFilter('all') }}>
                      Clear filters
                    </Button>
                  </Box>
                )}
              </Box>
            </Box>
          )}

          <Box flex={1} minW={0} minH={0} overflowY="auto">
            {!effectiveRepo ? (
              <Flex h="100%" align="center" justify="center" direction="column" gap={2} color="gray.600" px={4} textAlign="center">
                <Text fontSize="sm">Select a repository to inspect its status and run impact analysis.</Text>
              </Flex>
            ) : statusLoading ? (
              <Flex h="100%" align="center" justify="center" direction="column" gap={3} color="gray.600">
                <Spinner size="lg" color="var(--accent)" />
                <Text fontSize="sm">Reading git history…</Text>
              </Flex>
            ) : selectedStatus === 'needs-architecture' ? (
              <Box px={4} py={4} maxW="720px">
                <RepositoryReadiness repository={effectiveRepo} reloading={statusLoading} onReload={() => void reloadStatus(effectiveRepo.ref)} />
              </Box>
            ) : !localPath ? (
              <Box px={4} py={4} maxW="720px">
                <Text fontSize="sm" fontWeight="semibold" color="gray.100">
                  Link a local checkout
                </Text>
                <Text mt={1} fontSize="sm" color="gray.400">
                  Point this repository at its local git checkout to compare commits and run impact analysis.
                </Text>
                <Flex mt={3} gap={3} direction={{ base: 'column', md: 'row' }} maxW="560px">
                  <Input size="sm" placeholder="/path/to/local/checkout" value={linkPath} onChange={(event) => setLinkPath(event.target.value)} />
                  <Button size="sm" colorScheme="blue" isLoading={linking} isDisabled={!linkPath.trim()} onClick={linkCheckout} flexShrink={0}>
                    Link checkout
                  </Button>
                </Flex>
              </Box>
            ) : (
              <>
                <Box borderBottom="1px solid" borderColor="whiteAlpha.100">
                  <Flex px={4} h="40px" align="center" gap={2} borderBottom="1px solid" borderColor="whiteAlpha.100" flexShrink={0}>
                    <Text fontSize="10px" color="gray.500" fontWeight="bold" textTransform="uppercase" flex={1} isTruncated>
                      Compare
                    </Text>
                    {baseCommit && headCommit && (
                      <Code fontSize="xs" color="whiteAlpha.600" title={`${baseCommit.sha} compared with ${headCommit.sha}`}>
                        {baseCommit.short_sha} … {headCommit.short_sha}
                      </Code>
                    )}
                    {([['Latest', 1], ['Last 5', 4], ['Last 10', 9]] as const).map(([label, depth]) => (
                      <Button key={label} size="xs" variant="ghost" color="whiteAlpha.600" _hover={{ color: 'white', bg: 'whiteAlpha.100' }} onClick={() => applyPreset(depth)} isDisabled={commits.length === 0}>
                        {label}
                      </Button>
                    ))}
                  </Flex>
                  <Box px={4} py={3}>
                    <Flex gap={3} align={{ base: 'stretch', md: 'center' }} direction={{ base: 'column', md: 'row' }}>
                      <CompareSide label="Base" dot="gray.400" value={base} commits={commits} selected={baseCommit} onSelect={setBase} />
                      <Tooltip label="Swap base and head" placement="top">
                        <IconButton
                          aria-label="Swap base and head"
                          icon={<ArrowUpDownIcon />}
                          size="sm"
                          variant="outline"
                          borderRadius="full"
                          onClick={swapRange}
                          alignSelf="center"
                          flexShrink={0}
                        />
                      </Tooltip>
                      <CompareSide label="Head" dot="green.400" value={head} commits={commits} selected={headCommit} onSelect={setHead} />
                    </Flex>
                    <Flex mt={3} align="center" justify="space-between" gap={3} wrap="wrap">
                      <HStack spacing={2} wrap="wrap" minW={0}>
                        <Badge borderRadius="full" px={2} variant="subtle" colorScheme={commitsApart === 0 ? 'gray' : 'blue'}>
                          {rangeLabel}
                        </Badge>
                        {rangeCommits.length > 1 && (
                          <Text fontSize="xs" color="gray.500" isTruncated>
                            {formatCommitDate(rangeCommits[rangeCommits.length - 1].date)}
                            {' → '}
                            {formatCommitDate(rangeCommits[0].date)}
                          </Text>
                        )}
                      </HStack>
                      <Button size="sm" colorScheme="blue" px={6} isLoading={running} isDisabled={!base || commits.length === 0} onClick={runImpact}>
                        Run impact
                      </Button>
                    </Flex>
                    {rangeCommits.length > 0 && (
                      <Box mt={2} borderTop="1px solid" borderColor="whiteAlpha.100" pt={1}>
                        <Button
                          variant="ghost"
                          size="xs"
                          color="gray.400"
                          _hover={{ color: 'gray.100' }}
                          rightIcon={<ChevronDownIcon transform={showRangeCommits ? 'rotate(180deg)' : undefined} transition="transform 0.2s" />}
                          onClick={() => setShowRangeCommits((current) => !current)}
                          aria-expanded={showRangeCommits}
                        >
                          {rangeCommits.length} commit{rangeCommits.length === 1 ? '' : 's'} in range
                        </Button>
                        <Collapse in={showRangeCommits} animateOpacity>
                          <VStack align="stretch" spacing={0} mt={1}>
                            {rangeCommits.slice(0, 8).map((commit, index) => (
                              <HStack key={commit.sha} spacing={3} py={1.5} align="flex-start">
                                <VStack spacing={0} align="center" pt={1}>
                                  <Box w={2} h={2} borderRadius="full" bg={index === 0 || index === rangeCommits.slice(0, 8).length - 1 ? 'var(--accent)' : 'gray.500'} />
                                  {index < Math.min(rangeCommits.length, 8) - 1 && <Box w="1px" h={4} bg="whiteAlpha.100" />}
                                </VStack>
                                <Box flex="1" minW={0}>
                                  <HStack spacing={2} minW={0} wrap="wrap">
                                    <Code fontSize="xs" color="gray.300">{commit.short_sha}</Code>
                                    <Text fontSize="sm" color="gray.200" isTruncated flex="1">
                                      {commit.subject}
                                    </Text>
                                  </HStack>
                                  <Text fontSize="xs" color="gray.500">
                                    {[commit.author, formatCommitDate(commit.date)].filter(Boolean).join(' · ')}
                                  </Text>
                                </Box>
                              </HStack>
                            ))}
                            {rangeCommits.length > 8 && (
                              <Text fontSize="xs" color="gray.500" pl={5}>
                                +{rangeCommits.length - 8} more in range
                              </Text>
                            )}
                          </VStack>
                        </Collapse>
                      </Box>
                    )}
                  </Box>
                </Box>

                {running && (
                  <Flex py={8} align="center" justify="center" direction="column" gap={3} color="gray.600" borderBottom="1px solid" borderColor="whiteAlpha.100">
                    <Spinner size="lg" color="var(--accent)" />
                    <Text fontSize="sm">Analyzing impact…</Text>
                  </Flex>
                )}

                {report && !running && (
                  <>
                    {reportStats && (
                      <Flex borderBottom="1px solid" borderColor="whiteAlpha.100" align="stretch">
                        <StatCell label="Changed elements" value={String(reportStats.elements)} sub={`${report.related.length} related`} />
                        <Box w="1px" alignSelf="stretch" bg="whiteAlpha.100" flexShrink={0} />
                        <StatCell label="Changed files" value={String(reportStats.files)} sub={`+${reportStats.added} / -${reportStats.removed} lines`} />
                        <Box w="1px" alignSelf="stretch" bg="whiteAlpha.100" flexShrink={0} />
                        <StatCell label="Coverage" value={coverageLabel(report.coverage)} sub={`${report.coverage.anchored_elements}/${report.coverage.total_elements} bound`} />
                        <Box w="1px" alignSelf="stretch" bg="whiteAlpha.100" flexShrink={0} />
                        <StatCell label="Binding gaps" value={String(reportStats.gaps)} sub={reportStats.gaps === 0 ? 'Fully bound' : 'Needs attention'} />
                      </Flex>
                    )}
                    <Box px={4} py={3}>
                      <Tabs
                        size="sm"
                        variant="enclosed"
                        index={resultTab === 'architecture' ? 0 : resultTab === 'files' ? 1 : 2}
                        onChange={(index) => setResultTab(index === 0 ? 'architecture' : index === 1 ? 'files' : 'coverage')}
                      >
                        <TabList>
                          <Tab>
                            Architecture
                          </Tab>
                          <Tab>
                            Files
                            <Badge ml={1.5} variant="subtle" fontSize="2xs" borderRadius="full">{report.changed_files.length}</Badge>
                          </Tab>
                          <Tab>
                            Gaps
                            <Badge ml={1.5} variant="subtle" fontSize="2xs" borderRadius="full" colorScheme={report.coverage.gaps.length ? 'orange' : 'green'}>
                              {report.coverage.gaps.length}
                            </Badge>
                          </Tab>
                        </TabList>
                        <TabPanels>
                          <TabPanel p={3}>
                            <Flex mb={3} align="center" justify="space-between" gap={3} wrap="wrap">
                              <Box w="200px">
                                <SegmentedControl<ArchitectureView>
                                  ariaLabel="Architecture view"
                                  value={architectureView}
                                  onChange={setArchitectureView}
                                  options={[
                                    { value: 'diagram', label: 'Diagram' },
                                    { value: 'markdown', label: 'Markdown' },
                                  ]}
                                />
                              </Box>
                              <Tooltip label="Copy the PR-comment markdown" placement="top">
                                <Button
                                  size="xs"
                                  variant="ghost"
                                  color="gray.400"
                                  _hover={{ color: 'gray.100' }}
                                  leftIcon={<CopyIcon />}
                                  onClick={() => copyPath(impactMarkdownText, 'Markdown')}
                                >
                                  Copy as markdown
                                </Button>
                              </Tooltip>
                            </Flex>
                            {architectureView === 'markdown' ? (
                              <Box
                                h="460px"
                                overflowY="auto"
                                bg="var(--bg-canvas)"
                                border="1px solid"
                                borderColor="var(--border-main)"
                                borderRadius="xl"
                                sx={markdownPanelBodySx}
                              >
                                <Box p={4}>
                                  <MarkdownPreview markdown={impactMarkdownText} />
                                </Box>
                              </Box>
                            ) : (
                              <ImpactCanvas report={report} onOpenElement={openElement} />
                            )}
                          </TabPanel>
                          <TabPanel p={3}>
                            <Flex gap={2} mb={3} direction={{ base: 'column', md: 'row' }} align={{ base: 'stretch', md: 'center' }}>
                              <InputGroup size="sm" flex="1">
                                <InputLeftElement pointerEvents="none" color="gray.500">
                                  <SearchIcon boxSize={3.5} />
                                </InputLeftElement>
                                <Input placeholder="Filter files or elements…" value={fileQuery} onChange={(event) => setFileQuery(event.target.value)} variant="elevated" _placeholder={{ color: 'gray.600' }} />
                              </InputGroup>
                              <HStack spacing={2}>
                                <Select size="sm" maxW="140px" value={fileChangeFilter} onChange={(event) => setFileChangeFilter(event.target.value as FileChangeFilter)}>
                                  <option value="all">All changes</option>
                                  <option value="added">Added</option>
                                  <option value="modified">Modified</option>
                                  <option value="deleted">Deleted</option>
                                </Select>
                                <SegmentedControl<ChangeView>
                                  ariaLabel="Change view"
                                  value={changeView}
                                  onChange={setChangeView}
                                  options={[
                                    { value: 'files', label: 'Files' },
                                    { value: 'elements', label: 'Elements' },
                                  ]}
                                />
                              </HStack>
                            </Flex>
                            <Box maxH="460px" overflowY="auto" pr={1}>
                              {changeView === 'files' ? (
                                <ChangedFilesTree files={report.changed_files} query={fileQuery} changeFilter={fileChangeFilter} />
                              ) : (
                                <VStack align="stretch" spacing={3}>
                                  {report.changed.length > 0 && (
                                    <Box>
                                      <Box px={2} mb={1}>
                                        <MicroLabel>Changed architecture</MicroLabel>
                                      </Box>
                                      {report.changed
                                        .filter((element) => !fileQuery.trim() || element.name.toLowerCase().includes(fileQuery.trim().toLowerCase()))
                                        .map((element) => (
                                          <Button
                                            key={element.ref}
                                            variant="ghost"
                                            size="sm"
                                            justifyContent="flex-start"
                                            px={2}
                                            w="full"
                                            fontWeight="normal"
                                            onClick={() => element.element_id && openElement(element.element_id)}
                                            isDisabled={!element.element_id}
                                          >
                                            <Box w={2} h={2} borderRadius="sm" bg="green.400" mr={2} flexShrink={0} />
                                            <Text fontSize="sm" isTruncated>
                                              {element.name}
                                            </Text>
                                          </Button>
                                        ))}
                                    </Box>
                                  )}
                                  {report.unmapped.length > 0 && (
                                    <Box>
                                      <Box px={2} mb={1}>
                                        <MicroLabel>Needs binding</MicroLabel>
                                      </Box>
                                      {report.unmapped
                                        .filter((file) => !fileQuery.trim() || file.toLowerCase().includes(fileQuery.trim().toLowerCase()))
                                        .map((file) => (
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
                                      <Box px={2} mb={1}>
                                        <MicroLabel>Related context</MicroLabel>
                                      </Box>
                                      {report.related
                                        .filter((element) => !fileQuery.trim() || element.name.toLowerCase().includes(fileQuery.trim().toLowerCase()))
                                        .map((element) => (
                                          <Button
                                            key={element.ref}
                                            variant="ghost"
                                            size="sm"
                                            justifyContent="flex-start"
                                            px={2}
                                            w="full"
                                            fontWeight="normal"
                                            onClick={() => element.element_id && openElement(element.element_id)}
                                            isDisabled={!element.element_id}
                                          >
                                            <Box w={2} h={2} borderRadius="sm" bg="blue.400" mr={2} flexShrink={0} />
                                            <Text fontSize="sm" isTruncated>
                                              {element.name}
                                            </Text>
                                          </Button>
                                        ))}
                                    </Box>
                                  )}
                                  {report.changed.length === 0 && report.unmapped.length === 0 && report.related.length === 0 && (
                                    <Text fontSize="sm" color="gray.500" px={2} py={3}>
                                      No changed files or architecture elements.
                                    </Text>
                                  )}
                                </VStack>
                              )}
                            </Box>
                          </TabPanel>
                          <TabPanel p={3}>
                            <CoveragePanel coverage={report.coverage} />
                          </TabPanel>
                        </TabPanels>
                      </Tabs>
                    </Box>
                  </>
                )}

                {!report && !running && commits.length === 0 && (
                  <Box px={4} py={6} textAlign="center">
                    <Text fontSize="sm" color="gray.500">
                      No commits found for this checkout. Check the branch name and reload status.
                    </Text>
                  </Box>
                )}
              </>
            )}
          </Box>
        </Flex>
      )}

      <AddRepositoryModal
        isOpen={addDisclosure.isOpen}
        onClose={addDisclosure.onClose}
        onAdded={(added) => {
          void load().then(() => selectRepo(added.ref))
          toast({ title: `Linked ${added.name}`, status: 'success', duration: 2000 })
        }}
      />

      <ConfirmDialog
        isOpen={deleteDisclosure.isOpen}
        onClose={() => { setPendingDelete(null); deleteDisclosure.onClose() }}
        onConfirm={() => { void confirmRemove() }}
        title="Remove repository?"
        body={pendingDelete ? `Unlink ${pendingDelete.name} (${pendingDelete.local_path || pendingDelete.ref})? This does not delete files on disk.` : 'Unlink this repository?'}
        confirmLabel="Remove"
        confirmColorScheme="red"
        isLoading={removing}
      />
    </Box>
  )
}
