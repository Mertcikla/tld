import React, { useState } from 'react'
import {
  Box,
  HStack,
  Text,
  VStack,
  IconButton,
  Wrap,
  WrapItem,
  Popover,
  PopoverTrigger,
  useDisclosure,
  Flex,
} from '@chakra-ui/react'
import { GroupNamingPopover } from './GroupNamingPopover'
import { DeleteIcon } from '@chakra-ui/icons'
import { EyeIcon, EyeOffIcon, ChevronDownIcon } from '../../Icons'
import { ViewLayer } from '../../../types'
import { TagItem } from './TagItem'
import { ColorPicker } from './ColorPicker'

interface Props {
  layer: ViewLayer
  isActive: boolean
  isExpanded: boolean
  tagCount: number
  tagColors: Record<string, import('../../../types').Tag>
  onToggleActive: () => void
  onToggleExpanded: () => void
  onDelete: () => void
  onSetColor: (color: string) => void
  onSetTagColor?: (tag: string, color: string, description?: string) => void
  onAddTag: (tag: string) => void
  onRemoveTag: (tag: string) => void
  onHover: (active: boolean) => void
  selectedElementTags?: string[]
  onToggleTagOnElement?: (tag: string) => void
  namingPopover?: { isOpen: boolean; defaultName: string }
  onConfirmNaming?: (name: string) => void
  onCloseNaming?: () => void
}

export const LayerItem: React.FC<Props> = ({
  layer,
  isActive,
  isExpanded,
  tagCount,
  tagColors,
  onToggleActive,
  onToggleExpanded,
  onDelete,
  onSetColor,
  onSetTagColor,
  onAddTag,
  onRemoveTag,
  onHover,
  selectedElementTags,
  onToggleTagOnElement,
  namingPopover,
  onConfirmNaming,
  onCloseNaming,
}) => {
  const [isOver, setIsOver] = useState(false)
  const { isOpen: isColorOpen, onOpen: onColorOpen, onClose: onColorClose } = useDisclosure()

  const handleDragOver = (e: React.DragEvent) => {
    if (e.dataTransfer.types.includes('application/diag-tag')) {
      e.preventDefault()
      e.dataTransfer.dropEffect = 'copy'
      setIsOver(true)
    }
  }

  const handleDragLeave = () => {
    setIsOver(false)
  }

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setIsOver(false)
    const tag = e.dataTransfer.getData('application/diag-tag')
    if (tag && !layer.tags.includes(tag)) {
      onAddTag(tag)
    }
  }

  const handleDragStart = (e: React.DragEvent) => {
    e.dataTransfer.setData('application/diag-layer', String(layer.id))
    e.dataTransfer.effectAllowed = 'copyMove'
  }

  return (
    <Box
      data-testid="tag-manager-layer"
      data-layer-id={layer.id}
      data-layer-name={layer.name}
      borderBottom="1px solid"
      borderColor="whiteAlpha.100"
      bg={isOver ? 'whiteAlpha.100' : 'transparent'}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      draggable
      onDragStart={handleDragStart}
      transition="background-color 0.2s"
    >
      <Popover
        isOpen={!!namingPopover?.isOpen}
        onClose={onCloseNaming || (() => {})}
        placement="top"
        isLazy
        closeOnBlur
      >
        <PopoverTrigger>
          <HStack
            px={3}
            py={2}
            spacing={2}
            _hover={{ bg: 'whiteAlpha.50' }}
            cursor="pointer"
            onClick={onToggleExpanded}
            onMouseEnter={() => onHover(true)}
            onMouseLeave={() => onHover(false)}
          >
            <IconButton
              data-testid="tag-manager-layer-visibility"
              aria-label={isActive ? 'Hide Layer' : 'Show Layer'}
              icon={isActive ? <EyeIcon size={14} /> : <EyeOffIcon size={14} />}
              size="xs"
              variant="ghost"
              color={isActive ? 'whiteAlpha.800' : 'whiteAlpha.400'}
              onClick={(e) => {
                e.stopPropagation()
                onToggleActive()
              }}
            />
            
            <Popover
              isOpen={isColorOpen}
              onClose={onColorClose}
              placement="right"
              closeOnBlur
            >
              <PopoverTrigger>
                <Box
                  w="10px"
                  h="10px"
                  rounded="full"
                  bg={layer.color || 'gray.500'}
                  flexShrink={0}
                  _hover={{ transform: 'scale(1.2)' }}
                  transition="transform 0.1s"
                  onClick={(e) => {
                    e.stopPropagation()
                    onColorOpen()
                  }}
                />
              </PopoverTrigger>
              <ColorPicker onSelect={onSetColor} onClose={onColorClose} />
            </Popover>

            <VStack align="start" spacing={0} flex={1} minW={0}>
              <Text fontSize="xs" fontWeight="600" color="white" isTruncated>
                {layer.name}
              </Text>
              <Text fontSize="10px" color="gray.500">
                {layer.tags.length} tags · {tagCount} elements
              </Text>
            </VStack>

            <Box
              color="whiteAlpha.400"
              transform={isExpanded ? 'none' : 'rotate(-90deg)'}
              transition="transform 0.15s"
            >
              <ChevronDownIcon size={12} />
            </Box>
          </HStack>
        </PopoverTrigger>
        {namingPopover && onConfirmNaming && (
          <GroupNamingPopover
            isOpen={namingPopover.isOpen}
            onClose={onCloseNaming || (() => {})}
            onConfirm={onConfirmNaming}
            defaultName={namingPopover.defaultName}
          />
        )}
      </Popover>

      {isExpanded && (
        <Box px={4} pb={3} pt={1}>
          <Flex align="flex-start" justify="space-between">
            <Wrap spacing={1.5} align="center" flex={1}>
              {layer.tags.map((tag) => (
                <WrapItem key={tag}>
                  <TagItem
                    tag={tag}
                    color={tagColors[tag]?.color || '#A0AEC0'}
                    description={tagColors[tag]?.description || null}
                    isAssigned={selectedElementTags?.includes(tag)}
                    onToggle={() => onToggleTagOnElement?.(tag)}
                    onConfirmNaming={onConfirmNaming}
                    onCloseNaming={onCloseNaming}
                    onHover={(active) => onHover(active)}
                    onSetColor={onSetTagColor ? (color) => onSetTagColor(tag, color) : undefined}
                    onSetDescription={onSetTagColor ? (desc) => onSetTagColor(tag, tagColors[tag]?.color, desc) : undefined}
                    onRemove={() => onRemoveTag(tag)}
                  />
                </WrapItem>
              ))}
            </Wrap>
            <IconButton
              data-testid="tag-manager-layer-delete"
              aria-label="Delete Layer"
              icon={<DeleteIcon />}
              size="xs"
              variant="ghost"
              colorScheme="red"
              opacity={0.4}
              _hover={{ opacity: 1 }}
              onClick={(e) => {
                e.stopPropagation()
                onDelete()
              }}
              ml={2}
              flexShrink={0}
            />
          </Flex>
        </Box>
      )}
    </Box>
  )
}
