import { describe, expect, it } from 'vitest'
import { isTouchOnlyCanvasInput, shouldEnableCanvasWheelPan } from './canvasInputMode'

describe('canvas input mode', () => {
  it('treats coarse-only devices as touch-only canvas input', () => {
    expect(isTouchOnlyCanvasInput({ anyFinePointer: false, anyCoarsePointer: true })).toBe(true)
  })

  it('keeps desktop canvas interactions when a fine pointer is available', () => {
    expect(isTouchOnlyCanvasInput({ anyFinePointer: true, anyCoarsePointer: false })).toBe(false)
    expect(isTouchOnlyCanvasInput({ anyFinePointer: true, anyCoarsePointer: true })).toBe(false)
  })

  it('falls back to desktop canvas interactions when pointer capability is unknown', () => {
    expect(isTouchOnlyCanvasInput({ anyFinePointer: false, anyCoarsePointer: false })).toBe(false)
  })

  it('suppresses wheel panning only for touch-only mobile layout', () => {
    expect(shouldEnableCanvasWheelPan({ isMobileLayout: false, touchOnlyInput: true })).toBe(true)
    expect(shouldEnableCanvasWheelPan({ isMobileLayout: true, touchOnlyInput: false })).toBe(true)
    expect(shouldEnableCanvasWheelPan({ isMobileLayout: true, touchOnlyInput: true })).toBe(false)
  })
})
