import { useEffect, useState } from 'react'
import { RepeatIcon, TimeIcon } from '@chakra-ui/icons'
import { faCodeBranch, faEllipsis, faPlay } from '@fortawesome/free-solid-svg-icons'
import { Box, Button, Code, Flex, Grid, HStack, Popover, PopoverBody, PopoverContent, PopoverTrigger, Portal, Spinner, Switch, Text, Tooltip } from '@chakra-ui/react'
import type { RepositoryImpactScene, RepositoryWatchStatus } from '../api/client'
import type { MapChangeTarget } from '../utils/repositoryChangeHistory'
import RepositoryChangeHistory from './RepositoryChangeHistory'
import FontAwesomeIcon from './FontAwesomeIcon'

import { watcherActivity, type RepositoryFollowMode } from '../utils/repositoryWatcher'

export default function RepositoryWatcherPanel({ status, repositoryRoot, branch, revision, commitMessage = '', worktreeStats = null, busy, onStart, onStop, onRestart, scene = null, onFocusChanges, followMode = 'off', onFollowModeChange }: {
  status: RepositoryWatchStatus | null; repositoryRoot: string; branch: string; revision: string; commitMessage?: string; busy: boolean
  worktreeStats?: { additions: number; deletions: number } | null
  scene?: RepositoryImpactScene | null; onFocusChanges?: (targets: MapChangeTarget[]) => void
  followMode?: RepositoryFollowMode; onFollowModeChange?: (mode: RepositoryFollowMode) => void
  onStart: () => void; onStop: () => void; onRestart: () => void
}) {
  const [lastGit, setLastGit] = useState({ repositoryRoot, branch, revision, commitMessage })
  useEffect(() => {
    // Startup publishes empty Git metadata before the first capture finishes.
    if (branch || revision || repositoryRoot !== lastGit.repositoryRoot) {
      setLastGit((previous) => ({ repositoryRoot, branch, revision, commitMessage: commitMessage || (previous.repositoryRoot === repositoryRoot && previous.revision === revision ? previous.commitMessage : '') }))
    }
  }, [repositoryRoot, branch, revision, commitMessage, lastGit.repositoryRoot])
  const git = branch || revision ? { branch, revision, commitMessage: commitMessage || (lastGit.repositoryRoot === repositoryRoot && lastGit.revision === revision ? lastGit.commitMessage : '') } : lastGit.repositoryRoot === repositoryRoot ? lastGit : { branch: '', revision: '', commitMessage: '' }
  const activity = watcherActivity(status)
  const stopping = status?.stopRequested || status?.state === 'stopping'
  const unavailable = status?.cliAvailable === false
  const hasError = !!status?.error || status?.state === 'error'
  const statusColor = hasError ? 'orange.300' : status?.running ? 'green.300' : 'gray.400'
  const lastIndexed = status?.lastScanUnix ? new Date(status.lastScanUnix * 1000) : null
  const following = followMode === 'following'

  return (
    <Box as="section" aria-label="Repository watcher" px={{ base: 4, md: 5 }} pt={4} pb={3} borderBottom="1px solid" borderColor="whiteAlpha.200" bg="var(--bg-canvas)" flexShrink={0} data-testid="repository-watcher">
      <Flex align="stretch" justify="space-between" gap={4} wrap="wrap">
        <HStack spacing={3} minW={0} flex="1 1 260px" align="start">
          {(git.branch || git.revision) && <Box aria-label="Watched Git revision" minW={0} w={{ base: '160px', md: '280px' }} maxW="full" flexShrink={0} border="1px solid" borderColor="rgba(var(--accent-rgb), 0.20)" bg="rgba(var(--accent-rgb), 0.05)" borderRadius="lg" px={3} py={2}>
            <HStack spacing={1.5} color="var(--accent)" minW={0}>
              <FontAwesomeIcon icon={faCodeBranch} size={12} />
              <Text flex={1} minW={0} fontSize="xs" fontWeight="semibold" isTruncated title={git.branch || 'Detached HEAD'}>{git.branch || 'Detached HEAD'}</Text>
              <HStack spacing={1.5} flexShrink={0} fontFamily="mono" fontSize="10px" fontWeight="medium" aria-label="Worktree line changes" title={worktreeStats ? `Worktree against the current commit: ${worktreeStats.additions.toLocaleString()} lines added, ${worktreeStats.deletions.toLocaleString()} lines deleted` : 'Loading worktree line totals.'}>
                <Text color="green.300" data-testid="watch-lines-added">+{worktreeStats ? worktreeStats.additions.toLocaleString() : '—'}</Text>
                <Text color="red.300" data-testid="watch-lines-removed">−{worktreeStats ? worktreeStats.deletions.toLocaleString() : '—'}</Text>
              </HStack>
            </HStack>
            {git.revision && <HStack mt={1.5} spacing={1.5}>
              {git.commitMessage && <Text flex={1} minW={0} fontSize="10px" color="gray.400" isTruncated title={git.commitMessage} data-testid="watch-commit-message">{git.commitMessage.split('\n')[0]}</Text>}
              <Text fontFamily="mono" fontSize="10px" color="gray.500" flexShrink={0} title={git.revision}>{git.revision.slice(0, 7)}</Text>
            </HStack>}
          </Box>}
          <Box minW={0} alignSelf="center">
            <HStack spacing={2}>
              {activity.active ? <Spinner size="xs" color="var(--accent)" /> : <Box boxSize="6px" flexShrink={0} bg={statusColor} borderRadius="full" aria-hidden="true" />}
              <Text fontSize={{ base: 'lg', md: 'xl' }} lineHeight="short" fontWeight="semibold" letterSpacing="-0.02em" title={activity.detail} data-testid="watch-state" aria-live="polite">{activity.label === 'Idle · watching for changes' ? 'Watching for changes' : activity.label}</Text>
            </HStack>
          </Box>
        </HStack>
        <Box display="flex" alignSelf="stretch" w="fit-content" maxW="full" flexShrink={0} border="1px solid" borderColor="whiteAlpha.200" borderRadius="xl" bg="whiteAlpha.50" boxShadow="0 2px 8px rgba(0, 0, 0, 0.08)">
          <HStack spacing={1} p={1} align="stretch" flex={1}>
            {status?.running ? <Button size="sm" flex={1} h="auto" minH="48px" px={3} py={2} borderRadius="lg" variant="ghost" bg="whiteAlpha.50" color="gray.200" fontSize="xs" fontWeight="medium" leftIcon={<Box boxSize="14px" bg="red.400" borderRadius="3px" />} _hover={{ bg: 'rgba(252, 129, 129, 0.10)', color: 'red.200' }} _active={{ bg: 'rgba(252, 129, 129, 0.16)' }} _focusVisible={{ boxShadow: 'inset 0 0 0 2px var(--accent)' }} isDisabled={busy || !!stopping} onClick={onStop} data-testid="watch-stop">Stop watcher</Button> : <Tooltip label={unavailable ? status?.installHint || 'Install the tld CLI to start a watcher' : undefined}>
              <Button size="sm" flex={1} h="48px" minH="auto" px={3} py={2} variant="primary" fontSize="xs" fontWeight="semibold" leftIcon={<FontAwesomeIcon icon={faPlay} size={15} />} isLoading={busy} isDisabled={unavailable || !status} onClick={onStart} data-testid="watch-start">{status?.error ? 'Retry watcher' : 'Start watcher'}</Button>
            </Tooltip>}
            {onFollowModeChange && <>
              <Box h="40px" w="1px" bg="whiteAlpha.200" alignSelf="center" aria-hidden="true" />
              <Tooltip label="Keep changed code in view. Manual navigation pauses following." openDelay={300} closeOnPointerDown closeOnScroll>
                <Box as="span" display="flex" alignItems="stretch">
                  <Switch size="sm" minH="auto" px={2} py={2} display="flex" flexDirection="row-reverse" alignItems="center" justifyContent="space-between" gap={2} borderRadius="lg" color={following ? 'var(--accent)' : 'gray.400'} bg={following ? 'rgba(var(--accent-rgb), 0.04)' : 'transparent'} _hover={{ bg: 'whiteAlpha.50' }} aria-label="Follow" isChecked={following} onChange={(event) => onFollowModeChange(event.target.checked ? 'following' : 'off')} sx={{ '& .chakra-switch__label': { margin: 0 }, '& .chakra-switch__track': { margin: 0, bg: 'whiteAlpha.300' }, '& .chakra-switch__track[data-checked]': { bg: 'var(--accent)' }, '& .chakra-switch__track[data-focus-visible]': { boxShadow: '0 0 0 2px var(--bg-canvas), 0 0 0 4px var(--accent)' } }}>
                    <Text as="span" fontSize="11px" lineHeight="16px" fontWeight="medium" whiteSpace="nowrap">Follow</Text>
                  </Switch>
                </Box>
              </Tooltip>
            </>}
            <Box h="40px" w="1px" bg="whiteAlpha.200" alignSelf="center" aria-hidden="true" />
            <Popover placement="bottom-end" isLazy>
              <PopoverTrigger>
                <Button size="sm" w="30px" h="auto" minH="auto" minW="30px" borderRadius="lg" variant="ghost" color="gray.400" _hover={{ bg: 'whiteAlpha.100', color: 'gray.100' }} _expanded={{ bg: 'whiteAlpha.100', color: 'var(--accent)' }} _focusVisible={{ boxShadow: 'inset 0 0 0 2px var(--accent)' }} aria-label="Watcher options"><FontAwesomeIcon icon={faEllipsis} size={14} /></Button>
              </PopoverTrigger>
              <Portal>
                <PopoverContent w="360px" maxW="calc(100vw - 32px)" bg="var(--bg-panel)" borderColor="whiteAlpha.200" borderRadius="lg" boxShadow="lg" _focus={{ boxShadow: 'lg' }} data-testid="watch-options-panel">
                  <PopoverBody p={0} maxH="min(440px, calc(100dvh - 100px))" overflowY="auto">
                    <Box p={1.5} borderBottom="1px solid" borderColor="whiteAlpha.100">
                      {status?.running && <Button size="sm" w="full" justifyContent="flex-start" variant="ghost" fontSize="xs" leftIcon={<RepeatIcon />} isDisabled={busy || !!stopping || unavailable} title={unavailable ? status?.installHint || 'Install the tld CLI to restart the watcher' : undefined} onClick={onRestart} data-testid="watch-restart">Restart watcher</Button>}
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
        </Box>
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
