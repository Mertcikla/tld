import React, { useState } from 'react'
import { HStack, Box, Popover, PopoverTrigger, PopoverContent, useDisclosure, IconButton, VStack, Text, Input, Tooltip } from '@chakra-ui/react'
import { SmallCloseIcon } from '@chakra-ui/icons'
import { EyeIcon, EyeOffIcon } from '../../Icons'
import { GroupNamingPopover } from './GroupNamingPopover'
import { ColorPicker } from './ColorPicker'

interface Props {
  tag: string
  color: string
  isAssigned?: boolean
  tagCount?: number
  onToggle?: () => void
  onHover?: (active: boolean) => void
  onDropTag?: (draggedTag: string) => void
  onDropLayer?: (layerId: number) => void
  namingPopover?: { isOpen: boolean; defaultName: string }
  onConfirmNaming?: (name: string) => void
  onCloseNaming?: () => void
  onSetColor?: (color: string) => void
  onSetDescription?: (description: string) => void
  onRemove?: () => void
  description?: string | null
  isVisible?: boolean
  onToggleVisibility?: () => void
}

export const TagItem: React.FC<Props> = ({
  tag,
  color,
  isAssigned,
  tagCount = 0,
  onToggle,
  onHover,
  onDropTag,
  onDropLayer,
  namingPopover,
  onConfirmNaming,
  onCloseNaming,
  onSetColor,
  onSetDescription,
  onRemove,
  description,
  isVisible = true,
  onToggleVisibility,
}) => {
  const [isOver, setIsOver] = useState(false)
  const { isOpen: isColorOpen, onOpen: onColorOpen, onClose: onColorClose } = useDisclosure()

  const handleDragStart = (e: React.DragEvent) => {
    e.dataTransfer.setData('application/diag-tag', tag)
    e.dataTransfer.effectAllowed = 'copyMove'
  }

  const handleDragOver = (e: React.DragEvent) => {
    if (e.dataTransfer.types.includes('application/diag-tag') || e.dataTransfer.types.includes('application/diag-layer')) {
      const draggedTag = e.dataTransfer.getData('application/diag-tag')
      if (draggedTag !== tag) {
        e.preventDefault()
        e.dataTransfer.dropEffect = 'link'
        setIsOver(true)
      }
    }
  }

  const handleDragLeave = () => {
    setIsOver(false)
  }

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setIsOver(false)
    const draggedTag = e.dataTransfer.getData('application/diag-tag')
    const draggedLayerId = e.dataTransfer.getData('application/diag-layer')
    
    if (draggedTag && draggedTag !== tag) {
      onDropTag?.(draggedTag)
    } else if (draggedLayerId) {
      onDropLayer?.(Number(draggedLayerId))
    }
  }

  return (
    <Popover
      isOpen={!!namingPopover?.isOpen}
      onClose={onCloseNaming || (() => {})}
      placement="top"
      isLazy
      closeOnBlur
    >
      <PopoverTrigger>
        <HStack
          spacing={0}
          align="center"
          px={2}
          overflow="hidden"
          rounded="md"
          border="1px solid"
          borderColor={isAssigned ? color + '55' : isOver ? 'var(--accent)' : 'whiteAlpha.100'}
          bg={isOver ? 'whiteAlpha.100' : 'transparent'}
          transition="all 0.15s"
          onMouseEnter={() => onHover?.(true)}
          onMouseLeave={() => onHover?.(false)}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
          cursor="grab"
          _active={{ cursor: 'grabbing' }}
          role="group"
          opacity={isVisible ? 1 : 0.6}
        >
          {onToggleVisibility && (
            <IconButton
              aria-label={isVisible ? 'Hide Tagged Elements' : 'Show Tagged Elements'}
              icon={isVisible ? <EyeIcon size={10} /> : <EyeOffIcon size={10} />}
              size="xs"
              variant="ghost"
              color={isVisible ? 'whiteAlpha.600' : 'whiteAlpha.300'}
              _hover={{ color: isVisible ? 'white' : 'whiteAlpha.600', bg: 'transparent' }}
              onClick={(e) => {
                e.stopPropagation()
                onToggleVisibility()
              }}
              w="14px"
              h="14px"
              minW="auto"
              p={0}
              mr={1}
              flexShrink={0}
            />
          )}
          {onSetColor && (
            <Popover
              isOpen={isColorOpen}
              onClose={onColorClose}
              placement="top"
              closeOnBlur
            >
              <PopoverTrigger>
                <Box
                  w="8px"
                  h="8px"
                  rounded="full"
                  bg={color}
                  cursor="pointer"
                  _hover={{ transform: 'scale(1.2)' }}
                  onClick={(e) => {
                    e.stopPropagation()
                    onColorOpen()
                  }}
                  flexShrink={0}
                  transition="transform 0.1s"
                />
              </PopoverTrigger>
              <PopoverContent bg="gray.800" borderColor="whiteAlpha.200" p={2} w="200px" shadow="xl">
                <VStack spacing={3} align="stretch">
                  <ColorPicker onSelect={onSetColor} onClose={onColorClose} />
                  {onSetDescription && (
                    <Box pt={1}>
                      <Text fontSize="10px" fontWeight="bold" color="gray.500" mb={1} textTransform="uppercase">Description</Text>
                      <Input
                        size="xs"
                        placeholder="Tag description..."
                        defaultValue={description || ''}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            onSetDescription((e.target as HTMLInputElement).value)
                            onColorClose()
                          }
                        }}
                        bg="whiteAlpha.50"
                        borderColor="whiteAlpha.100"
                        _focus={{ borderColor: 'var(--accent)' }}
                      />
                    </Box>
                  )}
                </VStack>
              </PopoverContent>
            </Popover>
          )}
          <Tooltip label={description} placement="top" openDelay={500} isDisabled={!description}>
            <Box
              as="button"
              pl={onSetColor ? 2 : 0}
              pr={onRemove ? 1 : 0}
              py={1}
              fontSize="11px"
              fontWeight="600"
              bg="transparent"
              color={isAssigned ? color : 'gray.500'}
              _hover={{ color: color }}
              transition="all 0.1s"
              onClick={onToggle}
              draggable
              onDragStart={handleDragStart}
              width="full"
              textAlign="left"
            >
              <Text isTruncated>
                {tag}
                {tagCount > 0 && (
                  <Box as="span" ml={1.5} opacity={0.6} fontWeight="normal">
                    {tagCount}
                  </Box>
                )}
              </Text>
            </Box>
          </Tooltip>
          {onRemove && (
            <IconButton
              aria-label="Remove tag"
              icon={<SmallCloseIcon />}
              size="xs"
              variant="ghost"
              fontSize="10px"
              w="14px"
              h="14px"
              minW="auto"
              p={0}
              color="gray.500"
              opacity={0}
              _groupHover={{ opacity: 0.6 }}
              _hover={{ opacity: 1, color: 'red.400' }}
              onClick={(e) => {
                e.stopPropagation()
                onRemove()
              }}
            />
          )}
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
  )
}
