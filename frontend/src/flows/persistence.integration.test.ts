import { env } from 'node:process'
import { createClient, Code } from '@connectrpc/connect'
import { WorkspaceService } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/workspace_service_pb'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { stringify } from 'yaml'
import { transport } from '../api/transport'
import { flowApi } from './api'
import { allSteps, newStep, type FlowDiagram, type StoryFlow } from './model'
import { exportFlowYaml, importFlowYaml, parseFlowYaml } from './yaml'
import type { Connector } from '../types'

// Opt in against a disposable local server, for example:
// TLD_FLOW_TEST_API_URL=http://127.0.0.1:18060/api npx vitest run src/flows/persistence.integration.test.ts
vi.mock('../api/transport', async () => {
  const { env } = await import('node:process')
  const { createConnectTransport } = await import('@connectrpc/connect-web')
  return { transport: createConnectTransport({ baseUrl: env.TLD_FLOW_TEST_API_URL ?? 'http://127.0.0.1:1/api' }) }
})

const workspace = createClient(WorkspaceService, transport)
const localOrgId = '11111111-1111-1111-1111-111111111111'
const diagram = (offset = 0): FlowDiagram => ({
  elements: ['API', 'Database', 'Worker'].map((name, index) => ({ element_id: offset + index + 1, name })),
  connectors: [{ id: offset + 10, source_element_id: offset + 1, target_element_id: offset + 2, label: 'Reads', direction: 'both' } as Connector],
})
const source = `format: tld-flow
version: 1
name: Checkout → 日本語
description: |-
  A story with branches.
  Preserve this second line.
steps:
  - id: intro
    title: "null"
  - id: mixed
    description: Submit and store
    elements: [API, Database]
    connectors: [{source: API, target: Database, label: Reads}]
  - id: decision
    branches:
      mode: alternate
      paths:
        - id: empty
          name: No work
          steps: []
        - id: accepted
          name: Accepted
          steps:
            - id: concurrent
              branches:
                mode: parallel
                paths:
                  - id: worker
                    name: Worker
                    steps: [{id: task, elements: [Worker]}]
                  - id: storage
                    name: Storage
                    steps: [{id: read, connectors: [{source: API, target: Database, label: Reads}]}]
  - id: finish
    description: Done
`

describe.skipIf(!env.TLD_FLOW_TEST_API_URL)('flow YAML and real API/database integration', () => {
  const views: { id: number; name: string; owner: number }[] = []
  beforeAll(async () => {
    for (const name of [`Flow integration A ${crypto.randomUUID()}`, `Flow integration B ${crypto.randomUUID()}`]) {
      const owner = (await workspace.createElement({ name })).element
      if (!owner) throw new Error('Missing owner element')
      const view = (await workspace.createView({ orgId: localOrgId, name, ownerElementId: owner.id })).view
      if (!view) throw new Error('Missing view')
      views.push({ id: view.id, name: view.name, owner: owner.id })
    }
  })
  afterEach(async () => {
    for (const flow of await flowApi.list()) if (views.some(view => view.id === flow.viewId)) await flowApi.remove(flow)
  })
  afterAll(async () => {
    for (const view of views) {
      await workspace.deleteView({ orgId: localOrgId, viewId: view.id })
      await workspace.deleteElement({ orgId: localOrgId, elementId: view.owner })
    }
  })
  const load = async (flow: StoryFlow) => {
    const loaded = (await flowApi.list(flow.viewId)).find(candidate => candidate.id === flow.id)
    if (!loaded) throw new Error('Saved flow was not listed')
    return loaded
  }

  it('round-trips nested YAML, ordering, empty paths and highlights through saved rows', async () => {
    const draft = importFlowYaml(source, views[0].id, diagram())
    const saved = await flowApi.save(draft)
    expect(saved).toEqual({ ...draft, id: saved.id, revision: 1 })
    const loaded = await load(saved)
    expect(loaded).toEqual(saved)
    expect(exportFlowYaml(loaded, diagram())).toBe(exportFlowYaml(draft, diagram()))
    expect(allSteps(loaded.steps)).toHaveLength(7)
    expect(loaded.steps[2].branches?.paths[0].steps).toEqual([])
    const changed = { ...loaded, name: 'Updated story', steps: [...loaded.steps].reverse() }
    changed.steps[1] = { ...changed.steps[1], branches: { ...changed.steps[1].branches!, mode: 'parallel', paths: [...changed.steps[1].branches!.paths].reverse() } }
    changed.steps.push({ ...newStep(diagram()), id: 'added', elementIds: [1, 3], connectorIds: [10], description: 'New root' })
    const updated = await flowApi.save(changed)
    expect(await load(updated)).toEqual({ ...changed, revision: 2 })
    const exported = exportFlowYaml(await load(updated), diagram())
    const rebound = importFlowYaml(exported, views[1].id, diagram(1000))
    const second = await flowApi.save(rebound)
    expect(exportFlowYaml(await load(second), diagram(1000))).toBe(exported)
    expect(second.steps.at(-1)).toMatchObject({ elementIds: [1001, 1003], connectorIds: [1010] })
    expect(parseFlowYaml(exported)).not.toHaveProperty('revision')
  })

  it('preserves cross-view flow links and soft references after deleting their targets', async () => {
    const child = await flowApi.save({ id: '', viewId: views[1].id, revision: 0, name: 'Child story', description: '', steps: [] })
    const doc = parseFlowYaml(source)
    doc.steps.push({ id: 'call', goToFlow: { name: child.name, view: views[1].name } })
    const catalog = { flows: [child], views }
    const caller = await flowApi.save(importFlowYaml(stringify(doc), views[0].id, diagram(), {}, catalog))
    expect(exportFlowYaml(await load(caller), diagram(), catalog)).toContain('Child story')
    await flowApi.remove(child)
    const missing = { ...await load(caller), name: 'Edit after deletion' }
    missing.steps.push({ ...newStep(diagram()), id: 'missing', elementIds: [987654], connectorIds: [876543], goToFlowId: 'never-existed' })
    const updated = await flowApi.save(missing)
    expect(await load(updated)).toEqual(updated)
    expect(updated.steps.at(-2)?.goToFlowId).toBe(child.id)
    expect(updated.steps.at(-1)).toMatchObject({ elementIds: [987654], connectorIds: [876543], goToFlowId: 'never-existed' })
  })

  it('rejects stale mutations, supports clearing every step, and deletes cleanly', async () => {
    const original = await flowApi.save(importFlowYaml(source, views[0].id, diagram()))
    const cleared = await flowApi.save({ ...original, steps: [] })
    expect((await load(cleared)).steps).toEqual([])
    await expect(flowApi.save({ ...original, name: 'Stale' })).rejects.toMatchObject({ code: Code.Aborted })
    await expect(flowApi.remove(original)).rejects.toMatchObject({ code: Code.Aborted })
    expect(await load(cleared)).toEqual(cleared)
    const restored = await flowApi.save({ ...cleared, steps: original.steps })
    expect((await load(restored)).steps).toEqual(original.steps)
    await flowApi.remove(restored)
    expect((await flowApi.list(views[0].id)).some(flow => flow.id === original.id)).toBe(false)
    await expect(flowApi.remove(restored)).rejects.toMatchObject({ code: Code.Aborted })
  })

  it('rejects invalid API/YAML changes without replacing the saved tree', async () => {
    const original = await flowApi.save(importFlowYaml(source, views[0].id, diagram()))
    await expect(flowApi.save({ ...original, steps: [original.steps[0], original.steps[0]] })).rejects.toMatchObject({ code: Code.InvalidArgument })
    await expect(flowApi.save({ ...original, steps: [{ ...newStep(diagram()), elementIds: [1, 1] }] })).rejects.toMatchObject({ code: Code.InvalidArgument })
    expect(() => importFlowYaml(source.replace('elements: [API, Database]', 'elements: [API, Missing]'), views[0].id, diagram())).toThrow(/not in the target/)
    expect(() => importFlowYaml(source.replace('mode: alternate', 'mode: invalid'), views[0].id, diagram())).toThrow(/mode/)
    expect(await load(original)).toEqual(original)
  })
})
