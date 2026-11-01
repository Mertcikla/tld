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
