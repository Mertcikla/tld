import { describe, expect, it } from 'vitest'
import { isMouseWheelGesture, isNotchedWheelGesture, normalizeWheelDeltaY, wheelZoomFactor, type WheelDeltaLike } from './wheel'

function wheel(overrides: Partial<WheelDeltaLike>): WheelDeltaLike {
  return {
    deltaX: 0,
    deltaY: 0,
    deltaMode: 0,
    ctrlKey: false,
    ...overrides,
  }
}

describe('wheel gesture helpers', () => {
  it('classifies line-mode wheel events as mouse wheels even with small raw deltas', () => {
    const event = wheel({ deltaY: 3, deltaMode: 1 })

    expect(isNotchedWheelGesture(event)).toBe(false)
    expect(isMouseWheelGesture(event)).toBe(true)
  })

  it('classifies pixel-mode notched wheel events as mouse wheels', () => {
    const event = wheel({ deltaY: 120 })

    expect(isNotchedWheelGesture(event)).toBe(true)
    expect(isMouseWheelGesture(event)).toBe(true)
  })

  it('keeps small pixel-mode vertical deltas available for trackpad pan', () => {
    expect(isMouseWheelGesture(wheel({ deltaY: 6 }))).toBe(false)
  })

  it('does not classify horizontal pixel-mode gestures as notched wheel zooms', () => {
    const event = wheel({ deltaX: 8, deltaY: 120 })

    expect(isNotchedWheelGesture(event)).toBe(false)
    expect(isMouseWheelGesture(event)).toBe(false)
  })

  it('does not classify ctrl pixel-wheel gestures as mouse wheels', () => {
    const event = wheel({ ctrlKey: true, deltaY: 120 })

    expect(isNotchedWheelGesture(event)).toBe(false)
    expect(isMouseWheelGesture(event)).toBe(false)
  })

  it('does not classify fractional pixel-mode deltas as notched wheel events', () => {
    const event = wheel({ deltaY: 20.5 })

    expect(isNotchedWheelGesture(event)).toBe(false)
    expect(isMouseWheelGesture(event)).toBe(false)
  })

  it('normalizes line and page wheel deltas to pixel-equivalent distances', () => {
    expect(normalizeWheelDeltaY(wheel({ deltaY: 3, deltaMode: 1 }))).toBe(120)
    expect(normalizeWheelDeltaY(wheel({ deltaY: -1, deltaMode: 2 }))).toBe(-800)
  })

  it('leaves pixel and unknown wheel delta modes unchanged', () => {
    expect(normalizeWheelDeltaY(wheel({ deltaY: 12, deltaMode: 0 }))).toBe(12)
    expect(normalizeWheelDeltaY(wheel({ deltaY: -12, deltaMode: 99 }))).toBe(-12)
  })

  it('uses normalized mouse-wheel deltas for ZUI zoom factors', () => {
    expect(wheelZoomFactor(wheel({ deltaY: 3, deltaMode: 1 }), true)).toBe(0.85)
    expect(wheelZoomFactor(wheel({ deltaY: -3, deltaMode: 1 }), true)).toBe(1.15)
  })

  it('preserves moderate normalized mouse-wheel zoom steps before clamping', () => {
    expect(wheelZoomFactor(wheel({ deltaY: 1, deltaMode: 1 }), true)).toBeCloseTo(0.92)
    expect(wheelZoomFactor(wheel({ deltaY: -1, deltaMode: 1 }), true)).toBeCloseTo(1.08)
  })

  it('uses raw deltas for pinch-style wheel zoom factors', () => {
    expect(wheelZoomFactor(wheel({ deltaY: 5 }), false)).toBeCloseTo(0.95)
    expect(wheelZoomFactor(wheel({ deltaY: -5 }), false)).toBeCloseTo(1.05)
  })

  it('clamps very large zoom factors in both directions', () => {
    expect(wheelZoomFactor(wheel({ deltaY: 400 }), true)).toBe(0.85)
    expect(wheelZoomFactor(wheel({ deltaY: -400 }), true)).toBe(1.15)
    expect(wheelZoomFactor(wheel({ deltaY: 40 }), false)).toBe(0.85)
    expect(wheelZoomFactor(wheel({ deltaY: -40 }), false)).toBe(1.15)
  })
})
