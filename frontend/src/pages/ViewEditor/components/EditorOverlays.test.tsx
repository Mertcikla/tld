import React from 'react'
import { act, create } from 'react-test-renderer'
import { Position, type Node } from 'reactflow'
import { afterEach, expect, it, vi } from 'vitest'
import { EditorOverlays } from './EditorOverlays'
import { resolvePendingConnectorHandles } from '../utils'
import { buildViewConnectorPath } from '../../../utils/connectorRoute'

vi.mock('reactflow', async () => ({
  ...await vi.importActual<typeof import('reactflow')>('reactflow'),
  useViewport: () => ({ x: 20, y: 30, zoom: 2 }),
}))
afterEach(() => vi.unstubAllGlobals())

it('snaps the body-hover ghost to the attachment used for creation, at the current zoom', () => {
  vi.stubGlobal('document', { querySelector: () => ({ getBoundingClientRect: () => ({ left: 10, top: 15 }) }) })
  const source: Node = { id: '1', position: { x: 0, y: 0 }, width: 180, height: 85, data: {} }
  const target: Node = { id: '2', position: { x: 400, y: 0 }, width: 220, height: 100, data: {} }
  const handles = resolvePendingConnectorHandles(source, target, 'right-2')
  expect(handles.targetHandle).toBe('left-2')
  const props = {
    clickConnectMode: { sourceNodeId: source.id, sourceHandle: handles.sourceHandle, targetHandle: handles.targetHandle, targetNodeId: target.id },
    clickConnectCursorPos: { x: 1050, y: 145 }, // Cursor is at the node center.
    connectorRouteStyle: 'bezier' as const,
    handleReconnectDrag: null,
    rfNodes: [source, target],
  }
  const renderer = create(<EditorOverlays {...props} />)
  const expected = buildViewConnectorPath({
    routeStyle: 'bezier', sourceX: 390, sourceY: 130, targetX: 830, targetY: 145,
    sourcePosition: Position.Right, targetPosition: Position.Left,
    sourceWidth: 360, sourceHeight: 170, targetWidth: 440, targetHeight: 200,
  }).path
  expect(renderer.root.findByType('path').props.d).toBe(expected)

  act(() => renderer.update(<EditorOverlays {...props}
    clickConnectMode={{ sourceNodeId: source.id, sourceHandle: handles.sourceHandle }}
    clickConnectCursorPos={{ x: 1200, y: 500 }}
  />))
  expect(renderer.root.findByType('path').props.d).toMatch(/1200,500$/)
  act(() => renderer.unmount())
})

it.each(['source', 'target'] as const)('snaps a dragged %s endpoint to the node boundary rather than its center', endpoint => {
  vi.stubGlobal('document', { querySelector: () => ({ getBoundingClientRect: () => ({ left: 10, top: 15 }) }) })
  const fixed: Node = { id: '1', position: { x: 0, y: 0 }, width: 180, height: 85, data: {} }
  const moving: Node = { id: '2', position: { x: 400, y: 0 }, width: 220, height: 100, data: {} }
  const handle = resolvePendingConnectorHandles(fixed, moving, 'right-2').targetHandle
  const renderer = create(<EditorOverlays
    clickConnectMode={null} clickConnectCursorPos={null} connectorRouteStyle="bezier"
    rfNodes={[fixed, moving]}
    handleReconnectDrag={{ endpoint, fixedNodeId: '1', fixedHandle: 'right-2', movingHandle: handle,
      hoveredNodeId: '2', hoveredHandleId: handle, cursorPos: { x: 1050, y: 145 } }}
  />)
  const movingIsSource = endpoint === 'source'
  expect(renderer.root.findByType('path').props.d).toBe(buildViewConnectorPath({
    routeStyle: 'bezier', sourceX: movingIsSource ? 830 : 390, sourceY: movingIsSource ? 145 : 130,
    targetX: movingIsSource ? 390 : 830, targetY: movingIsSource ? 130 : 145,
    sourcePosition: movingIsSource ? Position.Left : Position.Right,
    targetPosition: movingIsSource ? Position.Right : Position.Left,
    sourceWidth: movingIsSource ? 440 : 360, sourceHeight: movingIsSource ? 200 : 170,
    targetWidth: movingIsSource ? 360 : 440, targetHeight: movingIsSource ? 170 : 200,
  }).path)
  act(() => renderer.unmount())
})
