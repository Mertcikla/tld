import { create, toJson } from '@bufbuild/protobuf'
import { FlowSchema, FlowBranchMode } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/flow_service_pb'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flowApi } from './api'
import { newStep } from './model'

const { fetchMock } = vi.hoisted(() => ({ fetchMock: vi.fn<typeof fetch>() }))
vi.mock('../api/transport', async () => {
  const { createConnectTransport } = await import('@connectrpc/connect-web')
  return { transport: createConnectTransport({ baseUrl: 'https://flows.test', fetch: fetchMock }) }
})
const loaded = () => create(FlowSchema, {
  id: 'flow', viewId: 1, revision: 1,
  content: { name: 'Checkout', steps: [{ id: 'existing', title: 'Store order', elementIds: [1, 2], connectorIds: [10] }] },
})
const jsonResponse = (value: unknown) => new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json' } })
beforeEach(() => {
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input, init) => {
    if (String(input).endsWith('/ListFlows')) return jsonResponse({ flows: [toJson(FlowSchema, loaded())] })
    const request = await new Response(init?.body).json()
    return jsonResponse({ flow: { content: request.content, viewId: request.viewId, id: request.id || 'created', revision: 2 } })
  })
})

describe('flow API serialization', () => {
  it('sends content separately from identity and expected revision through the real Connect serializer', async () => {
    const [flow] = await flowApi.list(1)
    const step = newStep({ elements: [], connectors: [] })
    const saved = await flowApi.save({ ...flow, steps: [...flow.steps, step] })
    expect(saved.revision).toBe(2)
    expect(saved.steps.map(item => item.id)).toEqual(['existing', step.id])
    const request = await new Response(fetchMock.mock.lastCall![1]?.body).json()
    expect(request).toMatchObject({ id: 'flow', viewId: 1, expectedRevision: 1 })
    expect(request.content.steps[0]).toMatchObject({ elementIds: [1, 2], connectorIds: [10] })
    expect(request.content).not.toHaveProperty('id')
    expect(request.content).not.toHaveProperty('revision')
    expect(request).not.toHaveProperty('flow')
  })

  it('copies highlight arrays and keeps protobuf markers out of domain data', async () => {
    const [flow] = await flowApi.list(1)
    const saved = await flowApi.save({ ...flow, steps: [{ ...flow.steps[0], connectorIds: [...flow.steps[0].connectorIds, 11] }] })
    expect(saved.steps[0].connectorIds).toEqual([10, 11])
    expect(flow.steps[0].connectorIds).toEqual([10])
    for (const value of [flow, saved]) {
      expect(value).not.toHaveProperty('$typeName')
      expect(value.steps[0]).not.toHaveProperty('$typeName')
      expect(value.steps[0]).not.toHaveProperty('metadata')
    }
  })

  it('round-trips nested branches and unavailable linked flows using typed controls', async () => {
    const [flow] = await flowApi.list(1)
    const note = newStep({ elements: [], connectors: [] })
    flow.steps = [{ ...note, id: 'decision', branches: { mode: 'alternate', paths: [
      { id: 'yes', name: 'Yes', steps: [{ ...note, id: 'link', goToFlowId: 'missing-flow' }] },
      { id: 'no', name: 'No', steps: [] },
    ] } }]
    const saved = await flowApi.save(flow)
    expect(saved.steps).toEqual(flow.steps)
    const request = await new Response(fetchMock.mock.lastCall![1]?.body).json()
    expect(request.content.steps[0].branches.mode).toBe('FLOW_BRANCH_MODE_ALTERNATE')
    expect(saved.steps[0].branches?.paths[0].steps[0]).not.toHaveProperty('$typeName')
  })

  it('rejects conflicting domain controls and unknown branch modes', async () => {
    const [flow] = await flowApi.list(1)
    const step = newStep({ elements: [], connectors: [] })
    const branches = { mode: 'alternate', paths: [] }
    await expect(flowApi.save({ ...flow, steps: [{ ...step, branches, goToFlowId: 'other' }] })).rejects.toThrow(/only one/)
    await expect(flowApi.save({ ...flow, steps: [{ ...step, branches: { ...branches, mode: 'unknown' } }] })).rejects.toThrow(/branch mode/)
    fetchMock.mockResolvedValue(jsonResponse({ flows: [toJson(FlowSchema, create(FlowSchema, { id: 'flow', viewId: 1, revision: 1, content: { name: 'Bad mode', steps: [{ id: 's', control: { case: 'branches', value: { mode: FlowBranchMode.UNSPECIFIED } } }] } }))] }))
    await expect(flowApi.list()).rejects.toThrow(/branch mode/)
  })
})
