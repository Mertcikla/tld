export type CanvasInputCapabilities = {
  anyFinePointer: boolean
  anyCoarsePointer: boolean
}

export type CanvasWheelPanOptions = {
  isMobileLayout: boolean
  touchOnlyInput: boolean
}

export function isTouchOnlyCanvasInput({ anyFinePointer, anyCoarsePointer }: CanvasInputCapabilities): boolean {
  return anyCoarsePointer && !anyFinePointer
}

export function shouldEnableCanvasWheelPan({ isMobileLayout, touchOnlyInput }: CanvasWheelPanOptions): boolean {
  return !isMobileLayout || !touchOnlyInput
}

export function getCanvasInputCapabilities(): CanvasInputCapabilities {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return { anyFinePointer: true, anyCoarsePointer: false }
  }

  const anyFinePointer = window.matchMedia('(any-pointer: fine)').matches ||
    window.matchMedia('(pointer: fine)').matches
  const anyCoarsePointer = window.matchMedia('(any-pointer: coarse)').matches ||
    window.matchMedia('(pointer: coarse)').matches

  return { anyFinePointer, anyCoarsePointer }
}
