import type { CSSProperties } from 'react'
import { Position } from 'reactflow'

export const HANDLE_SLOT_COUNT = 5
export const HANDLE_SLOT_GAP = 12
export const HANDLE_SLOT_CENTER_INDEX = Math.floor(HANDLE_SLOT_COUNT / 2)

export type LogicalHandleSide = 'top' | 'bottom' | 'left' | 'right'

export const DEFAULT_SOURCE_HANDLE_SIDE: LogicalHandleSide = 'right'
export const DEFAULT_TARGET_HANDLE_SIDE: LogicalHandleSide = 'left'

function clampSlot(slot: number) {
  return Math.max(0, Math.min(HANDLE_SLOT_COUNT - 1, slot))
}

export function getLogicalHandleId(
  handleId: string | null | undefined,
  fallback: LogicalHandleSide | null = null,
): LogicalHandleSide | null {
  if (!handleId) return fallback
  const side = handleId.split('-', 1)[0]
  if (side === 'top' || side === 'bottom' || side === 'left' || side === 'right') return side
  return fallback
}

export function getVisualHandleSlot(groupIndex: number, groupCount: number) {
  if (groupIndex < 0) return HANDLE_SLOT_CENTER_INDEX
  if (groupCount <= 1) return HANDLE_SLOT_CENTER_INDEX
  return clampSlot(Math.round((groupIndex * (HANDLE_SLOT_COUNT - 1)) / (groupCount - 1)))
}

export function getVisualHandleSlotFromId(handleId: string | null | undefined) {
  if (!handleId) return null
  const parts = handleId.split('-')
  if (parts.length < 2) return null
  const slot = Number(parts[1])
  return Number.isInteger(slot) ? clampSlot(slot) : null
}

export function getVisualHandleId(side: LogicalHandleSide, slot: number) {
  return `${side}-${clampSlot(slot)}`
}

export function getVisualHandleIdForGroup(side: LogicalHandleSide, groupIndex: number, groupCount: number) {
  return getVisualHandleId(side, getVisualHandleSlot(groupIndex, groupCount))
}

export function ensureVisualHandleId(
  handleId: string | null | undefined,
  fallback: LogicalHandleSide,
) {
  const side = getLogicalHandleId(handleId, fallback)
  if (!side) return null
  const slot = getVisualHandleSlotFromId(handleId) ?? HANDLE_SLOT_CENTER_INDEX
  return getVisualHandleId(side, slot)
}

export function getHandleSlotOffset(slot: number) {
  return (clampSlot(slot) - HANDLE_SLOT_CENTER_INDEX) * HANDLE_SLOT_GAP
}

export function getHandleSlotOffsetFromId(handleId: string | null | undefined) {
  const slot = getVisualHandleSlotFromId(handleId)
  if (slot === null) return 0
  return getHandleSlotOffset(slot)
}

export function getVisualHandleStyle(position: Position, slot: number): CSSProperties {
  const offset = getHandleSlotOffset(slot)

  switch (position) {
    case Position.Top:
    case Position.Bottom:
      return { left: `calc(50% + ${offset}px)` }
    case Position.Left:
    case Position.Right:
      return { top: `calc(50% + ${offset}px)` }
  }
}

export function getHandleFlowPosition(
  nodeX: number,
  nodeY: number,
  width: number,
  height: number,
  handleId: string | null | undefined,
  fallback: LogicalHandleSide,
) {
  const side = getLogicalHandleId(handleId, fallback) ?? fallback
  const offset = getHandleSlotOffsetFromId(handleId)

  switch (side) {
    case 'top':
      return { x: nodeX + width / 2 + offset, y: nodeY, side }
    case 'bottom':
      return { x: nodeX + width / 2 + offset, y: nodeY + height, side }
    case 'left':
      return { x: nodeX, y: nodeY + height / 2 + offset, side }
    case 'right':
      return { x: nodeX + width, y: nodeY + height / 2 + offset, side }
  }
}
