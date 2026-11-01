import { describe, expect, it } from 'vitest'
import type { Node } from 'reactflow'
import { resolvePendingConnectorHandles } from './utils'

const source: Node = { id: 'source', position: { x: 0, y: 0 }, width: 180, height: 85, data: {} }

describe('pending connector attachment', () => {
  it.each([
    [{ x: 400, y: 0 }, 'left-2'],
    [{ x: -400, y: 0 }, 'right-2'],
    [{ x: 0, y: 300 }, 'top-2'],
    [{ x: 0, y: -300 }, 'bottom-2'],
  ])('faces the source from a ghost at %j without moving the chosen source slot', (position, targetHandle) => {
    expect(resolvePendingConnectorHandles(source, { position }, 'right-4')).toEqual({ sourceHandle: 'right-4', targetHandle })
  })

  it('uses the same attachment for the preview and newly placed element', () => {
    const position = { x: 30, y: 250 }
    const ghost = resolvePendingConnectorHandles(source, { position }, 'left-0')
    const placed = resolvePendingConnectorHandles(source, { position, width: 180, height: 85 }, 'left-0')
    expect(placed).toEqual(ghost)
    expect(placed).toEqual({ sourceHandle: 'left-0', targetHandle: 'top-2' })
  })

  it('uses absolute node coordinates when the source is nested', () => {
    expect(resolvePendingConnectorHandles({ ...source, positionAbsolute: { x: 800, y: 0 } }, { position: { x: 400, y: 0 } }, 'left-2')).toEqual({ sourceHandle: 'left-2', targetHandle: 'right-2' })
  })
})
