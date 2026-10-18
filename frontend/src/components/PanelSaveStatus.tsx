import { Box, Button, HStack, Text } from '@chakra-ui/react'
import { useEffect, useState } from 'react'
import type { PanelSaveState } from '../hooks/usePanelSaveFeedback'

interface Props {
  state: PanelSaveState
  hasFooter?: boolean
  onRetry?: () => void
}

export default function PanelSaveStatus({ state, hasFooter = false, onRetry }: Props) {
  const [visibleState, setVisibleState] = useState<PanelSaveState>('idle')

  useEffect(() => {
    if (state === 'saving') {
      // Fast saves can finish without flashing an intermediate message.
      const timer = setTimeout(() => setVisibleState('saving'), 300)
      return () => clearTimeout(timer)
    }
    setVisibleState(state)
    if (state === 'saved') {
      const timer = setTimeout(() => setVisibleState('idle'), 2000)
      return () => clearTimeout(timer)
    }
  }, [state])

  if (visibleState === 'idle') return null

  const message = visibleState === 'error'
    ? 'Changes weren’t saved.'
    : visibleState === 'saving'
      ? 'Saving changes…'
      : 'Changes saved.'

  return (
    <Box data-testid="panel-save-feedback" position="absolute" left={3} right={3} bottom={hasFooter ? '68px' : 3} zIndex={2} display="flex" justifyContent="center" pointerEvents="none">
      <HStack maxW="full" minH={8} px={3} py={1.5} spacing={2} bg="var(--bg-menu)" border="1px solid var(--border-main)" borderRadius="lg" boxShadow="panel-sm" backdropFilter="blur(12px)">
        <Text
          data-testid="panel-save-status"
          role={visibleState === 'error' ? 'alert' : 'status'}
          aria-live={visibleState === 'error' ? 'assertive' : 'polite'}
          fontSize="xs"
          lineHeight="18px"
          minW={0}
          isTruncated
          color={visibleState === 'error' ? 'red.300' : 'text.muted'}
        >
          {message}
        </Text>
        {visibleState === 'error' && onRetry && (
          <Button data-testid="panel-save-retry" size="xs" variant="outline" flexShrink={0} pointerEvents="auto" isDisabled={state === 'saving'} onClick={state === 'error' ? onRetry : undefined}>
            Retry
          </Button>
        )}
      </HStack>
    </Box>
  )
}
