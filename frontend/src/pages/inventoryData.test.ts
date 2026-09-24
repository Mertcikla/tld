import { describe, expect, it } from 'vitest'
import type { Connector, LibraryElement, ViewTreeNode } from '../types'
import { buildInventoryRows, dependencyConnectorToConnector, filterInventoryRows, flattenInventoryViews } from './inventoryData'

const element = (id: number, name: string, tags: string[] = [], overrides: Partial<LibraryElement> = {}): LibraryElement => ({
  id,
  name,
  kind: 'service',
  description: null,
  technology: 'Go',
  url: null,
  logo_url: null,
  technology_connectors: [],
  tags,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
  has_view: id === 1,
  view_label: id === 1 ? 'Container' : null,
  ...overrides,
})

const view = (id: number, name: string, tags: string[] = [], overrides: Partial<ViewTreeNode> = {}): ViewTreeNode => ({
  id,
  owner_element_id: null,
  name,
  description: null,
  level_label: 'Context',
  tags,
  level: 0,
  depth: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-03T00:00:00Z',
  parent_view_id: null,
  children: [],
  ...overrides,
})

const connector = (id: number, tags: string[] = [], overrides: Partial<Connector> = {}): Connector => ({
  id,
  view_id: 10,
  source_element_id: 1,
  target_element_id: 2,
  label: 'writes',
  description: null,
  relationship: 'SQL',
  direction: 'forward',
  style: 'bezier',
  url: null,
  source_handle: null,
  target_handle: null,
  tags,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-04T00:00:00Z',
  ...overrides,
})

const keys = (rows: ReturnType<typeof filterInventoryRows>) => rows.map((row) => row.key).sort()

describe('inventoryData', () => {
  it('keeps internal group markers out of inventory tags and search text', () => {
    const rows = buildInventoryRows(
      [element(1, 'API', ['payments', 'group:12345678-1234-4234-a234-123456789012'])],
      [],
      [],
      {},
    )
    const row = rows[0]

    expect(row.tags).toEqual(['payments'])
    expect(row.searchableText).not.toContain('group:')
    expect(filterInventoryRows(rows, { type: 'all', query: '12345678', tags: [], kind: '', qualities: [] })).toEqual([])
  })

  it('builds rows with relationship labels and quality flags', () => {
    const rows = buildInventoryRows(
      [element(1, 'API', ['payments']), element(2, 'Database')],
      [view(10, 'System Context')],
      [connector(20, ['critical'])],
      { 10: { placements: 2, connectors: 1 } },
    )

    expect(rows.map((row) => row.key)).toContain('element:1')
    expect(rows.find((row) => row.key === 'connector:20')).toMatchObject({
      name: 'writes',
      subtitle: 'API -> Database',
      viewName: 'System Context',
      tags: ['critical'],
    })
    expect(rows.find((row) => row.key === 'view:10')?.qualityFlags).toContain('untagged')
  })

  it('filters by object type, search, tag, kind, and quality', () => {
    const rows = buildInventoryRows(
      [element(1, 'Payments API', ['payments']), element(2, 'Ledger DB')],
      [view(10, 'Payments View', ['payments'])],
      [connector(20)],
      { 10: { placements: 0, connectors: 0 } },
    )

    expect(filterInventoryRows(rows, { type: 'elements', query: 'payments', tags: [], kind: '', qualities: [] }).map((row) => row.key)).toEqual(['element:1'])
    expect(filterInventoryRows(rows, { type: 'all', query: '', tags: ['payments'], kind: '', qualities: [] }).map((row) => row.key).sort()).toEqual(['element:1', 'view:10'])
    expect(filterInventoryRows(rows, { type: 'views', query: '', tags: [], kind: 'Context', qualities: ['empty view'] }).map((row) => row.key)).toEqual(['view:10'])
  })

  it('searches trimmed text case-insensitively across inventory metadata', () => {
    const rows = buildInventoryRows(
      [
        element(1, 'Payments API', ['critical'], {
          description: 'Handles checkout flows',
          kind: 'backend',
          technology: 'Go',
          url: 'https://payments.example.com',
          repo: 'payments-service',
          branch: 'main',
          file_path: 'internal/payments/api.go',
        }),
        element(2, 'Ledger DB', ['storage'], { kind: 'database', technology: 'Postgres' }),
      ],
      [view(10, 'Revenue Context', ['finance'], { description: 'Money movement view', level_label: 'System' })],
      [connector(20, ['edge'], {
        label: 'writes ledger events',
        description: 'Persists settlement records',
        relationship: 'SQL',
        direction: 'forward',
        style: 'step',
      })],
      { 10: { placements: 2, connectors: 1 } },
    )

    expect(keys(filterInventoryRows(rows, { type: 'all', query: '  CHECKOUT  ', tags: [], kind: '', qualities: [] }))).toEqual(['element:1'])
    expect(keys(filterInventoryRows(rows, { type: 'all', query: 'payments-service', tags: [], kind: '', qualities: [] }))).toEqual(['element:1'])
    expect(keys(filterInventoryRows(rows, { type: 'all', query: 'finance', tags: [], kind: '', qualities: [] }))).toEqual(['view:10'])
    expect(keys(filterInventoryRows(rows, { type: 'all', query: 'settlement', tags: [], kind: '', qualities: [] }))).toEqual(['connector:20'])
    expect(keys(filterInventoryRows(rows, { type: 'all', query: 'ledger db', tags: [], kind: '', qualities: [] }))).toEqual(['connector:20', 'element:2'])
    expect(keys(filterInventoryRows(rows, { type: 'all', query: 'revenue context', tags: [], kind: '', qualities: [] }))).toEqual(['connector:20', 'view:10'])
    expect(keys(filterInventoryRows(rows, { type: 'all', query: 'step', tags: [], kind: '', qualities: [] }))).toEqual(['connector:20'])
  })

  it('ANDs filter groups while OR-ing multiple tags and qualities within each group', () => {
    const rows = buildInventoryRows(
      [
        element(1, 'Payments API', ['payments'], { description: 'Documented API', has_view: true }),
        element(2, 'Billing Worker', ['billing'], { description: null, has_view: false }),
        element(3, 'Unused Search Index', ['search'], { kind: 'database', description: null, has_view: false }),
      ],
      [view(10, 'Payments View', ['payments'])],
      [connector(20, [], { label: null, description: null })],
      { 10: { placements: 0, connectors: 0 } },
    )

    expect(keys(filterInventoryRows(rows, {
      type: 'elements',
      query: '',
      tags: ['payments', 'billing'],
      kind: 'service',
      qualities: ['has child view', 'missing description'],
    }))).toEqual(['element:1', 'element:2'])

    expect(keys(filterInventoryRows(rows, {
      type: 'elements',
      query: 'payments',
      tags: ['billing'],
      kind: 'service',
      qualities: ['missing description'],
    }))).toEqual([])
  })

  it('assigns quality flags for all inventory object types', () => {
    const rows = buildInventoryRows(
      [
        element(1, 'Parent API', [], { description: null, has_view: true }),
        element(2, 'Unused Cache', ['infra'], { description: null, has_view: false }),
      ],
      [view(10, 'Empty View', [], { description: null })],
      [connector(20, [], { label: null, description: null })],
      { 10: { placements: 0, connectors: 0 } },
    )

    expect(rows.find((row) => row.key === 'element:1')?.qualityFlags).toEqual(expect.arrayContaining(['untagged', 'missing description', 'has child view']))
    expect(rows.find((row) => row.key === 'element:2')?.qualityFlags).toEqual(expect.arrayContaining(['missing description']))
    expect(rows.find((row) => row.key === 'view:10')?.qualityFlags).toEqual(expect.arrayContaining(['untagged', 'missing description', 'empty view']))
    expect(rows.find((row) => row.key === 'connector:20')?.qualityFlags).toEqual(expect.arrayContaining(['untagged', 'missing description', 'missing label']))
  })

  it('flattens nested views and builds connector fallback labels for missing references', () => {
    const nestedViews = [
      view(1, 'Root', [], {
        children: [
          view(2, 'Child', [], {
            parent_view_id: 1,
            children: [view(3, 'Grandchild', [], { parent_view_id: 2 })],
          }),
        ],
      }),
    ]

    expect(flattenInventoryViews(nestedViews).map((item) => item.name)).toEqual(['Root', 'Child', 'Grandchild'])

    const rows = buildInventoryRows(
      [element(1, 'Known Source')],
      [view(10, 'Known View')],
      [connector(20, [], { label: null, source_element_id: 1, target_element_id: 999, view_id: 999 })],
      {},
    )

    expect(rows.find((row) => row.key === 'connector:20')).toMatchObject({
      name: 'Known Source -> Element 999',
      subtitle: 'Known Source -> Element 999',
      viewName: 'View 999',
      usageLabel: 'View 999',
    })
  })

  it('converts dependency connectors into editable connectors with tags', () => {
    expect(dependencyConnectorToConnector({
      id: '5',
      view_id: '7',
      source_element_id: '1',
      target_element_id: '2',
      label: null,
      description: null,
      relationship_type: 'HTTP',
      direction: 'forward',
      connector_type: 'step',
      url: null,
      source_handle: null,
      target_handle: null,
      tags: ['edge'],
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-02T00:00:00Z',
    })).toMatchObject({ id: 5, view_id: 7, relationship: 'HTTP', style: 'step', tags: ['edge'] })
  })
})
