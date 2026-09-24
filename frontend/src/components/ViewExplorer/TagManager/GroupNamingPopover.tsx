import React, { useState, useEffect } from 'react'
import {
  PopoverContent,
  PopoverBody,
  PopoverArrow,
  Button,
  HStack,
  Text,
  VStack,
} from '@chakra-ui/react'
import SearchCreateInput from '../../SearchCreateInput'

interface Props {
  isOpen: boolean
  onClose: () => void
  onConfirm: (name: string) => void | Promise<void>
  defaultName: string
  availableNames?: string[]
  anchorEl?: HTMLElement | null
}

export const GroupNamingPopover: React.FC<Props> = ({
  isOpen,
  onClose,
  onConfirm,
  defaultName,
  availableNames = [],
}) => {
  const [name, setName] = useState(defaultName)

  useEffect(() => {
    if (isOpen) {
      setName(defaultName)
    }
  }, [isOpen, defaultName])

  const handleConfirm = async (candidate?: string) => {
    const next = (candidate ?? name).trim()
    if (next) {
      await onConfirm(next)
      onClose()
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      onClose()
    }
  }

  return (
    <PopoverContent
      bg="gray.800"
      border="1px solid"
      borderColor="whiteAlpha.200"
      shadow="2xl"
      w="240px"
      zIndex={2000}
      onKeyDown={handleKeyDown}
    >
      <PopoverArrow bg="gray.800" />
      <PopoverBody p={3}>
        <VStack align="stretch" spacing={2}>
          <Text fontSize="10px" fontWeight="700" color="var(--accent)" textTransform="uppercase">
            New Tag Group
          </Text>
          <HStack spacing={2} align="flex-start">
            <SearchCreateInput
              value={name}
              onChange={setName}
              options={availableNames}
              onSubmit={(value) => { void handleConfirm(value) }}
              submitOnSelect
              autoFocus
              inputTestId="group-naming-input"
              createOptionTestId="group-naming-create-option"
              existingOptionTestId="group-naming-existing-option"
              placeholder="Group name..."
            />
            <Button
              size="xs"
              colorScheme="blue"
              onClick={() => { void handleConfirm() }}
              bg="var(--accent)"
              _hover={{ bg: 'var(--accent-hover)' }}
            >
              Create
            </Button>
          </HStack>
        </VStack>
      </PopoverBody>
    </PopoverContent>
  )
}
