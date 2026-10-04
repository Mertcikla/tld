import { useEffect, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Alert, AlertIcon, Badge, Box, Button, Flex, Grid, HStack,
  Input, Spinner, Switch, Text, VStack,
} from '@chakra-ui/react'
import {
  api, type CodeSnapshot, type CompletedRepositoryMap, type IndexedRepository,
  type RepositoryGitHistory, type RepositoryMapConfiguration, type RepositoryRemote,
  type RepositorySettings as Settings, type RepositoryWatchStatus,
} from '../api/client'
import ConfirmDialog from '../components/ConfirmDialog'
import RepositoryWatcherPanel from '../components/RepositoryWatcherPanel'

const fields: { key: keyof RepositoryMapConfiguration; label: string; description: string; group: string }[] = [
  { key: 'resolution', label: 'Resolution', description: 'Higher values produce more, smaller groups.', group: 'Grouping' },
  { key: 'minGroupSize', label: 'Minimum group size', description: 'Merge tiny leaf groups into their strongest sibling.', group: 'Grouping' },
  { key: 'minRootGroups', label: 'Minimum root groups', description: 'Lower bound for top-level groups.', group: 'Grouping' },
  { key: 'maxRootGroups', label: 'Maximum root groups', description: 'Upper bound for top-level groups.', group: 'Grouping' },
  { key: 'maxChildren', label: 'Maximum child groups', description: 'Limit subdivisions within each group.', group: 'Grouping' },
  { key: 'maxDepth', label: 'Maximum depth', description: 'Limit the grouping hierarchy depth.', group: 'Grouping' },
  { key: 'maxLeafFiles', label: 'Maximum files per leaf', description: 'Subdivide leaf groups that exceed this size.', group: 'Grouping' },
  { key: 'maxConnectorsPerView', label: 'Connectors per view', description: 'Limit connections drawn in group views.', group: 'Connection budgets' },
  { key: 'maxLeafConnectorsPerView', label: 'Connectors per leaf view', description: 'Limit connections drawn in file-level views.', group: 'Connection budgets' },
]

interface Props {
  repository: IndexedRepository
  snapshots: CodeSnapshot[]
  maps: CompletedRepositoryMap[]
  history: RepositoryGitHistory | null
  busy: boolean
  dataError?: string
  onBack: () => void
  onDelete: () => void
  onUpdated: () => void
  children: ReactNode
}

const fieldColumns = 'repeat(auto-fit, minmax(min(100%, 280px), 1fr))'

function SettingsSection({ title, description, children, action }: {
  title: string
  description?: string
  children: ReactNode
  action?: ReactNode
}) {
  return (
    <Box as="section" minW={0} bg="var(--bg-panel)" border="1px solid var(--border-main)" borderRadius="xl" p={{ base: 4, md: 5 }}>
      <Flex align="start" justify="space-between" gap={3} wrap="wrap" mb={4}>
        <Box flex={1} minW={0}>
          <Text as="h2" fontSize="md" fontWeight="semibold">{title}</Text>
          {description && <Text fontSize="sm" color="gray.400" mt={1}>{description}</Text>}
        </Box>
        {action}
      </Flex>
      {children}
    </Box>
  )
}

function InfoField({ label, value, mono = false }: { label: string; value: ReactNode; mono?: boolean }) {
  return (
    <Box minW={0}>
      <Text as="dt" fontSize="xs" color="gray.400" mb={1}>{label}</Text>
      <Text as="dd" fontSize="sm" fontFamily={mono ? 'mono' : undefined} overflowWrap="anywhere">{value}</Text>
    </Box>
  )
}

function RemoteEditor({ remote, existing, disabled, onSave, onRemove, onCancel }: {
  remote: RepositoryRemote
  existing: boolean
  disabled: boolean
  onSave: (remote: RepositoryRemote) => Promise<void>
  onRemove: (remote: RepositoryRemote) => void
  onCancel?: () => void
}) {
  const [name, setName] = useState(remote.name)
  const [fetch, setFetch] = useState(remote.fetchUrls.join('\n'))
  const [push, setPush] = useState(remote.pushUrls.join('\n'))
  const urls = (value: string) => value.split('\n').map(url => url.trim()).filter(Boolean)

  return (
    <Box border="1px solid var(--border-main)" borderRadius="lg" p={4} minW={0}>
      <Flex align="center" justify="space-between" gap={3} mb={4}>
        <Text as="h3" fontSize="sm" fontWeight="semibold" overflowWrap="anywhere">{existing ? remote.name : 'New remote'}</Text>
        {existing && <Badge fontSize="2xs" textTransform="none">Git remote</Badge>}
      </Flex>
      {!existing && (
        <Box mb={4}>
          <Text as="label" htmlFor="remote-new-name" fontSize="sm">Remote name</Text>
          <Input id="remote-new-name" size="sm" mt={1} maxW="240px" value={name} isDisabled={disabled} placeholder="origin" autoFocus onChange={event => setName(event.target.value)} />
        </Box>
      )}
      <Grid templateColumns="repeat(auto-fit, minmax(min(100%, 240px), 1fr))" gap={4}>
        <Box minW={0}>
          <Text as="label" htmlFor={`remote-${remote.name}-fetch`} fontSize="sm">Fetch URLs</Text>
          <Box as="textarea" id={`remote-${remote.name}-fetch`} aria-label={`Fetch URLs for ${remote.name || 'new remote'}`} aria-describedby={`remote-${remote.name}-fetch-help`} value={fetch} disabled={disabled} onChange={(event: React.ChangeEvent<HTMLTextAreaElement>) => setFetch(event.target.value)} rows={2} w="full" mt={1} p={2} bg="var(--bg-canvas)" border="1px solid var(--border-main)" borderRadius="md" fontSize="sm" fontFamily="mono" resize="vertical" _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '2px' }} _disabled={{ opacity: 0.4, cursor: 'not-allowed' }} />
          <Text id={`remote-${remote.name}-fetch-help`} fontSize="xs" color="gray.400" mt={1}>One URL per line.</Text>
        </Box>
        <Box minW={0}>
          <Text as="label" htmlFor={`remote-${remote.name}-push`} fontSize="sm">Push URLs <Box as="span" color="gray.500">(optional)</Box></Text>
          <Box as="textarea" id={`remote-${remote.name}-push`} aria-label={`Push URLs for ${remote.name || 'new remote'}`} aria-describedby={`remote-${remote.name}-push-help`} value={push} disabled={disabled} onChange={(event: React.ChangeEvent<HTMLTextAreaElement>) => setPush(event.target.value)} rows={2} w="full" mt={1} p={2} bg="var(--bg-canvas)" border="1px solid var(--border-main)" borderRadius="md" fontSize="sm" fontFamily="mono" resize="vertical" _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '2px' }} _disabled={{ opacity: 0.4, cursor: 'not-allowed' }} />
          <Text id={`remote-${remote.name}-push-help`} fontSize="xs" color="gray.400" mt={1}>Leave empty to use fetch URLs.</Text>
        </Box>
      </Grid>
      <Flex mt={4} gap={2} wrap="wrap" justify="space-between">
        {existing ? (
          <Button size="sm" variant="ghost" colorScheme="red" isDisabled={disabled} onClick={() => onRemove(remote)}>Remove remote</Button>
        ) : (
          <Button size="sm" variant="ghost" isDisabled={disabled} onClick={onCancel}>Cancel</Button>
        )}
        <Button size="sm" variant="outline" isDisabled={disabled || !name.trim() || !urls(fetch).length} onClick={() => void onSave({ name: name.trim(), fetchUrls: urls(fetch), pushUrls: urls(push) })}>{existing ? 'Save remote' : 'Add remote'}</Button>
      </Flex>
    </Box>
  )
}

export default function RepositorySettings({ repository, snapshots, maps, history, busy, dataError, onBack, onDelete, onUpdated, children }: Props) {
  const navigate = useNavigate()
  const [settings, setSettings] = useState<Settings | null>(null)
  const [watch, setWatch] = useState<RepositoryWatchStatus | null>(null)
  const [draft, setDraft] = useState<Partial<Record<keyof RepositoryMapConfiguration, string>>>({})
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')
  const [addingRemote, setAddingRemote] = useState(false)
  const [loadNonce, setLoadNonce] = useState(0)
  const [removeRemote, setRemoveRemote] = useState<RepositoryRemote | null>(null)
  const disabled = busy || saving
  const applySettings = (value: Settings) => {
    setSettings(value)
    setDraft(Object.fromEntries(
      fields.filter(field => value.mapOverrides[field.key] !== undefined)
        .map(field => [field.key, String(value.mapOverrides[field.key])]),
    ))
  }
  useEffect(() => {
    const controller = new AbortController()
    setError('')
    void api.repositories.settings(repository.id, controller.signal).then(value => {
      if (!controller.signal.aborted) applySettings(value)
    }).catch(err => { if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not load repository settings') })
    void api.repositories.watchStatus(repository.id).then(value => { if (!controller.signal.aborted) setWatch(value) }).catch(() => {})
    return () => controller.abort()
  }, [repository.id, loadNonce])

  const saveOverrides = async () => {
    if (!settings) return
    const overrides: RepositoryMapConfiguration = {}
    for (const field of fields) {
      const raw = draft[field.key]
      if (raw === undefined) continue
      const value = Number(raw)
      if (!raw.trim() || !Number.isFinite(value) || value <= 0 || (field.key !== 'resolution' && (!Number.isInteger(value) || value > 2147483647))) {
        setError(`${field.label} must be a positive ${field.key === 'resolution' ? 'number' : 'integer no larger than 2147483647'}.`)
        return
      }
      overrides[field.key] = value
    }
    if ((overrides.minRootGroups ?? settings.mapDefaults.minRootGroups ?? 3) > (overrides.maxRootGroups ?? settings.mapDefaults.maxRootGroups ?? 20)) {
      setError('Maximum root groups must be at least minimum root groups, including inherited defaults.')
      return
    }
    setSaving(true)
    setError('')
    setMessage('')
    try {
      applySettings(await api.repositories.updateMapConfiguration(repository.id, overrides))
      setMessage('Repository overrides saved. Build a new map to apply them.')
      onUpdated()
    } catch (err) { setError(err instanceof Error ? err.message : 'Could not save map overrides') }
    finally { setSaving(false) }
  }
  const updateRemote = async (remote: RepositoryRemote, remove = false) => {
    setSaving(true)
    setError('')
    setMessage('')
    try {
      const updated = await api.repositories.updateRemote(repository.id, remote, remove)
      // Remote edits must not discard unsaved map overrides.
      setSettings(updated)
      setAddingRemote(false)
      setRemoveRemote(null)
      setMessage(remove ? 'Remote removed.' : 'Remote saved to the local Git checkout.')
      onUpdated()
    } catch (err) { setError(err instanceof Error ? err.message : 'Could not update remote') }
    finally { setSaving(false) }
  }
  const watchAction = async (action: 'start' | 'stop' | 'restart' | 'refresh') => {
    setSaving(true)
    setError('')
    try {
      if (action === 'stop' || action === 'restart') await api.repositories.stopWatch(repository.id)
      if (action === 'start' || action === 'restart') await api.repositories.startWatch(repository.id, { materialize: false })
      setWatch(await api.repositories.watchStatus(repository.id))
      onUpdated()
    } catch (err) { setError(err instanceof Error ? err.message : 'Could not update watcher') }
    finally { setSaving(false) }
  }
  const overrideCount = fields.filter(field => draft[field.key] !== undefined).length
  const hasMapChanges = !!settings && fields.some(field => draft[field.key] !== (
    settings.mapOverrides[field.key] === undefined ? undefined : String(settings.mapOverrides[field.key])
  ))

  return (
    <VStack data-testid="repository-settings-page" align="stretch" spacing={5} p={{ base: 4, md: 6 }} pb={{ base: 'calc(var(--bottomnav-container-h, 0px) + env(safe-area-inset-bottom, 0px) + 24px)', md: 8 }} maxW="1100px" w="full" minW={0} mx="auto">
      <Box>
        <Button size="sm" variant="ghost" ml={-3} mb={4} onClick={onBack}>← Back to repository</Button>
        <Text fontSize="xs" color="gray.400" mb={1}>{repository.root.split(/[/\\]/).filter(Boolean).pop()}</Text>
        <Text as="h1" fontSize={{ base: 'xl', md: '2xl' }} fontWeight="semibold">Repository settings</Text>
        <Text fontSize="sm" color="gray.400" mt={2} overflowWrap="anywhere">{repository.root}</Text>
      </Box>

      {error && <Alert status="error" role="alert" borderRadius="lg" fontSize="sm"><AlertIcon /><Box flex={1} minW={0} overflowWrap="anywhere">{error}</Box></Alert>}
      {error && !settings && <Button size="sm" alignSelf="start" onClick={() => setLoadNonce(value => value + 1)}>Retry loading settings</Button>}
      {dataError && <Alert status="error" borderRadius="lg" fontSize="sm"><AlertIcon /><Box minW={0} overflowWrap="anywhere">{dataError}</Box></Alert>}
      {settings?.mapValidationError && <Alert status="warning" borderRadius="lg" fontSize="sm"><AlertIcon /><Box minW={0} overflowWrap="anywhere">{settings.mapValidationError} Adjust the overrides or restore global defaults.</Box></Alert>}
      {message && <Alert status="info" role="status" borderRadius="lg" fontSize="sm"><AlertIcon />{message}</Alert>}

      <SettingsSection title="Repository information" action={settings && <Badge colorScheme={settings.isGit ? 'green' : 'gray'} textTransform="none">{settings.isGit ? 'Git available' : 'Git unavailable'}</Badge>}>
        <Grid as="dl" templateColumns="repeat(auto-fit, minmax(min(100%, 220px), 1fr))" gap={4}>
          <InfoField label="Current branch" value={settings?.currentBranch || history?.currentBranch || '—'} />
          <InfoField label="Indexed branch" value={repository.gitBranch || '—'} />
          <InfoField label="Last indexed" value={repository.latestCreatedUnix ? new Date(repository.latestCreatedUnix * 1000).toLocaleString() : 'Never'} />
        </Grid>
        <Box as="details" mt={4}>
          <Text as="summary" fontSize="xs" color="gray.400" cursor="pointer" _hover={{ color: 'gray.200' }}>Repository identifiers and revisions</Text>
          <Grid as="dl" templateColumns={fieldColumns} gap={4} mt={4}>
            <InfoField label="Repository ID" value={repository.id} mono />
            <InfoField label="HEAD" value={settings?.headSha || history?.headSha || '—'} mono />
            <InfoField label="Latest snapshot" value={repository.latestSnapshotId || 'None'} mono />
            <InfoField label="Indexed revision" value={repository.gitRevision || '—'} mono />
          </Grid>
        </Box>
        <Box borderTop="1px solid var(--border-main)" mt={5} pt={4}>
          <Text as="h3" fontSize="xs" color="gray.400" mb={3}>Index statistics</Text>
          <Grid templateColumns="repeat(auto-fit, minmax(100px, 1fr))" gap={3}>
            {[
              ['Files', repository.sources], ['Facts', repository.facts], ['Edges', repository.edges],
              ['Chunks', repository.chunks], ['Snapshots', snapshots.length], ['Maps', maps.length],
            ].map(([label, count]) => (
              <Box key={label}>
                <Text fontSize="xl" fontWeight="semibold" sx={{ fontVariantNumeric: 'tabular-nums' }}>{Number(count).toLocaleString()}</Text>
                <Text fontSize="xs" color="gray.400">{label}</Text>
              </Box>
            ))}
          </Grid>
          <Text fontSize="xs" color="gray.500" mt={3}>File, fact, edge and chunk counts are from the latest indexed snapshot.</Text>
        </Box>
      </SettingsSection>

      <SettingsSection title="Git remotes" description="Manage fetch and push URLs in this repository’s local Git checkout." action={settings?.isGit && !addingRemote && <Button size="sm" variant="outline" isDisabled={disabled} onClick={() => setAddingRemote(true)}>Add remote</Button>}>
        {!settings ? (
          <HStack color="gray.400" fontSize="sm">{!error && <Spinner size="sm" />}<Text>{error ? 'Settings unavailable.' : 'Loading remotes…'}</Text></HStack>
        ) : settings.isGit ? (
          <VStack align="stretch" spacing={3}>
            {settings.remotes.map(remote => <RemoteEditor key={`${remote.name}:${remote.fetchUrls.join(',')}:${remote.pushUrls.join(',')}`} remote={remote} existing disabled={disabled} onSave={updateRemote} onRemove={setRemoveRemote} />)}
            {!settings.remotes.length && !addingRemote && <Text fontSize="sm" color="gray.400" py={2}>No remotes configured. Add a remote to connect this checkout to a Git host.</Text>}
            {addingRemote && <RemoteEditor remote={{ name: '', fetchUrls: [], pushUrls: [] }} existing={false} disabled={disabled} onSave={updateRemote} onRemove={setRemoveRemote} onCancel={() => setAddingRemote(false)} />}
          </VStack>
        ) : <Text fontSize="sm" color="gray.400">Remote management requires an available Git checkout.</Text>}
      </SettingsSection>

      <SettingsSection title="Map configuration" description="Customize how this repository is grouped and connected. Changes apply to new full maps." action={settings && <Badge colorScheme={overrideCount ? 'blue' : 'gray'} textTransform="none">{overrideCount ? `${overrideCount} ${overrideCount === 1 ? 'override' : 'overrides'}` : 'Using global defaults'}</Badge>}>
        {!settings ? (
          <HStack color="gray.400" fontSize="sm">{!error && <Spinner size="sm" />}<Text>{error ? 'Settings unavailable.' : 'Loading configuration…'}</Text></HStack>
        ) : (
          <>
            <Text fontSize="xs" color="gray.400" mb={4}>Enable an override to set a repository value. Turn it off to inherit the current global default.</Text>
            <VStack align="stretch" spacing={5}>
              {['Grouping', 'Connection budgets'].map(group => (
                <Box key={group}>
                  <Text as="h3" fontSize="sm" fontWeight="semibold" mb={3}>{group}</Text>
                  <Grid templateColumns={fieldColumns} gap={3}>
                    {fields.filter(field => field.group === group).map(field => {
                      const override = draft[field.key] !== undefined
                      return (
                        <Box key={field.key} border="1px solid" borderColor={override ? 'var(--accent)' : 'var(--border-main)'} borderRadius="lg" p={3} minW={0}>
                          <Flex align="start" justify="space-between" gap={3}>
                            <Text as="label" htmlFor={`map-${field.key}`} fontSize="sm" fontWeight="medium">{field.label}</Text>
                            <Switch size="sm" flexShrink={0} mt={1} aria-label={`Override ${field.label}`} data-testid={`repository-override-${field.key}`} isChecked={override} isDisabled={disabled} onChange={event => {
                              setDraft(old => {
                                const next = { ...old }
                                if (event.target.checked) next[field.key] = String(settings.mapDefaults[field.key])
                                else delete next[field.key]
                                return next
                              })
                              setMessage('')
                            }} />
                          </Flex>
                          <Text id={`map-${field.key}-help`} fontSize="xs" color="gray.400" mt={1} minH="36px">{field.description}</Text>
                          <Flex align="center" justify="space-between" gap={3} mt={3}>
                            <Box>
                              <Text fontSize="xs" color={override ? 'var(--accent)' : 'gray.400'}>{override ? 'Override' : 'Inherited'}</Text>
                              <Text fontSize="xs" color="gray.500">Global default: {settings.mapDefaults[field.key]}</Text>
                            </Box>
                            <Input id={`map-${field.key}`} aria-describedby={`map-${field.key}-help`} data-testid={`repository-map-${field.key}`} type="number" size="sm" w="100px" flexShrink={0} min={field.key === 'resolution' ? undefined : 1} step={field.key === 'resolution' ? 'any' : 1} value={override ? draft[field.key] : settings.mapDefaults[field.key] ?? ''} isDisabled={disabled || !override} onChange={event => {
                              setDraft(old => ({ ...old, [field.key]: event.target.value }))
                              setMessage('')
                            }} />
                          </Flex>
                        </Box>
                      )
                    })}
                  </Grid>
                </Box>
              ))}
            </VStack>
            <Flex align="center" justify="space-between" gap={3} wrap="wrap" mt={5} pt={4} borderTop="1px solid var(--border-main)">
              <Button size="sm" variant="ghost" isDisabled={disabled} onClick={() => {
                setDraft({})
                setMessage('All overrides cleared. Save to inherit global defaults.')
                setError('')
              }}>Use global defaults</Button>
              <Flex align="center" gap={3} wrap="wrap">
                {hasMapChanges && <Text fontSize="xs" color="orange.300" role="status">Unsaved changes</Text>}
                <Button size="sm" bg="var(--accent)" color="white" _hover={{ filter: 'brightness(1.1)' }} isLoading={saving} isDisabled={busy} data-testid="repository-map-save" onClick={() => void saveOverrides()}>Save map overrides</Button>
              </Flex>
            </Flex>
          </>
        )}
      </SettingsSection>

      <SettingsSection title="Watcher" description="Keep the index up to date as local files change.">
        <Box mx={-4} mb={-4} sx={{ '> [data-testid="repository-watcher"]': { borderBottom: 0 } }}>
          <RepositoryWatcherPanel status={watch} repositoryRoot={repository.root} branch={watch?.gitBranch || settings?.currentBranch || history?.currentBranch || ''} revision={watch?.gitRevision || settings?.headSha || history?.headSha || ''} busy={disabled} onStart={() => void watchAction('start')} onStop={() => void watchAction('stop')} onRestart={() => void watchAction('restart')} onRefresh={() => void watchAction('refresh')} />
        </Box>
      </SettingsSection>

      <Grid templateColumns="repeat(auto-fit, minmax(min(100%, 360px), 1fr))" gap={5} alignItems="start">
        <SettingsSection title="Snapshots" description="Saved indexes of this repository." action={<Badge sx={{ fontVariantNumeric: 'tabular-nums' }}>{snapshots.length}</Badge>}>
          <Box mx={-4} mb={-3}>{children}</Box>
        </SettingsSection>
        <SettingsSection title="Completed maps" description="Open a map built from a saved snapshot." action={<Badge sx={{ fontVariantNumeric: 'tabular-nums' }}>{maps.length}</Badge>}>
          {maps.length ? (
            <VStack align="stretch" spacing={3}>
              {maps.map(map => (
                <Box key={`${map.result.runId}:${map.configHash}`} border="1px solid var(--border-main)" borderRadius="lg" p={3} minW={0}>
                  <Text fontSize="sm" fontWeight="medium">{new Date(map.completedUnix * 1000).toLocaleString()}</Text>
                  <Text fontSize="xs" color="gray.400" mt={1}>{map.result.clusters.toLocaleString()} groups · {map.result.facts.toLocaleString()} facts</Text>
                  <Text fontSize="xs" color="gray.400" mt={1}>External imports {map.includeImports ? 'included' : 'excluded'}</Text>
                  <Box as="details" mt={3}>
                    <Text as="summary" fontSize="xs" color="gray.500" cursor="pointer">Snapshot ID</Text>
                    <Text fontSize="xs" fontFamily="mono" overflowWrap="anywhere" mt={1}>{map.result.snapshotId}</Text>
                  </Box>
                  <Button size="sm" variant="outline" mt={3} onClick={() => navigate(`/views/${map.result.viewId}`)}>Open map</Button>
                </Box>
              ))}
            </VStack>
          ) : <Text fontSize="sm" color="gray.400" py={2}>No maps built yet.</Text>}
        </SettingsSection>
      </Grid>

      <Box as="section" border="1px solid" borderColor="red.900" borderRadius="xl" p={{ base: 4, md: 5 }}>
        <Flex align="center" justify="space-between" gap={4} wrap="wrap">
          <Box flex={1} minW="min(100%, 240px)">
            <Text as="h2" fontSize="md" fontWeight="semibold" color="red.300">Delete repository</Text>
            <Text fontSize="sm" color="gray.400" mt={1}>Remove this repository and its indexed snapshots. Local source files stay on disk. You can also remove materialized workspace resources in the confirmation.</Text>
          </Box>
          <Button size="sm" colorScheme="red" variant="outline" data-testid={`repositories-delete-${repository.id}`} isDisabled={disabled} onClick={onDelete}>Delete repository</Button>
        </Flex>
      </Box>
      <ConfirmDialog isOpen={!!removeRemote} onClose={() => { if (!saving) setRemoveRemote(null) }} onConfirm={() => { if (removeRemote) void updateRemote(removeRemote, true) }} title="Remove Git remote" body={`Remove remote "${removeRemote?.name}" and its remote-tracking branches from this local checkout?`} confirmLabel="Remove remote" confirmColorScheme="red" isLoading={saving} />
    </VStack>
  )
}
