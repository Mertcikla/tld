import { useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import {
  Badge,
  Box,
  Button,
  Checkbox,
  Flex,
  HStack,
  IconButton,
  Input,
  InputGroup,
  InputLeftElement,
  InputRightElement,
  Menu,
  MenuButton,
  MenuItem,
  MenuList,
  Popover,
  PopoverArrow,
  PopoverBody,
  PopoverContent,
  PopoverFooter,
  PopoverTrigger,
  Spinner,
  Tag,
  TagCloseButton,
  TagLabel,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import { ChevronDownIcon, ChevronRightIcon, ChevronUpIcon, DeleteIcon, EditIcon, SearchIcon, SmallCloseIcon, TriangleDownIcon, TriangleUpIcon } from '@chakra-ui/icons'
import { ZoomInIcon } from '../components/Icons'
import { api } from '../api/client'
import { TYPE_COLORS } from '../types'
import { resolveElementIconUrl } from '../utils/elementIcon'
import ConnectorPanel from '../components/ConnectorPanel'
import ElementPanel from '../components/ElementPanel'
import ViewPanel from '../components/ViewPanel'
import InspectDrawer from '../components/InventoryInspector'
import { ViewEditorContext } from './ViewEditor/context'
import type { Connector, LibraryElement, ViewTreeNode } from '../types'
import {
  buildInventoryRows,
  dependencyConnectorToConnector,
  filterInventoryRows,
  flattenInventoryViews,
  type InventoryFilters,
  type InventoryRow,
  type InventoryType,
} from './inventoryData'

const TYPE_OPTIONS: { value: InventoryType; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'elements', label: 'Elements' },
  { value: 'views', label: 'Views' },
  { value: 'connectors', label: 'Connectors' },
]

const QUALITY_OPTIONS = ['untagged', 'missing description', 'has child view', 'unused element', 'empty view', 'missing label']
const PAGE_SIZE_OPTIONS = [50, 100, 250]
const DEFAULT_PAGE_SIZE = 100
const TAGS_COLUMN_WIDTH = '112px'
const MAX_VISIBLE_FILTER_OPTIONS = 100
const DEFAULT_COLLAPSED_FILTER_SECTIONS = ['tags', 'kind', 'quality', 'sort']
const ACCENT_CHECKBOX_SX = {
  '.chakra-checkbox__control[data-checked], .chakra-checkbox__control[data-indeterminate]': {
    bg: 'var(--accent)',
    borderColor: 'var(--accent)',
    color: 'rgb(var(--bg-main-rgb))',
  },
}

function parseInventoryType(value: string | null): InventoryType {
  return value === 'elements' || value === 'views' || value === 'connectors' ? value : 'all'
}

function parsePositiveInt(value: string | null, fallback: number): number {
  const parsed = Number(value)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback
}

function parsePageSize(value: string | null): number {
  const parsed = parsePositiveInt(value, DEFAULT_PAGE_SIZE)
  return PAGE_SIZE_OPTIONS.includes(parsed) ? parsed : DEFAULT_PAGE_SIZE
}

function formatUpdated(value: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export default function Inventory() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const [loading, setLoading] = useState(true)
  const [elements, setElements] = useState<LibraryElement[]>([])
  const [views, setViews] = useState<ViewTreeNode[]>([])
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [countsByView, setCountsByView] = useState<Record<number, { placements: number; connectors: number }>>({})
  const [placementByViewElement, setPlacementByViewElement] = useState<Record<string, { x: number; y: number }>>({})
  const [availableTags, setAvailableTags] = useState<string[]>([])
  const [editing, setEditing] = useState<InventoryRow | null>(null)
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set())
  const [collapsedSections, setCollapsedSections] = useState<Set<string>>(() => new Set(DEFAULT_COLLAPSED_FILTER_SECTIONS))
  const [bulkLoading, setBulkLoading] = useState(false)
  const [sortKey, setSortKey] = useState<'name' | 'updatedAt' | 'usage' | 'type'>('name')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')
  const [tagSearch, setTagSearch] = useState('')
  const [tagColorMap, setTagColorMap] = useState<Record<string, { color: string }>>({})
  const [tagDeleteConfirm, setTagDeleteConfirm] = useState<{ tag: string; count: number } | null>(null)
  const [deletingTag, setDeletingTag] = useState<string | null>(null)
  const [bulkDeleteConfirmOpen, setBulkDeleteConfirmOpen] = useState(false)
  const searchInputRef = useRef<HTMLInputElement>(null)

  const selectedType = parseInventoryType(searchParams.get('type'))
  const query = searchParams.get('q') ?? ''
  const deferredQuery = useDeferredValue(query)
  const selectedObject = searchParams.get('object')
  const tagsFilter = useMemo(() => searchParams.get('tags')?.split(',').filter(Boolean) ?? [], [searchParams])
  const kindFilter = searchParams.get('kind') ?? ''
  const qualitiesFilter = useMemo(() => searchParams.get('qualities')?.split(',').filter(Boolean) ?? [], [searchParams])
  const requestedPage = parsePositiveInt(searchParams.get('page'), 1)
  const pageSize = parsePageSize(searchParams.get('pageSize'))

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const [allElements, gridData, dependencies, fetchedTagColors] = await Promise.all([
        api.elements.list({ limit: 0 }),
        api.workspace.views.gridData(),
        api.dependencies.list(),
        api.workspace.orgs.tagColors.list().catch(() => ({})),
      ])
      const flatViews = flattenInventoryViews(gridData.views)
      const nextCounts = Object.fromEntries(
        Object.entries(gridData.content).map(([viewId, content]) => [
          Number(viewId),
          { placements: content.placements.length, connectors: content.connectors.length },
        ]),
      )
      const nextPlacementLookup: Record<string, { x: number; y: number }> = {}
      Object.entries(gridData.content).forEach(([viewId, content]) => {
        content.placements.forEach((placement) => {
          nextPlacementLookup[`${viewId}:${placement.element_id}`] = {
            x: placement.position_x,
            y: placement.position_y,
          }
        })
      })
      const nextConnectors = dependencies.connectors.map(dependencyConnectorToConnector)
      const tagSet = new Set<string>(Object.keys(fetchedTagColors))
      allElements.forEach((element) => element.tags.forEach((tag) => tagSet.add(tag)))
      flatViews.forEach((view) => (view.tags ?? []).forEach((tag) => tagSet.add(tag)))
      nextConnectors.forEach((connector) => (connector.tags ?? []).forEach((tag) => tagSet.add(tag)))
      setElements(allElements)
      setViews(flatViews)
      setConnectors(nextConnectors)
      setCountsByView(nextCounts)
      setPlacementByViewElement(nextPlacementLookup)
      setAvailableTags(Array.from(tagSet).sort((a, b) => a.localeCompare(b)))
      setTagColorMap(fetchedTagColors as Record<string, { color: string }>)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  // ⌘K / Ctrl+K focuses search
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        searchInputRef.current?.focus()
        searchInputRef.current?.select()
      }
    }
    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [])

  const rows = useMemo(() => buildInventoryRows(elements, views, connectors, countsByView), [elements, views, connectors, countsByView])
  const filters = useMemo<InventoryFilters>(() => ({
    type: selectedType,
    query: deferredQuery,
    tags: tagsFilter,
    kind: kindFilter,
    qualities: qualitiesFilter,
  }), [deferredQuery, kindFilter, qualitiesFilter, selectedType, tagsFilter])
  const filteredRows = useMemo(() => {
    const base = filterInventoryRows(rows, filters)
    return [...base].sort((a, b) => {
      let cmp = 0
      if (sortKey === 'name') cmp = a.name.localeCompare(b.name)
      else if (sortKey === 'updatedAt') cmp = a.updatedAt.localeCompare(b.updatedAt)
      else if (sortKey === 'usage') cmp = a.usageLabel.localeCompare(b.usageLabel)
      else if (sortKey === 'type') cmp = a.typeLabel.localeCompare(b.typeLabel)
      return sortDir === 'asc' ? cmp : -cmp
    })
  }, [filters, rows, sortKey, sortDir])
  const selectedObjectRow = useMemo(() => {
    if (!selectedObject) return null
    return rows.find((row) => row.key === selectedObject) ?? null
  }, [rows, selectedObject])
  const totalPages = Math.max(1, Math.ceil(filteredRows.length / pageSize))
  const selectedRowIndex = selectedObjectRow ? filteredRows.findIndex((row) => row.key === selectedObjectRow.key) : -1
  const currentPage = selectedRowIndex >= 0
    ? Math.floor(selectedRowIndex / pageSize) + 1
    : Math.min(requestedPage, totalPages)
  const pageStart = filteredRows.length === 0 ? 0 : (currentPage - 1) * pageSize
  const pageEnd = Math.min(pageStart + pageSize, filteredRows.length)
  const paginatedRows = useMemo(
    () => filteredRows.slice(pageStart, pageEnd),
    [filteredRows, pageEnd, pageStart],
  )
  const selectedRow = useMemo(
    () => (selectedRowIndex >= 0 ? selectedObjectRow : null) ?? paginatedRows[0] ?? null,
    [paginatedRows, selectedObjectRow, selectedRowIndex],
  )
  const navigationUrls = useMemo(() => {
    if (!selectedRow) return { exploreUrl: '', editorUrl: '' }

    if (selectedRow.objectType === 'view') {
      return {
        exploreUrl: `/views?view=explore&focus=${selectedRow.id}`,
        editorUrl: `/views/${selectedRow.id}`,
      }
    }

    if (selectedRow.objectType === 'element') {
      const placementKeys = Object.keys(placementByViewElement)
      const elementPlacementKey = placementKeys.find((k) => k.endsWith(`:${selectedRow.id}`))
      const viewId = elementPlacementKey ? Number(elementPlacementKey.split(':')[0]) : null
      const fallbackViewId = viewId || views[0]?.id || 1

      return {
        exploreUrl: `/views?view=explore&focus=${fallbackViewId}&element=${selectedRow.id}`,
        editorUrl: `/views/${fallbackViewId}?element=${selectedRow.id}`,
      }
    }

    if (selectedRow.objectType === 'connector' && selectedRow.connector) {
      const conn = selectedRow.connector
      return {
        exploreUrl: `/views?view=explore&focus=${conn.view_id}&element=${conn.source_element_id}`,
        editorUrl: `/views/${conn.view_id}`,
      }
    }

    return { exploreUrl: '', editorUrl: '' }
  }, [selectedRow, views, placementByViewElement])
  const selectedKeysOnPage = useMemo(
    () => paginatedRows.filter((row) => selectedKeys.has(row.key)).length,
    [paginatedRows, selectedKeys],
  )

  const tagCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    rows.forEach((row) => row.tags.forEach((tag) => { counts[tag] = (counts[tag] ?? 0) + 1 }))
    return counts
  }, [rows])

  const qualityCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    QUALITY_OPTIONS.forEach((q) => { counts[q] = 0 })
    rows.forEach((row) => row.qualityFlags.forEach((q) => { counts[q] = (counts[q] ?? 0) + 1 }))
    return counts
  }, [rows])

  const kindCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    rows.forEach((row) => { counts[row.typeLabel] = (counts[row.typeLabel] ?? 0) + 1 })
    return counts
  }, [rows])

  const setParam = useCallback((key: string, value: string) => {
    const next = new URLSearchParams(searchParams)
    if (value) next.set(key, value)
    else next.delete(key)
    if (key !== 'object') next.delete('object')
    if (key !== 'page') next.delete('page')
    setSearchParams(next, { replace: true })
  }, [searchParams, setSearchParams])

  const toggleTagFilter = useCallback((tag: string) => {
    const next = new URLSearchParams(searchParams)
    const current = next.get('tags')?.split(',').filter(Boolean) ?? []
    const idx = current.indexOf(tag)
    if (idx >= 0) current.splice(idx, 1)
    else current.push(tag)
    if (current.length > 0) next.set('tags', current.join(','))
    else next.delete('tags')
    next.delete('object')
    next.delete('page')
    setSearchParams(next, { replace: true })
  }, [searchParams, setSearchParams])

  const toggleQualityFilter = useCallback((quality: string) => {
    const next = new URLSearchParams(searchParams)
    const current = next.get('qualities')?.split(',').filter(Boolean) ?? []
    const idx = current.indexOf(quality)
    if (idx >= 0) current.splice(idx, 1)
    else current.push(quality)
    if (current.length > 0) next.set('qualities', current.join(','))
    else next.delete('qualities')
    next.delete('object')
    next.delete('page')
    setSearchParams(next, { replace: true })
  }, [searchParams, setSearchParams])

  const selectRow = (row: InventoryRow) => {
    if (selectedKeys.size > 0) {
      toggleSelectKey(row.key)
      return
    }
    const next = new URLSearchParams(searchParams)
    next.set('object', row.key)
    setSearchParams(next, { replace: true })
    if (typeof window !== 'undefined' && window.matchMedia('(max-width: 1279px)').matches) {
      setEditing(row)
    }
  }

  const toggleSelectKey = (key: string) => {
    setSelectedKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const handleSelectAll = () => {
    const pageKeys = paginatedRows.map((row) => row.key)
    setSelectedKeys((prev) => {
      const next = new Set(prev)
      const allPageRowsSelected = pageKeys.length > 0 && pageKeys.every((key) => next.has(key))
      pageKeys.forEach((key) => {
        if (allPageRowsSelected) next.delete(key)
        else next.add(key)
      })
      return next
    })
  }

  const resetFilters = () => {
    setSearchParams(new URLSearchParams(selectedType === 'all' ? {} : { type: selectedType }), { replace: true })
  }

  const toggleSort = (key: typeof sortKey) => {
    if (sortKey === key) setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    else { setSortKey(key); setSortDir('asc') }
    setParam('page', '')
  }

  const setPage = (page: number) => {
    const next = new URLSearchParams(searchParams)
    if (page <= 1) next.delete('page')
    else next.set('page', String(Math.min(page, totalPages)))
    next.delete('object')
    setSearchParams(next, { replace: true })
  }

  const setPageSize = (size: number) => {
    const next = new URLSearchParams(searchParams)
    if (size === DEFAULT_PAGE_SIZE) next.delete('pageSize')
    else next.set('pageSize', String(size))
    next.delete('page')
    next.delete('object')
    setSearchParams(next, { replace: true })
  }

  const toggleSection = (section: string) => {
    setCollapsedSections((prev) => {
      const next = new Set(prev)
      if (next.has(section)) next.delete(section)
      else next.add(section)
      return next
    })
  }

  const distinctTypeLabels = useMemo(() => {
    return Array.from(new Set(rows.map((row) => row.typeLabel).filter(Boolean))).sort((a, b) => a.localeCompare(b))
  }, [rows])

  const hasActiveFilters = tagsFilter.length > 0 || qualitiesFilter.length > 0 || kindFilter

  const selectedRows = useMemo(() => rows.filter((r) => selectedKeys.has(r.key)), [rows, selectedKeys])

  useEffect(() => {
    if (selectedKeys.size === 0) return
    const rowKeys = new Set(rows.map((row) => row.key))
    setSelectedKeys((prev) => {
      const next = new Set(Array.from(prev).filter((key) => rowKeys.has(key)))
      return next.size === prev.size ? prev : next
    })
  }, [rows, selectedKeys.size])

  useEffect(() => {
    if (selectedKeys.size === 0) setBulkDeleteConfirmOpen(false)
  }, [selectedKeys.size])

  const typeCounts = useMemo(() => {
    const counts: Record<InventoryType, number> = {
      all: rows.length,
      elements: 0,
      views: 0,
      connectors: 0,
    }
    rows.forEach((row) => {
      if (row.objectType === 'element') counts.elements += 1
      else if (row.objectType === 'view') counts.views += 1
      else counts.connectors += 1
    })
    return counts
  }, [rows])

  const filteredAvailableTags = useMemo(() => {
    const normalized = tagSearch.trim().toLowerCase()
    return availableTags.filter((tag) => !normalized || tag.toLowerCase().includes(normalized))
  }, [availableTags, tagSearch])
  const visibleAvailableTags = filteredAvailableTags.slice(0, MAX_VISIBLE_FILTER_OPTIONS)

  const handleBulkAddTag = async (tag: string) => {
    setBulkLoading(true)
    try {
      await Promise.all(
        selectedRows.map(async (row) => {
          if (row.objectType === 'element' && row.element) {
            const next = Array.from(new Set([...row.element.tags, tag]))
            await api.workspace.elements.update(row.element.id, { tags: next })
          } else if (row.objectType === 'view' && row.view) {
            const next = Array.from(new Set([...(row.view.tags ?? []), tag]))
            await api.workspace.views.update(row.view.id, { name: row.view.name, tags: next })
          } else if (row.objectType === 'connector' && row.connector) {
            const next = Array.from(new Set([...(row.connector.tags ?? []), tag]))
            await api.workspace.connectors.update(row.connector.view_id, row.connector.id, { tags: next })
          }
        }),
      )
      setSelectedKeys(new Set())
      void refresh()
    } finally {
      setBulkLoading(false)
    }
  }

  const handleBulkRemoveTag = async (tag: string) => {
    setBulkLoading(true)
    try {
      await Promise.all(
        selectedRows.map(async (row) => {
          if (row.objectType === 'element' && row.element) {
            const next = row.element.tags.filter((t) => t !== tag)
            await api.workspace.elements.update(row.element.id, { tags: next })
          } else if (row.objectType === 'view' && row.view) {
            const next = (row.view.tags ?? []).filter((t) => t !== tag)
            await api.workspace.views.update(row.view.id, { name: row.view.name, tags: next })
          } else if (row.objectType === 'connector' && row.connector) {
            const next = (row.connector.tags ?? []).filter((t) => t !== tag)
            await api.workspace.connectors.update(row.connector.view_id, row.connector.id, { tags: next })
          }
        }),
      )
      setSelectedKeys(new Set())
      void refresh()
    } finally {
      setBulkLoading(false)
    }
  }

  const handleBulkDelete = async () => {
    setBulkDeleteConfirmOpen(false)
    setBulkLoading(true)
    try {
      await Promise.all(
        selectedRows.map(async (row) => {
          if (row.objectType === 'element') {
            await api.workspace.elements.delete('', row.id)
          } else if (row.objectType === 'view') {
            await api.workspace.views.delete('', row.id)
          } else if (row.objectType === 'connector') {
            await api.workspace.connectors.delete('', row.id)
          }
        }),
      )
      setSelectedKeys(new Set())
      setParam('object', '')
      void refresh()
    } finally {
      setBulkLoading(false)
    }
  }

  const editorContext = useMemo(() => ({
    viewId: editing?.connector?.view_id ?? selectedRow?.connector?.view_id ?? null,
    canEdit: true,
    isOwner: true,
    isFreePlan: false,
    snapToGrid: false,
    setSnapToGrid: () => undefined,
    selectedElement: editing?.element ?? selectedRow?.element ?? null,
    selectedConnector: editing?.connector ?? selectedRow?.connector ?? null,
  }), [editing, selectedRow])

  const tagsInSelection = useMemo(() => {
    const counts: Record<string, number> = {}
    selectedRows.forEach((row) => row.tags.forEach((tag) => { counts[tag] = (counts[tag] ?? 0) + 1 }))
    return counts
  }, [selectedRows])

  const handleDeleteTag = async (tag: string) => {
    setDeletingTag(tag)
    try {
      await api.workspace.orgs.tagColors.delete(tag)
      setTagDeleteConfirm(null)
      void refresh()
    } catch (err) {
      console.error('Failed to delete tag', err)
    } finally {
      setDeletingTag(null)
    }
  }

  return (
    <ViewEditorContext.Provider value={editorContext}>
      <Box data-testid="inventory-page" h="100%" bg="var(--bg-canvas)" display="flex" flexDir="column" overflow="hidden">
        {/* Header bar */}
        <Flex px={4} py={2.5} gap={3} align="center" justify={{ base: 'flex-start', md: 'center' }} borderBottom="1px solid" borderColor="whiteAlpha.100" flexShrink={0} position="relative">
          <InputGroup size="sm" maxW={{ base: 'none', md: '480px' }} w="full" flex={1} minW={0}>
            <InputLeftElement pointerEvents="none" color="gray.500"><SearchIcon boxSize={3.5} /></InputLeftElement>
            <Input
              ref={searchInputRef}
              data-testid="inventory-search"
              value={query}
              onChange={(event) => setParam('q', event.target.value)}
              onKeyDown={(e) => { if (e.key === 'Escape') { setParam('q', ''); (e.target as HTMLInputElement).blur() } }}
              placeholder="Search names, tags, kinds…"
              variant="elevated"
              pl={8}
              pr={query ? '2.5rem' : '5.5rem'}
              _placeholder={{ color: 'gray.600' }}
            />
            {query ? (
              <InputRightElement>
                <IconButton
                  aria-label="Clear search"
                  icon={<SmallCloseIcon />}
                  size="xs"
                  variant="ghost"
                  color="gray.500"
                  _hover={{ color: 'gray.200' }}
                  onClick={() => { setParam('q', ''); searchInputRef.current?.focus() }}
                />
              </InputRightElement>
            ) : (
              <InputRightElement w="auto" pr={2} pointerEvents="none">
                <Text fontSize="10px" color="gray.600" fontFamily="mono" px={1} border="1px solid" borderColor="whiteAlpha.200" borderRadius="md">⌘K</Text>
              </InputRightElement>
            )}
          </InputGroup>
          <Flex align="center" gap={2} position={{ base: 'static', md: 'absolute' }} right={{ md: 4 }} flexShrink={0}>
            <Box fontSize="xs" color="gray.500" whiteSpace="nowrap">
              {loading ? <Spinner size="xs" /> : <><Box as="span" color="gray.300" fontWeight="medium">{filteredRows.length}</Box> / {rows.length}</>}
            </Box>
          </Flex>
        </Flex>

        {/* Active filter chips */}
        {hasActiveFilters && (
          <Flex px={4} py={2} gap={2} flexWrap="wrap" borderBottom="1px solid" borderColor="whiteAlpha.100" flexShrink={0} align="center">
            {tagsFilter.map((tag) => (
              <Tag key={tag} size="sm" colorScheme="blue" variant="subtle" borderRadius="full">
                <TagLabel>tag: {tag}</TagLabel>
                <TagCloseButton onClick={() => toggleTagFilter(tag)} />
              </Tag>
            ))}
            {qualitiesFilter.map((q) => (
              <Tag key={q} size="sm" colorScheme="orange" variant="subtle" borderRadius="full">
                <TagLabel>{q}</TagLabel>
                <TagCloseButton onClick={() => toggleQualityFilter(q)} />
              </Tag>
            ))}
            {kindFilter && (
              <Tag size="sm" colorScheme="purple" variant="subtle" borderRadius="full">
                <TagLabel>kind: {kindFilter}</TagLabel>
                <TagCloseButton onClick={() => setParam('kind', '')} />
              </Tag>
            )}
            <Button size="xs" variant="ghost" color="gray.500" onClick={resetFilters} ml="auto">Clear all</Button>
          </Flex>
        )}

        <Flex flex={1} minH={0} overflow="hidden">
          {/* Left filter panel */}
          <Box w={{ base: '0', lg: '220px' }} display={{ base: 'none', lg: 'flex' }} flexDir="column" borderRight="1px solid" borderColor="whiteAlpha.100" overflowY="auto" flexShrink={0}>
            {/* Type selector */}
            <Box px={3} pt={3} pb={2} borderBottom="1px solid" borderColor="whiteAlpha.100">
              <Text fontSize="10px" color="gray.600" fontWeight="bold" textTransform="uppercase" mb={2}>Object type</Text>
              <VStack align="stretch" spacing={0.5}>
                {TYPE_OPTIONS.map((option) => {
                  const isActive = selectedType === option.value
                  const count = typeCounts[option.value]
                  return (
                    <Flex
                      key={option.value}
                      data-testid={`inventory-tab-${option.value}`}
                      align="center"
                      px={2.5}
                      py={1.5}
                      borderRadius="md"
                      cursor="pointer"
                      bg={isActive ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
                      _hover={{ bg: isActive ? 'rgba(var(--accent-rgb), 0.16)' : 'whiteAlpha.50' }}
                      onClick={() => setParam('type', option.value === 'all' ? '' : option.value)}
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
            </Box>

            <FilterSection title="Tags" collapsed={collapsedSections.has('tags')} onToggle={() => toggleSection('tags')}>
              {availableTags.length > 5 && (
                <InputGroup size="xs" mb={2}>
                  <InputLeftElement pointerEvents="none" color="gray.600"><SearchIcon boxSize={3} /></InputLeftElement>
                  <Input
                    value={tagSearch}
                    onChange={(e) => setTagSearch(e.target.value)}
                    placeholder="Filter tags…"
                    variant="filled"
                    pl={7}
                    bg="whiteAlpha.50"
                    _focus={{ bg: 'whiteAlpha.100' }}
                    borderRadius="md"
                  />
                  {tagSearch && (
                    <InputRightElement>
                      <IconButton aria-label="Clear tag search" icon={<SmallCloseIcon />} size="xs" variant="ghost" color="gray.600" onClick={() => setTagSearch('')} />
                    </InputRightElement>
                  )}
                </InputGroup>
              )}
              <VStack align="stretch" spacing={0.5}>
                {visibleAvailableTags
                  .map((tag) => {
                    const isActive = tagsFilter.includes(tag)
                    const count = tagCounts[tag] ?? 0
                    return (
                      <Box key={tag}>
                        <Flex
                          data-testid={`inventory-tag-filter-${tag}`}
                          role="group"
                          align="center"
                          px={2}
                          py={1}
                          borderRadius="md"
                          cursor="pointer"
                          bg={isActive ? 'rgba(66, 153, 225, 0.12)' : 'transparent'}
                          _hover={{ bg: isActive ? 'rgba(66, 153, 225, 0.18)' : 'whiteAlpha.50' }}
                          onClick={() => toggleTagFilter(tag)}
                          userSelect="none"
                        >
                          <Box
                            w="6px"
                            h="6px"
                            borderRadius="full"
                            bg={isActive ? 'blue.400' : 'gray.600'}
                            mr={2}
                            flexShrink={0}
                            transition="background 0.1s"
                          />
                          <Text fontSize="xs" color={isActive ? 'blue.300' : 'gray.400'} flex={1} isTruncated>{tag}</Text>
                          <Text fontSize="10px" color={count === 0 ? 'gray.700' : isActive ? 'blue.400' : 'gray.600'} fontWeight="bold" ml={1} _groupHover={{ display: 'none' }}>{count}</Text>
                          <Popover
                            placement="right-start"
                            isOpen={tagDeleteConfirm?.tag === tag}
                            onClose={() => setTagDeleteConfirm(null)}
                            closeOnBlur
                          >
                            <Tooltip label={`Delete tag "${tag}"`} placement="right" openDelay={400}>
                              <Box>
                                <PopoverTrigger>
                                  <IconButton
                                    aria-label={`Delete tag ${tag}`}
                                    icon={<DeleteIcon boxSize="9px" />}
                                    size="xs"
                                    variant="ghost"
                                    color="red.400"
                                    display="none"
                                    _groupHover={{ display: 'flex' }}
                                    onClick={(e) => {
                                      e.stopPropagation()
                                      setTagDeleteConfirm((prev) => (prev?.tag === tag ? null : { tag, count }))
                                    }}
                                    h="16px"
                                    minW="16px"
                                    ml={1}
                                    isDisabled={deletingTag !== null}
                                  />
                                </PopoverTrigger>
                              </Box>
                            </Tooltip>
                            <PopoverContent
                              onClick={(e) => e.stopPropagation()}
                              bg="rgb(var(--bg-main-rgb))"
                              borderColor="rgba(245, 101, 101, 0.45)"
                              boxShadow="0 14px 36px rgba(0, 0, 0, 0.45)"
                              maxW="280px"
                              data-testid={`inventory-tag-delete-confirm-${tag}`}
                            >
                              <PopoverArrow bg="rgb(var(--bg-main-rgb))" />
                              <PopoverBody pt={3} pb={2}>
                                <Text fontSize="sm" color="gray.100" lineHeight={1.35}>
                                  {count > 0 ? `Delete "${tag}" and remove it from ${count} item(s)?` : `Delete "${tag}"?`}
                                </Text>
                              </PopoverBody>
                              <PopoverFooter border="0" pt={0} pb={3}>
                                <HStack justify="flex-end" spacing={2}>
                                  <Button
                                    size="xs"
                                    variant="ghost"
                                    colorScheme="gray"
                                    onClick={() => setTagDeleteConfirm(null)}
                                    isDisabled={deletingTag === tag}
                                  >
                                    Cancel
                                  </Button>
                                  <Button
                                    size="xs"
                                    colorScheme="red"
                                    onClick={() => { void handleDeleteTag(tag) }}
                                    isLoading={deletingTag === tag}
                                    loadingText="Deleting"
                                  >
                                    Delete
                                  </Button>
                                </HStack>
                              </PopoverFooter>
                            </PopoverContent>
                          </Popover>
                        </Flex>
                      </Box>
                    )
                  })}
                {availableTags.length === 0 && (
                  <Text fontSize="xs" color="gray.600" px={2}>No tags yet</Text>
                )}
                {filteredAvailableTags.length > visibleAvailableTags.length && (
                  <Text fontSize="xs" color="gray.600" px={2}>Showing first {visibleAvailableTags.length} matches</Text>
                )}
                {tagSearch && filteredAvailableTags.length === 0 && (
                  <Text fontSize="xs" color="gray.600" px={2}>No matching tags</Text>
                )}
              </VStack>
            </FilterSection>

            <FilterSection title="Kind" collapsed={collapsedSections.has('kind')} onToggle={() => toggleSection('kind')}>
              <VStack align="stretch" spacing={0.5}>
                <Flex
                  data-testid="inventory-kind-filter-any"
                  align="center"
                  px={2}
                  py={1}
                  borderRadius="md"
                  cursor="pointer"
                  bg={!kindFilter ? 'rgba(var(--accent-rgb), 0.1)' : 'transparent'}
                  _hover={{ bg: !kindFilter ? 'rgba(var(--accent-rgb), 0.14)' : 'whiteAlpha.50' }}
                  onClick={() => setParam('kind', '')}
                  userSelect="none"
                >
                  <Text fontSize="xs" color={!kindFilter ? 'var(--accent)' : 'gray.400'} flex={1}>Any kind</Text>
                  <Text fontSize="10px" color={!kindFilter ? 'var(--accent)' : 'gray.600'} fontWeight="bold">{rows.length}</Text>
                </Flex>
                {distinctTypeLabels.map((label) => {
                  const isActive = kindFilter === label
                  const count = kindCounts[label] ?? 0
                  return (
                    <Flex
                      key={label}
                      data-testid={`inventory-kind-filter-${label}`}
                      align="center"
                      px={2}
                      py={1}
                      borderRadius="md"
                      cursor="pointer"
                      bg={isActive ? 'rgba(159, 122, 234, 0.12)' : 'transparent'}
                      _hover={{ bg: isActive ? 'rgba(159, 122, 234, 0.18)' : 'whiteAlpha.50' }}
                      onClick={() => setParam('kind', label)}
                      userSelect="none"
                    >
                      <Text fontSize="xs" color={isActive ? 'purple.300' : 'gray.400'} flex={1} isTruncated>{label}</Text>
                      <Text fontSize="10px" color={count === 0 ? 'gray.700' : isActive ? 'purple.400' : 'gray.600'} fontWeight="bold" ml={1}>{count}</Text>
                    </Flex>
                  )
                })}
              </VStack>
            </FilterSection>

            <FilterSection title="Quality" collapsed={collapsedSections.has('quality')} onToggle={() => toggleSection('quality')}>
              <VStack align="stretch" spacing={0}>
                {QUALITY_OPTIONS.map((quality) => {
                  const isActive = qualitiesFilter.includes(quality)
                  const count = qualityCounts[quality] ?? 0
                  return (
                    <Flex
                      key={quality}
                      data-testid={`inventory-quality-filter-${quality}`}
                      align="center"
                      px={2}
                      py={1.5}
                      borderRadius="md"
                      cursor="pointer"
                      bg={isActive ? 'rgba(237, 137, 54, 0.15)' : 'transparent'}
                      _hover={{ bg: isActive ? 'rgba(237, 137, 54, 0.2)' : 'whiteAlpha.50' }}
                      onClick={() => toggleQualityFilter(quality)}
                    >
                      <Checkbox
                        isChecked={isActive}
                        colorScheme="orange"
                        size="sm"
                        onChange={() => toggleQualityFilter(quality)}
                        onClick={(e) => e.stopPropagation()}
                        mr={2}
                        flexShrink={0}
                      />
                      <Text fontSize="xs" color={isActive ? 'orange.300' : 'gray.400'} flex={1}>{quality}</Text>
                      <Text fontSize="10px" color={count === 0 ? 'gray.700' : isActive ? 'orange.400' : 'gray.600'} fontWeight="bold">{count}</Text>
                    </Flex>
                  )
                })}
              </VStack>
            </FilterSection>

            {/* Sort section */}
            <FilterSection title="Sort" collapsed={collapsedSections.has('sort')} onToggle={() => toggleSection('sort')}>
              <VStack align="stretch" spacing={0.5}>
                {([
                  { key: 'name', label: 'Name' },
                  { key: 'type', label: 'Type' },
                  { key: 'updatedAt', label: 'Last updated' },
                  { key: 'usage', label: 'Usage' },
                ] as const).map(({ key, label }) => {
                  const isActive = sortKey === key
                  return (
                    <Flex
                      key={key}
                      align="center"
                      px={2}
                      py={1}
                      borderRadius="md"
                      cursor="pointer"
                      bg={isActive ? 'whiteAlpha.100' : 'transparent'}
                      _hover={{ bg: 'whiteAlpha.50' }}
                      onClick={() => toggleSort(key)}
                      userSelect="none"
                    >
                      <Text fontSize="xs" color={isActive ? 'gray.200' : 'gray.500'} flex={1}>{label}</Text>
                      {isActive && (
                        <Box color="gray.400">
                          {sortDir === 'asc' ? <TriangleUpIcon boxSize={2.5} /> : <TriangleDownIcon boxSize={2.5} />}
                        </Box>
                      )}
                    </Flex>
                  )
                })}
              </VStack>
            </FilterSection>
          </Box>

          <Flex flex={1} minW={0} direction="column" overflow="hidden">
            <Box flex={1} minH={0} overflowX="auto" overflowY="hidden">
              {loading ? (
                <Flex h="100%" align="center" justify="center" direction="column" gap={3} color="gray.600">
                  <Spinner size="lg" color="var(--accent)" />
                  <Text fontSize="sm">Loading inventory…</Text>
                </Flex>
              ) : (
                <Flex minW="720px" h="100%" minH={0} direction="column">
                  {/* Sortable header */}
                  <Flex
                    h={selectedKeys.size > 0 ? '40px' : '40px'}
                    px={4}
                    align="center"
                    borderBottom="1px solid"
                    borderColor={selectedKeys.size > 0 ? 'rgba(var(--accent-rgb), 0.28)' : 'whiteAlpha.100'}
                    bg="rgb(var(--bg-main-rgb))"
                    color="gray.500"
                    fontSize="10px"
                    fontWeight="bold"
                    textTransform={selectedKeys.size > 0 ? 'none' : 'uppercase'}
                    flexShrink={0}
                    boxShadow={selectedKeys.size > 0 ? 'inset 0 -1px 0 rgba(var(--accent-rgb), 0.12)' : undefined}
                  >
                    <Flex w="32px" flexShrink={0} justify="center" align="center">
                      <Checkbox
                        size="sm"
                        isIndeterminate={selectedKeysOnPage > 0 && selectedKeysOnPage < paginatedRows.length}
                        isChecked={paginatedRows.length > 0 && selectedKeysOnPage === paginatedRows.length}
                        onChange={handleSelectAll}
                        colorScheme="blue"
                        opacity={selectedKeys.size > 0 ? 1 : 0.4}
                        _hover={{ opacity: 1 }}
                        sx={ACCENT_CHECKBOX_SX}
                      />
                    </Flex>
                    {selectedKeys.size > 0 ? (
                      <>
                        <Text fontSize="sm" fontWeight="semibold" color="var(--accent)" minW="96px">
                          {selectedKeys.size} selected
                        </Text>
                        <HStack spacing={1.5} flex={1} minW={0}>
                          <Menu>
                            <MenuButton
                              data-testid="inventory-bulk-add-tag"
                              as={Button}
                              size="xs"
                              variant="outline"
                              color="var(--accent)"
                              borderColor="rgba(var(--accent-rgb), 0.55)"
                              bg="rgba(var(--accent-rgb), 0.06)"
                              _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)' }}
                              rightIcon={<ChevronDownIcon />}
                              isLoading={bulkLoading}
                            >
                              Add tag
                            </MenuButton>
                            <MenuList maxH="240px" overflowY="auto">
                              {availableTags.map((tag) => (
                                <MenuItem data-testid={`inventory-bulk-add-tag-${tag}`} key={tag} onClick={() => void handleBulkAddTag(tag)}>{tag}</MenuItem>
                              ))}
                              {availableTags.length === 0 && (
                                <MenuItem isDisabled>No tags available</MenuItem>
                              )}
                            </MenuList>
                          </Menu>
                          {Object.keys(tagsInSelection).length > 0 && (
                            <Menu>
                              <MenuButton
                                data-testid="inventory-bulk-remove-tag"
                                as={Button}
                                size="xs"
                                variant="ghost"
                                color="gray.300"
                                _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                                rightIcon={<ChevronDownIcon />}
                                isLoading={bulkLoading}
                              >
                                Remove tag
                              </MenuButton>
                              <MenuList maxH="240px" overflowY="auto">
                                {Object.keys(tagsInSelection).map((tag) => (
                                  <MenuItem data-testid={`inventory-bulk-remove-tag-${tag}`} key={tag} onClick={() => void handleBulkRemoveTag(tag)}>
                                    <Flex justify="space-between" w="full">
                                      <Text>{tag}</Text>
                                      <Badge colorScheme="gray" ml={2}>{tagsInSelection[tag]}</Badge>
                                    </Flex>
                                  </MenuItem>
                                ))}
                              </MenuList>
                            </Menu>
                          )}
                        </HStack>
                        <Popover
                          placement="left-start"
                          isOpen={bulkDeleteConfirmOpen}
                          onClose={() => setBulkDeleteConfirmOpen(false)}
                          closeOnBlur
                        >
                          <Tooltip label="Delete selected">
                            <Box>
                              <PopoverTrigger>
                                <IconButton
                                  data-testid="inventory-bulk-delete"
                                  aria-label="Delete selected"
                                  icon={<DeleteIcon />}
                                  size="xs"
                                  colorScheme="red"
                                  variant="ghost"
                                  isLoading={bulkLoading}
                                  onClick={() => setBulkDeleteConfirmOpen((prev) => !prev)}
                                />
                              </PopoverTrigger>
                            </Box>
                          </Tooltip>
                          <PopoverContent
                            bg="rgb(var(--bg-main-rgb))"
                            borderColor="rgba(245, 101, 101, 0.45)"
                            boxShadow="0 14px 36px rgba(0, 0, 0, 0.45)"
                            maxW="290px"
                            data-testid="inventory-bulk-delete-confirm"
                          >
                            <PopoverArrow bg="rgb(var(--bg-main-rgb))" />
                            <PopoverBody pt={3} pb={2}>
                              <Text fontSize="sm" color="gray.100" lineHeight={1.35}>
                                {`Delete ${selectedKeys.size} selected item(s)? This cannot be undone.`}
                              </Text>
                            </PopoverBody>
                            <PopoverFooter border="0" pt={0} pb={3}>
                              <HStack justify="flex-end" spacing={2}>
                                <Button
                                  size="xs"
                                  variant="ghost"
                                  colorScheme="gray"
                                  onClick={() => setBulkDeleteConfirmOpen(false)}
                                  isDisabled={bulkLoading}
                                >
                                  Cancel
                                </Button>
                                <Button
                                  data-testid="inventory-bulk-delete-submit"
                                  size="xs"
                                  colorScheme="red"
                                  onClick={() => { void handleBulkDelete() }}
                                  isLoading={bulkLoading}
                                  loadingText="Deleting"
                                >
                                  Delete
                                </Button>
                              </HStack>
                            </PopoverFooter>
                          </PopoverContent>
                        </Popover>
                        <IconButton
                          aria-label="Clear selection"
                          icon={<SmallCloseIcon />}
                          size="xs"
                          variant="ghost"
                          color="gray.400"
                          onClick={() => setSelectedKeys(new Set())}
                        />
                      </>
                    ) : (
                      <>
                        <SortableHeader label="Name" sortKey="name" activeSortKey={sortKey} sortDir={sortDir} onSort={toggleSort} flex={1} />
                        <Box w={TAGS_COLUMN_WIDTH}><SortableHeader label="Tags" sortKey={null} activeSortKey={sortKey} sortDir={sortDir} onSort={toggleSort} /></Box>
                        <Box w="160px"><SortableHeader label="Usage" sortKey="usage" activeSortKey={sortKey} sortDir={sortDir} onSort={toggleSort} /></Box>
                        <Box w="90px"><SortableHeader label="Updated" sortKey="updatedAt" activeSortKey={sortKey} sortDir={sortDir} onSort={toggleSort} /></Box>
                      </>
                    )}
                  </Flex>
                  <Box flex={1} minH={0} overflowY="auto">
                    {paginatedRows.map((row) => {
                      const isSelected = selectedKeys.has(row.key)
                      const isHighlighted = selectedRow?.key === row.key && selectedKeys.size === 0
                      return (
                        <Flex
                          data-testid="inventory-row"
                          data-inventory-key={row.key}
                          key={row.key}
                          px={4}
                          py={2.5}
                          align="center"
                          borderBottom="1px solid"
                          borderColor="whiteAlpha.50"
                          bg={isSelected ? 'rgba(var(--accent-rgb), 0.08)' : isHighlighted ? 'rgba(var(--accent-rgb), 0.08)' : 'transparent'}
                          cursor="pointer"
                          role="group"
                          transition="background 0.1s"
                          _hover={{ bg: isSelected ? 'rgba(var(--accent-rgb), 0.12)' : isHighlighted ? 'rgba(var(--accent-rgb), 0.12)' : 'whiteAlpha.50' }}
                          onClick={() => selectRow(row)}
                        >
                          <Flex
                            w="32px"
                            flexShrink={0}
                            justify="center"
                            align="center"
                            opacity={selectedKeys.size > 0 || isSelected ? 1 : 0}
                            _groupHover={{ opacity: 1 }}
                            transition="opacity 0.1s"
                            onClick={(e) => { e.stopPropagation(); toggleSelectKey(row.key) }}
                          >
                            <Checkbox
                              data-testid={`inventory-row-select-${row.key}`}
                              isChecked={isSelected}
                              colorScheme="blue"
                              size="sm"
                              onChange={() => toggleSelectKey(row.key)}
                              onClick={(e) => e.stopPropagation()}
                              sx={ACCENT_CHECKBOX_SX}
                            />
                          </Flex>
                          <HStack flex={1} minW={0} spacing={2.5}>
                            <InventoryRowIcon row={row} />
                            <Box minW={0}>
                              <HStack spacing={2} mb={0.5}>
                                <Text fontSize="sm" fontWeight="semibold" noOfLines={1} color={isHighlighted ? 'white' : 'gray.100'}>{row.name}</Text>
                              </HStack>
                              <Text fontSize="11px" color="gray.500" noOfLines={1}>{row.subtitle}</Text>
                            </Box>
                          </HStack>
                          <Box w={TAGS_COLUMN_WIDTH} minW={TAGS_COLUMN_WIDTH} flexShrink={0}>
                            <InventoryTagList
                              tags={row.tags}
                              tagColorMap={tagColorMap}
                              activeTags={tagsFilter}
                              onTagClick={toggleTagFilter}
                            />
                          </Box>
                          <Text w="160px" fontSize="xs" color="gray.400" noOfLines={1}>{row.usageLabel || '—'}</Text>
                          <Text w="90px" fontSize="xs" color="gray.500">{formatUpdated(row.updatedAt)}</Text>
                        </Flex>
                      )
                    })}
                    {filteredRows.length === 0 && (
                      <Flex h="220px" align="center" justify="center" color="gray.600" direction="column" gap={3}>
                        <Text fontSize="sm" color="gray.500">{query || hasActiveFilters ? 'No matching objects' : 'Nothing here yet'}</Text>
                        <Text fontSize="xs" color="gray.700">{query ? `No results for "${query}"` : hasActiveFilters ? 'Try adjusting your filters' : 'Add elements and views to get started'}</Text>
                        {hasActiveFilters && (
                          <Button size="xs" variant="outline" colorScheme="gray" onClick={resetFilters}>Clear filters</Button>
                        )}
                      </Flex>
                    )}
                  </Box>
                  {filteredRows.length > 0 && (
                    <Flex
                      h="44px"
                      px={4}
                      align="center"
                      gap={2}
                      borderTop="1px solid"
                      borderColor="whiteAlpha.100"
                      bg="rgb(var(--bg-main-rgb))"
                      color="gray.500"
                      flexShrink={0}
                    >
                      <Text fontSize="xs" flex={1}>
                        Showing <Box as="span" color="gray.300">{pageStart + 1}-{pageEnd}</Box> of {filteredRows.length}
                      </Text>
                      <Menu>
                        <MenuButton
                          as={Button}
                          size="xs"
                          variant="ghost"
                          color="gray.400"
                          rightIcon={<ChevronDownIcon />}
                        >
                          {pageSize} / page
                        </MenuButton>
                        <MenuList minW="120px">
                          {PAGE_SIZE_OPTIONS.map((size) => (
                            <MenuItem key={size} onClick={() => setPageSize(size)}>
                              {size} rows
                            </MenuItem>
                          ))}
                        </MenuList>
                      </Menu>
                      <Button size="xs" variant="ghost" color="gray.400" onClick={() => setPage(currentPage - 1)} isDisabled={currentPage <= 1}>
                        Previous
                      </Button>
                      <Text fontSize="xs" color="gray.500" minW="70px" textAlign="center">
                        {currentPage} / {totalPages}
                      </Text>
                      <Button size="xs" variant="ghost" color="gray.400" onClick={() => setPage(currentPage + 1)} isDisabled={currentPage >= totalPages}>
                        Next
                      </Button>
                    </Flex>
                  )}
                </Flex>
              )}
            </Box>
            <InspectDrawer
              selectedRow={selectedRow}
              elements={elements}
              views={views}
              connectors={connectors}
              placementByViewElement={placementByViewElement}
              onSelectRow={(key) => {
                const found = rows.find((r) => r.key === key)
                if (found) selectRow(found)
              }}
            />
          </Flex>

          <Box w={{ base: '0', xl: '340px' }} display={{ base: 'none', xl: 'flex' }} flexDir="column" borderLeft="1px solid" borderColor="whiteAlpha.100" overflow="hidden" position="relative">
            <Box flex={1} minH={0} overflow="auto" position="relative">
              {selectedRow?.objectType === 'view' && (
                <ViewPanel
                  isOpen={true}
                  onClose={() => { }}
                  view={selectedRow.view ?? null}
                  canEdit
                  onSave={() => refresh()}
                  availableTags={availableTags}
                  hasBackdrop={false}
                  isInline={true}
                  actions={
                    <HStack spacing={0.5}>
                      <Tooltip label="Explore" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Explore"
                          data-testid="inventory-action-explore"
                          icon={<ZoomInIcon size={15} strokeWidth={2.5} />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'rgba(var(--accent-rgb), 0.15)' }}
                          onClick={() => navigate(navigationUrls.exploreUrl)}
                        />
                      </Tooltip>
                      <Tooltip label="Open in Editor" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Open in Editor"
                          data-testid="inventory-action-editor"
                          icon={<EditIcon boxSize="14px" />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                          onClick={() => navigate(navigationUrls.editorUrl)}
                        />
                      </Tooltip>
                    </HStack>
                  }
                />
              )}
              {selectedRow?.objectType === 'element' && (
                <ElementPanel
                  isOpen={true}
                  onClose={() => { }}
                  element={selectedRow.element ?? null}
                  onSave={() => refresh()}
                  onDelete={() => { setParam('object', ''); void refresh() }}
                  onPermanentDelete={() => { setParam('object', ''); void refresh() }}
                  availableTags={availableTags}
                  orgId=""
                  hasBackdrop={false}
                  isInline={true}
                  autoSave
                  actions={
                    <HStack spacing={0.5}>
                      <Tooltip label="Explore" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Explore"
                          data-testid="inventory-action-explore"
                          icon={<ZoomInIcon size={15} strokeWidth={2.5} />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'rgba(var(--accent-rgb), 0.15)' }}
                          onClick={() => navigate(navigationUrls.exploreUrl)}
                        />
                      </Tooltip>
                      <Tooltip label="Open in Editor" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Open in Editor"
                          data-testid="inventory-action-editor"
                          icon={<EditIcon boxSize="14px" />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                          onClick={() => navigate(navigationUrls.editorUrl)}
                        />
                      </Tooltip>
                    </HStack>
                  }
                />
              )}
              {selectedRow?.objectType === 'connector' && (
                <ConnectorPanel
                  isOpen={true}
                  onClose={() => { }}
                  connector={selectedRow.connector ?? null}
                  orgId=""
                  onSave={() => refresh()}
                  onDelete={() => { setParam('object', ''); void refresh() }}
                  availableTags={availableTags}
                  hasBackdrop={false}
                  isInline={true}
                  autoSave
                  actions={
                    <HStack spacing={0.5}>
                      <Tooltip label="Explore" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Explore"
                          data-testid="inventory-action-explore"
                          icon={<ZoomInIcon size={15} strokeWidth={2.5} />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'rgba(var(--accent-rgb), 0.15)' }}
                          onClick={() => navigate(navigationUrls.exploreUrl)}
                        />
                      </Tooltip>
                      <Tooltip label="Open in Editor" placement="bottom" hasArrow openDelay={400}>
                        <IconButton
                          aria-label="Open in Editor"
                          data-testid="inventory-action-editor"
                          icon={<EditIcon boxSize="14px" />}
                          size="sm"
                          variant="ghost"
                          color="var(--accent)"
                          _hover={{ bg: 'whiteAlpha.100', color: 'white' }}
                          onClick={() => navigate(navigationUrls.editorUrl)}
                        />
                      </Tooltip>
                    </HStack>
                  }
                />
              )}
              {!selectedRow && (
                <Flex h="100%" align="center" justify="center" color="gray.600" fontSize="sm">
                  No object selected.
                </Flex>
              )}
            </Box>
          </Box>
        </Flex>

        <ElementPanel
          isOpen={editing?.objectType === 'element'}
          onClose={() => setEditing(null)}
          element={editing?.element ?? null}
          onSave={() => { setEditing(null); void refresh() }}
          onDelete={() => { setEditing(null); void refresh() }}
          availableTags={availableTags}
          orgId=""
          hasBackdrop={false}
        />
        <ViewPanel
          isOpen={editing?.objectType === 'view'}
          onClose={() => setEditing(null)}
          view={editing?.view ?? null}
          canEdit
          onSave={() => { setEditing(null); void refresh() }}
          availableTags={availableTags}
          hasBackdrop={false}
        />
        <ConnectorPanel
          isOpen={editing?.objectType === 'connector'}
          onClose={() => setEditing(null)}
          connector={editing?.connector ?? null}
          orgId=""
          onSave={() => { setEditing(null); void refresh() }}
          onDelete={() => { setEditing(null); void refresh() }}
          availableTags={availableTags}
          hasBackdrop={false}
        />
      </Box>
    </ViewEditorContext.Provider>
  )
}

function InventoryRowIcon({ row }: { row: InventoryRow }) {
  // Elements: mirror ElementLibrary — logo img if available, else kind-letter with TYPE_COLORS
  if (row.objectType === 'element' && row.element) {
    const color = TYPE_COLORS[row.element.kind?.toLowerCase() ?? ''] ?? 'gray'
    const logoUrl = resolveElementIconUrl(row.element.logo_url, row.element.technology_connectors)
    if (logoUrl) {
      return (
        <Flex w="26px" h="26px" align="center" justify="center" flexShrink={0} bg="whiteAlpha.100" rounded="md" p="3px">
          <Box as="img" src={logoUrl} maxW="100%" maxH="100%" objectFit="contain" />
        </Flex>
      )
    }
    return (
      <Flex
        w="26px"
        h="26px"
        align="center"
        justify="center"
        flexShrink={0}
        bg={`${color}.900`}
        color={`${color}.300`}
        rounded="md"
        fontSize="11px"
        fontWeight="bold"
        userSelect="none"
      >
        {(row.element.kind || '?').charAt(0).toUpperCase()}
      </Flex>
    )
  }

  // Views: a small 3×2 grid of squares
  if (row.objectType === 'view') {
    return (
      <Flex w="26px" h="26px" align="center" justify="center" flexShrink={0} bg="purple.900" color="purple.300" rounded="md">
        <Box as="svg" width="14px" height="14px" viewBox="0 0 14 14" fill="currentColor">
          <rect x="1" y="1" width="5" height="4" rx="0.8" />
          <rect x="8" y="1" width="5" height="4" rx="0.8" />
          <rect x="1" y="7" width="5" height="4" rx="0.8" />
          <rect x="8" y="7" width="5" height="4" rx="0.8" />
        </Box>
      </Flex>
    )
  }

  // Connectors: a small arrow from left to right with a node at each end
  return (
    <Flex w="26px" h="26px" align="center" justify="center" flexShrink={0} bg="orange.900" color="orange.300" rounded="md">
      <Box as="svg" width="14px" height="14px" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="2.5" cy="7" r="1.5" fill="currentColor" stroke="none" />
        <line x1="4" y1="7" x2="9" y2="7" />
        <polyline points="7.5,5 10,7 7.5,9" fill="none" />
      </Box>
    </Flex>
  )
}

// ─── Tag palette & helpers ────────────────────────────────────────────────────

const TAG_PALETTE = [
  '#68D391', '#63B3ED', '#FC8181', '#F6AD55', '#B794F4',
  '#76E4F7', '#F687B3', '#ECC94B', '#4FD1C5', '#667EEA',
]

function resolveTagColor(tag: string, tagColorMap: Record<string, { color: string }>): string {
  if (tagColorMap[tag]?.color) return tagColorMap[tag].color
  let h = 0
  for (let i = 0; i < tag.length; i++) h = (h * 31 + tag.charCodeAt(i)) >>> 0
  return TAG_PALETTE[h % TAG_PALETTE.length]
}

function TagPill({
  tag,
  tagColorMap,
  isActive,
  maxTagW,
  onTagClick,
}: {
  tag: string
  tagColorMap: Record<string, { color: string }>
  isActive?: boolean
  maxTagW?: string
  onTagClick: (tag: string) => void
}) {
  const color = resolveTagColor(tag, tagColorMap)
  return (
    <Tag
      size="sm"
      bg={isActive ? `color-mix(in srgb, ${color} 28%, transparent)` : `color-mix(in srgb, ${color} 12%, transparent)`}
      color={color}
      fontSize="10px"
      px={1.5}
      py="2px"
      borderRadius="md"
      border="1px solid"
      borderColor={isActive ? `color-mix(in srgb, ${color} 55%, transparent)` : `color-mix(in srgb, ${color} 28%, transparent)`}
      minW={0}
      maxW={maxTagW}
      flexShrink={1}
      cursor="pointer"
      _hover={{
        bg: `color-mix(in srgb, ${color} 22%, transparent)`,
        borderColor: `color-mix(in srgb, ${color} 50%, transparent)`,
      }}
      onClick={(e) => { e.stopPropagation(); onTagClick(tag) }}
      transition="background 0.1s, border-color 0.1s"
      title={tag}
    >
      <TagLabel noOfLines={1}>{tag}</TagLabel>
    </Tag>
  )
}

function OverflowBadge({
  tags,
  tagColorMap,
  onTagClick,
}: {
  tags: string[]
  tagColorMap: Record<string, { color: string }>
  onTagClick: (tag: string) => void
}) {
  const [isOpen, setIsOpen] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const open = () => { if (timerRef.current) clearTimeout(timerRef.current); setIsOpen(true) }
  const close = () => { timerRef.current = setTimeout(() => setIsOpen(false), 150) }

  return (
    <Popover isOpen={isOpen} placement="top" isLazy gutter={6} closeOnBlur={false}>
      <PopoverTrigger>
        <Tag
          size="sm"
          bg="whiteAlpha.50"
          color="gray.500"
          fontSize="10px"
          px={1.5}
          py="2px"
          borderRadius="md"
          border="1px solid"
          borderColor="whiteAlpha.100"
          flexShrink={0}
          cursor="default"
          fontWeight="medium"
          onMouseEnter={open}
          onMouseLeave={close}
          _hover={{ bg: 'whiteAlpha.100', color: 'gray.300' }}
          transition="all 0.1s"
        >
          +{tags.length}
        </Tag>
      </PopoverTrigger>
      <PopoverContent
        bg="rgb(var(--bg-main-rgb))"
        borderColor="whiteAlpha.200"
        boxShadow="0 4px 20px rgba(0,0,0,0.5)"
        w="auto"
        maxW="260px"
        _focus={{ outline: 'none' }}
        onMouseEnter={open}
        onMouseLeave={close}
      >
        <PopoverArrow bg="rgb(var(--bg-main-rgb))" borderColor="whiteAlpha.200" />
        <PopoverBody px={2.5} py={2.5}>
          <Flex flexWrap="wrap" gap={1}>
            {tags.map((tag) => (
              <TagPill key={tag} tag={tag} tagColorMap={tagColorMap} onTagClick={onTagClick} />
            ))}
          </Flex>
        </PopoverBody>
      </PopoverContent>
    </Popover>
  )
}

function InventoryTagList({
  tags,
  tagColorMap,
  activeTags,
  onTagClick,
}: {
  tags: string[]
  tagColorMap: Record<string, { color: string }>
  activeTags: string[]
  onTagClick: (tag: string) => void
}) {
  if (tags.length === 0) return <Text fontSize="xs" color="gray.700">—</Text>

  const MAX_VISIBLE = 3
  const visibleTags = tags.slice(0, MAX_VISIBLE)
  const overflowTags = tags.slice(MAX_VISIBLE)
  const hasOverflow = overflowTags.length > 0
  const perTagMaxW = visibleTags.length === 1
    ? '160px'
    : visibleTags.length === 2
      ? (hasOverflow ? '72px' : '84px')
      : (hasOverflow ? '48px' : '56px')

  return (
    <HStack spacing={1} w="full" overflow="hidden" flexWrap="nowrap" align="center">
      {visibleTags.map((tag) => (
        <TagPill
          key={tag}
          tag={tag}
          tagColorMap={tagColorMap}
          isActive={activeTags.includes(tag)}
          maxTagW={perTagMaxW}
          onTagClick={onTagClick}
        />
      ))}
      {hasOverflow && (
        <OverflowBadge tags={overflowTags} tagColorMap={tagColorMap} onTagClick={onTagClick} />
      )}
    </HStack>
  )
}

function FilterSection({
  title,
  children,
  collapsed,
  onToggle,
}: {
  title: string
  children: React.ReactNode
  collapsed: boolean
  onToggle: () => void
}) {
  return (
    <Box data-testid={`inventory-filter-section-${title.toLowerCase()}`} borderBottom="1px solid" borderColor="whiteAlpha.100">
      <Flex
        px={3}
        py={2}
        align="center"
        cursor="pointer"
        onClick={onToggle}
        _hover={{ bg: 'whiteAlpha.50' }}
        userSelect="none"
      >
        <Text fontSize="10px" color="gray.500" fontWeight="bold" textTransform="uppercase" flex={1}>{title}</Text>
        <Box color="gray.600" transform={collapsed ? 'rotate(0deg)' : 'rotate(90deg)'} transition="transform 0.15s">
          <ChevronRightIcon boxSize={3} />
        </Box>
      </Flex>
      {!collapsed && <Box px={3} pb={3}>{children}</Box>}
    </Box>
  )
}

function SortableHeader({
  label,
  sortKey,
  activeSortKey,
  sortDir,
  onSort,
  flex,
}: {
  label: string
  sortKey: 'name' | 'updatedAt' | 'usage' | 'type' | null
  activeSortKey: string
  sortDir: 'asc' | 'desc'
  onSort: (key: 'name' | 'updatedAt' | 'usage' | 'type') => void
  flex?: number
}) {
  const isActive = sortKey !== null && activeSortKey === sortKey
  return (
    <Flex
      align="center"
      gap={1}
      flex={flex}
      cursor={sortKey ? 'pointer' : 'default'}
      color={isActive ? 'gray.200' : 'gray.500'}
      _hover={sortKey ? { color: 'gray.300' } : undefined}
      userSelect="none"
      onClick={sortKey ? () => onSort(sortKey) : undefined}
      role={sortKey ? 'button' : undefined}
    >
      <Text fontSize="10px" fontWeight="bold" textTransform="uppercase">{label}</Text>
      {isActive && (
        <Box flexShrink={0}>
          {sortDir === 'asc' ? <TriangleUpIcon boxSize={2} /> : <TriangleDownIcon boxSize={2} />}
        </Box>
      )}
      {sortKey && !isActive && (
        <Box flexShrink={0} opacity={0.3}>
          <ChevronUpIcon boxSize={2.5} />
        </Box>
      )}
    </Flex>
  )
}
