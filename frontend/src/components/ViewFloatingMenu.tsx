import React, { memo } from 'react'
import type { ViewFloatingMenuSlots } from '../slots'

import {
  HStack, Tooltip, Button, Box, Text, Popover, PopoverTrigger, Portal, PopoverContent, PopoverBody, IconButton, useDisclosure
} from '@chakra-ui/react'
import { DownloadIcon } from '@chakra-ui/icons'
import {
  AddElementIcon as AddElementSvg,
  EditIcon as PencilSvg,
  EyeIcon as EyeSvg,
  EyeOffIcon as EyeOffSvg,
  ImportIcon,
  ExpandExtrasIcon as ExpandExtrasSvg,
  CollapseExtrasIcon as CollapseExtrasSvg,
  FocusIcon as FocusSvg,
  TagsIcon,
} from './Icons'
import { KbdHint } from './PanelUI'
import { useViewEditorContext } from '../pages/ViewEditor/context'
import type { Tag, ViewLayer } from '../types'

export interface ViewFloatingMenuProps extends ViewFloatingMenuSlots {
  handleAddElementAtCenter: () => void
  drawingMode: boolean
  setDrawingMode: React.Dispatch<React.SetStateAction<boolean>>
  hasDrawingPaths: boolean
  drawingVisible: boolean
  setDrawingVisible: React.Dispatch<React.SetStateAction<boolean>>
  extrasOpen: boolean
  setExtrasOpen: React.Dispatch<React.SetStateAction<boolean>>
  disableImportExport?: boolean
  onImport: () => void
  onExport: () => void
  onShare?: () => void
  focusMode: boolean
  onFocusModeChange: (enabled: boolean) => void

  // Tag-related props
  allTags: string[]
  layers: ViewLayer[]
  tagColors: Record<string, Tag>
  hiddenTags: string[]
  toggleTagVisibility: (tag: string) => void
  toggleLayerVisibility: (layer: ViewLayer) => void
  tagCounts: Record<string, number>
  layerElementCounts: Record<number, number>
  setHighlightedTags: (tags: string[] | null) => void
  setHighlightColor: (color: string | null) => void

  toolbarSlot?: React.ReactNode
  hideFocusView?: boolean
  hideExpandExtras?: boolean
}

/**
 * Name: Floating Menu
 * Role: Shows add element, draw, export, import, and share buttons with a collapsible section.
 * Location: Floating at the bottom center of the editor.
 * Aliases: Bottom bar, Action bar.
 */
function ViewFloatingMenu({
  handleAddElementAtCenter,
  drawingMode,
  setDrawingMode,
  hasDrawingPaths,
  drawingVisible,
  setDrawingVisible,
  extrasOpen,
  setExtrasOpen,
  disableImportExport = false,
  onImport,
  onExport,
  focusMode,
  onFocusModeChange,
  allTags,
  layers,
  tagColors,
  hiddenTags,
  toggleTagVisibility,
  toggleLayerVisibility,
  tagCounts,
  layerElementCounts,
  setHighlightedTags,
  setHighlightColor,
  shareSlot,
  toolbarSlot,
  hideFocusView = false,
  hideExpandExtras = false,
}: ViewFloatingMenuProps) {
  const { canEdit } = useViewEditorContext()
  const { isOpen: isTagsOpen, onClose: onTagsClose, onToggle: onTagsToggle } = useDisclosure()

  return (
    <HStack
      position="absolute"
      left="50%"
      transform="translateX(-50%)"
      zIndex={20}
      spacing={0}
      bg="var(--bg-panel)"
      border="1px solid"
      borderColor="whiteAlpha.100"
      rounded="xl"
      boxShadow="0 8px 32px rgba(0,0,0,0.5)"
      backdropFilter="blur(20px)"
      px={1.5}
      py={1.5}
      pointerEvents="auto"
      style={{ bottom: 'calc(1.25rem + env(safe-area-inset-bottom, 0px))' } as React.CSSProperties}
      transition="all 0.2s cubic-bezier(0.4, 0, 0.2, 1)"
    >
      <Tooltip label="Create new element (C)" placement="top" openDelay={200}>
        <Button
          variant="ghost"
          h="28px"
          px={2.5}
          color="var(--accent)"
          isDisabled={!canEdit}
          _disabled={{ opacity: 0.35, cursor: 'not-allowed' }}
          _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
          onClick={() => handleAddElementAtCenter()}
        >
          <HStack spacing={1.5}>
            <AddElementSvg />
            <Text fontSize="11px" fontWeight="semibold">
              Add Element
            </Text>
            <KbdHint>C</KbdHint>
          </HStack>
        </Button>
      </Tooltip>

      {!hideFocusView && (
        <>
          <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />
          <Tooltip label={focusMode ? 'Show context' : 'Focus on this view'} placement="top" openDelay={200}>
            <Button
              variant="ghost"
              h="28px"
              px={2.5}
              color={focusMode ? 'var(--accent)' : 'gray.300'}
              bg={focusMode ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
              _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
              onClick={() => onFocusModeChange(!focusMode)}
            >
              <HStack spacing={1.5}>
                <FocusSvg />
                <Text fontSize="11px" fontWeight="normal">Focus View</Text>
                <Box w="6px" h="6px" rounded="full" bg={focusMode ? 'var(--accent)' : 'gray.500'} />
              </HStack>
            </Button>
          </Tooltip>
        </>
      )}
      <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />

      {(allTags.length > 0 || layers.length > 0) && (
        <>
          <Popover
            isOpen={isTagsOpen}
            onClose={() => { onTagsClose(); setHighlightedTags(null); setHighlightColor(null) }}
            placement="top"
            isLazy
            closeOnBlur
          >
            <PopoverTrigger>
              <Button
                variant="ghost" h="28px" px={2.5}
                color={isTagsOpen ? 'var(--accent)' : 'gray.300'}
                _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
                onClick={onTagsToggle}
              >
                <HStack spacing={1.5}>
                  <TagsIcon />
                  <Text fontSize="11px" fontWeight="normal">Tags</Text>
                </HStack>
              </Button>
            </PopoverTrigger>
            <Portal>
              <PopoverContent
                bg="var(--bg-panel)"
                backdropFilter="blur(20px)"
                borderColor="whiteAlpha.100"
                boxShadow="0 8px 32px rgba(0,0,0,0.5)"
                borderRadius="lg"
                width="220px"
                _focus={{ boxShadow: 'none' }}
                onMouseLeave={() => { setHighlightedTags(null); setHighlightColor(null) }}
              >
                <PopoverBody p={2} maxH="360px" overflowY="auto">
                  {layers.map(layer => {
                    const isHidden = layer.tags.length > 0 && layer.tags.every(t => hiddenTags.includes(t))
                    return (
                      <HStack
                        key={`layer-${layer.id}`}
                        px={2}
                        py={1}
                        spacing={2}
                        borderRadius="md"
                        _hover={{ bg: 'whiteAlpha.100' }}
                        onMouseEnter={() => { setHighlightedTags(layer.tags); setHighlightColor(layer.color || '') }}
                        opacity={isHidden ? 0.4 : 1}
                        transition="opacity 0.15s"
                      >
                        <Box w="10px" h="10px" rounded="full" bg={layer.color || 'gray.500'} flexShrink={0} />
                        <Text fontSize="xs" fontWeight="600" color="white" flex={1} isTruncated>
                          {layer.name}
                        </Text>
                        <Text fontSize="10px" color="gray.600" flexShrink={0}>
                          {layerElementCounts[layer.id] ?? 0}
                        </Text>
                        <IconButton
                          aria-label={isHidden ? 'Show layer' : 'Hide layer'}
                          icon={isHidden ? <EyeOffSvg size={12} /> : <EyeSvg size={12} />}
                          size="xs"
                          variant="ghost"
                          color={isHidden ? 'whiteAlpha.300' : 'whiteAlpha.600'}
                          _hover={{ color: 'white', bg: 'whiteAlpha.200' }}
                          onClick={(e) => { e.stopPropagation(); toggleLayerVisibility(layer) }}
                          flexShrink={0}
                        />
                      </HStack>
                    )
                  })}

                  {allTags.map(tag => {
                    const isHidden = hiddenTags.includes(tag)
                    return (
                      <HStack
                        key={`tag-${tag}`}
                        px={2}
                        py={1}
                        spacing={2}
                        borderRadius="md"
                        onMouseEnter={() => { setHighlightedTags([tag]); setHighlightColor(tagColors[tag]?.color || '') }}
                        opacity={isHidden ? 0.4 : 1}
                        transition="opacity 0.15s"
                      >
                        <Box w="8px" h="8px" rounded="full" bg={tagColors[tag]?.color || '#A0AEC0'} flexShrink={0} />
                        <Text fontSize="xs" fontWeight="600" color="gray.300" flex={1} isTruncated>
                          {tag}
                        </Text>
                        <Text fontSize="10px" color="gray.600" flexShrink={0}>
                          {tagCounts[tag] ?? 0}
                        </Text>
                        <IconButton
                          aria-label={isHidden ? 'Show tag' : 'Hide tag'}
                          icon={isHidden ? <EyeOffSvg size={12} /> : <EyeSvg size={12} />}
                          size="xs"
                          variant="ghost"
                          color={isHidden ? 'whiteAlpha.300' : 'whiteAlpha.600'}
                          _hover={{ color: 'white', bg: 'whiteAlpha.200' }}
                          onClick={(e) => { e.stopPropagation(); toggleTagVisibility(tag) }}
                          flexShrink={0}
                        />
                      </HStack>
                    )
                  })}
                </PopoverBody>
              </PopoverContent>
            </Portal>
          </Popover>
          <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />
        </>
      )}

      {/* Draw mode toggle */}
      <Tooltip
        label={drawingMode ? 'Exit drawing mode' : 'Draw on diagram'}
        placement="top"
        openDelay={200}
      >
        <Button
          variant="ghost"
          h="28px"
          px={2.5}
          color={drawingMode ? 'var(--accent)' : 'gray.300'}
          bg={drawingMode ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
          _hover={{ bg: drawingMode ? 'rgba(var(--accent-rgb), 0.18)' : 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
          onClick={() => setDrawingMode((v) => !v)}
          aria-label={drawingMode ? 'Exit drawing mode' : 'Enter drawing mode'}
        >
          <HStack spacing={1.5}>
            <PencilSvg />
            <Text fontSize="11px" fontWeight="normal">Draw</Text>
          </HStack>
        </Button>
      </Tooltip>

      {/* Drawing layer visibility toggle - only shown when there are strokes */}
      {hasDrawingPaths && (
        <>
          <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />
          <Tooltip
            label={drawingVisible ? 'Hide Drawings' : 'Show Drawings'}
            placement="top"
            openDelay={200}
          >
            <Button
              variant="ghost"
              h="28px"
              minW="28px"
              px={2}
              color={drawingVisible ? 'gray.300' : 'gray.600'}
              _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
              onClick={() => setDrawingVisible((v) => !v)}
              aria-label={drawingVisible ? 'Hide Drawings' : 'Show Drawings'}
            >
              {drawingVisible ? (
                <HStack spacing={1.5}>
                  <EyeSvg />
                  <Text fontSize="11px" fontWeight="normal">Hide Drawings</Text>
                </HStack>
              ) : (
                <HStack spacing={1.5}>
                  <EyeOffSvg />
                  <Text fontSize="11px" fontWeight="normal">Show Drawings</Text>
                </HStack>
              )}
            </Button>
          </Tooltip>
        </>
      )}

      {extrasOpen && (
        <>
          <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />
          <HStack spacing={1} pl={1} pr={0.5}>

            <Button
              variant="ghost"
              h="28px"
              px={2.5}
              color="gray.300"
              leftIcon={<ImportIcon />}
              isDisabled={disableImportExport}
              _disabled={{ opacity: 0.35, cursor: 'not-allowed' }}
              _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
              onClick={onImport}
            >
              <Text fontSize="11px" fontWeight="normal">Import</Text>
            </Button>

            <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />

            <Button
              variant="ghost"
              h="28px"
              px={2.5}
              color="gray.300"
              leftIcon={<DownloadIcon />}
              isDisabled={disableImportExport}
              _disabled={{ opacity: 0.35, cursor: 'not-allowed' }}
              _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
              onClick={onExport}
            >
              <Text fontSize="11px" fontWeight="normal">Export</Text>
            </Button>

            {shareSlot}
            {toolbarSlot}
          </HStack>
        </>
      )}

      {!hideExpandExtras && (
        <>
          <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />
          <Button
            variant="ghost"
            h="28px"
            minW="36px"
            px={2}
            display="inline-flex"
            alignItems="center"
            justifyContent="center"
            color="gray.300"
            _hover={{ bg: 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
            onClick={() => setExtrasOpen((prev) => !prev)}
            aria-label={extrasOpen ? 'Collapse extras' : 'Expand extras'}
          >
            {extrasOpen ? <CollapseExtrasSvg /> : <ExpandExtrasSvg />}
          </Button>
        </>
      )}
    </HStack>
  )
}

export default memo(ViewFloatingMenu)
