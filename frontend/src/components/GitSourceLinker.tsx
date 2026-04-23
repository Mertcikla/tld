import { useState, useEffect, useRef, useMemo } from 'react'
import { createPortal } from 'react-dom'
import { AnimatePresence, motion } from 'framer-motion'
import {
  Alert,
  AlertDescription,
  AlertIcon,
  Badge,
  Box,
  Button,
  CloseButton,
  FormControl,
  FormLabel,
  HStack,
  Input,
  InputGroup,
  InputRightElement,
  Progress,
  Spinner,
  Text,
  Tooltip,
  VStack,
  useToast,
  Accordion,
  AccordionItem,
  AccordionButton,
  AccordionPanel,
  AccordionIcon,
  Portal,
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverBody,
} from '@chakra-ui/react'
import { CheckIcon, ChevronRightIcon, ExternalLinkIcon, SearchIcon } from '@chakra-ui/icons'
import { getParser, extractSymbols, detectLanguage, type SupportedLanguage, type ParsedSymbol } from '../utils/treesitter'
import { githubCache } from '../utils/githubCache'
import { parseRepoSlug } from '../utils/url'
import { githubRequest } from '../utils/githubApi'
import type { LibraryElement } from '../types'

interface Props {
  element: LibraryElement
  isReadOnly: boolean
  onUpdate: (updates: Partial<LibraryElement>) => void
}

const SUPPORTED_LANGUAGES: SupportedLanguage[] = ['javascript', 'typescript', 'python', 'java', 'cpp', 'go', 'rust']

function parseExistingLink(element: LibraryElement) {
  const fp = element.file_path || ''
  const hashIdx = fp.indexOf('#')
  const basePath = hashIdx >= 0 ? fp.slice(0, hashIdx) : fp
  const anchorStr = hashIdx >= 0 ? fp.slice(hashIdx + 1) : ''
  let symbolName = ''
  let pickedLine: number | null = null
  if (anchorStr) {
    try {
      const p = JSON.parse(anchorStr)
      if (p.name) symbolName = p.name
      if (p.startLine) pickedLine = p.startLine
    } catch { /* intentionally empty */ }
  }
  return { basePath, symbolName, pickedLine }
}

const STEP_LABELS = ['Repo', 'Branch', 'File', 'Symbol']

function StepIndicator({ step }: { step: number }) {
  return (
    <Box mb={4} position="relative" w="full">
      {/* Static background line spanning all circles */}
      <Box position="absolute" top="9px" left="12.5%" right="12.5%" h="1px" bg="whiteAlpha.100" zIndex={0} />
      {/* Completed portion */}
      <Box
        position="absolute" top="9px" left="12.5%"
        w={step > 1 ? `${((step - 1) / 3) * 75}%` : '0%'}
        h="1px" bg="blue.700" zIndex={0} transition="width 0.3s"
      />
      <HStack spacing={0} w="full" position="relative" zIndex={1}>
        {STEP_LABELS.map((label, i) => {
          const n = i + 1
          const isCompleted = step > n
          const isActive = step === n
          return (
            <VStack key={n} flex={1} spacing={1} align="center">
              <Box
                w="20px" h="20px" rounded="full"
                display="flex" alignItems="center" justifyContent="center"
                fontSize="10px" fontWeight="bold"
                bg={isCompleted ? 'blue.500' : isActive ? 'var(--accent)' : 'whiteAlpha.150'}
                color={isCompleted || isActive ? 'white' : 'gray.500'}
                boxShadow={isActive ? '0 0 0 3px rgba(99,179,237,0.25)' : 'none'}
                transition="all 0.2s"
              >
                {isCompleted ? <CheckIcon w={2.5} h={2.5} /> : n}
              </Box>
              <Text
                fontSize="9px" textAlign="center" textTransform="uppercase"
                color={isCompleted ? 'blue.300' : isActive ? 'white' : 'gray.600'}
                fontWeight={isActive ? '600' : '400'}
              >
                {label}
              </Text>
            </VStack>
          )
        })}
      </HStack>
    </Box>
  )
}

// Scrollable code line renderer - fills its parent container
interface CodeLinesProps {
  code: string
  highlightStart?: number | null
  highlightEnd?: number | null
  selectedLine?: number | null
  searchQuery?: string
  onLineClick?: (line: number) => void
}

function CodeLines({ code, highlightStart, highlightEnd, selectedLine, searchQuery, onLineClick }: CodeLinesProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const lines = useMemo(() => code.split('\n'), [code])

  const matchingLines = useMemo(() => {
    if (!searchQuery?.trim()) return new Set<number>()
    const q = searchQuery.toLowerCase()
    const set = new Set<number>()
    lines.forEach((l, i) => { if (l.toLowerCase().includes(q)) set.add(i + 1) })
    return set
  }, [lines, searchQuery])

  const firstMatch = useMemo(() => Array.from(matchingLines)[0] ?? null, [matchingLines])

  useEffect(() => {
    if (!containerRef.current) return
    const target = highlightStart ?? firstMatch
    if (!target) return
    const el = containerRef.current.querySelector(`[data-line="${target}"]`)
    el?.scrollIntoView({ block: 'center', behavior: 'smooth' })
  }, [highlightStart, firstMatch])

  return (
    <Box
      ref={containerRef}
      flex={1} overflowY="auto"
      fontFamily="'Menlo', 'Monaco', 'Consolas', monospace" fontSize="11px" lineHeight="1.55"
      bg="#111827"
      className="custom-scrollbar"
    >
      {lines.map((line, i) => {
        const lineNum = i + 1
        const inHighlight = highlightStart != null && highlightEnd != null
          ? lineNum >= highlightStart && lineNum <= highlightEnd
          : false
        const isSelected = selectedLine === lineNum
        const isMatch = matchingLines.has(lineNum)

        return (
          <Box
            key={i}
            data-line={lineNum}
            display="flex"
            alignItems="stretch"
            bg={isSelected ? 'rgba(49,130,206,0.35)' : inHighlight ? 'rgba(49,130,206,0.12)' : isMatch ? 'rgba(236,201,75,0.06)' : 'transparent'}
            borderLeft="2px solid"
            borderColor={isSelected ? 'blue.400' : inHighlight ? 'blue.800' : 'transparent'}
            cursor={onLineClick ? 'pointer' : 'default'}
            _hover={onLineClick ? { bg: isSelected ? 'rgba(49,130,206,0.45)' : 'whiteAlpha.50' } : {}}
            onClick={() => onLineClick?.(lineNum)}
            transition="background 0.1s"
          >
            <Box
              w="38px" flexShrink={0}
              px={1.5} py={0.5}
              color={isSelected ? 'blue.300' : inHighlight ? 'blue.600' : isMatch ? 'yellow.500' : 'gray.700'}
              fontSize="10px" textAlign="right"
              userSelect="none"
              borderRight="1px solid" borderColor="whiteAlpha.50"
            >
              {lineNum}
            </Box>
            <Box
              px={2} py={0.5}
              color={isSelected ? 'white' : inHighlight ? 'blue.100' : isMatch ? 'yellow.200' : 'gray.400'}
              whiteSpace="pre" flex={1} overflow="hidden" textOverflow="ellipsis"
            >
              {line || '\u00a0'}
            </Box>
          </Box>
        )
      })}
    </Box>
  )
}

// Floating preview card - renders via portal, positioned to the left of the right ElementPanel
interface PreviewCardProps {
  filename: string
  isLoading: boolean
  rawCode: string
  highlightStart?: number | null
  highlightEnd?: number | null
  selectedLine?: number | null
  searchQuery?: string
  onLineClick?: (line: number) => void
  isUnsupported: boolean
  onSearchChange?: (q: string) => void
}

function PreviewCard({
  filename, isLoading, rawCode,
  highlightStart, highlightEnd,
  selectedLine, searchQuery, onLineClick,
  isUnsupported, onSearchChange,
}: PreviewCardProps) {
  const EASE = [0.25, 0.46, 0.45, 0.94]

  return createPortal(
    <AnimatePresence>
      <motion.div
        key="git-preview-card"
        initial={{ x: 32, opacity: 0 }}
        animate={{ x: 0, opacity: 1 }}
        exit={{ x: 32, opacity: 0 }}
        transition={{ duration: 0.2, ease: EASE }}
        style={{
          position: 'fixed',
          right: 'calc(300px + 1rem + 12px)',
          top: 0,
          bottom: 0,
          display: 'flex',
          alignItems: 'center',
          zIndex: 999,
          pointerEvents: 'none',
        }}
      >
        <Box
          pointerEvents="auto"
          w="420px"
          h="calc(90vh - 2rem)"
          maxH="calc(90vh - 2rem)"
          display="flex"
          flexDir="column"
          bg="var(--bg-panel)"
          bgImage="var(--grad-panel)"
          backdropFilter="blur(24px)"
          border="1px solid"
          borderColor="whiteAlpha.100"
          borderTop="2px solid"
          borderTopColor="var(--accent)"
          rounded="xl"
          shadow="panel"
          overflow="hidden"
        >
          {/* Header */}
          <Box px={4} pt={4} pb={3} borderBottom="1px solid" borderColor="whiteAlpha.100" flexShrink={0}>
            <Text fontSize="10px" color="gray.600" letterSpacing="widest" fontWeight="600" mb={0.5}>
              Preview
            </Text>
            <Text fontSize="sm" fontWeight="700" color="white" isTruncated letterSpacing="0.01em">
              {filename}
            </Text>
            {highlightStart != null && !isUnsupported && (
              <Text fontSize="10px" color="blue.400" fontFamily="mono" mt={0.5}>
                L{highlightStart}–L{highlightEnd ?? highlightStart}
              </Text>
            )}
            {isUnsupported && (
              <InputGroup size="sm" mt={2.5}>
                <InputRightElement pointerEvents="none">
                  <SearchIcon color="gray.600" boxSize={3} />
                </InputRightElement>
                <Input
                  value={searchQuery || ''}
                  onChange={e => onSearchChange?.(e.target.value)}
                  placeholder="Search in file..."
                  pr={8}
                  bg="whiteAlpha.50"
                  border="1px solid"
                  borderColor="whiteAlpha.150"
                  _focus={{ borderColor: 'blue.500', bg: 'whiteAlpha.100' }}
                />
              </InputGroup>
            )}
          </Box>

          {/* Body */}
          <Box flex={1} overflow="hidden" display="flex" flexDir="column" position="relative">
            {isLoading ? (
              <VStack flex={1} justify="center" align="center" spacing={3}>
                <Spinner color="blue.400" size="md" />
                <Text fontSize="xs" color="gray.500">
                  {isUnsupported ? 'Fetching file...' : 'Fetching & parsing symbols...'}
                </Text>
              </VStack>
            ) : rawCode ? (
              <CodeLines
                code={rawCode}
                highlightStart={highlightStart}
                highlightEnd={highlightEnd}
                selectedLine={selectedLine}
                searchQuery={searchQuery}
                onLineClick={onLineClick}
              />
            ) : (
              <VStack flex={1} justify="center" align="center">
                <Text fontSize="xs" color="gray.600">No preview available</Text>
              </VStack>
            )}
          </Box>
        </Box>
      </motion.div>
    </AnimatePresence>,
    document.body
  )
}

export default function GitSourceLinker({ element, isReadOnly, onUpdate }: Props) {
  const { basePath: initBasePath, symbolName: initSymbolName, pickedLine: initPickedLine } = parseExistingLink(element)

  const hasExistingLink = !!(element.repo && element.file_path)
  const [mode, setMode] = useState<'summary' | 'edit'>(hasExistingLink ? 'summary' : 'edit')

  const [step, setStep] = useState<1 | 2 | 3 | 4>(1)

  // Step 1
  const [repo, setRepo] = useState(element.repo || '')

  // Step 2
  const [branch, setBranch] = useState(element.branch || '')
  const [branches, setBranches] = useState<string[]>([])
  const [branchSearch, setBranchSearch] = useState('')
  const [branchOpen, setBranchOpen] = useState(false)
  const [branchLoading, setBranchLoading] = useState(false)
  const [branchActiveIndex, setBranchActiveIndex] = useState(0)
  const branchRef = useRef<HTMLDivElement>(null)
  const branchInputRef = useRef<HTMLInputElement>(null)

  // Step 3
  const [filePath, setFilePath] = useState(initBasePath)
  const [fileSearch, setFileSearch] = useState(initBasePath)
  const [fileTree, setFileTree] = useState<string[]>([])
  const [fileOpen, setFileOpen] = useState(false)
  const [fileTreeLoading, setFileTreeLoading] = useState(false)
  const [fileActiveIndex, setFileActiveIndex] = useState(0)
  const [language, setLanguage] = useState(element.language || '')
  const fileRef = useRef<HTMLDivElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  // Step 4: symbols
  const [symbols, setSymbols] = useState<ParsedSymbol[]>([])
  const [selectedSymbol, setSelectedSymbol] = useState<{ name: string; type: string } | null>(
    initSymbolName ? { name: initSymbolName, type: '' } : null
  )
  const [symbolLoading, setSymbolLoading] = useState(false)

  // Step 4: code preview (shared)
  const [rawCode, setRawCode] = useState('')
  const [rawCodeLoading, setRawCodeLoading] = useState(false)

  // Step 4b: line picker
  const [pickedLine, setPickedLine] = useState<number | null>(initPickedLine)
  const [lineSearch, setLineSearch] = useState('')

  const [apiRateLimited, setApiRateLimited] = useState(false)
  const toast = useToast()

  useEffect(() => {
    const hasLink = !!(element.repo && element.file_path)

    // Only force a reset if we are not currently editing, or if the link changed from underneath us
    if (mode === 'summary' || (element.repo !== repo && element.file_path !== filePath)) {
      setMode(hasLink ? 'summary' : 'edit')
      if (hasLink) {
        const { basePath, symbolName, pickedLine: pl } = parseExistingLink(element)
        setStep(1)
        setRepo(element.repo || '')
        setBranch(element.branch || '')
        setFilePath(basePath)
        setFileSearch(basePath)
        setLanguage(element.language || '')
        setSelectedSymbol(symbolName ? { name: symbolName, type: '' } : null)
        setPickedLine(pl)
        setBranches([])
        setFileTree([])
        setApiRateLimited(false)
        setSymbols([])
        setRawCode('')
        setLineSearch('')
      }
    }
  }, [element, filePath, mode, repo])

  useEffect(() => {
    setBranchActiveIndex(0)
  }, [branchSearch, branches])

  useEffect(() => {
    setFileActiveIndex(0)
  }, [fileSearch, fileTree])

  useEffect(() => {
    if (step === 2) {
      setTimeout(() => branchInputRef.current?.focus(), 100)
    } else if (step === 3) {
      setTimeout(() => fileInputRef.current?.focus(), 100)
    }
  }, [step])

  const filteredBranches = branches.filter(b => b.toLowerCase().includes(branchSearch.toLowerCase()))
  const filteredFiles = fileTree.filter(p => p.toLowerCase().includes(fileSearch.toLowerCase())).slice(0, 100)
  const isSupported = SUPPORTED_LANGUAGES.includes(language as SupportedLanguage)

  // Find full symbol data (with line numbers) for the currently selected symbol
  const selectedSymbolData = selectedSymbol
    ? symbols.find(s => s.name === selectedSymbol.name && s.type === selectedSymbol.type) ?? null
    : null

  async function fetchBranches(repoSlug: string) {
    const cached = githubCache.getBranches(repoSlug)
    if (cached) {
      setBranches(cached)
      if (!branch) {
        const def = cached.find(n => n === 'main') ?? cached.find(n => n === 'master')
        if (def) setBranch(def)
      }
      setBranchOpen(true)
      return
    }

    setBranchLoading(true)
    setApiRateLimited(false)
    try {
      const res = await githubRequest(`/repos/${repoSlug}/branches?per_page=100`)
      if (res.status === 403 || res.status === 429) { setApiRateLimited(true); return }
      if (!res.ok) throw new Error(res.statusText)
      const data: { name: string }[] = await res.json()
      const names = data.map(b => b.name)
      githubCache.setBranches(repoSlug, names)
      setBranches(names)
      if (!branch) {
        const def = names.find(n => n === 'main') ?? names.find(n => n === 'master')
        if (def) setBranch(def)
      }
      setBranchOpen(true) // Auto-open dropdown once fetched
    } catch {
      setApiRateLimited(true)
    } finally {
      setBranchLoading(false)
    }
  }

  const [fileTreeTruncated, setFileTreeTruncated] = useState(false)

  async function fetchFileTree(repoSlug: string, branchName: string) {
    const cached = githubCache.getTree(repoSlug, branchName)
    if (cached) {
      setFileTree(cached)
      setFileOpen(true)
      return
    }

    setFileTreeLoading(true)
    setApiRateLimited(false)
    setFileTreeTruncated(false)
    try {
      const res = await githubRequest(`/repos/${repoSlug}/git/trees/${branchName}?recursive=1`)
      if (res.status === 403 || res.status === 429) { setApiRateLimited(true); return }
      if (!res.ok) throw new Error(res.statusText)
      const data: { tree: { path: string; type: string }[]; truncated?: boolean } = await res.json()
      const files = data.tree.filter(i => i.type === 'blob').map(i => i.path)
      githubCache.setTree(repoSlug, branchName, files)
      setFileTree(files)
      if (data.truncated) setFileTreeTruncated(true)
      setFileOpen(true)
    } catch {
      setApiRateLimited(true)
    } finally {
      setFileTreeLoading(false)
    }
  }

  async function fetchAndParseSymbols(lang: SupportedLanguage) {
    setSymbolLoading(true)
    setSymbols([])
    setRawCode('')
    try {
      const rawUrl = `https://raw.githubusercontent.com/${repo}/refs/heads/${branch}/${filePath}`
      let source = githubCache.getContent(rawUrl)
      if (!source) {
        const res = await fetch(rawUrl)
        if (!res.ok) throw new Error(`Failed to fetch: ${res.statusText}`)
        source = await res.text()
        githubCache.setContent(rawUrl, source)
      }
      setRawCode(source)
      const parser = await getParser(lang)
      const tree = parser.parse(source)
      const extracted = extractSymbols(tree, lang)
      setSymbols(extracted)
      if (extracted.length === 0) {
        toast({ title: 'No symbols found in this file', status: 'info', duration: 3000 })
      }
    } catch (err: unknown) {
      toast({ title: 'Failed to fetch/parse file', description: err instanceof Error ? err.message : String(err), status: 'error', duration: 4000 })
    } finally {
      setSymbolLoading(false)
    }
  }

  async function fetchRawCode() {
    setRawCodeLoading(true)
    setRawCode('')
    try {
      const rawUrl = `https://raw.githubusercontent.com/${repo}/refs/heads/${branch}/${filePath}`
      const cached = githubCache.getContent(rawUrl)
      if (cached) {
        setRawCode(cached)
      } else {
        const res = await fetch(rawUrl)
        if (!res.ok) throw new Error(`Failed to fetch: ${res.statusText}`)
        const text = await res.text()
        githubCache.setContent(rawUrl, text)
        setRawCode(text)
      }
    } catch (err: unknown) {
      toast({ title: 'Failed to fetch file', description: err instanceof Error ? err.message : String(err), status: 'error', duration: 4000 })
    } finally {
      setRawCodeLoading(false)
    }
  }

  function advanceTo(nextStep: 1 | 2 | 3 | 4) {
    setStep(nextStep)
    if (nextStep === 2) {
      // Normalize repo slug before fetching branches
      const slug = parseRepoSlug(repo)
      setRepo(slug)

      // Avoid refetching if we already have branches for this repo
      if (branches.length === 0 || slug !== (element.repo ? parseRepoSlug(element.repo) : '')) {
        fetchBranches(slug)
      } else {
        setBranchOpen(true)
      }
    }
    if (nextStep === 3) {
      // Avoid refetching if we already have the tree for this repo/branch
      const slug = parseRepoSlug(repo)
      if (fileTree.length === 0 || slug !== (element.repo ? parseRepoSlug(element.repo) : '') || branch !== element.branch) {
        fetchFileTree(slug, branch)
      } else {
        setFileOpen(true)
      }
    }
    if (nextStep === 4) {
      const detected = detectLanguage(filePath)
      const lang = detected ?? 'unsupported'
      setLanguage(lang)
      if (SUPPORTED_LANGUAGES.includes(lang as SupportedLanguage)) {
        fetchAndParseSymbols(lang as SupportedLanguage)
      } else {
        fetchRawCode()
      }
    }
  }

  function handleFileSelect(path: string) {
    setFilePath(path)
    setFileSearch(path)
    setFileOpen(false)
    setSelectedSymbol(null)
    setSymbols([])
    setRawCode('')
    setPickedLine(null)
  }

  function buildFilePath(): string {
    if (isSupported && selectedSymbol) {
      return `${filePath}#${JSON.stringify({ name: selectedSymbol.name, type: selectedSymbol.type })}`
    }
    if (!isSupported && pickedLine) {
      return `${filePath}#${JSON.stringify({ startLine: pickedLine, endLine: pickedLine })}`
    }
    return filePath
  }

  function handleApply() {
    onUpdate({
      repo: parseRepoSlug(repo),
      branch,
      file_path: buildFilePath(),
      language: isSupported ? language : undefined,
    })
    setMode('summary')
  }

  function handleRemoveLink() {
    onUpdate({ repo: null, branch: null, file_path: null, language: null })
    // Explicitly reset local state to ensure immediate UI update
    setRepo('')
    setBranch('')
    setFilePath('')
    setFileSearch('')
    setLanguage('')
    setSymbols([])
    setRawCode('')
    setPickedLine(null)
    setSelectedSymbol(null)
    setMode('edit')
    setStep(1)
  }

  const repoValid = /^[\w.-]+\/[\w.-]+$/.test(parseRepoSlug(repo))
  const showPreviewCard = mode === 'edit' && step === 4

  // --- RENDER ---
  return (
    <Box borderTop="1px solid" borderColor="whiteAlpha.100" pt={1} overflow="visible">
      <Accordion allowToggle overflow="visible">
        <AccordionItem border="none" overflow="visible">
          <h2>
            <AccordionButton px={0} py={3} _hover={{ bg: 'transparent' }}>
              <HStack flex="1" textAlign="left" spacing={2}>
                <Text fontSize="sm" fontFamily="var(--chakra-fonts-heading)" >
                  Git Source
                </Text>
                {hasExistingLink && mode === 'summary' && (
                  <Badge variant="subtle" colorScheme="blue" fontSize="9px" ml={1} px={1.5}>
                    Linked
                  </Badge>
                )}
              </HStack>
              <AccordionIcon color="gray.500" />
            </AccordionButton>
          </h2>
          <AccordionPanel pb={4} px={0} overflow="visible">
            {mode === 'summary' ? (
              <VStack align="stretch" spacing={2}>
                <VStack align="stretch" spacing={1.5}
                  bg="whiteAlpha.50" rounded="lg" px={3} py={2.5}
                  border="1px solid" borderColor="whiteAlpha.100">
                  <HStack justify="space-between">
                    <HStack spacing={2} minW={0}>
                      <Text fontSize="xs" color="gray.500" flexShrink={0}>Repo</Text>
                      <Text fontSize="xs" color="white" fontFamily="mono" isTruncated>{element.repo ? parseRepoSlug(element.repo) : ''}</Text>
                    </HStack>
                    {!isReadOnly && (
                      <HStack spacing={1}>
                        <Button size="xs" variant="ghost" color="gray.500" h="20px" _hover={{ color: 'white', bg: 'whiteAlpha.100' }}
                          onClick={(e) => { e.stopPropagation(); setStep(1); setMode('edit') }}>
                          Edit
                        </Button>
                        <Tooltip label="Remove link" placement="top">
                          <CloseButton size="xs" color="gray.600" _hover={{ color: 'red.400', bg: 'whiteAlpha.100' }}
                            onClick={(e) => { e.stopPropagation(); handleRemoveLink() }} />
                        </Tooltip>
                      </HStack>
                    )}
                  </HStack>
                  <HStack spacing={2} minW={0}>
                    <Text fontSize="xs" color="gray.500" flexShrink={0}>Branch</Text>
                    <Badge colorScheme="blue" fontSize="9px">{element.branch || 'main'}</Badge>
                  </HStack>
                  <HStack spacing={2} minW={0}>
                    <Text fontSize="xs" color="gray.500" flexShrink={0}>File</Text>
                    <Text fontSize="xs" color="gray.300" fontFamily="mono" isTruncated>{parseExistingLink(element).basePath}</Text>
                  </HStack>
                  {parseExistingLink(element).symbolName && (
                    <HStack spacing={2} minW={0}>
                      <Text fontSize="xs" color="gray.500" flexShrink={0}>Symbol</Text>
                      <Text fontSize="xs" color="blue.300" fontFamily="mono" fontWeight="600">{parseExistingLink(element).symbolName}</Text>
                    </HStack>
                  )}
                  {parseExistingLink(element).pickedLine && !parseExistingLink(element).symbolName && (
                    <HStack spacing={2} minW={0}>
                      <Text fontSize="xs" color="gray.500" flexShrink={0}>Line</Text>
                      <Text fontSize="xs" color="blue.300" fontFamily="mono">L{parseExistingLink(element).pickedLine}</Text>
                    </HStack>
                  )}
                  {element.repo && (
                    <Button
                      as="a"
                      href={`https://github.com/${parseRepoSlug(element.repo)}/blob/${element.branch || 'main'}/${parseExistingLink(element).basePath}`}
                      target="_blank" rel="noopener noreferrer"
                      size="xs" variant="ghost" leftIcon={<ExternalLinkIcon />}
                      justifyContent="flex-start" px={0} mt={0.5} h="auto" py={1}
                      color="blue.400" _hover={{ color: 'blue.200', bg: 'transparent' }}>
                      Open in GitHub
                    </Button>
                  )}
                </VStack>
              </VStack>
            ) : (
              <VStack align="stretch" spacing={3} overflow="visible">
                {showPreviewCard && (
                  <PreviewCard
                    filename={filePath.split('/').pop() || filePath}
                    isLoading={symbolLoading || rawCodeLoading}
                    rawCode={rawCode}
                    highlightStart={selectedSymbolData?.startLine ?? null}
                    highlightEnd={selectedSymbolData?.endLine ?? null}
                    selectedLine={pickedLine}
                    searchQuery={lineSearch}
                    onLineClick={isReadOnly ? undefined : (line) => setPickedLine(line === pickedLine ? null : line)}
                    isUnsupported={!isSupported}
                    onSearchChange={setLineSearch}
                  />
                )}

                <HStack justify="space-between" mb={1}>
                  <Text fontSize="10px" color="gray.500" fontWeight="bold" textTransform="uppercase">
                    {hasExistingLink ? 'Re-configure link' : 'New link'}
                  </Text>
                  {hasExistingLink && (
                    <Button size="xs" variant="ghost" color="gray.500" h="20px" _hover={{ color: 'white', bg: 'whiteAlpha.100' }}
                      onClick={() => setMode('summary')}>
                      Cancel
                    </Button>
                  )}
                </HStack>

                <StepIndicator step={step} />

                {/* STEP 1 */}
                {step === 1 && (
                  <VStack align="stretch" spacing={3}>
                    <FormControl>
                      <FormLabel fontSize="xs" color="gray.400">Repository</FormLabel>
                      <Input
                        size="sm" value={repo}
                        onChange={e => setRepo(e.target.value)}
                        onKeyDown={e => { if (e.key === 'Enter' && repoValid) advanceTo(2) }}
                        placeholder="owner/repo  (e.g. facebook/react)"
                        isDisabled={isReadOnly}
                        bg="whiteAlpha.50"
                        borderColor="whiteAlpha.100"
                        _hover={{ borderColor: 'whiteAlpha.300' }}
                        _focus={{ borderColor: 'blue.500', bg: 'whiteAlpha.100' }}
                      />
                      <Text fontSize="10px" color="gray.600" mt={1}>Public GitHub repositories only</Text>
                    </FormControl>
                    <Button size="sm" rightIcon={<ChevronRightIcon />} isDisabled={!repoValid || isReadOnly}
                      onClick={() => advanceTo(2)} alignSelf="flex-end" colorScheme="blue" variant="outline" h="32px">
                      Next
                    </Button>
                  </VStack>
                )}

                {/* STEP 2 */}
                {step === 2 && (
                  <VStack align="stretch" spacing={3}>
                    <Text fontSize="xs" color="gray.500" fontFamily="mono">{repo}</Text>
                    <FormControl>
                      <FormLabel fontSize="xs" color="gray.400">Branch</FormLabel>
                      {apiRateLimited ? (
                        <>
                          <Alert status="warning" rounded="md" mb={2} py={2} px={3}>
                            <AlertIcon boxSize={3} mr={2} />
                            <AlertDescription fontSize="xs">GitHub API rate limit reached - enter branch manually</AlertDescription>
                          </Alert>
                          <Input size="sm" value={branch} onChange={e => setBranch(e.target.value)} placeholder="main" />
                        </>
                      ) : (
                        <Popover
                          isOpen={branchOpen && filteredBranches.length > 0}
                          onClose={() => setBranchOpen(false)}
                          placement="bottom-start"
                          autoFocus={false}
                          matchWidth
                        >
                          <PopoverTrigger>
                            <Box ref={branchRef}>
                              <InputGroup size="sm">
                                <Input
                                  ref={branchInputRef}
                                  value={branchSearch}
                                  onChange={e => { setBranchSearch(e.target.value); setBranchOpen(true) }}
                                  onFocus={() => setBranchOpen(true)}
                                  onClick={() => setBranchOpen(true)}
                                  onKeyDown={e => {
                                    if (e.key === 'ArrowDown') {
                                      e.preventDefault()
                                      setBranchActiveIndex(prev => Math.min(prev + 1, filteredBranches.length - 1))
                                    } else if (e.key === 'ArrowUp') {
                                      e.preventDefault()
                                      setBranchActiveIndex(prev => Math.max(prev - 1, 0))
                                    } else if (e.key === 'Enter') {
                                      e.preventDefault()
                                      if (filteredBranches.length > 0) {
                                        const selected = filteredBranches[branchActiveIndex]
                                        setBranch(selected)
                                        setBranchSearch('')
                                        setBranchOpen(false)
                                        advanceTo(3)
                                      } else if (branch) {
                                        advanceTo(3)
                                      }
                                    } else if (e.key === 'Escape') {
                                      setBranchOpen(false)
                                    }
                                  }}
                                  placeholder={branchLoading ? 'Loading branches...' : (branch || 'Search branches...')}
                                  isDisabled={branchLoading}
                                  bg="whiteAlpha.50"
                                  borderColor="whiteAlpha.100"
                                />
                                {branchLoading && (
                                  <InputRightElement><Spinner size="xs" color="gray.400" /></InputRightElement>
                                )}
                              </InputGroup>
                              {branchLoading && <Progress isIndeterminate size="xs" colorScheme="blue" mt={1} rounded="full" />}
                            </Box>
                          </PopoverTrigger>
                          <Portal>
                            <PopoverContent
                              bg="var(--bg-panel)"
                              border="1px solid"
                              borderColor="whiteAlpha.200"
                              rounded="lg"
                              shadow="panel-sm"
                              maxH="250px"
                              overflow="hidden"
                            >
                              <PopoverBody p={0} overflowY="auto" className="custom-scrollbar">
                                {filteredBranches.map((b, idx) => (
                                  <Box key={b} px={3} py={1.5} cursor="pointer" fontSize="sm" color="gray.200"
                                    _hover={{ bg: 'whiteAlpha.100' }}
                                    bg={branch === b ? 'blue.900' : branchActiveIndex === idx ? 'whiteAlpha.200' : undefined}
                                    onClick={() => { setBranch(b); setBranchSearch(''); setBranchOpen(false); advanceTo(3) }}>
                                    {b}
                                  </Box>
                                ))}
                              </PopoverBody>
                            </PopoverContent>
                          </Portal>
                        </Popover>
                      )}
                    </FormControl>
                    <HStack justify="space-between">
                      <Button size="sm" variant="ghost" color="gray.500" onClick={() => setStep(1)} h="32px">← Back</Button>
                      <Button size="sm" rightIcon={<ChevronRightIcon />} isDisabled={!branch || isReadOnly}
                        onClick={() => advanceTo(3)} colorScheme="blue" variant="outline" h="32px">
                        Next
                      </Button>
                    </HStack>
                  </VStack>
                )}

                {/* STEP 3 */}
                {step === 3 && (
                  <VStack align="stretch" spacing={3}>
                    <HStack spacing={1.5} flexWrap="wrap">
                      <Text fontSize="xs" color="gray.500" fontFamily="mono">{repo}</Text>
                      <Text fontSize="xs" color="gray.600">/</Text>
                      <Badge colorScheme="blue" fontSize="9px">{branch}</Badge>
                    </HStack>
                    <FormControl>
                      <FormLabel fontSize="xs" color="gray.400">File Path</FormLabel>
                      {apiRateLimited ? (
                        <>
                          <Alert status="warning" rounded="md" mb={2} py={2} px={3}>
                            <AlertIcon boxSize={3} mr={2} />
                            <AlertDescription fontSize="xs">GitHub API rate limit reached enter file path manually</AlertDescription>
                          </Alert>
                          <Input size="sm" value={fileSearch}
                            onChange={e => { setFileSearch(e.target.value); setFilePath(e.target.value) }}
                            onKeyDown={e => { if (e.key === 'Enter' && fileSearch) { handleFileSelect(fileSearch); advanceTo(4) } }}
                            placeholder="src/components/Foo.tsx"
                            bg="whiteAlpha.50" borderColor="whiteAlpha.100" />
                        </>
                      ) : (
                        <Popover
                          isOpen={fileOpen && filteredFiles.length > 0}
                          onClose={() => setFileOpen(false)}
                          placement="bottom-start"
                          autoFocus={false}
                          matchWidth
                        >
                          <PopoverTrigger>
                            <Box ref={fileRef}>
                              <InputGroup size="sm">
                                <Input
                                  ref={fileInputRef}
                                  value={fileSearch}
                                  onChange={e => { setFileSearch(e.target.value); setFileOpen(true) }}
                                  onFocus={() => { if (fileTree.length > 0) setFileOpen(true) }}
                                  onClick={() => { if (fileTree.length > 0) setFileOpen(true) }}
                                  onKeyDown={e => {
                                    if (e.key === 'ArrowDown') {
                                      e.preventDefault()
                                      setFileActiveIndex(prev => Math.min(prev + 1, filteredFiles.length - 1))
                                    } else if (e.key === 'ArrowUp') {
                                      e.preventDefault()
                                      setFileActiveIndex(prev => Math.max(prev - 1, 0))
                                    } else if (e.key === 'Tab') {
                                      e.preventDefault()
                                      if (filteredFiles.length === 0) return
                                      // Find longest common prefix of all matches
                                      let common = filteredFiles[0]
                                      for (const f of filteredFiles.slice(1)) {
                                        let i = 0
                                        while (i < common.length && i < f.length && common[i] === f[i]) i++
                                        common = common.slice(0, i)
                                      }
                                      if (common.length > fileSearch.length) {
                                        // Expand to next directory boundary
                                        const nextSlash = common.indexOf('/', fileSearch.length)
                                        setFileSearch(nextSlash >= 0 ? common.slice(0, nextSlash + 1) : common)
                                      } else if (filteredFiles.length === 1) {
                                        setFileSearch(filteredFiles[0])
                                      }
                                    } else if (e.key === 'Enter') {
                                      e.preventDefault()
                                      if (filteredFiles.length > 0) {
                                        handleFileSelect(filteredFiles[fileActiveIndex])
                                        advanceTo(4)
                                      } else if (fileSearch) {
                                        handleFileSelect(fileSearch)
                                        advanceTo(4)
                                      }
                                    } else if (e.key === 'Escape') {
                                      setFileOpen(false)
                                    }
                                  }}
                                  placeholder={fileTreeLoading ? 'Fetching file tree...' : 'Type to search files...'}
                                  bg="whiteAlpha.50"
                                  borderColor="whiteAlpha.100"
                                  _hover={{ borderColor: 'whiteAlpha.300' }}
                                  _focus={{ borderColor: 'blue.500', bg: 'whiteAlpha.100' }}
                                />
                                {fileTreeLoading && (
                                  <InputRightElement><Spinner size="xs" color="gray.400" /></InputRightElement>
                                )}
                              </InputGroup>
                              {fileTreeLoading && <Progress isIndeterminate size="xs" colorScheme="blue" mt={1} rounded="full" />}
                              {fileTreeTruncated && (
                                <Text fontSize="10px" color="orange.400" mt={1}>
                                  Large repo results may be incomplete. Type the full path if needed.
                                </Text>
                              )}
                            </Box>
                          </PopoverTrigger>
                          <Portal>
                            <PopoverContent
                              bg="var(--bg-panel)"
                              border="1px solid"
                              borderColor="whiteAlpha.200"
                              rounded="lg"
                              shadow="panel-sm"
                              maxH="300px"
                              overflow="hidden"
                            >
                              <PopoverBody p={0} overflowY="auto" className="custom-scrollbar">
                                {filteredFiles.map((p, idx) => (
                                  <Box key={p} px={3} py={1.5} cursor="pointer" fontSize="xs" color="gray.300"
                                    fontFamily="mono" _hover={{ bg: 'whiteAlpha.100' }}
                                    bg={filePath === p ? 'blue.900' : fileActiveIndex === idx ? 'whiteAlpha.200' : undefined}
                                    onClick={() => { handleFileSelect(p); advanceTo(4) }}>
                                    {p}
                                  </Box>
                                ))}
                              </PopoverBody>
                            </PopoverContent>
                          </Portal>
                        </Popover>
                      )}
                    </FormControl>
                    <HStack justify="space-between">
                      <Button size="sm" variant="ghost" color="gray.500" onClick={() => setStep(2)} h="32px">← Back</Button>
                      <Button size="sm" rightIcon={<ChevronRightIcon />}
                        isDisabled={!filePath || isReadOnly}
                        onClick={() => advanceTo(4)} colorScheme="blue" variant="outline" h="32px">
                        Next
                      </Button>
                    </HStack>
                  </VStack>
                )}

                {/* STEP 4 */}
                {step === 4 && (
                  <VStack align="stretch" spacing={3}>
                    <HStack spacing={1.5} flexWrap="wrap">
                      <Text fontSize="xs" color="gray.500" fontFamily="mono" isTruncated maxW="200px">{filePath.split('/').pop()}</Text>
                      {isSupported ? (
                        <Badge colorScheme="green" fontSize="9px">{language}</Badge>
                      ) : (
                        <Badge colorScheme="orange" fontSize="9px">unsupported</Badge>
                      )}
                    </HStack>

                    {isSupported ? (
                      <Box>
                        <FormLabel fontSize="xs" color="gray.400" mb={1.5}>Select Symbol</FormLabel>
                        {symbolLoading ? (
                          <Text fontSize="xs" color="gray.600" py={1}>Parsing symbols...</Text>
                        ) : symbols.length === 0 ? (
                          <Text fontSize="xs" color="gray.500" py={1}>No symbols found</Text>
                        ) : (
                          <VStack align="stretch" spacing={0} maxH="250px" overflowY="auto"
                            rounded="lg" border="1px solid" borderColor="whiteAlpha.100"
                            className="custom-scrollbar">
                            {symbols.map((s, i) => {
                              const isSelected = selectedSymbol?.name === s.name && selectedSymbol?.type === s.type
                              return (
                                <Box key={i} px={3} py={1.5} cursor="pointer"
                                  bg={isSelected ? 'blue.900' : undefined}
                                  _hover={{ bg: isSelected ? 'blue.900' : 'whiteAlpha.80' }}
                                  onClick={() => setSelectedSymbol(isSelected ? null : { name: s.name, type: s.type })}>
                                  <Text fontSize="xs" fontWeight="600" color="white">{s.name}</Text>
                                  <Text fontSize="10px" color="gray.500">{s.type.replace(/_/g, ' ')} · L{s.startLine}</Text>
                                </Box>
                              )
                            })}
                          </VStack>
                        )}
                      </Box>
                    ) : (
                      <Box>
                        <Alert status="warning" rounded="md" py={2} px={3} mb={2}>
                          <AlertIcon boxSize={3.5} mr={2} />
                          <AlertDescription fontSize="10px">
                            Select a line in the preview to link it.
                          </AlertDescription>
                        </Alert>
                        {pickedLine && (
                          <HStack spacing={1.5}>
                            <Text fontSize="10px" color="gray.600">Selected:</Text>
                            <Badge colorScheme="blue" fontSize="9px" fontFamily="mono">Line {pickedLine}</Badge>
                          </HStack>
                        )}
                      </Box>
                    )}

                    <HStack justify="space-between">
                      <Button size="sm" variant="ghost" color="gray.500" onClick={() => setStep(3)} h="32px">← Back</Button>
                      <Button size="sm" colorScheme="blue" onClick={handleApply} isDisabled={isReadOnly || symbolLoading || rawCodeLoading} h="32px">
                        Apply
                      </Button>
                    </HStack>
                  </VStack>
                )}
              </VStack>
            )}
          </AccordionPanel>
        </AccordionItem>
      </Accordion>
    </Box>
  )
}
