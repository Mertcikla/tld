import { describe, expect, it, vi } from 'vitest'
import { createFlowSnapshotLink, decodeFlowSnapshot, encodeFlowSnapshot, exportFlowSnapshotYaml, loadFlowSnapshotDiagrams, parseFlowSnapshotYaml } from './sharing'
import { exportCompleteFlowYaml, FLOW_YAML_MAX_BYTES } from './yaml'
import type { FlowDiagram, StoryFlow } from './model'
import type { Connector } from '../types'

const flow: StoryFlow = {
  id: 'private-flow-id', viewId: 42, name: 'Checkout 世界', description: 'A frozen story', revision: 7,
  steps: [{ id: 'step-1', title: 'Store request', description: 'First step', elementIds: [81], connectorIds: [96], branches: {
    mode: 'alternate', paths: [
      { id: 'ok', name: 'Success', steps: [{ id: 'nested', title: '', description: 'Saved ✓', elementIds: [82], connectorIds: [] }] },
      { id: 'error', name: 'Failure', steps: [] },
    ],
  } }],
}
const diagram: FlowDiagram = {
  elements: [
    { element_id: 81, name: 'API', kind: 'service', description: 'Accept requests', technology: 'Go', position_x: 120, position_y: -30, tags: ['backend'] },
    { element_id: 82, name: 'DB', technology: 'Postgres', logo_url: '/icons/db.svg', position_x: 450, position_y: 10 },
    { element_id: 83, name: 'Unrelated private element' },
  ],
  connectors: [{ id: 96, view_id: 42, source_element_id: 81, target_element_id: 82, label: 'Stores', description: 'Writes rows', direction: 'forward', style: 'dashed' } as Connector],
}

describe('flow snapshots', () => {
  it('opens a standard bzip2 payload produced outside the app', async () => {
    // Python bz2.compress, using the default block size of 9.
    const payload = 'QlpoOTFBWSZTWaWQZLoAABxbgAAQQAIwEAgKPyffkCAAVFTT0Mpsk9Q0aHkICn6mkyAGE0RggSGAQMtXJVWlqcqdwRbmGeoukVs4QMz4q91s4p8PoMQKEmQtjWdHqdYSgSdxCdGO-6mMXyLuSKcKEhSyDJdA'
    expect((await decodeFlowSnapshot('#flow=v1.' + payload)).catalog.flows[0].name).toBe('Standard bzip2')
  })
  it('bzip2/base64url encodes the complete YAML and restores a standalone scene', async () => {
    const source = exportFlowSnapshotYaml(flow, diagram)
    expect(source).toBe(exportCompleteFlowYaml(flow, diagram))
    const hash = await encodeFlowSnapshot(source)
    expect(hash).toMatch(/^#flow=v1\.Qlpo[A-Za-z0-9_-]+$/) // Standard BZh magic, in base64.
    expect(hash.slice('#flow=v1.'.length)).not.toMatch(/[+/=]/)
    const snapshot = await decodeFlowSnapshot(hash)
    expect(snapshot.catalog.flows[0]).toMatchObject({ name: 'Checkout 世界', description: flow.description, viewId: 1, revision: 0 })
    expect(snapshot.catalog.flows[0].steps[0]).toMatchObject({ title: 'Store request', elementIds: [1], connectorIds: [1], branches: { paths: [{ name: 'Success', steps: [{ elementIds: [2], description: 'Saved ✓' }] }, { name: 'Failure' }] } })
    expect(snapshot.data.views[1].placements).toHaveLength(2)
    expect(snapshot.data.views[1].placements[0]).toMatchObject({ name: 'API', technology: 'Go', tags: ['backend'], position_x: 120, position_y: -30 })
    expect(snapshot.data.views[1].connectors[0]).toMatchObject({ source_element_id: 1, target_element_id: 2, label: 'Stores', style: 'dashed', description: 'Writes rows' })
    expect(source).not.toContain('private-flow-id')
    expect(source).not.toContain('Unrelated private element')
  })

  it('captures linked flows across diagrams, including cycles, using only snapshot identities', async () => {
    const root = { ...flow, steps: [{ ...flow.steps[0], branches: undefined, goToFlowId: 'linked' }] }
    const linked = { ...flow, id: 'linked', viewId: 55, name: 'Downstream', steps: [{ ...flow.steps[0], branches: undefined, connectorIds: [], goToFlowId: flow.id }] }
    const catalog = { flows: [root, linked], views: [{ id: 42, name: 'Checkout' }, { id: 55, name: 'Warehouse' }] }
    const source = exportFlowSnapshotYaml(root, diagram, catalog, { 55: diagram })
    expect(source).not.toContain('private-flow-id')
    const snapshot = await decodeFlowSnapshot(await encodeFlowSnapshot(source))
    expect(snapshot.catalog.flows).toHaveLength(2)
    const [first, second] = snapshot.catalog.flows
    expect(first.steps[0].goToFlowId).toBe(second.id)
    expect(second.steps[0].goToFlowId).toBe(first.id)
    expect(snapshot.catalog.views.map(view => view.name)).toEqual(['Checkout', 'Warehouse'])
    expect(snapshot.diagrams[second.viewId].elements.map(element => element.element_id)).toEqual([3])
  })

  it('builds links at the supplied app route and leaves snapshot data in the fragment', async () => {
    const url = new URL(await createFlowSnapshotLink('https://example.com/app/explore/shared/flow-snapshot?old=true#old', exportFlowSnapshotYaml(flow, diagram)))
    expect(url.pathname).toBe('/app/explore/shared/flow-snapshot')
    expect(url.search).toBe('')
    expect(url.hash).toMatch(/^#flow=v1\./)
    const desktopLink = new URL(await createFlowSnapshotLink('wails://localhost/explore/shared/flow-snapshot', exportFlowSnapshotYaml(flow, diagram)))
    expect(desktopLink.origin).toBe('https://tldiagram.com')
    expect(desktopLink.pathname).toBe('/app/explore/shared/flow-snapshot')
  })

  it('loads missing linked diagrams once when sharing from a single-view editor', async () => {
    const root = { ...flow, steps: [{ ...flow.steps[0], branches: undefined, goToFlowId: 'linked' }] }
    const linked = { ...flow, id: 'linked', viewId: 55, name: 'Downstream', steps: [] }
    const catalog = { flows: [root, linked], views: [{ id: 42, name: 'Checkout' }, { id: 55, name: 'Warehouse' }] }
    const load = vi.fn(async () => diagram)
    const diagrams = await loadFlowSnapshotDiagrams(root, diagram, catalog, undefined, load)
    expect(load).toHaveBeenCalledExactlyOnceWith(55)
    expect(parseFlowSnapshotYaml(exportFlowSnapshotYaml(root, diagram, catalog, diagrams)).catalog.flows).toHaveLength(2)
    load.mockClear()
    await loadFlowSnapshotDiagrams(root, diagram, catalog, diagrams, load)
    expect(load).not.toHaveBeenCalled()
  })

  it.each(['', '#flow=v2.abc', '#flow=v1.', '#flow=v1.a', '#flow=v1.@@@', '#flow=v1.QlpoAAAA'])('rejects missing, unsupported, or corrupt payloads: %s', async hash => {
    await expect(decodeFlowSnapshot(hash)).rejects.toThrow()
  })

  it('bounds input and decompressed YAML and rejects missing resources or references', async () => {
    // A 1 MiB + 1 byte run of "a", produced by Python bz2.compress.
    await expect(decodeFlowSnapshot('#flow=v1.QlpoOTFBWSZTWf5KqEgACApBAIACIAAACCAAMMwFSepxBgFAYB4u5IpwoSH8lVCQ')).rejects.toThrow('1 MiB')
    await expect(encodeFlowSnapshot('x'.repeat(FLOW_YAML_MAX_BYTES + 1))).rejects.toThrow('1 MiB')
    await expect(decodeFlowSnapshot('#flow=v1.' + 'a'.repeat(2 * FLOW_YAML_MAX_BYTES + 1))).rejects.toThrow('Invalid')
    expect(() => parseFlowSnapshotYaml('format: tld-flow\nversion: 1\nname: Missing\nsteps: []')).toThrow('missing')
    expect(() => parseFlowSnapshotYaml('format: tld-flow\nversion: 1\nname: Missing\nsteps: [{elements: [Unknown]}]\nresources: {elements: [], connectors: []}')).toThrow('not in the target diagram')
    expect(() => parseFlowSnapshotYaml('format: tld-flow-snapshot\nversion: 1\nflows: []')).toThrow('Unsupported')
  })

  it('keeps text-only and empty flows shareable', async () => {
    const empty = { ...flow, steps: [] }
    const snapshot = await decodeFlowSnapshot(await encodeFlowSnapshot(exportFlowSnapshotYaml(empty, diagram)))
    expect(snapshot.catalog.flows[0].steps).toEqual([])
    expect(snapshot.data.views[1].placements).toEqual([])
  })
})
