import { computeLayout } from '../components/ZUI/layout'
import { describe, expect, it } from 'vitest'
import { create } from '@bufbuild/protobuf'
import { AgentWorkspaceEditSchema, PlanResourceSchema, PlanOperationSchema } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/plan_service_pb'
import { agentWorkspaceOverlay } from './agentWorkspaceOverlay'
import { connectorChangeFromTags, EDGE_CHANGE_META } from '../components/ZUI/edgeChange'
import type { ExploreData } from '../types'

const element = (id: number, name: string) => create(PlanResourceSchema, { key: `element:${id}`, value: { case: 'element', value: { id, name } } })
const empty = () => ({ data: { tree: [], views: {}, navigations: [] } as ExploreData, overlays: {} })
describe('agent proof-of-work overlay', () => {
  it('shows unplaced staged elements in blue without mutating the live scene', () => {
    const base = empty()
    const resource = element(1000000001, 'Worker')
    const edit = create(AgentWorkspaceEditSchema, { status: 'pending', operations: [{ action: { case: 'create', value: resource } }] })
    const rendered = agentWorkspaceOverlay(base, [edit], [])
    expect(Object.values(rendered.data.views).flatMap(v => v.placements).map(e => e.name)).toEqual(['Worker'])
    expect(rendered.overlays[1000000001].agentStaged).toBe(true)
    expect(base.data.views).toEqual({})
    expect(agentWorkspaceOverlay(base, [{ ...edit, status: 'discarded' }], [])).toBe(base)
    expect(agentWorkspaceOverlay(base, [{ ...edit, status: 'committed' }], [])).toBe(base)
  })
  it('shows staged connectors and endpoint context with the blue edge style', () => {
    const resource = create(PlanResourceSchema, { key: 'connector:1000000003', value: { case: 'connector', value: { id: 1000000003, viewId: 1, sourceElementId: 1, targetElementId: 2, label: 'calls' } } })
    const edit = create(AgentWorkspaceEditSchema, { status: 'pending', operations: [{ action: { case: 'create', value: resource } }] })
    const rendered = agentWorkspaceOverlay(empty(), [edit], [element(1, 'API'), element(2, 'Worker')])
    expect(rendered.data.views[1].placements).toHaveLength(2)
    expect(connectorChangeFromTags(rendered.data.views[1].connectors[0].tags)).toBe('staged')
    expect(EDGE_CHANGE_META.staged.color).toBe('#4299e1')
  })
  it('retains a removed element as a review ghost and ignores skipped edits', () => {
    const before = element(1, 'Existing')
    const edit = create(AgentWorkspaceEditSchema, { status: 'warning', operations: [create(PlanOperationSchema, { action: { case: 'remove', value: { key: before.key } }, preconditions: [{ before }] })] })
    const rendered = agentWorkspaceOverlay(empty(), [edit], [])
    expect(rendered.overlays[1]).toMatchObject({ agentStaged: true, path: 'Staged removal' })
    expect(agentWorkspaceOverlay(empty(), [{ ...edit, status: 'skipped' }], []).data.views).toEqual({})
  })
  it('marks staged views with a blue group border and their proposed name', () => {
    const resource = create(PlanResourceSchema, { key: 'view:1', value: { case: 'view', value: { id: 1, name: 'Proposed schema' } } })
    const edit = create(AgentWorkspaceEditSchema, { status: 'pending', operations: [{ action: { case: 'create', value: resource } }] })
    const rendered = agentWorkspaceOverlay(empty(), [edit], [])
    expect(rendered.data.tree[0].name).toBe('Proposed schema')
    expect(computeLayout(rendered.data).groups[0].agentStaged).toBe(true)
  })

})
