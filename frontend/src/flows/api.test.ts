import { create, toJson } from '@bufbuild/protobuf'
import { FlowSchema } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/flow_service_pb'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flowApi } from './api'
import { newStep, type StoryFlow } from './model'

const { fetchMock } = vi.hoisted(() => ({ fetchMock: vi.fn<typeof fetch>() }))
vi.mock('../api/transport', async () => {
  const { createConnectTransport } = await import('@connectrpc/connect-web')
  return { transport: createConnectTransport({ baseUrl: 'https://flows.test', fetch: fetchMock }) }
})

const loaded = () => create(FlowSchema, {
  id: 'flow', viewId: 1, name: 'Checkout', revision: 1,
  steps: [{ id: 'existing', title: 'Store order', elementIds: [1, 2], connectors: [{ id: 10, reverse: true }], metadata: { owner: 'Commerce' } }],
})
const jsonResponse = (value: unknown) => new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json' } })
beforeEach(() => {
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input, init) => {
    if (String(input).endsWith('/ListFlows')) return jsonResponse({ flows: [toJson(FlowSchema, loaded())] })
    const request = await new Response(init?.body).json()
    return jsonResponse({ flow: { ...request.flow, id: request.flow.id || 'created', revision: 2 } })
  })
})

describe('flow API serialization', () => {
  it('saves an added plain step in a previously loaded flow through the real Connect JSON serializer', async () => {
    const flow = await flowApi.list(1)
    const step = newStep({ elements: [], connectors: [] })
    const draft = { ...flow[0], steps: [...flow[0].steps, step] }
    const saved = await flowApi.save(draft)
    expect(saved.revision).toBe(2)
    expect(saved.steps.map(item => item.id)).toEqual(['existing', step.id])
    const request = await new Response(fetchMock.mock.lastCall![1]?.body).json()
    expect(request.flow.steps[0]).toMatchObject({ elementIds: [1, 2], connectors: [{ id: 10, reverse: true }], metadata: { owner: 'Commerce' } })
    expect(request.flow.steps[1].id).toBe(step.id)
  })

  it('repairs already cached protobuf drafts with plain steps and connector highlights', async () => {
    const message = loaded()
    const extra = newStep({ elements: [], connectors: [] })
    const draft: StoryFlow = { ...message, steps: [{ ...message.steps[0], connectors: [...message.steps[0].connectors, { id: 11, reverse: false }] }, extra] }
    const saved = await flowApi.save(draft)
    expect(saved.steps).toHaveLength(2)
    expect(saved.steps[0].connectors).toEqual([{ id: 10, reverse: true }, { id: 11, reverse: false }])
    expect(message.steps[0].connectors).toHaveLength(1)
    expect(extra).not.toHaveProperty('$typeName')
  })

  it('keeps protobuf message markers out of listed flows and save responses', async () => {
    const [flow] = await flowApi.list(1)
    const saved = await flowApi.save(flow)
    for (const result of [flow, saved]) {
      expect(result).not.toHaveProperty('$typeName')
      expect(result.steps[0]).not.toHaveProperty('$typeName')
      expect(result.steps[0].connectors[0]).not.toHaveProperty('$typeName')
      expect(result.steps[0].metadata).toEqual({ owner: 'Commerce' })
    }
  })
})

it('serializes new nested steps and control fields through the Connect serializer', async () => {
  const [flow] = await flowApi.list(1)
  const note = newStep({ elements: [], connectors: [] })
  flow.steps = [{ ...note, overview: true }, { ...note, id: 'decision', branches: { mode: 'alternate', paths: [
    { id: 'yes', name: 'Yes', steps: [{ ...note, id: 'link', goToFlowId: 'other-flow' }] },
    { id: 'no', name: 'No', steps: [] },
  ] } }]
  const saved = await flowApi.save(flow)
  expect(saved.steps).toEqual(flow.steps)
  expect(saved.steps[1].branches?.paths[0].steps[0]).not.toHaveProperty('$typeName')
})
