const DOM_DELTA_PIXEL = 0
const DOM_DELTA_LINE = 1
const DOM_DELTA_PAGE = 2

const WHEEL_LINE_HEIGHT_PX = 40
const WHEEL_PAGE_HEIGHT_PX = 800
const MOUSE_WHEEL_ZOOM_RATE = 0.002
const PINCH_WHEEL_ZOOM_RATE = 0.01
const MIN_WHEEL_ZOOM_FACTOR = 0.85
const MAX_WHEEL_ZOOM_FACTOR = 1.15

export type WheelDeltaLike = {
  deltaX: number
  deltaY: number
  deltaMode: number
  ctrlKey: boolean
}

export function normalizeWheelDeltaY(event: Pick<WheelDeltaLike, 'deltaY' | 'deltaMode'>): number {
  switch (event.deltaMode) {
    case DOM_DELTA_LINE:
      return event.deltaY * WHEEL_LINE_HEIGHT_PX
    case DOM_DELTA_PAGE:
      return event.deltaY * WHEEL_PAGE_HEIGHT_PX
    case DOM_DELTA_PIXEL:
    default:
      return event.deltaY
  }
}

export function isNotchedWheelGesture(event: WheelDeltaLike): boolean {
  return !event.ctrlKey && event.deltaX === 0 && Number.isInteger(event.deltaY) && Math.abs(event.deltaY) >= 20
}

export function isMouseWheelGesture(event: WheelDeltaLike): boolean {
  return event.deltaMode !== DOM_DELTA_PIXEL || isNotchedWheelGesture(event)
}

export function wheelZoomFactor(event: Pick<WheelDeltaLike, 'deltaY' | 'deltaMode'>, isMouseWheel: boolean): number {
  const deltaY = isMouseWheel ? normalizeWheelDeltaY(event) : event.deltaY
  const rate = isMouseWheel ? MOUSE_WHEEL_ZOOM_RATE : PINCH_WHEEL_ZOOM_RATE
  const factor = 1 - deltaY * rate
  return Math.max(MIN_WHEEL_ZOOM_FACTOR, Math.min(MAX_WHEEL_ZOOM_FACTOR, factor))
}
