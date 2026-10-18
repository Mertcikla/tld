import { Box, Button, HStack, Text } from '@chakra-ui/react'
import type { PanelSaveState } from '../hooks/usePanelSaveFeedback'

interface Props {
  state: PanelSaveState
  autoSave?: boolean
  onRetry?: () => void
}

export default function PanelSaveStatus({ state, autoSave = false, onRetry }: Props) {
  const message = state === 'error'
    ? 'Changes weren’t saved.'
    : state === 'saving'
      ? 'Saving changes…'
      : state === 'saved'
        ? 'Changes saved.'
        : autoSave ? 'Changes save automatically.' : 'Use Save to apply changes.'

  return (
    <Box px={4} py={2} borderTop="1px solid var(--border-main)" flexShrink={0}>
      <HStack justify="space-between" spacing={2}>
        <Text
          data-testid="panel-save-status"
          role={state === 'error' ? 'alert' : 'status'}
          aria-live={state === 'error' ? 'assertive' : 'polite'}
          fontSize="xs"
          color={state === 'error' ? 'red.300' : 'text.muted'}
        >
          {message}
        </Text>
        {state === 'error' && onRetry && (
          <Button data-testid="panel-save-retry" size="xs" variant="outline" onClick={onRetry}>
            Retry
          </Button>
        )}
      </HStack>
    </Box>
  )
}
