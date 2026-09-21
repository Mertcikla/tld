import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Badge,
  Box,
  Button,
  Divider,
  FormLabel,
  HStack,
  Input,
  Select,
  Spinner,
  Stack,
  Table,
  Tbody,
  Td,
  Text,
  Th,
  Thead,
  Tr,
  VStack,
} from '@chakra-ui/react'
import {
  api,
  type WatchEmbeddingConfig,
  type WatchHealthcheckResponse,
  type WatchRepository,
  type WatchScanProgress,
  type WatchSettings,
  type WatchSettingsDescriptor,
  type WatchWatcherStatus,
} from '../../api/client'
import { isWailsApp } from '../../config/runtime'
import { getCLIRuntimeStatus, getWatchCommand, installCLI, type CLIRuntimeStatus } from '../../lib/desktop'
import WatchSettingsForm from './WatchSettingsForm'
import { cloneWatchEmbedding, cloneWatchSettings } from './watchSettings'

type Props = {
  enabled: boolean
  compact?: boolean
}

export default function WatchControlPanel({ enabled, compact = false }: Props) {
  const [repositories, setRepositories] = useState<WatchRepository[]>([])
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [newPath, setNewPath] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [busyAction, setBusyAction] = useState('')
  const [error, setError] = useState('')
  const [status, setStatus] = useState<WatchWatcherStatus | null>(null)
  const [progress, setProgress] = useState<WatchScanProgress | null>(null)
  const [settings, setSettings] = useState<WatchSettings | null>(null)
  const [embedding, setEmbedding] = useState<WatchEmbeddingConfig | null>(null)
  const [descriptors, setDescriptors] = useState<WatchSettingsDescriptor[]>([])
  const [overridden, setOverridden] = useState(false)
  const [lspHealth, setLspHealth] = useState<WatchHealthcheckResponse | null>(null)
  const [embeddingHealth, setEmbeddingHealth] = useState<WatchHealthcheckResponse | null>(null)
  const [cliStatus, setCLIStatus] = useState<CLIRuntimeStatus | null>(null)
  const [watchCommand, setWatchCommand] = useState('')
  const [commandCopied, setCommandCopied] = useState(false)

  const selectedRepository = useMemo(
    () => repositories.find((repo) => repo.id === selectedId) ?? null,
    [repositories, selectedId],
  )

  const reloadRepositories = useCallback(async () => {
    const repos = await api.watch.repositories()
    setRepositories(repos)
    setSelectedId((current) => {
      if (current && repos.some((repo) => repo.id === current)) return current
      return repos.length > 0 ? repos[0].id : null
    })
  }, [])

  const reloadSelected = useCallback(async (repositoryId: number) => {
    const [settingsResponse, progressResponse] = await Promise.all([
      api.watch.settings(repositoryId),
      api.watch.scanProgress(repositoryId),
    ])
    setSettings(cloneWatchSettings(settingsResponse.settings))
    setEmbedding(cloneWatchEmbedding(settingsResponse.embedding))
    setDescriptors(settingsResponse.descriptors ?? [])
    setOverridden(settingsResponse.overridden)
    setProgress(progressResponse)
    setStatus(progressResponse.watcher)
  }, [])

  useEffect(() => {
    if (!enabled) return
    let cancelled = false
    setLoading(true)
    reloadRepositories()
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load repositories')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [enabled, reloadRepositories])

  useEffect(() => {
    if (!enabled || selectedId == null) return
    let cancelled = false
    setError('')
    setLspHealth(null)
    setEmbeddingHealth(null)
    reloadSelected(selectedId).catch((err) => {
      if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load repository settings')
    })
    return () => {
      cancelled = true
    }
  }, [enabled, selectedId, reloadSelected])

  const pollCLIStatus = useCallback(async () => {
    for (let attempt = 0; attempt < 20; attempt++) {
      await new Promise((resolve) => window.setTimeout(resolve, 3000))
      try {
        const status = await getCLIRuntimeStatus()
        setCLIStatus(status)
        if (status.available) return
      } catch {
        // keep polling until the attempt budget is exhausted
      }
    }
  }, [])

  useEffect(() => {
    if (!enabled || !isWailsApp) return
    let cancelled = false
    getCLIRuntimeStatus()
      .then((status) => {
        if (!cancelled) setCLIStatus(status)
      })
      .catch(() => {
        if (!cancelled) setCLIStatus(null)
      })
    return () => {
      cancelled = true
    }
  }, [enabled])

  useEffect(() => {
    if (!isWailsApp || !selectedRepository) {
      setWatchCommand('')
      return
    }
    let cancelled = false
    getWatchCommand(selectedRepository.repo_root)
      .then((command) => {
        if (!cancelled) setWatchCommand(command)
      })
      .catch(() => {
        if (!cancelled) setWatchCommand(`tld watch ${selectedRepository.repo_root}`)
      })
    return () => {
      cancelled = true
    }
  }, [selectedRepository])

  const refreshProgress = useCallback(async () => {
    if (selectedId == null) return
    const next = await api.watch.scanProgress(selectedId)
    setProgress(next)
    setStatus(next.watcher)
  }, [selectedId])

  const handleAddRepository = async () => {
    const path = newPath.trim()
    if (!path) return
    setBusyAction('add')
    setError('')
    try {
      const repo = await api.watch.addRepository(path)
      setNewPath('')
      await reloadRepositories()
      setSelectedId(repo.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add repository')
    } finally {
      setBusyAction('')
    }
  }

  const handleStart = async () => {
    if (selectedId == null) return
    setBusyAction('start')
    setError('')
    try {
      const next = await api.watch.start(selectedId)
      setStatus(next)
      await refreshProgress()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to start watch')
    } finally {
      setBusyAction('')
    }
  }

  const handleStop = async () => {
    if (selectedId == null) return
    setBusyAction('stop')
    setError('')
    try {
      await api.watch.stop(selectedId)
      await refreshProgress()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to stop watch')
    } finally {
      setBusyAction('')
    }
  }

  const handleInstallCLI = async () => {
    setBusyAction('install-cli')
    setError('')
    try {
      const status = await installCLI()
      setCLIStatus(status)
      // The installer runs interactively in a terminal; poll for the freshly
      // installed binary so the UI updates without a reopen once it completes.
      if (!status.available) {
        void pollCLIStatus()
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to start CLI install')
    } finally {
      setBusyAction('')
    }
  }

  const handleCopyCommand = async () => {
    if (!watchCommand) return
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(watchCommand)
      } else {
        const input = document.createElement('textarea')
        input.value = watchCommand
        input.style.position = 'fixed'
        input.style.opacity = '0'
        document.body.appendChild(input)
        input.select()
        document.execCommand('copy')
        document.body.removeChild(input)
      }
      setCommandCopied(true)
      window.setTimeout(() => setCommandCopied(false), 1500)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to copy command')
    }
  }

  const handleSaveSettings = async () => {
    if (selectedId == null || !settings || !embedding) return
    setSaving(true)
    setError('')
    try {
      const saved = await api.watch.saveSettings(selectedId, { settings, embedding })
      setSettings(cloneWatchSettings(saved.settings))
      setEmbedding(cloneWatchEmbedding(saved.embedding))
      setOverridden(saved.overridden)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save settings')
    } finally {
      setSaving(false)
    }
  }

  const handleResetSettings = async () => {
    if (selectedId == null) return
    setSaving(true)
    setError('')
    try {
      const reset = await api.watch.resetSettings(selectedId)
      setSettings(cloneWatchSettings(reset.settings))
      setEmbedding(cloneWatchEmbedding(reset.embedding))
      setOverridden(reset.overridden)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reset settings')
    } finally {
      setSaving(false)
    }
  }

  const handleLSPHealthcheck = async () => {
    if (selectedId == null) return
    setBusyAction('lsp')
    setError('')
    try {
      setLspHealth(await api.watch.lspHealthcheck(selectedId))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'LSP healthcheck failed')
    } finally {
      setBusyAction('')
    }
  }

  const handleEmbeddingHealthcheck = async () => {
    if (selectedId == null) return
    setBusyAction('embedding')
    setError('')
    try {
      setEmbeddingHealth(await api.watch.embeddingHealthcheck(selectedId))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Embedding healthcheck failed')
    } finally {
      setBusyAction('')
    }
  }

  const watched = status?.live ?? false

  return (
    <VStack align="stretch" spacing={compact ? 4 : 6} w="full" opacity={enabled ? 1 : 0.5}>
      {/* Repository selection */}
      <VStack align="stretch" spacing={2}>
        <FormLabel fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.400" mb={0}>
          Repository
        </FormLabel>
        <HStack spacing={2}>
          <Select
            size="sm"
            isDisabled={!enabled || loading}
            value={selectedId ?? ''}
            placeholder={repositories.length === 0 ? 'No repositories yet' : 'Select a repository'}
            onChange={(event) => setSelectedId(event.target.value ? Number(event.target.value) : null)}
          >
            {repositories.map((repo) => (
              <option key={repo.id} value={repo.id}>
                {repo.display_name} · {repo.repo_root}
              </option>
            ))}
          </Select>
          {loading && <Spinner size="sm" />}
        </HStack>
        <HStack spacing={2}>
          <Input
            size="sm"
            isDisabled={!enabled}
            placeholder="/path/to/git/repository"
            value={newPath}
            onChange={(event) => setNewPath(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') handleAddRepository()
            }}
          />
          <Button
            size="sm"
            colorScheme="blue"
            isDisabled={!enabled || busyAction === 'add' || !newPath.trim()}
            isLoading={busyAction === 'add'}
            onClick={handleAddRepository}
          >
            Add
          </Button>
        </HStack>
        {selectedRepository && (
          <Text fontSize="xs" color="gray.500" noOfLines={1} title={selectedRepository.repo_root}>
            {selectedRepository.repo_root}
            {selectedRepository.branch ? ` · ${selectedRepository.branch}` : ''}
          </Text>
        )}
      </VStack>

      {/* Start / stop */}
      <VStack align="stretch" spacing={2}>
        <FormLabel fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.400" mb={0}>
          Watch
        </FormLabel>
        <HStack spacing={2}>
          <Badge colorScheme={watched ? 'green' : 'gray'}>{watched ? (status?.paused ? 'paused' : 'running') : 'stopped'}</Badge>
          {status?.process?.pid ? (
            <Text fontSize="xs" color="gray.500">
              pid {status.process.pid}
            </Text>
          ) : null}
          <Box flex="1" />
          {!isWailsApp && (
            <>
              <Button
                size="sm"
                colorScheme="green"
                isDisabled={!enabled || selectedId == null || watched || busyAction === 'start'}
                isLoading={busyAction === 'start'}
                onClick={handleStart}
              >
                Start
              </Button>
              <Button
                size="sm"
                colorScheme="red"
                variant="outline"
                isDisabled={!enabled || selectedId == null || !watched || busyAction === 'stop'}
                isLoading={busyAction === 'stop'}
                onClick={handleStop}
              >
                Stop
              </Button>
            </>
          )}
          <Button size="sm" variant="ghost" isDisabled={!enabled || selectedId == null} onClick={refreshProgress}>
            Refresh
          </Button>
        </HStack>
        {isWailsApp && (
          <VStack align="stretch" spacing={2} pt={1}>
            <Text fontSize="xs" color="gray.500">
              The watch process runs via the standalone <Text as="span" fontFamily="mono">tld</Text> CLI. Install it, then run the command below in your terminal.
            </Text>
            <HStack spacing={2}>
              <Button
                size="sm"
                variant="outline"
                colorScheme="blue"
                isDisabled={!enabled || !cliStatus?.installSupported || busyAction === 'install-cli'}
                isLoading={busyAction === 'install-cli'}
                onClick={handleInstallCLI}
              >
                {cliStatus?.available ? 'Reinstall tld CLI' : 'Install tld CLI'}
              </Button>
              {cliStatus?.available ? (
                <Badge colorScheme="green" fontSize="2xs">
                  installed
                </Badge>
              ) : (
                <Badge colorScheme="orange" fontSize="2xs">
                  not found on PATH
                </Badge>
              )}
            </HStack>
            {cliStatus?.installHint && (
              <Text fontSize="xs" color="gray.600">
                {cliStatus.installHint}
              </Text>
            )}
            {watchCommand && (
              <HStack spacing={2}>
                <Input size="sm" readOnly value={watchCommand} fontFamily="mono" fontSize="xs" />
                <Button size="sm" variant="outline" onClick={handleCopyCommand} isDisabled={!enabled}>
                  {commandCopied ? 'Copied' : 'Copy'}
                </Button>
              </HStack>
            )}
          </VStack>
        )}
      </VStack>

      {/* Scan progress */}
      {progress && (
        <VStack align="stretch" spacing={2}>
          <FormLabel fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.400" mb={0}>
            Scan progress
          </FormLabel>
          <Stack direction={{ base: 'column', sm: 'row' }} spacing={4} fontSize="xs" color="gray.400">
            <Text>Files: {progress.summary.files}</Text>
            <Text>Symbols: {progress.summary.symbols}</Text>
            <Text>References: {progress.summary.references}</Text>
            <Text>Last scan: {progress.summary.last_scan_status || 'n/a'}</Text>
            <Text>Representation: {progress.representation.last_status || 'n/a'}</Text>
          </Stack>
        </VStack>
      )}

      <Divider borderColor="whiteAlpha.100" />

      {/* Healthchecks */}
      <VStack align="stretch" spacing={3}>
        <FormLabel fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.400" mb={0}>
          Healthchecks
        </FormLabel>
        <HStack spacing={2}>
          <Button size="sm" variant="outline" isDisabled={!enabled || selectedId == null || busyAction === 'lsp'} isLoading={busyAction === 'lsp'} onClick={handleLSPHealthcheck}>
            Check language servers
          </Button>
          <Button
            size="sm"
            variant="outline"
            isDisabled={!enabled || selectedId == null || busyAction === 'embedding'}
            isLoading={busyAction === 'embedding'}
            onClick={handleEmbeddingHealthcheck}
          >
            Check embeddings
          </Button>
        </HStack>
        {lspHealth && <LSPHealthTable response={lspHealth} />}
        {embeddingHealth && <EmbeddingHealthTable response={embeddingHealth} />}
      </VStack>

      <Divider borderColor="whiteAlpha.100" />

      {/* Tuning */}
      <VStack align="stretch" spacing={3}>
        <HStack justify="space-between">
          <FormLabel fontSize="xs" textTransform="uppercase" letterSpacing="0.12em" color="gray.400" mb={0}>
            Tuning
          </FormLabel>
          <HStack spacing={2}>
            {overridden && (
              <Badge colorScheme="purple" fontSize="2xs">
                override
              </Badge>
            )}
            <Button size="xs" variant="ghost" isDisabled={!enabled || selectedId == null || !overridden || saving} onClick={handleResetSettings}>
              Reset to global
            </Button>
            <Button size="xs" colorScheme="blue" isDisabled={!enabled || selectedId == null || saving} isLoading={saving} onClick={handleSaveSettings}>
              Save
            </Button>
          </HStack>
        </HStack>
        {settings && embedding ? (
          <WatchSettingsForm
            settings={settings}
            embedding={embedding}
            descriptors={descriptors}
            disabled={!enabled}
            onChange={({ settings: nextSettings, embedding: nextEmbedding }) => {
              setSettings(nextSettings)
              setEmbedding(nextEmbedding)
            }}
          />
        ) : (
          <Text fontSize="xs" color="gray.500">
            Select a repository to configure watch settings.
          </Text>
        )}
      </VStack>

      {error && (
        <Text fontSize="xs" color="red.300">
          {error}
        </Text>
      )}
    </VStack>
  )
}

function LSPHealthTable({ response }: { response: WatchHealthcheckResponse }) {
  const servers = response.lsp?.servers ?? []
  if (!response.lsp) {
    return null
  }
  return (
    <VStack align="stretch" spacing={1}>
      <Text fontSize="xs" color={response.ok ? 'green.300' : 'orange.300'}>
        {response.ok ? 'All requested language servers are available.' : response.message || 'Some language servers are unavailable.'}
      </Text>
      {servers.length > 0 && (
        <Table size="sm" variant="simple">
          <Thead>
            <Tr>
              <Th fontSize="2xs">Language</Th>
              <Th fontSize="2xs">State</Th>
              <Th fontSize="2xs">Command</Th>
              <Th fontSize="2xs">PID</Th>
            </Tr>
          </Thead>
          <Tbody>
            {servers.map((server) => (
              <Tr key={server.language}>
                <Td fontSize="2xs">{server.language}</Td>
                <Td fontSize="2xs" color={server.state === 'active' ? 'green.300' : server.state === 'unavailable' || server.state === 'failed' ? 'red.300' : 'gray.400'}>
                  {server.state}
                </Td>
                <Td fontSize="2xs" color="gray.500">
                  {server.command || '—'}
                </Td>
                <Td fontSize="2xs" color="gray.500">
                  {server.pid || '—'}
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
    </VStack>
  )
}

function EmbeddingHealthTable({ response }: { response: WatchHealthcheckResponse }) {
  if (!response.embedding) return null
  return (
    <VStack align="stretch" spacing={1}>
      <Text fontSize="xs" color={response.ok ? 'green.300' : 'orange.300'}>
        {response.ok ? 'Embedding provider healthy.' : response.message || 'Embedding provider unavailable.'}
      </Text>
      <HStack spacing={4} fontSize="xs" color="gray.400">
        <Text>provider: {response.embedding.provider || 'none'}</Text>
        <Text>model: {response.embedding.model || '—'}</Text>
        {response.embedding.endpoint ? <Text>endpoint: {response.embedding.endpoint}</Text> : null}
        {response.ok ? <Text>dimension: {response.embedding.dimension}</Text> : null}
        {response.ok ? <Text>similarity: {response.embedding.similarity.toFixed(3)}</Text> : null}
      </HStack>
    </VStack>
  )
}
