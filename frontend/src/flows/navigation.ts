import type { StoryFlow } from './model'

export interface FlowVisit {
  flow: StoryFlow
  stepId: string | null
  choices: Record<string, string>
  draft?: boolean
}
export interface FlowJourney { current: FlowVisit; stack: FlowVisit[] }

/** Explicit calls may be recursive, but cannot grow an unbounded return stack. */
export function enterFlow(current: FlowVisit, stack: FlowVisit[], target: StoryFlow): FlowJourney {
  if (stack.length >= 16) throw new Error('Return to a previous flow before opening another linked flow (maximum depth: 16).')
  return { current: { flow: target, stepId: null, choices: {} }, stack: [...stack, current] }
}

export function returnFromFlow(journey: FlowJourney): FlowJourney | null {
  const current = journey.stack[journey.stack.length - 1]
  return current ? { current, stack: journey.stack.slice(0, -1) } : null
}
