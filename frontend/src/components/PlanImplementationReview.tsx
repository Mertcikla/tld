import { Box, Button, HStack, Text } from '@chakra-ui/react'
import type { usePlanImplementation } from '../hooks/usePlanImplementation'

export default function PlanImplementationReview({ implementation }: { implementation: ReturnType<typeof usePlanImplementation> }) {
  const { state, busy, error, review } = implementation
  if (!state?.locked) return error ? <Text role="alert" color="orange.300">{error}</Text> : null
  const pending = state.edits.filter(edit => !['committed', 'skipped', 'discarded'].includes(edit.status))
  const first = pending[0]
  return <Box mt={3} p={3} border="1px solid" borderColor="blue.400" borderRadius="md" data-testid="agent-workspace-review">
    <HStack justify="space-between" flexWrap="wrap">
      <Text fontSize="sm">Workspace locked · {pending.length} staged agent edits</Text>
      <HStack>
        <Button size="sm" isDisabled={busy || !pending.length} onClick={() => void review('commit')}>Commit to workspace</Button>
        <Button size="sm" variant="ghost" isDisabled={busy || !pending.length} onClick={() => void review('discard')}>Discard staged edits</Button>
        {!pending.length && <Button size="sm" variant="ghost" isDisabled={busy} onClick={() => void review('unlock')}>Unlock</Button>}
      </HStack>
    </HStack>
    {state.pendingCommit && <Box role="alert" mt={2}>
      <Text fontSize="sm">New Git commit {state.pendingCommit.slice(0, 7)} detected. Apply the pending diagram edits to the workspace?</Text>
      <HStack mt={2}>
        <Button size="sm" isDisabled={busy} onClick={() => void review('commit')}>Confirm and apply</Button>
        <Button size="sm" isDisabled={busy} onClick={() => void review('discard')}>No, discard staged edits</Button>
      </HStack>
    </Box>}
    {first && ['warning', 'error'].includes(first.status) && <Box mt={2} role="alert">
      <Text fontSize="sm">{first.method}: {first.reason}</Text>
      <HStack mt={2}>
        {first.status === 'warning' && <Button size="sm" isDisabled={busy} onClick={() => void review('ignore', first.id, first.observedHash)}>Ignore change and continue</Button>}
        <Button size="sm" isDisabled={busy} onClick={() => void review('skip', first.id)}>Skip this edit</Button>
      </HStack>
    </Box>}
    {error && <Text role="alert" color="orange.300" mt={2}>{error}</Text>}
  </Box>
}
