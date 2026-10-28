import { createClient } from '@connectrpc/connect'
import { create } from '@bufbuild/protobuf'
import { FlowContentSchema, FlowStepSchema, FlowBranchMode, FlowService, type Flow, type FlowStep as ProtoFlowStep } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/flow_service_pb'
import { transport } from '../api/transport'
import type { FlowStep, StoryFlow } from './model'

const client = createClient(FlowService, transport)

function stepMessage(step: FlowStep): ProtoFlowStep {
  if (step.branches && step.goToFlowId !== undefined) throw new Error('Choose only one of branches or a linked flow.')
  if (step.branches && !['alternate', 'parallel'].includes(step.branches.mode)) throw new Error('Unknown flow branch mode.')
  return create(FlowStepSchema, {
    id: step.id, title: step.title, description: step.description,
    elementIds: [...step.elementIds], connectorIds: [...step.connectorIds],
    control: step.branches ? { case: 'branches', value: {
      mode: step.branches.mode === 'parallel' ? FlowBranchMode.PARALLEL : FlowBranchMode.ALTERNATE,
      paths: step.branches.paths.map(path => ({ id: path.id, name: path.name, steps: path.steps.map(stepMessage) })),
    } } : step.goToFlowId !== undefined ? { case: 'goToFlowId', value: step.goToFlowId } : undefined,
  })
}

function stepData(step: ProtoFlowStep): FlowStep {
  const branches = step.control.case === 'branches' ? step.control.value : undefined
  if (branches && branches.mode !== FlowBranchMode.ALTERNATE && branches.mode !== FlowBranchMode.PARALLEL) throw new Error('Unknown flow branch mode.')
  return {
    id: step.id, title: step.title, description: step.description,
    elementIds: [...step.elementIds], connectorIds: [...step.connectorIds],
    ...(step.control.case === 'goToFlowId' ? { goToFlowId: step.control.value } : {}),
    ...(branches ? { branches: { mode: branches.mode === FlowBranchMode.PARALLEL ? 'parallel' : 'alternate', paths: branches.paths.map(path => ({ id: path.id, name: path.name, steps: path.steps.map(stepData) })) } } : {}),
  }
}

function flowData(flow: Flow): StoryFlow {
  if (!flow.content) throw new Error('The server did not return flow content.')
  return {
    id: flow.id, viewId: flow.viewId, revision: flow.revision,
    name: flow.content.name, description: flow.content.description, steps: flow.content.steps.map(stepData),
  }
}

export const flowApi = {
  async list(viewId = 0): Promise<StoryFlow[]> {
    return (await client.listFlows({ viewId })).flows.map(flowData)
  },
  async save(flow: StoryFlow): Promise<StoryFlow> {
    const response = await client.saveFlow({
      id: flow.id, viewId: flow.viewId, expectedRevision: flow.revision,
      content: create(FlowContentSchema, { name: flow.name, description: flow.description, steps: flow.steps.map(stepMessage) }),
    })
    if (!response.flow) throw new Error('The server did not return the saved flow.')
    return flowData(response.flow)
  },
  async remove(flow: StoryFlow): Promise<void> {
    await client.deleteFlow({ id: flow.id, viewId: flow.viewId, expectedRevision: flow.revision })
  },
}
