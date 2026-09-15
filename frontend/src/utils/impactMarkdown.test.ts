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

    expect(code).toContain('flowchart TD')
    expect(code).toContain('%% coverage: 100% (high)')
    expect(code).toContain('n1["Core"]')
    expect(code).toContain('n2["API"]')
    expect(code).toContain('n1 -->|calls| n2')
    expect(code).toContain('n2 -.-> n1')
    expect(code).toContain('class n1 changed')
    expect(code).toContain('classDef changed fill:#fde68a')
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
    expect(md).toContain('**Coverage:** 100% (high) — 2/2 source files owned; analysis is complete')
    expect(md).not.toContain('**Changed**')
    expect(md).not.toContain('**Related**')
    expect(md).not.toContain('**Unmapped**')
    expect(md).toContain('_Dashed edges are observed in code but not declared in the architecture._')
    expect(md).toContain('```mermaid')
    expect(md).toContain('flowchart TD')
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
