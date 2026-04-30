import { useCallback, useEffect, useMemo, useState } from 'react'
import { ExternalLinkIcon, RepeatIcon } from '@chakra-ui/icons'
import {
  Box,
  Button,
  HStack,
  Select,
  Spinner,
  Table,
  Tbody,
  Td,
  Text,
  Th,
  Thead,
  Tr,
  VStack,
} from '@chakra-ui/react'
import { api, type WatchDiff, type WatchRepository, type WatchVersion } from '../api/client'
import { useSetHeader } from '../components/HeaderContext'

function shortHash(value?: string) {
  if (!value) return ''
  return value.length > 10 ? value.slice(0, 10) : value
}

export default function WatchHistory() {
  const setHeader = useSetHeader()
  const [repos, setRepos] = useState<WatchRepository[]>([])
  const [repoId, setRepoId] = useState<number | null>(null)
  const [versions, setVersions] = useState<WatchVersion[]>([])
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null)
  const [diffs, setDiffs] = useState<WatchDiff[]>([])
  const [loading, setLoading] = useState(true)
  const [ownerFilter, setOwnerFilter] = useState('')
  const [changeFilter, setChangeFilter] = useState('')
  const [resourceFilter, setResourceFilter] = useState('')
  const [languageFilter, setLanguageFilter] = useState('')

  const currentRepo = useMemo(() => repos.find((repo) => repo.id === repoId) ?? null, [repos, repoId])

  const loadRepositories = useCallback(async () => {
    setLoading(true)
    try {
      const nextRepos = await api.watch.repositories()
      setRepos(nextRepos)
      const nextRepoId = repoId ?? nextRepos[0]?.id ?? null
      setRepoId(nextRepoId)
      if (nextRepoId) {
        const nextVersions = await api.watch.versions(nextRepoId)
        setVersions(nextVersions)
        setSelectedVersion(nextVersions[0]?.id ?? null)
      }
    } finally {
      setLoading(false)
    }
  }, [repoId])

  useEffect(() => {
    setHeader(
      <HStack w="full" justify="space-between">
        <Text fontWeight="700">Watch History</Text>
        <Button size="sm" leftIcon={<RepeatIcon />} onClick={() => loadRepositories()}>
          Refresh
        </Button>
      </HStack>
    )
    return () => setHeader(null)
  }, [loadRepositories, setHeader])

  useEffect(() => {
    loadRepositories()
  }, [loadRepositories])

  useEffect(() => {
    if (!repoId) return
    api.watch.versions(repoId).then((next) => {
      setVersions(next)
      setSelectedVersion(next[0]?.id ?? null)
    })
  }, [repoId])

  useEffect(() => {
    if (!selectedVersion) {
      setDiffs([])
      return
    }
    api.watch.diffs(selectedVersion, { owner_type: ownerFilter, change_type: changeFilter, resource_type: resourceFilter, language: languageFilter }).then(setDiffs)
  }, [selectedVersion, ownerFilter, changeFilter, resourceFilter, languageFilter])

  if (loading) {
    return (
      <Box h="100%" display="grid" placeItems="center">
        <Spinner />
      </Box>
    )
  }

  return (
    <Box h="100%" overflow="auto" bg="var(--bg-canvas)" color="var(--text-primary)">
      <VStack align="stretch" spacing={4} maxW="1180px" mx="auto" px={{ base: 3, md: 6 }} py={5}>
        <HStack spacing={3} align="center" flexWrap="wrap">
          <Select size="sm" maxW="360px" value={repoId ?? ''} onChange={(event) => setRepoId(Number(event.target.value))}>
            {repos.map((repo) => (
              <option key={repo.id} value={repo.id}>{repo.display_name}</option>
            ))}
          </Select>
          {currentRepo && (
            <Text fontSize="sm" color="var(--text-muted)">
              {currentRepo.branch || 'detached'} · {shortHash(currentRepo.head_commit || undefined)}
            </Text>
          )}
        </HStack>

        <Box overflowX="auto" border="1px solid var(--border-subtle)" borderRadius="8px">
          <Table size="sm">
            <Thead>
              <Tr>
                <Th>Commit</Th>
                <Th>Branch</Th>
                <Th>Representation</Th>
                <Th>Created</Th>
              </Tr>
            </Thead>
            <Tbody>
              {versions.map((version) => (
                <Tr
                  key={version.id}
                  bg={selectedVersion === version.id ? 'rgba(var(--accent-rgb), 0.10)' : undefined}
                  cursor="pointer"
                  onClick={() => setSelectedVersion(version.id)}
                >
                  <Td fontFamily="mono">{shortHash(version.commit_hash)}</Td>
                  <Td>{version.branch || ''}</Td>
                  <Td fontFamily="mono">{shortHash(version.representation_hash)}</Td>
                  <Td>{new Date(version.created_at).toLocaleString()}</Td>
                </Tr>
              ))}
              {versions.length === 0 && (
                <Tr>
                  <Td colSpan={4}>
                    <Text color="var(--text-muted)">No watch versions have been created yet.</Text>
                  </Td>
                </Tr>
              )}
            </Tbody>
          </Table>
        </Box>

        <HStack spacing={3} flexWrap="wrap">
          <Select size="sm" maxW="220px" value={ownerFilter} onChange={(event) => setOwnerFilter(event.target.value)}>
            <option value="">All owners</option>
            <option value="repository">Repository</option>
            <option value="file">File</option>
            <option value="symbol">Symbol</option>
          </Select>
          <Select size="sm" maxW="220px" value={changeFilter} onChange={(event) => setChangeFilter(event.target.value)}>
            <option value="">All changes</option>
            <option value="added">Added</option>
            <option value="updated">Updated</option>
            <option value="deleted">Deleted</option>
          </Select>
          <Select size="sm" maxW="220px" value={resourceFilter} onChange={(event) => setResourceFilter(event.target.value)}>
            <option value="">All resources</option>
            <option value="file">File</option>
            <option value="symbol">Symbol</option>
            <option value="element">Element</option>
            <option value="view">View</option>
            <option value="connector">Connector</option>
          </Select>
          <Select size="sm" maxW="220px" value={languageFilter} onChange={(event) => setLanguageFilter(event.target.value)}>
            <option value="">All languages</option>
            <option value="go">Go</option>
            <option value="typescript">TypeScript</option>
            <option value="javascript">JavaScript</option>
            <option value="python">Python</option>
            <option value="java">Java</option>
            <option value="c">C</option>
            <option value="cpp">C++</option>
          </Select>
        </HStack>

        <Box overflowX="auto" border="1px solid var(--border-subtle)" borderRadius="8px">
          <Table size="sm">
            <Thead>
              <Tr>
                <Th>Change</Th>
                <Th>Owner</Th>
                <Th>Resource</Th>
                <Th>Language</Th>
                <Th>Summary</Th>
                <Th></Th>
              </Tr>
            </Thead>
            <Tbody>
              {diffs.map((diff) => (
                <Tr key={diff.id}>
                  <Td>{diff.change_type}</Td>
                  <Td>{diff.owner_type}</Td>
                  <Td fontFamily="mono">{diff.resource_id ?? diff.owner_key}</Td>
                  <Td>{diff.language || ''}</Td>
                  <Td>{diff.summary || ''}</Td>
                  <Td textAlign="right">
                    {diff.resource_type === 'element' && diff.resource_id && (
                      <Button as="a" href={`/views?element=${diff.resource_id}`} size="xs" variant="ghost" leftIcon={<ExternalLinkIcon />}>
                        Open
                      </Button>
                    )}
                  </Td>
                </Tr>
              ))}
              {diffs.length === 0 && (
                <Tr>
                  <Td colSpan={6}>
                    <Text color="var(--text-muted)">No diffs match the current filters.</Text>
                  </Td>
                </Tr>
              )}
            </Tbody>
          </Table>
        </Box>
      </VStack>
    </Box>
  )
}
