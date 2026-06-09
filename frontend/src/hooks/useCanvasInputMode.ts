import { useEffect, useState } from 'react'
import { getCanvasInputCapabilities, isTouchOnlyCanvasInput } from '../utils/canvasInputMode'

type MediaQueryChangeListener = (event: MediaQueryListEvent) => void
type MediaQueryListWithLegacyListeners = MediaQueryList & {
  addListener?: (listener: MediaQueryChangeListener) => void
  removeListener?: (listener: MediaQueryChangeListener) => void
}

const CANVAS_INPUT_POINTER_QUERIES = [
  '(any-pointer: fine)',
  '(pointer: fine)',
  '(any-pointer: coarse)',
  '(pointer: coarse)',
]

function getTouchOnlyCanvasInput() {
  return isTouchOnlyCanvasInput(getCanvasInputCapabilities())
}

function addMediaQueryListener(mql: MediaQueryListWithLegacyListeners, listener: MediaQueryChangeListener) {
  if (typeof mql.addEventListener === 'function') {
    mql.addEventListener('change', listener)
    return
  }
  mql.addListener?.(listener)
}

function removeMediaQueryListener(mql: MediaQueryListWithLegacyListeners, listener: MediaQueryChangeListener) {
  if (typeof mql.removeEventListener === 'function') {
    mql.removeEventListener('change', listener)
    return
  }
  mql.removeListener?.(listener)
}

export function useTouchOnlyCanvasInput(): boolean {
  const [touchOnlyCanvasInput, setTouchOnlyCanvasInput] = useState(getTouchOnlyCanvasInput)

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return

    const mediaQueries = CANVAS_INPUT_POINTER_QUERIES.map((query) => window.matchMedia(query))
    const update = () => setTouchOnlyCanvasInput(getTouchOnlyCanvasInput())

    update()
    mediaQueries.forEach((mql) => addMediaQueryListener(mql, update))
    return () => mediaQueries.forEach((mql) => removeMediaQueryListener(mql, update))
  }, [])

  return touchOnlyCanvasInput
}
