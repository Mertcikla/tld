import { CloseButton, HStack, Text } from '@chakra-ui/react'
import type { ReactNode } from 'react'

interface Props {
  title: ReactNode
  onClose?: () => void
  hasCloseButton?: boolean
  isInline?: boolean
  actions?: ReactNode
}

export default function PanelHeader({ title, onClose, hasCloseButton = true, actions }: Props) {
  const headerContent = (
    <>
      <Text fontSize="xs" fontWeight="700" color="white" letterSpacing="0.02em" textTransform="uppercase">
        {title}
      </Text>
      <HStack spacing={1}>
        {actions}
        {hasCloseButton && onClose && (
          <CloseButton
            data-testid="panel-close"
            size="sm"
            color="whiteAlpha.600"
            _hover={{ color: 'white', bg: 'whiteAlpha.100' }}
            onClick={onClose}
          />
        )}
      </HStack>
    </>
  )

  return (
    <HStack
      h="44px"
      minH="44px"
      px={4}
      justify="space-between"
      flexShrink={0}
      borderBottom="1px solid"
      borderColor="var(--border-main)"
    >
      {headerContent}
    </HStack>
  )
}
