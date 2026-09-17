import { describe, expect, it } from 'vitest'
import type { ImpactEdge, ImpactElement, ImpactReport } from '../api/client'
import { boundedDiagram, buildImpactDiagram, impactMermaid, laneFor } from './impactMarkdown'

const node = (ref: string, owner?: string): ImpactElement => ({ ref, name: ref, kind: 'component', owner, change: 'modified', evidence: [] })
const edge = (source_ref: string, target_ref: string, observed = false): ImpactEdge => ({ source_ref, target_ref, observed, label: 'calls' })

function report(overrides: Partial<ImpactReport> = {}): ImpactReport {
  return {
    base: 'main',
    head: 'HEAD',
    repo_root: '/repo',
    changed: [],
    candidates: [],
    related: [],
    edges: [],
    unmapped: [],
    coverage: {
      applicable: true,
      complete: true,
      score: 1,
      percent: 100,
      confidence: 'high',
      source_files: 2,
      bound_source_files: 2,
      weak_source_files: 0,
      unmapped_source_files: 0,
      non_source_files: 0,
      anchored_elements: 2,
      total_elements: 2,
      gaps: [],
    },
    changed_files: [],
    ...overrides,
  }
}

function busyReport(): ImpactReport {
  const changed = [node('a'), node('b')]
  const related: ImpactElement[] = []
  const edges: ImpactEdge[] = [edge('a', 'b')]
  for (let index = 1; index <= 20; index += 1) {
    const ref = `r${index}`
    related.push(node(ref))
    edges.push(edge('a', ref), edge('b', ref))
  }
  return report({ changed, related, edges })
}

describe('buildImpactDiagram', () => {
  it('renders the single reviewer projection and never promotes candidates', () => {
    const diagram = buildImpactDiagram(report({
      changed: [node('checkout')],
      candidates: [node('weak')],
      related: [node('payment')],
      edges: [edge('checkout', 'payment', true)],
    }))

    expect(diagram.style).toBe('review')
    expect(diagram.omittedNodes).toBe(0)
    expect(diagram.omittedEdges).toBe(0)
    expect(diagram.groups).toEqual([])
    expect(diagram.code).toContain('flowchart LR')
    expect(diagram.code).toContain('subgraph lane_changed')
    expect(diagram.code).toContain('n1["checkout"]')
    expect(diagram.code).toContain('n2["payment"]')
    expect(diagram.code).toContain('n1 -.->|calls| n2')
    expect(diagram.code).not.toContain('weak')
    expect(diagram.nodes.some((item) => item.ref === 'weak')).toBe(false)
    expect(diagram.code).toBe(impactMermaid(report({
      changed: [node('checkout')],
      candidates: [node('weak')],
      related: [node('payment')],
      edges: [edge('checkout', 'payment', true)],
    })))
  })

  it('preserves valid directed relationships and ignores the deprecated style argument', () => {
    const fixture = { ...busyReport(), candidates: [node('weak')] }
    const before = JSON.stringify(fixture)
    for (const style of ['review', 'bounded', 'lanes', 'groups', 'full'] as const) {
      const diagram = buildImpactDiagram(fixture, style)
      const refs = new Set(diagram.nodes.map((item) => item.ref))
      for (const relationship of diagram.edges) {
        expect(refs.has(relationship.source_ref)).toBe(true)
        expect(refs.has(relationship.target_ref)).toBe(true)
      }
      expect(refs.has('weak')).toBe(false)
      expect(diagram.code).toBe(buildImpactDiagram(fixture).code)
    }
    expect(JSON.stringify(fixture)).toBe(before)
  })

  it('is deterministic regardless of report order', () => {
    const fixture = busyReport()
    const shuffled = report({
      ...fixture,
      changed: [...fixture.changed].reverse(),
      related: [...fixture.related].reverse(),
      edges: [...fixture.edges].reverse(),
    })
    expect(buildImpactDiagram(shuffled)).toEqual(buildImpactDiagram(fixture))
  })

  it('badges changed nodes with aggregate change instead of file paths', () => {
    const annotated = report({
      changed: [{ ...node('core'), evidence: ['internal/core.go', 'internal/util.go'] }],
      related: [node('api')],
      edges: [edge('api', 'core', true)],
      changed_files: [{ path: 'internal/core.go', change: 'modified', added: 12, removed: 3 }],
    })
    const diagram = buildImpactDiagram(annotated)
    expect(diagram.code).toContain('n1["core<br/>2 files (+12 -3)"]')
    expect(diagram.code).toContain('n2["api"]')
    expect(diagram.code).toContain('n2 -.->|calls| n1')
    expect(diagram.code).not.toContain('internal/core.go')
  })

  it('renders containment edges distinctly', () => {
    const diagram = buildImpactDiagram(report({
      changed: [node('backend'), node('checkout')],
      edges: [{ source_ref: 'backend', target_ref: 'checkout', label: 'contains', observed: false }],
    }))
    expect(diagram.code).toContain('n1 --o|contains| n2')
  })

  it('ranks distinct changed neighbors before observed evidence, then stable refs', () => {
    const fixture = report({
      changed: [node('a'), node('b')],
      related: ['shared', 'observed', 'alpha', 'zeta'].map((ref) => node(ref)),
      edges: [edge('a', 'shared'), edge('b', 'shared'), edge('a', 'observed', true), edge('a', 'zeta'), edge('a', 'alpha')],
    })
    expect(boundedDiagram(fixture, 4).nodes.map((item) => item.ref)).toEqual(['a', 'b', 'shared', 'observed'])
    expect(boundedDiagram(fixture, 5).nodes.map((item) => item.ref)).toEqual(['a', 'b', 'shared', 'observed', 'alpha'])
  })

  it('keeps changed nodes, mutual edges, and one edge per neighbor beyond soft limits', () => {
    const fixture = report({
      changed: [node('a'), node('b'), node('c')],
      related: [node('d'), node('e')],
      edges: [edge('a', 'b'), edge('b', 'c'), edge('c', 'a'), edge('d', 'a'), edge('b', 'e', true)],
    })
    expect(boundedDiagram(fixture, 2, 1).nodes).toHaveLength(3)
    expect(boundedDiagram(fixture, 2, 1).edges).toHaveLength(3)
    const roomy = boundedDiagram(fixture, 5, 1)
    expect(roomy.edges).toHaveLength(5)
    expect(roomy.omittedNodes).toBe(0)
    expect(roomy.omittedEdges).toBe(0)
  })

  it('reduces a busy report to the node/edge budget and accounts for omissions', () => {
    const fixture = busyReport()
    const diagram = buildImpactDiagram(fixture)
    expect(diagram.nodes).toHaveLength(10)
    expect(diagram.edges).toHaveLength(16)
    expect(diagram.omittedNodes).toBe(fixture.changed.length + fixture.related.length - 10)
    expect(diagram.omittedEdges).toBe(fixture.edges.length - 16)
    expect(diagram.code).toContain('%% +')
    expect(diagram.code).toContain('omitted')
    const touched = new Set(fixture.changed.map((item) => item.ref))
    for (const selected of diagram.nodes.filter((item) => !touched.has(item.ref))) {
      expect(diagram.edges.some((link) => link.source_ref === selected.ref || link.target_ref === selected.ref)).toBe(true)
    }
  })

  it('drops disconnected related nodes but keeps changed nodes', () => {
    const fixture = report({ changed: [node('formatter')], related: [node('plugin')] })
    const diagram = buildImpactDiagram(fixture)
    expect(diagram.nodes.some((item) => item.ref === 'formatter')).toBe(true)
    expect(diagram.code).not.toContain('plugin')
  })

  it('classifies dependency roles and lays out lanes over the bounded selection', () => {
    const changed = new Set(['core'])
    const edges = [edge('in', 'core'), edge('core', 'out'), edge('both', 'core'), edge('core', 'both')]
    expect(laneFor('core', changed, edges)).toBe('changed')
    expect(laneFor('in', changed, edges)).toBe('incoming')
    expect(laneFor('out', changed, edges)).toBe('outgoing')
    expect(laneFor('both', changed, edges)).toBe('both')
    const fixture = report({ changed: [node('core')], related: [node('in'), node('out'), node('both')], edges })
    const diagram = buildImpactDiagram(fixture)
    expect(diagram.code).toContain('flowchart LR')
    for (const lane of ['incoming', 'changed', 'outgoing', 'both']) expect(diagram.code).toContain(`subgraph lane_${lane}`)
  })

  it('handles an empty report and ignores dangling endpoints', () => {
    const fixture = report({ edges: [edge('missing', 'also-missing')] })
    const diagram = buildImpactDiagram(fixture)
    expect(diagram.nodes).toEqual([])
    expect(diagram.edges).toEqual([])
    expect(diagram.code.toLowerCase()).not.toContain('undefined')
  })
})
