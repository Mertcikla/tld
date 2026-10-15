import { RepeatIcon, TimeIcon } from '@chakra-ui/icons'
import { faCodeBranch } from '@fortawesome/free-solid-svg-icons'
import { Box, Button, Code, Flex, Grid, HStack, Popover, PopoverBody, PopoverContent, PopoverTrigger, Portal, Spinner, Switch, Text, Tooltip } from '@chakra-ui/react'
import type { RepositoryImpactScene, RepositoryWatchStatus } from '../api/client'
import type { MapChangeTarget } from '../utils/repositoryChangeHistory'
import RepositoryChangeHistory from './RepositoryChangeHistory'
import FontAwesomeIcon from './FontAwesomeIcon'

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
          {(branch || revision) && <Box aria-label="Watched Git revision" minW={0} maxW={{ base: '130px', md: '190px' }} flexShrink={0} border="1px solid" borderColor="rgba(var(--accent-rgb), 0.20)" bg="rgba(var(--accent-rgb), 0.05)" borderRadius="lg" px={3} py={2}>
            <HStack spacing={1.5} color="var(--accent)" minW={0}>
              <FontAwesomeIcon icon={faCodeBranch} size={12} />
              <Text fontSize="xs" fontWeight="semibold" isTruncated title={branch || 'Detached HEAD'}>{branch || 'Detached HEAD'}</Text>
            </HStack>
            {revision && <HStack mt={1.5} spacing={1.5}>
              <Text fontSize="9px" color="gray.500" letterSpacing="0.05em">HEAD</Text>
              <Text fontFamily="mono" fontSize="10px" color="gray.300" title={revision}>{revision.slice(0, 7)}</Text>
            </HStack>}
          </Box>}
          <Box minW={0}>
            <HStack spacing={2} mb={1}>
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
            {status?.running ? <>
              <Button size="xs" h="30px" px={3} variant="outline" borderColor="whiteAlpha.300" leftIcon={<Box boxSize="6px" bg="currentColor" borderRadius="1px" />} isDisabled={busy || !!stopping} onClick={onStop} data-testid="watch-stop">{stopping ? 'Stopping…' : 'Stop watcher'}</Button>
            </> : <Tooltip label={unavailable ? status?.installHint || 'Install the tld CLI to start a watcher' : undefined}>
              <Button size="sm" h="30px" px={4} bg="var(--accent)" color="white" fontWeight="semibold" _hover={{ bg: 'var(--accent)', filter: 'brightness(1.08)' }} _active={{ bg: 'var(--accent)', filter: 'brightness(0.95)' }} isLoading={busy} isDisabled={unavailable || !status} onClick={onStart} data-testid="watch-start">{status?.error ? 'Retry watcher' : 'Start watcher'}</Button>
            </Tooltip>}
            <Popover placement="bottom-end" isLazy>
              <PopoverTrigger>
                <Button size="xs" boxSize="30px" minW="30px" variant="ghost" aria-label="Watcher options" fontSize="lg" lineHeight={1}>…</Button>
              </PopoverTrigger>
              <Portal>
                <PopoverContent w="360px" maxW="calc(100vw - 32px)" bg="var(--bg-panel)" borderColor="whiteAlpha.200" borderRadius="lg" boxShadow="lg" _focus={{ boxShadow: 'lg' }} data-testid="watch-options-panel">
                  <PopoverBody p={0} maxH="min(440px, calc(100dvh - 100px))" overflowY="auto">
                    <Box p={1.5} borderBottom="1px solid" borderColor="whiteAlpha.100">
                      {status?.running && <Button size="sm" w="full" justifyContent="flex-start" variant="ghost" fontSize="xs" leftIcon={<RepeatIcon />} isDisabled={busy || !!stopping || unavailable} title={unavailable ? status?.installHint || 'Install the tld CLI to restart the watcher' : undefined} onClick={onRestart} data-testid="watch-restart">Restart watcher</Button>}
                      <Button size="sm" w="full" justifyContent="flex-start" variant="ghost" fontSize="xs" isDisabled={busy} onClick={onRefresh} data-testid="watch-refresh">Refresh status</Button>
                    </Box>
                    <Box p={3} fontSize="xs" color="gray.400">
                      <Text fontWeight="semibold" color="gray.300" mb={2}>Watcher details</Text>
                      <Text lineHeight="tall">Live changes compare the current commit with staged, unstaged, and non-ignored untracked files.</Text>
                      {status?.running && <Grid templateColumns="repeat(2, minmax(0, 1fr))" gap={3} mt={3} mb={3}>
                        <Box><Text color="gray.500" mb={1}>Process</Text><Text overflowWrap="anywhere">{status.ownerKind || 'Watcher'} · pid {status.ownerPid}</Text></Box>
                        <Box><Text color="gray.500" mb={1}>Debounce</Text><Text>{status.debounceMs} ms</Text></Box>
                      </Grid>}
                      {unavailable && <Text mt={2} color="orange.200">{status?.installHint || 'Install the tld CLI to start a watcher.'}</Text>}
                      <Text mt={3} mb={1.5} color="gray.300">Run from your terminal</Text>
                      <Code display="block" p={2} borderRadius="md" fontSize="xs" whiteSpace="pre-wrap" wordBreak="break-all">tld index {repositoryRoot} --watch</Code>
                    </Box>
                  </PopoverBody>
                </PopoverContent>
              </Portal>
            </Popover>
          </HStack>
          {onFollowModeChange && <HStack spacing={1}>
            <Tooltip label="Keep changed code in view. Manual navigation pauses following." openDelay={300} closeOnPointerDown closeOnScroll>
              <Box as="span" display="inline-flex">
                <Switch size="sm" minH="24px" display="flex" alignItems="center" aria-label="Follow changes" isChecked={followMode === 'following'} onChange={(event) => onFollowModeChange(event.target.checked ? 'following' : 'off')} sx={{ '& .chakra-switch__track[data-checked]': { bg: 'var(--accent)' } }}>
                  <Text as="span" fontSize="xs" color="gray.400">Follow changes</Text>
                </Switch>
              </Box>
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

    </Box>
  )
}
