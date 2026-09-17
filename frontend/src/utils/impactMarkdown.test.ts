import { describe, expect, it } from 'vitest'
import type { ImpactReport } from '../api/client'
import { impactMarkdown, impactMermaid } from './impactMarkdown'

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

describe('impactMermaid', () => {
  it('marks changed nodes and distinguishes declared from observed edges', () => {
    const code = impactMermaid(
      report({
        changed: [{ ref: '1', name: 'Core', kind: 'component', change: 'modified', evidence: ['src/a.go'] }],
        related: [{ ref: '2', name: 'API', kind: 'component', change: 'modified', evidence: [] }],
        edges: [
          { source_ref: '1', target_ref: '2', label: 'calls', observed: false },
          { source_ref: '2', target_ref: '1', label: '', observed: true },
        ],
      }),
    )

    expect(code).toContain('flowchart LR')
    expect(code).toContain('%% coverage: 100% (high)')
    expect(code).toContain('subgraph lane_changed')
    expect(code).toContain('n1["Core<br/>1 file"]')
    expect(code).toContain('n2["API"]')
    expect(code).toContain('n1 -->|calls| n2')
    expect(code).toContain('n2 -.->|observed| n1')
    expect(code).toContain('class n1 changed')
    expect(code).toContain('classDef changed fill:#fde68a')
  })

  it('badges changed nodes with aggregate change instead of file paths', () => {
    const code = impactMermaid(
      report({
        changed: [
          { ref: '1', name: 'Core', kind: 'component', change: 'modified', evidence: ['internal/core.go', 'internal/util.go'] },
        ],
        related: [{ ref: '2', name: 'API', kind: 'component', change: 'modified', evidence: [] }],
        edges: [{ source_ref: '2', target_ref: '1', label: '', observed: true }],
        changed_files: [
          { path: 'internal/core.go', change: 'modified', added: 12, removed: 3 },
          { path: 'internal/util.go', change: 'modified', added: 0, removed: 0 },
        ],
      }),
    )

    expect(code).toContain('n1["Core<br/>2 files (+12 -3)"]')
    expect(code).toContain('n2["API"]')
    expect(code).toContain('n2 -.->|observed| n1')
    expect(code).not.toContain('internal/core.go')
  })

  it('renders folder containment with a distinct arrow', () => {
    const code = impactMermaid(
      report({
        changed: [
          { ref: '1', name: 'Backend', kind: 'component', change: 'modified', evidence: [] },
          { ref: '2', name: 'Checkout', kind: 'component', change: 'modified', evidence: [] },
        ],
        edges: [{ source_ref: '1', target_ref: '2', label: 'contains', observed: false }],
      }),
    )

    expect(code).toContain('n1 --o|contains| n2')
  })
})

describe('impactMarkdown', () => {
  it('renders the PR-comment sections with a mermaid block', () => {
    const md = impactMarkdown(
      report({
        changed: [{ ref: '1', name: 'Core', kind: 'component', change: 'modified', evidence: ['src/a.go'] }],
        unmapped: ['src/loose.ts'],
        edges: [{ source_ref: '1', target_ref: '2', label: '', observed: true }],
      }),
    )

    expect(md).toContain('## Architecture Impact')
    expect(md).toContain('**Coverage:** 100% (high) — 2/2 source files covered; all changed source files are covered')
    expect(md).not.toContain('**Changed**')
    expect(md).not.toContain('**Related**')
    expect(md).not.toContain('**Unmapped**')
    expect(md).toContain('_Dashed edges are observed in code but not declared in the architecture._')
    expect(md).not.toContain('list the source files that touched them')
    expect(md).toContain('```mermaid')
    expect(md).toContain('flowchart LR')
  })

  it('notes trimmed context and containment edges', () => {
    const related = Array.from({ length: 20 }, (_, index) => ({
      ref: `r${index + 1}`,
      name: `R${index + 1}`,
      kind: 'component',
      change: 'modified' as const,
      evidence: [] as string[],
    }))
    const md = impactMarkdown(
      report({
        changed: [{ ref: 'a', name: 'A', kind: 'component', change: 'modified', evidence: [] }],
        related,
        edges: [
          ...related.map((node) => ({ source_ref: 'a', target_ref: node.ref, label: 'calls', observed: false })),
          { source_ref: 'a', target_ref: 'child', label: 'contains', observed: false },
        ],
      }),
    )

    expect(md).toContain('_Graph trimmed for readability:')
    expect(md).toContain('_–o edges show folder containment between bound elements._')
  })

  it('renders binding gap suggestions as tld commands', () => {
    const md = impactMarkdown(
      report({
        unmapped: ['internal/risk.go'],
        coverage: {
          ...report().coverage,
          complete: false,
          percent: 50,
          bound_source_files: 1,
          gaps: [
            {
              file: 'internal/risk.go',
              change: 'added',
              reason: 'no element owns this file',
              suggested_element_ref: '42',
              suggested_element_name: 'Risk',
              suggested_score: 0.87,
              suggested_element_pattern: '',
              suggested_new_element: 'Risk Engine',
              suggested_new_ref: '',
            },
          ],
        },
      }),
    )

    expect(md).toContain('**Binding gaps**')
    expect(md).toContain('- `internal/risk.go` (added) — no element owns this file')
    expect(md).toContain('suggested owner: Risk (0.87) — `tld bind 42 --file "internal/risk.go"`')
    expect(md).toContain('or create an element: `tld add "Risk Engine" --file "internal/risk.go"`')
  })

  it('short-circuits element sections when nothing architecture-bound changed', () => {
    const md = impactMarkdown(report())
    expect(md).toContain('No architecture-bound code changed.')
    expect(md).toContain('```mermaid')
  })
})
