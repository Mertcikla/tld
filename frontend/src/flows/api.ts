import { createClient } from '@connectrpc/connect'
import { create } from '@bufbuild/protobuf'
import { FlowSchema, FlowService } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/flow_service_pb'
import { transport } from '../api/transport'
import type { StoryFlow } from './model'

const client = createClient(FlowService, transport)

// Drafts are plain domain data. Copy fields explicitly so protobuf markers from
// loaded/cached messages cannot bypass conversion of newly added nested objects.
function flowData(flow: StoryFlow): StoryFlow {
  return {
    id: flow.id, viewId: flow.viewId, name: flow.name, description: flow.description, revision: flow.revision,
    steps: flow.steps.map(step => ({
      id: step.id, title: step.title, description: step.description,
      elementIds: [...step.elementIds],
      connectors: step.connectors.map(connector => ({ id: connector.id, reverse: connector.reverse })),
      metadata: { ...step.metadata },
    })),
  }
}

export const flowApi = {
  async list(viewId = 0): Promise<StoryFlow[]> {
    return (await client.listFlows({ viewId })).flows.map(flowData)
  },
  async save(flow: StoryFlow): Promise<StoryFlow> {
    const response = await client.saveFlow({ flow: create(FlowSchema, flowData(flow)) })
    if (!response.flow) throw new Error('The server did not return the saved flow.')
    return flowData(response.flow)
  },
  async remove(flow: StoryFlow): Promise<void> {
    await client.deleteFlow({ id: flow.id, viewId: flow.viewId, expectedRevision: flow.revision })
  },
}
