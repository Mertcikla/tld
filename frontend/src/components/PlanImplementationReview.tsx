import { useState } from 'react'
import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react'
import type { usePlanImplementation } from '../hooks/usePlanImplementation'
import { resourceLabel, type PlanResource } from '../api/plans'
import PlanResourceDiff from './PlanResourceDiff'

type Implementation = ReturnType<typeof usePlanImplementation>
export default function PlanImplementationReview({ implementation }: { implementation: Implementation }) {
  const { state, plan, context, busy, error, message, review } = implementation
  const [expanded, setExpanded] = useState(false)
  const [confirm, setConfirm] = useState(false)
  if (!state?.locked) return <>{message && <Text role="status" fontSize="sm">{message}</Text>}{error && <Text role="alert" color="orange.300">{error}</Text>}</>
  const pending = state.edits.filter(edit => !['committed', 'skipped', 'discarded'].includes(edit.status))
  const first = pending[0]
  const labels = [...context, ...state.edits.flatMap(edit => edit.operations.flatMap(op => {
    const action = op.action
    const resource = action.case === 'create' ? action.value : action.case === 'update' ? action.value.resource : undefined
    return resource ? [resource] : []
  }))]
  const describe = (resource: PlanResource) => resourceLabel(resource, labels)
  return <Box my={3} p={3} border="1px solid" borderColor="blue.400" borderRadius="md" data-testid="agent-workspace-review">
    <HStack justify="space-between" flexWrap="wrap">
      <Box>
        <Text fontSize="sm" fontWeight="semibold">Implementing {plan?.name || 'plan'} · {pending.length} staged diagram updates</Text>
        <Text fontSize="xs">Workspace edits are staged across all repositories. Review updates here, with or without Watch.</Text>
      </Box>
      <HStack flexWrap="wrap">
        <Button size="sm" isDisabled={busy} onClick={() => void review('finish')}>{pending.length ? 'Apply updates and finish' : 'Finish implementation'}</Button>
        <Button size="sm" variant="ghost" isDisabled={busy} onClick={() => setConfirm(true)}>End without applying</Button>
      </HStack>
    </HStack>
    {confirm && <Box mt={2} role="alert"><Text fontSize="sm">Discard {pending.length} pending diagram updates and release the workspace? Already applied updates and source files remain unchanged.</Text>
      <HStack><Button size="sm" isDisabled={busy} onClick={() => { setConfirm(false); void review('cancel') }}>Discard pending updates and end</Button><Button size="sm" onClick={() => setConfirm(false)}>Keep implementing</Button></HStack>
    </Box>}
    {state.pendingCommit && <Text fontSize="sm" mt={2}>New Git commit {state.pendingCommit.slice(0, 7)} detected. Review the diagram updates and check requirements before finishing.</Text>}
    {!!pending.length && <HStack mt={2}>
      <Button size="sm" variant="ghost" onClick={() => setExpanded(v => !v)}>{expanded ? 'Hide updates' : 'Inspect updates'}</Button>
      <Button size="sm" variant="ghost" isDisabled={busy} onClick={() => void review('commit')}>Apply diagram updates; keep implementing</Button>
    </HStack>}
    {expanded && <VStack align="stretch" spacing={2} mt={2} maxH="320px" overflowY="auto">
      {pending.map(edit => <Box key={edit.id} p={2} borderWidth="1px" borderRadius="md">
        <Text fontSize="xs">{edit.status}</Text>
        {edit.operations.map((op, i) => {
          const a = op.action
          const resource = a.case === 'create' ? a.value : a.case === 'update' ? a.value.resource : op.preconditions.find(p => p.before)?.before
          if (!resource) return null
          const before = op.preconditions.find(p => p.resourceKey === resource.key)?.before
          return <Box key={op.id || i}>
            <Text fontSize="sm">{a.case === 'create' ? 'Add' : a.case === 'remove' ? 'Remove' : 'Update'} {describe(resource)}</Text>
            <PlanResourceDiff before={before} after={a.case === 'remove' ? undefined : resource} fields={a.case === 'update' ? a.value.fields?.paths : undefined} />
          </Box>
        })}
      </Box>)}
    </VStack>}
    {first && ['warning', 'error'].includes(first.status) && <Box mt={2} role="alert">
      <Text fontSize="sm">{first.reason}</Text>
      <HStack mt={2} flexWrap="wrap">
        {first.status === 'warning' && <Button size="sm" isDisabled={busy} onClick={() => void review('ignore', first.id, first.observedHash)}>Apply despite warning</Button>}
        <Button size="sm" isDisabled={busy} onClick={() => void review('skip', first.id)}>Skip this edit</Button>
      </HStack>
    </Box>}
    {message && <Text role="status" fontSize="sm" mt={2}>{message}</Text>}
    {error && <Text role="alert" color="orange.300" mt={2}>{error}</Text>}
  </Box>
}
