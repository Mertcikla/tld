import { memo, useEffect, useRef, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import type { ConnectorPanelSlots } from '../slots'
import {
  Badge,
  Box,
  Button,
  Divider,
  FormControl,
  FormLabel,
  HStack,
  SimpleGrid,
  Input,
  Textarea,
  Tag,
  TagCloseButton,
  TagLabel,
  useBreakpointValue,
  useDisclosure,
  VStack,
  Wrap,
  WrapItem,
} from '@chakra-ui/react'
import { api } from '../api/client'
import type { Connector } from '../types'
import ConfirmDialog from './ConfirmDialog'
import SlidingPanel from './SlidingPanel'
import PanelHeader from './PanelHeader'
import TagUpsert from './TagUpsert'

import { useViewEditorContext } from '../pages/ViewEditor/context'

const IconRadioBox = ({
  isSelected,
  onClick,
  isDisabled,
  children,
  title,
  'data-testid': dataTestId,
}: {
  isSelected: boolean
  onClick: () => void
  isDisabled?: boolean
  children: React.ReactNode
  title?: string
  'data-testid'?: string
}) => (
  <Box
    data-testid={dataTestId}
    as="button"
    type="button"
    onClick={onClick}
    disabled={isDisabled}
    title={title}
    w="full"
    h="36px"
    display="flex"
    alignItems="center"
    justifyContent="center"
    bg={isSelected ? 'whiteAlpha.200' : 'whiteAlpha.50'}
    borderWidth="1px"
    borderColor={isSelected ? 'whiteAlpha.400' : 'whiteAlpha.100'}
    borderRadius="md"
    color={isSelected ? 'white' : 'whiteAlpha.600'}
    opacity={isDisabled ? 0.5 : 1}
    cursor={isDisabled ? 'not-allowed' : 'pointer'}
    _hover={!isDisabled ? { bg: 'whiteAlpha.300', borderColor: 'whiteAlpha.300', color: 'white' } : undefined}
    transition="all 0.2s"
  >
    {children}
  </Box>
)

export interface ConnectorPanelProps extends ConnectorPanelSlots {
  isOpen: boolean
  onClose: () => void
  connector: Connector | null
  orgId: string
  onSave: (connector: Connector) => void
  autoSave?: boolean
  onDelete: (edgeId: number, ownerViewId?: number) => void
  visibilityOverrideDelta?: number
  onPromoteVisibility?: (id: number) => Promise<void> | void
  onDemoteVisibility?: (id: number) => Promise<void> | void
  onResetVisibility?: (id: number) => Promise<void> | void
  hasBackdrop?: boolean
  noFocusLock?: boolean
  availableTags?: string[]
  isInline?: boolean
  actions?: ReactNode
}

type ConnectorDraft = {
  label: string
  description: string
  relationship: string
  direction: string
  style: string
  url: string
  tags: string[]
}

const makeEmptyConnectorDraft = (): ConnectorDraft => ({
  label: '',
  description: '',
  relationship: '',
  direction: 'forward',
  style: 'bezier',
  url: '',
  tags: [],
})

const cloneConnectorDraft = (draft: ConnectorDraft): ConnectorDraft => ({
  ...draft,
  tags: [...draft.tags],
})

const connectorDraftFingerprint = (draft: ConnectorDraft) => JSON.stringify(cloneConnectorDraft(draft))

const draftFromConnector = (connector: Connector): ConnectorDraft => ({
  label: connector.label ?? '',
  description: connector.description ?? '',
  relationship: connector.relationship ?? '',
  direction: connector.direction === 'bidirectional' ? 'both' : (connector.direction ?? 'forward'),
  style: connector.style === 'default' ? 'bezier' : (connector.style ?? 'bezier'),
  url: connector.url ?? '',
  tags: connector.tags ?? [],
})

/**
 * Name: Edit Connector Panel
 * Role: Opens when clicked on a connector and displays its fields, allowing for editing. Same as Edit Element Panel but for connectors.
 * Location: Right side of the screen on desktop. Overlays screen on mobile.
 * Aliases: Connector Properties, Connector Details.
 */
function ConnectorPanel({ isOpen, onClose, connector, orgId, onSave, autoSave = false, onDelete, visibilityOverrideDelta = 0, onPromoteVisibility, onDemoteVisibility, onResetVisibility, hasBackdrop = true, noFocusLock, availableTags = [], connectorPanelAfterContentSlot, isInline = false, actions }: ConnectorPanelProps) {
  const { canEdit, viewId } = useViewEditorContext()
  const isReadOnly = !canEdit
  const autoSaveEdit = autoSave && !!connector && !isReadOnly
  const isMobile = useBreakpointValue({ base: true, md: false }) ?? false
  const [label, setLabel] = useState('')
  const [description, setDescription] = useState('')
  const [relType, setRelType] = useState('')
  const [direction, setDirection] = useState('forward')
  const [connectorType, setConnectorType] = useState('bezier')
  const [url, setUrl] = useState('')
  const [tags, setTags] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const confirmDelete = useDisclosure()

  const lastSavedFingerprintRef = useRef<string>('')
  const savingRef = useRef(false)
  const pendingSaveRef = useRef(false)
  const initializedConnectorIdRef = useRef<number | null>(null)
  const draftRef = useRef<ConnectorDraft>(makeEmptyConnectorDraft())

  const applyDraft = useCallback((draft: ConnectorDraft) => {
    const nextDraft = cloneConnectorDraft(draft)
    draftRef.current = nextDraft
    setLabel(nextDraft.label)
    setDescription(nextDraft.description)
    setRelType(nextDraft.relationship)
    setDirection(nextDraft.direction)
    setConnectorType(nextDraft.style)
    setUrl(nextDraft.url)
    setTags(nextDraft.tags)
  }, [])

  const patchDraft = useCallback((patch: Partial<ConnectorDraft>) => {
    const nextDraft = { ...draftRef.current, ...patch }
    if (patch.tags) {
      nextDraft.tags = [...patch.tags]
    }
    draftRef.current = nextDraft
  }, [])

  useEffect(() => {
    if (!isOpen) {
      initializedConnectorIdRef.current = null
      return
    }

    if (connector) {
      if (initializedConnectorIdRef.current === connector.id) return
      initializedConnectorIdRef.current = connector.id
      const draft = draftFromConnector(connector)
      applyDraft(draft)
      lastSavedFingerprintRef.current = connectorDraftFingerprint(draft)
    } else {
      initializedConnectorIdRef.current = null
      lastSavedFingerprintRef.current = ''
      applyDraft(makeEmptyConnectorDraft())
    }
  }, [applyDraft, connector, isOpen])

  const buildPayloadAndFingerprint = useCallback(async () => {
    const payload = cloneConnectorDraft(draftRef.current)
    return { payload, fingerprint: connectorDraftFingerprint(payload) }
  }, [])

  const saveIfDirty = useCallback(async () => {
    if (!autoSaveEdit || !connector) return

    if (viewId == null) return
    if (savingRef.current) {
      pendingSaveRef.current = true
      return
    }

    const { payload, fingerprint } = await buildPayloadAndFingerprint()
    if (fingerprint === lastSavedFingerprintRef.current) return

    savingRef.current = true
    try {
      const updated = await api.workspace.connectors.update(viewId, connector.id, {
        label: payload.label,
        description: payload.description,
        relationship: payload.relationship,
        direction: payload.direction,
        style: payload.style,
        url: payload.url,
        tags: payload.tags,
      })
      lastSavedFingerprintRef.current = fingerprint
      onSave(updated)
    } catch {
      // ignore
    } finally {
      savingRef.current = false
      if (pendingSaveRef.current) {
        pendingSaveRef.current = false
        window.setTimeout(() => {
          void saveIfDirtyRef.current?.()
        }, 0)
      }
    }
  }, [autoSaveEdit, connector, viewId, buildPayloadAndFingerprint, onSave])

  const saveIfDirtyRef = useRef<(() => Promise<void>) | null>(null)
  useEffect(() => { saveIfDirtyRef.current = saveIfDirty }, [saveIfDirty])

  const scheduleAutoSave = () => {
    if (!autoSaveEdit) return
    requestAnimationFrame(() => {
      void saveIfDirtyRef.current?.()
    })
  }

  useEffect(() => {
    if (!autoSaveEdit || !connector) return
    const timer = window.setTimeout(() => {
      void saveIfDirtyRef.current?.()
    }, 150)
    return () => window.clearTimeout(timer)
  }, [autoSaveEdit, connector, label, description, relType, direction, connectorType, url, tags])

  const handleSave = useCallback(async () => {
    if (isReadOnly || !connector || viewId == null) return
    setLoading(true)
    try {
      const { payload } = await buildPayloadAndFingerprint()
      const updated = await api.workspace.connectors.update(viewId, connector.id, {
        label: payload.label,
        description: payload.description,
        relationship: payload.relationship,
        direction: payload.direction,
        style: payload.style,
        url: payload.url,
        tags: payload.tags,
      })
      onSave(updated)
      onClose()
    } catch {
      // intentionally empty
    } finally {
      setLoading(false)
    }
  }, [isReadOnly, connector, viewId, buildPayloadAndFingerprint, onSave, onClose])

  const handleClose = useCallback(async () => {
    if (autoSaveEdit) {
      await saveIfDirtyRef.current?.()
    }
    onClose()
  }, [autoSaveEdit, onClose])

  useEffect(() => {
    if (!isOpen) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') handleClose()
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        if (!autoSaveEdit) {
          handleSave()
        } else {
          handleClose()
        }
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [isOpen, handleClose, autoSaveEdit, handleSave])

  const handleDelete = async () => {
    if (isReadOnly || !connector) return
    try {
      await api.workspace.connectors.delete(orgId, connector.id)
      onDelete(connector.id, connector.view_id)
      confirmDelete.onClose()
      onClose()
    } catch {
      // intentionally empty
    }
  }

  return (
    <>
      <SlidingPanel data-testid="connector-panel" isOpen={isOpen} onClose={handleClose} panelKey="connector" side={isMobile ? 'left' : 'right'} width="300px" hasBackdrop={hasBackdrop} noFocusLock={noFocusLock} autoFocus={true} isInline={isInline}>
        <PanelHeader title="Edit Connector" onClose={handleClose} hasCloseButton={!isInline} isInline={isInline} actions={actions} />

        {/* Body */}
        <Box px={4} py={4} overflowY="auto" flex={1}>
          <VStack spacing={4} align="stretch">
            <FormControl isDisabled={isReadOnly} id="connector-label">
              <FormLabel>Label / Name</FormLabel>
              <Input
                data-testid="connector-panel-label-input"
                name="label"
                size="sm"
                value={label}
                onChange={(e) => {
                  const nextLabel = e.target.value
                  patchDraft({ label: nextLabel })
                  setLabel(nextLabel)
                }}
                onBlur={scheduleAutoSave}
                placeholder="uses, calls, sends to…"
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly} id="connector-direction">
              <FormLabel>Direction</FormLabel>
              <SimpleGrid columns={2} spacing={2}>
                <IconRadioBox
                  data-testid="connector-panel-direction-forward"
                  isSelected={direction === 'forward'}
                  onClick={() => { patchDraft({ direction: 'forward' }); setDirection('forward'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Forward"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 12h14m-7-7 7 7-7 7"/></svg>
                </IconRadioBox>
                <IconRadioBox
                  data-testid="connector-panel-direction-backward"
                  isSelected={direction === 'backward'}
                  onClick={() => { patchDraft({ direction: 'backward' }); setDirection('backward'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Backward"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M19 12H5m7 7-7-7 7-7"/></svg>
                </IconRadioBox>
                <IconRadioBox
                  data-testid="connector-panel-direction-both"
                  isSelected={direction === 'both'}
                  onClick={() => { patchDraft({ direction: 'both' }); setDirection('both'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Bidirectional"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M8 8l-4 4 4 4M16 8l4 4-4 4M4 12h16"/></svg>
                </IconRadioBox>
                <IconRadioBox
                  data-testid="connector-panel-direction-none"
                  isSelected={direction === 'none'}
                  onClick={() => { patchDraft({ direction: 'none' }); setDirection('none'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="None"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 12h14"/></svg>
                </IconRadioBox>
              </SimpleGrid>
            </FormControl>
            <FormControl isDisabled={isReadOnly} id="connector-style">
              <FormLabel>Connector Style</FormLabel>
              <SimpleGrid columns={2} spacing={2}>
                <IconRadioBox
                  data-testid="connector-panel-style-smoothstep"
                  isSelected={connectorType === 'smoothstep'}
                  onClick={() => { patchDraft({ style: 'smoothstep' }); setConnectorType('smoothstep'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Smooth Step"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 19H9C10.6569 19 12 17.6569 12 16V8C12 6.34315 13.3431 5 15 5H19" /></svg>
                </IconRadioBox>
                <IconRadioBox
                  data-testid="connector-panel-style-bezier"
                  isSelected={connectorType === 'bezier'}
                  onClick={() => { patchDraft({ style: 'bezier' }); setConnectorType('bezier'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Bezier"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 19C5 11 19 13 19 5" /></svg>
                </IconRadioBox>
                <IconRadioBox
                  data-testid="connector-panel-style-straight"
                  isSelected={connectorType === 'straight'}
                  onClick={() => { patchDraft({ style: 'straight' }); setConnectorType('straight'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Straight"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 19L19 5" /></svg>
                </IconRadioBox>
                <IconRadioBox
                  data-testid="connector-panel-style-step"
                  isSelected={connectorType === 'step'}
                  onClick={() => { patchDraft({ style: 'step' }); setConnectorType('step'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Step"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 19H12V5H19" /></svg>
                </IconRadioBox>
              </SimpleGrid>
            </FormControl>
            <FormControl isDisabled={isReadOnly} id="connector-rel-type">
              <FormLabel>Relationship Type</FormLabel>
              <Input
                data-testid="connector-panel-relationship-input"
                name="relationship"
                size="sm"
                value={relType}
                onChange={(e) => {
                  const nextRelationship = e.target.value
                  patchDraft({ relationship: nextRelationship })
                  setRelType(nextRelationship)
                }}
                onBlur={scheduleAutoSave}
                placeholder="HTTP, gRPC, async…"
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly} id="connector-url">
              <FormLabel>URL</FormLabel>
              <Input
                data-testid="connector-panel-url-input"
                name="url"
                size="sm"
                value={url}
                onChange={(e) => {
                  const nextUrl = e.target.value
                  patchDraft({ url: nextUrl })
                  setUrl(nextUrl)
                }}
                onBlur={scheduleAutoSave}
                placeholder="https://docs.example.com/…"
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly} id="connector-description">
              <FormLabel>Description</FormLabel>
              <Textarea
                data-testid="connector-panel-description-input"
                name="description"
                size="sm"
                value={description}
                onChange={(e) => {
                  const nextDescription = e.target.value
                  patchDraft({ description: nextDescription })
                  setDescription(nextDescription)
                }}
                onBlur={scheduleAutoSave}
                placeholder="Describe the relationship…"
                rows={4}
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly} id="connector-tags">
              <FormLabel>Tags</FormLabel>
              <TagUpsert
                currentTags={tags}
                availableTags={availableTags}
                onAddTag={(tag) => {
                  if (!draftRef.current.tags.includes(tag)) {
                    const nextTags = [...draftRef.current.tags, tag]
                    patchDraft({ tags: nextTags })
                    setTags(nextTags)
                    scheduleAutoSave()
                  }
                }}
                isReadOnly={isReadOnly}
              />
              <Wrap mt={3}>
                {tags.map((tag) => (
                  <WrapItem key={tag}>
                    <Tag data-testid="connector-panel-tag-chip" size="sm" variant="subtle" bg="whiteAlpha.100" border="1px solid" borderColor="whiteAlpha.200">
                      <TagLabel color="white">{tag}</TagLabel>
                      {!isReadOnly && (
                        <TagCloseButton data-testid="connector-panel-tag-remove" onClick={() => {
                          const nextTags = draftRef.current.tags.filter((item) => item !== tag)
                          patchDraft({ tags: nextTags })
                          setTags(nextTags)
                          scheduleAutoSave()
                        }} />
                      )}
                    </Tag>
                  </WrapItem>
                ))}
              </Wrap>
            </FormControl>

            {connector && (onPromoteVisibility || onDemoteVisibility || onResetVisibility) && (
              <Box borderTop="1px solid" borderColor="whiteAlpha.100" pt={2}>
                <HStack justify="space-between" mb={2}>
                  <FormLabel fontSize="xs" fontWeight="bold" color="gray.400" mb={0}>DENSITY</FormLabel>
                  {visibilityOverrideDelta !== 0 && (
                    <Badge colorScheme={visibilityOverrideDelta > 0 ? 'teal' : 'orange'} variant="subtle">
                      {visibilityOverrideDelta > 0 ? `+${visibilityOverrideDelta}` : visibilityOverrideDelta}
                    </Badge>
                  )}
                </HStack>
                <HStack spacing={2}>
                  <Button variant="subtle" size="sm" color="teal.200" _hover={{ bg: 'teal.900', color: 'teal.100' }} onClick={() => onPromoteVisibility?.(connector.id)} flex={1} isDisabled={isReadOnly}>
                    Promote
                  </Button>
                  <Button variant="subtle" size="sm" color="orange.200" _hover={{ bg: 'orange.900', color: 'orange.100' }} onClick={() => onDemoteVisibility?.(connector.id)} flex={1} isDisabled={isReadOnly}>
                    Demote
                  </Button>
                  {visibilityOverrideDelta !== 0 && (
                    <Button variant="ghost" size="sm" onClick={() => onResetVisibility?.(connector.id)} isDisabled={isReadOnly}>
                      Reset
                    </Button>
                  )}
                </HStack>
              </Box>
            )}

            {connectorPanelAfterContentSlot}

          </VStack>
        </Box>

        <Divider borderColor="whiteAlpha.100" />

        {/* Footer */}
        <HStack px={4} py={3} justify="space-between" flexShrink={0}>
          {canEdit ? (
            <Button data-testid="connector-panel-delete" variant="ghost" size="sm" color="red.400" _hover={{ bg: 'red.900', color: 'red.100' }} onClick={confirmDelete.onOpen}>
              Delete
            </Button>
          ) : (
            <Box />
          )}
          <HStack>
            {!autoSaveEdit && (
              <>
                {!isInline && (
                  <Button variant="ghost" size="sm" onClick={handleClose}>
                    Cancel
                  </Button>
                )}
                {canEdit && (
                  <Button size="sm" px={5} colorScheme="blue" onClick={handleSave} isLoading={loading}>
                    Save
                  </Button>
                )}
              </>
            )}
            {autoSaveEdit && !isInline && (
              <Button variant="ghost" size="sm" onClick={handleClose}>
                Close
              </Button>
            )}
          </HStack>
        </HStack>
      </SlidingPanel>

      <ConfirmDialog
        isOpen={confirmDelete.isOpen}
        onClose={confirmDelete.onClose}
        onConfirm={handleDelete}
        title="Delete Connector"
        body="Delete this connector? This action cannot be undone."
        confirmLabel="Delete"
      />
    </>
  )
}

export default memo(ConnectorPanel)
