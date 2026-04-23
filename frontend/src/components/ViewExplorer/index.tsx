import React, { memo, useMemo, useState, useRef, useCallback } from 'react'
import {
  Box,
  Divider,
  HStack,
  VStack,
} from '@chakra-ui/react'
import '../../styles/editor-panels.css'
import SlidingPanel from '../SlidingPanel'
import PanelHeader from '../PanelHeader'
import { useViewEditorContext } from '../../pages/ViewEditor/context'
import type {
  ViewTreeNode,
  ViewConnector,
  PlacedElement,
  LibraryElement,
  ViewLayer,
  Tag,
} from '../../types'

import { buildTree, flattenTree } from './utils'
import { NavItem, TreeNode } from './types'
import { ViewNavigator } from './ViewNavigator'
import { ViewSearch } from './ViewSearch'
import { ViewTree } from './ViewTree'
import { TagManager } from './TagManager'

interface Props {
  treeNodes: ViewTreeNode[]
  linksMap: Record<number, ViewConnector[]>
  viewElements: PlacedElement[]
  onNavigate: (id: number) => void
  onHoverZoom?: (elementId: number | null, type: 'in' | 'out' | null) => void
  isOpen: boolean
  onToggle: () => void
  isMobile?: boolean
  suppressed?: boolean
  activeTags: string[]
  setActiveTags: (tags: string[]) => void
  hiddenLayerTags: string[]
  setHiddenLayerTags: (tags: string[]) => void
  availableTags: string[]
  tagColors: Record<string, Tag>
  selectedElement?: LibraryElement | null
  onUpdateTags?: (elementId: number, tags: string[]) => Promise<void>
  onCreateTag: (tag: string, color?: string) => Promise<void>
  layers: ViewLayer[]
  onHoverLayer: (tags: string[] | null, color?: string | null) => void
  onCreateLayer: (name: string, tags: string[], color: string) => Promise<void>
  onUpdateLayer: (layer: ViewLayer) => Promise<void>
  onDeleteLayer: (id: number) => Promise<void>
}

function ViewExplorer({
  treeNodes,
  linksMap,
  viewElements,
  onNavigate,
  onHoverZoom,
  isOpen,
  onToggle,
  isMobile = false,
  suppressed = false,
  hiddenLayerTags,
  setHiddenLayerTags,
  availableTags,
  tagColors,
  selectedElement,
  onUpdateTags,
  onCreateTag,
  layers,
  onHoverLayer,
  onCreateLayer,
  onUpdateLayer,
  onDeleteLayer,
}: Props) {
  const { viewId } = useViewEditorContext()
  const [query, setQuery] = useState('')
  const [activeFilter, setActiveFilter] = useState<'out' | 'in' | null>(null)
  const [focusedIdx, setFocusedIdx] = useState(-1)
  const [bottomHeight, setBottomHeight] = useState<number | undefined>(undefined)

  const containerRef = useRef<HTMLDivElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const itemRefs = useRef<(HTMLDivElement | null)[]>([])

  const onCanvasIds = useMemo(() => new Set(viewElements.map((o) => o.element_id)), [viewElements])

  // --- Layer/Tag counts ---
  const layerCounts = useMemo(() => {
    const counts: Record<number, number> = {}
    layers.forEach((layer) => {
      counts[layer.id] = viewElements.filter((obj) =>
        (obj.tags || []).some((t) => layer.tags.includes(t))
      ).length
    })
    return counts
  }, [layers, viewElements])

  const tagCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    availableTags.forEach((tag) => {
      counts[tag] = viewElements.filter((obj) => (obj.tags || []).includes(tag)).length
    })
    return counts
  }, [availableTags, viewElements])

  // --- Navigation helpers ---
  const handleNavigate = (id: number) => {
    setActiveFilter(null)
    onNavigate(id)
    if (isMobile) onToggle()
  }

  const roots = useMemo(() => buildTree(treeNodes), [treeNodes])
  const flat = useMemo(() => flattenTree(roots), [roots])

  const parents = useMemo(() => {
    if (viewId == null) return []
    const diagMap = new Map<number, NavItem>()
    
    // Find view path in tree to identify structural parent
    const findViewPath = (nodes: TreeNode[], targetId: number, path: TreeNode[] = []): TreeNode[] | null => {
      for (const node of nodes) {
        if (node.id === targetId) return [...path, node]
        const found = findViewPath(node.children, targetId, [...path, node])
        if (found) return found
      }
      return null
    }

    const viewPath = findViewPath(roots, viewId)
    const p = viewPath && viewPath.length > 1 ? viewPath[viewPath.length - 2] : null
    
    if (p) {
      diagMap.set(p.id, { id: p.id, name: p.name, subtitle: 'Parent View' })
    }

    return Array.from(diagMap.values())
  }, [viewId, roots])

  const children = useMemo(() => {
    if (viewId == null) return []
    const diagMap = new Map<number, NavItem>()
    flat.forEach((n) => {
      if (n.parent_view_id === viewId)
        diagMap.set(n.id, { id: n.id, name: n.name, subtitle: 'Child View' })
    })
    const objMap = new Map(viewElements.map((o) => [o.element_id, o.name]))
    Object.entries(linksMap).forEach(([elementIdStr, links]) => {
      const elementId = Number(elementIdStr)
      if (!Number.isFinite(elementId)) return
      const isOnCanvas = onCanvasIds.has(elementId)
      const objName = objMap.get(elementId) || 'Element'
      links.forEach((link) => {
        const existing = diagMap.get(link.to_view_id)
        if (existing) {
          if (!existing.elementId && isOnCanvas) existing.elementId = elementId
          if (existing.subtitle === 'Child View') {
            existing.subtitle = `Child View (Via ${objName})`
          } else if (!existing.subtitle?.includes(objName)) {
            existing.subtitle = `${existing.subtitle}, ${objName}`
          }
        } else {
          diagMap.set(link.to_view_id, {
            id: link.to_view_id,
            name: link.to_view_name,
            subtitle: `Via ${objName}`,
            elementId: isOnCanvas ? elementId : undefined,
          })
        }
      })
    })
    return Array.from(diagMap.values())
  }, [viewId, flat, linksMap, viewElements, onCanvasIds])

  const viewHoverMap = useMemo(() => {
    const map = new Map<number, { elementId: number | undefined; type: 'in' | 'out' }>()
    parents.forEach((p) => map.set(p.id, { elementId: p.elementId, type: 'out' }))
    children.forEach((c) => map.set(c.id, { elementId: c.elementId, type: 'in' }))
    return map
  }, [parents, children])

  const filteredByMode = useMemo(() => {
    if (activeFilter === 'out') {
      const parentIds = new Set(parents.map((p) => p.id))
      const ancestors = new Set<number>()
      const findAncestors = (id: number) => {
        const node = treeNodes.find(n => n.id === id)
        if (node?.parent_view_id) {
          ancestors.add(node.parent_view_id)
          findAncestors(node.parent_view_id)
        }
      }
      if (viewId) findAncestors(viewId)
      return flat.filter((n) => parentIds.has(n.id) || ancestors.has(n.id) || n.id === viewId)
    }
    if (activeFilter === 'in') {
      const childIds = new Set(children.map((c) => c.id))
      const subtree = new Set<number>()
      const traverse = (nodes: TreeNode[]) => {
        nodes.forEach(n => {
          subtree.add(n.id)
          traverse(n.children)
        })
      }
      const current = flat.find(n => n.id === viewId)
      if (current) traverse(current.children)
      return flat.filter((n) => childIds.has(n.id) || subtree.has(n.id) || n.id === viewId)
    }
    return flat
  }, [flat, parents, children, activeFilter, viewId, treeNodes])

  const filtered = useMemo(() => {
    if (!query.trim()) return filteredByMode
    const q = query.toLowerCase()
    return filteredByMode.filter((n) => n.name.toLowerCase().includes(q))
  }, [filteredByMode, query])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (filtered.length === 0) return
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault(); setFocusedIdx((i) => Math.min(i + 1, filtered.length - 1)); break
      case 'ArrowUp':
        e.preventDefault(); setFocusedIdx((i) => Math.max(i - 1, 0)); break
      case 'Enter':
        if (focusedIdx >= 0 && filtered[focusedIdx]) { e.preventDefault(); handleNavigate(filtered[focusedIdx].id) }; break
      case 'Escape':
        e.preventDefault(); setActiveFilter(null); break
    }
  }

  const handleDragStart = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault()
      const dragStartY = e.clientY
      const dragStartHeight = bottomHeight ?? (containerRef.current ? containerRef.current.offsetHeight / 2 : 240)
      const onMouseMove = (ev: MouseEvent) => {
        const delta = dragStartY - ev.clientY
        const maxHeight = containerRef.current ? containerRef.current.offsetHeight - 120 : 500
        setBottomHeight(Math.max(80, Math.min(maxHeight, dragStartHeight + delta)))
      }
      const onMouseUp = () => {
        window.removeEventListener('mousemove', onMouseMove)
        window.removeEventListener('mouseup', onMouseUp)
      }
      window.addEventListener('mousemove', onMouseMove)
      window.addEventListener('mouseup', onMouseUp)
    },
    [bottomHeight]
  )

  const handleToggleTagOnElement = async (tag: string) => {
    if (!selectedElement || !onUpdateTags) return
    const currentTags = selectedElement.tags || []
    const nextTags = currentTags.includes(tag)
      ? currentTags.filter((t) => t !== tag)
      : [...currentTags, tag]
    await onUpdateTags(selectedElement.id, nextTags)
  }

  return (
    <SlidingPanel
      isOpen={isOpen && !suppressed}
      onClose={onToggle}
      panelKey="view-explorer"
      side={isMobile ? 'left' : 'right'}
      width="300px"
      zIndex={1000}
      hasBackdrop={isMobile}
    >
      <VStack ref={containerRef} align="stretch" spacing={0} h="full" overflow="hidden">
        {/* Panel header */}
        <PanelHeader title="Explorer" onClose={onToggle} hasCloseButton={isMobile} />

        <ViewNavigator
          parents={parents}
          children={children}
          activeFilter={activeFilter}
          onFilterToggle={(type, items) => {
            if (items.length === 0) return
            if (items.length === 1) { handleNavigate(items[0].id); return }
            setActiveFilter((prev) => (prev === type ? null : type))
          }}
          onHoverZoom={onHoverZoom}
        />
        <Divider borderColor="whiteAlpha.100" />

        <ViewSearch query={query} setQuery={setQuery} activeFilter={activeFilter} />

        <ViewTree
          filtered={filtered}
          viewId={viewId}
          focusedIdx={focusedIdx}
          itemRefs={itemRefs}
          handleNavigate={handleNavigate}
          onHoverZoom={onHoverZoom}
          viewHoverMap={viewHoverMap}
          handleKeyDown={handleKeyDown}
          listRef={listRef}
        />

        {/* --- Drag handle --- */}
        <Box
          h="14px" 
          flexShrink={0} 
          cursor="row-resize" 
          position="relative"
          onMouseDown={handleDragStart} 
          userSelect="none"
          role="group"
          aria-label="Resize panels"
          transition="all 0.2s"
          _hover={{ bg: 'whiteAlpha.100' }}
        >
          {/* Visual separator line */}
          <Divider borderColor="whiteAlpha.200" />

          {/* Draggable handle indicator */}
          <Box
            position="absolute"
            top="50%"
            left="50%"
            transform="translate(-50%, -50%)"
            w="40px"
            h="4px"
            rounded="full"
            bg="whiteAlpha.300"
            pointerEvents="none"
            transition="all 0.2s cubic-bezier(0.4, 0, 0.2, 1)"
            _groupHover={{ 
              w: "48px",
              bg: "whiteAlpha.500",
              boxShadow: "0 0 10px rgba(255, 255, 255, 0.1)"
            }}
          />

          {/* Subtle textured dots for "drag" affordance */}
          <HStack
            position="absolute"
            top="50%"
            left="50%"
            transform="translate(-50%, -50%)"
            spacing="4px"
            pointerEvents="none"
            opacity={0.3}
            _groupHover={{ opacity: 0.6 }}
            transition="opacity 0.2s"
          >
            {[0, 1, 2].map((i) => (
              <Box key={i} w="2px" h="2px" rounded="full" bg="white" />
            ))}
          </HStack>
        </Box>

        <Box
          height={bottomHeight !== undefined ? `${bottomHeight}px` : '50%'}
          display="flex" flexDirection="column" overflow="hidden"
        >
          <TagManager
            availableTags={availableTags}
            tagColors={tagColors}
            layers={layers}
            viewElements={viewElements}
            selectedElement={selectedElement || null}
            hiddenLayerTags={hiddenLayerTags}
            setHiddenLayerTags={setHiddenLayerTags}
            onToggleTagOnElement={handleToggleTagOnElement}
            onCreateTag={onCreateTag}
            onHoverLayer={onHoverLayer}
            onCreateLayer={onCreateLayer}
            onUpdateLayer={onUpdateLayer}
            onDeleteLayer={onDeleteLayer}
            tagCounts={tagCounts}
            layerCounts={layerCounts}
          />
        </Box>
      </VStack>
    </SlidingPanel>
  )
}

export default memo(ViewExplorer)
