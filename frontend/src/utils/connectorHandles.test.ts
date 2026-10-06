import { describe, expect, it } from 'vitest'
import { chooseConnectorHandles } from './connectorHandles'

describe('chooseConnectorHandles', () => {
  it('faces side-by-side nodes across their horizontal edges', () => {
    expect(chooseConnectorHandles({ x: 0, y: 0 }, { x: 400, y: 0 })).toEqual({ source: 'right', target: 'left' })
    expect(chooseConnectorHandles({ x: 400, y: 0 }, { x: 0, y: 0 })).toEqual({ source: 'left', target: 'right' })
  })

  it('faces stacked nodes across their vertical edges', () => {
    expect(chooseConnectorHandles({ x: 0, y: 0 }, { x: 0, y: 300 })).toEqual({ source: 'bottom', target: 'top' })
    expect(chooseConnectorHandles({ x: 0, y: 300 }, { x: 0, y: 0 })).toEqual({ source: 'top', target: 'bottom' })
  })
})
