import { afterEach, describe, expect, it, vi } from 'vitest'
import { canvasHighlightTarget, isFlowPanelTarget, captureCanvasStep } from './canvasInteraction'
import { type FlowDiagram } from './model'
import type { Connector } from '../types'

const diagram: FlowDiagram = { elements: [{ element_id: 1, name: 'API' }], connectors: [{ id: 10 } as Connector] }
afterEach(() => vi.unstubAllGlobals())
describe('canvas step recording', () => {
  it('captures individual current-view references with distinct step identities', () => {
    const first = captureCanvasStep({ elementId: 1 }, diagram)!
    const repeat = captureCanvasStep({ elementId: 1 }, diagram)!
    expect(first.elementIds).toEqual([1])
    expect(first.connectors).toEqual([])
    expect(repeat.id).not.toBe(first.id)
    expect(captureCanvasStep({ connectorId: 10 }, diagram)).toEqual(expect.objectContaining({ elementIds: [], connectors: [{ id: 10, reverse: false }] }))
    expect(captureCanvasStep({ elementId: 99 }, diagram)).toBeNull()
    expect(captureCanvasStep({ connectorId: 99 }, diagram)).toBeNull()
  })

  it('recognizes node bodies, connection labels and panel controls without accepting synthetic nodes', () => {
    class Target {
      constructor(readonly selectors: Record<string, string>) {}
      closest(selector: string) { return selector in this.selectors ? { getAttribute: () => this.selectors[selector] } : null }
    }
    vi.stubGlobal('Element', Target)
    const target = (selectors: Record<string, string>) => new Target(selectors) as unknown as EventTarget
    expect(canvasHighlightTarget(target({ '.react-flow__node': '1' }))).toEqual({ elementId: 1 })
    expect(canvasHighlightTarget(target({ '[data-tag-drop-connector-id]': '10' }))).toEqual({ connectorId: 10 })
    expect(canvasHighlightTarget(target({ '.react-flow__node': 'context-1' }))).toBeNull()
    expect(canvasHighlightTarget(target({}))).toBeNull()
    expect(isFlowPanelTarget(target({ '[data-testid="flow-panel"]': '' }))).toBe(true)
    expect(isFlowPanelTarget(target({}))).toBe(false)
  })
})
