import { describe, expect, it } from 'vitest'
import type { PlanElement } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'
import type { LibraryElement } from '../types'
import {
  libraryElementToDependency,
  mapViewMarkdown,
  normalizeFrontendImportElements,
  protoElementToLibrary,
  protoPlacedElement,
} from './client'
import { normalizeConnectorRouteStyle, normalizeLogoUrl, normalizeTechnologyConnectors } from './client-normalize'

describe('normalizeConnectorRouteStyle', () => {
  it('keeps valid route styles', () => {
    expect(normalizeConnectorRouteStyle('bezier')).toBe('bezier')
    expect(normalizeConnectorRouteStyle('straight')).toBe('straight')
    expect(normalizeConnectorRouteStyle('step')).toBe('step')
    expect(normalizeConnectorRouteStyle('smoothstep')).toBe('smoothstep')
  })

  it('maps legacy line styles to bezier', () => {
    expect(normalizeConnectorRouteStyle('solid')).toBe('bezier')
    expect(normalizeConnectorRouteStyle('dashed')).toBe('bezier')
    expect(normalizeConnectorRouteStyle(undefined)).toBe('bezier')
  })
})

describe('element bypass noise gate normalization', () => {
  it('defaults API element and placement mappings to bypass_noise_gate false', () => {
    expect(protoElementToLibrary({ id: 1, name: 'API' }).bypass_noise_gate).toBe(false)
    expect(protoPlacedElement({ id: 1, viewId: 1, elementId: 1, name: 'API' }).bypass_noise_gate).toBe(false)
  })

  it('preserves bypass_noise_gate from proto casing variants and dependency mapping', () => {
    const library = protoElementToLibrary({ id: 1, name: 'API', bypassNoiseGate: true })
    expect(library.bypass_noise_gate).toBe(true)
    expect(protoPlacedElement({ id: 1, view_id: 1, element_id: 1, name: 'API', bypass_noise_gate: true }).bypass_noise_gate).toBe(true)

    const dependency = libraryElementToDependency({
      id: 1,
      name: 'API',
      kind: 'service',
      description: null,
      technology: null,
      url: null,
      logo_url: null,
      technology_connectors: [],
      tags: [],
      repo: null,
      branch: null,
      file_path: null,
      language: null,
      created_at: '2024-01-01',
      updated_at: '2024-01-01',
      has_view: false,
      view_label: null,
      bypass_noise_gate: true,
    } satisfies LibraryElement)
    expect(dependency.bypass_noise_gate).toBe(true)
  })

  it('maps proto-field technology_links onto API elements and placements', () => {
    const element = protoElementToLibrary({
      id: 1,
      name: 'Car',
      technology_links: [{ type: 'custom', label: 'fa:fa-car', is_primary_icon: true }],
    })
    const placement = protoPlacedElement({
      id: 2,
      view_id: 6,
      element_id: 1,
      name: 'Car',
      technology_links: [{ type: 'custom', label: 'fa:fa-car', is_primary_icon: true }],
    })

    expect(element.technology_connectors).toEqual([
      { type: 'custom', label: 'fa:fa-car', is_primary_icon: true },
    ])
    expect(placement.technology_connectors).toEqual([
      { type: 'custom', label: 'fa:fa-car', is_primary_icon: true },
    ])
  })

  it('defaults frontend import plan elements to bypass_noise_gate false', () => {
    const explicit = { ref: 'manual', name: 'Manual', bypassNoiseGate: true } as PlanElement
    const normalized = normalizeFrontendImportElements([
      { ref: 'api', name: 'API' } as PlanElement,
      explicit,
    ])

    expect((normalized[0] as Record<string, unknown>).bypassNoiseGate).toBe(false)
    expect(normalized[1]).toBe(explicit)
  })
})

describe('technology icon normalization', () => {
  it('derives a logo url from primary catalog technology links when logo_url is absent', () => {
    const links = normalizeTechnologyConnectors([
      { type: 'catalog', slug: 'go', label: 'Go', isPrimaryIcon: true },
    ])

    expect(normalizeLogoUrl(undefined, links)).toBe('/icons/go.svg')
  })

  it('normalizes explicit png catalog icon urls with alias remapping', () => {
    expect(normalizeLogoUrl('/icons/golang.png', [])).toBe('/icons/go.svg')
    expect(normalizeLogoUrl('/icons/javascript.png', [])).toBe('/icons/javascript.svg')
  })

  it('preserves explicit custom png icon urls', () => {
    expect(normalizeLogoUrl('/icons/acme-platform.png', [])).toBe('/icons/acme-platform.png')
  })

  it('preserves explicit no-icon logo clears', () => {
    const links = normalizeTechnologyConnectors([
      { type: 'catalog', slug: 'go', label: 'Go', isPrimaryIcon: true },
    ])

    expect(normalizeLogoUrl('', links)).toBe('')
  })
})

describe('markdown metadata mapping', () => {
  it('maps source, editability, git, and version metadata from proto casing variants', () => {
    const markdown = mapViewMarkdown({
      path: 'docs/diagrams/checkout.md',
      isManaged: true,
      updatedAt: '2026-06-12T00:00:00Z',
      sourceKind: 'REPO',
      exists: true,
      writable: false,
      canEdit: false,
      gitState: 'modified',
      repoRelativePath: 'docs/diagrams/checkout.md',
      linkedViewCount: 2,
      fileVersion: '123:45',
    })

    expect(markdown).toEqual({
      path: 'docs/diagrams/checkout.md',
      is_managed: true,
      updated_at: '2026-06-12T00:00:00Z',
      source_kind: 'REPO',
      exists: true,
      writable: false,
      can_edit: false,
      git_state: 'modified',
      repo_relative_path: 'docs/diagrams/checkout.md',
      linked_view_count: 2,
      file_version: '123:45',
    })
  })

  it('defaults legacy markdown metadata to editable unknown status', () => {
    expect(mapViewMarkdown({ path: 'view-markdown/view-1-system.md' })).toMatchObject({
      source_kind: '',
      exists: true,
      writable: true,
      can_edit: true,
      git_state: 'unknown',
      linked_view_count: 0,
      file_version: '',
    })
  })

  it('maps omitted modern availability booleans as false for missing files', () => {
    expect(mapViewMarkdown({
      path: 'docs/missing.md',
      sourceKind: 'ATTACHED',
      gitState: 'deleted',
      repoRelativePath: 'docs/missing.md',
    })).toMatchObject({
      source_kind: 'ATTACHED',
      exists: false,
      writable: false,
      can_edit: false,
      git_state: 'deleted',
      repo_relative_path: 'docs/missing.md',
    })
  })
})
