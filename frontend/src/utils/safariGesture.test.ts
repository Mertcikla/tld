import { describe, expect, it } from 'vitest'
import { safariGestureClientPoint, safariGestureFactor, safariGestureScale } from './safariGesture'

describe('Safari gesture helpers', () => {
  it('reads valid positive gesture scales', () => {
    expect(safariGestureScale({ scale: 1.25 })).toBe(1.25)
    expect(safariGestureScale({ scale: 0 })).toBeNull()
    expect(safariGestureScale({ scale: Number.NaN })).toBeNull()
    expect(safariGestureScale({ scale: '1.25' })).toBeNull()
  })

  it('converts cumulative Safari gesture scale to incremental zoom factors', () => {
    expect(safariGestureFactor({ scale: 1.2 }, 1)).toEqual({ scale: 1.2, factor: 1.2 })
    expect(safariGestureFactor({ scale: 1.5 }, 1.2)).toEqual({ scale: 1.5, factor: 1.25 })
    expect(safariGestureFactor({ scale: 0.75 }, 1.5)).toEqual({ scale: 0.75, factor: 0.5 })
  })

  it('uses the current scale as the baseline when a gesture starts without one', () => {
    expect(safariGestureFactor({ scale: 1.2 }, null)).toEqual({ scale: 1.2, factor: 1 })
  })

  it('falls back when Safari gesture events omit client coordinates', () => {
    expect(safariGestureClientPoint({ scale: 1.1 }, { clientX: 200, clientY: 120 })).toEqual({ clientX: 200, clientY: 120 })
    expect(safariGestureClientPoint({ clientX: 320, clientY: 240 }, { clientX: 200, clientY: 120 })).toEqual({ clientX: 320, clientY: 240 })
  })
})
