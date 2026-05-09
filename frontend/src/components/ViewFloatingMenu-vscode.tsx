import React from 'react'
import {
  HStack, Tooltip, Button, Box, Text,
  Menu, MenuButton, MenuList, MenuOptionGroup, MenuItemOption, MenuDivider, MenuItem
} from '@chakra-ui/react'
import { AddElementIcon as AddElementSvg } from './Icons'
import { KbdHint } from './PanelUI'
// Inline the props interface to avoid circular dependency with the web variant
// (vite.vscode.config.ts overrides ViewFloatingMenu.tsx → this file)
interface ViewFloatingMenuProps {
  canEdit: boolean
  handleAddElementAtCenter: () => void
  drawingMode: boolean
  setDrawingMode: React.Dispatch<React.SetStateAction<boolean>>
  hasDrawingPaths: boolean
  drawingVisible: boolean
  setDrawingVisible: React.Dispatch<React.SetStateAction<boolean>>
  extrasOpen: boolean
  setExtrasOpen: React.Dispatch<React.SetStateAction<boolean>>
  onImport: () => void
  onExport: () => void
  onShare: () => void
  isFreePlan: boolean
  canUpgrade?: boolean
  activeTags?: string[]
  setActiveTags?: (tags: string[]) => void
  availableTags?: string[]
}

function LayerIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <polygon points="12 2 2 7 12 12 22 7 12 2" />
      <polyline points="2 17 12 22 22 17" />
      <polyline points="2 12 12 17 22 12" />
    </svg>
  )
}

function PencilSvg() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z" />
    </svg>
  )
}
function EyeSvg() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}
function EyeOffSvg() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
      <line x1="1" y1="1" x2="23" y2="23" />
    </svg>
  )
}

/**
 * VS Code variant of ViewFloatingMenu.
 * Import, Export, and Share are removed they are registered as VS Code commands instead.
 * The extras toggle chevron is also removed since there are no extras to expand.
 */
export default function ViewFloatingMenu({
  canEdit,
  handleAddElementAtCenter,
  drawingMode,
  setDrawingMode,
  hasDrawingPaths,
  drawingVisible,
  setDrawingVisible,
  activeTags = [],
  setActiveTags,
  availableTags = [],
}: ViewFloatingMenuProps) {
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

      <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />

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

      <Box w="1px" h="16px" bg="whiteAlpha.100" flexShrink={0} mx={0.5} />

      {/* Layers/Tags menu */}
      <Menu closeOnSelect={false} placement="top">
        <Tooltip label="Filter elements by tag" placement="top" openDelay={200}>
          <MenuButton
            as={Button}
            variant="ghost"
            h="28px"
            px={2.5}
            color={activeTags.length > 0 ? 'var(--accent)' : 'gray.300'}
            bg={activeTags.length > 0 ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
            _hover={{ bg: activeTags.length > 0 ? 'rgba(var(--accent-rgb), 0.18)' : 'rgba(var(--accent-rgb), 0.12)', color: 'var(--accent)' }}
          >
            <HStack spacing={1.5}>
              <LayerIcon />
              <Text fontSize="11px" fontWeight="normal">Layers</Text>
              {activeTags.length > 0 && (
                <Box bg="var(--accent)" color="white" rounded="full" px={1} minW="14px" fontSize="9px" fontWeight="bold">
                  {activeTags.length}
                </Box>
              )}
            </HStack>
          </MenuButton>
        </Tooltip>
        <MenuList bg="var(--bg-panel)" borderColor="whiteAlpha.100" shadow="0 8px 32px rgba(0,0,0,0.5)" rounded="xl" py={2} maxH="300px" overflowY="auto">
          {availableTags.length === 0 ? (
            <Box px={4} py={2}>
              <Text fontSize="xs" color="gray.500">No tags in workspace</Text>
            </Box>
          ) : (
            <MenuOptionGroup
              title="Filter by Tags"
              type="checkbox"
              value={activeTags}
              onChange={(val) => setActiveTags?.(val as string[])}
            >
              {availableTags.map((tag) => (
                <MenuItemOption
                  key={tag}
                  value={tag}
                  fontSize="xs"
                  _hover={{ bg: 'whiteAlpha.50' }}
                  _checked={{ color: 'var(--accent)' }}
                >
                  {tag}
                </MenuItemOption>
              ))}
            </MenuOptionGroup>
          )}
          {activeTags.length > 0 && (
            <>
              <MenuDivider borderColor="whiteAlpha.100" />
              <MenuItem
                fontSize="xs"
                color="red.400"
                _hover={{ bg: 'whiteAlpha.50' }}
                onClick={() => setActiveTags?.([])}
              >
                Clear all filters
              </MenuItem>
            </>
          )}
        </MenuList>
      </Menu>
    </HStack>
  )
}
