import React from 'react'
import { act, create } from 'react-test-renderer'
import { ConnectionLineType, Position, type Node, type ConnectingHandle, type EdgeProps } from 'reactflow'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ViewBezierConnector, { ViewConnectorConnectionLine } from './ViewBezierConnector'
import { buildViewConnectorPath } from '../utils/connectorRoute'

const state = vi.hoisted(() => ({ canEdit: true, endHandle: null as ConnectingHandle | null, nodes: new Map<string, Node>(), drag: { kind: 'tag', name: 'critical', color: '#f00' } as { kind: string; name: string; color: string } | null, edges: [] as Array<{ id: string; data: Record<string, unknown> }> }))
vi.mock('reactflow', async () => {
  const actual = await vi.importActual<typeof import('reactflow')>('reactflow')
  return { ...actual,
    useStore: (selector: (store: unknown) => unknown) => selector({ nodeInternals: state.nodes, connectionEndHandle: state.endHandle, edges: state.edges }),
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
afterEach(() => { state.canEdit = true; state.drag = { kind: 'tag', name: 'critical', color: '#f00' }; state.edges = []; state.endHandle = null; state.nodes.clear() })

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


it('thickens only the visible stroke when selected', () => {
  const renderer = render({ selected: true })
  const paths = renderer.root.findAllByType('path')
  expect(paths[0].props.style).toMatchObject({ stroke: '#aaa', strokeWidth: 4 })
  expect(paths[1].props.style?.strokeWidth).not.toBe(4)
  act(() => renderer.unmount())
  const unselected = render()
  expect(unselected.root.findAllByType('path')[0].props.style.strokeWidth).toBe(2)
  act(() => unselected.unmount())
})


describe('snapped connector preview', () => {
  it.each([
    [Position.Top, 130, 100, 430, 100],
    [Position.Bottom, 130, 185, 430, 185],
    [Position.Left, 100, 130, 100, 430],
    [Position.Right, 280, 130, 280, 430],
  ] as const)('approaches a %s handle from outside and matches the placed route', (side, fromX, fromY, toX, toY) => {
    const fromNode: Node = { id: '1', position: { x: 100, y: 100 }, width: 180, height: 85, data: {} }
    const targetNode: Node = { id: '2', position: { x: 400, y: 100 }, width: 220, height: 110, data: {} }
    state.nodes.set('2', targetNode)
    state.endHandle = { nodeId: '2', type: 'source', handleId: `${side}-0` }
    const opposites = { top: Position.Bottom, bottom: Position.Top, left: Position.Right, right: Position.Left }
    const renderer = create(<ViewConnectorConnectionLine
      fromNode={fromNode} fromHandle={{ id: `${side}-0`, position: side, x: 0, y: 0, width: 8, height: 8 }}
      fromX={fromX} fromY={fromY} toX={toX} toY={toY}
      fromPosition={side} toPosition={opposites[side]}
      connectionLineType={ConnectionLineType.Bezier} connectionStatus="valid"
    />)
    const path = renderer.root.findByType('path').props.d
    expect(path).toBe(buildViewConnectorPath({
      routeStyle: 'bezier', sourceX: fromX, sourceY: fromY, targetX: toX, targetY: toY,
      sourcePosition: side, targetPosition: side, sourceWidth: 180, sourceHeight: 85, targetWidth: 220, targetHeight: 110,
    }).path)
    const coordinates = path.match(/-?\d+(?:\.\d+)?/g)!.map(Number)
    const [sx, sy, c1x, c1y, c2x, c2y, tx, ty] = coordinates
    if (side === Position.Top) { expect(c1y).toBeLessThan(sy); expect(c2y).toBeLessThan(ty) }
    if (side === Position.Bottom) { expect(c1y).toBeGreaterThan(sy); expect(c2y).toBeGreaterThan(ty) }
    if (side === Position.Left) { expect(c1x).toBeLessThan(sx); expect(c2x).toBeLessThan(tx) }
    if (side === Position.Right) { expect(c1x).toBeGreaterThan(sx); expect(c2x).toBeGreaterThan(tx) }
    act(() => renderer.unmount())
  })
})

it('snaps a newly dragged connector to a node body before releasing', () => {
  const fromNode: Node = { id: '1', type: 'elementNode', position: { x: 0, y: 0 }, width: 180, height: 85, data: {} }
  const targetNode: Node = { id: '2', type: 'elementNode', position: { x: 400, y: 0 }, width: 220, height: 100, data: {} }
  state.nodes.set('2', targetNode)
  const renderer = create(<ViewConnectorConnectionLine
    fromNode={fromNode} fromHandle={{ id: 'right-2', position: Position.Right, x: 180, y: 42.5, width: 8, height: 8 }}
    fromX={180} fromY={42.5} toX={510} toY={50}
    fromPosition={Position.Right} toPosition={Position.Left}
    connectionLineType={ConnectionLineType.Bezier} connectionStatus={null}
  />)
  expect(renderer.root.findByType('path').props.d).toBe(buildViewConnectorPath({
    routeStyle: 'bezier', sourceX: 180, sourceY: 42.5, targetX: 400, targetY: 50,
    sourcePosition: Position.Right, targetPosition: Position.Left,
    sourceWidth: 180, sourceHeight: 85, targetWidth: 220, targetHeight: 100,
  }).path)
  act(() => renderer.unmount())
})

it.each(['bezier', 'straight', 'step', 'smoothstep'])('keeps %s endpoints fixed at node borders during hover and selection', routeStyle => {
  state.nodes.set('1', { id: '1', position: { x: 0, y: 0 }, width: 180, height: 85, data: {} })
  state.nodes.set('2', { id: '2', position: { x: 400, y: 0 }, width: 180, height: 85, data: {} })
  for (const [selected, isHovered, start, end] of [
    [false, false, 180, 400], [false, true, 180, 400], [true, false, 180, 400],
  ] as const) {
    const renderer = render({ sourceX: 184, sourceY: 42.5, targetX: 396, targetY: 42.5,
      sourceHandleId: 'right-2', targetHandleId: 'left-2', selected, data: { style: routeStyle, isHovered } })
    const path = renderer.root.findAllByType('path')[0].props.path
    expect(path).toMatch(new RegExp(`^M ${start},42\\.5 `))
    expect(path).toMatch(new RegExp(`${end},42\\.5$`))
    act(() => renderer.unmount())
  }
})
