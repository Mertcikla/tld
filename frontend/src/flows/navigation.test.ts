import { describe, expect, it } from 'vitest'
import { enterFlow, returnFromFlow, type FlowVisit } from './navigation'
import type { StoryFlow } from './model'

const flow: StoryFlow = { id: 'parent', viewId: 1, name: 'Parent', description: '', revision: 1, steps: [] }
const current: FlowVisit = { flow, stepId: 'link', choices: { decision: 'failure' }, draft: true }
it('returns across diagrams to the exact caller and preserves unsaved content and choices', () => {
  const child = { ...flow, id: 'child', viewId: 2 }
  const journey = enterFlow(current, [], child)
  expect(journey.current).toEqual({ flow: child, stepId: null, choices: {} })
  expect(returnFromFlow(journey)).toEqual({ current, stack: [] })
  expect(returnFromFlow({ current, stack: [] })).toBeNull()
})
describe('recursive links', () => {
  it('allows explicit recursive visits with a bounded return stack', () => {
    let journey = enterFlow(current, [], flow)
    for (let i = 1; i < 16; i++) journey = enterFlow(journey.current, journey.stack, flow)
    expect(() => enterFlow(journey.current, journey.stack, flow)).toThrow(/maximum depth/)
    expect(returnFromFlow(journey)?.stack).toHaveLength(15)
  })
})
