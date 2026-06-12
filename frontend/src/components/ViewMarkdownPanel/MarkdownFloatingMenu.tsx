import type { Dispatch, RefObject, SetStateAction, PointerEvent as ReactPointerEvent } from 'react'
import {
  Badge,
  Box,
  Button,
  HStack,
  IconButton,
  Menu,
  MenuButton,
  MenuItem,
  MenuList,
  Text,
  Tooltip,
} from '@chakra-ui/react'
import { ChevronDownIcon, DragHandleIcon, ViewIcon, ViewOffIcon } from '@chakra-ui/icons'
import type { ViewMarkdownDocument } from '../../types'
import { markdownStatusLabel } from './metadata'
import type { FloatingMenuPosition } from './types'

interface MarkdownFloatingMenuProps {
  canEdit: boolean
  canEditDocument: boolean
  currentEditorMarkdown: () => string
  floatingMenuPosition: FloatingMenuPosition
  floatingMenuRef: RefObject<HTMLDivElement>
  handleCopyPath: () => Promise<void> | void
  handleFloatingMenuDragStart: (event: ReactPointerEvent<HTMLButtonElement>) => void
  hasSaveConflict: boolean
  isDirty: boolean
  isFloatingMenuDragging: boolean
  isFloatingMenuHidden: boolean
  isLoading: boolean
  markdown: ViewMarkdownDocument | null
  onClose: () => void
  onForceSave?: (markdown: string) => Promise<void> | void
  onOpenInEditor?: () => void
  onReload?: () => Promise<void> | void
  onSave: (markdown: string) => Promise<void> | void
  onUnlinkMarkdown?: (options?: { deleteManagedFile: boolean }) => Promise<void> | void
  setIsFloatingMenuHidden: Dispatch<SetStateAction<boolean>>
}

export function MarkdownFloatingMenu({
  canEdit,
  canEditDocument,
  currentEditorMarkdown,
  floatingMenuPosition,
  floatingMenuRef,
  handleCopyPath,
  handleFloatingMenuDragStart,
  hasSaveConflict,
  isDirty,
  isFloatingMenuDragging,
  isFloatingMenuHidden,
  isLoading,
  markdown,
  onClose,
  onForceSave,
  onOpenInEditor,
  onReload,
  onSave,
  onUnlinkMarkdown,
  setIsFloatingMenuHidden,
}: MarkdownFloatingMenuProps) {
  return (
    <Box
      ref={floatingMenuRef}
      data-testid="view-markdown-floating-menu"
      position="absolute"
      top={`${floatingMenuPosition.y}px`}
      left={floatingMenuPosition.x === null ? undefined : `${floatingMenuPosition.x}px`}
      right={floatingMenuPosition.x === null ? 3 : undefined}
      zIndex={30}
      pointerEvents="none"
      maxW="calc(100% - 16px)"
    >
      {isFloatingMenuHidden ? (
        <Tooltip label="Show notes menu" hasArrow openDelay={200}>
          <Button
            data-testid="view-markdown-menu-show"
            pointerEvents="auto"
            size="xs"
            leftIcon={<ViewIcon />}
            bg="rgba(2, 8, 23, 0.86)"
            color="whiteAlpha.900"
            border="1px solid"
            borderColor="whiteAlpha.200"
            boxShadow="0 12px 28px rgba(0,0,0,0.36)"
            backdropFilter="blur(18px)"
            _hover={{ bg: 'rgba(15, 23, 42, 0.94)' }}
            onClick={() => setIsFloatingMenuHidden(false)}
            onPointerDown={(event) => event.stopPropagation()}
          >
            Notes
          </Button>
        </Tooltip>
      ) : (
        <HStack
          data-testid="view-markdown-floating-menu-expanded"
          pointerEvents="auto"
          spacing={1}
          minW={0}
          maxW="100%"
          px={1.5}
          py={1}
          bg="rgba(2, 8, 23, 0.86)"
          border="1px solid"
          borderColor={isFloatingMenuDragging ? 'blue.300' : 'whiteAlpha.200'}
          rounded="lg"
          boxShadow="0 12px 28px rgba(0,0,0,0.36)"
          backdropFilter="blur(18px)"
          onPointerDown={(event) => event.stopPropagation()}
        >
          <Tooltip label="Move notes menu" hasArrow openDelay={200}>
            <IconButton
              data-testid="view-markdown-menu-drag-handle"
              aria-label="Move notes menu"
              size="xs"
              variant="ghost"
              icon={<DragHandleIcon />}
              color="whiteAlpha.700"
              cursor={isFloatingMenuDragging ? 'grabbing' : 'grab'}
              style={{ touchAction: 'none' }}
              _hover={{ bg: 'whiteAlpha.100', color: 'whiteAlpha.900' }}
              onPointerDown={handleFloatingMenuDragStart}
            />
          </Tooltip>
          <Box minW={0} maxW={{ base: '150px', md: '240px' }} px={1}>
            <HStack spacing={1.5} minW={0}>
              <Text fontSize="xs" fontWeight="semibold" color="whiteAlpha.900" flexShrink={0}>Notes</Text>
              <Badge data-testid="view-markdown-status" colorScheme={markdown?.exists === false ? 'red' : markdown?.can_edit === false ? 'yellow' : 'blue'} variant="subtle" maxW={{ base: '90px', md: '150px' }} overflow="hidden" textOverflow="ellipsis" whiteSpace="nowrap">
                {markdownStatusLabel(markdown)}
              </Badge>
            </HStack>
            {markdown?.path && (
              <Text fontSize="10px" color="gray.500" isTruncated title={markdown.repo_relative_path || markdown.path}>
                {markdown.repo_relative_path || markdown.path}
              </Text>
            )}
          </Box>
          {hasSaveConflict && markdown && (
            <HStack spacing={1} flexShrink={0}>
              <Button size="xs" variant="outline" onClick={() => { void onReload?.() }}>Reload</Button>
              <Button data-testid="view-markdown-overwrite" size="xs" colorScheme="blue" onClick={() => { void onForceSave?.(currentEditorMarkdown()) }}>
                Overwrite
              </Button>
            </HStack>
          )}
          <Menu placement="bottom-end">
            <MenuButton as={Button} size="xs" variant="ghost" rightIcon={<ChevronDownIcon boxSize={2.5} />} isDisabled={isLoading}>
              File
            </MenuButton>
            <MenuList bg="var(--bg-panel)" borderColor="whiteAlpha.200" fontSize="sm">
              <MenuItem bg="transparent" onClick={() => { void onReload?.() }} isDisabled={!markdown || isLoading}>Reload</MenuItem>
              <MenuItem bg="transparent" onClick={() => { void onSave(currentEditorMarkdown()) }} isDisabled={!canEditDocument || !markdown || !isDirty || isLoading}>Save</MenuItem>
              <MenuItem bg="transparent" onClick={() => { void onForceSave?.(currentEditorMarkdown()) }} isDisabled={!canEditDocument || !markdown || isLoading}>Overwrite from pane</MenuItem>
              <MenuItem bg="transparent" onClick={() => { void handleCopyPath() }} isDisabled={!markdown?.path}>Copy path</MenuItem>
              {onOpenInEditor && (
                <MenuItem bg="transparent" onClick={onOpenInEditor} isDisabled={!markdown}>Open in editor</MenuItem>
              )}
              <MenuItem data-testid="view-markdown-detach" bg="transparent" color="red.200" onClick={() => { void onUnlinkMarkdown?.({ deleteManagedFile: false }) }} isDisabled={!canEdit || !markdown || isLoading}>Detach from view</MenuItem>
              <MenuItem bg="transparent" onClick={onClose}>Close</MenuItem>
            </MenuList>
          </Menu>
          <Tooltip label="Hide notes menu" hasArrow openDelay={200}>
            <IconButton
              data-testid="view-markdown-menu-hide"
              aria-label="Hide notes menu"
              size="xs"
              variant="ghost"
              icon={<ViewOffIcon />}
              color="whiteAlpha.700"
              _hover={{ bg: 'whiteAlpha.100', color: 'whiteAlpha.900' }}
              onClick={() => setIsFloatingMenuHidden(true)}
            />
          </Tooltip>
        </HStack>
      )}
    </Box>
  )
}
