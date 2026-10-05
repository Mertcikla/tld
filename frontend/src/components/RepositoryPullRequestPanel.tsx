import { Alert, AlertIcon, Box, Button, Flex, HStack, IconButton, Input, Select, Text } from '@chakra-ui/react'
import { RepeatIcon } from '@chakra-ui/icons'
import type { OpenRepositoryPullRequest, RepositoryPullRequest } from '../api/client'

type Props = {
  repositoryUrl?: string
  input: string
  requests: OpenRepositoryPullRequest[] | null
  selected: RepositoryPullRequest | null
  busy: boolean
  loading: boolean
  error: string
  onInput: (value: string) => void
  onSelect: (value: string) => void
  onLoad: () => void
  onRefresh: () => void
}


export default function RepositoryPullRequestPanel({ repositoryUrl, input, requests, selected, busy, loading, error, onInput, onSelect, onLoad, onRefresh }: Props) {
  const items = requests ?? []
  const hasRequests = items.length > 0
  return (
    <Box px={3} py={2} borderBottom="1px solid" borderColor="whiteAlpha.100">
      <Box overflowX="auto">
        <Flex as="form" align="center" gap={2} minW={hasRequests ? '600px' : '420px'} onSubmit={(event) => { event.preventDefault(); onLoad() }}>
          <HStack spacing={1.5} minW="80px" flex={1} color="gray.400">

            {repositoryUrl ? (
              <Text as="a" href={repositoryUrl} target="_blank" rel="noreferrer" title={repositoryUrl} fontSize="xs" minW={0} isTruncated _hover={{ color: 'var(--accent)', textDecoration: 'underline' }}>{repositoryUrl}</Text>
            ) : <Text fontSize="xs" isTruncated>No remote URL</Text>}
            <IconButton type="button" aria-label="Load open PRs" title="Load open PRs" size="xs" h="28px" w="28px" minW="28px" p={0} variant="solid" icon={<RepeatIcon boxSize="13px" />} flexShrink={0} isLoading={loading} isDisabled={busy || !repositoryUrl} onClick={onRefresh} />
          </HStack>
          {hasRequests && (
            <Box minW="120px" flex={1.2}>
            <Select size="xs" h="28px" w="full" aria-label="Open pull requests" value={items.some((pr) => String(pr.number) === input) ? input : ''} isDisabled={busy || loading} onChange={(event) => onSelect(event.target.value)}>
              <option value="">Open PRs ({items.length})</option>
              {items.map((pr) => <option key={pr.number} value={String(pr.number)}>#{pr.number} · {pr.title}</option>)}
            </Select>
            </Box>
          )}
          <Input size="xs" h="28px" minW="110px" flex={1} aria-label="Pull request number or URL" placeholder="PR number or URL" value={input} isDisabled={busy} onChange={(event) => onInput(event.target.value)} />
          <Button size="xs" h="28px" type="submit" flexShrink={0} variant="outline" isLoading={busy && !selected} isDisabled={busy || !input.trim()}>Load PR</Button>
        </Flex>
      </Box>
      {error && <Alert status="error" mt={2} py={2} borderRadius="md"><AlertIcon /><Text fontSize="xs">{error}</Text></Alert>}
      {requests !== null && !hasRequests && <Text mt={2} fontSize="xs" color="gray.500">No open pull requests.</Text>}
      {selected && (
        <Box mt={3} pt={3} borderTop="1px solid" borderColor="whiteAlpha.100">
          <Text as="a" href={selected.url} target="_blank" rel="noreferrer" fontSize="sm" fontWeight="semibold" _hover={{ color: 'var(--accent)' }}>{selected.title}</Text>
          <Flex mt={1} gap={2} align="center" wrap="wrap" fontSize="xs" color="gray.500">
            <Text>{selected.headBranch} → {selected.baseBranch}</Text>
            <Text color="gray.600">·</Text>
            <Text>Base and head locked to PR commits</Text>
          </Flex>
        </Box>
      )}
    </Box>
  )
}
