import { CloseButton, Divider, HStack, Text } from '@chakra-ui/react'
import type { ReactNode } from 'react'

interface Props {
  title: ReactNode
  onClose?: () => void
  hasCloseButton?: boolean
}

export default function PanelHeader({ title, onClose, hasCloseButton = true }: Props) {
  return (
    <>
      <HStack
        px={4}
        pt={4}
        pb={3}
        justify="space-between"
        flexShrink={0}
        bgGradient="linear(to-b, whiteAlpha.50, transparent)"
      >
        <Text fontSize="xs" fontWeight="700" color="white" letterSpacing="0.02em" textTransform="uppercase">
          {title}
        </Text>
        {hasCloseButton && onClose && (
          <CloseButton
            size="sm"
            color="whiteAlpha.600"
            _hover={{ color: 'white', bg: 'whiteAlpha.100' }}
            onClick={onClose}
          />
        )}
      </HStack>
      <Divider borderColor="whiteAlpha.100" />
    </>
  )
}
