import React from 'react'
import { HStack, Text } from '@chakra-ui/react'
import { EditIcon } from '@chakra-ui/icons' // Or from './Icons' ? Let's use the one from chakra as in original code. Wait, original code uses from chakra. No, original code says: import { ChevronLeftIcon, ChevronRightIcon, DownloadIcon, EditIcon } from '@chakra-ui/icons'

export interface ViewHeaderButtonProps {
  name?: string
  onOpen: () => void
}

/**
 * Name: Diagram Button
 * Role: Appears on the headerbar and it opens the diagram details panel.
 * Location: Headerbar at the top.
 * Aliases: Diagram Name Button, Header Title.
 */
export default function ViewHeaderButton({ name, onOpen }: ViewHeaderButtonProps) {
  return (
    <HStack
      as="button"
      onClick={onOpen}
      spacing={2.5}
      bg="linear-gradient(180deg, rgba(var(--bg-main-rgb), 0.98) 0%, rgba(var(--bg-main-rgb), 0.9) 100%)"
      border="1px solid"
      borderTop="none"
      borderColor="whiteAlpha.100"
      px={5}
      py={0}
      borderBottomRadius="12px"
      borderTopRadius="0"
      maxW="460px"
      minH="34px"
      top="39px"
      cursor="pointer"
      position="relative"
      alignSelf="flex-start"
      boxShadow="0 8px 16px rgba(0,0,0,0.22), 0 2px 4px rgba(0,0,0,0.18), inset 0 -1px 0 rgba(255,255,255,0.04)"
      transition="all 0.2s"
      role="group"
    >
      <EditIcon
        boxSize="11px"
        color="var(--accent)"
        opacity={0.55}
        transition="all 0.2s"
        _groupHover={{ opacity: 1 }}
        flexShrink={0}
      />
      <Text
        fontSize="sm"
        color="whiteAlpha.900"
        fontWeight="700"
        letterSpacing="0.01em"
        noOfLines={1}
        textShadow="0 1px 0 rgba(0,0,0,0.22)"
      >
        {name || 'Untitled View'}
      </Text>
    </HStack>
  )
}
