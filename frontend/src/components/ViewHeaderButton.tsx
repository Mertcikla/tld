import React from 'react'
import { Box, HStack, IconButton, Tooltip, VStack, Text } from '@chakra-ui/react'
import { EditIcon } from '@chakra-ui/icons'
import { ZoomInIcon } from './Icons'
import { KbdHint } from './PanelUI'

export interface ViewHeaderButtonProps {
  name?: string
  isOpen?: boolean
  onToggle: () => void
  onOpenExplorer?: () => void
  supportsZoomOut?: boolean
  supportsZoomIn?: boolean
}

/**
 * Name: View Name Bar
 * Role: Floating bar that shows the diagram name, opens its details, and reveals it in the explorer.
 * Location: Top center of the canvas, floating.
 * Aliases: Diagram Name Button, View Title Bar.
 */
export default function ViewHeaderButton({
  name,
  isOpen,
  onToggle,
  onOpenExplorer,
  supportsZoomOut = false,
  supportsZoomIn = false,
}: ViewHeaderButtonProps) {
  return (
    <VStack
      position="absolute"
      left="50%"
      top={4}
      transform="translateX(-50%)"
      zIndex={20}
      spacing={1.5}
      align="center"
      pointerEvents="none"
    >
      <Box position="relative" maxW={onOpenExplorer ? 'calc(100vw - 100px)' : 'calc(100vw - 32px)'}>
        {onOpenExplorer && (
          <Tooltip label="Open current view in explorer" placement="bottom" openDelay={300}>
            <IconButton
              aria-label="Open current view in explorer"
              data-testid="view-header-open-explorer"
              icon={<ZoomInIcon size={17} />}
              onClick={onOpenExplorer}
              position="absolute"
              right="-34px"
              top={0}
              zIndex={0}
              pointerEvents="auto"
              w="75px"
              minW="46px"
              h="42px"
              pl="35px"
              bg="color-mix(in srgb, var(--bg-panel) 92%, white)"
              color="var(--accent)"
              borderColor="whiteAlpha.200"
              rounded="xl"
              boxShadow="0 3px 10px rgba(0,0,0,0.25)"
              transition="background 0.2s, color 0.2s"
              _hover={{ bg: 'color-mix(in srgb, var(--bg-panel) 78%, white)', color: 'var(--accent)' }}
              _active={{ bg: 'color-mix(in srgb, var(--bg-panel) 82%, white)' }}
              _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '2px' }}
            />
          </Tooltip>
        )}
        <HStack
          as="button"
          type="button"
          position="relative"
          zIndex={1}
          onClick={onToggle}
          pointerEvents="auto"
          spacing={2.5}
          bg={isOpen ? 'rgba(var(--accent-rgb), 0.12)' : 'var(--bg-panel)'}
          border="1px solid"
          borderColor={isOpen ? 'rgba(var(--accent-rgb), 0.3)' : 'whiteAlpha.100'}
          rounded="xl"
          boxShadow="0 1px 2px rgba(0,0,0,0.42), 0 5px 12px rgba(0,0,0,0.26), 0 10px 18px rgba(0,0,0,0.16)"
          backdropFilter="blur(20px)"
          h="42px"
          maxW="full"
          justify="center"
          px={4}
          py={1.5}
          cursor="pointer"
          role="group"
          transition="all 0.2s"
          _hover={{
            borderColor: isOpen ? 'rgba(var(--accent-rgb), 0.45)' : 'whiteAlpha.200',
            boxShadow: '0 1px 2px rgba(0,0,0,0.46), 0 6px 14px rgba(0,0,0,0.3), 0 12px 20px rgba(0,0,0,0.18)',
          }}
        >
          <EditIcon
            boxSize="15px"
            color="var(--accent)"
            opacity={isOpen ? 1 : 0.55}
            transition="all 0.2s"
            _groupHover={{ opacity: 1 }}
            flexShrink={0}
          />
          <Text
            fontSize="sm"
            color={isOpen ? 'var(--accent)' : 'whiteAlpha.900'}
            fontWeight="500"
            letterSpacing="0.01em"
            noOfLines={1}
            textShadow="0 1px 0 rgba(0,0,0,0.22)"
            transition="color 0.2s"
          >
            {name || 'Untitled View'}
          </Text>
        </HStack>
      </Box>
      <HStack
        spacing={1.5}
        opacity={0.6}
        userSelect="none"
        pointerEvents="none"
        // Include the explorer button's 34px overhang when centering the hints.
        transform={onOpenExplorer ? 'translateX(17px)' : undefined}
      >
        <KbdHint ml={0}>V</KbdHint>
        <Text fontSize="10px" fontWeight="bold" color="whiteAlpha.600">Details</Text>
        {supportsZoomOut && (
          <HStack spacing={1.5}>
            <KbdHint ml={0}>W</KbdHint>
            <Text fontSize="10px" fontWeight="bold" color="whiteAlpha.600">Zoom Out</Text>
          </HStack>
        )}
        {supportsZoomIn && (
          <HStack spacing={1.5}>
            <KbdHint ml={0}>S</KbdHint>
            <Text fontSize="10px" fontWeight="bold" color="whiteAlpha.600">Zoom In</Text>
          </HStack>
        )}
      </HStack>
    </VStack>
  )
}
