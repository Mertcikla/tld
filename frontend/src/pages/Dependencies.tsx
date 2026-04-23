import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { motion } from 'framer-motion'
import {
  Box,
  Button,
  Flex,
  HStack,
  Input,
  InputGroup,
  InputLeftElement,
  Menu,
  MenuButton,
  MenuItem,
  MenuList,
  Spinner,
  Tag,
  Text,
  VStack,
  Grid,
} from '@chakra-ui/react'
import { api } from '../api/client'
import type { DependencyConnector, DependencyElement } from '../types'
import { TYPE_COLORS, ELEMENT_TYPES } from '../types'
import { useSetHeader } from '../components/HeaderContext'
import { ElementContainer } from '../components/NodeContainer'
import { ElementBody } from '../components/NodeBody'
import DependenciesOnboarding from '../components/DependenciesOnboarding'
import { useTheme } from '../context/ThemeContext'
import { hexToRgba } from '../constants/colors'

// ── Data types ─────────────────────────────────────────────────────────────
interface ElementWithNeighbours extends DependencyElement {
  neighbourCount: number
}

interface NeighbourNode {
  element: DependencyElement
  connectors: DependencyConnector[]
  position: 'left' | 'right' | 'top' | 'bottom'
}

// ── Helpers ────────────────────────────────────────────────────────────────
function computeNeighbourCounts(elements: DependencyElement[], connectors: DependencyConnector[]): ElementWithNeighbours[] {
  const counts = new Map<string, Set<string>>()
  elements.forEach((element) => counts.set(element.id, new Set()))
  connectors.forEach((connector) => {
    counts.get(connector.source_element_id)?.add(connector.target_element_id)
    counts.get(connector.target_element_id)?.add(connector.source_element_id)
  })
  return elements.map((element) => ({ ...element, neighbourCount: counts.get(element.id)?.size ?? 0 }))
}

function getNeighbourGraph(selectedId: string, elements: DependencyElement[], allConnectors: DependencyConnector[]): NeighbourNode[] {
  const elementMap = new Map<string, DependencyElement>(elements.map((element) => [element.id, element]))
  const related = allConnectors.filter(
    (connector) => connector.source_element_id === selectedId || connector.target_element_id === selectedId,
  )
  const grouped = new Map<string, DependencyConnector[]>()
  related.forEach((connector) => {
    const otherId = connector.source_element_id === selectedId ? connector.target_element_id : connector.source_element_id
    if (!grouped.has(otherId)) grouped.set(otherId, [])
    grouped.get(otherId)!.push(connector)
  })
  const result: NeighbourNode[] = []
  grouped.forEach((connectors, otherId) => {
    const element = elementMap.get(otherId)
    if (!element) return
    let hasIncoming = false
    let hasOutgoing = false
    let hasBoth = false
    let hasUndirected = false
    connectors.forEach((connector) => {
      const dir = connector.direction || 'forward'
      if (dir === 'both' || dir === 'bidirectional') hasBoth = true
      else if (dir === 'none') hasUndirected = true
      else if (dir === 'forward') {
        if (connector.source_element_id === selectedId) hasOutgoing = true
        else hasIncoming = true
      } else if (dir === 'backward') {
        if (connector.source_element_id === selectedId) hasIncoming = true
        else hasOutgoing = true
      }
    })
    let position: NeighbourNode['position']
    if (hasBoth) position = 'top'
    else if (hasUndirected) position = 'bottom'
    else if (hasIncoming && hasOutgoing) position = 'top'
    else if (hasIncoming) position = 'left'
    else position = 'right'
    result.push({ element, connectors, position })
  })
  return result
}

function chunkNodes(nodes: NeighbourNode[], size = 20): NeighbourNode[][] {
  if (nodes.length <= size) return [nodes]

  const chunks: NeighbourNode[][] = []
  for (let index = 0; index < nodes.length; index += size) {
    chunks.push(nodes.slice(index, index + size))
  }
  return chunks
}

// ── Direction indicator ─────────────────────────────────────────────────────
function ConnectionIndicator({
  position,
  compactLevel,
}: {
  position: NeighbourNode['position']
  compactLevel: number
}) {
  const orientation = position === 'left' || position === 'right' ? 'horizontal' : 'vertical'
  const config =
    position === 'bottom'
      ? { icon: '·', label: 'undirected', color: '#94a3b8', tint: 'rgba(148,163,184,0.16)' }
      : position === 'top'
        ? { icon: '↕', label: 'bidirectional', color: '#5eead4', tint: 'rgba(45,212,191,0.16)' }
        : position === 'left'
          ? { icon: '→', label: 'directional', color: '#c4b5fd', tint: 'rgba(167,139,250,0.18)' }
          : { icon: '→', label: 'directional', color: '#7dd3fc', tint: 'rgba(56,189,248,0.18)' }
  const isCompact = compactLevel >= 2
  const lineColor = `${config.color}66`
  // outer = away from center node; inner = toward center node (longer to visually reach it)
  const outerLine = isCompact ? '10px' : '18px'
  const innerLine = isCompact ? '24px' : '44px'
  const firstLineSize = (position === 'right' || position === 'bottom') ? innerLine : outerLine
  const secondLineSize = (position === 'left' || position === 'top') ? innerLine : outerLine

  return (
    <Flex
      align="center"
      justify="center"
      direction={orientation === 'horizontal' ? 'row' : 'column'}
      gap={isCompact ? 1 : 1.5}
      flexShrink={0}
      aria-label={config.label}
    >
      <Box
        w={orientation === 'horizontal' ? firstLineSize : '1px'}
        h={orientation === 'vertical' ? firstLineSize : '1px'}
        bg={lineColor}
        borderRadius="full"
      />
      <Flex
        align="center"
        justify="center"
        w={isCompact ? '20px' : '24px'}
        h={isCompact ? '20px' : '24px'}
        borderRadius="full"
        border="1px solid"
        borderColor={lineColor}
        color={config.color}
        bg={config.tint}
        boxShadow={`0 0 0 1px ${config.tint}`}
        fontSize={isCompact ? '11px' : '12px'}
        fontWeight="bold"
      >
        {config.icon}
      </Flex>
      <Box
        w={orientation === 'horizontal' ? secondLineSize : '1px'}
        h={orientation === 'vertical' ? secondLineSize : '1px'}
        bg={lineColor}
        borderRadius="full"
      />
    </Flex>
  )
}

// ── Neighbour card ───────────────────────────────────────────────────────────
// compactLevel: 0 = full, 1 = no connector labels, 2 = no connectors/tech, 3 = name only + minimal padding
function NeighbourCard({
  node,
  onClick,
  setRef,
  compactLevel = 0,
}: {
  node: NeighbourNode
  onClick: () => void
  setRef?: (el: HTMLDivElement | null) => void
  compactLevel?: number
}) {
  const cardPadding = compactLevel >= 3 ? 1 : compactLevel >= 2 ? 1.5 : compactLevel >= 1 ? 2 : 3
  const showTech = compactLevel < 2
  const showType = compactLevel < 3
  const minW = compactLevel >= 2 ? '100px' : '130px'
  const maxW = compactLevel >= 2 ? '160px' : '200px'

  const rawName = node.element.name ?? ''
  const truncatedName = rawName.length > 30 ? rawName.slice(0, 29) + '…' : rawName
  const nameLen = truncatedName.length
  const nameSize =
    compactLevel >= 3 ? (nameLen > 15 ? '2xs' : 'xs') :
      compactLevel >= 2 ? (nameLen > 20 ? '2xs' : 'xs') :
        compactLevel >= 1 ? (nameLen > 22 ? 'xs' : 'sm') :
          (nameLen > 24 ? 'xs' : 'sm')

  return (
    <motion.div
      ref={setRef}
      data-pan-block="true"
      initial={{ opacity: 0, scale: 0.92 }}
      animate={{ opacity: 1, scale: 1 }}
      whileHover={{ scale: 1.02 }}
      transition={{ duration: 0.18 }}
    >
      <ElementContainer
        onClick={onClick}
        minW={minW}
        maxW={maxW}
        p={0}
        cursor="pointer"
        borderColor="whiteAlpha.200"
        _hover={{ borderColor: 'var(--accent)', boxShadow: '0 0 0 1px rgba(var(--accent-rgb), 0.25)' }}
      >
        <ElementBody
          name={truncatedName}
          type={showType ? (node.element.type ?? '') : ''}
          technology={showTech ? (node.element.technology || undefined) : undefined}
          nameSize={nameSize}
          align="flex-start"
          p={cardPadding}
        />
      </ElementContainer>
    </motion.div>
  )
}

// ── Search icon ──────────────────────────────────────────────────────────────
function SearchIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="11" cy="11" r="8" />
      <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
  )
}

// ── Type accent color map (CSS hex values for non-Chakra contexts) ───────────
const TYPE_HEX: Record<string, string> = {
  person: '#4fd1c5',
  system: '#63b3ed',
  container: '#b794f4',
  component: '#f6ad55',
  database: '#76e4f7',
  queue: '#faf089',
  api: '#68d391',
  service: '#fbb6ce',
  external: '#718096',
}

// ── Main element/component ───────────────────────────────────────────────────────────
export default function Dependencies() {
  const setHeader = useSetHeader()
  const { accent, elementColor } = useTheme()

  const [elements, setElements] = useState<DependencyElement[]>([])
  const [allEdges, setAllEdges] = useState<DependencyConnector[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState('')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [topRatio, setTopRatio] = useState(0.45)

  // Graph layout measurement
  const graphRef = useRef<HTMLDivElement>(null)
  const [graphHeight, setGraphHeight] = useState(0)

  // Divider drag
  const containerRef = useRef<HTMLDivElement>(null)
  const draggingRef = useRef(false)
  const [isDragging, setIsDragging] = useState(false)

  // Canvas pan kept entirely off React state to avoid re-renders on every mousemove
  const canvasPanRef = useRef({ x: 0, y: 0 })
  const panContainerRef = useRef<HTMLDivElement>(null)
  const canvasPanningRef = useRef(false)
  const canvasPanStartRef = useRef({ touchX: 0, touchY: 0, panX: 0, panY: 0 })

  const applyPan = useCallback((x: number, y: number) => {
    canvasPanRef.current = { x, y }
    if (panContainerRef.current) {
      panContainerRef.current.style.transform = `translate(${x}px, ${y}px)`
    }
  }, [])

  useEffect(() => { applyPan(0, 0) }, [selectedId, applyPan])

  // Header
  useEffect(() => {
    setHeader({
      hideMobileBar: true,
      node: (
        <HStack
          bg="whiteAlpha.50"
          border="1px solid"
          borderColor="whiteAlpha.100"
          px={3}
          py={1}
          borderRadius="md"
          spacing={3}
        >
          <Text fontSize="xs" color="whiteAlpha.800" fontWeight="medium" display={{ base: 'none', compact: 'inline' }}>
            {elements.length} <Text as="span" color="whiteAlpha.400" fontWeight="normal">elements</Text>
          </Text>
          <Box w="1px" h="10px" bg="whiteAlpha.200" display={{ base: 'none', compact: 'block' }} />
          <Text fontSize="xs" color="whiteAlpha.800" fontWeight="medium" display={{ base: 'none', compact: 'inline' }}>
            {allEdges.length} <Text as="span" color="whiteAlpha.400" fontWeight="normal">connectors</Text>
          </Text>
          <Text fontSize="xs" color="whiteAlpha.800" fontWeight="medium" display={{ base: 'none', sm: 'inline', compact: 'none' }}>
            {elements.length}<Text as="span" color="whiteAlpha.400">E</Text>
            <Text as="span" color="whiteAlpha.200" mx={1}>/</Text>
            {allEdges.length}<Text as="span" color="whiteAlpha.400">C</Text>
          </Text>
        </HStack>
      ),
    })
    return () => setHeader(null)
  }, [elements.length, allEdges.length, setHeader])

  // Data fetch
  useEffect(() => {
    api.dependencies
      .list()
      .then((resp) => {
        const objs = resp.elements || []
        const edgs = resp.connectors || []
        setElements(objs)
        setAllEdges(edgs)

        if (objs.length > 0) {
          const withCounts = computeNeighbourCounts(objs, edgs)
          const sorted = [...withCounts].sort((a, b) => b.neighbourCount - a.neighbourCount)
          setSelectedId(sorted[0].id)
        }
      })
      .catch(() => { /* intentionally empty */ })
      .finally(() => setLoading(false))
  }, [])

  // Derived data
  const elementsWithCounts = useMemo(
    () => computeNeighbourCounts(elements, allEdges),
    [elements, allEdges],
  )

  const filteredElements = useMemo(() => {
    let list = elementsWithCounts
    if (search) {
      const q = search.toLowerCase()
      list = list.filter((o) => {
        const nameMatch = (o.name || '').toLowerCase().includes(q)
        const typeMatch = (o.type || '').toLowerCase().includes(q)
        const techMatch = (o.technology || '').toLowerCase().includes(q)
        const tags = Array.isArray(o.tags) ? o.tags : []
        const tagMatch = tags.some((t) => (t || '').toLowerCase().includes(q))
        return nameMatch || typeMatch || techMatch || tagMatch
      })
    }
    if (typeFilter) list = list.filter((o) => o.type === typeFilter)
    return [...list].sort((a, b) => b.neighbourCount - a.neighbourCount)
  }, [elementsWithCounts, search, typeFilter])

  const selectedElement = useMemo(() => {
    if (selectedId === null) return null
    return elements.find((o) => o.id === selectedId) || null
  }, [elements, selectedId])
  const neighbourGraph = useMemo(() => {
    if (selectedId === null) return []
    return getNeighbourGraph(selectedId, elements, allEdges)
  }, [selectedId, elements, allEdges])

  // Divider drag
  const startDrag = useCallback(() => {
    draggingRef.current = true
    setIsDragging(true)
    document.body.style.cursor = 'row-resize'
    document.body.style.userSelect = 'none'
  }, [])

  const onDividerMouseDown = useCallback(() => { startDrag() }, [startDrag])

  const onDividerTouchStart = useCallback((e: React.TouchEvent) => {
    e.preventDefault()
    startDrag()
  }, [startDrag])

  useEffect(() => {
    const applyClientY = (clientY: number) => {
      if (!draggingRef.current || !containerRef.current) return
      const rect = containerRef.current.getBoundingClientRect()
      const ratio = Math.max(0.15, Math.min(0.85, (clientY - rect.top) / rect.height))
      setTopRatio(ratio)
    }
    const stopDrag = () => {
      draggingRef.current = false
      setIsDragging(false)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }
    const onMouseMove = (e: MouseEvent) => applyClientY(e.clientY)
    const onTouchMove = (e: TouchEvent) => {
      if (!draggingRef.current) return
      e.preventDefault()
      applyClientY(e.touches[0].clientY)
    }
    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', stopDrag)
    window.addEventListener('touchmove', onTouchMove, { passive: false })
    window.addEventListener('touchend', stopDrag)
    return () => {
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', stopDrag)
      window.removeEventListener('touchmove', onTouchMove)
      window.removeEventListener('touchend', stopDrag)
    }
  }, [])

  const shouldBlockCanvasPan = useCallback((target: EventTarget | null) => {
    return target instanceof HTMLElement && Boolean(target.closest('[data-pan-block="true"]'))
  }, [])

  const startCanvasPan = useCallback((clientX: number, clientY: number, target: EventTarget | null) => {
    if (shouldBlockCanvasPan(target)) return
    canvasPanningRef.current = true
    if (graphRef.current) graphRef.current.style.cursor = 'grabbing'
    canvasPanStartRef.current = {
      touchX: clientX,
      touchY: clientY,
      panX: canvasPanRef.current.x,
      panY: canvasPanRef.current.y,
    }
  }, [shouldBlockCanvasPan])

  const onCanvasMouseDown = useCallback((e: React.MouseEvent) => {
    if (e.button !== 0) return
    e.preventDefault()
    startCanvasPan(e.clientX, e.clientY, e.target)
  }, [startCanvasPan])

  const onCanvasTouchStart = useCallback((e: React.TouchEvent) => {
    if (e.touches.length !== 1) return
    startCanvasPan(e.touches[0].clientX, e.touches[0].clientY, e.target)
  }, [startCanvasPan])

  useEffect(() => {
    const onMouseMove = (e: MouseEvent) => {
      if (!canvasPanningRef.current) return
      applyPan(
        canvasPanStartRef.current.panX + e.clientX - canvasPanStartRef.current.touchX,
        canvasPanStartRef.current.panY + e.clientY - canvasPanStartRef.current.touchY,
      )
    }
    const stopCanvasPan = () => {
      if (!canvasPanningRef.current) return
      canvasPanningRef.current = false
      if (graphRef.current) graphRef.current.style.cursor = 'grab'
    }
    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', stopCanvasPan)
    return () => {
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', stopCanvasPan)
    }
  }, [applyPan])

  useEffect(() => {
    const onTouchMove = (e: TouchEvent) => {
      if (!canvasPanningRef.current || e.touches.length !== 1) return
      e.preventDefault()
      applyPan(
        canvasPanStartRef.current.panX + e.touches[0].clientX - canvasPanStartRef.current.touchX,
        canvasPanStartRef.current.panY + e.touches[0].clientY - canvasPanStartRef.current.touchY,
      )
    }
    const onTouchEnd = () => { canvasPanningRef.current = false }
    window.addEventListener('touchmove', onTouchMove, { passive: false })
    window.addEventListener('touchend', onTouchEnd)
    return () => {
      window.removeEventListener('touchmove', onTouchMove)
      window.removeEventListener('touchend', onTouchEnd)
    }
  }, [applyPan])

  // Track graph container height for responsive compactness
  useEffect(() => {
    if (selectedId === null) { setGraphHeight(0); return }
    const el = graphRef.current
    if (!el) return
    const ro = new ResizeObserver((entries) => {
      setGraphHeight(entries[0]?.contentRect.height ?? 0)
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [selectedId, topRatio])

  if (loading) {
    return (
      <Flex h="100vh" align="center" justify="center">
        <Spinner size="xl" color="blue.500" thickness="3px" />
      </Flex>
    )
  }

  const leftNodes = neighbourGraph.filter((n) => n.position === 'left')
  const rightNodes = neighbourGraph.filter((n) => n.position === 'right')
  const topNodes = neighbourGraph.filter((n) => n.position === 'top')
  const bottomNodes = neighbourGraph.filter((n) => n.position === 'bottom')
  const leftColumns = chunkNodes(leftNodes)
  const rightColumns = chunkNodes(rightNodes)
  const topRows = chunkNodes(topNodes)
  const bottomRows = chunkNodes(bottomNodes)
  const leftColumnSize = Math.max(...leftColumns.map((column) => column.length), 0)
  const rightColumnSize = Math.max(...rightColumns.map((column) => column.length), 0)

  // Responsive compactness: computed independently per column
  const toCompactLevel = (pxPerSlot: number) =>
    pxPerSlot > 160 ? 0 : pxPerSlot > 110 ? 1 : pxPerSlot > 70 ? 2 : 3
  const leftCompactLevel = toCompactLevel(
    graphHeight > 0 && leftColumnSize > 0 ? graphHeight / leftColumnSize : 999,
  )
  const rightCompactLevel = toCompactLevel(
    graphHeight > 0 && rightColumnSize > 0 ? graphHeight / rightColumnSize : 999,
  )
  // Top/bottom rows and overall layout spacing use the worst-case level
  const maxCompactLevel = Math.max(leftCompactLevel, rightCompactLevel, 0)
  const colSpacing = maxCompactLevel >= 3 ? 2 : maxCompactLevel >= 2 ? 3 : maxCompactLevel >= 1 ? 5 : 8
  const nodeSpacing = maxCompactLevel >= 2 ? 1 : maxCompactLevel >= 1 ? 2 : 3
  const selectedCardShadow = `0 0 0 3px ${hexToRgba(accent, 0.38)}, 0 18px 48px ${hexToRgba(accent, 0.12)}, 0 10px 36px rgba(0,0,0,0.55), 0 3px 10px rgba(0,0,0,0.4)`

  return (
    <Box h="100vh" display="flex" flexDir="column" bg="var(--bg-canvas)">
      <Box ref={containerRef} flex={1} display="flex" flexDir="column" overflow="hidden">

        {/* ── Top: Listing ──────────────────────────────────────────────────── */}
        <Box
          h={`${topRatio * 100}%`}
          minH="120px"
          display="flex"
          flexDir="column"
          overflow="hidden"
          bg="var(--bg-canvas)"
        >
          {/* Filter bar */}
          <Flex
            px={5}
            py={2.5}
            gap={3}
            flexShrink={0}
            align="center"
            borderBottom="1px solid"
            borderColor="whiteAlpha.100"
          >
            <InputGroup size="sm" maxW="340px">
              <InputLeftElement pointerEvents="none" color="gray.600" h="full" pl={1}>
                <SearchIcon />
              </InputLeftElement>
              <Input
                variant="elevated"
                placeholder="Search by name, type, technology…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                pl={8}
                fontSize="sm"
              />
            </InputGroup>
            <Menu placement="bottom-start">
              <MenuButton
                as={Button}
                variant="elevated"
                size="sm"
                minW="120px"
                textAlign="left"
                fontWeight="medium"
                fontSize="sm"
                rightIcon={
                  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                    <polyline points="6 9 12 15 18 9" />
                  </svg>
                }
              >
                {typeFilter || 'All types'}
              </MenuButton>
              <MenuList>
                <MenuItem onClick={() => setTypeFilter('')} fontWeight={!typeFilter ? 'bold' : 'normal'}>
                  All types
                </MenuItem>
                {ELEMENT_TYPES.map((t) => (
                  <MenuItem key={t} onClick={() => setTypeFilter(t)} fontWeight={typeFilter === t ? 'bold' : 'normal'}>
                    {t}
                  </MenuItem>
                ))}
              </MenuList>
            </Menu>
            <Box flex={1} />
            <Text fontSize="xs" color="gray.600">
              {filteredElements.length} element{filteredElements.length !== 1 ? 's' : ''}
            </Text>
          </Flex>

          {/* Column headers */}
          <Flex
            px={5}
            py={1.5}
            gap={0}
            borderBottom="1px solid"
            borderColor="whiteAlpha.50"
            flexShrink={0}
            align="center"
          >
            <Box flex={1} minW={0} pl={5}>
              <Text fontSize="10px" color="gray.600" textTransform="uppercase" letterSpacing="0.08em" fontWeight="bold">Name</Text>
            </Box>
            <Box w="110px" flexShrink={0}>
              <Text fontSize="10px" color="gray.600" textTransform="uppercase" letterSpacing="0.08em" fontWeight="bold">Type</Text>
            </Box>
            <Box w="130px" flexShrink={0} display={{ base: 'none', lg: 'block' }}>
              <Text fontSize="10px" color="gray.600" textTransform="uppercase" letterSpacing="0.08em" fontWeight="bold">Technology</Text>
            </Box>
          </Flex>

          {/* Scrollable list */}
          <Box flex={1} overflowY="auto">
            {filteredElements.length === 0 ? (
              <Flex flexDir="column" align="center" justify="center" py={12} gap={2}>
                <Text color="gray.600" fontSize="sm">No elements match your filters.</Text>
                <Button
                  variant="link"
                  size="sm"
                  color="blue.400"
                  onClick={() => { setSearch(''); setTypeFilter('') }}
                >
                  Clear filters
                </Button>
              </Flex>
            ) : (
              filteredElements.map((obj) => {
                const typeKey = obj.type ?? ''
                const color = TYPE_COLORS[typeKey] ?? 'gray'
                const accentHex = TYPE_HEX[typeKey] ?? '#718096'
                const isSelected = selectedId === obj.id

                return (
                  <Flex
                    key={obj.id}
                    px={5}
                    h="42px"
                    align="center"
                    cursor="pointer"
                    borderBottom="1px solid"
                    borderColor="whiteAlpha.50"
                    bg={isSelected ? 'rgba(66,153,225,0.07)' : 'transparent'}
                    _hover={{ bg: isSelected ? 'rgba(66,153,225,0.1)' : 'whiteAlpha.50' }}
                    transition="background 0.1s"
                    onClick={() => setSelectedId(isSelected ? null : obj.id)}
                    position="relative"
                    role="row"
                  >
                    {/* Left type-color accent */}
                    <Box
                      w="3px"
                      alignSelf="stretch"
                      borderRadius="full"
                      flexShrink={0}
                      mr={3.5}
                      style={{ background: accentHex, opacity: isSelected ? 1 : 0.4 }}
                    />

                    {/* Name */}
                    <Box flex={1} minW={0} mr={4}>
                      <Text
                        fontSize="sm"
                        fontWeight={isSelected ? 'semibold' : 'medium'}
                        color={isSelected ? 'white' : 'gray.100'}
                        noOfLines={1}
                      >
                        {obj.name}
                      </Text>
                    </Box>

                    {/* Type badge */}
                    <Box w="110px" flexShrink={0}>
                      {obj.type && (
                        <Tag
                          size="sm"
                          colorScheme={color}
                          variant="subtle"
                          fontSize="9px"
                          fontWeight="bold"
                          textTransform="uppercase"
                          letterSpacing="0.06em"
                        >
                          {obj.type}
                        </Tag>
                      )}
                    </Box>

                    {/* Technology */}
                    <Box w="130px" flexShrink={0} display={{ base: 'none', lg: 'block' }}>
                      <Text fontSize="xs" color="gray.500" fontFamily="mono" noOfLines={1}>
                        {obj.technology || <Text as="span" color="gray.700">-</Text>}
                      </Text>
                    </Box>

                  </Flex>
                )
              })
            )}
          </Box>
        </Box>

        {/* ── Divider ───────────────────────────────────────────────────────── */}
        <Flex
          h={{ base: '20px', md: '8px' }}
          flexShrink={0}
          cursor="row-resize"
          align="center"
          justify="center"
          bg="var(--bg-panel)"
          borderY="1px solid"
          borderColor="whiteAlpha.100"
          _hover={{ bg: 'blue.900', borderColor: 'blue.500' }}
          transition="all 0.2s"
          onMouseDown={onDividerMouseDown}
          onTouchStart={onDividerTouchStart}
          role="separator"
          sx={{ touchAction: 'none' }}
        >
          <Box w="48px" h="3px" bg={isDragging ? 'blue.400' : 'whiteAlpha.200'} borderRadius="full" />
        </Flex>

        {/* ── Bottom: Dependency graph ──────────────────────────────────────── */}
        <Box
          flex={1}
          minH="120px"
          display="flex"
          alignItems="center"
          justifyContent="center"
          bg="var(--bg-canvas)"
          backgroundImage="radial-gradient(circle, #2D3748 0.5px, transparent 0.5px)"
          backgroundSize="24px 24px"
          position="relative"
          overflow="hidden"
        >
          {!selectedId ? (
            <VStack spacing={3} opacity={0.55}>
              <Box
                p={4}
                borderRadius="full"
                bg="whiteAlpha.50"
                border="1px dashed"
                borderColor="whiteAlpha.150"
              >
                <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" style={{ color: '#4A5568' }}>
                  <circle cx="12" cy="5" r="2" />
                  <circle cx="5" cy="19" r="2" />
                  <circle cx="19" cy="19" r="2" />
                  <line x1="12" y1="7" x2="5" y2="17" />
                  <line x1="12" y1="7" x2="19" y2="17" />
                </svg>
              </Box>
              <VStack spacing={0.5}>
                <Text color="gray.400" fontSize="sm" fontWeight="medium">Select an element to explore</Text>
                <Text color="gray.600" fontSize="xs">Dependency graph appears here</Text>
              </VStack>
            </VStack>
          ) : (
            <Box
              ref={graphRef}
              key={selectedId}
              w="full"
              h="full"
              position="relative"
              style={{ cursor: 'grab' }}
              onMouseDown={onCanvasMouseDown}
              onTouchStart={onCanvasTouchStart}
              sx={{ touchAction: 'none' }}
            >
              {/* Pannable inner container transform applied imperatively to avoid re-renders */}
              <div ref={panContainerRef} style={{ position: 'absolute', inset: 0, overflow: 'visible' }}>
                {/* Node layout */}
                <motion.div
                  initial={{ opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ duration: 0.22 }}
                  style={{ width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', position: 'relative', zIndex: 1, padding: '40px' }}
                >
                  {/* Three-column row: left group | center column | right group */}
                  <Flex direction="column" align="center">

                    {/* Top group */}
                    {topNodes.length > 0 && (
                      <Flex direction="column" align="center">
                        <VStack spacing={nodeSpacing} align="center">
                          {topRows.map((row, rowIndex) => (
                            <HStack key={`top-row-${rowIndex}`} spacing={nodeSpacing} align="flex-end">
                              {row.map((n) => (
                                <NeighbourCard
                                  key={n.element.id}
                                  node={n}
                                  compactLevel={maxCompactLevel}
                                  onClick={() => setSelectedId(n.element.id)}
                                />
                              ))}
                            </HStack>
                          ))}
                        </VStack>
                        <ConnectionIndicator position="top" compactLevel={maxCompactLevel} />
                      </Flex>
                    )}

                    {/* Middle Row: Left Group → Selected Node → Right Group */}
                    <Grid templateColumns="1fr auto 1fr" gap={colSpacing} alignItems="center" w="full">
                      {/* Left group */}
                      <Flex justify="flex-end">
                        {leftNodes.length > 0 && (
                          <Flex gap={nodeSpacing} align="center">
                            {leftColumns.map((column, columnIndex) => (
                              <VStack key={`left-column-${columnIndex}`} spacing={nodeSpacing} align="flex-end">
                                {column.map((n) => (
                                  <NeighbourCard
                                    key={n.element.id}
                                    node={n}
                                    compactLevel={leftCompactLevel}
                                    onClick={() => setSelectedId(n.element.id)}
                                  />
                                ))}
                              </VStack>
                            ))}
                            <ConnectionIndicator position="left" compactLevel={leftCompactLevel} />
                          </Flex>
                        )}
                      </Flex>

                      {/* Center: selected node */}
                      <Box position="relative" zIndex={10} isolation="isolate" data-pan-block="true">
                        <ElementContainer
                          isSelected
                          px={8}
                          py={6}
                          minW="220px"
                          maxW="300px"
                          bg={elementColor}
                          borderColor={accent}
                          borderWidth="2px"
                          boxShadow={selectedCardShadow}
                        >
                          <ElementBody
                            name={selectedElement?.name || ''}
                            type={selectedElement?.type || ''}
                            technology={selectedElement?.technology || undefined}
                            nameSize="md"
                            p={0}
                          >
                            <HStack mt={4} spacing={2}>
                              <Box w="6px" h="6px" borderRadius="full" bg={accent} />
                              <Text fontSize="xs" color={accent} fontWeight="bold">
                                {neighbourGraph.length} connection{neighbourGraph.length !== 1 ? 's' : ''}
                              </Text>
                            </HStack>
                          </ElementBody>
                        </ElementContainer>
                      </Box>

                      {/* Right group */}
                      <Flex justify="flex-start">
                        {rightNodes.length > 0 && (
                          <Flex gap={nodeSpacing} align="center">
                            <ConnectionIndicator position="right" compactLevel={rightCompactLevel} />
                            {rightColumns.map((column, columnIndex) => (
                              <VStack key={`right-column-${columnIndex}`} spacing={nodeSpacing} align="flex-start">
                                {column.map((n) => (
                                  <NeighbourCard
                                    key={n.element.id}
                                    node={n}
                                    compactLevel={rightCompactLevel}
                                    onClick={() => setSelectedId(n.element.id)}
                                  />
                                ))}
                              </VStack>
                            ))}
                          </Flex>
                        )}
                      </Flex>
                    </Grid>

                    {/* Bottom group */}
                    {bottomNodes.length > 0 && (
                      <Flex direction="column" align="center">
                        <ConnectionIndicator position="bottom" compactLevel={maxCompactLevel} />
                        <VStack spacing={nodeSpacing} align="center">
                          {bottomRows.map((row, rowIndex) => (
                            <HStack key={`bottom-row-${rowIndex}`} spacing={nodeSpacing} align="flex-start">
                              {row.map((n) => (
                                <NeighbourCard
                                  key={n.element.id}
                                  node={n}
                                  compactLevel={maxCompactLevel}
                                  onClick={() => setSelectedId(n.element.id)}
                                />
                              ))}
                            </HStack>
                          ))}
                        </VStack>
                      </Flex>
                    )}

                    {neighbourGraph.length === 0 && (
                      <Text color="gray.600" fontSize="sm" fontStyle="italic">
                        No direct connections found.
                      </Text>
                    )}

                  </Flex>
                </motion.div>
              </div>{/* end pannable inner container */}
            </Box>
          )}
        </Box>
      </Box>

      <DependenciesOnboarding hasElements={elements.length > 0} />
    </Box>
  )
}
