import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Badge,
  Box,
  Button,
  Collapse,
  HStack,
  IconButton,
  Select,
  Spinner,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import { CloseIcon, RepeatIcon, TimeIcon, ViewIcon } from '@chakra-ui/icons'
import { api, type WatchDiff, type WatchRepository, type WatchVersion, type WorkspaceVersion } from '../api/client'
import { buildWorkspaceVersionPreview, useWorkspaceVersionPreview } from '../context/WorkspaceVersionContext'

function shortHash(value?: string) {
  if (!value) return ''
  return value.length > 10 ? value.slice(0, 10) : value
}

function changeLabel(diffs: WatchDiff[]) {
  const counts = diffs.reduce((acc, diff) => {
    const key = diff.change_type || 'changed'
    acc[key] = (acc[key] ?? 0) + 1
    return acc
  }, {} as Record<string, number>)
  const parts = [
    counts.added ? `+${counts.added}` : '',
    counts.updated ? `~${counts.updated}` : '',
    counts.deleted ? `-${counts.deleted}` : '',
    counts.changed ? `${counts.changed} changed` : '',
  ].filter(Boolean)
  return parts.length > 0 ? parts.join('  ') : 'No materialized changes'
}

export default function WorkspaceVersionPanel() {
  const navigate = useNavigate()
  const { preview, setPreview, clearPreview, requestFollow } = useWorkspaceVersionPreview()
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [repos, setRepos] = useState<WatchRepository[]>([])
  const [versions, setVersions] = useState<WatchVersion[]>([])
  const [workspaceVersions, setWorkspaceVersions] = useState<WorkspaceVersion[]>([])
  const [repoId, setRepoId] = useState<number | ''>('')
  const [versionId, setVersionId] = useState<number | ''>('')
  const [diffs, setDiffs] = useState<WatchDiff[]>([])

  const selectedRepo = useMemo(() => repos.find((repo) => repo.id === repoId) ?? null, [repos, repoId])
  const selectedVersion = useMemo(() => versions.find((version) => version.id === versionId) ?? null, [versions, versionId])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [nextRepos, nextWorkspaceVersions] = await Promise.all([
        api.watch.repositories().catch(() => [] as WatchRepository[]),
        api.versions.list(50).catch(() => [] as WorkspaceVersion[]),
      ])
      setRepos(nextRepos)
      setWorkspaceVersions(nextWorkspaceVersions)
      const nextRepoId = repoId || nextRepos[0]?.id || ''
      setRepoId(nextRepoId)
      if (nextRepoId) {
        const nextVersions = await api.watch.versions(nextRepoId)
        setVersions(nextVersions)
        setVersionId(versionId || nextVersions[0]?.id || '')
      }
    } finally {
      setLoading(false)
    }
  }, [repoId, versionId])

  useEffect(() => {
    if (!open && !preview) return
    void load()
    // Load once when the panel is first opened or when watch preview exists.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  useEffect(() => {
    if (!repoId) {
      setVersions([])
      setVersionId('')
      return
    }
    api.watch.versions(repoId).then((next) => {
      setVersions(next)
      setVersionId(next[0]?.id ?? '')
    }).catch(() => {
      setVersions([])
      setVersionId('')
    })
  }, [repoId])

  useEffect(() => {
    if (!versionId) {
      setDiffs([])
      return
    }
    api.watch.diffs(versionId).then(setDiffs).catch(() => setDiffs([]))
  }, [versionId])

  const applyPreview = useCallback(() => {
    setPreview(buildWorkspaceVersionPreview({
      repository: selectedRepo,
      version: selectedVersion,
      workspaceVersions,
      diffs,
    }))
  }, [diffs, selectedRepo, selectedVersion, setPreview, workspaceVersions])

  const follow = useCallback(() => {
    if (!preview && selectedVersion) applyPreview()
    requestFollow()
    navigate('/views?view=explore')
  }, [applyPreview, navigate, preview, requestFollow, selectedVersion])

  const compactSummary = preview ? changeLabel(preview.diffs) : diffs.length > 0 ? changeLabel(diffs) : 'Workspace versions'
  const activeVersion = preview?.version ?? selectedVersion
  const activeRepo = preview?.repository ?? selectedRepo

  return (
    <Box
      position="absolute"
      left={{ base: 3, md: 4 }}
      bottom={{ base: 'calc(var(--bottomnav-container-h) + 12px)', md: 4 }}
      zIndex={1001}
      pointerEvents="auto"
      maxW={{ base: 'calc(100vw - 24px)', md: '420px' }}
    >
      <Box
        bg="rgba(var(--bg-main-rgb), 0.94)"
        border="1px solid"
        borderColor={preview ? 'rgba(var(--accent-rgb), 0.45)' : 'whiteAlpha.200'}
        borderRadius="lg"
        boxShadow="0 18px 48px rgba(0,0,0,0.45)"
        backdropFilter="blur(18px)"
        overflow="hidden"
      >
        <HStack px={3} py={2.5} spacing={2} justify="space-between">
          <HStack spacing={2} minW={0}>
            <Badge colorScheme={preview ? 'blue' : 'gray'} borderRadius="md">Versions</Badge>
            <Box minW={0}>
              <Text fontSize="12px" fontWeight="800" color="gray.100" noOfLines={1}>
                {activeRepo?.display_name ?? 'Workspace'}
              </Text>
              <Text fontSize="11px" color="gray.400" noOfLines={1}>
                {activeVersion ? `${shortHash(activeVersion.commit_hash)} · ${compactSummary}` : compactSummary}
              </Text>
            </Box>
          </HStack>
          <HStack spacing={1}>
            {preview && (
              <Tooltip label="Clear diff preview">
                <IconButton aria-label="Clear diff preview" icon={<CloseIcon boxSize={2.5} />} size="xs" variant="ghost" color="whiteAlpha.800" onClick={clearPreview} />
              </Tooltip>
            )}
            <Tooltip label={open ? 'Collapse versions' : 'Workspace versions'}>
              <IconButton aria-label="Workspace versions" icon={<TimeIcon boxSize={3.5} />} size="xs" variant="ghost" color="whiteAlpha.800" onClick={() => setOpen((value) => !value)} />
            </Tooltip>
          </HStack>
        </HStack>

        <Collapse in={open} animateOpacity>
          <VStack align="stretch" spacing={3} px={3} pb={3} borderTop="1px solid" borderColor="whiteAlpha.100">
            <HStack pt={3} spacing={2}>
              <Select size="sm" value={repoId} onChange={(event) => setRepoId(Number(event.target.value) || '')}>
                {repos.map((repo) => <option key={repo.id} value={repo.id}>{repo.display_name}</option>)}
              </Select>
              <Tooltip label="Refresh versions">
                <IconButton aria-label="Refresh versions" icon={loading ? <Spinner size="xs" /> : <RepeatIcon />} size="sm" variant="ghost" onClick={load} />
              </Tooltip>
            </HStack>

            <Select size="sm" value={versionId} onChange={(event) => setVersionId(Number(event.target.value) || '')}>
              {versions.map((version) => (
                <option key={version.id} value={version.id}>
                  {shortHash(version.commit_hash)} · {version.branch || 'detached'} · {new Date(version.created_at).toLocaleString()}
                </option>
              ))}
            </Select>

            <HStack spacing={2} flexWrap="wrap">
              <Badge colorScheme="green">+{diffs.filter((d) => d.change_type === 'added').length}</Badge>
              <Badge colorScheme="yellow">~{diffs.filter((d) => d.change_type === 'updated').length}</Badge>
              <Badge colorScheme="red">-{diffs.filter((d) => d.change_type === 'deleted').length}</Badge>
              <Text fontSize="11px" color="gray.400">
                {workspaceVersions.length} workspace snapshots
              </Text>
            </HStack>

            {workspaceVersions.length > 0 && (
              <VStack align="stretch" spacing={1} maxH="92px" overflowY="auto" borderTop="1px solid" borderColor="whiteAlpha.100" pt={2}>
                {workspaceVersions.slice(0, 5).map((version) => (
                  <HStack key={version.id} justify="space-between" spacing={3}>
                    <Text fontSize="11px" color="gray.300" noOfLines={1}>
                      {version.description || version.version_id}
                    </Text>
                    <Text fontSize="10px" color="gray.500" flexShrink={0}>
                      {version.element_count} el · {version.connector_count} conn
                    </Text>
                  </HStack>
                ))}
              </VStack>
            )}

            <HStack spacing={2}>
              <Button size="sm" leftIcon={<ViewIcon />} onClick={applyPreview} isDisabled={!selectedVersion}>
                Preview diff
              </Button>
              <Button size="sm" variant="outline" onClick={follow} isDisabled={!selectedVersion}>
                Follow
              </Button>
              <Tooltip label="Rollback needs a backend restore endpoint for workspace snapshots.">
                <Button size="sm" variant="ghost" isDisabled>
                  Rollback
                </Button>
              </Tooltip>
            </HStack>
          </VStack>
        </Collapse>
      </Box>
    </Box>
  )
}
