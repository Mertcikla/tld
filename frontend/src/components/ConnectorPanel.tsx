import { memo, useEffect, useRef, useState, useCallback } from 'react'
import type { ConnectorPanelSlots } from '../slots'
import {
  Box,
  Button,
  Divider,
  FormControl,
  FormLabel,
  HStack,
  SimpleGrid,
  Input,
  Textarea,
  useBreakpointValue,
  useDisclosure,
  VStack,
} from '@chakra-ui/react'
import { api } from '../api/client'
import type { Connector } from '../types'
import ConfirmDialog from './ConfirmDialog'
import SlidingPanel from './SlidingPanel'
import PanelHeader from './PanelHeader'

import { useViewEditorContext } from '../pages/ViewEditor/context'

const IconRadioBox = ({
  isSelected,
  onClick,
  isDisabled,
  children,
  title,
}: {
  isSelected: boolean
  onClick: () => void
  isDisabled?: boolean
  children: React.ReactNode
  title?: string
}) => (
  <Box
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
  onDelete: (edgeId: number) => void
  hasBackdrop?: boolean
}

/**
 * Name: Edit Connector Panel
 * Role: Opens when clicked on a connector and displays its fields, allowing for editing. Same as Edit Element Panel but for connectors.
 * Location: Right side of the screen on desktop. Overlays screen on mobile.
 * Aliases: Connector Properties, Connector Details.
 */
function ConnectorPanel({ isOpen, onClose, connector, orgId, onSave, autoSave = false, onDelete, hasBackdrop = true, connectorPanelAfterContentSlot }: ConnectorPanelProps) {
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
  const [loading, setLoading] = useState(false)
  const confirmDelete = useDisclosure()

  const lastSavedFingerprintRef = useRef<string>('')
  const savingRef = useRef(false)
  const pendingSaveRef = useRef(false)

  useEffect(() => {
    if (connector) {
      setLabel(connector.label ?? '')
      setDescription(connector.description ?? '')
      setRelType(connector.relationship ?? '')
      const dir = connector.direction === 'bidirectional' ? 'both' : (connector.direction ?? 'forward')
      setDirection(dir)
      const type = connector.style === 'default' ? 'bezier' : (connector.style ?? 'bezier')
      setConnectorType(type)
      setUrl(connector.url ?? '')

      lastSavedFingerprintRef.current = JSON.stringify({
        label: connector.label ?? '',
        description: connector.description ?? '',
        relationship: connector.relationship ?? '',
        direction: dir,
        style: type,
        url: connector.url ?? '',
      })
    } else {
      lastSavedFingerprintRef.current = ''
    }
  }, [connector, isOpen])

  const buildPayloadAndFingerprint = useCallback(async () => {
    const payload = {
      label,
      description,
      relationship: relType,
      direction,
      style: connectorType,
      url,
    }
    return { payload, fingerprint: JSON.stringify(payload) }
  }, [label, description, relType, direction, connectorType, url])

  const saveIfDirty = useCallback(async () => {
    if (!autoSaveEdit || !connector) return

    if (savingRef.current || viewId == null) return

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
      })
      lastSavedFingerprintRef.current = fingerprint
      onSave(updated)
    } catch {
      // ignore
    } finally {
      savingRef.current = false
      if (pendingSaveRef.current) {
        pendingSaveRef.current = false
        void saveIfDirty()
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

  const handleClose = useCallback(() => {
    if (autoSaveEdit) {
      void saveIfDirtyRef.current?.()
    }
    onClose()
  }, [autoSaveEdit, onClose])

  useEffect(() => {
    if (!isOpen) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') handleClose()
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [isOpen, handleClose])

  const handleSave = async () => {
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
      })
      onSave(updated)
      onClose()
    } catch {
      // intentionally empty
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async () => {
    if (isReadOnly || !connector) return
    try {
      await api.workspace.connectors.delete(orgId, connector.id)
      onDelete(connector.id)
      confirmDelete.onClose()
      onClose()
    } catch {
      // intentionally empty
    }
  }

  return (
    <>
      <SlidingPanel isOpen={isOpen} onClose={handleClose} panelKey="connector" side={isMobile ? 'left' : 'right'} width="300px" hasBackdrop={hasBackdrop}>
        <PanelHeader title="Edit Connector" onClose={handleClose} />

        {/* Body */}
        <Box px={4} py={4} overflowY="auto" flex={1}>
          <VStack spacing={4} align="stretch">
            <FormControl isDisabled={isReadOnly} id="connector-label">
              <FormLabel>Label / Name</FormLabel>
              <Input
                name="label"
                size="sm"
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                onBlur={scheduleAutoSave}
                placeholder="uses, calls, sends to…"
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly} id="connector-direction">
              <FormLabel>Direction</FormLabel>
              <SimpleGrid columns={2} spacing={2}>
                <IconRadioBox
                  isSelected={direction === 'forward'}
                  onClick={() => { setDirection('forward'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Forward"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 12h14m-7-7 7 7-7 7"/></svg>
                </IconRadioBox>
                <IconRadioBox
                  isSelected={direction === 'backward'}
                  onClick={() => { setDirection('backward'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Backward"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M19 12H5m7 7-7-7 7-7"/></svg>
                </IconRadioBox>
                <IconRadioBox
                  isSelected={direction === 'both'}
                  onClick={() => { setDirection('both'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Bidirectional"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M8 8l-4 4 4 4M16 8l4 4-4 4M4 12h16"/></svg>
                </IconRadioBox>
                <IconRadioBox
                  isSelected={direction === 'none'}
                  onClick={() => { setDirection('none'); scheduleAutoSave() }}
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
                  isSelected={connectorType === 'smoothstep'}
                  onClick={() => { setConnectorType('smoothstep'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Smooth Step"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 19H9C10.6569 19 12 17.6569 12 16V8C12 6.34315 13.3431 5 15 5H19" /></svg>
                </IconRadioBox>
                <IconRadioBox
                  isSelected={connectorType === 'bezier'}
                  onClick={() => { setConnectorType('bezier'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Bezier"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 19C5 11 19 13 19 5" /></svg>
                </IconRadioBox>
                <IconRadioBox
                  isSelected={connectorType === 'straight'}
                  onClick={() => { setConnectorType('straight'); scheduleAutoSave() }}
                  isDisabled={isReadOnly}
                  title="Straight"
                >
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 19L19 5" /></svg>
                </IconRadioBox>
                <IconRadioBox
                  isSelected={connectorType === 'step'}
                  onClick={() => { setConnectorType('step'); scheduleAutoSave() }}
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
                name="relationship"
                size="sm"
                value={relType}
                onChange={(e) => setRelType(e.target.value)}
                onBlur={scheduleAutoSave}
                placeholder="HTTP, gRPC, async…"
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly} id="connector-url">
              <FormLabel>URL</FormLabel>
              <Input
                name="url"
                size="sm"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                onBlur={scheduleAutoSave}
                placeholder="https://docs.example.com/…"
              />
            </FormControl>
            <FormControl isDisabled={isReadOnly} id="connector-description">
              <FormLabel>Description</FormLabel>
              <Textarea
                name="description"
                size="sm"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                onBlur={scheduleAutoSave}
                placeholder="Describe the relationship…"
                rows={4}
              />
            </FormControl>

            {connectorPanelAfterContentSlot}

          </VStack>
        </Box>

        <Divider borderColor="whiteAlpha.100" />

        {/* Footer */}
        <HStack px={4} py={3} justify="space-between" flexShrink={0}>
          {canEdit ? (
            <Button variant="ghost" size="sm" color="red.400" _hover={{ bg: 'red.900', color: 'red.100' }} onClick={confirmDelete.onOpen}>
              Delete
            </Button>
          ) : (
            <Box />
          )}
          <HStack>
            {!autoSaveEdit && (
              <>
                <Button variant="ghost" size="sm" onClick={handleClose}>
                  Cancel
                </Button>
                {canEdit && (
                  <Button size="sm" px={5} colorScheme="blue" onClick={handleSave} isLoading={loading}>
                    Save
                  </Button>
                )}
              </>
            )}
            {autoSaveEdit && (
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
