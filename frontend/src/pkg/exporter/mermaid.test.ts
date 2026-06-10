import { describe, expect, it } from 'vitest'
import { parseMermaid } from '../importer/mermaid'
import { MermaidExporter, serializeViewToMermaid, type MermaidWorkspaceView } from './mermaid'
import type { Connector, PlacedElement } from '../../types'

describe('MermaidExporter', () => {
  it('serializes a workspace view to round-trippable flowchart Mermaid', () => {
    const view = {
      placements: [
        { element_id: 2, name: 'Worker "A"', view_id: 1, id: 1, position_x: 0, position_y: 0, kind: 'service', description: null, technology: null, url: null, logo_url: null, technology_connectors: [], tags: [], has_view: false, view_label: null },
        { element_id: 3, name: 'Database', view_id: 1, id: 2, position_x: 0, position_y: 0, kind: 'database', description: null, technology: null, url: null, logo_url: null, technology_connectors: [], tags: [], has_view: false, view_label: null },
      ] satisfies PlacedElement[],
      connectors: [
        { id: 9, view_id: 1, source_element_id: 2, target_element_id: 3, label: 'writes to', description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: null, target_handle: null, tags: [], created_at: '', updated_at: '' },
      ] satisfies Connector[],
    } satisfies MermaidWorkspaceView

    const code = new MermaidExporter(view).toMermaid()
    expect(serializeViewToMermaid(view.placements, view.connectors)).toBe(code)
    expect(code).toContain('flowchart LR')
    expect(code).toContain('node_2["Worker \\"A\\""]')
    expect(code).toContain('node_2 -- "writes to" --> node_3')
    expect(code.match(/%% tld\/v1/g)).toHaveLength(1)
    expect(code).toContain('%% element node_2 x=0 y=0 kind=service')
    expect(code).toContain('%% connector node_2->node_3')
    expect(code).not.toContain('label=')
    expect(code).not.toContain('dir=forward')
    expect(code).not.toContain('style=bezier')

    const parsed = parseMermaid(code)
    expect(parsed.warnings).toHaveLength(0)
    expect(parsed.elements).toHaveLength(2)
    expect(parsed.connectors).toHaveLength(1)
    expect(parsed.elements.find((element) => element.ref === 'node_2')?.placements[0]?.positionX).toBe(0)
  })

  it('places compact metadata comments immediately after their Mermaid resources', () => {
    const view = {
      placements: [
        { element_id: 2, name: 'Worker', view_id: 1, id: 1, position_x: 10, position_y: 20, kind: 'service', description: null, technology: null, url: null, logo_url: null, technology_connectors: [], tags: [], has_view: false, view_label: null },
        { element_id: 3, name: 'Database', view_id: 1, id: 2, position_x: 30, position_y: 40, kind: 'database', description: null, technology: null, url: null, logo_url: null, technology_connectors: [], tags: [], has_view: false, view_label: null },
      ],
      connectors: [
        { id: 9, view_id: 1, source_element_id: 2, target_element_id: 3, label: 'writes to', description: null, relationship: 'stores', direction: 'both', style: 'straight', url: null, source_handle: 'top', target_handle: 'bottom', tags: [], created_at: '', updated_at: '' },
      ],
    } satisfies MermaidWorkspaceView

    const lines = new MermaidExporter(view).toMermaid().trim().split('\n')

    expect(lines[0]).toBe('flowchart LR')
    expect(lines[1]).toBe('%% tld/v1')
    expect(lines.filter((line) => line.includes('tld/v1'))).toHaveLength(1)
    expect(lines[2]).toBe('  node_2["Worker"]')
    expect(lines[3]).toBe('%% element node_2 x=10 y=20 kind=service')
    expect(lines[4]).toBe('  node_3["Database"]')
    expect(lines[5]).toBe('%% element node_3 x=30 y=40 kind=database')
    expect(lines[7]).toBe('  node_2 -- "writes to" --> node_3')
    expect(lines[8]).toBe('%% connector node_2->node_3 rel=stores dir=both style=straight sourceHandle=top targetHandle=bottom')
  })

  it('round-trips escaped TLD metadata without storing names or edge labels in comments', () => {
    const description = 'Line 1\nLine 2, equals= pipe| colon: slash\\ space'
    const view = {
      placements: [
        {
          element_id: 2,
          name: 'Worker',
          view_id: 1,
          id: 1,
          position_x: 12.5,
          position_y: -8,
          kind: 'service',
          description,
          technology: 'Node.js',
          url: 'https://example.com/a?b=1',
          logo_url: '/icons/node.svg',
          technology_connectors: [
            { type: 'catalog', slug: 'nodejs', label: 'Node.js', is_primary_icon: true },
            { type: 'custom', label: 'Queue: A|B' },
          ],
          tags: ['alpha beta', 'x=y', 'comma,item', 'pipe|tag', 'colon:tag', 'slash\\tag'],
          repo: 'repo with space',
          branch: 'feature/a=b',
          file_path: 'src/a:b|c.ts',
          language: 'TypeScript',
          bypass_noise_gate: true,
          has_view: true,
          view_label: 'Service View',
        },
        { element_id: 3, name: 'Database', view_id: 1, id: 2, position_x: 100, position_y: 200, kind: 'database', description: null, technology: null, url: null, logo_url: null, technology_connectors: [], tags: [], has_view: false, view_label: null },
      ],
      connectors: [
        {
          id: 9,
          view_id: 1,
          source_element_id: 2,
          target_element_id: 3,
          label: 'writes to',
          description,
          relationship: 'runtime dependency',
          direction: 'both',
          style: 'smoothstep',
          url: 'https://edge.example.com/a=b',
          source_handle: 'top',
          target_handle: 'bottom',
          tags: ['not-exported'],
          created_at: '',
          updated_at: '',
        },
      ],
    } satisfies MermaidWorkspaceView

    const code = new MermaidExporter(view).toMermaid()
    const metadataLines = code.split('\n').filter((line) => line.startsWith('%% element') || line.startsWith('%% connector'))
    expect(metadataLines.join('\n')).not.toContain('name=')
    expect(metadataLines.join('\n')).not.toContain('label=')

    const parsed = parseMermaid(code)
    const worker = parsed.elements.find((element) => element.ref === 'node_2')
    const connector = parsed.connectors[0]

    expect(parsed.warnings).toHaveLength(0)
    expect(worker).toMatchObject({
      kind: 'service',
      description,
      technology: 'Node.js',
      url: 'https://example.com/a?b=1',
      logoUrl: '/icons/node.svg',
      tags: ['alpha beta', 'x=y', 'comma,item', 'pipe|tag', 'colon:tag', 'slash\\tag'],
      repo: 'repo with space',
      branch: 'feature/a=b',
      filePath: 'src/a:b|c.ts',
      language: 'TypeScript',
      bypassNoiseGate: true,
      hasView: true,
      viewLabel: 'Service View',
    })
    expect(worker?.placements[0]).toMatchObject({ parentRef: 'root', positionX: 12.5, positionY: -8 })
    expect(worker?.technologyLinks).toEqual([
      { type: 'catalog', slug: 'nodejs', label: 'Node.js', isPrimaryIcon: true },
      { type: 'custom', slug: undefined, label: 'Queue: A|B', isPrimaryIcon: false },
    ])
    expect(connector).toMatchObject({
      description,
      relationship: 'runtime dependency',
      direction: 'both',
      style: 'smoothstep',
      url: 'https://edge.example.com/a=b',
      sourceHandle: 'top',
      targetHandle: 'bottom',
    })
  })

  it('preserves metadata order for duplicate same-endpoint connectors', () => {
    const view = {
      placements: [
        { element_id: 2, name: 'Worker', view_id: 1, id: 1, position_x: 0, position_y: 0, kind: 'service', description: null, technology: null, url: null, logo_url: null, technology_connectors: [], tags: [], has_view: false, view_label: null },
        { element_id: 3, name: 'Database', view_id: 1, id: 2, position_x: 200, position_y: 0, kind: 'database', description: null, technology: null, url: null, logo_url: null, technology_connectors: [], tags: [], has_view: false, view_label: null },
      ],
      connectors: [
        { id: 8, view_id: 1, source_element_id: 2, target_element_id: 3, label: 'default edge', description: null, relationship: null, direction: 'forward', style: 'bezier', url: null, source_handle: null, target_handle: null, tags: [], created_at: '', updated_at: '' },
        { id: 9, view_id: 1, source_element_id: 2, target_element_id: 3, label: 'rich edge', description: null, relationship: 'writes', direction: 'both', style: 'straight', url: null, source_handle: null, target_handle: null, tags: [], created_at: '', updated_at: '' },
      ],
    } satisfies MermaidWorkspaceView

    const parsed = parseMermaid(new MermaidExporter(view).toMermaid())

    expect(parsed.warnings).toHaveLength(0)
    expect(parsed.connectors).toHaveLength(2)
    expect(parsed.connectors[0]?.label).toBe('default edge')
    expect(parsed.connectors[0]?.relationship).toBeUndefined()
    expect(parsed.connectors[1]?.label).toBe('rich edge')
    expect(parsed.connectors[1]?.relationship).toBe('writes')
    expect(parsed.connectors[1]?.direction).toBe('both')
    expect(parsed.connectors[1]?.style).toBe('straight')
  })
})
