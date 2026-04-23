import React from 'react'
import { Box, Button, Divider, HStack, Text, VStack } from '@chakra-ui/react'
import {
  AddElementIcon as AddElementSvg,
  TrashIcon as TrashSvg,
  EditIcon as PencilSvg,
  MoveSourceIcon as MoveSourceSvg,
  MoveTargetIcon as MoveTargetSvg,
  GridIcon as GridSvg,
} from '../../../components/Icons'
import { useViewEditorContext } from '../context'

const KbdHint = ({ children }: { children: string }) => (
  <Box as="span" display="inline-flex" alignItems="center" justifyContent="center"
    px={1.5} py={0.5} bg="whiteAlpha.300" rounded="sm" fontSize="8px"
    fontWeight="bold" color="whiteAlpha.900" flexShrink={0}>
    {children}
  </Box>
)

interface ConnectorContextMenuProps {
  menu: { edgeId: number; x: number; y: number } | null
  onEdit: (edgeId: number) => void
  onMoveSource: (edgeId: number) => void
  onMoveTarget: (edgeId: number) => void
  onDelete: (edgeId: number) => void
}

export const ConnectorContextMenu: React.FC<ConnectorContextMenuProps> = ({
  menu,
  onEdit,
  onMoveSource,
  onMoveTarget,
  onDelete,
}) => {
  const { canEdit } = useViewEditorContext()
  if (!menu) return null

  return (
    <Box position="absolute" left={`${menu.x}px`} top={`${menu.y}px`}
      transform="translate(-50%, calc(-100% - 8px))" zIndex={1000} bg="var(--bg-panel)"
      border="1px solid" borderColor="whiteAlpha.100" rounded="xl" boxShadow="0 8px 32px rgba(0,0,0,0.5)"
      backdropFilter="blur(20px)" p={1.5} minW="192px"
      onClick={(e) => e.stopPropagation()} onPointerDown={(e) => e.stopPropagation()}>
      <VStack spacing={0} align="stretch">
        <Button size="sm" variant="ghost" h="30px" px={2.5} justifyContent="flex-start" color="gray.200" _hover={{ bg: 'whiteAlpha.100' }}
          onClick={() => onEdit(menu.edgeId)}>
          <HStack spacing={2} w="full"><PencilSvg /><Text fontSize="xs" fontWeight="normal" flex={1}>Edit Connector</Text></HStack>
        </Button>
        {canEdit && (
          <>
            <Button size="sm" variant="ghost" h="30px" px={2.5} justifyContent="flex-start" color="gray.200" _hover={{ bg: 'whiteAlpha.100' }}
              onClick={() => onMoveSource(menu.edgeId)}>
              <HStack spacing={2} w="full"><MoveSourceSvg /><Text fontSize="xs" fontWeight="normal" flex={1}>Move Source</Text></HStack>
            </Button>
            <Button size="sm" variant="ghost" h="30px" px={2.5} justifyContent="flex-start" color="gray.200" _hover={{ bg: 'whiteAlpha.100' }}
              onClick={() => onMoveTarget(menu.edgeId)}>
              <HStack spacing={2} w="full"><MoveTargetSvg /><Text fontSize="xs" fontWeight="normal" flex={1}>Move Target</Text></HStack>
            </Button>
            <Divider borderColor="whiteAlpha.100" my={1} />
            <Button size="sm" variant="ghost" h="30px" px={2.5} justifyContent="flex-start" color="red.400" _hover={{ bg: 'rgba(254,178,178,0.08)', color: 'red.300' }}
              onClick={() => onDelete(menu.edgeId)}>
              <HStack spacing={2} w="full"><TrashSvg /><Text fontSize="xs" fontWeight="normal" flex={1}>Delete</Text></HStack>
            </Button>
          </>
        )}
        {!canEdit && <Divider borderColor="whiteAlpha.100" my={0} />}
      </VStack>
    </Box>
  )
}

interface CanvasContextMenuProps {
  menu: { x: number; y: number; flowX: number; flowY: number } | null
  onAddElement: (x: number, y: number) => void
}

export const CanvasContextMenu: React.FC<CanvasContextMenuProps> = ({ menu, onAddElement }) => {
  const { canEdit, snapToGrid, setSnapToGrid } = useViewEditorContext()
  if (!menu) return null

  return (
    <Box position="absolute" left={`${menu.x}px`} top={`${menu.y}px`}
      transform="translate(-50%, calc(-100% - 6px))" zIndex={1000} bg="var(--bg-panel)"
      border="1px solid" borderColor="whiteAlpha.100" rounded="xl" boxShadow="0 8px 32px rgba(0,0,0,0.5)"
      backdropFilter="blur(20px)" p={1.5} minW="180px"
      onClick={(e) => e.stopPropagation()} onPointerDown={(e) => e.stopPropagation()}>
      <VStack spacing={0} align="stretch">
        <Button size="sm" variant="ghost" h="30px" px={2.5} justifyContent="flex-start"
          color={canEdit ? 'var(--accent)' : 'gray.500'} _hover={{ bg: 'whiteAlpha.100', color: 'var(--accent)' }}
          _disabled={{ opacity: 0.4, cursor: 'not-allowed' }} isDisabled={!canEdit}
          onClick={() => onAddElement(menu.x, menu.y)}>
          <HStack spacing={2} w="full">
            <AddElementSvg />
            <Text fontSize="xs" fontWeight="normal" flex={1}>Add Element</Text>
            <KbdHint>C</KbdHint>
          </HStack>
        </Button>
        <Divider borderColor="whiteAlpha.100" my={1} />
        <Button size="sm" variant="ghost" h="30px" px={2.5} justifyContent="flex-start"
          color="clay.text" _hover={{ bg: 'whiteAlpha.100' }}
          onClick={() => setSnapToGrid(!snapToGrid)}>
          <HStack spacing={2} w="full">
            <GridSvg />
            <Text fontSize="xs" fontWeight="normal" flex={1}>Snap to Grid</Text>
            {snapToGrid && <Box w="6px" h="6px" rounded="full" bg="var(--accent)" />}
          </HStack>
        </Button>
      </VStack>
    </Box>
  )
}
