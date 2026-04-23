import { useEffect, useMemo, useState } from 'react'
import {
  Box,
  Button,
  Flex,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Text,
} from '@chakra-ui/react'
import type { ViewTreeNode } from '../types'

interface Props {
  isOpen: boolean
  onClose: () => void
  targetId: number | null
  treeData: ViewTreeNode[]
  onConfirm: (childId: number) => Promise<void>
}

export default function SetChildModal({ isOpen, onClose, targetId, treeData, onConfirm }: Props) {
  const [search, setSearch] = useState('')
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (isOpen) {
      setSearch('')
      setSelectedId(null)
    }
  }, [isOpen])

  const targetNode = useMemo(
    () => treeData.find((n) => n.id === targetId),
    [treeData, targetId]
  )

  // Forbidden: self + all ancestors of the target (to prevent cycles)
  const forbiddenIds = useMemo(() => {
    if (targetId === null) return new Set<number>()
    const ancestors = new Set<number>()
    ancestors.add(targetId)
    let current = treeData.find((n) => n.id === targetId)
    while (current && current.parent_view_id !== null) {
      ancestors.add(current.parent_view_id)
      current = treeData.find((n) => n.id === current!.parent_view_id)
    }
    return ancestors
  }, [treeData, targetId])

  // IDs that are already direct children of this diagram
  const existingChildIds = useMemo(() => {
    if (targetId === null) return new Set<number>()
    return new Set(treeData.filter((n) => n.parent_view_id === targetId).map((n) => n.id))
  }, [treeData, targetId])

  const filteredList = useMemo(() => {
    const q = search.trim().toLowerCase()
    return treeData
      .filter((n) => !forbiddenIds.has(n.id))
      .filter((n) => !q || n.name.toLowerCase().includes(q))
      .sort((a, b) => a.name.localeCompare(b.name))
  }, [treeData, forbiddenIds, search])

  const handleConfirm = async () => {
    if (selectedId === null) return
    setSaving(true)
    try {
      await onConfirm(selectedId)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal isOpen={isOpen} onClose={onClose} isCentered size="sm">
      <ModalOverlay bg="blackAlpha.700" backdropFilter="blur(4px)" />
      <ModalContent
        bg="var(--bg-panel)"
        border="1px solid"
        borderColor="var(--border-main)"
        borderRadius="12px"
        boxShadow="0 20px 60px rgba(0,0,0,0.6)"
      >
        <ModalHeader color="gray.100" pb={2} fontSize="md" fontWeight="semibold">
          Add Existing Child
        </ModalHeader>
        <ModalBody pb={0}>
          {targetNode && (
            <Text fontSize="xs" color="gray.500" mb={3}>
              Parent: <Text as="span" color="gray.300" fontWeight="medium">{targetNode.name}</Text>
            </Text>
          )}
          <Input
            placeholder="Search diagrams…"
            size="sm"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            mb={2}
            autoFocus
            bg="var(--bg-canvas)"
            borderColor="gray.600"
            color="gray.200"
            _placeholder={{ color: 'gray.600' }}
            _focus={{ borderColor: 'var(--accent)', boxShadow: '0 0 0 1px var(--accent)' }}
            onKeyDown={(e) => { if (e.key === 'Enter' && selectedId) handleConfirm() }}
          />
          <Box
            maxH="260px"
            overflowY="auto"
            borderRadius="8px"
            border="1px solid"
            borderColor="var(--border-main)"
            css={{ '&::-webkit-scrollbar': { width: '4px' }, '&::-webkit-scrollbar-thumb': { background: '#4a5568', borderRadius: '2px' } }}
          >
            {filteredList.length === 0 && (
              <Box px={3} py={4} textAlign="center">
                <Text fontSize="sm" color="gray.600">No diagrams available</Text>
              </Box>
            )}
            {filteredList.map((n, idx) => {
              const isAlreadyChild = existingChildIds.has(n.id)
              const isSelected = selectedId === n.id
              return (
                <Flex
                  key={n.id}
                  px={3}
                  py="9px"
                  align="center"
                  justify="space-between"
                  cursor="pointer"
                  bg={isSelected ? 'rgba(var(--accent-rgb), 0.12)' : 'transparent'}
                  borderBottom={idx < filteredList.length - 1 ? '1px solid' : 'none'}
                  borderColor="var(--border-main)"
                  transition="background 0.1s"
                  _hover={{ bg: isSelected ? 'rgba(var(--accent-rgb), 0.16)' : 'whiteAlpha.50' }}
                  onClick={() => setSelectedId(isSelected ? null : n.id)}
                >
                  <Text
                    fontSize="sm"
                    color={isSelected ? 'blue.200' : 'gray.300'}
                    fontWeight={isSelected ? 'medium' : 'normal'}
                    noOfLines={1}
                    flex={1}
                    minW={0}
                    pr={2}
                  >
                    {n.name}
                  </Text>
                  <Flex align="center" gap={2} flexShrink={0}>
                    {isAlreadyChild && (
                      <Text fontSize="8px" color="blue.500" letterSpacing="0.1em" fontWeight="bold" textTransform="uppercase">
                        child
                      </Text>
                    )}
                    {n.parent_view_id !== null && !isAlreadyChild && (
                      <Text fontSize="8px" color="gray.600" letterSpacing="0.08em" fontWeight="bold" textTransform="uppercase">
                        has parent
                      </Text>
                    )}
                    {isSelected && (
                      <Box color="blue.400" fontSize="13px" lineHeight={1}>✓</Box>
                    )}
                  </Flex>
                </Flex>
              )
            })}
          </Box>
        </ModalBody>
        <ModalFooter gap={2} pt={3}>
          <Button size="sm" variant="ghost" color="gray.500" _hover={{ color: 'gray.300', bg: 'whiteAlpha.100' }} onClick={onClose}>
            Cancel
          </Button>
          <Button
            size="sm"
            colorScheme="blue"
            isDisabled={selectedId === null}
            isLoading={saving}
            onClick={handleConfirm}
          >
            Set as Child
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  )
}
