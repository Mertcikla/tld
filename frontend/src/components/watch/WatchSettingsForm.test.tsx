import React from 'react'
import { act, create, type ReactTestInstance, type ReactTestRenderer } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import type { WatchEmbeddingConfig, WatchSettings, WatchSettingsDescriptor } from '../../api/client'
import WatchSettingsForm from './WatchSettingsForm'

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const passthrough = (name: string) => {
    const Component = ({ children, ...rest }: { children?: React.ReactNode }) =>
      ReactModule.createElement(name, rest, children)
    Component.displayName = name
    return Component
  }
  return {
    Box: passthrough('Box'),
    Button: passthrough('Button'),
    Checkbox: passthrough('Checkbox'),
    FormControl: passthrough('FormControl'),
    FormLabel: passthrough('FormLabel'),
    HStack: passthrough('HStack'),
    Input: passthrough('Input'),
    NumberInput: passthrough('NumberInput'),
    NumberInputField: passthrough('NumberInputField'),
    Select: passthrough('Select'),
    SimpleGrid: passthrough('SimpleGrid'),
    Switch: passthrough('Switch'),
    Text: passthrough('Text'),
    VStack: passthrough('VStack'),
  }
})

const settings: WatchSettings = {
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
}

const embedding: WatchEmbeddingConfig = { provider: 'none', model: 'x', dimension: 512 }
const descriptors: WatchSettingsDescriptor[] = []

function collectText(node: ReactTestInstance | null): string {
  if (!node) return ''
  return node.children.map((child) => (typeof child === 'string' ? child : collectText(child))).join(' ')
}

function renderForm(): ReactTestRenderer {
  let renderer!: ReactTestRenderer
  act(() => {
    renderer = create(
      <WatchSettingsForm settings={settings} embedding={embedding} descriptors={descriptors} onChange={() => {}} />,
    )
  })
  return renderer
}

function advancedButton(renderer: ReactTestRenderer): ReactTestInstance {
  return renderer.root.findAll((node) => (node.type as unknown as string) === 'Button')[0]
}

describe('WatchSettingsForm progressive disclosure', () => {
  it('hides advanced detail settings behind the disclosure by default', () => {
    const text = collectText(renderForm().root)
    expect(text).toContain('Discovery')
    expect(text).toContain('Advanced')
    expect(text).not.toContain('Visibility weights')
    expect(text).not.toContain('Representation thresholds')
  })

  it('reveals advanced detail settings when expanded', () => {
    const renderer = renderForm()
    act(() => {
      advancedButton(renderer).props.onClick()
    })
    const text = collectText(renderer.root)
    expect(text).toContain('Visibility weights')
    expect(text).toContain('Representation thresholds')
    expect(text).toContain('Layout')
  })
})
