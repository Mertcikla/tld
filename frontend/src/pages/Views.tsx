import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import {
  Box,
  Flex,
  Button,
  Text,
  Spinner,
  Center,
  FormControl,
  FormLabel,
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
  useDisclosure,
  useBreakpointValue,
} from '@chakra-ui/react'
import { AddIcon, CloseIcon, SearchIcon } from '@chakra-ui/icons'
import { motion, AnimatePresence } from 'framer-motion'
import ViewsGrid from './ViewsGrid'
import InfiniteZoom, { type InfiniteZoomHandle } from './InfiniteZoom'
import { api } from '../api/client'
import { toast } from '../utils/toast'
import type { ViewTreeNode } from '../types'

interface Props {
  shareSlot?: React.ReactNode
  onShareView?: (viewId: number) => void
}

type ViewType = 'explore' | 'hierarchy'

const MotionBox = motion.create(Box)

function flattenTree(roots: ViewTreeNode[]): ViewTreeNode[] {
  const result: ViewTreeNode[] = []
  const traverse = (node: ViewTreeNode) => {
    result.push(node)
    node.children.forEach(traverse)
  }
  roots.forEach(traverse)
  return result
}

interface DiagramJumpToolbarProps {
  view: ViewType
  searchTerm: string
  searchResults: ViewTreeNode[]
  activeSearchIndex: number
  onSearchChange: (term: string) => void
  onSearchKeyDown: (e: React.KeyboardEvent) => void
  onResultClick: (result: ViewTreeNode) => void
  onViewChange: (view: ViewType) => void
  onCreateOpen: () => void
}

function DiagramJumpToolbar({
  view,
  searchTerm,
  searchResults,
  activeSearchIndex,
  onSearchChange,
  onSearchKeyDown,
  onResultClick,
  onViewChange,
  onCreateOpen,
}: DiagramJumpToolbarProps) {
  const isMobileLayout = useBreakpointValue({ base: true, md: false }) ?? false
  const toolbarRef = useRef<HTMLDivElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const [searchExpanded, setSearchExpanded] = useState(false)
  const bgColor = 'var(--bg)'
  const activeColor = 'clay.bg'
  const inactiveColor = 'gray.400'
  const showSearchInput = searchExpanded || searchTerm.length > 0 || searchResults.length > 0

  const expandSearch = useCallback(() => {
    setSearchExpanded(true)
    window.setTimeout(() => searchInputRef.current?.focus(), 0)
  }, [])

  const maybeCollapseSearch = useCallback(() => {
    window.setTimeout(() => {
      if (toolbarRef.current?.contains(document.activeElement)) return
      if (searchTerm || searchResults.length > 0) return
      setSearchExpanded(false)
    }, 80)
  }, [searchResults.length, searchTerm])

  useEffect(() => {
    if (searchTerm || searchResults.length > 0) return
    if (toolbarRef.current?.contains(document.activeElement)) return
    setSearchExpanded(false)
  }, [searchResults.length, searchTerm])

  return (
    <Box
      ref={toolbarRef}
      position="absolute"
      top={isMobileLayout ? 3 : 4}
      left="50%"
      transform="translateX(-50%)"
      zIndex={1000}
      pointerEvents="auto"
      w={isMobileLayout ? "calc(100vw - 24px)" : "auto"}
      maxW="calc(100vw - 24px)"
    >
      <motion.div
        initial={{ y: -10, opacity: 0 }}
        animate={{ y: 0, opacity: 1 }}
        transition={{ duration: 0.35, ease: [0.16, 1, 0.3, 1] }}
      >
        <Flex
          bg={bgColor}
          backdropFilter="blur(18px) saturate(160%)"
          border="1px solid"
          borderColor="whiteAlpha.100"
          borderRadius={isMobileLayout ? "18px" : "full"}
          p={1}
          gap={1}
          boxShadow="0 10px 32px rgba(0, 0, 0, 0.42), inset 0 1px 0 rgba(255,255,255,0.05)"
          align="center"
          direction={isMobileLayout ? "column" : "row"}
        >
          <HStack spacing={1} w={isMobileLayout ? "full" : "auto"}>
            <Button
              size="sm"
              variant="ghost"
              borderRadius="full"
              position="relative"
              px={isMobileLayout ? 4 : 6}
              h="32px"
              flex={isMobileLayout ? 1 : undefined}
              onClick={() => onViewChange('explore')}
              _hover={{ bg: 'transparent' }}
              _active={{ bg: 'transparent' }}
              color={view === 'explore' ? 'white' : inactiveColor}
              transition="color 0.2s"
            >
              {view === 'explore' && (
                <MotionBox
                  layoutId="active-pill"
                  position="absolute"
                  inset={0}
                  bg={activeColor}
                  borderRadius="full"
                  zIndex={-1}
                  transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}
                />
              )}
              <Text fontSize="xs" fontWeight="bold" zIndex={1}>Explore</Text>
            </Button>
            <Button
              size="sm"
              variant="ghost"
              borderRadius="full"
              position="relative"
              px={isMobileLayout ? 4 : 6}
              h="32px"
              flex={isMobileLayout ? 1 : undefined}
              onClick={() => onViewChange('hierarchy')}
              _hover={{ bg: 'transparent' }}
              _active={{ bg: 'transparent' }}
              color={view === 'hierarchy' ? 'white' : inactiveColor}
              transition="color 0.2s"
            >
              {view === 'hierarchy' && (
                <MotionBox
                  layoutId="active-pill"
                  position="absolute"
                  inset={0}
                  bg={activeColor}
                  borderRadius="full"
                  zIndex={-1}
                  transition={{ type: 'spring', bounce: 0.2, duration: 0.6 }}
                />
              )}
              <Text fontSize="xs" fontWeight="bold" zIndex={1}>Hierarchy</Text>
            </Button>
          </HStack>

          <Box w={isMobileLayout ? "full" : "1px"} h={isMobileLayout ? "1px" : "20px"} bg="whiteAlpha.100" mx={isMobileLayout ? 0 : 1} />

          <HStack spacing={1} w={isMobileLayout ? "full" : "auto"}>
            <Button
              size="sm"
              h="32px"
              leftIcon={<AddIcon fontSize="9px" />}
              bg="var(--accent)"
              color="white"
              _hover={{ bg: "var(--accent)", filter: "brightness(1.1)", transform: 'translateY(-1px)' }}
              _active={{ transform: 'translateY(0)', filter: "brightness(0.9)" }}
              variant="solid"
              borderRadius="full"
              px={4}
              fontSize="xs"
              fontWeight="bold"
              letterSpacing="0.02em"
              onClick={onCreateOpen}
              flexShrink={0}
              transition="transform 0.18s ease, filter 0.18s ease"
            >
              New
            </Button>

            <AnimatePresence initial={false} mode="wait">
              {showSearchInput ? (
                <motion.div
                  key="search-input"
                  initial={{ opacity: 0, scale: 0.96 }}
                  animate={{ opacity: 1, scale: 1 }}
                  exit={{ opacity: 0, scale: 0.96 }}
                  transition={{ duration: 0.16, ease: "easeOut" }}
                  style={isMobileLayout ? { flex: 1, minWidth: 0 } : { width: '320px' }}
                >
                  <InputGroup size="sm" w="full">
                    <InputLeftElement pointerEvents="none" h="full">
                      <SearchIcon color="whiteAlpha.400" fontSize="10px" />
                    </InputLeftElement>
                    <Input
                      ref={searchInputRef}
                      placeholder="Jump to diagram..."
                      value={searchTerm}
                      onChange={(e) => onSearchChange(e.target.value)}
                      onKeyDown={onSearchKeyDown}
                      onFocus={() => setSearchExpanded(true)}
                      onBlur={maybeCollapseSearch}
                      variant="unstyled"
                      fontSize="xs"
                      color="white"
                      h="32px"
                      pr={searchTerm ? 8 : 3}
                      _placeholder={{ color: 'whiteAlpha.300' }}
                    />
                    {searchTerm && (
                      <InputRightElement h="full">
                        <IconButton
                          aria-label="Clear search"
                          icon={<CloseIcon fontSize="8px" />}
                          size="xs"
                          variant="ghost"
                          color="whiteAlpha.400"
                          _hover={{ color: 'white', bg: 'transparent' }}
                          onClick={() => {
                            onSearchChange('')
                            searchInputRef.current?.focus()
                          }}
                        />
                      </InputRightElement>
                    )}
                  </InputGroup>
                </motion.div>
              ) : (
                <motion.div
                  key="search-button"
                  initial={{ opacity: 0, scale: 0.96 }}
                  animate={{ opacity: 1, scale: 1 }}
                  exit={{ opacity: 0, scale: 0.96 }}
                  transition={{ duration: 0.16, ease: "easeOut" }}
                >
                  <IconButton
                    aria-label="Jump to diagram"
                    icon={<SearchIcon fontSize="11px" />}
                    size="sm"
                    h="32px"
                    w="32px"
                    minW="32px"
                    borderRadius="full"
                    variant="ghost"
                    color="whiteAlpha.600"
                    _hover={{ color: 'white', bg: 'whiteAlpha.100' }}
                    onClick={expandSearch}
                    onFocus={expandSearch}
                    onBlur={maybeCollapseSearch}
                  />
                </motion.div>
              )}
            </AnimatePresence>
          </HStack>
        </Flex>
      </motion.div>

      <AnimatePresence>
        {searchResults.length > 0 && (
          <motion.div
            initial={{ opacity: 0, y: 8, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 8, scale: 0.98 }}
            transition={{ duration: 0.18, ease: "easeOut" }}
            style={{
              position: 'absolute',
              top: '100%',
              marginTop: '8px',
              left: isMobileLayout ? 0 : 'auto',
              right: 0,
              width: isMobileLayout ? '100%' : '320px',
              zIndex: 110,
            }}
          >
            <Box
              bg="var(--bg-panel)"
              backdropFilter="blur(24px) saturate(180%)"
              border="1px solid"
              borderColor="var(--border-main)"
              borderRadius="10px"
              overflow="hidden"
              boxShadow="0 20px 50px rgba(0,0,0,0.5), 0 0 0 1px rgba(255,255,255,0.05)"
            >
              {searchResults.map((result, idx) => (
                <Flex
                  key={result.id}
                  px={4}
                  py={2.5}
                  align="center"
                  gap={3}
                  cursor="pointer"
                  bg={idx === activeSearchIndex ? 'whiteAlpha.100' : 'transparent'}
                  _hover={{ bg: 'whiteAlpha.50' }}
                  onClick={() => onResultClick(result)}
                  transition="all 0.15s ease"
                >
                  <Box
                    w="6px"
                    h="6px"
                    borderRadius="full"
                    bg={idx === activeSearchIndex ? 'var(--accent)' : 'whiteAlpha.300'}
                    boxShadow={idx === activeSearchIndex ? `0 0 10px var(--accent)` : 'none'}
                    transition="all 0.2s"
                  />
                  <Box flex={1} minW={0}>
                    <Text color="white" fontSize="xs" fontWeight="600" isTruncated>
                      {result.name}
                    </Text>
                    <Text color="whiteAlpha.500" fontSize="10px" textTransform="uppercase" letterSpacing="0.05em">
                      Level {result.level} • {result.level_label || 'Diagram'}
                    </Text>
                  </Box>
                  {idx === activeSearchIndex && (
                    <HStack spacing={1} opacity={0.8}>
                      <Text color="var(--accent)" fontSize="9px" fontWeight="800" letterSpacing="0.1em">
                        {view === 'explore' ? 'ZOOM' : 'OPEN'}
                      </Text>
                      <Text color="whiteAlpha.400" fontSize="9px">↵</Text>
                    </HStack>
                  )}
                </Flex>
              ))}
            </Box>
          </motion.div>
        )}
      </AnimatePresence>
    </Box>
  )
}

export default function ViewsPage({ shareSlot, onShareView }: Props) {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedView = searchParams.get('view')
  const initialView: ViewType = requestedView === 'edit' ? 'hierarchy' : ((requestedView as ViewType) || 'explore')
  const [view, setView] = useState<ViewType>(initialView)
  const [initializing, setInitializing] = useState(true)
  const [treeData, setTreeData] = useState<ViewTreeNode[]>([])
  const [treeLoading, setTreeLoading] = useState(true)
  const [focusedHierarchyId, setFocusedHierarchyId] = useState<number | null>(null)
  const [searchTerm, setSearchTerm] = useState('')
  const [searchResults, setSearchResults] = useState<ViewTreeNode[]>([])
  const [activeSearchIndex, setActiveSearchIndex] = useState(-1)
  const { isOpen: isCreateOpen, onOpen: onCreateOpen, onClose: onCreateClose } = useDisclosure()
  const [newName, setNewName] = useState('')
  const [isCreating, setIsCreating] = useState(false)
  const exploreRef = useRef<InfiniteZoomHandle>(null)

  const flatTree = useMemo(() => flattenTree(treeData), [treeData])

  const handleViewChange = useCallback((newView: ViewType) => {
    setView(newView)
    const newParams = new URLSearchParams(searchParams)
    newParams.set('view', newView)
    setSearchParams(newParams, { replace: true })
  }, [searchParams, setSearchParams])

  // Sync state with search params
  useEffect(() => {
    const v = searchParams.get('view')
    if (v === 'explore' || v === 'hierarchy') {
      setView(v)
    }
    if (v === 'edit') {
      setView('hierarchy')
    }
  }, [searchParams])

  const refreshTree = useCallback(async () => {
    setTreeLoading(true)
    const tree = await api.workspace.views.tree().catch(() => null)
    if (tree) {
      setTreeData(tree)
      if (tree.length === 0 && !searchParams.get('view')) {
        handleViewChange('hierarchy')
      }
    }
    setTreeLoading(false)
    setInitializing(false)
  }, [handleViewChange, searchParams])

  useEffect(() => {
    let mounted = true
    setTreeLoading(true)
    api.workspace.views.tree()
      .then((tree) => {
        if (!mounted) return
        if (tree) setTreeData(tree)
        if (!tree || tree.length === 0) {
          // Only auto-switch to edit if no view is explicitly set in URL
          if (!searchParams.get('view')) {
            handleViewChange('hierarchy')
          }
        }
      })
      .catch(() => {
        // Fallback to explore on error
      })
      .finally(() => {
        if (mounted) {
          setTreeLoading(false)
          setInitializing(false)
        }
      })
    return () => { mounted = false }
    // Initial tree load only; view changes should not refetch the hierarchy.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const commitSearchResult = useCallback((result: ViewTreeNode) => {
    if (view === 'explore') {
      exploreRef.current?.focusDiagram(result.id)
    } else {
      setFocusedHierarchyId(result.id)
      navigate(`/views/${result.id}`)
    }
    setSearchResults([])
    setActiveSearchIndex(-1)
    setSearchTerm('')
  }, [navigate, view])

  const handleSearchChange = useCallback((term: string) => {
    setSearchTerm(term)
    if (term.trim().length < 3) {
      setSearchResults([])
      setActiveSearchIndex(-1)
      return
    }

    const normalized = term.trim().toLowerCase()
    const matches = flatTree
      .filter((n) => n.name.toLowerCase().includes(normalized))
      .slice(0, 5)

    setSearchResults(matches)
    if (matches.length > 0) {
      setActiveSearchIndex(0)
      if (view === 'hierarchy') setFocusedHierarchyId(matches[0].id)
    } else {
      setActiveSearchIndex(-1)
    }
  }, [flatTree, view])

  const handleSearchKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      setSearchResults([])
      setActiveSearchIndex(-1)
      return
    }
    if (searchResults.length === 0) return

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      const nextIndex = (activeSearchIndex + 1) % searchResults.length
      setActiveSearchIndex(nextIndex)
      if (view === 'hierarchy') setFocusedHierarchyId(searchResults[nextIndex].id)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      const nextIndex = (activeSearchIndex - 1 + searchResults.length) % searchResults.length
      setActiveSearchIndex(nextIndex)
      if (view === 'hierarchy') setFocusedHierarchyId(searchResults[nextIndex].id)
    } else if (e.key === 'Enter' && activeSearchIndex >= 0) {
      e.preventDefault()
      commitSearchResult(searchResults[activeSearchIndex])
    }
  }, [activeSearchIndex, commitSearchResult, searchResults, view])

  const handleCreate = useCallback(async () => {
    if (!newName.trim()) return
    setIsCreating(true)
    try {
      const d = await api.workspace.views.create({ name: newName.trim() })
      await refreshTree()
      navigate(`/views/${d.id}`)
      onCreateClose()
      setNewName('')
    } catch (err: unknown) {
      toast({ title: 'Failed to create diagram', description: err instanceof Error ? err.message : 'An unexpected error occurred', status: 'error' })
    } finally {
      setIsCreating(false)
    }
  }, [navigate, newName, onCreateClose, refreshTree])

  if (initializing) {
    return (
      <Center h="full">
        <Spinner size="xl" color="var(--accent)" />
      </Center>
    )
  }

  return (
    <Box position="relative" w="full" h="full" overflow="hidden">
      <DiagramJumpToolbar
        view={view}
        searchTerm={searchTerm}
        searchResults={searchResults}
        activeSearchIndex={activeSearchIndex}
        onSearchChange={handleSearchChange}
        onSearchKeyDown={handleSearchKeyDown}
        onResultClick={commitSearchResult}
        onViewChange={handleViewChange}
        onCreateOpen={() => {
          setNewName('')
          onCreateOpen()
        }}
      />

      {/* Page Content */}
      <AnimatePresence mode="wait">
        <MotionBox
          key={view}
          initial={{ opacity: 0, scale: 0.98 }}
          animate={{ opacity: 1, scale: 1 }}
          exit={{ opacity: 0, scale: 1.02 }}
          transition={{ duration: 0.3, ease: 'easeInOut' }}
          w="full"
          h="full"
        >
          {view === 'explore' ? (
            <InfiniteZoom ref={exploreRef} shareSlot={shareSlot} />
          ) : (
            <ViewsGrid
              onShare={onShareView}
              treeData={treeData}
              loading={treeLoading}
              focusedId={focusedHierarchyId}
              onFocusChange={setFocusedHierarchyId}
              setTreeData={setTreeData}
              refreshTree={refreshTree}
            />
          )}
        </MotionBox>
      </AnimatePresence>

      <Modal
        isOpen={isCreateOpen}
        onClose={onCreateClose}
        isCentered
        size="sm"
      >
        <ModalOverlay bg="blackAlpha.700" backdropFilter="blur(4px)" />
        <ModalContent
          bg="var(--bg-panel)"
          border="1px solid"
          borderColor="var(--border-main)"
          borderRadius="xl"
          boxShadow="0 24px 64px rgba(0,0,0,0.8)"
        >
          <ModalHeader color="gray.100" pb={1} fontSize="md">Create New Diagram</ModalHeader>
          <ModalBody>
            <FormControl id="new-view-name">
              <FormLabel fontSize="xs" color="gray.500" textTransform="uppercase" letterSpacing="0.05em">
                Diagram Name
              </FormLabel>
              <Input
                name="name"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                size="sm"
                bg="whiteAlpha.50"
                border="1px solid"
                borderColor="whiteAlpha.100"
                _hover={{ borderColor: 'whiteAlpha.300' }}
                _focus={{ borderColor: 'var(--accent)', boxShadow: '0 0 0 1px var(--accent)' }}
                autoFocus
                onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
                placeholder="My New Architecture"
              />
            </FormControl>
          </ModalBody>
          <ModalFooter gap={2} pt={6}>
            <Button size="sm" variant="ghost" color="gray.500" _hover={{ color: 'white', bg: 'whiteAlpha.100' }} onClick={onCreateClose}>
              Cancel
            </Button>
            <Button
              size="sm"
              bg="var(--accent)"
              color="white"
              _hover={{ bg: "var(--accent)", filter: "brightness(1.1)" }}
              _active={{ bg: "var(--accent)", filter: "brightness(0.9)" }}
              isLoading={isCreating}
              isDisabled={!newName.trim()}
              onClick={handleCreate}
              borderRadius="lg"
              px={6}
            >
              Create
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </Box>
  )
}
