import { useState } from 'react'
import {
  Button,
  FormControl,
  FormLabel,
  Input,
  Modal,
  ModalBody,
  ModalCloseButton,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Select,
  Text,
  VStack,
} from '@chakra-ui/react'
import { api } from '../api/client'
import type { LibraryElement, ViewTreeNode } from '../types'
import type { InventoryObjectType } from './inventoryData'

// Element creation reuses ElementPanel. Views and connectors need only their identity here;
// their full details continue to be edited by the existing panels.
export default function InventoryCreateDialog({
  type,
  elements,
  views,
  placementByViewElement,
  onClose,
  onCreated,
}: {
  type: Exclude<InventoryObjectType, 'element'>
  elements: LibraryElement[]
  views: ViewTreeNode[]
  placementByViewElement: Record<string, { x: number; y: number }>
  onClose: () => void
  onCreated: (key: string) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [viewId, setViewId] = useState('')
  const [source, setSource] = useState('')
  const [target, setTarget] = useState('')
  const [elementSearch, setElementSearch] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const placed = elements.filter((element) => `${viewId}:${element.id}` in placementByViewElement)
  const matches = elements.filter((element) =>
    element.name.toLowerCase().includes(elementSearch.toLowerCase()),
  )
  const valid = type === 'view' ? !!name.trim() : !!viewId && !!source && !!target
  const save = async () => {
    if (!valid || saving) return
    setSaving(true)
    setError('')
    try {
      const result =
        type === 'view'
          ? await api.workspace.views.create({ name: name.trim() })
          : await api.workspace.connectors.create(Number(viewId), {
              source_element_id: Number(source),
              target_element_id: Number(target),
              label: name.trim(),
              direction: 'forward',
              style: 'bezier',
            })
      await onCreated(`${type}:${result.id}`)
      onClose()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : `Could not create ${type}. Please retry.`)
    } finally {
      setSaving(false)
    }
  }
  return (
    <Modal
      isOpen
      onClose={() => {
        if (!saving) onClose()
      }}
      isCentered
      closeOnOverlayClick={!saving}
      closeOnEsc={!saving}
    >
      <ModalOverlay />
      <ModalContent
        mx={3}
        as="form"
        onSubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <ModalHeader>New {type}</ModalHeader>
        <ModalCloseButton isDisabled={saving} />
        <ModalBody>
          <VStack align="stretch" spacing={4}>
            <FormControl isRequired={type === 'view'}>
              <FormLabel htmlFor="inventory-create-name">
                {type === 'view' ? 'Name' : 'Label (optional)'}
              </FormLabel>
              <Input
                id="inventory-create-name"
                autoFocus
                value={name}
                onChange={(event) => setName(event.target.value)}
                isDisabled={saving}
              />
            </FormControl>
            {type === 'connector' && (
              <>
                <FormControl isRequired>
                  <FormLabel htmlFor="inventory-create-view">View</FormLabel>
                  <Select
                    id="inventory-create-view"
                    value={viewId}
                    onChange={(event) => {
                      setViewId(event.target.value)
                      setSource('')
                      setTarget('')
                    }}
                    placeholder="Choose a view"
                    isDisabled={saving}
                  >
                    {views.map((view) => (
                      <option key={view.id} value={view.id}>
                        {view.name}
                      </option>
                    ))}
                  </Select>
                </FormControl>
                {viewId && (
                  <>
                    <Input
                      aria-label="Find connector endpoints"
                      placeholder="Find elements…"
                      value={elementSearch}
                      onChange={(event) => setElementSearch(event.target.value)}
                      isDisabled={saving}
                    />
                    {(['Source', 'Target'] as const).map((label) => {
                      const value = label === 'Source' ? source : target
                      const setValue = label === 'Source' ? setSource : setTarget
                      return (
                        <FormControl key={label} isRequired>
                          <FormLabel htmlFor={`inventory-create-${label.toLowerCase()}`}>{label}</FormLabel>
                          <Select
                            id={`inventory-create-${label.toLowerCase()}`}
                            value={value}
                            onChange={(event) => setValue(event.target.value)}
                            placeholder={`Choose ${label.toLowerCase()}`}
                            isDisabled={saving}
                          >
                            {elements
                              .filter((element) => matches.includes(element) || String(element.id) === value)
                              .map((element) => (
                                <option key={element.id} value={element.id}>
                                  {element.name}
                                  {placed.includes(element) ? ' (in this view)' : ''}
                                </option>
                              ))}
                          </Select>
                        </FormControl>
                      )
                    })}
                  </>
                )}
                <Text fontSize="sm" color="var(--chakra-colors-chakra-subtle-text)">
                  {!elements.length
                    ? 'Create elements before adding a connector.'
                    : 'Choose elements from the inventory. Elements already placed in this view are marked.'}
                </Text>
              </>
            )}
            {error && (
              <Text role="alert" color="red.400">
                {error}
              </Text>
            )}
          </VStack>
        </ModalBody>
        <ModalFooter gap={2}>
          <Button onClick={onClose} isDisabled={saving}>
            Cancel
          </Button>
          <Button type="submit" colorScheme="blue" isLoading={saving} isDisabled={!valid}>
            Create {type}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  )
}
