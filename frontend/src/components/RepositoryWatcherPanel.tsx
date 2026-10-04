import { Box, Button, Code, Flex, HStack, Spinner, Text, Tooltip } from '@chakra-ui/react'
import type { RepositoryWatchStatus } from '../api/client'

import { watcherActivity } from '../utils/repositoryWatcher'

export default function RepositoryWatcherPanel({ status, repositoryRoot, branch, revision, busy, onStart, onStop, onRestart, onRefresh }: {
  status: RepositoryWatchStatus | null; repositoryRoot: string; branch: string; revision: string; busy: boolean
  onStart: () => void; onStop: () => void; onRestart: () => void; onRefresh: () => void
}) {
  const activity = watcherActivity(status)
  const stopping = status?.stopRequested || status?.state === 'stopping'
  const unavailable = status?.cliAvailable === false
  return (
    <Box p={4} borderBottom="1px solid" borderColor="whiteAlpha.100" data-testid="repository-watcher">
      <Flex align="center" gap={3} wrap="wrap" mb={2}>
        <HStack spacing={2}>
          {activity.active ? <Spinner size="xs" color="var(--accent)" /> : <Box w="7px" h="7px" borderRadius="full" bg={status?.running ? 'green.300' : 'gray.500'} />}
          <Text fontSize="sm" fontWeight="semibold" data-testid="watch-state" aria-live="polite">{activity.label}</Text>
        </HStack>
        <Text fontSize="xs" color="gray.400">{branch || 'Detached HEAD'} · {revision ? revision.slice(0, 12) : '—'}</Text>
        <Flex ml="auto" gap={2} wrap="wrap">
          {status?.running ? <>
            <Button size="xs" variant="outline" isDisabled={busy || !!stopping} onClick={onStop} data-testid="watch-stop">{stopping ? 'Stopping…' : 'Stop watcher'}</Button>
            <Tooltip label={unavailable ? status?.installHint : 'Stop and start the watcher again'}><Button size="xs" variant="ghost" isDisabled={busy || !!stopping || unavailable} onClick={onRestart} data-testid="watch-restart">Restart</Button></Tooltip>
          </> : <Tooltip label={unavailable ? status?.installHint || 'Install the tld CLI to start a watcher' : undefined}>
            <Button size="xs" colorScheme="green" isLoading={busy} isDisabled={unavailable || !status} onClick={onStart} data-testid="watch-start">{status?.error ? 'Retry watcher' : 'Start watcher'}</Button>
          </Tooltip>}
          <Button size="xs" variant="ghost" isDisabled={busy} onClick={onRefresh} data-testid="watch-refresh">Refresh</Button>
        </Flex>
      </Flex>
      <Text fontSize="xs" color="gray.400" mb={3}>{activity.detail}</Text>
      {status?.running && <HStack spacing={3} mb={3} fontSize="xs" flexWrap="wrap">
        {[['listen', 'Listen'], ['index', 'Index'], ['map', 'Update map']].map(([step, label]) => <Text key={step} color={activity.step === step ? 'var(--accent)' : 'gray.500'} fontWeight={activity.step === step ? 'semibold' : 'normal'}>{label}</Text>)}
      </HStack>}
      <Flex gap={4} wrap="wrap" fontSize="xs" color="gray.400" data-testid="watch-detail">
        <Text>{status?.pendingFiles ?? 0} pending files</Text>
        {!!status?.lastScanUnix && <Text>Last indexed {new Date(status.lastScanUnix * 1000).toLocaleTimeString()}{status.lastScanMs > 0 ? ` · ${(status.lastScanMs / 1000).toFixed(1)}s` : ''}</Text>}
      </Flex>
      {status?.error && <Text fontSize="xs" color="red.300" mt={2}>{status.error}</Text>}
      <Box as="details" mt={3} fontSize="xs" color="gray.500">
        <Text as="summary" cursor="pointer">Details and CLI fallback</Text>
        <Text mt={2}>Live changes compare the current commit with staged, unstaged, and non-ignored untracked files.</Text>
        {status?.running && <Text mt={1}>{status.ownerKind || 'Watcher'} · pid {status.ownerPid} · poll {status.pollIntervalMs} ms · debounce {status.debounceMs} ms</Text>}
        {unavailable && <Text mt={2}>{status?.installHint}</Text>}
        <Text mt={2}>To run the watcher from a terminal:</Text>
        <Code display="block" mt={1} whiteSpace="pre-wrap" wordBreak="break-all">tld index {repositoryRoot} --watch</Code>
      </Box>
    </Box>
  )
}
