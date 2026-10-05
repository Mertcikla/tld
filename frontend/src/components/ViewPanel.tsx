import { memo, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import {
  Box,
  Button,
  Checkbox,
  Collapse,
  Divider,
  FormControl,
  FormLabel,
  HStack,
  Icon,
  Input,
  Tag,
  TagCloseButton,
  TagLabel,
  Text,
  Textarea,
  useBreakpointValue,
  VStack,
  Wrap,
  WrapItem,
} from '@chakra-ui/react'
import { api } from '../api/client'
import type { Connector, ViewTreeNode, ViewMarkdownDocument } from '../types'
import SlidingPanel from './SlidingPanel'
import PanelHeader from './PanelHeader'
import LayoutSection from './LayoutSection'
import ScrollIndicatorWrapper from './ScrollIndicatorWrapper'
import TagUpsert from './TagUpsert'
import { ChevronDownIcon, ChevronRightIcon } from './Icons'

import { useContext } from 'react'
import { ViewEditorContext } from '../pages/ViewEditor/context'

interface Props {
  isOpen: boolean
  onClose: () => void
  view: ViewTreeNode | null
  canEdit?: boolean
  onSave: (updated: ViewTreeNode) => void
  onUnsupportedMutation?: () => void
  onConnectorSaved?: (connector: Connector) => void
  hasBackdrop?: boolean
  availableTags?: string[]
  isInline?: boolean
  dbOnlyNotes?: boolean
  markdown?: ViewMarkdownDocument | null
  markdownLoading?: boolean
  onUnlinkMarkdown?: (options?: { deleteManagedFile: boolean }) => Promise<void> | void
  onOpenMarkdown?: () => void
  actions?: ReactNode
}

function viewMarkdownSummary(markdown: ViewMarkdownDocument) {
  if (!markdown.exists) return 'Missing file'
  const source = markdown.source_kind === 'REPO'
    ? 'Repo note'
    : markdown.source_kind === 'ATTACHED'
      ? 'Attached file'
      : markdown.source_kind === 'PRIVATE_WORKSPACE' || markdown.source_kind === 'PRIVATE_APP'
        ? 'Private note'
        : markdown.is_managed ? 'Private note' : 'Attached file'
  if (!markdown.can_edit) return `${source} · read-only`
  if (markdown.source_kind === 'REPO' && markdown.git_state && markdown.git_state !== 'unknown') {
    return `${source} · ${markdown.git_state.replace(/_/g, ' ')}`
  }
  return source
}

/**
 * Name: View Details Panel
 * Role: Opens on the right and allows view field updates.
 * Location: Right side of the screen on desktop. Overlays screen on mobile.
 * Aliases: View Properties, View Settings.
 */
function ViewPanel({
  isOpen,
  onClose,
  view,
  canEdit: canEditProp,
  onSave,
  onUnsupportedMutation,
  onConnectorSaved,
  hasBackdrop = true,
  availableTags = [],
  isInline = false,
  dbOnlyNotes = false,
  markdown = null,
  markdownLoading = false,
  onUnlinkMarkdown,
  onOpenMarkdown,
  actions,
}: Props) {
  const ctx = useContext(ViewEditorContext)
  const canEdit = canEditProp ?? ctx?.canEdit ?? true
  const isReadOnly = !canEdit
  const isMobile = useBreakpointValue({ base: true, md: false }) ?? false
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [levelLabel, setLevelLabel] = useState('')
  const [tags, setTags] = useState<string[]>([])
  const [saving, setSaving] = useState(false)

  const [deleteManagedFile, setDeleteManagedFile] = useState(true)
  const [markdownAction, setMarkdownAction] = useState<'unlink' | null>(null)
  const [markdownOpen, setMarkdownOpen] = useState(!!markdown)

  useEffect(() => {
    if (view) {
      setName(view.name)
      setDescription(view.description || '')
      setLevelLabel(view.level_label || '')
      setTags(view.tags || [])
      setDeleteManagedFile(true)
      setMarkdownOpen(!!markdown)
    }
  }, [view, isOpen, markdown])

  useEffect(() => {
    if (!isOpen) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [isOpen, onClose])

  const handleSave = async () => {
    if (isReadOnly || !view) return
    setSaving(true)
    try {
      const updated = await api.workspace.views.update(view.id, {
        name: name.trim(),
        description,
        label: levelLabel,
        tags,
      })
      onSave({ ...view, name: updated.name, description, level_label: updated.label, tags: updated.tags })
      onClose()
    } catch {
      // intentionally empty
    } finally {
      setSaving(false)
    }
  }

  const handleUnlinkMarkdown = async () => {
    if (!canEdit || !onUnlinkMarkdown) return
    setMarkdownAction('unlink')
    try {
      await onUnlinkMarkdown({ deleteManagedFile })
    } finally {
      setMarkdownAction(null)
    }
  }

  return (
    <SlidingPanel data-testid="view-panel" isOpen={isOpen} onClose={onClose} panelKey="view" side={isMobile ? 'left' : 'right'} width="320px" hasBackdrop={hasBackdrop} isInline={isInline}>
      <PanelHeader title="View Details" onClose={onClose} hasCloseButton={!isInline} isInline={isInline} actions={actions} />

      {/* Body */}
      <ScrollIndicatorWrapper px={4} py={4}>
        <VStack spacing={4} align="stretch">
          <FormControl isRequired>
            <FormLabel>Name</FormLabel>
            <Input
              data-testid="view-panel-name-input"
              size="sm"
              value={name}
              isDisabled={isReadOnly}
              onChange={(e) => setName(e.target.value)}
            />
          </FormControl>
          <FormControl>
            <FormLabel>Level Label</FormLabel>
            <Input
              data-testid="view-panel-label-input"
              size="sm"
              value={levelLabel}
              isDisabled={isReadOnly}
              onChange={(e) => setLevelLabel(e.target.value)}
              placeholder="e.g. System Context, Containers…"
            />
          </FormControl>
          <FormControl>
            <FormLabel>Description</FormLabel>
            <Textarea
              data-testid="view-panel-description-input"
              size="sm"
              value={description}
              isDisabled={isReadOnly}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional description"
              rows={4}
            />
          </FormControl>
          <FormControl isDisabled={isReadOnly}>
            <FormLabel>Tags</FormLabel>
            <TagUpsert
              currentTags={tags}
              availableTags={availableTags}
              onAddTag={(tag) => {
                if (!tags.includes(tag)) setTags((prev) => [...prev, tag])
              }}
              isReadOnly={isReadOnly}
            />
            <Wrap mt={3}>
              {tags.map((tag) => (
                <WrapItem key={tag}>
                  <Tag size="sm" variant="subtle" bg="whiteAlpha.100" border="1px solid" borderColor="whiteAlpha.200">
                    <TagLabel color="white">{tag}</TagLabel>
                    {!isReadOnly && (
                      <TagCloseButton onClick={() => setTags((prev) => prev.filter((item) => item !== tag))} />
                    )}
                  </Tag>
                </WrapItem>
              ))}
            </Wrap>
          </FormControl>
          <LayoutSection view={view} canEdit={canEdit} onUnsupportedMutation={onUnsupportedMutation} onConnectorSaved={onConnectorSaved} />

          {(canEdit || !!markdown) && (
            <>
              <Divider borderColor="whiteAlpha.100" my={2} />
              <VStack align="stretch" spacing={3}>
                <HStack
                  data-testid="view-panel-markdown-toggle"
                  cursor="pointer"
                  onClick={() => setMarkdownOpen(v => !v)}
                  justify="space-between"
                  userSelect="none"
                  _hover={{ color: 'whiteAlpha.900' }}
                  transition="color 0.15s"
                >
                  <Text fontWeight="semibold" fontSize="sm" color="whiteAlpha.800">
                    Markdown Notes
                  </Text>
                  <Icon
                    as={markdownOpen ? ChevronDownIcon : ChevronRightIcon}
                    boxSize={3.5}
                    strokeWidth={3.5}
                    color="whiteAlpha.500"
                  />
                </HStack>

                <Collapse in={markdownOpen} animateOpacity>
                  <VStack align="stretch" spacing={3} pt={1}>
                    <Text fontSize="xs" color="gray.400">
                      {dbOnlyNotes ? 'Current note metadata.' : 'Current notes file metadata.'}
                    </Text>

                    {markdownLoading ? (
                      <Text fontSize="xs" color="gray.500">Loading markdown metadata…</Text>
                    ) : markdown ? (
                      <Box
                        p={3}
                        bg="whiteAlpha.50"
                        border="1px solid"
                        borderColor="whiteAlpha.100"
                        borderRadius="md"
                      >
                        <VStack align="stretch" spacing={2.5}>
                          <HStack justify="space-between" align="start">
                            {dbOnlyNotes ? (
                              <Text fontSize="xs" color="gray.400" flex={1} mr={2}>
                                Private note
                              </Text>
                            ) : (
                              <Text fontSize="xs" color="gray.400" wordBreak="break-all" flex={1} mr={2}>
                                {markdown.path}
                              </Text>
                            )}
                            {onOpenMarkdown && (
                              <Button data-testid="view-panel-markdown-open" size="xs" variant="outline" onClick={onOpenMarkdown}>
                                Open
                              </Button>
                            )}
                          </HStack>
                          {markdown.updated_at && (
                            <Text fontSize="10px" color="gray.500">
                              Updated {new Date(markdown.updated_at).toLocaleString()}
                            </Text>
                          )}
                          <Text fontSize="10px" color={markdown.exists ? 'gray.500' : 'red.300'}>
                            {viewMarkdownSummary(markdown)}
                          </Text>
                          {markdown.is_managed && canEdit && !dbOnlyNotes && (
                            <Checkbox
                              size="sm"
                              isChecked={deleteManagedFile}
                              onChange={(event) => setDeleteManagedFile(event.target.checked)}
                            >
                              <Text fontSize="xs" color="gray.300">Also delete file</Text>
                            </Checkbox>
                          )}
                          {canEdit && (
                            <Button
                              data-testid="view-panel-markdown-unlink"
                              size="sm"
                              colorScheme="red"
                              variant="outline"
                              onClick={() => { void handleUnlinkMarkdown() }}
                              isLoading={markdownAction === 'unlink'}
                            >
                              {dbOnlyNotes ? 'Delete note' : 'Detach'}
                            </Button>
                          )}
                        </VStack>
                      </Box>
                    ) : (
                      <Text fontSize="xs" color="gray.500">
                        {dbOnlyNotes ? 'No note created.' : 'No file attached.'}
                      </Text>
                    )}

                    {canEdit && !markdown && (
                      <Text fontSize="xs" color="gray.500">
                        {dbOnlyNotes
                          ? 'Open Notes from the canvas toolbar to create a note.'
                          : 'Open Notes from the canvas toolbar to create or attach a markdown file.'}
                      </Text>
                    )}
                  </VStack>
                </Collapse>
              </VStack>
            </>
          )}

          {view && (
            <Box pt={2} borderTop="1px solid" borderColor="whiteAlpha.50">
              <Text fontSize="xs" color="gray.600">
                Created {new Date(view.created_at).toLocaleString()}
              </Text>
              <Text fontSize="xs" color="gray.600">
                Updated {new Date(view.updated_at).toLocaleString()}
              </Text>
            </Box>
          )}
        </VStack>
      </ScrollIndicatorWrapper>

      <Divider borderColor="whiteAlpha.100" />

      {/* Footer */}
      <HStack px={4} py={3} justify="flex-end" flexShrink={0}>
        <HStack>
          {!isInline && (
            <Button data-testid="view-panel-cancel" variant="ghost" size="sm" onClick={onClose}>
              Cancel
            </Button>
          )}
          {canEdit && (
            <Button
              data-testid="view-panel-save"
              size="sm"
              px={5}
              colorScheme="blue"
              onClick={handleSave}
              isLoading={saving}
              isDisabled={isReadOnly || !name.trim()}
            >
              Save
            </Button>
          )}
        </HStack>
      </HStack>
    </SlidingPanel>
  )
}

export default memo(ViewPanel)
