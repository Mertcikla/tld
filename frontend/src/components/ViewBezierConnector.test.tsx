import React from 'react'
import { act, create } from 'react-test-renderer'
import { Position, type EdgeProps } from 'reactflow'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ViewBezierConnector from './ViewBezierConnector'

const state = vi.hoisted(() => ({ canEdit: true, drag: { kind: 'tag', name: 'critical', color: '#f00' } as { kind: string; name: string; color: string } | null, edges: [] as Array<{ id: string; data: Record<string, unknown> }> }))
vi.mock('reactflow', async () => {
  const actual = await vi.importActual<typeof import('reactflow')>('reactflow')
  return { ...actual,
    useStore: (selector: (store: unknown) => unknown) => selector({ nodeInternals: new Map(), edges: state.edges }),
    BaseEdge: (props: Record<string, unknown>) => <path {...props} />,
    EdgeLabelRenderer: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  }
})
vi.mock('../pages/ViewEditor/context', () => ({ useViewEditorContext: () => ({ canEdit: state.canEdit }) }))
vi.mock('./ViewExplorer/TagManager/tagDragGhostState', () => ({ useTagDragGhost: () => state.drag }))
vi.mock('./ViewEditorEdgeLabelLayout', () => ({ measureEdgeLabel: (text: string) => text.length * 7, useEdgeLabelLayout: () => ({ x: 100, y: 50 }) }))

function render(overrides: Partial<EdgeProps> = {}) {
  return create(<ViewBezierConnector {...{
    id: '12', source: '1', target: '2', sourceX: 0, sourceY: 0, targetX: 200, targetY: 100,
    sourcePosition: Position.Right, targetPosition: Position.Left, label: 'reads', selected: false,
    style: { stroke: '#aaa', strokeWidth: 2 }, data: { style: 'bezier' }, ...overrides,
  } as EdgeProps} />)
}
afterEach(() => { state.canEdit = true; state.drag = { kind: 'tag', name: 'critical', color: '#f00' }; state.edges = [] })

it.each([
  ['x'.repeat(20), 'x'.repeat(20)],
  ['x'.repeat(21), 'x'.repeat(21)],
  ['x'.repeat(30), 'x'.repeat(30)],
  ['x'.repeat(31), `${'x'.repeat(20)}…`],
  ['A connector label longer than twenty characters', 'A connector label longer…'],
  [`${'x'.repeat(30)} more text`, `${'x'.repeat(30)}…`],
  [`${'x'.repeat(20)}   more text that continues`, `${'x'.repeat(20)}…`],
  ['🚀'.repeat(31), `${'🚀'.repeat(20)}…`],
])('truncates %s using the soft limit and whole words', (label, expected) => {
  const renderer = render({ label })
  const text = renderer.root.findByProps({ 'aria-label': label })
  expect(text.children).toEqual([expected])
  expect(text.props.title).toBe(label)
  expect(text.props.style.width).toBe(expected.length * 7 + 8)
  act(() => renderer.unmount())
})

it('expands long labels on hover or selection and collapses them when inactive', () => {
  const label = 'A connector label longer than twenty characters'
  const renderer = render({ label })
  const labelWidth = () => renderer.root.findByProps({ 'aria-label': label }).props.style.width
  const collapsedWidth = 'A connector label longer…'.length * 7 + 8
  const expandedWidth = label.length * 7 + 8
  expect(labelWidth()).toBe(collapsedWidth)
  for (const [selected, isHovered, expectedWidth] of [
    [false, true, expandedWidth],
    [false, false, collapsedWidth],
    [true, true, expandedWidth],
    [true, false, expandedWidth],
  ] as const) {
    act(() => renderer.update(<ViewBezierConnector {...{
      id: '12', source: '1', target: '2', sourceX: 0, sourceY: 0, targetX: 200, targetY: 100,
      sourcePosition: Position.Right, targetPosition: Position.Left, label, selected,
      data: { style: 'bezier', isHovered },
    } as EdgeProps} />))
    expect(labelWidth()).toBe(expectedWidth)
    expect(renderer.root.findByProps({ 'aria-label': label }).children).toEqual([
      selected || isHovered ? label : 'A connector label longer…',
    ])
  }
  act(() => renderer.unmount())
})

it('opens aggregate details without letting pointer presses change canvas selection', () => {
  const details = { key: 'pair', connectors: [] }
  const openDetails = vi.fn()
  state.edges = [{ id: '12', data: { proxyBadgeCount: 1, proxyBadgeDetails: details, onOpenProxyBadge: openDetails } }]
  const renderer = render()
  const button = renderer.root.findByType('button')
  for (const handler of ['onPointerDown', 'onMouseDown']) {
    const event = { stopPropagation: vi.fn(), preventDefault: vi.fn() }
    act(() => button.props[handler](event))
    expect(event.stopPropagation).toHaveBeenCalledOnce()
    expect(event.preventDefault).not.toHaveBeenCalled()
  }
  act(() => button.props.onClick({ stopPropagation: vi.fn(), preventDefault: vi.fn() }))
  expect(openDetails).toHaveBeenCalledWith(details)
  act(() => renderer.unmount())
})

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
