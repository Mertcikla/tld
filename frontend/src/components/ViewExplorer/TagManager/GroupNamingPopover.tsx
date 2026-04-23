import React, { useState, useEffect, useRef } from 'react'
import {
  PopoverContent,
  PopoverBody,
  PopoverArrow,
  Input,
  Button,
  HStack,
  Text,
  VStack,
} from '@chakra-ui/react'

interface Props {
  isOpen: boolean
  onClose: () => void
  onConfirm: (name: string) => void | Promise<void>
  defaultName: string
  anchorEl?: HTMLElement | null
}

export const GroupNamingPopover: React.FC<Props> = ({
  isOpen,
  onClose,
  onConfirm,
  defaultName,
}) => {
  const [name, setName] = useState(defaultName)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (isOpen) {
      setName(defaultName)
      // Focus after a short delay to ensure popover is rendered
      setTimeout(() => inputRef.current?.focus(), 100)
    }
  }, [isOpen, defaultName])

  const handleConfirm = async () => {
    if (name.trim()) {
      await onConfirm(name.trim())
      onClose()
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      void handleConfirm()
    } else if (e.key === 'Escape') {
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
          <HStack spacing={2}>
            <Input
              ref={inputRef}
              size="xs"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Group name..."
              bg="whiteAlpha.50"
              borderColor="whiteAlpha.100"
              color="white"
              _focus={{ borderColor: 'var(--accent)' }}
            />
            <Button
              size="xs"
              colorScheme="blue"
              onClick={handleConfirm}
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
