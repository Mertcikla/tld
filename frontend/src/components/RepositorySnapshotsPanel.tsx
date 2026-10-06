import { useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  Alert,
  AlertIcon,
  Badge,
  Box,
  Button,
  Flex,
  Grid,
  HStack,
  IconButton,
  Spinner,
  Switch,
  Text,
  Tooltip,
  VStack,
} from '@chakra-ui/react'
import { DeleteIcon } from '@chakra-ui/icons'
import type { CodeSnapshot, CompletedRepositoryMap, RepositoryIndexerCheck } from '../api/client'
import RepositoryIndexerChecklist from './RepositoryIndexerChecklist'

const snapshotPageSize = 5
const accentStyle = {
  bg: 'var(--accent)',
  color: 'white',
  _hover: { bg: 'var(--accent)', filter: 'brightness(1.08)' },
}

export interface RepositorySnapshotsPanelProps {
  snapshots: CodeSnapshot[]
  maps: CompletedRepositoryMap[]
  loading?: boolean
  // busy is true while any capture or map operation runs for the repository.
  busy?: boolean
  // capturing distinguishes a running snapshot capture from a running map.
  capturing?: boolean
  deleting?: boolean
  // isGit is false when the repository has no Git checkout; commit captures
  // are unavailable and the panel always captures the working tree.
  isGit?: boolean
  onDelete: (snapshot: CodeSnapshot) => void
  onCapture?: (options: { workingTree: boolean }) => void
  onMap?: (snapshot: CodeSnapshot) => void
  onOpenMap?: (viewId: number) => void
  onCancel?: () => void
  // Indexer scout state for the repository; rendered when onCheckIndexers is set.
  indexerCheck?: RepositoryIndexerCheck | null
  checkingIndexers?: boolean
  indexersOpen?: boolean
  onCheckIndexers?: () => void
  onToggleIndexers?: () => void
}

function age(unix: number) {
  const minutes = Math.max(0, Math.floor((Date.now() / 1000 - unix) / 60))
  return minutes < 1
    ? 'just now'
    : minutes < 60
      ? `${minutes}m ago`
      : minutes < 1440
        ? `${Math.floor(minutes / 60)}h ago`
        : `${Math.floor(minutes / 1440)}d ago`
}

function provenanceLabel(provenance?: string) {
  switch (provenance) {
    case 'commit':
      return 'Commit'
    case 'manual':
      return 'Manual'
    case 'working_tree':
      return 'Working tree'
    default:
      return 'Unknown provenance'
  }
}

function provenanceColorScheme(provenance?: string) {
  switch (provenance) {
    case 'commit':
      return 'blue'
    case 'manual':
      return 'purple'
    case 'working_tree':
      return 'orange'
    default:
      return 'gray'
  }
}

function Label({ children }: { children: ReactNode }) {
  return (
    <Text
      fontSize="10px"
      fontWeight="bold"
      color="gray.500"
      textTransform="uppercase"
      letterSpacing="0.06em"
    >
      {children}
    </Text>
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

// SnapshotDetails renders the metadata-only view of one snapshot: identity,
// capture provenance, statistics, project manifests, and indexing warnings.
function SnapshotDetails({ snapshot, mapped, busy, onMap, onOpenMap }: {
  snapshot: CodeSnapshot
  mapped?: CompletedRepositoryMap
  busy?: boolean
  onMap?: (snapshot: CodeSnapshot) => void
  onOpenMap?: (viewId: number) => void
}) {
  const statistics = snapshot.statistics
  const toolVersions = Object.entries(snapshot.toolVersions ?? {})
  return (
    <Box
      mt={3}
      pt={3}
      borderTop="1px solid"
      borderColor="whiteAlpha.200"
      data-testid={`repositories-snapshot-details-${snapshot.id}`}
    >
      <Grid as="dl" templateColumns="repeat(auto-fit, minmax(min(100%, 200px), 1fr))" gap={3}>
        <InfoField label="Snapshot ID" value={snapshot.id} mono />
        <InfoField label="Captured" value={new Date(snapshot.createdUnix * 1000).toLocaleString()} />
        <InfoField label="Branch" value={snapshot.gitBranch || '—'} />
        <InfoField label="Revision" value={snapshot.gitRevision || '—'} mono />
        <InfoField label="Provenance" value={provenanceLabel(snapshot.provenance)} />
        <InfoField label="Ingestion" value={snapshot.ingestionStatus || '—'} />
        {statistics && (
          <InfoField
            label="Statistics"
            value={`${statistics.facts.toLocaleString()} facts · ${statistics.edges.toLocaleString()} edges · ${statistics.sources.toLocaleString()} files`}
          />
        )}
        {snapshot.contentFingerprint && <InfoField label="Content fingerprint" value={snapshot.contentFingerprint} mono />}
        {snapshot.configHash && <InfoField label="Index configuration" value={snapshot.configHash} mono />}
        {mapped && (
          <InfoField
            label="Mapped into workspace"
            value={`${new Date(mapped.completedUnix * 1000).toLocaleString()} · view ${mapped.result.viewId}`}
          />
        )}
      </Grid>
      {snapshot.commitMessage && (
        <Box mt={3}>
          <InfoField label="Commit message" value={snapshot.commitMessage} />
        </Box>
      )}
      {snapshot.projects.length > 0 && (
        <Box mt={3}>
          <Text fontSize="xs" color="gray.400" mb={1}>Projects</Text>
          {snapshot.projects.map((project) => (
            <Text key={`${project.root}:${project.configPath}`} fontSize="xs" color="gray.300" fontFamily="mono" overflowWrap="anywhere">
              {project.language} · {project.root}{project.configPath ? ` · ${project.configPath}` : ''}
            </Text>
          ))}
        </Box>
      )}
      {toolVersions.length > 0 && (
        <Box mt={3}>
          <Text fontSize="xs" color="gray.400" mb={1}>Tool versions</Text>
          {toolVersions.map(([tool, version]) => (
            <Text key={tool} fontSize="xs" color="gray.300" fontFamily="mono" overflowWrap="anywhere">
              {tool}: {version}
            </Text>
          ))}
        </Box>
      )}
      {snapshot.warnings.length > 0 && (
        <Alert status="warning" mt={3} borderRadius="md" alignItems="flex-start" data-testid={`repositories-snapshot-warnings-${snapshot.id}`}>
          <AlertIcon />
          <Box minW={0} flex={1}>
            <Text fontSize="xs" fontWeight="semibold">{snapshot.warnings.length} indexing {snapshot.warnings.length === 1 ? 'warning' : 'warnings'}</Text>
            {snapshot.warnings.map((warning, index) => (
              <Text key={index} fontSize="xs" mt={0.5} overflowWrap="anywhere">{warning}</Text>
            ))}
          </Box>
        </Alert>
      )}
      {onMap && (
        <Flex justify="flex-end" mt={3} pt={3} borderTop="1px solid" borderColor="whiteAlpha.200">
          {mapped ? (
            <Button
              size="xs"
              variant="outline"
              data-testid={`repositories-snapshot-open-map-${snapshot.id}`}
              onClick={() => onOpenMap?.(mapped.result.viewId)}
            >
              Open map
            </Button>
          ) : (
            <Button
              size="xs"
              style={accentStyle}
              data-testid={`repositories-snapshot-map-${snapshot.id}`}
              isDisabled={busy}
              onClick={() => onMap(snapshot)}
            >
              Map into workspace
            </Button>
          )}
        </Flex>
      )}
    </Box>
  )
}

// RepositorySnapshotsPanel lists a repository's saved snapshots and exposes the
// snapshot lifecycle: capture, view metadata, map into the workspace, and
// delete. It is shared by the Repositories Snapshots tab and repository
// settings; capture and map actions only render when their handlers are given.
export default function RepositorySnapshotsPanel({
  snapshots,
  maps,
  loading = false,
  busy = false,
  capturing = false,
  deleting = false,
  isGit,
  onDelete,
  onCapture,
  onMap,
  onOpenMap,
  onCancel,
  indexerCheck = null,
  checkingIndexers = false,
  indexersOpen = true,
  onCheckIndexers,
  onToggleIndexers,
}: RepositorySnapshotsPanelProps) {
  const [visibleCount, setVisibleCount] = useState(snapshotPageSize)
  const [viewingId, setViewingId] = useState('')
  const [captureWorkingTree, setCaptureWorkingTree] = useState(true)

  useEffect(() => {
    setVisibleCount(snapshotPageSize)
  }, [snapshots])
  useEffect(() => {
    if (viewingId && !snapshots.some((snapshot) => snapshot.id === viewingId)) setViewingId('')
  }, [snapshots, viewingId])

  const newest = useMemo(() => [...snapshots].reverse(), [snapshots])
  const visible = newest.slice(0, visibleCount)
  const mappedBySnapshot = useMemo(() => {
    const bySnapshot = new Map<string, CompletedRepositoryMap>()
    for (const map of maps) {
      if (map.result.snapshotId) bySnapshot.set(map.result.snapshotId, map)
    }
    return bySnapshot
  }, [maps])

  const canCaptureCommit = isGit !== false
  const workingTree = captureWorkingTree || !canCaptureCommit

  return (
    <Box data-testid="repositories-snapshots-panel">
      <Flex align="center" gap={2} wrap="wrap" mb={3}>
        <Label>Snapshots · {snapshots.length}</Label>
        <Box flex={1} />
        {onCapture && (
          <HStack spacing={3} wrap="wrap">
            {canCaptureCommit && (
              <HStack spacing={1.5}>
                <Switch
                  size="sm"
                  data-testid="repositories-snapshot-include-worktree"
                  isChecked={captureWorkingTree}
                  isDisabled={busy}
                  onChange={(event) => setCaptureWorkingTree(event.target.checked)}
                />
                <Text fontSize="xs" color="gray.400">Include uncommitted changes</Text>
              </HStack>
            )}
            <Button
              size="xs"
              style={accentStyle}
              data-testid="repositories-snapshot-capture"
              isLoading={capturing}
              loadingText="Capturing…"
              isDisabled={busy && !capturing}
              onClick={() => onCapture({ workingTree })}
            >
              Take snapshot
            </Button>
            {capturing && onCancel && (
              <Button size="xs" variant="ghost" color="gray.400" onClick={onCancel}>
                Cancel
              </Button>
            )}
          </HStack>
        )}
      </Flex>
      {onCheckIndexers && (
        <RepositoryIndexerChecklist
          check={indexerCheck}
          loading={checkingIndexers}
          open={indexersOpen}
          onToggle={onToggleIndexers ?? (() => {})}
          onCheck={onCheckIndexers}
        />
      )}
      {loading && !snapshots.length && <Spinner size="xs" />}
      {!loading && !snapshots.length && (
        <Text fontSize="xs" color="gray.500">
          No snapshots recorded.
        </Text>
      )}
      <VStack align="stretch" spacing={2}>
        {visible.map((snapshot) => {
          const mapped = mappedBySnapshot.get(snapshot.id)
          const viewing = viewingId === snapshot.id
          return (
            <Box
              key={snapshot.id}
              px={3}
              py={2.5}
              bg="whiteAlpha.50"
              borderRadius="md"
              border="1px solid"
              borderColor="whiteAlpha.100"
              data-testid={`repositories-snapshot-${snapshot.id}`}
            >
              <Flex gap={2} align="center" wrap="wrap">
                <Text fontSize="sm" color="gray.200" fontWeight="medium" isTruncated title={snapshot.gitBranch || snapshot.gitRevision || snapshot.id}>
                  {snapshot.gitBranch || 'No captured branch'}
                </Text>
                <Badge colorScheme={provenanceColorScheme(snapshot.provenance)} fontSize="2xs" textTransform="none">
                  {provenanceLabel(snapshot.provenance)}
                </Badge>
                {mapped && <Badge colorScheme="green" fontSize="2xs" textTransform="none">Mapped</Badge>}
                {snapshot.ingestionStatus !== 'complete' && (
                  <Badge colorScheme="orange" fontSize="2xs" textTransform="none">
                    {snapshot.ingestionStatus || 'Incomplete'}
                  </Badge>
                )}
                <Box flex={1} />
                <Text
                  fontSize="10px"
                  color="gray.500"
                  whiteSpace="nowrap"
                  title={new Date(snapshot.createdUnix * 1000).toLocaleString()}
                >
                  {age(snapshot.createdUnix)}
                </Text>
              </Flex>
              {snapshot.commitMessage && (
                <Text fontSize="xs" color="gray.300" mt={1} isTruncated title={snapshot.commitMessage}>
                  {snapshot.commitMessage}
                </Text>
              )}
              {snapshot.statistics ? (
                <Text fontSize="10px" color="gray.400" mt={1}>
                  {snapshot.statistics.facts.toLocaleString()} facts · {snapshot.statistics.edges.toLocaleString()} edges · {snapshot.statistics.sources.toLocaleString()} files
                </Text>
              ) : (
                <Text fontSize="10px" color="gray.500" mt={1}>Statistics unavailable</Text>
              )}
              {!viewing && snapshot.warnings.length > 0 && (
                <Text fontSize="10px" color="orange.300" mt={1}>
                  {snapshot.warnings.length} indexing {snapshot.warnings.length === 1 ? 'warning' : 'warnings'}
                </Text>
              )}
              <HStack spacing={2} mt={2}>
                <Button
                  size="xs"
                  variant="ghost"
                  color="gray.300"
                  data-testid={`repositories-snapshot-view-${snapshot.id}`}
                  aria-expanded={viewing}
                  onClick={() => setViewingId(viewing ? '' : snapshot.id)}
                >
                  {viewing ? 'Hide details' : 'View details'}
                </Button>
                <Box flex={1} />
                <Tooltip label="Delete snapshot" placement="top" openDelay={200}>
                  <IconButton
                    aria-label={`Delete snapshot ${snapshot.id}`}
                    data-testid={`repositories-snapshot-delete-${snapshot.id}`}
                    icon={<DeleteIcon boxSize="12px" />}
                    size="xs"
                    variant="ghost"
                    color="red.400"
                    isDisabled={deleting}
                    onClick={(event) => {
                      event.stopPropagation()
                      onDelete(snapshot)
                    }}
                  />
                </Tooltip>
              </HStack>
              {viewing && <SnapshotDetails snapshot={snapshot} mapped={mapped} busy={busy} onMap={onMap} onOpenMap={onOpenMap} />}
            </Box>
          )
        })}
      </VStack>
      {(visibleCount < snapshots.length || visibleCount > snapshotPageSize) && (
        <HStack data-testid="repositories-snapshots-pagination" mt={3} spacing={2}>
          {visibleCount > snapshotPageSize && (
            <Button
              data-testid="repositories-snapshots-show-less"
              size="xs"
              variant="ghost"
              flex={1}
              color="gray.400"
              onClick={() => setVisibleCount(snapshotPageSize)}
            >
              Show newest {snapshotPageSize}
            </Button>
          )}
          {visibleCount < snapshots.length && (
            <Button
              data-testid="repositories-snapshots-load-more"
              size="xs"
              variant="ghost"
              flex={1}
              color="gray.400"
              onClick={() => setVisibleCount((count) => Math.min(count + snapshotPageSize, snapshots.length))}
            >
              Load {Math.min(snapshotPageSize, snapshots.length - visibleCount)} more
            </Button>
          )}
        </HStack>
      )}
    </Box>
  )
}
