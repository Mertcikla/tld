import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import type { ConcreteFloatingMenuPosition, FloatingMenuPosition } from '../types'

const FLOATING_MENU_MARGIN = 8
const FLOATING_MENU_DEFAULT_Y = 56

function sameFloatingMenuPosition(left: FloatingMenuPosition, right: FloatingMenuPosition) {
  return left.x === right.x && left.y === right.y
}

export function useFloatingMarkdownMenu() {
  const panelBodyRef = useRef<HTMLDivElement | null>(null)
  const floatingMenuRef = useRef<HTMLDivElement | null>(null)
  const floatingMenuDragRef = useRef<{ pointerId: number; offsetX: number; offsetY: number } | null>(null)
  const [floatingMenuPosition, setFloatingMenuPosition] = useState<FloatingMenuPosition>(() => ({
    x: null,
    y: FLOATING_MENU_DEFAULT_Y,
  }))
  const [isFloatingMenuHidden, setIsFloatingMenuHidden] = useState(false)
  const [isFloatingMenuDragging, setIsFloatingMenuDragging] = useState(false)

  const clampFloatingMenuPosition = useCallback((position: ConcreteFloatingMenuPosition): ConcreteFloatingMenuPosition => {
    const panelRect = panelBodyRef.current?.getBoundingClientRect()
    if (!panelRect) return position

    const menuRect = floatingMenuRef.current?.getBoundingClientRect()
    const maxMenuWidth = Math.max(0, panelRect.width - FLOATING_MENU_MARGIN * 2)
    const maxMenuHeight = Math.max(0, panelRect.height - FLOATING_MENU_MARGIN * 2)
    const menuWidth = Math.min(menuRect?.width ?? 280, maxMenuWidth)
    const menuHeight = Math.min(menuRect?.height ?? 36, maxMenuHeight)
    const maxX = Math.max(FLOATING_MENU_MARGIN, panelRect.width - menuWidth - FLOATING_MENU_MARGIN)
    const maxY = Math.max(FLOATING_MENU_MARGIN, panelRect.height - menuHeight - FLOATING_MENU_MARGIN)

    return {
      x: Math.min(Math.max(position.x, FLOATING_MENU_MARGIN), maxX),
      y: Math.min(Math.max(position.y, FLOATING_MENU_MARGIN), maxY),
    }
  }, [])

  const handleFloatingMenuDragStart = useCallback((event: ReactPointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0) return

    const panelRect = panelBodyRef.current?.getBoundingClientRect()
    const menuRect = floatingMenuRef.current?.getBoundingClientRect()
    if (!panelRect || !menuRect) return

    floatingMenuDragRef.current = {
      pointerId: event.pointerId,
      offsetX: event.clientX - menuRect.left,
      offsetY: event.clientY - menuRect.top,
    }
    setFloatingMenuPosition(clampFloatingMenuPosition({
      x: menuRect.left - panelRect.left,
      y: menuRect.top - panelRect.top,
    }))
    setIsFloatingMenuDragging(true)
    event.preventDefault()
    event.stopPropagation()
  }, [clampFloatingMenuPosition])

  useEffect(() => {
    if (!isFloatingMenuDragging) return

    const handlePointerMove = (event: PointerEvent) => {
      const dragState = floatingMenuDragRef.current
      const panelRect = panelBodyRef.current?.getBoundingClientRect()
      if (!dragState || dragState.pointerId !== event.pointerId || !panelRect) return

      event.preventDefault()
      setFloatingMenuPosition(clampFloatingMenuPosition({
        x: event.clientX - panelRect.left - dragState.offsetX,
        y: event.clientY - panelRect.top - dragState.offsetY,
      }))
    }

    const stopDragging = (event: PointerEvent) => {
      const dragState = floatingMenuDragRef.current
      if (dragState && dragState.pointerId !== event.pointerId) return
      floatingMenuDragRef.current = null
      setIsFloatingMenuDragging(false)
    }

    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', stopDragging)
    window.addEventListener('pointercancel', stopDragging)

    return () => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', stopDragging)
      window.removeEventListener('pointercancel', stopDragging)
    }
  }, [clampFloatingMenuPosition, isFloatingMenuDragging])

  useEffect(() => {
    const clampCurrentPosition = () => {
      setFloatingMenuPosition((current) => {
        if (current.x === null) return current
        const next = clampFloatingMenuPosition({ x: current.x, y: current.y })
        return sameFloatingMenuPosition(current, next) ? current : next
      })
    }

    clampCurrentPosition()
    const observer = typeof ResizeObserver === 'undefined'
      ? null
      : new ResizeObserver(clampCurrentPosition)
    if (panelBodyRef.current) observer?.observe(panelBodyRef.current)
    if (floatingMenuRef.current) observer?.observe(floatingMenuRef.current)
    window.addEventListener('resize', clampCurrentPosition)
    return () => {
      observer?.disconnect()
      window.removeEventListener('resize', clampCurrentPosition)
    }
  }, [clampFloatingMenuPosition, isFloatingMenuHidden])

  return {
    panelBodyRef,
    floatingMenuRef,
    floatingMenuPosition,
    isFloatingMenuHidden,
    setIsFloatingMenuHidden,
    isFloatingMenuDragging,
    handleFloatingMenuDragStart,
  }
}
