import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import WatchControlPanel from './WatchControlPanel'

const repositoriesMock = vi.fn(async () => [
  { id: 1, remote_url: null, repo_root: '/repo', display_name: 'repo', branch: 'main', head_commit: 'abc', identity_status: 'known' },
])
const settingsMock = vi.fn(async () => ({
  repository_id: 1,
  settings: {
    languages: ['go'],
    watcher: 'auto',
    poll_interval: 10_000_000_000,
    debounce: 500_000_000,
    dependencies: { enabled: false },
    thresholds: {
      max_elements_per_view: 100,
      max_connectors_per_view: 200,
      max_incoming_per_element: 20,
      max_outgoing_per_element: 20,
      max_expanded_connectors_per_group: 24,
    },
    visibility: {
      core_threshold_enabled: true,
      core_threshold: 1,
      tier_multiplier: 0.5,
      max_expansion_multiplier: 2,
      weights: {
        changed: 100,
        selected: 100,
        user_show: 100,
        user_hide: -100,
        high_signal_fact: 1.5,
        relationship_proximity: 1,
        dependency_fact: 0.2,
        utility_noise: -0.8,
        high_degree_noise: -1.5,
      },
    },
    scale: { strategy: 'auto', max_tracked_files: 20000, max_limited_files: 2000, max_recent_files: 1000, max_caller_depth: 10, max_blast_radius_hops: 1 },
    lsp: { enabled: true, health_interval: 60_000_000_000, memory_limit_bytes: 4294967296, commands: {} },
  },
  embedding: { provider: 'local-lexical', model: 'lexical-code-fingerprint-v1', dimension: 512 },
  overridden: false,
  descriptors: [{ key: 'watch.watcher', value: 'auto', description: 'watcher backend' }],
}))
const scanProgressMock = vi.fn(async () => ({
  repository_id: 1,
  summary: { repository_id: 1, files: 0, symbols: 0, references: 0 },
  representation: { repository_id: 1, elements_created: 0, elements_updated: 0, connectors_created: 0, connectors_updated: 0, views_created: 0 },
  watcher: { repository_id: 1, managed: false, live: false, paused: false },
}))

vi.mock('../../api/client', () => ({
  api: {
    watch: {
      repositories: () => repositoriesMock(),
      settings: () => settingsMock(),
      scanProgress: () => scanProgressMock(),
    },
  },
}))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const passthrough = (name: string) => {
    const Component = ({ children, ...rest }: { children?: React.ReactNode }) =>
      ReactModule.createElement(name, rest, children)
    Component.displayName = name
    return Component
  }
  return {
    Badge: passthrough('Badge'),
    Box: passthrough('Box'),
    Button: passthrough('Button'),
    Divider: passthrough('Divider'),
    FormLabel: passthrough('FormLabel'),
    HStack: passthrough('HStack'),
    Input: passthrough('Input'),
    Select: passthrough('Select'),
    Spinner: passthrough('Spinner'),
    Stack: passthrough('Stack'),
    Table: passthrough('Table'),
    Tbody: passthrough('Tbody'),
    Td: passthrough('Td'),
    Text: passthrough('Text'),
    Th: passthrough('Th'),
    Thead: passthrough('Thead'),
    Tr: passthrough('Tr'),
    VStack: passthrough('VStack'),
    Checkbox: passthrough('Checkbox'),
    FormControl: passthrough('FormControl'),
    NumberInput: passthrough('NumberInput'),
    NumberInputField: passthrough('NumberInputField'),
    SimpleGrid: passthrough('SimpleGrid'),
    Switch: passthrough('Switch'),
  }
})

describe('WatchControlPanel', () => {
  beforeEach(() => {
    repositoriesMock.mockClear()
    settingsMock.mockClear()
    scanProgressMock.mockClear()
  })

  it('does not load repositories or settings while disabled', async () => {
    await act(async () => {
      create(<WatchControlPanel enabled={false} />)
    })
    expect(repositoriesMock).not.toHaveBeenCalled()
    expect(settingsMock).not.toHaveBeenCalled()
  })

  it('loads repositories and settings when enabled', async () => {
    await act(async () => {
      create(<WatchControlPanel enabled />)
    })
    expect(repositoriesMock).toHaveBeenCalledTimes(1)
    expect(settingsMock).toHaveBeenCalledTimes(1)
    expect(scanProgressMock).toHaveBeenCalledTimes(1)
  })
})
