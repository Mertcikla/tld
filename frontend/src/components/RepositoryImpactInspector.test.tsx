import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import type { RepositoryImpactScene } from '../api/client'
import RepositoryImpactInspector from './RepositoryImpactInspector'

vi.mock('../utils/toast', () => ({ toast: vi.fn() }))
vi.mock('../api/client', () => ({ api: { repositories: { impactMermaid: vi.fn() } } }))
vi.mock('@chakra-ui/icons', () => ({ ChevronLeftIcon: () => null, InfoIcon: () => null }))
vi.mock('@chakra-ui/react', () => {
  const component = ({ children, ...props }: { children?: React.ReactNode }) => React.createElement('div', props, children)
  return Object.fromEntries(['Box', 'Button', 'Flex', 'HStack', 'IconButton', 'Input', 'Spinner', 'Text', 'VStack'].map((name) => [name, component]))
})

const scene = {
  schemaVersion: '2', repositoryId: 'repo', comparisonKey: 'pair', coverage: [{ path: 'a.go', placementKeys: ['placement|1|2'], factsUnavailable: false }],
  requestedRadius: 2, effectiveRadius: 1, manifestComplete: true,
  placements: { 'placement|1|2': { key: 'placement|1|2', objectKind: 'placement', viewId: 1, resourceId: 2, name: 'Service', origin: 'authored', grounding: 'resolved', role: 'direct', stage: 'authored', reason: 'Preserved authored placement', paths: ['a.go'], distance: 0, witnesses: [], evidence: [], endpointKeys: [], partial: false } },
  audit: { references: {}, declaredConnectors: [], matches: [], witnesses: [], excludedSymbols: {}, dependencies: [], decisions: [{ stage: 'reduce', action: 'suppress', target: 'v1/e3', reason: 'Hidden authored context in Plain mode' }] },
} as unknown as RepositoryImpactScene

function hosts(renderer: ReturnType<typeof create>, id: string) {
  return renderer.root.findAll((node) => node.type === 'div' && node.props['data-testid'] === id)
}

describe('RepositoryImpactInspector', () => {
  it('shows file representatives and exposes audit decisions only with debug enabled', async () => {
    const onSelect = vi.fn()
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<RepositoryImpactInspector scene={scene} selectedKey="placement|1|2" onSelect={onSelect} debug={false} onDebugChange={() => {}} plain={false} />) })
    expect(hosts(renderer, 'impact-selected-provenance')).toHaveLength(1)
    expect(hosts(renderer, 'impact-pipeline-stages')).toHaveLength(0)
    expect(hosts(renderer, 'impact-coverage-summary')[0].find((node) => node.type === 'div' && node.props.fontWeight === 'medium').children.join('')).toBe('1 changed files represented')
    await act(async () => { hosts(renderer, 'impact-primitive')[0].props.onClick() })
    expect(onSelect).toHaveBeenCalledWith('placement|1|2')
    await act(async () => { renderer.update(<RepositoryImpactInspector scene={scene} selectedKey="placement|1|2" onSelect={onSelect} debug onDebugChange={() => {}} plain={false} />) })
    expect(hosts(renderer, 'impact-pipeline-stages')).toHaveLength(0)
    expect(hosts(renderer, 'impact-decisions')).toHaveLength(1)
    await act(async () => { renderer.unmount() })
  })
})
