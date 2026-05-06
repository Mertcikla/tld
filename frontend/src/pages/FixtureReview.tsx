import { useEffect, useMemo, useState } from 'react'
import { Box, Badge, Code, Flex, Heading, Spinner, Text, VStack, HStack, Divider } from '@chakra-ui/react'
import { apiUrl } from '../config/runtime'

export interface FixtureManifest {
  name: string
  status: string
  language?: string
  domain?: string
  framework?: string
  type?: string
  notes?: string[]
  review_status?: string
  accuracy?: string
  review_comments?: string[]
}

export interface FixtureSnapshot {
  name: string
  counts?: Record<string, number>
  elements?: FixtureElement[]
  connectors?: FixtureConnector[]
  views?: FixtureView[]
  facts?: FixtureFact[]
  filter_decisions?: FixtureDecision[]
}

interface FixtureElement {
  owner_type: string
  owner_key: string
  name: string
  kind?: string
  technology?: string
  file_path?: string
  tags?: string[]
}

interface FixtureConnector {
  owner_type: string
  owner_key: string
  label?: string
  source: string
  target: string
  view: string
}

interface FixtureView {
  owner_type: string
  owner_key: string
  name: string
  level: number
}

interface FixtureFact {
  type: string
  enricher: string
  stable_key: string
  file_path: string
  name?: string
  tags?: string[]
}

interface FixtureDecision {
  owner_type: string
  owner_key: string
  decision: string
  reason?: string
  signals?: string[]
}

export interface FixtureSnapshotResponse {
  manifest: FixtureManifest
  snapshot: FixtureSnapshot
}

export default function FixtureReview() {
  const fixture = useMemo(() => new URLSearchParams(window.location.search).get('fixture') ?? '', [])
  const [data, setData] = useState<FixtureSnapshotResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let mounted = true
    fetch(apiUrl(`/dev/fixtures/snapshot?fixture=${encodeURIComponent(fixture)}`))
      .then(async (res) => {
        if (!res.ok) throw new Error((await res.json().catch(() => null))?.error ?? res.statusText)
        return res.json() as Promise<FixtureSnapshotResponse>
      })
      .then((next) => mounted && setData(next))
      .catch((err: Error) => mounted && setError(err.message))
    return () => {
      mounted = false
    }
  }, [fixture])

  if (error) {
    return (
      <CenterShell>
        <Text color="red.500">{error}</Text>
      </CenterShell>
    )
  }

  if (!data) {
    return (
      <CenterShell>
        <Spinner size="xl" />
      </CenterShell>
    )
  }

  return <FixtureSnapshotView fixture={fixture} data={data} />
}

export function FixtureSnapshotView({ fixture, data }: { fixture: string; data: FixtureSnapshotResponse }) {
  const snapshot = data.snapshot
  const manifest = data.manifest
  return (
    <Box minH="100dvh" bg="gray.50" color="gray.900" p={{ base: 4, md: 6 }} overflow="auto">
      <VStack align="stretch" spacing={5} maxW="1280px" mx="auto">
        <Box>
          <HStack spacing={3} flexWrap="wrap">
            <Heading size="lg">{manifest.name || snapshot.name || fixture}</Heading>
            <Badge colorScheme="blue">{manifest.status || 'fixture'}</Badge>
            <Badge colorScheme="purple">{manifest.review_status || 'pending'}</Badge>
            <Badge>{manifest.accuracy || 'accuracy unset'}</Badge>
          </HStack>
          <Text color="gray.600" mt={2}>{fixture}</Text>
          <Text color="gray.600">{[manifest.language, manifest.domain, manifest.framework, manifest.type].filter(Boolean).join(' / ')}</Text>
        </Box>

        <Section title="Golden Counts">
          <HStack spacing={3} flexWrap="wrap">
            {Object.entries(snapshot.counts ?? {
              elements: snapshot.elements?.length ?? 0,
              connectors: snapshot.connectors?.length ?? 0,
              views: snapshot.views?.length ?? 0,
              facts: snapshot.facts?.length ?? 0,
              filter_decisions: snapshot.filter_decisions?.length ?? 0,
            }).map(([key, value]) => (
              <Badge key={key} colorScheme="green" variant="subtle">{key}: {value}</Badge>
            ))}
          </HStack>
        </Section>

        <Grid>
          <Section title="Elements">
            {(snapshot.elements ?? []).map((item) => (
              <Row key={`${item.owner_type}:${item.owner_key}`}>
                <Text fontWeight="semibold">{item.name}</Text>
                <Code>{item.owner_type}:{item.owner_key}</Code>
                <Text color="gray.600">{[item.kind, item.technology, item.file_path].filter(Boolean).join(' - ')}</Text>
                <Tags values={item.tags ?? []} />
              </Row>
            ))}
          </Section>

          <Section title="Facts">
            {(snapshot.facts ?? []).map((item) => (
              <Row key={`${item.type}:${item.enricher}:${item.stable_key}`}>
                <Text fontWeight="semibold">{item.type}</Text>
                <Code>{item.stable_key}</Code>
                <Text color="gray.600">{item.enricher} - {item.file_path} {item.name ? `- ${item.name}` : ''}</Text>
                <Tags values={item.tags ?? []} />
              </Row>
            ))}
          </Section>
        </Grid>

        <Grid>
          <Section title="Views">
            {(snapshot.views ?? []).map((item) => (
              <Row key={`${item.owner_type}:${item.owner_key}`}>
                <Text fontWeight="semibold">{item.name}</Text>
                <Code>{item.owner_type}:{item.owner_key}</Code>
                <Text color="gray.600">level {item.level}</Text>
              </Row>
            ))}
          </Section>

          <Section title="Connectors">
            {(snapshot.connectors ?? []).map((item) => (
              <Row key={`${item.owner_type}:${item.owner_key}`}>
                <Text fontWeight="semibold">{item.label || 'connector'}</Text>
                <Code>{item.owner_type}:{item.owner_key}</Code>
                <Text color="gray.600">{item.source} {'->'} {item.target}</Text>
                <Text color="gray.500">{item.view}</Text>
              </Row>
            ))}
          </Section>
        </Grid>

        <Section title="Filter Decisions">
          {(snapshot.filter_decisions ?? []).map((item) => (
            <Row key={`${item.owner_type}:${item.owner_key}`}>
              <HStack>
                <Badge colorScheme={item.decision === 'visible' ? 'green' : 'gray'}>{item.decision}</Badge>
                <Code>{item.owner_type}:{item.owner_key}</Code>
              </HStack>
              <Text color="gray.600">{item.reason}</Text>
              <Tags values={item.signals ?? []} />
            </Row>
          ))}
        </Section>
      </VStack>
    </Box>
  )
}

function CenterShell({ children }: { children: React.ReactNode }) {
  return <Flex minH="100dvh" align="center" justify="center" bg="gray.50">{children}</Flex>
}

function Grid({ children }: { children: React.ReactNode }) {
  return <Box display="grid" gridTemplateColumns={{ base: '1fr', lg: '1fr 1fr' }} gap={5}>{children}</Box>
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Box bg="white" border="1px solid" borderColor="gray.200" borderRadius="8px" p={4}>
      <Heading size="sm" mb={3}>{title}</Heading>
      <VStack align="stretch" spacing={3}>
        {children && (Array.isArray(children) ? children.length > 0 : true) ? children : <Text color="gray.500">none</Text>}
      </VStack>
    </Box>
  )
}

function Row({ children }: { children: React.ReactNode }) {
  return (
    <Box>
      <VStack align="stretch" spacing={1}>{children}</VStack>
      <Divider mt={3} />
    </Box>
  )
}

function Tags({ values }: { values: string[] }) {
  if (values.length === 0) return null
  return (
    <HStack spacing={1} flexWrap="wrap">
      {values.map((value) => <Badge key={value} variant="outline">{value}</Badge>)}
    </HStack>
  )
}
