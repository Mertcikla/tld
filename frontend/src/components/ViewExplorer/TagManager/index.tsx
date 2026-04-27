import React, { useState } from 'react'
import {
  Box,
  VStack,
  HStack,
  Text,
  Wrap,
  WrapItem,
  IconButton,
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverBody,
  Input,
  Button,
  Divider,
  useDisclosure,
} from '@chakra-ui/react'
import { AddIcon } from '@chakra-ui/icons'
import ScrollIndicatorWrapper from '../../ScrollIndicatorWrapper'
import { ViewLayer, LibraryElement, PlacedElement } from '../../../types'
import { TagItem } from './TagItem'
import { LayerItem } from './LayerItem'
import { ColorPicker } from './ColorPicker'
import { pickUnusedColor } from '../utils'
import { ChevronDownIcon } from '../../Icons'

interface Props {
  availableTags: string[]
  tagColors: Record<string, import('../../../types').Tag>
  layers: ViewLayer[]
  viewElements: PlacedElement[]
  selectedElement: LibraryElement | null
  hiddenLayerTags: string[]
  setHiddenLayerTags: (tags: string[]) => void
  onToggleTagOnElement: (tag: string) => void
  onHoverLayer: (tags: string[] | null, color?: string | null) => void
  onCreateLayer: (name: string, tags: string[], color: string) => Promise<void>
  onUpdateLayer: (layer: ViewLayer) => Promise<void>
  onDeleteLayer: (id: number) => Promise<void>
  onCreateTag: (tag: string, color?: string, description?: string) => Promise<void>
  tagCounts: Record<string, number>
  layerCounts: Record<number, number>
}

export const TagManager: React.FC<Props> = ({
  availableTags,
  tagColors,
  layers,
  selectedElement,
  hiddenLayerTags,
  setHiddenLayerTags,
  onToggleTagOnElement,
  onHoverLayer,
  onCreateLayer,
  onCreateTag,
  onUpdateLayer,
  onDeleteLayer,
  tagCounts,
  layerCounts,
}) => {
  const [isCollapsed, setIsCollapsed] = useState(false)
  const [isUnusedCollapsed, setIsUnusedCollapsed] = useState(true)
  const [expandedLayerIds, setExpandedLayerIds] = useState<Set<number>>(new Set())
  const [namingPopover, setNamingPopover] = useState<{
    isOpen: boolean;
    tags: string[];
    defaultName: string;
    targetTag?: string | null;
    targetLayerId?: number | null;
  }>({
    isOpen: false,
    tags: [],
    defaultName: '',
  })

  const toggleLayerActive = (layer: ViewLayer) => {
    const isActive = layer.tags.length === 0 || !layer.tags.some((t) => hiddenLayerTags.includes(t))
    if (isActive) {
      setHiddenLayerTags(Array.from(new Set([...hiddenLayerTags, ...layer.tags])))
    } else {
      setHiddenLayerTags(hiddenLayerTags.filter((t) => !layer.tags.includes(t)))
    }
  }

  const toggleTagVisibility = (tag: string) => {
    if (hiddenLayerTags.includes(tag)) {
      setHiddenLayerTags(hiddenLayerTags.filter((t) => t !== tag))
    } else {
      setHiddenLayerTags([...hiddenLayerTags, tag])
    }
  }

  const toggleLayerExpanded = (id: number) => {
    setExpandedLayerIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const handleCreateGroup = (tagA: string, tagB: string, targetTag: string) => {
    setNamingPopover({
      isOpen: true,
      tags: [tagA, tagB],
      defaultName: `${tagA} & ${tagB}`,
      targetTag,
    })
  }

  const handleCreateGroupFromLayer = (targetTag: string, sourceLayerId: number) => {
    const layer = layers.find(l => l.id === sourceLayerId)
    if (!layer) return
    setNamingPopover({
      isOpen: true,
      tags: Array.from(new Set([...layer.tags, targetTag])),
      defaultName: `Group ${layer.name} & ${targetTag}`,
      targetTag,
    })
  }

  const handleConfirmNaming = async (name: string) => {
    const { tags } = namingPopover
    if (!tags || tags.length === 0) return

    const color = pickUnusedColor(Object.values(tagColors).map(t => t.color))
    await onCreateLayer(name, tags, color)
    setNamingPopover(prev => ({ ...prev, isOpen: false, targetTag: null, targetLayerId: null }))
  }

  const { isOpen: isTagOpen, onOpen: onTagOpen, onClose: onTagClose } = useDisclosure()
  const { isOpen: isNewTagColorOpen, onOpen: onNewTagColorOpen, onClose: onNewTagColorClose } = useDisclosure()
  const [newTagName, setNewTagName] = useState('')
  const [newTagColor, setNewTagColor] = useState('#A0AEC0')
  const [newTagDescription, setNewTagDescription] = useState('')

  const handleOpenAddTag = () => {
    setNewTagColor(pickUnusedColor(Object.values(tagColors).map(t => t.color)))
    onTagOpen()
  }

  const handleAddTag = async () => {
    const name = newTagName.trim()
    if (!name) return

    try {
      await onCreateTag(name, newTagColor, newTagDescription)
      if (selectedElement && !(selectedElement.tags || []).includes(name)) {
        onToggleTagOnElement(name)
      }
      setNewTagName('')
      setNewTagDescription('')
      onTagClose()
    } catch {
      // Keep the popover open so the user can retry.
    }
  }

  const usedTags = availableTags.filter(tag => (tagCounts[tag] || 0) > 0)
  const unusedTags = availableTags.filter(tag => (tagCounts[tag] || 0) === 0)

  return (
    <Box
      display="flex"
      flexDirection="column"
      overflow="hidden"
      flex="1"
      borderTop="1px solid"
      borderColor="whiteAlpha.100"
    >
      <HStack px={4} py={2} spacing={1.5} flexShrink={0}>
        <Text
          fontSize="xs"
          fontWeight="700"
          color="white"
          textTransform="uppercase"
          letterSpacing="0.02em"
          flex={1}
        >
          Tags
        </Text>

        <Popover isOpen={isTagOpen} onClose={onTagClose} placement="bottom-end">
          <PopoverTrigger>
            <IconButton
              aria-label="Add Tag"
              icon={<AddIcon boxSize="8px" />}
              size="xs"
              variant="ghost"
              color="gray.600"
              _hover={{ color: 'white', bg: 'whiteAlpha.100' }}
              onClick={handleOpenAddTag}
            />
          </PopoverTrigger>
          <PopoverContent bg="gray.800" borderColor="whiteAlpha.200" w="240px" shadow="xl">
            <PopoverBody p={2}>
              <VStack spacing={2} align="stretch">
                <HStack spacing={2}>
                  <Popover
                    isOpen={isNewTagColorOpen}
                    onClose={onNewTagColorClose}
                    placement="left"
                    closeOnBlur
                  >
                    <PopoverTrigger>
                      <Box
                        w="20px"
                        h="20px"
                        rounded="full"
                        bg={newTagColor}
                        cursor="pointer"
                        flexShrink={0}
                        onClick={onNewTagColorOpen}
                      />
                    </PopoverTrigger>
                    <ColorPicker onSelect={setNewTagColor} onClose={onNewTagColorClose} />
                  </Popover>
                  <VStack spacing={2} flex={1}>
                    <Input
                      size="xs"
                      placeholder="Tag name..."
                      value={newTagName}
                      onChange={(e) => setNewTagName(e.target.value)}
                      onKeyDown={(e) => { if (e.key === 'Enter') { void handleAddTag() } }}
                      autoFocus
                      bg="whiteAlpha.50"
                      borderColor="whiteAlpha.100"
                      _focus={{ borderColor: 'var(--accent)' }}
                    />
                    <Input
                      size="xs"
                      placeholder="Optional description..."
                      value={newTagDescription}
                      onChange={(e) => setNewTagDescription(e.target.value)}
                      onKeyDown={(e) => { if (e.key === 'Enter') { void handleAddTag() } }}
                      bg="whiteAlpha.50"
                      borderColor="whiteAlpha.100"
                      _focus={{ borderColor: 'var(--accent)' }}
                    />
                  </VStack>
                  <Button size="xs" colorScheme="blue" h="auto" py={4} onClick={() => { void handleAddTag() }}>Add</Button>
                </HStack>
              </VStack>
            </PopoverBody>
          </PopoverContent>
        </Popover>

        <IconButton
          aria-label="Toggle Section"
          icon={<ChevronDownIcon size={12} />}
          size="xs"
          variant="ghost"
          color="whiteAlpha.500"
          _hover={{ color: 'white', bg: 'whiteAlpha.200' }}
          onClick={() => setIsCollapsed(!isCollapsed)}
          transform={isCollapsed ? 'rotate(-90deg)' : 'none'}
        />
      </HStack>

      {!isCollapsed && (
        <ScrollIndicatorWrapper flex={1} minH={0}>
          <VStack align="stretch" spacing={0} pb={3}>
            {/* Active Element Section */}
            {selectedElement && (
              <Box px={3} py={2.5} bg="whiteAlpha.50" borderBottom="1px solid" borderColor="whiteAlpha.100">
                <HStack mb={2} spacing={1.5}>
                  <Box w="5px" h="5px" rounded="full" bg="var(--accent)" flexShrink={0} />
                  <Text fontSize="9px" fontWeight="700" color="whiteAlpha.400" textTransform="uppercase" letterSpacing="0.08em" isTruncated>
                    {selectedElement.name}
                  </Text>
                </HStack>
                <VStack align="stretch" spacing={2.5}>
                  {usedTags.length > 0 && (
                    <Wrap spacing={1}>
                      {usedTags.map((tag) => (
                        <WrapItem key={tag}>
                          <TagItem
                            tag={tag}
                            color={tagColors[tag]?.color || '#A0AEC0'}
                            description={tagColors[tag]?.description || null}
                            isAssigned={(selectedElement.tags || []).includes(tag)}
                            tagCount={tagCounts[tag]}
                            onToggle={() => onToggleTagOnElement(tag)}
                            onHover={(active) => onHoverLayer(active ? [tag] : null, tagColors[tag]?.color)}
                            onDropTag={(dragged: string) => handleCreateGroup(tag, dragged, tag)}
                            onDropLayer={(draggedId: number) => handleCreateGroupFromLayer(tag, draggedId)}
                            namingPopover={namingPopover.targetTag === tag ? namingPopover : undefined}
                            onConfirmNaming={handleConfirmNaming}
                            onCloseNaming={() => setNamingPopover(prev => ({ ...prev, isOpen: false, targetTag: null }))}
                            isVisible={!hiddenLayerTags.includes(tag)}
                            onToggleVisibility={() => toggleTagVisibility(tag)}
                            onSetColor={(color) => onCreateTag(tag, color)}
                            onSetDescription={(desc) => onCreateTag(tag, tagColors[tag]?.color, desc)}
                          />
                        </WrapItem>
                      ))}
                    </Wrap>
                  )}
                  {usedTags.length > 0 && unusedTags.length > 0 && (
                    <Divider borderColor="whiteAlpha.100" />
                  )}
                  {unusedTags.length > 0 && (
                    <VStack align="stretch" spacing={2}>
                      <HStack
                        spacing={1.5}
                        cursor="pointer"
                        onClick={() => setIsUnusedCollapsed(!isUnusedCollapsed)}
                        role="button"
                        opacity={0.6}
                        _hover={{ opacity: 1 }}
                        transition="opacity 0.2s"
                      >
                        <Box
                          transform={isUnusedCollapsed ? 'rotate(-90deg)' : 'none'}
                          transition="transform 0.2s"
                          display="flex"
                          alignItems="center"
                        >
                          <ChevronDownIcon size={10} />
                        </Box>
                        <Text fontSize="10px" fontWeight="bold" color="gray.500" textTransform="uppercase" letterSpacing="0.05em">
                          Unused on view ({unusedTags.length})
                        </Text>
                      </HStack>
                      {!isUnusedCollapsed && (
                        <Wrap spacing={1} opacity={0.6}>
                          {unusedTags.map((tag) => (
                            <WrapItem key={tag}>
                              <TagItem
                                tag={tag}
                                color={tagColors[tag]?.color || '#A0AEC0'}
                                description={tagColors[tag]?.description || null}
                                isAssigned={(selectedElement.tags || []).includes(tag)}
                                tagCount={tagCounts[tag]}
                                onToggle={() => onToggleTagOnElement(tag)}
                                onHover={(active) => onHoverLayer(active ? [tag] : null, tagColors[tag]?.color)}
                                onDropTag={(dragged: string) => handleCreateGroup(tag, dragged, tag)}
                                onDropLayer={(draggedId: number) => handleCreateGroupFromLayer(tag, draggedId)}
                                namingPopover={namingPopover.targetTag === tag ? namingPopover : undefined}
                                onConfirmNaming={handleConfirmNaming}
                                onCloseNaming={() => setNamingPopover(prev => ({ ...prev, isOpen: false, targetTag: null }))}
                                onSetColor={(color) => onCreateTag(tag, color)}
                                onSetDescription={(desc) => onCreateTag(tag, tagColors[tag]?.color, desc)}
                              />
                            </WrapItem>
                          ))}
                        </Wrap>
                      )}
                    </VStack>
                  )}
                </VStack>
              </Box>
            )}

            {/* Layers (Groups) List */}
            {layers.map((layer) => (
              <LayerItem
                key={layer.id}
                layer={layer}
                isActive={layer.tags.length === 0 || !layer.tags.some((t) => hiddenLayerTags.includes(t))}
                isExpanded={expandedLayerIds.has(layer.id)}
                tagCount={layerCounts[layer.id] || 0}
                tagColors={tagColors}
                onToggleActive={() => toggleLayerActive(layer)}
                onToggleExpanded={() => toggleLayerExpanded(layer.id)}
                onDelete={() => onDeleteLayer(layer.id)}
                onSetColor={(color) => onUpdateLayer({ ...layer, color })}
                onSetTagColor={onCreateTag}
                onAddTag={(tag) => onUpdateLayer({ ...layer, tags: Array.from(new Set([...layer.tags, tag])) })}
                onRemoveTag={(tag) => onUpdateLayer({ ...layer, tags: layer.tags.filter((existingTag) => existingTag !== tag) })}
                onHover={(active) => onHoverLayer(active ? layer.tags : null, layer.color)}
                selectedElementTags={selectedElement?.tags}
                onToggleTagOnElement={onToggleTagOnElement}
                namingPopover={namingPopover.targetLayerId === layer.id ? namingPopover : undefined}
                onConfirmNaming={handleConfirmNaming}
                onCloseNaming={() => setNamingPopover(prev => ({ ...prev, isOpen: false, targetLayerId: null }))}
              />
            ))}

            {/* All Tags (if no element selected or just as library) */}
            {!selectedElement && (
              <Box px={4} py={3}>
                <Text fontSize="10px" fontWeight="bold" color="gray.600" mb={2} textTransform="uppercase">Drag & Drop to tag elements</Text>
                <VStack align="stretch" spacing={3}>
                  {usedTags.length > 0 && (
                    <Wrap spacing={2}>
                      {usedTags.map((tag) => (
                        <WrapItem key={tag}>
                          <TagItem
                            tag={tag}
                            color={tagColors[tag]?.color || '#A0AEC0'}
                            description={tagColors[tag]?.description || null}
                            tagCount={tagCounts[tag]}
                            onHover={(active) => onHoverLayer(active ? [tag] : null, tagColors[tag]?.color)}
                            onDropTag={(dragged: string) => handleCreateGroup(tag, dragged, tag)}
                            onDropLayer={(draggedId: number) => handleCreateGroupFromLayer(tag, draggedId)}
                            namingPopover={namingPopover.targetTag === tag ? namingPopover : undefined}
                            onConfirmNaming={handleConfirmNaming}
                            onCloseNaming={() => setNamingPopover(prev => ({ ...prev, isOpen: false, targetTag: null }))}
                            isVisible={!hiddenLayerTags.includes(tag)}
                            onToggleVisibility={() => toggleTagVisibility(tag)}
                            onSetColor={(color) => onCreateTag(tag, color)}
                            onSetDescription={(desc) => onCreateTag(tag, tagColors[tag]?.color, desc)}
                          />
                        </WrapItem>
                      ))}
                    </Wrap>
                  )}
                  {usedTags.length > 0 && unusedTags.length > 0 && (
                    <Divider borderColor="whiteAlpha.100" />
                  )}
                  {unusedTags.length > 0 && (
                    <VStack align="stretch" spacing={2}>
                      <HStack
                        spacing={1.5}
                        cursor="pointer"
                        onClick={() => setIsUnusedCollapsed(!isUnusedCollapsed)}
                        role="button"
                        opacity={0.6}
                        _hover={{ opacity: 1 }}
                        transition="opacity 0.2s"
                      >
                        <Box
                          transform={isUnusedCollapsed ? 'rotate(-90deg)' : 'none'}
                          transition="transform 0.2s"
                          display="flex"
                          alignItems="center"
                        >
                          <ChevronDownIcon size={10} />
                        </Box>
                        <Text fontSize="10px" fontWeight="bold" color="gray.500" textTransform="uppercase" letterSpacing="0.05em">
                          Other tags ({unusedTags.length})
                        </Text>
                      </HStack>
                      {!isUnusedCollapsed && (
                        <Wrap spacing={2} opacity={0.6}>
                          {unusedTags.map((tag) => (
                            <WrapItem key={tag}>
                              <TagItem
                                tag={tag}
                                color={tagColors[tag]?.color || '#A0AEC0'}
                                description={tagColors[tag]?.description || null}
                                tagCount={tagCounts[tag]}
                                onHover={(active) => onHoverLayer(active ? [tag] : null, tagColors[tag]?.color)}
                                onDropTag={(dragged: string) => handleCreateGroup(tag, dragged, tag)}
                                onDropLayer={(draggedId: number) => handleCreateGroupFromLayer(tag, draggedId)}
                                namingPopover={namingPopover.targetTag === tag ? namingPopover : undefined}
                                onConfirmNaming={handleConfirmNaming}
                                onCloseNaming={() => setNamingPopover(prev => ({ ...prev, isOpen: false, targetTag: null }))}
                                onSetColor={(color) => onCreateTag(tag, color)}
                                onSetDescription={(desc) => onCreateTag(tag, tagColors[tag]?.color, desc)}
                              />
                            </WrapItem>
                          ))}
                        </Wrap>
                      )}
                    </VStack>
                  )}
                </VStack>
              </Box>
            )}
          </VStack>
        </ScrollIndicatorWrapper>
      )}

    </Box>
  )
}
