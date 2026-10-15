import { ChevronRightIcon, RepeatIcon, TimeIcon } from '@chakra-ui/icons'
import { Box, Button, Code, Flex, Grid, HStack, Icon, Spinner, Switch, Text, Tooltip } from '@chakra-ui/react'
import type { RepositoryImpactScene, RepositoryWatchStatus } from '../api/client'
import type { MapChangeTarget } from '../utils/repositoryChangeHistory'
import RepositoryChangeHistory from './RepositoryChangeHistory'

import { watcherActivity, type RepositoryFollowMode } from '../utils/repositoryWatcher'

export default function RepositoryWatcherPanel({ status, repositoryRoot, branch, revision, busy, onStart, onStop, onRestart, onRefresh, scene = null, onFocusChanges, followMode = 'off', onFollowModeChange }: {
  status: RepositoryWatchStatus | null; repositoryRoot: string; branch: string; revision: string; busy: boolean
  scene?: RepositoryImpactScene | null; onFocusChanges?: (targets: MapChangeTarget[]) => void
  followMode?: RepositoryFollowMode; onFollowModeChange?: (mode: RepositoryFollowMode) => void
  onStart: () => void; onStop: () => void; onRestart: () => void; onRefresh: () => void
}) {
  const activity = watcherActivity(status)
  const stopping = status?.stopRequested || status?.state === 'stopping'
  const unavailable = status?.cliAvailable === false
  const hasError = !!status?.error || status?.state === 'error'
  const statusColor = hasError ? 'orange.300' : status?.running ? 'green.300' : 'gray.400'
  const statusLabel = !status ? 'Connecting' : stopping ? 'Stopping' : hasError ? 'Needs attention' : status.running ? 'Live' : 'Stopped'
  const lastIndexed = status?.lastScanUnix ? new Date(status.lastScanUnix * 1000) : null

  return (
    <Box as="section" aria-label="Repository watcher" px={{ base: 4, md: 5 }} pt={4} pb={3} borderBottom="1px solid" borderColor="whiteAlpha.200" bg="var(--bg-canvas)" flexShrink={0} data-testid="repository-watcher">
      <Flex align="start" justify="space-between" gap={4} wrap="wrap">
        <HStack spacing={3} minW={0} flex="1 1 260px" align="start">
          <Flex aria-hidden="true" boxSize="38px" flexShrink={0} align="center" justify="center" borderRadius="xl" color="var(--accent)" bg="rgba(var(--accent-rgb), 0.10)" border="1px solid" borderColor="rgba(var(--accent-rgb), 0.20)">
            <Icon viewBox="0 0 24 24" boxSize={5} fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
              <path d="M3 12h4l3-7 4 14 3-7h4" />
            </Icon>
          </Flex>
          <Box minW={0}>
            <HStack spacing={2} mb={1}>
              <Text fontSize="10px" fontWeight="bold" letterSpacing="0.12em" color="gray.400" textTransform="uppercase">Live watch</Text>
              <HStack spacing={1.5} px={1.5} py="1px" borderRadius="md" bg="whiteAlpha.50" color={statusColor} fontSize="10px" fontWeight="semibold">
                <Box boxSize="5px" bg="currentColor" borderRadius="full" aria-hidden="true" />
                <Text>{statusLabel}</Text>
              </HStack>
            </HStack>
            <HStack spacing={2}>
              {activity.active && <Spinner size="xs" color="var(--accent)" />}
              <Text fontSize="md" lineHeight="short" fontWeight="semibold" letterSpacing="-0.02em" data-testid="watch-state" aria-live="polite">{activity.label === 'Idle · watching for changes' ? 'Watching for changes' : activity.label}</Text>
            </HStack>
            <Text fontSize="xs" color="gray.400" mt={1}>{activity.detail}</Text>
          </Box>
        </HStack>
        <Flex direction="column" align="end" gap={1.5} flexShrink={0}>
          <HStack spacing={1.5} flexWrap="wrap" rowGap={2}>
            <Tooltip label="Refresh watcher status">
              <Button size="xs" h="30px" px={2.5} variant="ghost" color="gray.400" isDisabled={busy} onClick={onRefresh} data-testid="watch-refresh">Refresh</Button>
            </Tooltip>
            {status?.running ? <>
              <Tooltip label={unavailable ? status?.installHint || 'Install the tld CLI to restart the watcher' : 'Stop and start the watcher again'}>
                <Button size="xs" h="30px" px={2.5} variant="ghost" leftIcon={<RepeatIcon />} isDisabled={busy || !!stopping || unavailable} onClick={onRestart} data-testid="watch-restart">Restart</Button>
              </Tooltip>
              <Button size="xs" h="30px" px={3} variant="outline" borderColor="whiteAlpha.300" leftIcon={<Box boxSize="6px" bg="currentColor" borderRadius="1px" />} isDisabled={busy || !!stopping} onClick={onStop} data-testid="watch-stop">{stopping ? 'Stopping…' : 'Stop watcher'}</Button>
            </> : <Tooltip label={unavailable ? status?.installHint || 'Install the tld CLI to start a watcher' : undefined}>
              <Button size="sm" h="30px" px={4} bg="var(--accent)" color="white" fontWeight="semibold" _hover={{ bg: 'var(--accent)', filter: 'brightness(1.08)' }} _active={{ bg: 'var(--accent)', filter: 'brightness(0.95)' }} isLoading={busy} isDisabled={unavailable || !status} onClick={onStart} data-testid="watch-start">{status?.error ? 'Retry watcher' : 'Start watcher'}</Button>
            </Tooltip>}
          </HStack>
          {onFollowModeChange && <HStack spacing={1}>
            <Tooltip label="Keep changed code in view. Manual navigation pauses following.">
              <Switch size="sm" minH="24px" display="flex" alignItems="center" aria-label="Follow changes" isChecked={followMode === 'following'} onChange={(event) => onFollowModeChange(event.target.checked ? 'following' : 'off')} sx={{ '& .chakra-switch__track[data-checked]': { bg: 'var(--accent)' } }}>
                <Text as="span" fontSize="xs" color="gray.400">Follow changes</Text>
              </Switch>
            </Tooltip>
          </HStack>}
        </Flex>
      </Flex>

      <RepositoryChangeHistory scene={scene} onFocus={onFocusChanges} summary={
        <HStack spacing={4} fontSize="xs" color="gray.400" flexWrap="wrap" rowGap={1} data-testid="watch-detail">
          <Text><Text as="span" fontWeight="semibold" color={status?.pendingFiles ? 'var(--accent)' : 'gray.200'}>{status?.pendingFiles ?? 0}</Text> pending files</Text>
          <HStack spacing={1.5} title={lastIndexed?.toLocaleString()}>
            <TimeIcon boxSize={3} aria-hidden="true" />
            <Text>{lastIndexed ? <>Indexed {lastIndexed.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}{status && status.lastScanMs > 0 && <Text as="span" color="gray.500"> · {(status.lastScanMs / 1000).toFixed(1)}s</Text>}</> : 'No index yet'}</Text>
          </HStack>
        </HStack>
      } />

      {status?.error && <Box role="alert" borderLeft="2px solid" borderColor="orange.300" bg="rgba(246, 173, 85, 0.06)" px={3} py={2} mb={3} borderRadius="sm"><Text fontSize="xs" color="orange.200" overflowWrap="anywhere">{status.error}</Text></Box>}
      <Flex borderTop="1px solid" borderColor="whiteAlpha.100" pt={2.5} gap={2} wrap="wrap" align="start">
        {(branch || revision) && <HStack spacing={2} minW={0} maxW="full" flex="1" fontSize="xs" color="gray.400" aria-label="Watched Git revision">
          <Icon viewBox="0 0 16 16" boxSize="13px" fill="none" stroke="currentColor" strokeWidth="1.3" flexShrink={0} aria-hidden="true"><circle cx="4" cy="3" r="1.5" /><circle cx="4" cy="13" r="1.5" /><circle cx="12" cy="3" r="1.5" /><path d="M4 4.5v7M12 4.5v1A4.5 4.5 0 0 1 7.5 10H4" /></Icon>
          {branch && <Text isTruncated title={branch} color="gray.300">{branch}</Text>}
          {revision && <Text fontFamily="mono" fontSize="10px" flexShrink={0} title={revision} px={1.5} py="1px" borderRadius="sm" bg="whiteAlpha.50">{revision.slice(0, 7)}</Text>}
        </HStack>}
        <Box as="details" ml="auto" minW={0} maxW="full" fontSize="xs" color="gray.400" sx={{ '&[open]': { flexBasis: '100%' }, '&[open] .watch-details-chevron': { transform: 'rotate(90deg)' } }}>
          <Flex as="summary" w="fit-content" ml="auto" align="center" gap={1} cursor="pointer" listStyleType="none" borderRadius="sm" _hover={{ color: 'gray.100' }} _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '3px' }} sx={{ '&::-webkit-details-marker': { display: 'none' } }}>
            Watcher details <ChevronRightIcon className="watch-details-chevron" boxSize={3.5} />
          </Flex>
          <Box mt={3} p={3} border="1px solid" borderColor="whiteAlpha.100" borderRadius="md" bg="var(--bg-panel)">
            <Text lineHeight="tall">Live changes compare the current commit with staged, unstaged, and non-ignored untracked files.</Text>
            {status?.running && <Grid templateColumns="repeat(auto-fit, minmax(130px, 1fr))" gap={3} mt={3} mb={3}>
              <Box><Text color="gray.500" mb={1}>Process</Text><Text>{status.ownerKind || 'Watcher'} · pid {status.ownerPid}</Text></Box>
              <Box><Text color="gray.500" mb={1}>Poll interval</Text><Text>{status.pollIntervalMs} ms</Text></Box>
              <Box><Text color="gray.500" mb={1}>Debounce</Text><Text>{status.debounceMs} ms</Text></Box>
            </Grid>}
            {unavailable && <Text mt={2} color="orange.200">{status?.installHint || 'Install the tld CLI to start a watcher.'}</Text>}
            <Text mt={3} mb={1.5} color="gray.300">Run from your terminal</Text>
            <Code display="block" p={2} borderRadius="md" fontSize="xs" whiteSpace="pre-wrap" wordBreak="break-all">tld index {repositoryRoot} --watch</Code>
          </Box>
        </Box>
      </Flex>
    </Box>
  )
}
