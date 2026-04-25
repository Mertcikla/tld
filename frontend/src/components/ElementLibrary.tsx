import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Badge,
  Box,
  Button,
  Checkbox,
  Divider,
  Flex,
  HStack,
  IconButton,
  Input,
  InputGroup,
  InputLeftElement,
  Spinner,
  Text,
  Tooltip,
  VStack,
  useBreakpointValue,
} from '@chakra-ui/react'
import SlidingPanel from './SlidingPanel'
import PanelHeader from './PanelHeader'
import { AddIcon, CheckIcon, SearchIcon, ViewIcon } from '@chakra-ui/icons'
import '../styles/editor-panels.css'
import { KbdHint } from './PanelUI'
import { api } from '../api/client'
import type { LibraryElement } from '../types'
import { TYPE_COLORS } from '../types'
import { resolveIconPath } from '../utils/url'
import ScrollIndicatorWrapper from './ScrollIndicatorWrapper'

import { useViewEditorContext } from '../pages/ViewEditor/context'

interface Props {
  existingElementIds: Set<number>
  existingElements?: LibraryElement[]
  onCreateNew: () => void
  refresh: number
  isOpen: boolean
  onClose: () => void
  onTapAdd?: (obj: LibraryElement) => void
  onFindElement?: (id: number) => void
  onTouchDrop?: (obj: LibraryElement, clientX: number, clientY: number) => void
}

function mergeUniqueElements(existing: LibraryElement[], incoming: LibraryElement[]) {
  if (incoming.length === 0) return existing

  const merged = [...existing]
  const seenIds = new Set(existing.map((element) => element.id))

  for (const element of incoming) {
    if (seenIds.has(element.id)) continue
    seenIds.add(element.id)
    merged.push(element)
  }

  return merged
}

const DragHandle = () => (
  <Box
    as="svg"
    width="10px"
    height="16px"
    viewBox="0 0 10 16"
    fill="currentColor"
    color="whiteAlpha.300"
    flexShrink={0}
    mr={2}
  >
    <circle cx="3" cy="2" r="1" />
    <circle cx="7" cy="2" r="1" />
    <circle cx="3" cy="6" r="1" />
    <circle cx="7" cy="6" r="1" />
    <circle cx="3" cy="10" r="1" />
    <circle cx="7" cy="10" r="1" />
    <circle cx="3" cy="14" r="1" />
    <circle cx="7" cy="14" r="1" />
  </Box>
)

/**
 * Name: Element Library
 * Role: Panel/drawer that displays the organization's element library and shows which are in the current view and which are not. Includes an add new element button.
 * Location: Left side panel/drawer on desktop. Collapsed under a button on mobile.
 * Aliases: Element sidebar, Library panel.
 */
function ElementLibrary({
  existingElementIds,
  existingElements = [],
  onCreateNew,
  refresh,
  isOpen,
  onClose,
  onTapAdd,
  onFindElement,
  onTouchDrop,
}: Props) {
  const { canEdit } = useViewEditorContext()
  const [elements, setElements] = useState<LibraryElement[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [hasMore, setHasMore] = useState(true)
  const [hideExisting, setHideExisting] = useState(false)
  const isMobile = useBreakpointValue({ base: true, md: false }) ?? false

  const isFetching = useRef(false)
  const searchRef = useRef(search)
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  useEffect(() => { searchRef.current = search }, [search])

  const fetchElements = useCallback(async (offset: number, currentSearch: string, isInitial = false) => {
    if (isFetching.current) return
    isFetching.current = true
    setLoading(true)
    try {
      const limit = 20
      const newElements = await api.elements.list({ limit, offset, search: currentSearch })
      if (isInitial) {
        setElements(mergeUniqueElements([], newElements))
      } else {
        setElements((prev) => mergeUniqueElements(prev, newElements))
      }
      setHasMore(newElements.length === limit)
    } catch (err) {
      console.error('Failed to fetch elements:', err)
    } finally {
      isFetching.current = false
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (isOpen) {
      fetchElements(0, searchRef.current, true)
    }
  }, [isOpen, refresh, fetchElements])

  // Debounced search
  useEffect(() => {
    if (!isOpen) return
    const timer = setTimeout(() => {
      fetchElements(0, search, true)
    }, 300)
    return () => clearTimeout(timer)
  }, [search, isOpen, fetchElements])

  const filtered = useMemo(() => {
    let searchResults = existingElements
    if (search.trim()) {
      const query = search.toLowerCase()
      searchResults = searchResults.filter(o =>
        (o.name || '').toLowerCase().includes(query) ||
        (o.description || '').toLowerCase().includes(query) ||
        (o.technology || '').toLowerCase().includes(query) ||
        (o.kind || '').toLowerCase().includes(query)
      )
    }

    // Combine fetched elements with existing elements from the view to ensure they're always visible and sorted first.
    let result = mergeUniqueElements(searchResults, elements)
    if (hideExisting) {
      result = result.filter((o) => !existingElementIds.has(o.id))
    }

    return result.sort((a, b) => {
      const aExists = existingElementIds.has(a.id)
      const bExists = existingElementIds.has(b.id)
      if (aExists && !bExists) return -1
      if (!aExists && bExists) return 1
      return (a.name || '').localeCompare(b.name || '')
    })
  }, [existingElementIds, existingElements, hideExisting, elements, search])

  // If a fetch completes but the container still isn't scrollable, keep loading
  useEffect(() => {
    if (!hasMore || loading || !isOpen) return
    const el = scrollContainerRef.current
    if (!el) return
    if (el.scrollHeight <= el.clientHeight) {
      fetchElements(elements.length, search)
    }
  }, [elements, hasMore, loading, isOpen, search, fetchElements])

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const { scrollTop, scrollHeight, clientHeight } = e.currentTarget
    if (scrollHeight - scrollTop <= clientHeight + 50 && hasMore && !loading) {
      fetchElements(elements.length, search)
    }
  }

  const onDragStart = (e: React.DragEvent, obj: LibraryElement) => {
    if (!canEdit) return
    e.dataTransfer.setData('application/diag-element', JSON.stringify(obj))
    e.dataTransfer.effectAllowed = 'move'
  }

  const onItemPointerDown = (e: React.PointerEvent, obj: LibraryElement) => {
    if (!canEdit) return
    if (e.pointerType === 'mouse') return
    if (existingElementIds.has(obj.id)) return
    // Don't intercept taps on buttons
    if ((e.target as HTMLElement).closest('button')) return

    const startX = e.clientX
    const startY = e.clientY
    const pointerId = e.pointerId
    let dragging = false
    let decided = false
    let ghostEl: HTMLDivElement | null = null
    let captureDiv: HTMLDivElement | null = null

    const showGhost = (x: number, y: number): HTMLDivElement => {
      const el = document.createElement('div')
      const color = TYPE_COLORS[obj.kind ?? ''] ?? 'gray'
      el.style.cssText = [
        'position:fixed',
        `left:${x}px`,
        `top:${y}px`,
        'transform:translate(-50%,-120%)',
        'z-index:9999',
        'pointer-events:none',
        'background:#161e2b',
        'border:2px solid var(--accent)',
        'border-radius:8px',
        'padding:8px 10px',
        'box-shadow:0 8px 30px rgba(0,0,0,0.6)',
        'opacity:0.75',
        'max-width:180px',
        'overflow:hidden',
      ].join(';')
      el.innerHTML = `
        <div style="color:#e2e8f0;font-size:14px;font-weight:500;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">${obj.name}</div>
        <span style="font-size:10px;font-weight:600;text-transform:uppercase;color:${color === 'gray' ? '#a0aec0' : 'var(--accent)'};">${obj.kind ?? ''}</span>
      `
      document.body.appendChild(el)
      return el as HTMLDivElement
    }

    const cleanup = () => {
      document.removeEventListener('pointermove', onMove)
      document.removeEventListener('pointerup', onUp)
      document.removeEventListener('pointercancel', onCancel)
      captureDiv?.remove()
      captureDiv = null
      ghostEl?.remove()
      ghostEl = null
    }

    const onMove = (me: PointerEvent) => {
      if (me.pointerId !== pointerId) return
      const dx = me.clientX - startX
      const dy = me.clientY - startY
      if (!decided && Math.hypot(dx, dy) > 8) {
        decided = true
        if (Math.abs(dy) > Math.abs(dx)) {
          // Primarily vertical it's a scroll, bail out
          cleanup()
          return
        }
        // Primarily horizontal it's a drag
        dragging = true
        captureDiv = document.createElement('div')
        captureDiv.style.cssText = 'position:fixed;inset:0;z-index:9998;touch-action:none;'
        document.body.appendChild(captureDiv)
        try { captureDiv.setPointerCapture(pointerId) } catch { /* intentionally empty */ }
        onClose()
        ghostEl = showGhost(me.clientX, me.clientY)
      }
      if (dragging) {
        me.preventDefault()
        if (ghostEl) {
          ghostEl.style.left = `${me.clientX}px`
          ghostEl.style.top = `${me.clientY}px`
        }
      }
    }

    const onUp = (ue: PointerEvent) => {
      if (ue.pointerId !== pointerId) return
      const wasDropped = dragging
      cleanup()
      if (wasDropped) onTouchDrop?.(obj, ue.clientX, ue.clientY)
    }

    const onCancel = (ce: PointerEvent) => {
      if (ce.pointerId !== pointerId) return
      cleanup()
    }

    document.addEventListener('pointermove', onMove)
    document.addEventListener('pointerup', onUp)
    document.addEventListener('pointercancel', onCancel)
  }

  const listContent = (
    <>
      {/* New Element */}
      {canEdit && (
        <>
          <Box flexShrink={0}>
            <Box
              data-testid="element-library-new"
              as="button"
              className="panel-action-button"
              onClick={onCreateNew}
              role="group"
            >
              <Box className="panel-action-icon-container" color="var(--accent)">
                <AddIcon boxSize={4} />
              </Box>
              <VStack align="start" spacing={0} flex={1}>
                <Text fontWeight="semibold" fontSize="sm" color="white">New Element</Text>
                <Text fontSize="10px" color="gray.500">Add to the catalog</Text>
              </VStack>
              <KbdHint>C</KbdHint>
            </Box>
          </Box>
          <Divider borderColor="whiteAlpha.100" />
        </>
      )}

      {/* Search */}
      <Box className="panel-search-container">
        <InputGroup size="sm">
          <InputLeftElement pointerEvents="none">
            <SearchIcon color="gray.500" />
          </InputLeftElement>
          <Input
            data-testid="element-library-search"
            className="panel-search-input"
            placeholder="Search catalog…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </InputGroup>
        <Checkbox
          data-testid="element-library-hide-existing"
          size="sm"
          mt={2}
          ml={0.5}
          colorScheme="blue"
          isChecked={hideExisting}
          onChange={(e) => setHideExisting(e.target.checked)}
        >
          <Text fontSize="11px" color="gray.400">Hide existing</Text>
        </Checkbox>
      </Box>


      {/* List */}
      <ScrollIndicatorWrapper ref={scrollContainerRef} flex={1} px={2} pt={2} pb={3} onScroll={handleScroll}>
        <VStack align="stretch" spacing={1.5}>
          {loading && elements.length === 0 && (
            <Flex justify="center" py={10}>
              <Spinner size="sm" color="blue.500" thickness="2px" />
            </Flex>
          )}

          {!loading && filtered.length === 0 && elements.length === 0 && (
            <VStack spacing={2} py={6} px={2} textAlign="center">
              <Text color="gray.600" fontSize="sm" fontWeight="medium">No elements yet</Text>
              <Text color="gray.700" fontSize="xs" lineHeight="tall">
                Elements are reusable building blocks - services, databases, people, and more.
              </Text>
              {canEdit && (
                <Button size="xs" colorScheme="blue" variant="outline" mt={1} onClick={onCreateNew}>
                  Create your first element
                </Button>
              )}
            </VStack>
          )}
          {filtered.length === 0 && elements.length > 0 && (
            <Text color="gray.700" textAlign="center" py={6} fontSize="sm">
              No results for &ldquo;{search}&rdquo;
            </Text>
          )}
          {filtered.map((obj) => {
            const already = existingElementIds.has(obj.id)
            const color = TYPE_COLORS[obj.kind ?? ''] ?? 'gray'
            const hasLogo = !!obj.logo_url

            return (
              <Tooltip
                key={obj.id}
                label={already ? 'Already on canvas' : isMobile ? 'Drag to canvas or tap +' : 'Drag to canvas'}
                placement="right"
                openDelay={500}
              >
                <Box
                  data-testid="element-library-item"
                  data-element-id={obj.id}
                  data-element-name={obj.name}
                  draggable={canEdit && !already && !isMobile}
                  onDragStart={(e) => onDragStart(e, obj)}
                  onPointerDown={(e) => onItemPointerDown(e, obj)}
                  onClick={() => {
                    if (already && onFindElement) onFindElement(obj.id)
                  }}
                  p={2}
                  minH="54px"
                  display="flex"
                  alignItems="center"
                  bg={already ? 'rgba(var(--accent-rgb), 0.06)' : 'whiteAlpha.50'}
                  border="1px solid"
                  borderColor={already ? 'rgba(var(--accent-rgb), 0.25)' : 'whiteAlpha.100'}
                  rounded="lg"
                  cursor={!canEdit ? 'default' : already ? 'pointer' : (isMobile ? 'pointer' : 'grab')}
                  position="relative"
                  role="group"
                  transition="all 0.15s ease"
                  _hover={{
                    bg: already ? 'rgba(var(--accent-rgb), 0.1)' : 'whiteAlpha.100',
                    borderColor: already ? 'rgba(var(--accent-rgb), 0.45)' : 'whiteAlpha.300',
                    transform: already ? 'none' : 'translateY(-1px)',
                    boxShadow: already ? 'none' : '0 4px 12px rgba(0,0,0,0.4)',
                  }}
                >
                  <HStack spacing={2} align="center" w="full">
                    <HStack spacing={0} flexShrink={0}>
                      {already && onFindElement && (
                        <Tooltip label="Find on canvas" placement="top" openDelay={200}>
                          <IconButton
                            data-testid="element-library-find"
                            aria-label="Find on canvas"
                            icon={<ViewIcon />}
                            size="xs"
                            variant="ghost"
                            colorScheme="blue"
                            color="var(--accent)"
                            _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)' }}
                            onClick={(e) => {
                              e.stopPropagation()
                              onFindElement(obj.id)
                            }}
                          />
                        </Tooltip>
                      )}
                      {canEdit && !already && onTapAdd && (
                        <Tooltip label="Add to canvas" placement="top" openDelay={200}>
                          <IconButton
                            data-testid="element-library-add"
                            aria-label="Add to canvas"
                            icon={<AddIcon boxSize={2.5} />}
                            size="xs"
                            colorScheme="blue"
                            variant="ghost"
                            flexShrink={0}
                            onClick={(e) => {
                              e.stopPropagation()
                              onTapAdd(obj)
                              if (isMobile) onClose()
                            }}
                          />
                        </Tooltip>
                      )}
                    </HStack>

                    {hasLogo ? (
                      <Flex w="24px" h="24px" align="center" justify="center" flexShrink={0} bg="whiteAlpha.100" rounded="md" p={1}>
                        <Box as="img" src={resolveIconPath(obj.logo_url!)} maxW="100%" maxH="100%" objectFit="contain" />
                      </Flex>
                    ) : (
                      <Flex w="24px" h="24px" align="center" justify="center" flexShrink={0} bg={`${color}.900`} color={`${color}.300`} rounded="md" fontSize="10px" fontWeight="bold">
                        {(obj.kind || '?').charAt(0).toUpperCase()}
                      </Flex>
                    )}

                    <Box flex={1} minW={0}>
                      <Text fontSize="sm" fontWeight="medium" noOfLines={1} color={already ? 'gray.400' : 'gray.100'}>
                        {obj.name}
                      </Text>
                      <HStack spacing={1} mt={0.5}>
                        <Badge variant="subtle" colorScheme={color} fontSize="8px" px={1} rounded="sm">
                          {obj.kind}
                        </Badge>
                        {obj.technology && (
                          <Text fontSize="10px" color="gray.500" noOfLines={1}>
                            {obj.technology}
                          </Text>
                        )}
                      </HStack>
                    </Box>

                    <Flex w="24px" justify="center" align="center" ml={0} flexShrink={0}>
                      {already ? (
                        <CheckIcon color="var(--accent)" boxSize={3} transform="translateX(-4px)" />
                      ) : (
                        canEdit && !isMobile && <DragHandle />
                      )}
                    </Flex>

                  </HStack>
                </Box>
              </Tooltip>
            )
          })}

          {loading && elements.length > 0 && (
            <Flex justify="center" py={2}>
              <Spinner size="xs" color="blue.500" />
            </Flex>
          )}
        </VStack>
      </ScrollIndicatorWrapper>
    </>
  )

  return (
    <SlidingPanel data-testid="element-library-panel" isOpen={isOpen} onClose={onClose} panelKey="elementlibrary" side="left" width="300px" hasBackdrop={false} zIndex={1000}>
      <PanelHeader title="Element Library" onClose={onClose} hasCloseButton={isMobile} />

      <Box p={0} display="flex" flexDir="column" overflow="hidden" flex={1}>
        {listContent}
      </Box>
    </SlidingPanel>
  )
}

export default memo(ElementLibrary)
