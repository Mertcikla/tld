import { describe, expect, it } from 'vitest'
import type { PlacedElement, ViewTreeNode } from '../types'
import type { Connector } from '../types'
import { sceneMermaid, type SceneMermaidView } from './sceneMermaid'

const view = (id: number, name = 'Mine', children: ViewTreeNode[] = []): ViewTreeNode => ({ id, name, description: null, level_label: null, level: 0, depth: 0, created_at: '', updated_at: '', parent_view_id: null, children })
const placement = (elementId: number, name: string): PlacedElement => ({
  id: elementId, element_id: elementId, view_id: 51, position_x: 0, position_y: 0, name,
  description: null, kind: 'component', technology: null, url: null, logo_url: null,
  technology_connectors: [], tags: [], repo: null, repository_id: null, branch: null,
  file_path: null, language: null, bypass_noise_gate: false, has_view: false, view_label: null,
})
const connector = (id: number, source: number, target: number, label: string | null, tags: string[] = []): Connector => ({
  id, view_id: 51, source_element_id: source, target_element_id: target, label,
  description: null, relationship: null, direction: 'forward', style: 'bezier', url: null,
  source_handle: null, target_handle: null, tags, created_at: '', updated_at: '',
})

// Mirrors internal/mermaid/scene_test.go sceneFixture so both renderers stay
// in lockstep: same views, nodes, badges, arrows, and link styles.
function fixture(): SceneMermaidView {
  return {
    data: {
      tree: [view(51), view(-1, 'Changes')],
      views: {
        51: {
          placements: [placement(7, 'svc/auth'), placement(8, 'svc/db')],
          connectors: [connector(1, 7, 8, 'calls', ['change:added'])],
        },
        '-1': {
          placements: [{ ...placement(-1, 'new.go'), view_id: -1 }],
          connectors: [],
        },
      },
      navigations: [],
    },
    overlays: {
      7: { change: 'modified', linesAdded: 3, linesRemoved: 1 },
      [-1]: { change: 'added', linesAdded: 1, linesRemoved: 0 },
    },
    provenance: { 7: 'authored', 8: 'authored', [-1]: 'augmented' },
  }
}

const options = { repositoryId: 'r1', comparisonKey: 'k1', scope: 'grounded', radius: 0 }

describe('sceneMermaid', () => {
  it('renders the filtered canvas data node for node', () => {
    const code = sceneMermaid(fixture(), options)
    for (const want of [
      'flowchart LR',
      '%% tld-scene repo=r1 key=k1 scope=grounded radius=0',
      'subgraph view_51["Mine"]',
      'el_51_7["svc/auth<br/>modified +3 −1"]',
      'el_51_8["svc/db"]',
      'el_51_7 ==>|"calls"| el_51_8',
      'linkStyle 0 stroke:#48bb78',
      'subgraph view_neg1["Changes"]',
      'el_neg1_neg1["◇ new.go<br/>added +1 −0"]',
    ]) {
      expect(code).toContain(want)
    }
  })

  it('marks removed connectors with crossed arrows and dashed link styles', () => {
    const view = fixture()
    view.data.views[51].connectors = [connector(1, 7, 8, 'calls', ['change:removed'])]
    const code = sceneMermaid(view, options)
    expect(code).toContain('el_51_7--x|"calls"|el_51_8')
    expect(code).toContain('linkStyle 0 stroke:#fc8181,stroke-dasharray:5 5')
  })

  it('drops endpoints pruned from the view instead of dangling', () => {
    const view = fixture()
    view.data.views[51].placements = [placement(7, 'svc/auth')]
    const code = sceneMermaid(view, options)
    expect(code).not.toContain('el_51_7 ==>')
    expect(code).not.toContain('linkStyle')
  })
})
