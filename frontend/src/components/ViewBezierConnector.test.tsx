import React from 'react'
import { act, create } from 'react-test-renderer'
import { Position, type EdgeProps } from 'reactflow'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ViewBezierConnector from './ViewBezierConnector'

const state = vi.hoisted(() => ({ canEdit: true, drag: { kind: 'tag', name: 'critical', color: '#f00' } as { kind: string; name: string; color: string } | null }))
vi.mock('reactflow', async () => {
  const actual = await vi.importActual<typeof import('reactflow')>('reactflow')
  return { ...actual,
    useStore: (selector: (store: unknown) => unknown) => selector({ nodeInternals: new Map(), edges: [] }),
    BaseEdge: (props: Record<string, unknown>) => <path {...props} />,
    EdgeLabelRenderer: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  }
})
vi.mock('../pages/ViewEditor/context', () => ({ useViewEditorContext: () => ({ canEdit: state.canEdit }) }))
vi.mock('./ViewExplorer/TagManager/tagDragGhostState', () => ({ useTagDragGhost: () => state.drag }))
vi.mock('./ViewEditorEdgeLabelLayout', () => ({ measureEdgeLabel: () => 40, useEdgeLabelLayout: () => ({ x: 100, y: 50 }) }))

function render(overrides: Partial<EdgeProps> = {}) {
  return create(<ViewBezierConnector {...{
    id: '12', source: '1', target: '2', sourceX: 0, sourceY: 0, targetX: 200, targetY: 100,
    sourcePosition: Position.Right, targetPosition: Position.Left, label: 'reads', selected: false,
    style: { stroke: '#aaa', strokeWidth: 2 }, data: { style: 'bezier' }, ...overrides,
  } as EdgeProps} />)
}
afterEach(() => { state.canEdit = true; state.drag = { kind: 'tag', name: 'critical', color: '#f00' } })

describe('connector tag drop affordance', () => {
  it('highlights the line and exposes its label as the same drop target, then clears on drop', () => {
    const renderer = render()
    const target = renderer.root.findByType('g')
    const event = { preventDefault: vi.fn(), stopPropagation: vi.fn(), dataTransfer: { types: ['application/diag-tag'], dropEffect: '' } }
    act(() => target.props.onDragOver(event))
    expect(event.preventDefault).toHaveBeenCalledOnce()
    expect(event.dataTransfer.dropEffect).toBe('copy')
    expect(renderer.root.findAllByType('path')[0].props.style).toMatchObject({ stroke: '#f00', strokeWidth: 3 })
    const label = renderer.root.findAllByType('div').find((node) => node.props['data-tag-drop-connector-id'] === '12')!
    expect(label.props.style.pointerEvents).toBe('auto')
    act(() => label.props.onDrop())
    expect(renderer.root.findAllByType('path')[0].props.style).toMatchObject({ stroke: '#aaa', strokeWidth: 2 })
    act(() => renderer.unmount())
  })

  it('does not accept drops on read-only or dimmed connectors', () => {
    for (const readOnly of [true, false]) {
      state.canEdit = !readOnly
      const renderer = render(readOnly ? {} : { style: { pointerEvents: 'none', opacity: 0.1 } })
      const target = renderer.root.findByType('g')
      const event = { preventDefault: vi.fn(), dataTransfer: { types: ['application/diag-tag'] } }
      act(() => target.props.onDragOver(event))
      expect(event.preventDefault).not.toHaveBeenCalled()
      expect(target.props['data-tag-drop-connector-id']).toBeUndefined()
      act(() => renderer.unmount())
    }
  })
})

describe('connector hover direction', () => {
  it.each([
    ['forward', 'forward'], ['backward', 'backward'], ['both', 'both'],
    ['bidirectional', 'both'], ['none', 'none'], [undefined, 'forward'],
  ])('exposes %s as %s for directional hover animation', (direction, expected) => {
    const renderer = render({ data: { style: 'bezier', direction } })
    const visual = renderer.root.findByType('g')
    expect(visual.props.className).toBe('vieweditor-connector')
    expect(visual.props['data-connector-direction']).toBe(expected)
    act(() => renderer.unmount())
  })
})
