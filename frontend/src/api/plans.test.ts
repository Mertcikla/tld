import { create } from '@bufbuild/protobuf'
import { describe, expect, it, vi } from 'vitest'
import { PlanResourceSchema } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/plan_service_pb'
vi.mock('./transport', () => ({ transport: {} }))
import { resourceLabel } from './plans'
describe('plan resource labels', () => {
  const api = create(PlanResourceSchema, {key:'element:uuid', value:{case:'element',value:{id:-1,name:'API'}}})
  const db = create(PlanResourceSchema, {value:{case:'element',value:{id:2,name:'Database'}}})
  const view = create(PlanResourceSchema, {value:{case:'view',value:{id:1,name:'Workspace'}}})
  const resources = [api,db,view]
  it('describes unlabeled connectors by their endpoints', () => {
    const r = create(PlanResourceSchema, {key:'connector:uuid',value:{case:'connector',value:{sourceElementId:-1,targetElementId:2,direction:'forward'}}})
    expect(resourceLabel(r,resources)).toBe('API → Database')
    if (r.value.case === 'connector') r.value.value.label = 'reads'
    expect(resourceLabel(r,resources)).toBe('API → Database (reads)')
  })
  it('describes placements and missing references without raw keys', () => {
    const r = create(PlanResourceSchema, {key:'placement:uuid',value:{case:'placement',value:{elementId:-1,viewId:1}}})
    expect(resourceLabel(r,resources)).toBe('API in Workspace')
    expect(resourceLabel(r,[])).toBe('Missing element in Missing view')
  })
})
