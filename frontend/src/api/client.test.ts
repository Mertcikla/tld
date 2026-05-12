import { describe, expect, it } from 'vitest'
import { normalizeConnectorRouteStyle } from './client'

describe('normalizeConnectorRouteStyle', () => {
  it('keeps valid route styles', () => {
    expect(normalizeConnectorRouteStyle('bezier')).toBe('bezier')
    expect(normalizeConnectorRouteStyle('straight')).toBe('straight')
    expect(normalizeConnectorRouteStyle('step')).toBe('step')
    expect(normalizeConnectorRouteStyle('smoothstep')).toBe('smoothstep')
  })

  it('maps legacy line styles to bezier', () => {
    expect(normalizeConnectorRouteStyle('solid')).toBe('bezier')
    expect(normalizeConnectorRouteStyle('dashed')).toBe('bezier')
    expect(normalizeConnectorRouteStyle(undefined)).toBe('bezier')
  })
})
